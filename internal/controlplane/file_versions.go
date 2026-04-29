package controlplane

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	FileLineageStateLive    = "live"
	FileLineageStateDeleted = "deleted"
)

const (
	FileVersionKindFile      = "file"
	FileVersionKindSymlink   = "symlink"
	FileVersionKindTombstone = "tombstone"
)

type FileLineage struct {
	FileID        string    `json:"file_id"`
	WorkspaceID   string    `json:"workspace_id"`
	CurrentPath   string    `json:"current_path"`
	State         string    `json:"state"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	DeletedAt     time.Time `json:"deleted_at,omitempty"`
	HeadOrdinal   int64     `json:"head_ordinal,omitempty"`
	HeadVersionID string    `json:"head_version_id,omitempty"`
}

type FileVersion struct {
	VersionID     string    `json:"version_id"`
	FileID        string    `json:"file_id"`
	Ordinal       int64     `json:"ordinal"`
	Path          string    `json:"path"`
	PrevPath      string    `json:"prev_path,omitempty"`
	Op            string    `json:"op"`
	Kind          string    `json:"kind"`
	BlobID        string    `json:"blob_id,omitempty"`
	ContentHash   string    `json:"content_hash,omitempty"`
	PrevHash      string    `json:"prev_hash,omitempty"`
	SizeBytes     int64     `json:"size_bytes,omitempty"`
	DeltaBytes    int64     `json:"delta_bytes,omitempty"`
	Mode          uint32    `json:"mode,omitempty"`
	Target        string    `json:"target,omitempty"`
	Source        string    `json:"source,omitempty"`
	SessionID     string    `json:"session_id,omitempty"`
	AgentID       string    `json:"agent_id,omitempty"`
	User          string    `json:"user,omitempty"`
	CheckpointIDs []string  `json:"checkpoint_ids,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

func (s *Store) CreateFileLineage(ctx context.Context, workspace, path string, createdAt time.Time) (FileLineage, error) {
	_, storageID, err := s.resolveWorkspaceMeta(ctx, workspace)
	if err != nil {
		return FileLineage{}, err
	}
	normalizedPath, err := normalizeVersionedPath(path)
	if err != nil {
		return FileLineage{}, err
	}
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	fileID, err := newFileLineageID()
	if err != nil {
		return FileLineage{}, err
	}

	lineage := FileLineage{
		FileID:      fileID,
		WorkspaceID: storageID,
		CurrentPath: normalizedPath,
		State:       FileLineageStateLive,
		CreatedAt:   createdAt,
		UpdatedAt:   createdAt,
	}

	for attempt := 0; attempt < 8; attempt++ {
		err = s.rdb.Watch(ctx, func(tx *redis.Tx) error {
			currentID, err := tx.HGet(ctx, workspacePathFileIDsKey(storageID), normalizedPath).Result()
			if err != nil && err != redis.Nil {
				return err
			}
			if err == nil && strings.TrimSpace(currentID) != "" {
				return ErrWorkspaceConflict
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				if err := setJSON(ctx, pipe, fileLineageMetaKey(storageID, fileID), lineage); err != nil {
					return err
				}
				pipe.HSet(ctx, workspacePathFileIDsKey(storageID), normalizedPath, fileID)
				return nil
			})
			return err
		}, workspacePathFileIDsKey(storageID))
		if err == nil {
			return lineage, nil
		}
		if errors.Is(err, ErrWorkspaceConflict) {
			return FileLineage{}, err
		}
		if err != redis.TxFailedErr {
			return FileLineage{}, err
		}
	}

	return FileLineage{}, ErrWorkspaceConflict
}

func (s *Store) GetFileLineage(ctx context.Context, workspace, fileID string) (FileLineage, error) {
	_, storageID, err := s.resolveWorkspaceMeta(ctx, workspace)
	if err != nil {
		return FileLineage{}, err
	}
	return s.getFileLineageByStorageID(ctx, storageID, fileID)
}

func (s *Store) ResolveLiveFileLineageByPath(ctx context.Context, workspace, path string) (FileLineage, error) {
	_, storageID, err := s.resolveWorkspaceMeta(ctx, workspace)
	if err != nil {
		return FileLineage{}, err
	}
	normalizedPath, err := normalizeVersionedPath(path)
	if err != nil {
		return FileLineage{}, err
	}
	fileID, err := s.rdb.HGet(ctx, workspacePathFileIDsKey(storageID), normalizedPath).Result()
	if err == redis.Nil {
		return FileLineage{}, os.ErrNotExist
	}
	if err != nil {
		return FileLineage{}, err
	}
	lineage, err := s.getFileLineageByStorageID(ctx, storageID, fileID)
	if err != nil {
		return FileLineage{}, err
	}
	if lineage.State != FileLineageStateLive || lineage.CurrentPath != normalizedPath {
		return FileLineage{}, os.ErrNotExist
	}
	return lineage, nil
}

