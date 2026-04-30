package controlplane

import (
	"context"
	"sort"
	"strconv"
	"strings"
)

const (
	DiffOpCreate   = "create"
	DiffOpUpdate   = "update"
	DiffOpDelete   = "delete"
	DiffOpRename   = "rename"
	DiffOpMetadata = "metadata"
)

type DiffState struct {
	View         string `json:"view"`
	CheckpointID string `json:"checkpoint_id,omitempty"`
	ManifestHash string `json:"manifest_hash,omitempty"`
	FileCount    int    `json:"file_count"`
	FolderCount  int    `json:"folder_count"`
	TotalBytes   int64  `json:"total_bytes"`
}

type DiffSummary struct {
	Total           int   `json:"total"`
	Created         int   `json:"created"`
	Updated         int   `json:"updated"`
	Deleted         int   `json:"deleted"`
	Renamed         int   `json:"renamed"`
	MetadataChanged int   `json:"metadata_changed"`
	BytesAdded      int64 `json:"bytes_added"`
	BytesRemoved    int64 `json:"bytes_removed"`
}

type DiffEntry struct {
	Op                string `json:"op"`
	Path              string `json:"path"`
	PreviousPath      string `json:"previous_path,omitempty"`
	Kind              string `json:"kind,omitempty"`
	PreviousKind      string `json:"previous_kind,omitempty"`
	SizeBytes         int64  `json:"size_bytes,omitempty"`
	PreviousSizeBytes int64  `json:"previous_size_bytes,omitempty"`
	DeltaBytes        int64  `json:"delta_bytes,omitempty"`
	ContentHash       string `json:"content_hash,omitempty"`
	PreviousHash      string `json:"previous_hash,omitempty"`
	Mode              uint32 `json:"mode,omitempty"`
	PreviousMode      uint32 `json:"previous_mode,omitempty"`
}

type WorkspaceDiffResponse struct {
	WorkspaceID   string      `json:"workspace_id"`
	WorkspaceName string      `json:"workspace_name,omitempty"`
	Base          DiffState   `json:"base"`
	Head          DiffState   `json:"head"`
	Summary       DiffSummary `json:"summary"`
	Entries       []DiffEntry `json:"entries"`
}

func (s *Service) DiffWorkspace(ctx context.Context, workspace, baseView, headView string) (WorkspaceDiffResponse, error) {
	baseView = defaultString(strings.TrimSpace(baseView), "head")
	headView = defaultString(strings.TrimSpace(headView), "working-copy")

	meta, baseState, baseManifest, err := s.resolveDiffManifestForView(ctx, workspace, baseView)
	if err != nil {
		return WorkspaceDiffResponse{}, err
	}
	_, headState, headManifest, err := s.resolveDiffManifestForView(ctx, workspace, headView)
	if err != nil {
		return WorkspaceDiffResponse{}, err
	}
	entries := diffManifests(baseManifest, headManifest)
	return WorkspaceDiffResponse{
		WorkspaceID:   workspaceStorageID(meta),
		WorkspaceName: meta.Name,
		Base:          baseState,
		Head:          headState,
		Summary:       summarizeDiffEntries(entries),
		Entries:       entries,
	}, nil
}

func (s *Service) resolveDiffManifestForView(ctx context.Context, workspace, rawView string) (WorkspaceMeta, DiffState, Manifest, error) {
	view, err := parseViewRef(rawView)
	if err != nil {
		return WorkspaceMeta{}, DiffState{}, Manifest{}, err
	}
	meta, err := s.store.GetWorkspaceMeta(ctx, workspace)
	if err != nil {
		return WorkspaceMeta{}, DiffState{}, Manifest{}, err
	}
	meta = applyWorkspaceMetaDefaults(s.cfg, meta)
	storageID := workspaceStorageID(meta)

	if view.Kind == "working-copy" {
		if _, _, _, err := EnsureWorkspaceRoot(ctx, s.store, storageID); err != nil {
			return WorkspaceMeta{}, DiffState{}, Manifest{}, err
		}
		manifestValue, _, _, _, _, err := BuildManifestFromWorkspaceRoot(ctx, s.store.rdb, storageID, "working-copy")
		if err != nil {
			return WorkspaceMeta{}, DiffState{}, Manifest{}, err
		}
		manifestHash, err := HashManifest(manifestValue)
		if err != nil {
			return WorkspaceMeta{}, DiffState{}, Manifest{}, err
		}
		stats := manifestStats(manifestValue)
		return meta, DiffState{
			View:         viewName(view, ""),
			ManifestHash: manifestHash,
			FileCount:    stats.FileCount,
			FolderCount:  stats.DirCount,
			TotalBytes:   stats.TotalBytes,
		}, manifestValue, nil
	}

	_, checkpoint, manifestValue, err := s.resolveManifestForView(ctx, storageID, view)
	if err != nil {
		return WorkspaceMeta{}, DiffState{}, Manifest{}, err
	}
	return meta, DiffState{
		View:         viewName(view, checkpoint.ID),
		CheckpointID: checkpoint.ID,
		ManifestHash: checkpoint.ManifestHash,
		FileCount:    checkpoint.FileCount,
		FolderCount:  checkpoint.DirCount,
		TotalBytes:   checkpoint.TotalBytes,
	}, manifestValue, nil
}

