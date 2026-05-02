package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

func retentionEnabled(policy WorkspaceVersioningPolicy) bool {
	return policy.MaxVersionsPerFile > 0 || policy.MaxAgeDays > 0 || policy.MaxTotalBytes > 0
}

func (s *Store) listFileVersionsForCmd(ctx context.Context, cmd redis.Cmdable, workspace, fileID string, ascending bool) ([]FileVersion, error) {
	var ids []string
	var err error
	if ascending {
		ids, err = cmd.ZRange(ctx, fileLineageVersionsKey(workspace, fileID), 0, -1).Result()
	} else {
		ids, err = cmd.ZRevRange(ctx, fileLineageVersionsKey(workspace, fileID), 0, -1).Result()
	}
	if err != nil {
		return nil, err
	}
	versions := make([]FileVersion, 0, len(ids))
	for _, versionID := range ids {
		version, err := getJSON[FileVersion](ctx, cmd, fileVersionKey(workspace, fileID, versionID))
		if err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, nil
}

func fileVersionsToTrim(policy WorkspaceVersioningPolicy, existing []FileVersion, appended FileVersion, now time.Time) []FileVersion {
	if policy.MaxVersionsPerFile <= 0 && policy.MaxAgeDays <= 0 {
		return nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	all := make([]FileVersion, 0, len(existing)+1)
	all = append(all, existing...)
	all = append(all, appended)
	if len(all) <= 1 {
		return nil
	}

	headVersionID := appended.VersionID
	remove := make(map[string]FileVersion)

	if policy.MaxAgeDays > 0 {
		cutoff := now.AddDate(0, 0, -policy.MaxAgeDays)
		for _, version := range all {
			if version.VersionID == headVersionID {
				continue
			}
			if version.CreatedAt.Before(cutoff) {
				remove[version.VersionID] = version
			}
		}
	}

	if policy.MaxVersionsPerFile > 0 && len(all)-len(remove) > policy.MaxVersionsPerFile {
		remaining := len(all) - len(remove)
		for _, version := range all {
			if remaining <= policy.MaxVersionsPerFile {
				break
			}
			if version.VersionID == headVersionID {
				continue
			}
			if _, already := remove[version.VersionID]; already {
				continue
			}
			remove[version.VersionID] = version
			remaining--
		}
	}

	if len(remove) >= len(all) {
		delete(remove, headVersionID)
	}

	trimmed := make([]FileVersion, 0, len(remove))
	for _, version := range all {
		if doomed, ok := remove[version.VersionID]; ok {
			trimmed = append(trimmed, doomed)
		}
	}
	return trimmed
}

func applyTrimmedFileVersions(ctx context.Context, pipe redis.Pipeliner, workspace, fileID string, trimmed []FileVersion) {
	for _, version := range trimmed {
		pipe.Del(ctx, fileVersionKey(workspace, fileID, version.VersionID))
		pipe.HDel(ctx, workspaceVersionFileIDsKey(workspace), version.VersionID)
		pipe.ZRem(ctx, fileLineageVersionsKey(workspace, fileID), version.VersionID)
		pipe.LRem(ctx, workspacePathHistoryKey(workspace, version.Path), 0, version.VersionID)
		if version.PrevPath != "" && version.PrevPath != version.Path {
			pipe.LRem(ctx, workspacePathHistoryKey(workspace, version.PrevPath), 0, version.VersionID)
		}
		pipe.ZRem(ctx, workspaceVersionOrderKey(workspace), encodeWorkspaceVersionOrderMember(fileID, version.VersionID))
		if storedBytes := fileVersionStoredBytes(version); storedBytes != 0 {
			pipe.DecrBy(ctx, workspaceVersionBytesKey(workspace), storedBytes)
		}
	}
}

func fileVersionStoredBytes(version FileVersion) int64 {
	if version.Kind != FileVersionKindFile || strings.TrimSpace(version.BlobID) == "" {
		return 0
	}
	return version.SizeBytes
}

func applyLargeFileGuardrail(policy WorkspaceVersioningPolicy, snapshot VersionedFileSnapshot) VersionedFileSnapshot {
	if !snapshot.Exists || snapshot.Kind != "file" || policy.LargeFileCutoffBytes <= 0 {
		return snapshot
	}
	if snapshot.SizeBytes <= policy.LargeFileCutoffBytes {
		return snapshot
	}
	snapshot.Content = nil
	snapshot.BlobID = ""
	return snapshot
}

type workspaceVersionOrderMember struct {
	FileID    string `json:"file_id"`
	VersionID string `json:"version_id"`
}

func encodeWorkspaceVersionOrderMember(fileID, versionID string) string {
	payload, _ := json.Marshal(workspaceVersionOrderMember{
		FileID:    strings.TrimSpace(fileID),
		VersionID: strings.TrimSpace(versionID),
	})
	return string(payload)
}

func decodeWorkspaceVersionOrderMember(raw string) (workspaceVersionOrderMember, error) {
	var member workspaceVersionOrderMember
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &member); err != nil {
		return workspaceVersionOrderMember{}, fmt.Errorf("invalid workspace version order member: %w", err)
	}
	member.FileID = strings.TrimSpace(member.FileID)
	member.VersionID = strings.TrimSpace(member.VersionID)
	if member.FileID == "" || member.VersionID == "" {
		return workspaceVersionOrderMember{}, fmt.Errorf("invalid workspace version order member")
	}
	return member, nil
}

func (s *Store) workspaceVersionBytesForCmd(ctx context.Context, cmd redis.Cmdable, workspace string) (int64, error) {
	value, err := cmd.Get(ctx, workspaceVersionBytesKey(workspace)).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	return value, err
}

func (s *Store) workspaceBudgetTrimCandidatesForCmd(
	ctx context.Context,
	cmd redis.Cmdable,
	workspace string,
	policy WorkspaceVersioningPolicy,
	currentFileID string,
	appended FileVersion,
	alreadyTrimmed map[string]FileVersion,
) ([]FileVersion, error) {
	if policy.MaxTotalBytes <= 0 {
		return nil, nil
	}
	currentBytes, err := s.workspaceVersionBytesForCmd(ctx, cmd, workspace)
	if err != nil {
		return nil, err
	}
	for _, version := range alreadyTrimmed {
		currentBytes -= fileVersionStoredBytes(version)
	}
	currentBytes += fileVersionStoredBytes(appended)
	if currentBytes <= policy.MaxTotalBytes {
		return nil, nil
	}

	orderMembers, err := cmd.ZRange(ctx, workspaceVersionOrderKey(workspace), 0, -1).Result()
	if err != nil {
		return nil, err
	}

	trimmed := make([]FileVersion, 0)
	for _, rawMember := range orderMembers {
		if currentBytes <= policy.MaxTotalBytes {
			break
		}
		member, err := decodeWorkspaceVersionOrderMember(rawMember)
		if err != nil {
			return nil, err
		}
		if _, ok := alreadyTrimmed[member.VersionID]; ok {
			continue
		}
		if member.FileID == currentFileID && member.VersionID == appended.VersionID {
			continue
		}
		version, err := getJSON[FileVersion](ctx, cmd, fileVersionKey(workspace, member.FileID, member.VersionID))
		if err != nil {
			return nil, err
		}
		storedBytes := fileVersionStoredBytes(version)
		if storedBytes == 0 {
			continue
		}

		lineage, err := s.getFileLineageForCmd(ctx, cmd, workspace, member.FileID)
		if err != nil {
			return nil, err
		}
		headVersionID := strings.TrimSpace(lineage.HeadVersionID)
		if member.FileID == currentFileID {
			headVersionID = appended.VersionID
		}
		if member.VersionID == headVersionID {
			continue
		}

		trimmed = append(trimmed, version)
		currentBytes -= storedBytes
	}

	return trimmed, nil
}