func (s *Store) RenameFileLineage(ctx context.Context, workspace, fileID, newPath string, updatedAt time.Time) (FileLineage, error) {
	_, storageID, err := s.resolveWorkspaceMeta(ctx, workspace)
	if err != nil {
		return FileLineage{}, err
	}
	normalizedPath, err := normalizeVersionedPath(newPath)
	if err != nil {
		return FileLineage{}, err
	}
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}

	for attempt := 0; attempt < 8; attempt++ {
		var updated FileLineage
		err = s.rdb.Watch(ctx, func(tx *redis.Tx) error {
			lineage, err := s.getFileLineageForCmd(ctx, tx, storageID, fileID)
			if err != nil {
				return err
			}
			if lineage.State != FileLineageStateLive {
				return os.ErrNotExist
			}
			if lineage.CurrentPath == normalizedPath {
				updated = lineage
				return nil
			}
			currentID, err := tx.HGet(ctx, workspacePathFileIDsKey(storageID), normalizedPath).Result()
			if err != nil && err != redis.Nil {
				return err
			}
			if err == nil && strings.TrimSpace(currentID) != "" && strings.TrimSpace(currentID) != fileID {
				return ErrWorkspaceConflict
			}
			oldPathID, err := tx.HGet(ctx, workspacePathFileIDsKey(storageID), lineage.CurrentPath).Result()
			if err != nil && err != redis.Nil {
				return err
			}
			if lineage.CurrentPath != "" && strings.TrimSpace(oldPathID) != fileID {
				return ErrWorkspaceConflict
			}
			updated = lineage
			updated.CurrentPath = normalizedPath
			updated.UpdatedAt = updatedAt
			oldPath := lineage.CurrentPath
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				if err := setJSON(ctx, pipe, fileLineageMetaKey(storageID, fileID), updated); err != nil {
					return err
				}
				if oldPath != "" {
					pipe.HDel(ctx, workspacePathFileIDsKey(storageID), oldPath)
				}
				pipe.HSet(ctx, workspacePathFileIDsKey(storageID), normalizedPath, fileID)
				return nil
			})
			return err
		}, fileLineageMetaKey(storageID, fileID), workspacePathFileIDsKey(storageID))
		if err == nil {
			return updated, nil
		}
		if errors.Is(err, ErrWorkspaceConflict) || errors.Is(err, os.ErrNotExist) {
			return FileLineage{}, err
		}
		if err != redis.TxFailedErr {
			return FileLineage{}, err
		}
	}

	return FileLineage{}, ErrWorkspaceConflict
}

func (s *Store) DeleteFileLineage(ctx context.Context, workspace, fileID string, deletedAt time.Time) (FileLineage, error) {
	_, storageID, err := s.resolveWorkspaceMeta(ctx, workspace)
	if err != nil {
		return FileLineage{}, err
	}
	if deletedAt.IsZero() {
		deletedAt = time.Now().UTC()
	}

	for attempt := 0; attempt < 8; attempt++ {
		var updated FileLineage
		err = s.rdb.Watch(ctx, func(tx *redis.Tx) error {
			lineage, err := s.getFileLineageForCmd(ctx, tx, storageID, fileID)
			if err != nil {
				return err
			}
			if lineage.State == FileLineageStateDeleted {
				updated = lineage
				return nil
			}
			updated = lineage
			updated.State = FileLineageStateDeleted
			updated.DeletedAt = deletedAt
			updated.UpdatedAt = deletedAt
			currentID, err := tx.HGet(ctx, workspacePathFileIDsKey(storageID), lineage.CurrentPath).Result()
			if err != nil && err != redis.Nil {
				return err
			}
			if lineage.CurrentPath != "" && strings.TrimSpace(currentID) != fileID {
				return ErrWorkspaceConflict
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				if err := setJSON(ctx, pipe, fileLineageMetaKey(storageID, fileID), updated); err != nil {
					return err
				}
				if lineage.CurrentPath != "" {
					pipe.HDel(ctx, workspacePathFileIDsKey(storageID), lineage.CurrentPath)
				}
				return nil
			})
			return err
		}, fileLineageMetaKey(storageID, fileID), workspacePathFileIDsKey(storageID))
		if err == nil {
			return updated, nil
		}
		if err != redis.TxFailedErr {
			return FileLineage{}, err
		}
	}

	return FileLineage{}, ErrWorkspaceConflict
}

