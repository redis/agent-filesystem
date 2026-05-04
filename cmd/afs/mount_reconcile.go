package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type mountReconcilePlan struct {
	LocalCount        int
	RemoteCount       int
	StateCount        int
	StateLiveCount    int
	StateDeletedCount int
	SameCount         int
	ImportCount       int
	UploadCount       int
	DownloadCount     int
	DeleteLocalCount  int
	DeleteRemoteCount int
	ConflictCount     int
	SkippedCount      int
	Operations        []mountReconcileOperation
	Baseline          map[string]SyncEntry
}

type mountReconcileOperation struct {
	Code    string
	Path    string
	Kind    string
	Details string
}

type mountLocalSnapshot struct {
	Exists     bool
	EntryCount int
}

func inspectMountLocalRoot(root string) (mountLocalSnapshot, error) {
	info, err := os.Stat(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return mountLocalSnapshot{}, nil
		}
		return mountLocalSnapshot{}, err
	}
	if !info.IsDir() {
		return mountLocalSnapshot{}, fmt.Errorf("%s is not a directory", root)
	}
	count, err := countMountableLocalEntries(root)
	if err != nil {
		return mountLocalSnapshot{}, err
	}
	return mountLocalSnapshot{Exists: true, EntryCount: count}, nil
}

func buildMountReconcilePlan(ctx context.Context, d *syncDaemon) (mountReconcilePlan, error) {
	if d == nil || d.full == nil {
		return mountReconcilePlan{}, errors.New("mount reconcile: nil daemon")
	}
	local, err := d.full.scanLocalMeta()
	if err != nil {
		return mountReconcilePlan{}, fmt.Errorf("scan local mount path: %w", err)
	}
	remote, err := d.full.scanRemoteMeta(ctx, nil)
	if err != nil {
		return mountReconcilePlan{}, fmt.Errorf("scan remote workspace: %w", err)
	}

	plan := mountReconcilePlan{
		LocalCount:  len(local),
		RemoteCount: len(remote),
		Baseline:    make(map[string]SyncEntry),
	}
	if d.stateWriter != nil {
		if snap := d.stateWriter.snapshot(); snap != nil {
			plan.StateCount = len(snap.Entries)
			plan.StateLiveCount, plan.StateDeletedCount = syncStateEntryCounts(snap)
			if len(snap.Entries) > 0 {
				return buildKnownMountReconcilePlan(ctx, d, local, remote, snap, plan)
			}
		}
	}
	paths := sortedMountReconcilePaths(local, remote)
	now := time.Now().UTC()
	for _, path := range paths {
		l, lok := local[path]
		r, rok := remote[path]
		switch {
		case lok && !rok:
			plan.addLocalOnly(path, l, d.cfg.MaxFileBytes)
		case !lok && rok:
			plan.DownloadCount++
			plan.Operations = append(plan.Operations, mountReconcileOperation{
				Code:    "D",
				Path:    path,
				Kind:    r.kind,
				Details: "remote only; download to local folder",
			})
		case lok && rok:
			entry, same, detail, err := mountBaselineEntry(ctx, d, path, l, r, now)
			if err != nil {
				return mountReconcilePlan{}, err
			}
			if same {
				plan.SameCount++
				plan.Baseline[path] = entry
				continue
			}
			plan.ConflictCount++
			plan.Operations = append(plan.Operations, mountReconcileOperation{
				Code:    "C",
				Path:    path,
				Kind:    l.kind,
				Details: detail,
			})
		}
	}
	return plan, nil
}

func (p mountReconcilePlan) hasReportableChanges() bool {
	return p.ImportCount > 0 || p.UploadCount > 0 || p.DownloadCount > 0 || p.DeleteLocalCount > 0 || p.DeleteRemoteCount > 0 || p.ConflictCount > 0 || p.SkippedCount > 0
}

func (p mountReconcilePlan) requiresConfirmation() bool {
	if p.StateCount > 0 {
		return p.hasReportableChanges()
	}
	if p.LocalCount == 0 {
		return false
	}
	return p.ImportCount > 0 || p.UploadCount > 0 || p.DownloadCount > 0 || p.SkippedCount > 0
}

