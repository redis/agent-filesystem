package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func setupNativeSession(t *testing.T, rdb *redis.Client, ctx context.Context, key string) NativeClient {
	t.Helper()
	if err := New(rdb, key).Mkdir(ctx, "/"); err != nil {
		t.Fatal(err)
	}
	if err := rdb.SetNX(ctx, newKeyBuilder(key).generation(), "g_1", 0).Err(); err != nil {
		t.Fatal(err)
	}
	c, err := NewNativeWithCache(ctx, rdb, key, "g_1", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestNativeRangeConcurrentDisjointAndOverlap(t *testing.T) {
	for _, overlap := range []bool{false, true} {
		t.Run(fmt.Sprint(overlap), func(t *testing.T) {
			rdb, ctx := setupTestRedis(t)
			a := setupNativeSession(t, rdb, ctx, "native-ranges")
			b := setupNativeSession(t, rdb, ctx, "native-ranges")
			if err := a.Echo(ctx, "/f", bytes.Repeat([]byte("0"), 8192)); err != nil {
				t.Fatal(err)
			}
			st, _ := a.Stat(ctx, "/f")
			start := make(chan struct{})
			errs := make(chan error, 2)
			for i, c := range []NativeClient{a, b} {
				go func(i int, c NativeClient) {
					<-start
					off := int64(i * 4096)
					if overlap {
						off = 0
					}
					for j := 0; j < 20; j++ {
						if err := c.WriteInodeAtPath(ctx, st.Inode, "/f", bytes.Repeat([]byte{byte('A' + i)}, 4096), off); err != nil {
							errs <- err
							return
						}
					}
					errs <- nil
				}(i, c)
			}
			close(start)
			for i := 0; i < 2; i++ {
				if err := <-errs; err != nil {
					t.Fatal(err)
				}
			}
			got, err := a.ReadInodeAt(ctx, st.Inode, 0, 8192)
			if err != nil {
				t.Fatal(err)
			}
			if !overlap {
				if !bytes.Equal(got, append(bytes.Repeat([]byte("A"), 4096), bytes.Repeat([]byte("B"), 4096)...)) {
					t.Fatal("disjoint acknowledged writes lost")
				}
			} else {
				if !bytes.Equal(got[:4096], bytes.Repeat(got[:1], 4096)) || (got[0] != 'A' && got[0] != 'B') {
					t.Fatal("overlapping write was torn")
				}
			}
		})
	}
}

func TestNativeAtomicAppend(t *testing.T) {
	rdb, ctx := setupTestRedis(t)
	a := setupNativeSession(t, rdb, ctx, "append")
	b := setupNativeSession(t, rdb, ctx, "append")
	st, _, err := a.CreateFile(ctx, "/f", 0644, false)
	if err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 2)
	start := make(chan struct{})
	for i, c := range []NativeClient{a, b} {
		go func(i int, c NativeClient) {
			<-start
			for j := 0; j < 20; j++ {
				if err := c.WriteInodeAtPath(ctx, st.Inode, "/f", []byte(fmt.Sprintf("%d:%02d\n", i, j)), -1); err != nil {
					errs <- err
					return
				}
			}
			errs <- nil
		}(i, c)
	}
	close(start)
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	got, err := a.Cat(ctx, "/f")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(got), "\n"), "\n")
	seen := map[string]bool{}
	for _, line := range lines {
		seen[line] = true
	}
	if len(lines) != 40 || len(seen) != 40 {
		t.Fatalf("append records lost or duplicated: %q", got)
	}
}

