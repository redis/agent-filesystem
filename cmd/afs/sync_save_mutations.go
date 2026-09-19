package main

import (
	"context"
	"path/filepath"

	"github.com/redis/agent-filesystem/mount/client"
)

// Record each successful mutation immediately, even if a later save operation
// fails. Use the observed remote entry, rather than the older sync baseline, so
// retries account only for changes they actually apply. File history uses the
// exact byte buffer sent to Redis, including for large files.
func recordSyncSaveChange(ctx context.Context, r *reconciler, op uploadOp, previous syncSaveEntry) {
	if r.recordChange == nil {
		return
	}
	op.HasStored = previous.Type != ""
	op.StoredEntry = SyncEntry{
		Type: previous.Type, Mode: previous.Mode, Size: previous.Size,
		RemoteHash: previous.Hash, Target: previous.Target,
	}
	r.recordChange(ctx, uploadResult{Op: op})
}

func removeSyncSaveEntry(ctx context.Context, r *reconciler, rel string, previous syncSaveEntry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := r.fs.Rm(client.WithExpectedStat(ctx, previous.stat), absoluteRemotePath(rel)); err != nil {
		return err
	}
	recordSyncSaveChange(ctx, r, uploadOp{Kind: opUploadDelete, Path: rel}, previous)
	return nil
}

func writeSyncSaveEntry(ctx context.Context, r *reconciler, rel string, entry, previous syncSaveEntry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	remotePath := absoluteRemotePath(rel)
	// Mode-only changes retain file content and its version identity.
	if previous.Type != "" && sameSyncSaveContent(entry, previous) {
		return chmodSyncSaveEntry(ctx, r, rel, entry.Mode, previous)
	}
	op := uploadOp{Path: rel, Mode: entry.Mode, LocalHash: entry.Hash}
	switch entry.Type {
	case "dir":
		if err := r.fs.Mkdir(ctx, remotePath); err != nil {
			return err
		}
		// Mkdir uses 0755. Record creation before a separately fallible chmod.
		op.Kind, op.Mode = opUploadMkdir, 0o755
		recordSyncSaveChange(ctx, r, op, previous)
		return finishSyncSaveCreate(ctx, r, rel, entry.Mode, syncSaveEntry{Type: "dir", Mode: 0o755})
	case "symlink":
		if err := r.fs.Ln(ctx, entry.Target, remotePath); err != nil {
			return err
		}
		op.Kind, op.Mode, op.Symlink = opUploadSymlink, 0o777, entry.Target
		recordSyncSaveChange(ctx, r, op, previous)
		return finishSyncSaveCreate(ctx, r, rel, entry.Mode, syncSaveEntry{Type: "symlink", Mode: 0o777, Target: entry.Target})
	case "file":
		data, err := readSyncSaveFile(ctx, filepath.Join(r.root, filepath.FromSlash(rel)), entry, r.maxFileBytes)
		if err != nil {
			return err
		}
		if err := r.fs.EchoCreate(client.WithExpectedStat(ctx, previous.stat), remotePath, data, entry.Mode); err != nil {
			return err
		}
		op.Kind, op.Content = opUploadFile, data
		recordSyncSaveChange(ctx, r, op, previous)
	}
	return nil
}

func chmodSyncSaveEntry(ctx context.Context, r *reconciler, rel string, mode uint32, previous syncSaveEntry) error {
	if previous.Mode == mode {
		return nil
	}
	if err := r.fs.Chmod(client.WithExpectedStat(ctx, previous.stat), absoluteRemotePath(rel), mode); err != nil {
		return err
	}
	recordSyncSaveChange(ctx, r, uploadOp{Kind: opUploadChmod, Path: rel, Mode: mode, LocalHash: previous.Hash}, previous)
	return nil
}

// Creation and chmod are separate Redis operations. If chmod fails, retain the
// creation's actual mode as the baseline so recovery/retry recognizes our own
// partial write, while still detecting an intervening remote change.
func finishSyncSaveCreate(ctx context.Context, r *reconciler, rel string, mode uint32, created syncSaveEntry) error {
	created.stat, _ = r.fs.Stat(ctx, absoluteRemotePath(rel))
	err := chmodSyncSaveEntry(ctx, r, rel, mode, created)
	if err != nil {
		r.state.mu.Lock()
		r.state.state.Entries[rel] = SyncEntry{Type: created.Type, Mode: created.Mode,
			Target: created.Target, Version: r.state.nextVersion()}
		r.state.dirty = true
		r.state.mu.Unlock()
	}
	return err
}
