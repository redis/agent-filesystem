package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type attachReconcilePlan struct {
	LocalCount        int
	RemoteCount       int
	StateCount        int
	SameCount         int
	ImportCount       int
	UploadCount       int
	DownloadCount     int
	DeleteLocalCount  int
	DeleteRemoteCount int
	ConflictCount     int
	SkippedCount      int
	Operations        []attachReconcileOperation
	Baseline          map[string]SyncEntry
}

type attachReconcileOperation struct {
	Code    string
	Path    string
	Details string
}

func buildAttachReconcilePlan(ctx context.Context, d *syncDaemon) (attachReconcilePlan, error) {
	if d == nil || d.full == nil {
		return attachReconcilePlan{}, errors.New("attach reconcile: nil daemon")
	}
	local, err := d.full.scanLocalMeta()
	if err != nil {
		return attachReconcilePlan{}, fmt.Errorf("scan local attach path: %w", err)
	}
	remote, err := d.full.scanRemoteMeta(ctx, nil)
	if err != nil {
		return attachReconcilePlan{}, fmt.Errorf("scan remote workspace: %w", err)
	}

	plan := attachReconcilePlan{
		LocalCount:  len(local),
		RemoteCount: len(remote),
		Baseline:    make(map[string]SyncEntry),
	}
	if d.stateWriter != nil {
		if snap := d.stateWriter.snapshot(); snap != nil {
			plan.StateCount = len(snap.Entries)
			if len(snap.Entries) > 0 {
				return buildKnownAttachReconcilePlan(ctx, d, local, remote, snap, plan)
			}
		}
	}
	paths := sortedAttachReconcilePaths(local, remote)
	now := time.Now().UTC()
	for _, path := range paths {
		l, lok := local[path]
		r, rok := remote[path]
		switch {
		case lok && !rok:
			plan.addLocalOnly(path, l, d.cfg.MaxFileBytes)
		case !lok && rok:
			plan.DownloadCount++
			plan.Operations = append(plan.Operations, attachReconcileOperation{
				Code:    "D",
				Path:    path,
				Details: "remote only; download to local folder",
			})
		case lok && rok:
			entry, same, detail, err := attachBaselineEntry(ctx, d, path, l, r, now)
			if err != nil {
				return attachReconcilePlan{}, err
			}
			if same {
				plan.SameCount++
				plan.Baseline[path] = entry
				continue
			}
			plan.ConflictCount++
			plan.Operations = append(plan.Operations, attachReconcileOperation{
				Code:    "C",
				Path:    path,
				Details: detail,
			})
		}
	}
	return plan, nil
}

func (p attachReconcilePlan) hasReportableChanges() bool {
	return p.ImportCount > 0 || p.UploadCount > 0 || p.DownloadCount > 0 || p.DeleteLocalCount > 0 || p.DeleteRemoteCount > 0 || p.ConflictCount > 0 || p.SkippedCount > 0
}

func (p attachReconcilePlan) requiresConfirmation() bool {
	if p.StateCount > 0 {
		return p.hasReportableChanges()
	}
	if p.LocalCount == 0 {
		return false
	}
	return p.ImportCount > 0 || p.UploadCount > 0 || p.DownloadCount > 0 || p.SkippedCount > 0
}

func (p *attachReconcilePlan) addLocalOnly(path string, meta observedMeta, maxFileBytes int64) {
	if meta.kind == "file" && maxFileBytes > 0 && meta.size > maxFileBytes {
		p.SkippedCount++
		p.Operations = append(p.Operations, attachReconcileOperation{
			Code:    "S",
			Path:    path,
			Details: fmt.Sprintf("local file is %s; exceeds sync cap %s", formatBytes(meta.size), formatBytes(maxFileBytes)),
		})
		return
	}
	p.ImportCount++
	p.Operations = append(p.Operations, attachReconcileOperation{
		Code:    "I",
		Path:    path,
		Details: "local only; upload to workspace",
	})
}