func TestNativeTruncateAndRenameHandle(t *testing.T) {
	rdb, ctx := setupTestRedis(t)
	c := setupNativeSession(t, rdb, ctx, "truncate")
	if err := c.Echo(ctx, "/old", []byte("abcdef")); err != nil {
		t.Fatal(err)
	}
	st, _ := c.Stat(ctx, "/old")
	if err := c.Rename(ctx, "/old", "/new", 0); err != nil {
		t.Fatal(err)
	}
	if err := c.WriteInodeAtPath(ctx, st.Inode, "/old", []byte("XY"), 2); err != nil {
		t.Fatal(err)
	}
	if err := c.TruncateInodeAtPath(ctx, st.Inode, "/old", 3); err != nil {
		t.Fatal(err)
	}
	if err := c.TruncateInodeAtPath(ctx, st.Inode, "/old", 6); err != nil {
		t.Fatal(err)
	}
	got, err := c.Cat(ctx, "/new")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte{'a', 'b', 'X', 0, 0, 0}) {
		t.Fatalf("truncate bytes %q", got)
	}
	if old, _ := c.Stat(ctx, "/old"); old != nil {
		t.Fatal("stale path hint recreated old name")
	}
	path, err := rdb.HGet(ctx, newKeyBuilder("truncate").inode(fmt.Sprint(st.Inode)), "path").Result()
	if err != nil || path != "/new" {
		t.Fatalf("canonical path %q %v", path, err)
	}
}

func TestNativeExpectedStatAndMixedSyncPublication(t *testing.T) {
	rdb, ctx := setupTestRedis(t)
	native := setupNativeSession(t, rdb, ctx, "mixed")
	syncClient := New(rdb, "mixed")
	if err := native.Echo(ctx, "/f", []byte("aaaabbbb")); err != nil {
		t.Fatal(err)
	}
	observed, _ := syncClient.Stat(ctx, "/f")
	if err := native.WriteInodeAtPath(ctx, observed.Inode, "/f", []byte("NNNN"), 0); err != nil {
		t.Fatal(err)
	}
	if err := syncClient.WriteChunks(WithExpectedStat(ctx, observed), "/f", map[int][]byte{1: []byte("SSSS")}, 4, 8, nil); !errors.Is(err, ErrWriteConflict) {
		t.Fatalf("stale sync candidate: %v", err)
	}
	if err := native.WriteInodeAtPath(WithExpectedStat(ctx, observed), observed.Inode, "/f", []byte("stale"), 0); !errors.Is(err, ErrWriteConflict) {
		t.Fatalf("stale native observation: %v", err)
	}
	got, _ := native.Cat(ctx, "/f")
	if string(got) != "NNNNbbbb" {
		t.Fatalf("native bytes lost: %q", got)
	}
	fresh, _ := syncClient.Stat(ctx, "/f")
	if err := syncClient.WriteChunks(WithExpectedStat(ctx, fresh), "/f", map[int][]byte{1: []byte("SSSS")}, 4, 8, nil); err != nil {
		t.Fatal(err)
	}
	got, err := native.ReadInodeAt(ctx, fresh.Inode, 0, 8)
	if err != nil || string(got) != "NNNNSSSS" {
		t.Fatalf("native did not see sync: %q %v", got, err)
	}
	if err := native.WriteInodeAt(ctx, fresh.Inode, []byte("!"), 0); err != nil {
		t.Fatal(err)
	}
	size, hashes, err := syncClient.ChunkMeta(ctx, "/f")
	if err != nil || size != 0 || len(hashes) != 0 {
		t.Fatalf("stale chunk hashes retained: %d %v %v", size, hashes, err)
	}
}

func TestNativeStaleGenerationEveryRequest(t *testing.T) {
	rdb, ctx := setupTestRedis(t)
	c := setupNativeSession(t, rdb, ctx, "fenced")
	if err := c.Echo(ctx, "/f", []byte("old")); err != nil {
		t.Fatal(err)
	}
	st, _ := c.Stat(ctx, "/f")
	_, _ = c.LsLong(ctx, "/")
	if err := rdb.Set(ctx, newKeyBuilder("fenced").generation(), "g_2", 0).Err(); err != nil {
		t.Fatal(err)
	}
	tests := map[string]func() error{
		"stat cache": func() error { _, e := c.Stat(ctx, "/f"); return e }, "list cache": func() error { _, e := c.LsLong(ctx, "/"); return e }, "cat": func() error { _, e := c.Cat(ctx, "/f"); return e }, "info": func() error { _, e := c.Info(ctx); return e },
		"inode stat": func() error { _, e := c.StatInode(ctx, st.Inode); return e }, "inode read": func() error { _, e := c.ReadInodeAt(ctx, st.Inode, 0, 3); return e }, "range write": func() error { return c.WriteInodeAt(ctx, st.Inode, []byte("bad"), 0) }, "truncate": func() error { return c.TruncateInode(ctx, st.Inode, 0) }, "create": func() error { return c.Echo(ctx, "/new", []byte("bad")) }, "metadata": func() error { return c.Chmod(ctx, "/f", 0600) }, "delete": func() error { return c.Rm(ctx, "/f") }, "barrier": func() error { return c.Barrier(ctx) }, "lock": func() error {
			return c.Setlk(ctx, st.Inode, "owner", &FileLock{Type: syscall.F_WRLCK, End: 100}, false)
		},
	}
	for name, run := range tests {
		t.Run(name, func(t *testing.T) {
			if err := run(); !errors.Is(err, ErrWorkspaceChanged) {
				t.Fatalf("stale request: %v", err)
			}
		})
	}
	got, _ := New(rdb, "fenced").Cat(ctx, "/f")
	if string(got) != "old" {
		t.Fatalf("stale mutation: %q", got)
	}
}

