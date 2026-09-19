package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/redis/go-redis/v9"
)

type cutPublicationHook struct {
	fired bool
	rdb   *redis.Client
}

func (h *cutPublicationHook) DialHook(next redis.DialHook) redis.DialHook          { return next }
func (h *cutPublicationHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook { return next }
func (h *cutPublicationHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		if !h.fired {
			for _, cmd := range cmds {
				if cmd.Name() == "setrange" {
					h.fired = true
					if err := h.rdb.Do(ctx, cmd.Args()...).Err(); err != nil {
						return err
					}
					return io.ErrUnexpectedEOF
				}
			}
		}
		return next(ctx, cmds)
	}
}

func TestInterruptedChunkPublicationKeepsPreviousBytes(t *testing.T) {
	rdb, ctx := setupTestRedis(t)
	reader := New(rdb, "atomic-cut")
	old := []byte("aaaabbbbcccc")
	if err := reader.Echo(ctx, "/file", old); err != nil {
		t.Fatal(err)
	}
	writerRedis := redis.NewClient(rdb.Options())
	t.Cleanup(func() { _ = writerRedis.Close() })
	hook := &cutPublicationHook{rdb: rdb}
	writerRedis.AddHook(hook)
	writer := New(writerRedis, "atomic-cut")
	err := writer.WriteChunks(ctx, "/file", map[int][]byte{0: []byte("AAAA"), 2: []byte("CCCC")}, 4, 12, nil)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("write error=%v; want injected disconnect", err)
	}
	got, err := reader.Cat(ctx, "/file")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(old) {
		t.Fatalf("interrupted publication exposed partial bytes %q; old %q", got, old)
	}
}

type publicationCommandHook struct {
	before func()
	after  func()
	fired  bool
}

func (h *publicationCommandHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h *publicationCommandHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (h *publicationCommandHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		publication := cmd.Name() == "eval" || cmd.Name() == "evalsha"
		if publication && !h.fired && h.before != nil {
			h.fired = true
			h.before()
		}
		err := next(ctx, cmd)
		if publication && !h.fired && h.after != nil && err == nil {
			h.fired = true
			h.after()
			cmd.SetErr(io.ErrUnexpectedEOF)
			return io.ErrUnexpectedEOF
		}
		return err
	}
}

func TestConcurrentPublicationRejectsStaleCandidate(t *testing.T) {
	rdb, ctx := setupTestRedis(t)
	peer := New(rdb, "concurrent-publish")
	if err := peer.Echo(ctx, "/file", []byte("base")); err != nil {
		t.Fatal(err)
	}
	writerRedis := redis.NewClient(rdb.Options())
	t.Cleanup(func() { _ = writerRedis.Close() })
	hook := &publicationCommandHook{before: func() {
		if err := peer.Echo(ctx, "/file", []byte("peer")); err != nil {
			t.Fatal(err)
		}
	}}
	writerRedis.AddHook(hook)
	err := New(writerRedis, "concurrent-publish").Echo(ctx, "/file", []byte("mine"))
	if !errors.Is(err, ErrWriteConflict) {
		t.Fatalf("write error=%v; want conflict", err)
	}
	got, err := peer.Cat(ctx, "/file")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "peer" {
		t.Fatalf("concurrent peer bytes lost: %q", got)
	}
}

func TestLostPublicationAcknowledgementDoesNotReplayOverNewerFile(t *testing.T) {
	for _, newer := range []bool{false, true} {
		t.Run(fmt.Sprintf("newer_%v", newer), func(t *testing.T) {
			rdb, ctx := setupTestRedis(t)
			peer := New(rdb, "lost-ack")
			if err := peer.Echo(ctx, "/file", []byte("base")); err != nil {
				t.Fatal(err)
			}
			expected, err := peer.Stat(ctx, "/file")
			if err != nil {
				t.Fatal(err)
			}
			writerRedis := redis.NewClient(rdb.Options())
			t.Cleanup(func() { _ = writerRedis.Close() })
			hook := &publicationCommandHook{after: func() {
				if newer {
					if err := peer.Echo(ctx, "/file", []byte("newer")); err != nil {
						t.Fatal(err)
					}
				}
			}}
			writerRedis.AddHook(hook)
			writer := New(writerRedis, "lost-ack")
			writeCtx := WithExpectedStat(ctx, expected)
			err = writer.Echo(writeCtx, "/file", []byte("mine"))
			if !hook.fired {
				t.Fatal("lost acknowledgement was not injected")
			}
			if err != nil {
				t.Fatalf("committed publication not recognized: %v", err)
			}
			if newer {
				if err := writer.Echo(writeCtx, "/file", []byte("mine")); !errors.Is(err, ErrWriteConflict) {
					t.Fatalf("stale retry error=%v", err)
				}
			}
			got, err := peer.Cat(ctx, "/file")
			if err != nil {
				t.Fatal(err)
			}
			want := "mine"
			if newer {
				want = "newer"
			}
			if string(got) != want {
				t.Fatalf("got %q want %q", got, want)
			}
			info, err := peer.Info(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if info.Files != 1 || info.TotalDataBytes != int64(len(want)) {
				t.Fatalf("publication replay changed counters: %+v", info)
			}
		})
	}
}

func TestChunkPublicationCreatesCompleteNewFile(t *testing.T) {
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "new-chunks")
	if err := c.WriteChunks(WithExpectedStat(ctx, nil), "/nested/file", map[int][]byte{0: []byte("abcd"), 1: []byte("ef")}, 4, 6, nil); err != nil {
		t.Fatal(err)
	}
	got, err := c.Cat(ctx, "/nested/file")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "abcdef" {
		t.Fatalf("got %q", got)
	}
	if err := c.WriteChunks(WithExpectedStat(ctx, nil), "/nested/incomplete", map[int][]byte{1: []byte("ef")}, 4, 6, nil); err == nil {
		t.Fatal("incomplete new file accepted")
	}
	stat, err := c.Stat(ctx, "/nested/incomplete")
	if err != nil {
		t.Fatal(err)
	}
	if stat != nil {
		t.Fatal("incomplete file became visible")
	}
}