func (s *Store) ReviveFileLineage(ctx context.Context, workspace, fileID, path string, revivedAt time.Time) (FileLineage, error) {
	_, storageID, err := s.resolveWorkspaceMeta(ctx, workspace)
	if err != nil {
		return FileLineage{}, err
	}
	normalizedPath, err := normalizeVersionedPath(path)
	if err != nil {
		return FileLineage{}, err
	}
	if revivedAt.IsZero() {
		revivedAt = time.Now().UTC()
	}

	for attempt := 0; attempt < 8; attempt++ {
		var updated FileLineage
		err = s.rdb.Watch(ctx, func(tx *redis.Tx) error {
			lineage, err := s.getFileLineageForCmd(ctx, tx, storageID, fileID)
			if err != nil {
				return err
			}
			if lineage.State == FileLineageStateLive && lineage.CurrentPath == normalizedPath {
				updated = lineage
				return nil
			}
			currentID, err := tx.HGet(ctx, workspacePathFileIDsKey(storageID), normalizedPath).Result()
			if err != nil && err != redis.Nil {
				return err
			}
			if err == nil && strings.TrimSpace(currentID) != "" && strings.TrimSpace(currentID) != fileID {
				return ErrWorkspaceConflict
			}
			updated = lineage
			updated.State = FileLineageStateLive
			updated.CurrentPath = normalizedPath
			updated.DeletedAt = time.Time{}
			updated.UpdatedAt = revivedAt
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				if err := setJSON(ctx, pipe, fileLineageMetaKey(storageID, fileID), updated); err != nil {
					return err
				}
				pipe.HSet(ctx, workspacePathFileIDsKey(storageID), normalizedPath, fileID)
				return nil
			})
			return err
		}, fileLineageMetaKey(storageID, fileID), workspacePathFileIDsKey(storageID))
		if err == nil {
			return updated, nil
		}
		if errors.Is(err, ErrWorkspaceConflict) {
			return FileLineage{}, err
		}
		if err != redis.TxFailedErr {
			return FileLineage{}, err
		}
	}

	return FileLineage{}, ErrWorkspaceConflict
}

func (s *Store) AppendFileVersion(ctx context.Context, workspace, fileID string, version FileVersion) (FileVersion, error) {
	_, storageID, err := s.resolveWorkspaceMeta(ctx, workspace)
	if err != nil {
		return FileVersion{}, err
	}
	policy, err := s.GetWorkspaceVersioningPolicy(ctx, workspace)
	if err != nil {
		return FileVersion{}, err
	}
	if strings.TrimSpace(fileID) == "" {
		return FileVersion{}, fmt.Errorf("file id is required")
	}
	if strings.TrimSpace(version.Op) == "" {
		return FileVersion{}, fmt.Errorf("file version op is required")
	}
	if strings.TrimSpace(version.Kind) == "" {
		version.Kind = FileVersionKindFile
	}
	version.FileID = strings.TrimSpace(fileID)
	version.Path, err = normalizeVersionedPath(version.Path)
	if err != nil {
		return FileVersion{}, err
	}
	if strings.TrimSpace(version.PrevPath) != "" {
		version.PrevPath, err = normalizeVersionedPath(version.PrevPath)
		if err != nil {
			return FileVersion{}, err
		}
	}
	version.CheckpointIDs = slices.Clone(version.CheckpointIDs)
	if version.CreatedAt.IsZero() {
		version.CreatedAt = time.Now().UTC()
	}
	if strings.TrimSpace(version.VersionID) == "" {
		version.VersionID, err = newFileVersionID()
		if err != nil {
			return FileVersion{}, err
		}
	}

	for attempt := 0; attempt < 8; attempt++ {
		var appended FileVersion
		err = s.rdb.Watch(ctx, func(tx *redis.Tx) error {
			lineage, err := s.getFileLineageForCmd(ctx, tx, storageID, fileID)
			if err != nil {
				return err
			}
			existingVersions, err := s.listFileVersionsForCmd(ctx, tx, storageID, fileID, true)
			if err != nil {
				return err
			}
			appended = version
			appended.Ordinal = lineage.HeadOrdinal + 1
			nextLineage := lineage
			nextLineage.HeadOrdinal = appended.Ordinal
			nextLineage.HeadVersionID = appended.VersionID
			nextLineage.UpdatedAt = appended.CreatedAt
			trimmedVersions := fileVersionsToTrim(policy, existingVersions, appended, time.Now().UTC())
			trimmedByID := make(map[string]FileVersion, len(trimmedVersions))
			for _, trimmed := range trimmedVersions {
				trimmedByID[trimmed.VersionID] = trimmed
			}
			workspaceTrimmed, err := s.workspaceBudgetTrimCandidatesForCmd(ctx, tx, storageID, policy, fileID, appended, trimmedByID)
			if err != nil {
				return err
			}
			for _, trimmed := range workspaceTrimmed {
				if _, ok := trimmedByID[trimmed.VersionID]; ok {
					continue
				}
				trimmedVersions = append(trimmedVersions, trimmed)
				trimmedByID[trimmed.VersionID] = trimmed
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				if err := setJSON(ctx, pipe, fileVersionKey(storageID, fileID, appended.VersionID), appended); err != nil {
					return err
				}
				if err := setJSON(ctx, pipe, fileLineageMetaKey(storageID, fileID), nextLineage); err != nil {
					return err
				}
				pipe.ZAdd(ctx, fileLineageVersionsKey(storageID, fileID), redis.Z{
					Score:  float64(appended.Ordinal),
					Member: appended.VersionID,
				})
				pipe.HSet(ctx, workspaceVersionFileIDsKey(storageID), appended.VersionID, fileID)
				pipe.RPush(ctx, workspacePathHistoryKey(storageID, appended.Path), appended.VersionID)
				if appended.PrevPath != "" && appended.PrevPath != appended.Path {
					pipe.RPush(ctx, workspacePathHistoryKey(storageID, appended.PrevPath), appended.VersionID)
				}
				pipe.ZAdd(ctx, workspaceVersionOrderKey(storageID), redis.Z{
					Score:  float64(appended.CreatedAt.UTC().UnixMilli()),
					Member: encodeWorkspaceVersionOrderMember(fileID, appended.VersionID),
				})
				if storedBytes := fileVersionStoredBytes(appended); storedBytes != 0 {
					pipe.IncrBy(ctx, workspaceVersionBytesKey(storageID), storedBytes)
				}
				applyTrimmedFileVersions(ctx, pipe, storageID, fileID, trimmedVersions)
				return nil
			})
			return err
		}, fileLineageMetaKey(storageID, fileID))
		if err == nil {
			return appended, nil
		}
		if err != redis.TxFailedErr {
			return FileVersion{}, err
		}
	}

	return FileVersion{}, ErrWorkspaceConflict
}