func TestNativeLocksOwnersPartialUnlockAndCrashExpiry(t *testing.T) {
	rdb, ctx := setupTestRedis(t)
	a := setupNativeSession(t, rdb, ctx, "locks")
	b := setupNativeSession(t, rdb, ctx, "locks")
	st, _, err := a.CreateFile(ctx, "/f", 0644, false)
	if err != nil {
		t.Fatal(err)
	}
	lock := &FileLock{Start: 0, End: 99, Type: syscall.F_WRLCK, PID: 123}
	if err := a.Setlk(ctx, st.Inode, "same-local-owner", lock, false); err != nil {
		t.Fatal(err)
	}
	if err := b.Setlk(ctx, st.Inode, "same-local-owner", lock, false); !errors.Is(err, ErrLockWouldBlock) {
		t.Fatalf("mount owners collided: %v", err)
	}
	if err := a.Setlk(ctx, st.Inode, "same-local-owner", &FileLock{Start: 20, End: 29, Type: syscall.F_UNLCK}, false); err != nil {
		t.Fatal(err)
	}
	if err := b.Setlk(ctx, st.Inode, "same-local-owner", &FileLock{Start: 20, End: 29, Type: syscall.F_WRLCK}, false); err != nil {
		t.Fatal(err)
	}
	conflict, err := b.Getlk(ctx, st.Inode, "same-local-owner", lock)
	if err != nil || conflict.Type != syscall.F_WRLCK || conflict.PID != 123 {
		t.Fatalf("conflict=%+v %v", conflict, err)
	}
	s := a.(*nativeSession)
	if err := rdb.PExpire(ctx, s.lease, time.Millisecond).Err(); err != nil {
		t.Fatal(err)
	}
	for rdb.Exists(ctx, s.lease).Val() != 0 {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Millisecond):
		}
	}
	if err := b.Setlk(ctx, st.Inode, "same-local-owner", lock, false); err != nil {
		t.Fatalf("crashed owner still blocks: %v", err)
	}
	if err := a.WriteInodeAt(ctx, st.Inode, []byte("stale"), 0); !errors.Is(err, ErrNativeSessionLost) {
		t.Fatalf("expired writer resumed: %v", err)
	}
	if err := b.UnlockAll(ctx, st.Inode, "same-local-owner"); err != nil {
		t.Fatal(err)
	}
}