func TestDeleteRejectsConcurrentRemoteEdit(t *testing.T) {
	rdb, ctx := setupTestRedis(t)
	peer := New(rdb, "delete-edit")
	if err := peer.Echo(ctx, "/file", []byte("base")); err != nil {
		t.Fatal(err)
	}
	expected, err := peer.Stat(ctx, "/file")
	if err != nil {
		t.Fatal(err)
	}
	writerRedis := redis.NewClient(rdb.Options())
	t.Cleanup(func() { _ = writerRedis.Close() })
	writerRedis.AddHook(&publicationCommandHook{before: func() {
		if err := peer.Echo(ctx, "/file", []byte("edited")); err != nil {
			t.Fatal(err)
		}
	}})
	if err := New(writerRedis, "delete-edit").Rm(WithExpectedStat(ctx, expected), "/file"); !errors.Is(err, ErrWriteConflict) {
		t.Fatalf("delete error=%v", err)
	}
	got, err := peer.Cat(ctx, "/file")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "edited" {
		t.Fatalf("remote edit lost: %q", got)
	}
}

type generationBoundaryHook struct {
	fired   bool
	advance func()
}

func (h *generationBoundaryHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h *generationBoundaryHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if !h.fired && (cmd.Name() == "eval" || cmd.Name() == "evalsha") {
			h.fired = true
			h.advance()
		}
		return next(ctx, cmd)
	}
}
func (h *generationBoundaryHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		if !h.fired {
			for _, cmd := range cmds {
				if cmd.Name() == "hset" || cmd.Name() == "hdel" {
					h.fired = true
					h.advance()
					break
				}
			}
		}
		return next(ctx, cmds)
	}
}

func TestWorkspaceGenerationChangesAtMutationBoundary(t *testing.T) {
	mutations := map[string]func(context.Context, Client) error{
		"put": func(ctx context.Context, c Client) error { return c.Echo(ctx, "/file", []byte("stale")) },
		"chunks": func(ctx context.Context, c Client) error {
			return c.WriteChunks(ctx, "/file", map[int][]byte{0: []byte("bad!")}, 4, 4, nil)
		},
		"delete":  func(ctx context.Context, c Client) error { return c.Rm(ctx, "/file") },
		"rename":  func(ctx context.Context, c Client) error { return c.Mv(ctx, "/file", "/renamed") },
		"chmod":   func(ctx context.Context, c Client) error { return c.Chmod(ctx, "/file", 0o600) },
		"mkdir":   func(ctx context.Context, c Client) error { return c.Mkdir(ctx, "/new-dir") },
		"symlink": func(ctx context.Context, c Client) error { return c.Ln(ctx, "file", "/new-link") },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			rdb, ctx := setupTestRedis(t)
			peer := New(rdb, "generation-race")
			if err := peer.Echo(ctx, "/file", []byte("base")); err != nil {
				t.Fatal(err)
			}
			generationKey := newKeyBuilder("generation-race").generation()
			if err := rdb.Set(ctx, generationKey, "g_before", 0).Err(); err != nil {
				t.Fatal(err)
			}
			writerRedis := redis.NewClient(rdb.Options())
			t.Cleanup(func() { _ = writerRedis.Close() })
			hook := &generationBoundaryHook{advance: func() {
				if err := rdb.Set(ctx, generationKey, "g_after", 0).Err(); err != nil {
					t.Fatal(err)
				}
			}}
			writerRedis.AddHook(hook)
			err := mutate(WithWorkspaceGeneration(ctx, "g_before"), New(writerRedis, "generation-race"))
			if !hook.fired {
				t.Fatal("generation boundary not reached")
			}
			if !errors.Is(err, ErrWorkspaceChanged) {
				t.Fatalf("stale mutation error=%v", err)
			}
			got, err := peer.Cat(ctx, "/file")
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != "base" {
				t.Fatalf("stale bytes published: %q", got)
			}
			stat, err := peer.Stat(ctx, "/file")
			if err != nil {
				t.Fatal(err)
			}
			if stat.Mode != 0o644 {
				t.Fatalf("stale mode published: %o", stat.Mode)
			}
			names, err := peer.Ls(ctx, "/")
			if err != nil {
				t.Fatal(err)
			}
			if len(names) != 1 || names[0] != "file" {
				t.Fatalf("stale tree mutation: %v", names)
			}
		})
	}
}