func (s *Store) RenameFileLineageWithVersion(ctx context.Context, workspace, fileID string, expected VersionedFileSnapshot, version FileVersion) (FileVersion, error) {
	_, storageID, err := s.resolveWorkspaceMeta(ctx, workspace)
	if err != nil {
		return FileVersion{}, err
	}
	policy, err := s.GetWorkspaceVersioningPolicy(ctx, workspace)
	if err != nil {
		return FileVersion{}, err
	}
	if strings.TrimSpace(fileID) == "" {
		return FileVersion{}, fmt.Errorf("file id is required")
	}
	expected, err = normalizeVersionedSnapshot(expected)
	if err != nil {
		return FileVersion{}, err
	}
	if strings.TrimSpace(version.Path) == "" || strings.TrimSpace(version.PrevPath) == "" {
		return FileVersion{}, fmt.Errorf("rename version requires path and prev_path")
	}
	version.FileID = strings.TrimSpace(fileID)
	version.Path, err = normalizeVersionedPath(version.Path)
	if err != nil {
		return FileVersion{}, err
	}
	version.PrevPath, err = normalizeVersionedPath(version.PrevPath)
	if err != nil {
		return FileVersion{}, err
	}
	if version.Path == version.PrevPath {
		return s.AppendFileVersion(ctx, workspace, fileID, version)
	}
	if strings.TrimSpace(version.Kind) == "" {
		version.Kind = FileVersionKindFile
	}
	if version.CreatedAt.IsZero() {
		version.CreatedAt = time.Now().UTC()
	}
	if strings.TrimSpace(version.VersionID) == "" {
		version.VersionID, err = newFileVersionID()
		if err != nil {
			return FileVersion{}, err
		}
	}

	for attempt := 0; attempt < 8; attempt++ {
		var appended FileVersion
		err = s.rdb.Watch(ctx, func(tx *redis.Tx) error {
			lineage, err := s.getFileLineageForCmd(ctx, tx, storageID, fileID)
			if err != nil {
				return err
			}
			existingVersions, err := s.listFileVersionsForCmd(ctx, tx, storageID, fileID, true)
			if err != nil {
				return err
			}
			if lineage.State != FileLineageStateLive {
				return os.ErrNotExist
			}
			if lineage.CurrentPath != version.PrevPath {
				return ErrWorkspaceConflict
			}
			if !expected.Exists || expected.Path != version.PrevPath {
				return ErrWorkspaceConflict
			}
			currentID, err := tx.HGet(ctx, workspacePathFileIDsKey(storageID), version.Path).Result()
			if err != nil && err != redis.Nil {
				return err
			}
			if err == nil && strings.TrimSpace(currentID) != "" && strings.TrimSpace(currentID) != fileID {
				return ErrWorkspaceConflict
			}
			oldPathID, err := tx.HGet(ctx, workspacePathFileIDsKey(storageID), version.PrevPath).Result()
			if err != nil && err != redis.Nil {
				return err
			}
			if strings.TrimSpace(oldPathID) != fileID {
				return ErrWorkspaceConflict
			}
			current, err := s.currentLineageSnapshotForCmd(ctx, tx, storageID, lineage)
			if err != nil {
				return err
			}
			if !versionedSnapshotsEquivalent(expected, current) {
				return ErrWorkspaceConflict
			}

			appended = version
			appended.Ordinal = lineage.HeadOrdinal + 1
			nextLineage := lineage
			nextLineage.CurrentPath = version.Path
			nextLineage.HeadOrdinal = appended.Ordinal
			nextLineage.HeadVersionID = appended.VersionID
			nextLineage.UpdatedAt = appended.CreatedAt
			trimmedVersions := fileVersionsToTrim(policy, existingVersions, appended, time.Now().UTC())
			trimmedByID := make(map[string]FileVersion, len(trimmedVersions))
			for _, trimmed := range trimmedVersions {
				trimmedByID[trimmed.VersionID] = trimmed
			}
			workspaceTrimmed, err := s.workspaceBudgetTrimCandidatesForCmd(ctx, tx, storageID, policy, fileID, appended, trimmedByID)
			if err != nil {
				return err
			}
			for _, trimmed := range workspaceTrimmed {
				if _, ok := trimmedByID[trimmed.VersionID]; ok {
					continue
				}
				trimmedVersions = append(trimmedVersions, trimmed)
				trimmedByID[trimmed.VersionID] = trimmed
			}

			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				if err := setJSON(ctx, pipe, fileVersionKey(storageID, fileID, appended.VersionID), appended); err != nil {
					return err
				}
				if err := setJSON(ctx, pipe, fileLineageMetaKey(storageID, fileID), nextLineage); err != nil {
					return err
				}
				pipe.ZAdd(ctx, fileLineageVersionsKey(storageID, fileID), redis.Z{
					Score:  float64(appended.Ordinal),
					Member: appended.VersionID,
				})
				pipe.HSet(ctx, workspaceVersionFileIDsKey(storageID), appended.VersionID, fileID)
				pipe.RPush(ctx, workspacePathHistoryKey(storageID, appended.Path), appended.VersionID)
				pipe.RPush(ctx, workspacePathHistoryKey(storageID, appended.PrevPath), appended.VersionID)
				pipe.HDel(ctx, workspacePathFileIDsKey(storageID), appended.PrevPath)
				pipe.HSet(ctx, workspacePathFileIDsKey(storageID), appended.Path, fileID)
				pipe.ZAdd(ctx, workspaceVersionOrderKey(storageID), redis.Z{
					Score:  float64(appended.CreatedAt.UTC().UnixMilli()),
					Member: encodeWorkspaceVersionOrderMember(fileID, appended.VersionID),
				})
				if storedBytes := fileVersionStoredBytes(appended); storedBytes != 0 {
					pipe.IncrBy(ctx, workspaceVersionBytesKey(storageID), storedBytes)
				}
				applyTrimmedFileVersions(ctx, pipe, storageID, fileID, trimmedVersions)
				return nil
			})
			return err
		}, fileLineageMetaKey(storageID, fileID), workspacePathFileIDsKey(storageID))
		if err == nil {
			return appended, nil
		}
		if errors.Is(err, ErrWorkspaceConflict) || errors.Is(err, os.ErrNotExist) {
			return FileVersion{}, err
		}
		if err != redis.TxFailedErr {
			return FileVersion{}, err
		}
	}

	return FileVersion{}, ErrWorkspaceConflict
}