func TestNativeBarrierTimeoutResumesAdmission(t *testing.T) {
	rdb, ctx := setupTestRedis(t)
	c := setupNativeSession(t, rdb, ctx, "barrier").(*nativeSession)
	_, finish, err := c.begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	short, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	if err := c.Barrier(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("barrier ignored inflight request: %v", err)
	}
	if err := finish(nil); err != nil {
		t.Fatal(err)
	}
	if err := c.Mkdir(ctx, "/after"); err != nil {
		t.Fatalf("barrier left admission paused: %v", err)
	}
	if err := c.Barrier(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestNativeRangeReadersNeverSeeTornPublication(t *testing.T) {
	rdb, ctx := setupTestRedis(t)
	a := setupNativeSession(t, rdb, ctx, "readatomic")
	b := setupNativeSession(t, rdb, ctx, "readatomic")
	if err := a.Echo(ctx, "/f", bytes.Repeat([]byte("A"), 8192)); err != nil {
		t.Fatal(err)
	}
	st, _ := a.Stat(ctx, "/f")
	var wg sync.WaitGroup
	done := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 40; i++ {
			if err := a.WriteInodeAt(ctx, st.Inode, bytes.Repeat([]byte{byte('A' + i%2)}, 8192), 0); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	for i := 0; i < 40; i++ {
		got, err := b.ReadInodeAt(ctx, st.Inode, 0, 8192)
		if errors.Is(err, ErrWriteConflict) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 8192 || !bytes.Equal(got, bytes.Repeat(got[:1], 8192)) {
			t.Fatal("reader observed torn published write")
		}
	}
	wg.Wait()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestNativeRangePublicationFencesInFlightReplacement(t *testing.T) {
	for _, action := range []string{"restore", "delete", "lease", "unlink"} {
		t.Run(action, func(t *testing.T) {
			rdb, ctx := setupTestRedis(t)
			seed := setupNativeSession(t, rdb, ctx, "midflight")
			if err := seed.Echo(ctx, "/f", []byte("previous")); err != nil {
				t.Fatal(err)
			}
			st, _ := seed.Stat(ctx, "/f")
			wr := redis.NewClient(rdb.Options())
			t.Cleanup(func() { _ = wr.Close() })
			writer, err := NewNative(ctx, wr, "midflight", "g_1")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = writer.Close() })
			hook := &publicationCommandHook{before: func() {
				switch action {
				case "restore":
					if err := rdb.Set(ctx, newKeyBuilder("midflight").generation(), "g_2", 0).Err(); err != nil {
						t.Fatal(err)
					}
				case "delete":
					if err := rdb.Set(ctx, newKeyBuilder("midflight").generation(), "deleted", 0).Err(); err != nil {
						t.Fatal(err)
					}
				case "lease":
					if err := rdb.Del(ctx, writer.(*nativeSession).lease).Err(); err != nil {
						t.Fatal(err)
					}
				case "unlink":
					if err := seed.Rm(ctx, "/f"); err != nil {
						t.Fatal(err)
					}
				}
			}}
			wr.AddHook(&nativePublicationHook{hook})
			err = writer.WriteInodeAtPath(ctx, st.Inode, "/f", []byte("BAD"), 0)
			want := ErrWorkspaceChanged
			if action == "lease" {
				want = ErrNativeSessionLost
			}
			if action == "unlink" {
				want = ErrNotFound
			}
			if !errors.Is(err, want) {
				t.Fatalf("in-flight %s write=%v want %v", action, err, want)
			}
			if !hook.fired {
				t.Fatal("publication boundary not reached")
			}
			raw := newNativeClient(rdb, "midflight", nil)
			if action == "unlink" {
				if st, _ := raw.Stat(ctx, "/f"); st != nil {
					t.Fatal("unlinked inode resurrected")
				}
			} else {
				got, _ := raw.Cat(ctx, "/f")
				if string(got) != "previous" {
					t.Fatalf("stale write published %q", got)
				}
			}
		})
	}
}

func TestNativeRangeLostAcknowledgementDoesNotReplay(t *testing.T) {
	for _, newer := range []bool{false, true} {
		t.Run(fmt.Sprint(newer), func(t *testing.T) {
			rdb, ctx := setupTestRedis(t)
			seed := setupNativeSession(t, rdb, ctx, "range-ack")
			if err := seed.Echo(ctx, "/f", []byte("before")); err != nil {
				t.Fatal(err)
			}
			st, _ := seed.Stat(ctx, "/f")
			wr := redis.NewClient(rdb.Options())
			t.Cleanup(func() { _ = wr.Close() })
			writer, err := NewNative(ctx, wr, "range-ack", "g_1")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = writer.Close() })
			hook := &publicationCommandHook{after: func() {
				if newer {
					// The fixture writer deliberately observes the completed range
					// publication before committing the newer full-file version.
					seed.InvalidateCache()
					if err := seed.Echo(ctx, "/f", []byte("newest")); err != nil {
						t.Fatal(err)
					}
				}
			}}
			wr.AddHook(&nativePublicationHook{hook})
			err = writer.WriteInodeAt(ctx, st.Inode, []byte("RANGE!"), 0)
			if !hook.fired {
				t.Fatal("lost acknowledgement not injected")
			}
			if err != nil {
				t.Fatalf("committed write not recognized: %v", err)
			}
			got, _ := New(rdb, "range-ack").Cat(ctx, "/f")
			want := "RANGE!"
			if newer {
				want = "newest"
			}
			if string(got) != want {
				t.Fatalf("lost acknowledgement replayed over latest bytes %q", got)
			}
		})
	}
}