// Discard the first successful publication reply and resend the identical
// command, as a transport retry would. A peer may delete between the attempts.
type replayPublicationHook struct {
	peer              Client
	peerContext       context.Context
	deleteBeforeRetry bool
	replayed          bool
}

func (h *replayPublicationHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h *replayPublicationHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (h *replayPublicationHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		err := next(ctx, cmd)
		if h.replayed || err != nil || (cmd.Name() != "eval" && cmd.Name() != "evalsha") {
			return err
		}
		h.replayed = true
		if h.deleteBeforeRetry {
			if err := h.peer.Rm(h.peerContext, "/file"); err != nil {
				return err
			}
		}
		return next(ctx, cmd)
	}
}

func TestCreatePublicationRetryPreservesConcurrentDeletion(t *testing.T) {
	for _, content := range []string{"", "hello"} {
		for _, deleted := range []bool{false, true} {
			t.Run(fmt.Sprintf("size_%d/deleted_%v", len(content), deleted), func(t *testing.T) {
				rdb, ctx := setupTestRedis(t)
				const fsKey = "create-replay"
				peer := New(rdb, fsKey)
				writerRedis := redis.NewClient(rdb.Options())
				t.Cleanup(func() { _ = writerRedis.Close() })
				hook := &replayPublicationHook{peer: peer, peerContext: ctx, deleteBeforeRetry: deleted}
				writerRedis.AddHook(hook)
				err := New(writerRedis, fsKey).Echo(WithExpectedStat(ctx, nil), "/file", []byte(content))
				if !hook.replayed {
					t.Fatal("publication was not replayed")
				}
				stat, statErr := peer.Stat(ctx, "/file")
				if statErr != nil {
					t.Fatal(statErr)
				}
				if deleted {
					if stat != nil {
						t.Fatalf("publication retry recreated a deleted file: %+v", stat)
					}
					if err != nil {
						t.Fatalf("committed operation was not acknowledged: %v", err)
					}
				} else {
					if err != nil || stat == nil {
						t.Fatalf("live same-token publication was not recognized: stat=%+v, err=%v", stat, err)
					}
					data, err := peer.Cat(ctx, "/file")
					if err != nil || string(data) != content {
						t.Fatalf("replayed content=%q, err=%v", data, err)
					}
				}
				info, err := peer.Info(ctx)
				if err != nil {
					t.Fatal(err)
				}
				files, size := int64(1), int64(len(content))
				if deleted {
					files, size = 0, 0
				}
				if info.Files != files || info.TotalDataBytes != size {
					t.Fatalf("retry changed counters: %+v, want files=%d bytes=%d", info, files, size)
				}
			})
		}
	}
}

func TestEmptyFilePublicationPaths(t *testing.T) {
	rdb, ctx := setupTestRedis(t)
	checkEmptyFilePublicationPaths(t, rdb, ctx)
}

func TestArrayEmptyFilePublicationPaths(t *testing.T) {
	rdb, ctx := setupArrayRedisFromEnv(t)
	checkEmptyFilePublicationPaths(t, rdb, ctx)
}

func checkEmptyFilePublicationPaths(t *testing.T, rdb *redis.Client, ctx context.Context) {
	t.Helper()
	c := New(rdb, "empty-publication-paths")
	if err := c.Echo(ctx, "/empty", nil); err != nil {
		t.Fatal(err)
	}
	if err := c.WriteChunks(ctx, "/new-empty", nil, 0, 0, nil); err != nil {
		t.Fatalf("create empty through chunks: %v", err)
	}
	if err := c.WriteChunks(ctx, "/empty", nil, 0, 0, nil); err != nil {
		t.Fatalf("clear empty chunk metadata: %v", err)
	}
	if err := c.WriteChunks(ctx, "/empty", map[int][]byte{0: []byte("data")}, 4, 4, nil); err != nil {
		t.Fatalf("grow empty file: %v", err)
	}
	if data, err := c.Cat(ctx, "/empty"); err != nil || string(data) != "data" {
		t.Fatalf("grown file=%q, err=%v", data, err)
	}
	if err := c.WriteChunks(ctx, "/empty", nil, 4, 0, nil); err != nil {
		t.Fatalf("truncate through chunks: %v", err)
	}
	for _, path := range []string{"/empty", "/new-empty"} {
		data, err := c.Cat(ctx, path)
		if err != nil || len(data) != 0 {
			t.Fatalf("empty file %s=%q, err=%v", path, data, err)
		}
	}
}