func buildKnownAttachReconcilePlan(ctx context.Context, d *syncDaemon, local, remote map[string]observedMeta, st *SyncState, plan attachReconcilePlan) (attachReconcilePlan, error) {
	paths := sortedKnownAttachReconcilePaths(local, remote, st.Entries)
	now := time.Now().UTC()
	for _, path := range paths {
		stored, hasStored := st.Entries[path]
		if hasStored && stored.Deleted {
			hasStored = false
		}
		l, lok := local[path]
		r, rok := remote[path]
		switch {
		case lok && !rok:
			if hasStored {
				plan.DeleteLocalCount++
				plan.Operations = append(plan.Operations, attachReconcileOperation{
					Code:    "DL",
					Path:    path,
					Details: "remote deleted while detached; remove local copy",
				})
				continue
			}
			plan.addKnownUpload(path, l, d.cfg.MaxFileBytes)
		case !lok && rok:
			if hasStored {
				if observedChangedFromStored(r, stored, false) {
					plan.addConflict(path, "local deleted while remote changed")
					continue
				}
				plan.DeleteRemoteCount++
				plan.Operations = append(plan.Operations, attachReconcileOperation{
					Code:    "DR",
					Path:    path,
					Details: "local deleted while detached; remove from workspace",
				})
				continue
			}
			plan.DownloadCount++
			plan.Operations = append(plan.Operations, attachReconcileOperation{
				Code:    "D",
				Path:    path,
				Details: "remote created while detached; download to local folder",
			})
		case lok && rok:
			if !hasStored {
				entry, same, detail, err := attachBaselineEntry(ctx, d, path, l, r, now)
				if err != nil {
					return attachReconcilePlan{}, err
				}
				if same {
					plan.SameCount++
					plan.Baseline[path] = entry
					continue
				}
				plan.addConflict(path, detail)
				continue
			}
			localChanged := observedChangedFromStored(l, stored, true)
			remoteChanged := observedChangedFromStored(r, stored, false)
			switch {
			case !localChanged && !remoteChanged:
				plan.SameCount++
			case localChanged && !remoteChanged:
				plan.addKnownUpload(path, l, d.cfg.MaxFileBytes)
			case !localChanged && remoteChanged:
				plan.DownloadCount++
				plan.Operations = append(plan.Operations, attachReconcileOperation{
					Code:    "D",
					Path:    path,
					Details: "remote changed while detached; download to local folder",
				})
			default:
				entry, same, _, err := attachBaselineEntry(ctx, d, path, l, r, now)
				if err != nil {
					return attachReconcilePlan{}, err
				}
				if same {
					plan.SameCount++
					plan.Baseline[path] = entry
					continue
				}
				plan.addConflict(path, "local and remote both changed while detached")
			}
		}
	}
	return plan, nil
}

func (p *attachReconcilePlan) addKnownUpload(path string, meta observedMeta, maxFileBytes int64) {
	if meta.kind == "file" && maxFileBytes > 0 && meta.size > maxFileBytes {
		p.SkippedCount++
		p.Operations = append(p.Operations, attachReconcileOperation{
			Code:    "S",
			Path:    path,
			Details: fmt.Sprintf("local file is %s; exceeds sync cap %s", formatBytes(meta.size), formatBytes(maxFileBytes)),
		})
		return
	}
	p.UploadCount++
	p.Operations = append(p.Operations, attachReconcileOperation{
		Code:    "U",
		Path:    path,
		Details: "local changed while detached; upload to workspace",
	})
}

func (p *attachReconcilePlan) addConflict(path, details string) {
	p.ConflictCount++
	p.Operations = append(p.Operations, attachReconcileOperation{
		Code:    "C",
		Path:    path,
		Details: details,
	})
}

func observedChangedFromStored(meta observedMeta, stored SyncEntry, localSide bool) bool {
	if stored.Deleted || meta.kind != stored.Type {
		return true
	}
	switch meta.kind {
	case "dir":
		return stored.Mode != 0 && meta.mode != 0 && meta.mode != stored.Mode
	case "symlink":
		return meta.target != stored.Target
	case "file":
		if meta.size != stored.Size {
			return true
		}
		if localSide {
			return stored.LocalMtimeMs != 0 && meta.mtimeMs != stored.LocalMtimeMs
		}
		return stored.RemoteMtimeMs != 0 && meta.mtimeMs != stored.RemoteMtimeMs
	default:
		return true
	}
}