func TestNativeRangeKeepsUnchangedBytesOnRedis(t *testing.T) {
	rdb, ctx := setupTestRedis(t)
	c := setupNativeSession(t, rdb, ctx, "bandwidth")
	if err := c.Echo(ctx, "/f", bytes.Repeat([]byte("z"), 1024*1024)); err != nil {
		t.Fatal(err)
	}
	st, _ := c.Stat(ctx, "/f")
	var mu sync.Mutex
	copies, ranges, fullReads := 0, 0, 0
	rdb.AddHook(&commandCountHook{track: func(cmd redis.Cmder) {
		mu.Lock()
		defer mu.Unlock()
		switch cmd.Name() {
		case "copy":
			copies++
		case "setrange":
			ranges++
		case "get":
			if len(cmd.Args()) > 1 && strings.Contains(fmt.Sprint(cmd.Args()[1]), ":content:") {
				fullReads++
			}
		}
	}})
	if err := c.WriteInodeAt(ctx, st.Inode, []byte("small edit"), 1000); err != nil {
		t.Fatal(err)
	}
	if err := c.TruncateInode(ctx, st.Inode, 512*1024); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if copies < 2 || ranges < 1 || fullReads != 0 {
		t.Fatalf("range path copies=%d ranges=%d fullReads=%d", copies, ranges, fullReads)
	}
}

func TestNativeReadLegacyContentDoesNotPublishMigration(t *testing.T) {
	rdb, ctx := setupTestRedis(t)
	c := setupNativeSession(t, rdb, ctx, "inline")
	st, _, err := c.CreateFile(ctx, "/f", 0644, false)
	if err != nil {
		t.Fatal(err)
	}
	key := newKeyBuilder("inline").inode(fmt.Sprint(st.Inode))
	if err := rdb.HSet(ctx, key, "content", "legacy", "size", 6, "content_ref", "").Err(); err != nil {
		t.Fatal(err)
	}
	got, err := c.ReadInodeAt(ctx, st.Inode, 1, 3)
	if err != nil || string(got) != "ega" {
		t.Fatalf("inline read %q %v", got, err)
	}
	ref := rdb.HGet(ctx, key, "content_ref").Val()
	if ref != "" {
		t.Fatalf("read changed live representation to %q", ref)
	}
	if err := c.WriteInodeAt(ctx, st.Inode, []byte("L"), 0); err != nil {
		t.Fatal(err)
	}
	got, err = c.ReadInodeAt(ctx, st.Inode, 0, 6)
	if err != nil || string(got) != "Legacy" {
		t.Fatalf("staged migration %q %v", got, err)
	}
}

