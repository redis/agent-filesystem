package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/redis/agent-filesystem/internal/controlplane"
	"github.com/redis/agent-filesystem/mount/client"
	"github.com/redis/go-redis/v9"
)

func TestUploaderRecoveryInlineContent(t *testing.T) {
	for _, tc := range []struct {
		name, remote    string
		chunkedBaseline bool
		conflict        bool
		writes          int
	}{
		{name: "already uploaded", remote: "queued content"},
		{name: "baseline unchanged", remote: "baseline", writes: 1},
		{name: "remote diverged", remote: "another writer", conflict: true},
		{name: "chunked baseline", remote: "baseline", chunkedBaseline: true, writes: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newSyncTestEnv(t)
			op := recoveryUploadOp(t, env, []byte("queued content"), false)
			op.StoredEntry = SyncEntry{Mode: 0o644, RemoteHash: sha256Hex([]byte("baseline"))}
			if tc.chunkedBaseline {
				op.StoredEntry.ChunkSize = 4
				op.StoredEntry.RemoteHash = compositeHash(uploadChunkHashes([]byte("baseline"), 4))
			}
			env.writeRemoteFile(t, op.Path, tc.remote)
			fs := &recoveryUploadClient{Client: env.fsClient}
			res := runRecoveryUpload(fs, op)
			if res.Err != nil || res.Skipped || res.Conflict != tc.conflict {
				t.Fatalf("upload result = %+v", res)
			}
			if fs.echoes != tc.writes {
				t.Fatalf("content writes = %d, want %d", fs.echoes, tc.writes)
			}
			assertRecoveryUploadOutcome(t, env, fs, op, res, tc.remote)
		})
	}
}

func TestUploaderRecoveryChunkedContent(t *testing.T) {
	baseline := bytes.Repeat([]byte("b"), defaultChunkThreshold+17)
	intended := bytes.Repeat([]byte("q"), len(baseline))
	diverged := bytes.Repeat([]byte("x"), len(baseline))
	for _, tc := range []struct {
		name             string
		remote           []byte
		baselineChunked  bool
		staleChunkHashes bool
		conflict         bool
		writes           int
	}{
		{name: "already uploaded by Echo", remote: intended},
		{name: "already uploaded in chunks", remote: intended, staleChunkHashes: true},
		{name: "plain baseline", remote: baseline, writes: 1},
		{name: "chunked baseline after Echo", remote: baseline, baselineChunked: true, writes: 1},
		{name: "divergent bytes with matching stale chunks", remote: diverged, staleChunkHashes: true, conflict: true},
		{name: "divergent bytes without chunk metadata", remote: diverged, conflict: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newSyncTestEnv(t)
			op := recoveryUploadOp(t, env, intended, true)
			op.StoredEntry = SyncEntry{Mode: 0o644, RemoteHash: sha256Hex(baseline)}
			if tc.baselineChunked {
				op.StoredEntry.ChunkSize = op.ChunkSize
				op.StoredEntry.RemoteHash = compositeHash(uploadChunkHashes(baseline, op.ChunkSize))
			}
			env.writeRemoteFile(t, op.Path, string(tc.remote))
			if tc.staleChunkHashes {
				chunks := make(map[int][]byte, len(op.ChunkHashes))
				for i := range op.ChunkHashes {
					start := i * op.ChunkSize
					chunks[i] = intended[start:min(start+op.ChunkSize, len(intended))]
				}
				if err := env.fsClient.WriteChunks(context.Background(), "/"+op.Path, chunks, op.ChunkSize, op.FileSize, op.ChunkHashes); err != nil {
					t.Fatal(err)
				}
				// Full reconciliation writes through Echo, which can leave the
				// old chunk manifest even when the current bytes differ.
				if err := env.fsClient.Echo(context.Background(), "/"+op.Path, tc.remote); err != nil {
					t.Fatal(err)
				}
			}
			fs := &recoveryUploadClient{Client: env.fsClient}
			res := runRecoveryUpload(fs, op)
			if res.Err != nil || res.Skipped || res.Conflict != tc.conflict {
				t.Fatalf("upload result: error=%v skipped=%v conflict=%v, want conflict=%v", res.Err, res.Skipped, res.Conflict, tc.conflict)
			}
			if fs.chunkWrites != tc.writes {
				t.Fatalf("chunk writes = %d, want %d", fs.chunkWrites, tc.writes)
			}
			assertRecoveryUploadOutcome(t, env, fs, op, res, string(tc.remote))
		})
	}
}