func attachBaselineEntry(ctx context.Context, d *syncDaemon, path string, local, remote observedMeta, now time.Time) (SyncEntry, bool, string, error) {
	if local.kind != remote.kind {
		return SyncEntry{}, false, fmt.Sprintf("type differs: local %s, remote %s", local.kind, remote.kind), nil
	}
	switch local.kind {
	case "dir":
		return SyncEntry{
			Type:          "dir",
			Mode:          remote.mode,
			LocalMtimeMs:  local.mtimeMs,
			RemoteMtimeMs: remote.mtimeMs,
			LastSyncedAt:  now,
		}, true, "", nil
	case "symlink":
		if local.target != remote.target {
			return SyncEntry{}, false, "symlink target differs", nil
		}
		return SyncEntry{
			Type:         "symlink",
			Target:       local.target,
			LastSyncedAt: now,
		}, true, "", nil
	case "file":
		if local.size != remote.size {
			return SyncEntry{}, false, fmt.Sprintf("file size differs: local %s, remote %s", formatBytes(local.size), formatBytes(remote.size)), nil
		}
		localPath := filepath.Join(d.cfg.LocalRoot, filepath.FromSlash(path))
		localData, err := os.ReadFile(localPath)
		if err != nil {
			return SyncEntry{}, false, "", fmt.Errorf("read local %s: %w", path, err)
		}
		remoteData, err := d.cfg.FS.Cat(ctx, absoluteRemotePath(path))
		if err != nil {
			return SyncEntry{}, false, "", fmt.Errorf("read remote %s: %w", path, err)
		}
		localHash := sha256Hex(localData)
		remoteHash := sha256Hex(remoteData)
		if localHash != remoteHash {
			return SyncEntry{}, false, "file content differs", nil
		}
		mode := remote.mode
		if mode == 0 {
			mode = local.mode
		}
		return SyncEntry{
			Type:          "file",
			Mode:          mode,
			Size:          local.size,
			LocalHash:     localHash,
			RemoteHash:    remoteHash,
			LocalMtimeMs:  local.mtimeMs,
			RemoteMtimeMs: remote.mtimeMs,
			LastSyncedAt:  now,
		}, true, "", nil
	default:
		return SyncEntry{}, false, fmt.Sprintf("unsupported type %s", local.kind), nil
	}
}

func approveAttachReconcilePlan(d *syncDaemon, plan attachReconcilePlan) {
	if d == nil || d.stateWriter == nil {
		return
	}
	d.cfg.ApprovedInitialAttachMerge = true
	if len(plan.Baseline) == 0 {
		return
	}
	d.stateWriter.mu.Lock()
	for path, entry := range plan.Baseline {
		entry.Version = d.stateWriter.nextVersion()
		d.stateWriter.state.Entries[path] = entry
	}
	d.stateWriter.dirty = true
	d.stateWriter.mu.Unlock()
	select {
	case d.stateWriter.flushCh <- struct{}{}:
	default:
	}
}

func printAttachReconcilePlan(title, workspace, localRoot string, plan attachReconcilePlan, showOperations bool) {
	rows := []outputRow{
		{Label: "workspace", Value: workspace},
		{Label: "path", Value: compactDisplayPath(localRoot)},
		{Label: "local entries", Value: fmt.Sprintf("%d", plan.LocalCount)},
		{Label: "remote entries", Value: fmt.Sprintf("%d", plan.RemoteCount)},
		{Label: "sync state", Value: fmt.Sprintf("%d", plan.StateCount)},
		{Label: "same", Value: fmt.Sprintf("%d", plan.SameCount)},
		{Label: "import", Value: fmt.Sprintf("%d", plan.ImportCount)},
		{Label: "upload", Value: fmt.Sprintf("%d", plan.UploadCount)},
		{Label: "download", Value: fmt.Sprintf("%d", plan.DownloadCount)},
		{Label: "delete local", Value: fmt.Sprintf("%d", plan.DeleteLocalCount)},
		{Label: "delete remote", Value: fmt.Sprintf("%d", plan.DeleteRemoteCount)},
		{Label: "conflict", Value: fmt.Sprintf("%d", plan.ConflictCount)},
		{Label: "skipped", Value: fmt.Sprintf("%d", plan.SkippedCount)},
	}
	printSection(title, rows)
	if !showOperations || len(plan.Operations) == 0 {
		return
	}
	tableRows := make([][]string, 0, len(plan.Operations))
	for _, op := range plan.Operations {
		tableRows = append(tableRows, []string{op.Code, op.Path, op.Details})
	}
	printPlainTable([]string{"OP", "PATH", "DETAILS"}, tableRows)
	fmt.Println()
}

func sortedAttachReconcilePaths(local, remote map[string]observedMeta) []string {
	seen := make(map[string]struct{}, len(local)+len(remote))
	for path := range local {
		seen[path] = struct{}{}
	}
	for path := range remote {
		seen[path] = struct{}{}
	}
	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func sortedKnownAttachReconcilePaths(local, remote map[string]observedMeta, entries map[string]SyncEntry) []string {
	seen := make(map[string]struct{}, len(local)+len(remote)+len(entries))
	for path := range local {
		seen[path] = struct{}{}
	}
	for path := range remote {
		seen[path] = struct{}{}
	}
	for path := range entries {
		seen[path] = struct{}{}
	}
	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}
