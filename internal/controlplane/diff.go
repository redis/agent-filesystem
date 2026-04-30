package controlplane

import (
	"bytes"
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

const (
	diffTextMaxBytes = 256 * 1024
	diffTextMaxLines = 4000
	diffTextContext  = 3
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
	Op                string    `json:"op"`
	Path              string    `json:"path"`
	PreviousPath      string    `json:"previous_path,omitempty"`
	Kind              string    `json:"kind,omitempty"`
	PreviousKind      string    `json:"previous_kind,omitempty"`
	SizeBytes         int64     `json:"size_bytes,omitempty"`
	PreviousSizeBytes int64     `json:"previous_size_bytes,omitempty"`
	DeltaBytes        int64     `json:"delta_bytes,omitempty"`
	ContentHash       string    `json:"content_hash,omitempty"`
	PreviousHash      string    `json:"previous_hash,omitempty"`
	Mode              uint32    `json:"mode,omitempty"`
	PreviousMode      uint32    `json:"previous_mode,omitempty"`
	TextDiff          *TextDiff `json:"text_diff,omitempty"`
}

type TextDiff struct {
	Available     bool           `json:"available"`
	SkippedReason string         `json:"skipped_reason,omitempty"`
	Language      string         `json:"language,omitempty"`
	MaxBytes      int            `json:"max_bytes,omitempty"`
	Hunks         []TextDiffHunk `json:"hunks,omitempty"`
}

type TextDiffHunk struct {
	OldStart int            `json:"old_start"`
	OldLines int            `json:"old_lines"`
	NewStart int            `json:"new_start"`
	NewLines int            `json:"new_lines"`
	Lines    []TextDiffLine `json:"lines"`
}

type TextDiffLine struct {
	Kind    string `json:"kind"`
	OldLine int    `json:"old_line,omitempty"`
	NewLine int    `json:"new_line,omitempty"`
	Text    string `json:"text"`
}

type WorkspaceDiffResponse struct {
	WorkspaceID   string      `json:"workspace_id"`
	WorkspaceName string      `json:"workspace_name,omitempty"`
	Base          DiffState   `json:"base"`
	Head          DiffState   `json:"head"`
	Summary       DiffSummary `json:"summary"`
	Entries       []DiffEntry `json:"entries"`
}

type diffManifestSnapshot struct {
	Meta     WorkspaceMeta
	State    DiffState
	Manifest Manifest
	Blobs    map[string][]byte
}

func (s *Service) DiffWorkspace(ctx context.Context, workspace, baseView, headView string) (WorkspaceDiffResponse, error) {
	baseView = defaultString(strings.TrimSpace(baseView), "head")
	headView = defaultString(strings.TrimSpace(headView), "working-copy")

	baseSnapshot, err := s.resolveDiffManifestSnapshot(ctx, workspace, baseView)
	if err != nil {
		return WorkspaceDiffResponse{}, err
	}
	headSnapshot, err := s.resolveDiffManifestSnapshot(ctx, workspace, headView)
	if err != nil {
		return WorkspaceDiffResponse{}, err
	}
	entries := diffManifests(baseSnapshot.Manifest, headSnapshot.Manifest)
	entries = s.enrichDiffEntriesWithText(ctx, workspaceStorageID(baseSnapshot.Meta), baseSnapshot, headSnapshot, entries)
	return WorkspaceDiffResponse{
		WorkspaceID:   workspaceStorageID(baseSnapshot.Meta),
		WorkspaceName: baseSnapshot.Meta.Name,
		Base:          baseSnapshot.State,
		Head:          headSnapshot.State,
		Summary:       summarizeDiffEntries(entries),
		Entries:       entries,
	}, nil
}

func (s *Service) resolveDiffManifestSnapshot(ctx context.Context, workspace, rawView string) (diffManifestSnapshot, error) {
	view, err := parseViewRef(rawView)
	if err != nil {
		return diffManifestSnapshot{}, err
	}
	meta, err := s.store.GetWorkspaceMeta(ctx, workspace)
	if err != nil {
		return diffManifestSnapshot{}, err
	}
	meta = applyWorkspaceMetaDefaults(s.cfg, meta)
	storageID := workspaceStorageID(meta)

	if view.Kind == "working-copy" {
		if _, _, _, err := EnsureWorkspaceRoot(ctx, s.store, storageID); err != nil {
			return diffManifestSnapshot{}, err
		}
		manifestValue, blobs, _, _, _, err := BuildManifestFromWorkspaceRoot(ctx, s.store.rdb, storageID, "working-copy")
		if err != nil {
			return diffManifestSnapshot{}, err
		}
		manifestHash, err := HashManifest(manifestValue)
		if err != nil {
			return diffManifestSnapshot{}, err
		}
		stats := manifestStats(manifestValue)
		return diffManifestSnapshot{
			Meta: meta,
			State: DiffState{
				View:         viewName(view, ""),
				ManifestHash: manifestHash,
				FileCount:    stats.FileCount,
				FolderCount:  stats.DirCount,
				TotalBytes:   stats.TotalBytes,
			},
			Manifest: manifestValue,
			Blobs:    blobs,
		}, nil
	}

	_, checkpoint, manifestValue, err := s.resolveManifestForView(ctx, storageID, view)
	if err != nil {
		return diffManifestSnapshot{}, err
	}
	return diffManifestSnapshot{
		Meta: meta,
		State: DiffState{
			View:         viewName(view, checkpoint.ID),
			CheckpointID: checkpoint.ID,
			ManifestHash: checkpoint.ManifestHash,
			FileCount:    checkpoint.FileCount,
			FolderCount:  checkpoint.DirCount,
			TotalBytes:   checkpoint.TotalBytes,
		},
		Manifest: manifestValue,
	}, nil
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

func (s *Service) enrichDiffEntriesWithText(ctx context.Context, storageID string, base, head diffManifestSnapshot, entries []DiffEntry) []DiffEntry {
	for i := range entries {
		entry := &entries[i]
		switch entry.Op {
		case DiffOpCreate, DiffOpUpdate, DiffOpDelete:
		default:
			continue
		}
		var (
			oldEntry ManifestEntry
			newEntry ManifestEntry
			hadOld   bool
			hasNew   bool
		)
		if entry.Op != DiffOpCreate {
			oldEntry, hadOld = base.Manifest.Entries[entry.Path]
		}
		if entry.Op != DiffOpDelete {
			newEntry, hasNew = head.Manifest.Entries[entry.Path]
		}
		entry.TextDiff = s.textDiffForEntries(ctx, storageID, entry.Path, oldEntry, hadOld, base.Blobs, newEntry, hasNew, head.Blobs)
	}
	return entries
}

func (s *Service) textDiffForEntries(ctx context.Context, storageID, p string, oldEntry ManifestEntry, hadOld bool, oldBlobs map[string][]byte, newEntry ManifestEntry, hasNew bool, newBlobs map[string][]byte) *TextDiff {
	if hadOld && oldEntry.Type != "file" && oldEntry.Type != "symlink" {
		return nil
	}
	if hasNew && newEntry.Type != "file" && newEntry.Type != "symlink" {
		return nil
	}
	if hadOld && oldEntry.Type == "file" && oldEntry.Size > diffTextMaxBytes {
		return skippedTextDiff("too_large")
	}
	if hasNew && newEntry.Type == "file" && newEntry.Size > diffTextMaxBytes {
		return skippedTextDiff("too_large")
	}

	oldData, oldOK, oldReason := s.diffEntryData(ctx, storageID, oldEntry, hadOld, oldBlobs)
	if !oldOK {
		return skippedTextDiff(oldReason)
	}
	newData, newOK, newReason := s.diffEntryData(ctx, storageID, newEntry, hasNew, newBlobs)
	if !newOK {
		return skippedTextDiff(newReason)
	}
	if len(oldData)+len(newData) > diffTextMaxBytes {
		return skippedTextDiff("too_large")
	}
	if isBinary(oldData) || isBinary(newData) {
		return skippedTextDiff("binary")
	}
	oldLines := splitDiffLines(string(oldData))
	newLines := splitDiffLines(string(newData))
	if len(oldLines)+len(newLines) > diffTextMaxLines {
		return skippedTextDiff("too_many_lines")
	}
	hunks := buildTextDiffHunks(oldLines, newLines)
	if len(hunks) == 0 {
		return nil
	}
	return &TextDiff{
		Available: true,
		Language:  language(p),
		MaxBytes:  diffTextMaxBytes,
		Hunks:     hunks,
	}
}

func (s *Service) diffEntryData(ctx context.Context, storageID string, entry ManifestEntry, ok bool, blobs map[string][]byte) ([]byte, bool, string) {
	if !ok {
		return nil, true, ""
	}
	if entry.Type == "symlink" {
		return []byte(entry.Target), true, ""
	}
	if entry.Type != "file" {
		return nil, false, "unsupported_kind"
	}
	data, err := ManifestEntryData(entry, func(blobID string) ([]byte, error) {
		if data, ok := blobs[blobID]; ok {
			return data, nil
		}
		return s.store.GetBlob(ctx, storageID, blobID)
	})
	if err != nil {
		return nil, false, "content_unavailable"
	}
	return data, true, ""
}

func skippedTextDiff(reason string) *TextDiff {
	return &TextDiff{
		Available:     false,
		SkippedReason: reason,
		MaxBytes:      diffTextMaxBytes,
	}
}

func splitDiffLines(text string) []string {
	if text == "" {
		return nil
	}
	raw := strings.SplitAfter(text, "\n")
	if len(raw) > 0 && raw[len(raw)-1] == "" {
		raw = raw[:len(raw)-1]
	}
	for i := range raw {
		raw[i] = strings.TrimSuffix(raw[i], "\n")
		raw[i] = strings.TrimSuffix(raw[i], "\r")
	}
	return raw
}

func buildTextDiffHunks(oldLines, newLines []string) []TextDiffHunk {
	if bytes.Equal([]byte(strings.Join(oldLines, "\n")), []byte(strings.Join(newLines, "\n"))) {
		return nil
	}
	prefix := 0
	for prefix < len(oldLines) && prefix < len(newLines) && oldLines[prefix] == newLines[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(oldLines)-prefix && suffix < len(newLines)-prefix &&
		oldLines[len(oldLines)-1-suffix] == newLines[len(newLines)-1-suffix] {
		suffix++
	}

	contextStart := maxInt(0, prefix-diffTextContext)
	oldChangeEnd := len(oldLines) - suffix
	newChangeEnd := len(newLines) - suffix
	oldContextEnd := minInt(len(oldLines), oldChangeEnd+diffTextContext)
	newContextEnd := minInt(len(newLines), newChangeEnd+diffTextContext)

	lines := make([]TextDiffLine, 0, (oldContextEnd-contextStart)+(newContextEnd-prefix))
	for i := contextStart; i < prefix; i++ {
		lines = append(lines, TextDiffLine{Kind: "context", OldLine: i + 1, NewLine: i + 1, Text: oldLines[i]})
	}
	for i := prefix; i < oldChangeEnd; i++ {
		lines = append(lines, TextDiffLine{Kind: "delete", OldLine: i + 1, Text: oldLines[i]})
	}
	for i := prefix; i < newChangeEnd; i++ {
		lines = append(lines, TextDiffLine{Kind: "insert", NewLine: i + 1, Text: newLines[i]})
	}
	for offset := 0; offset < oldContextEnd-oldChangeEnd && offset < newContextEnd-newChangeEnd; offset++ {
		oldIndex := oldChangeEnd + offset
		newIndex := newChangeEnd + offset
		lines = append(lines, TextDiffLine{Kind: "context", OldLine: oldIndex + 1, NewLine: newIndex + 1, Text: oldLines[oldIndex]})
	}
	if len(lines) == 0 {
		return nil
	}
	return []TextDiffHunk{{
		OldStart: contextStart + 1,
		OldLines: oldContextEnd - contextStart,
		NewStart: contextStart + 1,
		NewLines: newContextEnd - contextStart,
		Lines:    lines,
	}}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