func diffManifests(base, head Manifest) []DiffEntry {
	baseEntries := base.Entries
	if baseEntries == nil {
		baseEntries = map[string]ManifestEntry{}
	}
	headEntries := head.Entries
	if headEntries == nil {
		headEntries = map[string]ManifestEntry{}
	}

	deleted := map[string]ManifestEntry{}
	created := map[string]ManifestEntry{}
	paths := map[string]struct{}{}
	for p, entry := range baseEntries {
		if p == "/" {
			continue
		}
		if _, ok := headEntries[p]; !ok {
			deleted[p] = entry
		}
		paths[p] = struct{}{}
	}
	for p, entry := range headEntries {
		if p == "/" {
			continue
		}
		if _, ok := baseEntries[p]; !ok {
			created[p] = entry
		}
		paths[p] = struct{}{}
	}

	entries := detectRenames(deleted, created)
	renamedPaths := map[string]struct{}{}
	for _, entry := range entries {
		delete(deleted, entry.PreviousPath)
		delete(created, entry.Path)
		renamedPaths[entry.PreviousPath] = struct{}{}
		renamedPaths[entry.Path] = struct{}{}
	}

	for p := range paths {
		if _, ok := renamedPaths[p]; ok {
			continue
		}
		prev, hadPrev := baseEntries[p]
		next, hasNext := headEntries[p]
		switch {
		case hadPrev && !hasNext:
			entries = append(entries, deletedDiffEntry(p, prev))
		case !hadPrev && hasNext:
			entries = append(entries, createdDiffEntry(p, next))
		case hadPrev && hasNext:
			if manifestEntryEquivalent(prev, next) {
				continue
			}
			entries = append(entries, updatedDiffEntry(p, prev, next))
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Path == entries[j].Path {
			return entries[i].PreviousPath < entries[j].PreviousPath
		}
		return entries[i].Path < entries[j].Path
	})
	return entries
}

func detectRenames(deleted, created map[string]ManifestEntry) []DiffEntry {
	deletedByKey := map[string][]string{}
	createdByKey := map[string][]string{}
	for p, entry := range deleted {
		if key := renameDiffKey(entry); key != "" {
			deletedByKey[key] = append(deletedByKey[key], p)
		}
	}
	for p, entry := range created {
		if key := renameDiffKey(entry); key != "" {
			createdByKey[key] = append(createdByKey[key], p)
		}
	}

	keys := make([]string, 0, len(deletedByKey))
	for key := range deletedByKey {
		if len(createdByKey[key]) > 0 {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	renames := make([]DiffEntry, 0)
	for _, key := range keys {
		oldPaths := deletedByKey[key]
		newPaths := createdByKey[key]
		sort.Strings(oldPaths)
		sort.Strings(newPaths)
		limit := len(oldPaths)
		if len(newPaths) < limit {
			limit = len(newPaths)
		}
		for i := 0; i < limit; i++ {
			prev := deleted[oldPaths[i]]
			next := created[newPaths[i]]
			renames = append(renames, DiffEntry{
				Op:                DiffOpRename,
				Path:              newPaths[i],
				PreviousPath:      oldPaths[i],
				Kind:              next.Type,
				PreviousKind:      prev.Type,
				SizeBytes:         next.Size,
				PreviousSizeBytes: prev.Size,
				ContentHash:       entryHash(next),
				PreviousHash:      entryHash(prev),
				Mode:              next.Mode,
				PreviousMode:      prev.Mode,
			})
		}
	}
	return renames
}

func renameDiffKey(entry ManifestEntry) string {
	if entry.Type != "file" && entry.Type != "symlink" {
		return ""
	}
	hash := entryHash(entry)
	if hash == "" {
		return ""
	}
	return entry.Type + "\x00" + hash + "\x00" + strconv.FormatInt(entry.Size, 10)
}

func createdDiffEntry(p string, next ManifestEntry) DiffEntry {
	return DiffEntry{
		Op:          DiffOpCreate,
		Path:        p,
		Kind:        next.Type,
		SizeBytes:   next.Size,
		DeltaBytes:  next.Size,
		ContentHash: entryHash(next),
		Mode:        next.Mode,
	}
}

func deletedDiffEntry(p string, prev ManifestEntry) DiffEntry {
	return DiffEntry{
		Op:                DiffOpDelete,
		Path:              p,
		PreviousKind:      prev.Type,
		PreviousSizeBytes: prev.Size,
		DeltaBytes:        -prev.Size,
		PreviousHash:      entryHash(prev),
		PreviousMode:      prev.Mode,
	}
}

func updatedDiffEntry(p string, prev, next ManifestEntry) DiffEntry {
	op := DiffOpUpdate
	if entryContentEquivalent(prev, next) {
		op = DiffOpMetadata
	}
	return DiffEntry{
		Op:                op,
		Path:              p,
		Kind:              next.Type,
		PreviousKind:      prev.Type,
		SizeBytes:         next.Size,
		PreviousSizeBytes: prev.Size,
		DeltaBytes:        next.Size - prev.Size,
		ContentHash:       entryHash(next),
		PreviousHash:      entryHash(prev),
		Mode:              next.Mode,
		PreviousMode:      prev.Mode,
	}
}

func summarizeDiffEntries(entries []DiffEntry) DiffSummary {
	summary := DiffSummary{Total: len(entries)}
	for _, entry := range entries {
		switch entry.Op {
		case DiffOpCreate:
			summary.Created++
		case DiffOpUpdate:
			summary.Updated++
		case DiffOpDelete:
			summary.Deleted++
		case DiffOpRename:
			summary.Renamed++
		case DiffOpMetadata:
			summary.MetadataChanged++
		}
		if entry.DeltaBytes > 0 {
			summary.BytesAdded += entry.DeltaBytes
		} else if entry.DeltaBytes < 0 {
			summary.BytesRemoved += -entry.DeltaBytes
		}
	}
	return summary
}