func (s *Store) AppendFileVersionChecked(ctx context.Context, workspace, fileID string, expected VersionedFileSnapshot, version FileVersion, deleteAfter bool) (FileVersion, error) {
	_, storageID, err := s.resolveWorkspaceMeta(ctx, workspace)
	if err != nil {
		return FileVersion{}, err
	}
	policy, err := s.GetWorkspaceVersioningPolicy(ctx, workspace)
	if err != nil {
		return FileVersion{}, err
	}
	if strings.TrimSpace(fileID) == "" {
		return FileVersion{}, fmt.Errorf("file id is required")
	}
	if strings.TrimSpace(version.Op) == "" {
		return FileVersion{}, fmt.Errorf("file version op is required")
	}
	expected, err = normalizeVersionedSnapshot(expected)
	if err != nil {
		return FileVersion{}, err
	}
	if strings.TrimSpace(version.Kind) == "" {
		version.Kind = FileVersionKindFile
	}
	version.FileID = strings.TrimSpace(fileID)
	version.Path, err = normalizeVersionedPath(version.Path)
	if err != nil {
		return FileVersion{}, err
	}
	if strings.TrimSpace(version.PrevPath) != "" {
		version.PrevPath, err = normalizeVersionedPath(version.PrevPath)
		if err != nil {
			return FileVersion{}, err
		}
	}
	version.CheckpointIDs = slices.Clone(version.CheckpointIDs)
	if version.CreatedAt.IsZero() {
		version.CreatedAt = time.Now().UTC()
	}
	if strings.TrimSpace(version.VersionID) == "" {
		version.VersionID, err = newFileVersionID()
		if err != nil {
			return FileVersion{}, err
		}
	}

	for attempt := 0; attempt < 8; attempt++ {
		var appended FileVersion
		err = s.rdb.Watch(ctx, func(tx *redis.Tx) error {
			lineage, err := s.getFileLineageForCmd(ctx, tx, storageID, fileID)
			if err != nil {
				return err
			}
			existingVersions, err := s.listFileVersionsForCmd(ctx, tx, storageID, fileID, true)
			if err != nil {
				return err
			}
			if lineage.State != FileLineageStateLive {
				return ErrWorkspaceConflict
			}
			if !expected.Exists || lineage.CurrentPath != expected.Path {
				return ErrWorkspaceConflict
			}
			current, err := s.currentLineageSnapshotForCmd(ctx, tx, storageID, lineage)
			if err != nil {
				return err
			}
			if !versionedSnapshotsEquivalent(expected, current) {
				return ErrWorkspaceConflict
			}

			appended = version
			appended.Ordinal = lineage.HeadOrdinal + 1
			nextLineage := lineage
			nextLineage.HeadOrdinal = appended.Ordinal
			nextLineage.HeadVersionID = appended.VersionID
			nextLineage.UpdatedAt = appended.CreatedAt
			if deleteAfter {
				nextLineage.State = FileLineageStateDeleted
				nextLineage.DeletedAt = appended.CreatedAt
			}
			trimmedVersions := fileVersionsToTrim(policy, existingVersions, appended, time.Now().UTC())
			trimmedByID := make(map[string]FileVersion, len(trimmedVersions))
			for _, trimmed := range trimmedVersions {
				trimmedByID[trimmed.VersionID] = trimmed
			}
			workspaceTrimmed, err := s.workspaceBudgetTrimCandidatesForCmd(ctx, tx, storageID, policy, fileID, appended, trimmedByID)
			if err != nil {
				return err
			}
			for _, trimmed := range workspaceTrimmed {
				if _, ok := trimmedByID[trimmed.VersionID]; ok {
					continue
				}
				trimmedVersions = append(trimmedVersions, trimmed)
				trimmedByID[trimmed.VersionID] = trimmed
			}

			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				if err := setJSON(ctx, pipe, fileVersionKey(storageID, fileID, appended.VersionID), appended); err != nil {
					return err
				}
				if err := setJSON(ctx, pipe, fileLineageMetaKey(storageID, fileID), nextLineage); err != nil {
					return err
				}
				pipe.ZAdd(ctx, fileLineageVersionsKey(storageID, fileID), redis.Z{
					Score:  float64(appended.Ordinal),
					Member: appended.VersionID,
				})
				pipe.HSet(ctx, workspaceVersionFileIDsKey(storageID), appended.VersionID, fileID)
				pipe.RPush(ctx, workspacePathHistoryKey(storageID, appended.Path), appended.VersionID)
				if appended.PrevPath != "" && appended.PrevPath != appended.Path {
					pipe.RPush(ctx, workspacePathHistoryKey(storageID, appended.PrevPath), appended.VersionID)
				}
				pipe.ZAdd(ctx, workspaceVersionOrderKey(storageID), redis.Z{
					Score:  float64(appended.CreatedAt.UTC().UnixMilli()),
					Member: encodeWorkspaceVersionOrderMember(fileID, appended.VersionID),
				})
				if storedBytes := fileVersionStoredBytes(appended); storedBytes != 0 {
					pipe.IncrBy(ctx, workspaceVersionBytesKey(storageID), storedBytes)
				}
				applyTrimmedFileVersions(ctx, pipe, storageID, fileID, trimmedVersions)
				if deleteAfter {
					pipe.HDel(ctx, workspacePathFileIDsKey(storageID), lineage.CurrentPath)
				}
				return nil
			})
			return err
		}, fileLineageMetaKey(storageID, fileID), workspacePathFileIDsKey(storageID))
		if err == nil {
			return appended, nil
		}
		if errors.Is(err, ErrWorkspaceConflict) {
			return FileVersion{}, err
		}
		if err != redis.TxFailedErr {
			return FileVersion{}, err
		}
	}

	return FileVersion{}, ErrWorkspaceConflict
}

