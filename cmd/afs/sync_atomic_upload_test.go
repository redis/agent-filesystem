package main

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/redis/agent-filesystem/mount/client"
)

// The peer runs after sync has validated its baseline, but before the native
// client resolves the write. A CAS covering only native staging misses this gap.
type atomicUploadRaceClient struct {
	client.Client
	before func()
	after  func()
}

func (c *atomicUploadRaceClient) Echo(ctx context.Context, p string, data []byte) error {
	if c.before != nil {
		c.before()
	}
	err := c.Client.Echo(ctx, p, data)
	if err == nil && c.after != nil {
		c.after()
	}
	return err
}

func (c *atomicUploadRaceClient) WriteChunks(ctx context.Context, p string, chunks map[int][]byte, size int, length int64, hashes []string) error {
	if c.before != nil {
		c.before()
	}
	err := c.Client.WriteChunks(ctx, p, chunks, size, length, hashes)
	if err == nil && c.after != nil {
		c.after()
	}
	return err
}

func TestSyncAtomicUploadRejectsPeerBetweenBaselineAndWrite(t *testing.T) {
	for _, chunked := range []bool{false, true} {
		for _, initial := range []string{"", "base"} {
			name := "inline"
			if chunked {
				name = "chunked"
			}
			t.Run(name+"/"+initial, func(t *testing.T) {
				env := newSyncTestEnv(t)
				if initial != "" {
					env.writeRemoteFile(t, "file", initial)
				}
				abs := env.writeLocalFile(t, "file", "mine")
				op := uploadOp{Kind: opUploadFile, Path: "file", AbsPath: abs, Content: []byte("mine"), LocalHash: sha256Hex([]byte("mine")), Mode: 0o640}
				if initial != "" {
					op.HasStored = true
					op.StoredEntry = SyncEntry{RemoteHash: sha256Hex([]byte(initial))}
				}
				if chunked {
					op.Chunked, op.ChunkSize, op.FileSize = true, 2, 4
					op.ChunkHashes = uploadChunkHashes([]byte("mine"), 2)
					op.LocalHash = compositeHash(op.ChunkHashes)
					op.DirtyChunks = []int{0, 1}
				}
				var peerStat *client.StatResult
				fs := &atomicUploadRaceClient{Client: env.fsClient, before: func() {
					env.writeRemoteFile(t, "file", "peer")
					if err := env.fsClient.Chmod(context.Background(), "/file", 0o600); err != nil {
						t.Fatal(err)
					}
					peerStat, _ = env.fsClient.Stat(context.Background(), "/file")
				}}
				result := runRecoveryUpload(fs, op)
				if !result.Conflict || result.Err != nil {
					t.Fatalf("want conflict, got %+v", result)
				}
				if got := env.readRemoteFile(t, "file"); got != "peer" {
					t.Fatalf("lost peer content: %q", got)
				}
				current, _ := env.fsClient.Stat(context.Background(), "/file")
				if !client.SameFileRevision(current, peerStat) {
					t.Fatalf("conflict changed peer metadata: %+v -> %+v", peerStat, current)
				}
				if got, _ := os.ReadFile(abs); string(got) != "mine" {
					t.Fatalf("lost local candidate: %q", got)
				}
			})
		}
	}
}

func TestSyncAtomicUploadReturnsOwnRevisionAfterPeerWrite(t *testing.T) {
	env := newSyncTestEnv(t)
	env.writeRemoteFile(t, "file", "base")
	abs := env.writeLocalFile(t, "file", "mine")
	op := uploadOp{Kind: opUploadFile, Path: "file", AbsPath: abs, Content: []byte("mine"), LocalHash: sha256Hex([]byte("mine")), Mode: 0o640, HasStored: true, StoredEntry: SyncEntry{RemoteHash: sha256Hex([]byte("base"))}}
	var committed *client.StatResult
	fs := &atomicUploadRaceClient{Client: env.fsClient, after: func() {
		committed, _ = env.fsClient.Stat(context.Background(), "/file")
		env.writeRemoteFile(t, "file", "newer")
	}}
	result := runRecoveryUpload(fs, op)
	if result.Err != nil || result.Conflict || !client.SameFileRevision(result.RemoteStat, committed) {
		t.Fatalf("wrong commit acknowledgement: %+v; own %+v", result, committed)
	}
	if got := env.readRemoteFile(t, "file"); got != "newer" {
		t.Fatalf("lost newer content: %q", got)
	}
}

func TestSyncAtomicUploadFencesRenamedParent(t *testing.T) {
	env := newSyncTestEnv(t)
	env.writeRemoteFile(t, "dir/file", "base")
	abs := env.writeLocalFile(t, "dir/file", "mine")
	fs := &atomicUploadRaceClient{Client: env.fsClient, before: func() {
		if err := env.fsClient.Rename(context.Background(), "/dir", "/moved", 0); err != nil {
			t.Fatal(err)
		}
		env.writeRemoteFile(t, "dir/file", "peer")
	}}
	op := uploadOp{Kind: opUploadFile, Path: "dir/file", AbsPath: abs, Content: []byte("mine"), LocalHash: sha256Hex([]byte("mine")), Mode: 0o644, HasStored: true, StoredEntry: SyncEntry{RemoteHash: sha256Hex([]byte("base"))}}
	result := runRecoveryUpload(fs, op)
	if !result.Conflict {
		t.Fatalf("want conflict: %+v", result)
	}
	if got := env.readRemoteFile(t, "moved/file"); got != "base" {
		t.Fatalf("modified renamed tree: %q", got)
	}
	if got := env.readRemoteFile(t, "dir/file"); got != "peer" {
		t.Fatalf("modified replacement tree: %q", got)
	}
}

func TestSyncSaveConditionalWriteRejectsPeer(t *testing.T) {
	env := newSyncTestEnv(t)
	env.writeRemoteFile(t, "file", "base")
	env.writeLocalFile(t, "file", "mine")
	r := newSyncSaveTestReconciler(t, env)
	before, err := scanSyncSaveRemote(context.Background(), r, r.state.snapshot())
	if err != nil {
		t.Fatal(err)
	}
	local, err := scanSyncSaveLocal(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	env.writeRemoteFile(t, "file", "peer")
	err = writeSyncSaveEntry(context.Background(), r, "file", local["file"], before["file"])
	if !errors.Is(err, client.ErrWriteConflict) {
		t.Fatalf("want conflict, got %v", err)
	}
	if got := env.readRemoteFile(t, "file"); got != "peer" {
		t.Fatalf("lost peer: %q", got)
	}
}

func TestSyncRecoveryDetectsRevisionChangeWithIdenticalMetadata(t *testing.T) {
	stored := SyncEntry{Type: "file", Mode: 0644, Size: 4, LocalMtimeMs: 100, RemoteMtimeMs: 100, RemoteRevision: "before", RemoteInode: 2}
	local := observedMeta{kind: "file", mode: 0644, size: 4, mtimeMs: 100}
	remote := observedMeta{kind: "file", mode: 0644, size: 4, mtimeMs: 100, revision: "after", inode: 2}
	if metaMatch(local, remote, stored, true) {
		t.Fatal("same millisecond timestamp hid a new revision")
	}
	if !observedChangedFromStored(remote, stored, false) {
		t.Fatal("recovery did not detect a changed revision")
	}
	if observedChangedFromStored(local, stored, true) {
		t.Fatal("unchanged local content marked dirty")
	}
}