func TestNativePostPublicationHintCannotCrossGeneration(t *testing.T) {
	rdb, ctx := setupTestRedis(t)
	seed := setupNativeSession(t, rdb, ctx, "post-hint")
	if err := seed.Echo(ctx, "/f", []byte("before")); err != nil {
		t.Fatal(err)
	}
	wr := redis.NewClient(rdb.Options())
	t.Cleanup(func() { _ = wr.Close() })
	writer, err := NewNative(ctx, wr, "post-hint", "g_1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	keys := newKeyBuilder("post-hint")
	hook := &publicationCommandHook{after: func() {
		if err := rdb.Set(ctx, keys.generation(), "g_2", 0).Err(); err != nil {
			t.Fatal(err)
		}
		if err := rdb.Del(ctx, keys.rootDirty()).Err(); err != nil {
			t.Fatal(err)
		}
	}}
	wr.AddHook(&nativePublicationHook{hook})
	if err := writer.Echo(ctx, "/f", []byte("accepted before replacement")); !errors.Is(err, ErrWorkspaceChanged) {
		t.Fatalf("generation boundary error %v", err)
	}
	if !hook.fired {
		t.Fatal("publication boundary not reached")
	}
	if rdb.Exists(ctx, keys.rootDirty()).Val() != 0 {
		t.Fatal("old session recreated dirty marker after generation replacement")
	}
}

func TestNativeInodePathUsesLinksAfterAncestorRename(t *testing.T) {
	rdb, ctx := setupTestRedis(t)
	c := setupNativeSession(t, rdb, ctx, "inode-path")
	if err := c.Echo(ctx, "/parent/sub/f", []byte("old inode")); err != nil {
		t.Fatal(err)
	}
	st, _ := c.Stat(ctx, "/parent/sub/f")
	if err := c.Rename(ctx, "/parent", "/moved", 0); err != nil {
		t.Fatal(err)
	}
	if err := rdb.HSet(ctx, newKeyBuilder("inode-path").inode(fmt.Sprint(st.Inode)), "path", "/stale-indexed-path").Err(); err != nil {
		t.Fatal(err)
	}
	if err := c.Echo(ctx, "/parent/sub/f", []byte("replacement inode")); err != nil {
		t.Fatal(err)
	}
	path, err := c.InodePath(ctx, st.Inode)
	if err != nil || path != "/moved/sub/f" {
		t.Fatalf("inode path %q %v", path, err)
	}
	if err := c.Rm(ctx, path); err != nil {
		t.Fatal(err)
	}
	if _, err := c.InodePath(ctx, st.Inode); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unlinked handle resolved to replacement: %v", err)
	}
}

// Restrict failure injection to final publication, after private staging. Native
// inode path resolution and staging also use Lua and are not that boundary.
type nativePublicationHook struct{ *publicationCommandHook }

func (h *nativePublicationHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	wrapped := h.publicationCommandHook.ProcessHook(next)
	return func(ctx context.Context, cmd redis.Cmder) error {
		args := cmd.Args()
		if len(args) > 1 && (fmt.Sprint(args[1]) == publishFileScript.Hash() || strings.Contains(fmt.Sprint(args[1]), "Every publication consumes staging")) {
			return wrapped(ctx, cmd)
		}
		return next(ctx, cmd)
	}
}

// Removing a stage between COPY and SETRANGE represents root replacement or
// stage expiry. Losing the connection immediately afterward also defeats the
// best-effort cleanup, exactly the crash window that needs an atomic TTL.
type stageExpiryCutHook struct {
	peer  *redis.Client
	stage string
	fired bool
}

func (h *stageExpiryCutHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h *stageExpiryCutHook) target(cmd redis.Cmder) bool {
	return (cmd.Name() == "setrange" || cmd.Name() == "arset") && len(cmd.Args()) > 1 && strings.Contains(fmt.Sprint(cmd.Args()[1]), ":stage:")
}
func (h *stageExpiryCutHook) cut(ctx context.Context, cmd redis.Cmder) {
	h.fired = true
	h.stage = fmt.Sprint(cmd.Args()[1])
	_ = h.peer.Del(ctx, h.stage).Err()
}
func (h *stageExpiryCutHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if cmd.Name() == "del" && len(cmd.Args()) > 1 && fmt.Sprint(cmd.Args()[1]) == h.stage {
			return io.ErrUnexpectedEOF
		}
		if !h.fired && h.target(cmd) {
			h.cut(ctx, cmd)
			if err := next(ctx, cmd); err != nil {
				return err
			}
			return io.ErrUnexpectedEOF
		}
		return next(ctx, cmd)
	}
}
func (h *stageExpiryCutHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		for _, cmd := range cmds {
			if !h.fired && h.target(cmd) {
				h.cut(ctx, cmd)
				if err := next(ctx, cmds); err != nil {
					return err
				}
				return io.ErrUnexpectedEOF
			}
		}
		return next(ctx, cmds)
	}
}
func TestNativeInterruptedRecreatedStageStillExpires(t *testing.T) {
	rdb, ctx := setupTestRedis(t)
	seed := setupNativeSession(t, rdb, ctx, "stage-expiry")
	if err := seed.Echo(ctx, "/f", []byte("original")); err != nil {
		t.Fatal(err)
	}
	st, _ := seed.Stat(ctx, "/f")
	wr := redis.NewClient(rdb.Options())
	t.Cleanup(func() { _ = wr.Close() })
	writer, err := NewNative(ctx, wr, "stage-expiry", "g_1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	hook := &stageExpiryCutHook{peer: rdb}
	wr.AddHook(hook)
	if err := writer.WriteInodeAt(ctx, st.Inode, []byte("bad"), 0); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("cut not returned: %v", err)
	}
	if !hook.fired {
		t.Fatal("stage write not reached")
	}
	ttl, err := rdb.PTTL(ctx, hook.stage).Result()
	if err != nil || (ttl <= 0 && ttl != -2*time.Nanosecond) {
		t.Fatalf("interrupted recreated stage became immortal: ttl=%v err=%v", ttl, err)
	}
	got, _ := New(rdb, "stage-expiry").Cat(ctx, "/f")
	if string(got) != "original" {
		t.Fatalf("partial range became live: %q", got)
	}
}