func (p *mountReconcilePlan) addLocalOnly(path string, meta observedMeta, maxFileBytes int64) {
	if meta.kind == "file" && maxFileBytes > 0 && meta.size > maxFileBytes {
		p.SkippedCount++
		p.Operations = append(p.Operations, mountReconcileOperation{
			Code:    "S",
			Path:    path,
			Kind:    meta.kind,
			Details: fmt.Sprintf("local file is %s; exceeds sync cap %s", formatBytes(meta.size), formatBytes(maxFileBytes)),
		})
		return
	}
	p.ImportCount++
	p.Operations = append(p.Operations, mountReconcileOperation{
		Code:    "I",
		Path:    path,
		Kind:    meta.kind,
		Details: "local only; upload to workspace",
	})
}

func buildKnownMountReconcilePlan(ctx context.Context, d *syncDaemon, local, remote map[string]observedMeta, st *SyncState, plan mountReconcilePlan) (mountReconcilePlan, error) {
	paths := sortedKnownMountReconcilePaths(local, remote, st.Entries)
	now := time.Now().UTC()
	for _, path := range paths {
		stored, hasStored := st.Entries[path]
		l, lok := local[path]
		r, rok := remote[path]
		switch {
		case lok && !rok:
			if hasStored && !stored.Deleted {
				plan.DeleteLocalCount++
				plan.Operations = append(plan.Operations, mountReconcileOperation{
					Code:    "DL",
					Path:    path,
					Kind:    l.kind,
					Details: "remote deleted while unmounted; remove local copy",
				})
				continue
			}
			plan.addKnownUpload(path, l, d.cfg.MaxFileBytes)
		case !lok && rok:
			if hasStored && stored.Deleted {
				if r.kind == "dir" && mountRemoteDirHasLiveDescendants(path, remote, st.Entries) {
					plan.DownloadCount++
					plan.Operations = append(plan.Operations, mountReconcileOperation{
						Code:    "D",
						Path:    path,
						Kind:    r.kind,
						Details: "remote directory has live children; keep local folder for downloads",
					})
					continue
				}
				plan.DeleteRemoteCount++
				plan.Operations = append(plan.Operations, mountReconcileOperation{
					Code:    "DR",
					Path:    path,
					Kind:    r.kind,
					Details: "local deletion pending; remove from workspace",
				})
				continue
			}
			if hasStored {
				if observedChangedFromStored(r, stored, false) {
					plan.addConflict(path, "local deleted while remote changed")
					continue
				}
				plan.DeleteRemoteCount++
				plan.Operations = append(plan.Operations, mountReconcileOperation{
					Code:    "DR",
					Path:    path,
					Kind:    r.kind,
					Details: "local deleted while unmounted; remove from workspace",
				})
				continue
			}
			plan.DownloadCount++
			plan.Operations = append(plan.Operations, mountReconcileOperation{
				Code:    "D",
				Path:    path,
				Kind:    r.kind,
				Details: "remote created while unmounted; download to local folder",
			})
		case lok && rok:
			if !hasStored {
				entry, same, detail, err := mountBaselineEntry(ctx, d, path, l, r, now)
				if err != nil {
					return mountReconcilePlan{}, err
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
				plan.Operations = append(plan.Operations, mountReconcileOperation{
					Code:    "D",
					Path:    path,
					Kind:    r.kind,
					Details: "remote changed while unmounted; download to local folder",
				})
			default:
				entry, same, _, err := mountBaselineEntry(ctx, d, path, l, r, now)
				if err != nil {
					return mountReconcilePlan{}, err
				}
				if same {
					plan.SameCount++
					plan.Baseline[path] = entry
					continue
				}
				plan.addConflict(path, "local and remote both changed while unmounted")
			}
		}
	}
	return plan, nil
}

func (p *mountReconcilePlan) addKnownUpload(path string, meta observedMeta, maxFileBytes int64) {
	if meta.kind == "file" && maxFileBytes > 0 && meta.size > maxFileBytes {
		p.SkippedCount++
		p.Operations = append(p.Operations, mountReconcileOperation{
			Code:    "S",
			Path:    path,
			Kind:    meta.kind,
			Details: fmt.Sprintf("local file is %s; exceeds sync cap %s", formatBytes(meta.size), formatBytes(maxFileBytes)),
		})
		return
	}
	p.UploadCount++
	p.Operations = append(p.Operations, mountReconcileOperation{
		Code:    "U",
		Path:    path,
		Kind:    meta.kind,
		Details: "local changed while unmounted; upload to workspace",
	})
}

func (p *mountReconcilePlan) addConflict(path, details string) {
	p.ConflictCount++
	p.Operations = append(p.Operations, mountReconcileOperation{
		Code:    "C",
		Path:    path,
		Details: details,
	})
}

func mountPlanDeletesRemoteFromEmptyLocal(plan mountReconcilePlan, local mountLocalSnapshot) bool {
	return local.EntryCount == 0 &&
		plan.LocalCount == 0 &&
		plan.RemoteCount > 0 &&
		plan.StateCount > 0 &&
		plan.DeleteRemoteCount > 0
}

func mountPlanShouldResetEmptyLocalState(plan mountReconcilePlan, local mountLocalSnapshot) bool {
	if !mountPlanDeletesRemoteFromEmptyLocal(plan, local) {
		return false
	}
	return !local.Exists || plan.StateLiveCount == 0
}

func mountRemoteDirHasLiveDescendants(path string, remote map[string]observedMeta, entries map[string]SyncEntry) bool {
	prefix := strings.TrimSuffix(path, "/") + "/"
	for child := range remote {
		if !strings.HasPrefix(child, prefix) {
			continue
		}
		if entry, ok := entries[child]; ok && entry.Deleted {
			continue
		}
		return true
	}
	return false
}

func resetMountSyncState(d *syncDaemon) {
	if d == nil || d.stateWriter == nil {
		return
	}
	d.stateWriter.mu.Lock()
	d.stateWriter.state = newSyncState(d.cfg.Workspace, d.cfg.LocalRoot)
	d.stateWriter.dirty = true
	d.stateWriter.mu.Unlock()
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

func mountBaselineEntry(ctx context.Context, d *syncDaemon, path string, local, remote observedMeta, now time.Time) (SyncEntry, bool, string, error) {
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

func approveMountReconcilePlan(d *syncDaemon, plan mountReconcilePlan) {
	if d == nil || d.stateWriter == nil {
		return
	}
	d.cfg.ApprovedInitialMountMerge = true
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

func printMountReconcilePlan(title, workspace, localRoot string, plan mountReconcilePlan, showOperations bool) {
	rows := []outputRow{
		{Label: "workspace", Value: workspace},
		{Label: "path", Value: compactDisplayPath(localRoot)},
		{Label: "local entries", Value: fmt.Sprintf("%d", plan.LocalCount)},
		{Label: "remote entries", Value: fmt.Sprintf("%d", plan.RemoteCount)},
		{Label: "sync state", Value: mountSyncStateDisplay(plan)},
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
	printMountOperationLegend()
	tableRows := make([][]string, 0, len(plan.Operations))
	for _, op := range plan.Operations {
		tableRows = append(tableRows, []string{op.Code, mountOperationPathDisplay(op)})
	}
	printPlainTable([]string{"OP", "PATH"}, tableRows)
	fmt.Println()
}

func mountSyncStateDisplay(plan mountReconcilePlan) string {
	if plan.StateDeletedCount == 0 {
		return fmt.Sprintf("%d", plan.StateCount)
	}
	return fmt.Sprintf("%d live, %d deleted", plan.StateLiveCount, plan.StateDeletedCount)
}

func printMountOperationLegend() {
	fmt.Println("# OPs: I=Import, D=Download, U=Upload, DL=Delete local, DR=Delete remote,")
	fmt.Println("#      C=Conflict, S=Skipped")
}

func mountOperationPathDisplay(op mountReconcileOperation) string {
	path := strings.TrimSpace(op.Path)
	if path == "" {
		path = "."
	}
	if op.Kind == "dir" && !strings.HasSuffix(path, "/") {
		path += "/"
	}
	if detail := mountOperationInlineDetail(op); detail != "" {
		path += " (" + detail + ")"
	}
	return path
}

func mountOperationInlineDetail(op mountReconcileOperation) string {
	switch op.Code {
	case "C", "DC", "UC", "S":
		return strings.TrimSpace(op.Details)
	default:
		return ""
	}
}

func sortedMountReconcilePaths(local, remote map[string]observedMeta) []string {
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

func sortedKnownMountReconcilePaths(local, remote map[string]observedMeta, entries map[string]SyncEntry) []string {
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