func TestUploaderRecoverySkipsChangedLocalFile(t *testing.T) {
	for _, chunked := range []bool{false, true} {
		for _, change := range []string{"replacement", "deleted", "mode", "during remote read"} {
			t.Run(fmt.Sprintf("chunked=%v/%s", chunked, change), func(t *testing.T) {
				env := newSyncTestEnv(t)
				op := recoveryUploadOp(t, env, []byte("queued content"), chunked)
				env.writeRemoteFile(t, op.Path, "recovery uploaded newer content")
				fs := &recoveryUploadClient{Client: env.fsClient}
				switch change {
				case "replacement":
					env.writeLocalFile(t, op.Path, "new local content")
				case "deleted":
					if err := os.Remove(op.AbsPath); err != nil {
						t.Fatal(err)
					}
				case "mode":
					if err := os.Chmod(op.AbsPath, 0o600); err != nil {
						t.Fatal(err)
					}
				case "during remote read":
					fs.afterCat = func() { env.writeLocalFile(t, op.Path, "new local content") }
				}
				res := runRecoveryUpload(fs, op)
				if res.Err != nil || res.Conflict || !res.Skipped {
					t.Fatalf("obsolete upload result = %+v", res)
				}
				if fs.echoes != 0 || fs.chunkWrites != 0 || fs.chmods != 0 {
					t.Fatalf("obsolete upload mutated remote: echo=%d chunks=%d chmod=%d", fs.echoes, fs.chunkWrites, fs.chmods)
				}
				if got := env.readRemoteFile(t, op.Path); got != "recovery uploaded newer content" {
					t.Fatalf("remote changed to %q", got)
				}
			})
		}
	}
}