// The binary path starts a disposable server; unlike an address supplied by an
// owner, it cannot accidentally point this test at an existing Redis database.
func TestNativeArrayRangeStaging(t *testing.T) {
	binary := os.Getenv("AFS_TEST_ARRAY_REDIS_SERVER")
	if binary == "" {
		t.Skip("set AFS_TEST_ARRAY_REDIS_SERVER to an Array-capable redis-server binary")
	}
	port := freeTCPPort(t)
	cmd := exec.Command(binary, "--port", strconv.Itoa(port), "--save", "", "--appendonly", "no")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	rdb := redis.NewClient(&redis.Options{Addr: fmt.Sprintf("127.0.0.1:%d", port)})
	t.Cleanup(func() { _ = rdb.Close() })
	for rdb.Ping(ctx).Err() != nil {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
	a := setupNativeSession(t, rdb, ctx, "array-range")
	b := setupNativeSession(t, rdb, ctx, "array-range")
	if err := a.Echo(ctx, "/f", bytes.Repeat([]byte("z"), 8192)); err != nil {
		t.Fatal(err)
	}
	st, _ := a.Stat(ctx, "/f")
	ref := rdb.HGet(ctx, newKeyBuilder("array-range").inode(fmt.Sprint(st.Inode)), "content_ref").Val()
	if ref != "array" {
		t.Fatalf("test binary is not Array-capable: ref=%q", ref)
	}
	errs := make(chan error, 2)
	for i, c := range []NativeClient{a, b} {
		go func(i int, c NativeClient) {
			errs <- c.WriteInodeAt(ctx, st.Inode, bytes.Repeat([]byte{byte('A' + i)}, 2000), int64(1000+i*3000))
		}(i, c)
	}
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	expected := bytes.Repeat([]byte("z"), 8192)
	copy(expected[1000:], bytes.Repeat([]byte("A"), 2000))
	copy(expected[4000:], bytes.Repeat([]byte("B"), 2000))
	got, err := a.ReadInodeAt(ctx, st.Inode, 0, 8192)
	if err != nil || !bytes.Equal(got, expected) {
		t.Fatalf("Array ranges differ: %v", err)
	}
	if err := a.TruncateInode(ctx, st.Inode, 4500); err != nil {
		t.Fatal(err)
	}
	if err := a.TruncateInode(ctx, st.Inode, 7000); err != nil {
		t.Fatal(err)
	}
	expected = append(expected[:4500], make([]byte, 2500)...)
	got, err = a.ReadInodeAt(ctx, st.Inode, 0, 7000)
	if err != nil || !bytes.Equal(got, expected) {
		t.Fatalf("Array truncate differs: %v", err)
	}
	wr := redis.NewClient(rdb.Options())
	t.Cleanup(func() { _ = wr.Close() })
	writer, err := NewNative(ctx, wr, "array-range", "g_1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	hook := &stageExpiryCutHook{peer: rdb}
	wr.AddHook(hook)
	if err := writer.WriteInodeAt(ctx, st.Inode, []byte("interrupted"), 0); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("Array cut: %v", err)
	}
	ttl := rdb.PTTL(ctx, hook.stage).Val()
	if !hook.fired || (ttl <= 0 && ttl != -2*time.Nanosecond) {
		t.Fatal("recreated Array stage lacks expiry")
	}
	got, err = a.ReadInodeAt(ctx, st.Inode, 0, 7000)
	if err != nil || !bytes.Equal(got, expected) {
		t.Fatalf("interrupted Array write became live: %v", err)
	}
}
