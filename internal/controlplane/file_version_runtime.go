package controlplane

import (
	"bytes"
	"context"
	"errors"
	"os"
	"sort"
	"strings"
	"time"
)

type VersionedFileSnapshot struct {
	Path        string
	Exists      bool
	Kind        string
	Mode        uint32
	Content     []byte
	Target      string
	BlobID      string
	ContentHash string
	SizeBytes   int64
}

type FileVersionMutationMetadata struct {
	Source       string
	SessionID    string
	AgentID      string
	User         string
	CheckpointID string
}

func (s *Store) RecordFileVersionMutation(ctx context.Context, workspace string, before, after VersionedFileSnapshot, metadata FileVersionMutationMetadata) (*FileVersion, error) {
	if !before.Exists && !after.Exists {
		return nil, nil
	}

	var err error
	before, err = normalizeVersionedSnapshot(before)
	if err != nil {
		return nil, err
	}
	after, err = normalizeVersionedSnapshot(after)
	if err != nil {
		return nil, err
	}

	livePath := after.Path
	if !after.Exists {
		livePath = before.Path
	}
	rename := before.Exists && after.Exists && before.Path != "" && after.Path != "" && before.Path != after.Path

	lineagePath := livePath
	if rename {
		lineagePath = before.Path
	}
	lineage, err := s.ResolveLiveFileLineageByPath(ctx, workspace, lineagePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	trackedAlready := err == nil
	if !trackedAlready {
		shouldTrack, trackErr := s.shouldTrackVersionedPath(ctx, workspace, livePath)
		if trackErr != nil {
			return nil, trackErr
		}
		if !shouldTrack || !after.Exists {
			return nil, nil
		}
		lineage, err = s.CreateFileLineage(ctx, workspace, livePath, time.Now().UTC())
		if err != nil {
			return nil, err
		}
	}
	policy, err := s.GetWorkspaceVersioningPolicy(ctx, workspace)
	if err != nil {
		return nil, err
	}
	after = applyLargeFileGuardrail(policy, after)
	if after.Exists && after.Kind == "file" && after.BlobID != "" && after.Content != nil {
		if err := s.SaveBlobs(ctx, workspace, map[string][]byte{after.BlobID: after.Content}); err != nil {
			return nil, err
		}
	}

	version := FileVersion{
		Path:          livePath,
		Kind:          versionKindForSnapshot(after, before),
		Source:        strings.TrimSpace(metadata.Source),
		SessionID:     strings.TrimSpace(metadata.SessionID),
		AgentID:       strings.TrimSpace(metadata.AgentID),
		User:          strings.TrimSpace(metadata.User),
		CheckpointIDs: nil,
		CreatedAt:     time.Now().UTC(),
	}
	if checkpointID := strings.TrimSpace(metadata.CheckpointID); checkpointID != "" {
		version.CheckpointIDs = []string{checkpointID}
	}

	switch {
	case !before.Exists && after.Exists:
		populateVersionFromSnapshot(&version, after)
		version.Op = versionOpForSnapshot(after)

	case before.Exists && after.Exists:
		if rename {
			populateVersionFromSnapshot(&version, after)
			version.PrevPath = before.Path
			version.PrevHash = before.ContentHash
			version.DeltaBytes = after.SizeBytes - before.SizeBytes
			version.Op = ChangeOpRename
			break
		}
		beforeEqual := snapshotsEquivalent(before, after)
		if beforeEqual {
			if strings.TrimSpace(metadata.Source) != ChangeSourceVersionRestore {
				return nil, nil
			}
			populateVersionFromSnapshot(&version, after)
			version.PrevHash = before.ContentHash
			version.DeltaBytes = 0
			version.Op = versionOpForSnapshot(after)
			break
		}
		populateVersionFromSnapshot(&version, after)
		version.PrevHash = before.ContentHash
		version.DeltaBytes = after.SizeBytes - before.SizeBytes
		if contentOnlyEqual(before, after) {
			version.Op = ChangeOpChmod
		} else {
			version.Op = versionOpForSnapshot(after)
		}

	case before.Exists && !after.Exists:
		version.Op = ChangeOpDelete
		version.Kind = FileVersionKindTombstone
		version.PrevHash = before.ContentHash
		version.DeltaBytes = -before.SizeBytes
		version.Mode = before.Mode
	}

	var appended FileVersion
	if rename {
		appended, err = s.RenameFileLineageWithVersion(ctx, workspace, lineage.FileID, before, version)
	} else if before.Exists && !skipFileVersionHeadValidation(metadata.Source) {
		appended, err = s.AppendFileVersionChecked(ctx, workspace, lineage.FileID, before, version, !after.Exists)
	} else {
		appended, err = s.AppendFileVersion(ctx, workspace, lineage.FileID, version)
	}
	if err != nil {
		return nil, err
	}
	return &appended, nil
}

func skipFileVersionHeadValidation(source string) bool {
	switch strings.TrimSpace(source) {
	case ChangeSourceVersionRestore, ChangeSourceCheckpoint, ChangeSourceImport, ChangeSourceServerRestore, "checkpoint_restore":
		return true
	default:
		return false
	}
}

func (s *Store) RecordManifestVersionChanges(ctx context.Context, workspace string, before, after Manifest, metadata FileVersionMutationMetadata) error {
	_, err := s.recordManifestVersionChanges(ctx, workspace, before, after, metadata)
	return err
}

func (s *Store) RecordManifestVersionChangesWithResults(ctx context.Context, workspace string, before, after Manifest, metadata FileVersionMutationMetadata) (map[string]*FileVersion, error) {
	return s.recordManifestVersionChanges(ctx, workspace, before, after, metadata)
}

func (s *Store) recordManifestVersionChanges(ctx context.Context, workspace string, before, after Manifest, metadata FileVersionMutationMetadata) (map[string]*FileVersion, error) {
	paths := make(map[string]struct{}, len(before.Entries)+len(after.Entries))
	for path := range before.Entries {
		paths[path] = struct{}{}
	}
	for path := range after.Entries {
		paths[path] = struct{}{}
	}
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		if path == "/" {
			continue
		}
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	results := make(map[string]*FileVersion, len(ordered))

	for _, manifestPath := range ordered {
		beforeSnapshot, err := s.snapshotFromManifestEntry(ctx, workspace, manifestPath, before.Entries[manifestPath])
		if err != nil {
			return nil, err
		}
		if entry, ok := before.Entries[manifestPath]; !ok || entry.Type == "dir" {
			beforeSnapshot.Exists = false
		}

		afterSnapshot, err := s.snapshotFromManifestEntry(ctx, workspace, manifestPath, after.Entries[manifestPath])
		if err != nil {
			return nil, err
		}
		if entry, ok := after.Entries[manifestPath]; !ok || entry.Type == "dir" {
			afterSnapshot.Exists = false
		}

		version, err := s.RecordFileVersionMutation(ctx, workspace, beforeSnapshot, afterSnapshot, metadata)
		if err != nil {
			return nil, err
		}
		if version != nil {
			results[manifestPath] = version
		}
	}

	return results, nil
}

func (s *Store) shouldTrackVersionedPath(ctx context.Context, workspace, path string) (bool, error) {
	policy, err := s.GetWorkspaceVersioningPolicy(ctx, workspace)
	if err != nil {
		return false, err
	}
	return WorkspaceVersioningPolicyTracksPath(policy, path)
}

func (s *Store) snapshotFromManifestEntry(ctx context.Context, workspace, manifestPath string, entry ManifestEntry) (VersionedFileSnapshot, error) {
	snapshot := VersionedFileSnapshot{
		Path:   manifestPath,
		Exists: entry.Type != "",
		Kind:   entry.Type,
		Mode:   entry.Mode,
		BlobID: entry.BlobID,
	}
	switch entry.Type {
	case "":
		return snapshot, nil
	case "dir":
		snapshot.Exists = false
		return snapshot, nil
	case "symlink":
		snapshot.Target = entry.Target
		snapshot.SizeBytes = int64(len(entry.Target))
		return completeVersionedSnapshot(snapshot), nil
	case "file":
		data, err := ManifestEntryData(entry, func(blobID string) ([]byte, error) {
			return s.GetBlob(ctx, workspace, blobID)
		})
		if err != nil {
			return VersionedFileSnapshot{}, err
		}
		snapshot.Content = data
		snapshot.SizeBytes = entry.Size
		return completeVersionedSnapshot(snapshot), nil
	default:
		return snapshot, nil
	}
}

func normalizeVersionedSnapshot(snapshot VersionedFileSnapshot) (VersionedFileSnapshot, error) {
	if !snapshot.Exists {
		if strings.TrimSpace(snapshot.Path) == "" {
			return snapshot, nil
		}
		normalized, err := normalizeVersionedPath(snapshot.Path)
		if err != nil {
			return VersionedFileSnapshot{}, err
		}
		snapshot.Path = normalized
		return snapshot, nil
	}
	normalized, err := normalizeVersionedPath(snapshot.Path)
	if err != nil {
		return VersionedFileSnapshot{}, err
	}
	snapshot.Path = normalized
	return completeVersionedSnapshot(snapshot), nil
}

func completeVersionedSnapshot(snapshot VersionedFileSnapshot) VersionedFileSnapshot {
	if !snapshot.Exists {
		return snapshot
	}
	switch snapshot.Kind {
	case "symlink":
		if snapshot.SizeBytes == 0 {
			snapshot.SizeBytes = int64(len(snapshot.Target))
		}
		if strings.TrimSpace(snapshot.ContentHash) == "" {
			snapshot.ContentHash = "symlink:" + snapshot.Target
		}
	case "file":
		if snapshot.SizeBytes == 0 {
			snapshot.SizeBytes = int64(len(snapshot.Content))
		}
		if strings.TrimSpace(snapshot.ContentHash) == "" {
			snapshot.ContentHash = textSHA256(string(snapshot.Content))
		}
		if strings.TrimSpace(snapshot.BlobID) == "" {
			snapshot.BlobID = snapshot.ContentHash
		}
	}
	return snapshot
}

func versionKindForSnapshot(after, before VersionedFileSnapshot) string {
	if after.Exists {
		switch after.Kind {
		case "symlink":
			return FileVersionKindSymlink
		default:
			return FileVersionKindFile
		}
	}
	if before.Kind == "symlink" {
		return FileVersionKindSymlink
	}
	return FileVersionKindFile
}

func versionOpForSnapshot(snapshot VersionedFileSnapshot) string {
	if snapshot.Kind == "symlink" {
		return ChangeOpSymlink
	}
	return ChangeOpPut
}

func snapshotsEquivalent(before, after VersionedFileSnapshot) bool {
	if before.Exists != after.Exists || before.Kind != after.Kind || before.Mode != after.Mode {
		return false
	}
	switch after.Kind {
	case "symlink":
		return before.Target == after.Target
	case "file":
		if before.ContentHash != "" && after.ContentHash != "" {
			return before.ContentHash == after.ContentHash
		}
		return bytes.Equal(before.Content, after.Content)
	default:
		return true
	}
}

func contentOnlyEqual(before, after VersionedFileSnapshot) bool {
	if before.Kind != after.Kind {
		return false
	}
	switch after.Kind {
	case "symlink":
		return before.Target == after.Target
	case "file":
		if before.ContentHash != "" && after.ContentHash != "" {
			return before.ContentHash == after.ContentHash
		}
		return bytes.Equal(before.Content, after.Content)
	default:
		return true
	}
}

func populateVersionFromSnapshot(version *FileVersion, snapshot VersionedFileSnapshot) {
	version.Mode = snapshot.Mode
	version.BlobID = snapshot.BlobID
	version.ContentHash = snapshot.ContentHash
	version.SizeBytes = snapshot.SizeBytes
	if snapshot.Kind == "symlink" {
		version.Target = snapshot.Target
	}
}