func TestUploaderRecoveryRejectsChangedChunkBytes(t *testing.T) {
	env := newSyncTestEnv(t)
	op := recoveryUploadOp(t, env, []byte("queued content"), true)
	env.writeRemoteFile(t, op.Path, "baseline")
	op.StoredEntry.RemoteHash = sha256Hex([]byte("baseline"))
	fs := &recoveryUploadClient{Client: env.fsClient, afterCat: func() {
		// Keep metadata unchanged to exercise byte validation at chunk read.
		if err := os.WriteFile(op.AbsPath, []byte("edited content"), os.FileMode(op.Mode)); err != nil {
			t.Fatal(err)
		}
		stamp := time.UnixMilli(op.LocalMtimeMs)
		if err := os.Chtimes(op.AbsPath, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}}
	res := runRecoveryUpload(fs, op)
	if res.Err != nil || res.Conflict || !res.Skipped || fs.chunkWrites != 0 || fs.chmods != 0 {
		t.Fatalf("changed chunk result: error=%v conflict=%v skipped=%v writes=%d chmod=%d", res.Err, res.Conflict, res.Skipped, fs.chunkWrites, fs.chmods)
	}
}

func TestUploaderRecoveryMatchingContentPreservesRemoteMode(t *testing.T) {
	for _, chunked := range []bool{false, true} {
		for _, tc := range []struct {
			name              string
			baseline, remote  uint32
			skipped, conflict bool
		}{
			{name: "local mode changed", baseline: 0o644, remote: 0o644},
			{name: "both chose same mode", baseline: 0o644, remote: 0o640},
			{name: "remote mode changed", baseline: 0o640, remote: 0o600, skipped: true},
			{name: "both modes diverged", baseline: 0o644, remote: 0o600, conflict: true},
		} {
			t.Run(fmt.Sprintf("chunked=%v/%s", chunked, tc.name), func(t *testing.T) {
				env := newSyncTestEnv(t)
				op := recoveryUploadOp(t, env, []byte("queued content"), chunked)
				op.StoredEntry.Mode = tc.baseline
				env.writeRemoteFile(t, op.Path, "queued content")
				if err := env.fsClient.Chmod(context.Background(), "/"+op.Path, tc.remote); err != nil {
					t.Fatal(err)
				}
				fs := &recoveryUploadClient{Client: env.fsClient}
				res := runRecoveryUpload(fs, op)
				if res.Err != nil || res.Skipped != tc.skipped || res.Conflict != tc.conflict {
					t.Fatalf("mode result: error=%v skipped=%v conflict=%v", res.Err, res.Skipped, res.Conflict)
				}
				want := op.Mode
				if tc.skipped || tc.conflict {
					want = tc.remote
					if fs.chmods != 0 {
						t.Fatal("overwrote the remote mode change")
					}
				}
				stat, err := env.fsClient.Stat(context.Background(), "/"+op.Path)
				if err != nil || stat == nil || stat.Mode != want {
					t.Fatalf("remote stat=%+v, error=%v, want mode=%o", stat, err, want)
				}
				if fs.echoes != 0 || fs.chunkWrites != 0 {
					t.Fatal("matching content was rewritten")
				}
			})
		}
	}
}

func TestUploaderRecoverySkipsSameMetadataReplacement(t *testing.T) {
	for _, replacement := range []bool{false, true} {
		t.Run(fmt.Sprintf("new inode=%v", replacement), func(t *testing.T) {
			env := newSyncTestEnv(t)
			op := recoveryUploadOp(t, env, []byte("queued content"), false)
			env.writeRemoteFile(t, op.Path, "recovery uploaded newer content")
			content := []byte("edited content")
			if replacement {
				if err := os.Rename(op.AbsPath, op.AbsPath+".previous"); err != nil {
					t.Fatal(err)
				}
				content = op.Content // identical bytes still belong to a replacement inode
			}
			if err := os.WriteFile(op.AbsPath, content, os.FileMode(op.Mode)); err != nil {
				t.Fatal(err)
			}
			stamp := time.UnixMilli(op.LocalMtimeMs)
			if err := os.Chtimes(op.AbsPath, stamp, stamp); err != nil {
				t.Fatal(err)
			}
			fs := &recoveryUploadClient{Client: env.fsClient}
			res := runRecoveryUpload(fs, op)
			if res.Err != nil || res.Conflict || !res.Skipped || fs.echoes != 0 || fs.chmods != 0 {
				t.Fatalf("same metadata obsolete result: error=%v conflict=%v skipped=%v writes=%d chmod=%d", res.Err, res.Conflict, res.Skipped, fs.echoes, fs.chmods)
			}
		})
	}
}

func TestUploaderRecoveryDeleteRecreatedLocal(t *testing.T) {
	env := newSyncTestEnv(t)
	abs := env.writeLocalFile(t, "recreated", "current local content")
	env.writeRemoteFile(t, "recreated", "current remote content")
	fs := &recoveryUploadClient{Client: env.fsClient}
	results := make(chan uploadResult, 1)
	u := newUploader(fs, results, 0, false, nil)
	op := uploadOp{Kind: opUploadDelete, Path: "recreated", AbsPath: abs}
	u.processDelete(context.Background(), op)
	res := <-results
	if res.Err != nil || res.Conflict || !res.Skipped {
		t.Fatalf("recreated delete result = %+v", res)
	}
	if got := env.readRemoteFile(t, op.Path); got != "current remote content" {
		t.Fatalf("remote recreated file = %q", got)
	}
	if err := os.Remove(abs); err != nil {
		t.Fatal(err)
	}
	u.processDelete(context.Background(), op)
	res = <-results
	if res.Err != nil || res.Skipped || res.Conflict {
		t.Fatalf("current delete result = %+v", res)
	}
	stat, err := env.fsClient.Stat(context.Background(), "/"+op.Path)
	if err != nil || stat != nil {
		t.Fatalf("deleted remote stat=%+v, error=%v", stat, err)
	}
}

func TestUploaderRecoveryDeleteAbsent(t *testing.T) {
	env := newSyncTestEnv(t)
	u := newUploader(env.fsClient, make(chan uploadResult, 1), 0, false, nil)
	results := make(chan uploadResult, 1)
	u.results = results
	for i := 0; i < 2; i++ {
		u.processDelete(context.Background(), uploadOp{Kind: opUploadDelete, Path: "absent"})
		if res := <-results; res.Err != nil {
			t.Fatalf("repeated absent delete %d: %v", i, res.Err)
		}
	}
	for _, tc := range []struct {
		name string
		err  error
		fail bool
	}{
		{name: "wrapped absence", err: fmt.Errorf("remove inode: %w", redis.Nil)},
		{name: "path absence", err: errors.New("no such file")},
		{name: "backend error", err: errors.New("backend unavailable"), fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u.fs = &recoveryUploadClient{Client: env.fsClient, rmErr: tc.err}
			u.processDelete(context.Background(), uploadOp{Kind: opUploadDelete, Path: "absent"})
			res := <-results
			if (res.Err != nil) != tc.fail || tc.fail && !errors.Is(res.Err, tc.err) {
				t.Fatalf("delete error = %v, want failure=%v preserving %v", res.Err, tc.fail, tc.err)
			}
		})
	}
}