func (s *Store) currentLineageSnapshotForCmd(ctx context.Context, cmd redis.Cmdable, storageID string, lineage FileLineage) (VersionedFileSnapshot, error) {
	if lineage.State != FileLineageStateLive || strings.TrimSpace(lineage.HeadVersionID) == "" {
		return VersionedFileSnapshot{}, ErrWorkspaceConflict
	}
	currentVersion, err := getJSON[FileVersion](ctx, cmd, fileVersionKey(storageID, lineage.FileID, lineage.HeadVersionID))
	if err != nil {
		return VersionedFileSnapshot{}, err
	}
	snapshot := VersionedFileSnapshot{
		Path:        lineage.CurrentPath,
		Exists:      true,
		Mode:        currentVersion.Mode,
		BlobID:      currentVersion.BlobID,
		ContentHash: currentVersion.ContentHash,
		SizeBytes:   currentVersion.SizeBytes,
	}
	switch currentVersion.Kind {
	case FileVersionKindSymlink:
		snapshot.Kind = "symlink"
		snapshot.Target = currentVersion.Target
	case FileVersionKindTombstone:
		return VersionedFileSnapshot{}, ErrWorkspaceConflict
	default:
		snapshot.Kind = "file"
	}
	return completeVersionedSnapshot(snapshot), nil
}

func versionedSnapshotsEquivalent(left, right VersionedFileSnapshot) bool {
	if left.Exists != right.Exists || left.Kind != right.Kind || left.Mode != right.Mode || left.Path != right.Path {
		return false
	}
	switch left.Kind {
	case "symlink":
		return left.Target == right.Target
	case "file":
		if left.ContentHash != "" && right.ContentHash != "" {
			return left.ContentHash == right.ContentHash
		}
		return bytes.Equal(left.Content, right.Content)
	default:
		return true
	}
}

func (s *Store) GetFileVersion(ctx context.Context, workspace, versionID string) (FileVersion, error) {
	_, storageID, err := s.resolveWorkspaceMeta(ctx, workspace)
	if err != nil {
		return FileVersion{}, err
	}
	fileID, err := s.rdb.HGet(ctx, workspaceVersionFileIDsKey(storageID), strings.TrimSpace(versionID)).Result()
	if err == redis.Nil {
		return FileVersion{}, os.ErrNotExist
	}
	if err != nil {
		return FileVersion{}, err
	}
	return getJSON[FileVersion](ctx, s.rdb, fileVersionKey(storageID, fileID, versionID))
}