func TestUploaderRecoverySkippedDoesNotEmitChange(t *testing.T) {
	env := newSyncTestEnv(t)
	u := newUploader(env.fsClient, nil, 0, false, nil)
	u.mountChangelog(env.rdb, "recovery-workspace", "recovery-session", "", "", "", "")
	u.send(uploadResult{Op: uploadOp{Kind: opUploadFile, Path: "skipped"}, Skipped: true})
	count, err := env.rdb.XLen(context.Background(), controlplane.ChangelogStreamKey("recovery-workspace")).Result()
	if err != nil || count != 0 {
		t.Fatalf("skipped upload changelog count=%d, error=%v", count, err)
	}
}

func recoveryUploadOp(t *testing.T, env *syncTestEnv, content []byte, chunked bool) uploadOp {
	t.Helper()
	abs := env.writeLocalFile(t, "queued", string(content))
	if err := os.Chmod(abs, 0o640); err != nil {
		t.Fatal(err)
	}
	stamp := time.Unix(10, 0)
	if err := os.Chtimes(abs, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	op := uploadOp{Kind: opUploadFile, Path: "queued", AbsPath: abs, Content: content, Mode: 0o640, LocalIdentity: localFileIdentityFromPath(abs), LocalMtimeMs: stamp.UnixMilli(), LocalHash: sha256Hex(content), HasStored: true}
	if chunked {
		op.Chunked = true
		op.Content = nil
		op.ChunkSize = defaultChunkSize
		op.FileSize = int64(len(content))
		op.ChunkHashes = uploadChunkHashes(content, op.ChunkSize)
		op.LocalHash = compositeHash(op.ChunkHashes)
		for i := range op.ChunkHashes {
			op.DirtyChunks = append(op.DirtyChunks, i)
		}
	}
	return op
}

func runRecoveryUpload(fs client.Client, op uploadOp) uploadResult {
	results := make(chan uploadResult, 1)
	u := newUploader(fs, results, 0, false, nil)
	u.processFile(context.Background(), op)
	return <-results
}

func assertRecoveryUploadOutcome(t *testing.T, env *syncTestEnv, fs *recoveryUploadClient, op uploadOp, res uploadResult, original string) {
	t.Helper()
	want := original
	if !res.Conflict {
		data, err := os.ReadFile(op.AbsPath)
		if err != nil {
			t.Fatal(err)
		}
		want = string(data)
		if res.RemoteStat == nil || res.RemoteStat.Mode != op.Mode || res.RemoteHashSeen != op.LocalHash || fs.chmods > 1 {
			t.Fatalf("successful upload did not preserve mode/hash result: stat=%+v hash=%s chmod=%d", res.RemoteStat, res.RemoteHashSeen, fs.chmods)
		}
	} else if fs.chmods != 0 {
		t.Fatal("conflicting upload changed remote mode")
	}
	if got := env.readRemoteFile(t, op.Path); got != want {
		t.Fatalf("remote content differs: got %d bytes, want %d", len(got), len(want))
	}
}

type recoveryUploadClient struct {
	client.Client
	echoes, chunkWrites, chmods int
	rmErr                       error
	afterCat                    func()
}

func (c *recoveryUploadClient) Echo(ctx context.Context, path string, data []byte) error {
	c.echoes++
	return c.Client.Echo(ctx, path, data)
}

func (c *recoveryUploadClient) WriteChunks(ctx context.Context, path string, chunks map[int][]byte, size int, total int64, hashes []string) error {
	c.chunkWrites++
	return c.Client.WriteChunks(ctx, path, chunks, size, total, hashes)
}

func (c *recoveryUploadClient) Chmod(ctx context.Context, path string, mode uint32) error {
	c.chmods++
	return c.Client.Chmod(ctx, path, mode)
}

func (c *recoveryUploadClient) Rm(ctx context.Context, path string) error {
	if c.rmErr != nil {
		return c.rmErr
	}
	return c.Client.Rm(ctx, path)
}

func (c *recoveryUploadClient) Cat(ctx context.Context, path string) ([]byte, error) {
	data, err := c.Client.Cat(ctx, path)
	if c.afterCat != nil {
		c.afterCat()
	}
	return data, err
}