func (s *Store) GetFileVersionAtOrdinal(ctx context.Context, workspace, fileID string, ordinal int64) (FileVersion, error) {
	_, storageID, err := s.resolveWorkspaceMeta(ctx, workspace)
	if err != nil {
		return FileVersion{}, err
	}
	if strings.TrimSpace(fileID) == "" {
		return FileVersion{}, fmt.Errorf("file id is required")
	}
	ids, err := s.rdb.ZRangeByScore(ctx, fileLineageVersionsKey(storageID, strings.TrimSpace(fileID)), &redis.ZRangeBy{
		Min: strconv.FormatInt(ordinal, 10),
		Max: strconv.FormatInt(ordinal, 10),
	}).Result()
	if err != nil {
		return FileVersion{}, err
	}
	if len(ids) == 0 {
		return FileVersion{}, os.ErrNotExist
	}
	return getJSON[FileVersion](ctx, s.rdb, fileVersionKey(storageID, strings.TrimSpace(fileID), ids[0]))
}

func (s *Store) ListFileVersions(ctx context.Context, workspace, fileID string, ascending bool) ([]FileVersion, error) {
	_, storageID, err := s.resolveWorkspaceMeta(ctx, workspace)
	if err != nil {
		return nil, err
	}
	var ids []string
	if ascending {
		ids, err = s.rdb.ZRange(ctx, fileLineageVersionsKey(storageID, fileID), 0, -1).Result()
	} else {
		ids, err = s.rdb.ZRevRange(ctx, fileLineageVersionsKey(storageID, fileID), 0, -1).Result()
	}
	if err != nil {
		return nil, err
	}
	versions := make([]FileVersion, 0, len(ids))
	for _, versionID := range ids {
		version, err := getJSON[FileVersion](ctx, s.rdb, fileVersionKey(storageID, fileID, versionID))
		if err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, nil
}

func (s *Store) ListPathHistoryVersionIDs(ctx context.Context, workspace, path string) ([]string, error) {
	_, storageID, err := s.resolveWorkspaceMeta(ctx, workspace)
	if err != nil {
		return nil, err
	}
	normalizedPath, err := normalizeVersionedPath(path)
	if err != nil {
		return nil, err
	}
	return s.rdb.LRange(ctx, workspacePathHistoryKey(storageID, normalizedPath), 0, -1).Result()
}

func (s *Store) CloneFileVersionHistory(ctx context.Context, sourceStorageID, destStorageID string) error {
	sourceStorageID = strings.TrimSpace(sourceStorageID)
	destStorageID = strings.TrimSpace(destStorageID)
	if sourceStorageID == "" || destStorageID == "" {
		return fmt.Errorf("source and destination workspace ids are required")
	}

	if policy, err := getJSON[WorkspaceVersioningPolicy](ctx, s.rdb, workspaceVersioningPolicyKey(sourceStorageID)); err == nil {
		if err := setJSON(ctx, s.rdb, workspaceVersioningPolicyKey(destStorageID), policy); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	pathFileIDs, err := s.rdb.HGetAll(ctx, workspacePathFileIDsKey(sourceStorageID)).Result()
	if err != nil {
		return err
	}
	if len(pathFileIDs) > 0 {
		if err := s.rdb.HSet(ctx, workspacePathFileIDsKey(destStorageID), pathFileIDs).Err(); err != nil {
			return err
		}
	}

	versionFileIDs, err := s.rdb.HGetAll(ctx, workspaceVersionFileIDsKey(sourceStorageID)).Result()
	if err != nil {
		return err
	}
	if len(versionFileIDs) == 0 {
		return nil
	}
	if err := s.rdb.HSet(ctx, workspaceVersionFileIDsKey(destStorageID), versionFileIDs).Err(); err != nil {
		return err
	}

	fileIDs := make([]string, 0)
	seenFiles := make(map[string]struct{}, len(versionFileIDs))
	paths := make([]string, 0)
	seenPaths := make(map[string]struct{})
	blobs := make(map[string][]byte)

	for versionID, fileID := range versionFileIDs {
		if _, ok := seenFiles[fileID]; !ok {
			seenFiles[fileID] = struct{}{}
			fileIDs = append(fileIDs, fileID)
		}
		version, err := getJSON[FileVersion](ctx, s.rdb, fileVersionKey(sourceStorageID, fileID, versionID))
		if err != nil {
			return err
		}
		if err := setJSON(ctx, s.rdb, fileVersionKey(destStorageID, fileID, versionID), version); err != nil {
			return err
		}
		if blobID := strings.TrimSpace(version.BlobID); blobID != "" {
			if _, ok := blobs[blobID]; !ok {
				data, err := s.GetBlob(ctx, sourceStorageID, blobID)
				if err != nil {
					return err
				}
				blobs[blobID] = data
			}
		}
		for _, candidate := range []string{version.Path, version.PrevPath} {
			candidate = strings.TrimSpace(candidate)
			if candidate == "" {
				continue
			}
			if _, ok := seenPaths[candidate]; ok {
				continue
			}
			seenPaths[candidate] = struct{}{}
			paths = append(paths, candidate)
		}
	}
	if len(blobs) > 0 {
		if err := s.SaveBlobs(ctx, destStorageID, blobs); err != nil {
			return err
		}
	}

	for _, fileID := range fileIDs {
		lineage, err := s.getFileLineageByStorageID(ctx, sourceStorageID, fileID)
		if err != nil {
			return err
		}
		lineage.WorkspaceID = destStorageID
		if err := setJSON(ctx, s.rdb, fileLineageMetaKey(destStorageID, fileID), lineage); err != nil {
			return err
		}
		zs, err := s.rdb.ZRangeWithScores(ctx, fileLineageVersionsKey(sourceStorageID, fileID), 0, -1).Result()
		if err != nil {
			return err
		}
		if len(zs) > 0 {
			if err := s.rdb.ZAdd(ctx, fileLineageVersionsKey(destStorageID, fileID), zs...).Err(); err != nil {
				return err
			}
		}
	}

	for _, historyPath := range paths {
		ids, err := s.rdb.LRange(ctx, workspacePathHistoryKey(sourceStorageID, historyPath), 0, -1).Result()
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			continue
		}
		values := make([]any, 0, len(ids))
		for _, id := range ids {
			values = append(values, id)
		}
		if err := s.rdb.RPush(ctx, workspacePathHistoryKey(destStorageID, historyPath), values...).Err(); err != nil {
			return err
		}
	}
	if order, err := s.rdb.ZRangeWithScores(ctx, workspaceVersionOrderKey(sourceStorageID), 0, -1).Result(); err == nil {
		if len(order) > 0 {
			if err := s.rdb.ZAdd(ctx, workspaceVersionOrderKey(destStorageID), order...).Err(); err != nil {
				return err
			}
		}
	} else {
		return err
	}
	if bytesValue, err := s.rdb.Get(ctx, workspaceVersionBytesKey(sourceStorageID)).Result(); err == nil {
		if err := s.rdb.Set(ctx, workspaceVersionBytesKey(destStorageID), bytesValue, 0).Err(); err != nil {
			return err
		}
	} else if err != redis.Nil {
		return err
	}

	return nil
}

func (s *Store) getFileLineageByStorageID(ctx context.Context, storageID, fileID string) (FileLineage, error) {
	return getJSON[FileLineage](ctx, s.rdb, fileLineageMetaKey(storageID, strings.TrimSpace(fileID)))
}

func (s *Store) getFileLineageForCmd(ctx context.Context, cmd redis.Cmdable, storageID, fileID string) (FileLineage, error) {
	return getJSON[FileLineage](ctx, cmd, fileLineageMetaKey(storageID, strings.TrimSpace(fileID)))
}

func WorkspacePathFileIDsKey(workspace string) string {
	return workspacePathFileIDsKey(workspace)
}

func WorkspacePathHistoryKey(workspace, normalizedPath string) string {
	return workspacePathHistoryKey(workspace, normalizedPath)
}

func WorkspaceVersionFileIDsKey(workspace string) string {
	return workspaceVersionFileIDsKey(workspace)
}

func FileLineageMetaKey(workspace, fileID string) string {
	return fileLineageMetaKey(workspace, fileID)
}

func FileLineageVersionsKey(workspace, fileID string) string {
	return fileLineageVersionsKey(workspace, fileID)
}

func FileVersionKey(workspace, fileID, versionID string) string {
	return fileVersionKey(workspace, fileID, versionID)
}

func workspacePathFileIDsKey(workspace string) string {
	return fmt.Sprintf("afs:{%s}:workspace:path_file_ids", workspace)
}

func workspacePathHistoryKey(workspace, normalizedPath string) string {
	return fmt.Sprintf("afs:{%s}:workspace:path_history:%s", workspace, normalizedPath)
}

func workspaceVersionFileIDsKey(workspace string) string {
	return fmt.Sprintf("afs:{%s}:workspace:version_file_ids", workspace)
}

func workspaceVersionOrderKey(workspace string) string {
	return fmt.Sprintf("afs:{%s}:workspace:version_order", workspace)
}

func workspaceVersionBytesKey(workspace string) string {
	return fmt.Sprintf("afs:{%s}:workspace:version_bytes", workspace)
}

func fileLineageMetaKey(workspace, fileID string) string {
	return fmt.Sprintf("afs:{%s}:file:%s:meta", workspace, fileID)
}

func fileLineageVersionsKey(workspace, fileID string) string {
	return fmt.Sprintf("afs:{%s}:file:%s:versions", workspace, fileID)
}

func fileVersionKey(workspace, fileID, versionID string) string {
	return fmt.Sprintf("afs:{%s}:file:%s:version:%s", workspace, fileID, versionID)
}

func normalizeVersionedPath(raw string) (string, error) {
	normalized, err := normalizeManifestPath(raw)
	if err != nil {
		return "", err
	}
	if normalized == "/" {
		return "", fmt.Errorf("file history path must reference a file path, got %q", raw)
	}
	return normalized, nil
}

func newFileLineageID() (string, error) {
	return newVersioningObjectID("file_")
}

func newFileVersionID() (string, error) {
	return newVersioningObjectID("ver_")
}

func newVersioningObjectID(prefix string) (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(raw), nil
}
