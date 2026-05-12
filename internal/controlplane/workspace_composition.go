package controlplane

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const workspaceCompositionVersion = 1

type workspaceCompositionMount struct {
	VolumeID      string `json:"volume_id"`
	VolumeName    string `json:"volume_name,omitempty"`
	MountPath     string `json:"mount_path"`
	Readonly      bool   `json:"readonly"`
	VolumeTokenID string `json:"volume_token_id,omitempty"`
}

type workspaceComposition struct {
	Version        int                         `json:"version"`
	ID             string                      `json:"id"`
	Name           string                      `json:"name"`
	Description    string                      `json:"description,omitempty"`
	DatabaseID     string                      `json:"database_id,omitempty"`
	DatabaseName   string                      `json:"database_name,omitempty"`
	CloudAccount   string                      `json:"cloud_account,omitempty"`
	OwnerSubject   string                      `json:"owner_subject,omitempty"`
	OwnerLabel     string                      `json:"owner_label,omitempty"`
	Mounts         []workspaceCompositionMount `json:"mounts"`
	CreatedAt      time.Time                   `json:"created_at"`
	UpdatedAt      time.Time                   `json:"updated_at"`
	LastActivityAt time.Time                   `json:"last_activity_at,omitempty"`
}

type workspaceCompositionListResponse struct {
	Items []workspaceCompositionSummary `json:"items"`
}

type workspaceCompositionSummary struct {
	ID                  string                            `json:"id"`
	Name                string                            `json:"name"`
	Description         string                            `json:"description,omitempty"`
	DatabaseID          string                            `json:"database_id,omitempty"`
	DatabaseName        string                            `json:"database_name,omitempty"`
	CloudAccount        string                            `json:"cloud_account,omitempty"`
	OwnerSubject        string                            `json:"owner_subject,omitempty"`
	OwnerLabel          string                            `json:"owner_label,omitempty"`
	MountCount          int                               `json:"mount_count"`
	MountedVolumes      []workspaceCompositionVolumeLabel `json:"mounted_volumes"`
	ConnectedAgentCount int                               `json:"connected_agent_count"`
	LastActivityAt      string                            `json:"last_activity_at,omitempty"`
	UpdatedAt           string                            `json:"updated_at"`
}

type workspaceCompositionVolumeLabel struct {
	ID        string `json:"id"`
	Name      string `json:"name,omitempty"`
	MountPath string `json:"mount_path"`
	Readonly  bool   `json:"readonly"`
}

type workspaceCompositionDetail struct {
	ID                  string                      `json:"id"`
	Name                string                      `json:"name"`
	Description         string                      `json:"description,omitempty"`
	DatabaseID          string                      `json:"database_id,omitempty"`
	DatabaseName        string                      `json:"database_name,omitempty"`
	CloudAccount        string                      `json:"cloud_account,omitempty"`
	OwnerSubject        string                      `json:"owner_subject,omitempty"`
	OwnerLabel          string                      `json:"owner_label,omitempty"`
	Mounts              []workspaceCompositionMount `json:"mounts"`
	Bookmarks           []workspaceBookmark         `json:"bookmarks"`
	ConnectedAgentCount int                         `json:"connected_agent_count"`
	CreatedAt           string                      `json:"created_at"`
	UpdatedAt           string                      `json:"updated_at"`
	LastActivityAt      string                      `json:"last_activity_at,omitempty"`
}

type createWorkspaceCompositionRequest struct {
	Name         string                      `json:"name"`
	Description  string                      `json:"description,omitempty"`
	DatabaseID   string                      `json:"database_id,omitempty"`
	DatabaseName string                      `json:"database_name,omitempty"`
	CloudAccount string                      `json:"cloud_account,omitempty"`
	Mounts       []workspaceCompositionMount `json:"mounts,omitempty"`
}

type updateWorkspaceCompositionRequest struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

type replaceWorkspaceCompositionMountsRequest struct {
	Mounts []workspaceCompositionMount `json:"mounts"`
}

type workspaceBookmarkVolume struct {
	VolumeID     string `json:"volume_id"`
	VolumeName   string `json:"volume_name,omitempty"`
	CheckpointID string `json:"checkpoint_id"`
}

type workspaceBookmark struct {
	WorkspaceID string                    `json:"workspace_id"`
	Name        string                    `json:"name"`
	Description string                    `json:"description,omitempty"`
	Volumes     []workspaceBookmarkVolume `json:"volumes"`
	CreatedAt   time.Time                 `json:"created_at"`
}

type workspaceBookmarkListResponse struct {
	Items []workspaceBookmark `json:"items"`
}

type createWorkspaceBookmarkRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type WorkspaceCompositionMount = workspaceCompositionMount
type WorkspaceCompositionListResponse = workspaceCompositionListResponse
type WorkspaceCompositionSummary = workspaceCompositionSummary
type WorkspaceCompositionDetail = workspaceCompositionDetail
type WorkspaceBookmark = workspaceBookmark
type WorkspaceBookmarkListResponse = workspaceBookmarkListResponse
type CreateWorkspaceCompositionRequest = createWorkspaceCompositionRequest
type UpdateWorkspaceCompositionRequest = updateWorkspaceCompositionRequest
type CreateWorkspaceBookmarkRequest = createWorkspaceBookmarkRequest

func newWorkspaceCompositionID() (string, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return "ws_" + hex.EncodeToString(raw[:]), nil
}

func workspaceCompositionNameIndexKey() string {
	return "afs:workspace-composition:index:names"
}

func workspaceCompositionMetaKey(workspaceID string) string {
	return fmt.Sprintf("afs:{%s}:workspace:composition:meta", workspaceID)
}

func workspaceCompositionBookmarkIndexKey(workspaceID string) string {
	return fmt.Sprintf("afs:{%s}:workspace:composition:bookmarks", workspaceID)
}

func workspaceCompositionBookmarkKey(workspaceID, name string) string {
	return fmt.Sprintf("afs:{%s}:workspace:composition:bookmark:%s", workspaceID, name)
}

func workspaceCompositionPattern(workspaceID string) string {
	return fmt.Sprintf("afs:{%s}:workspace:composition:*", workspaceID)
}

func (s *Store) resolveWorkspaceComposition(ctx context.Context, workspace string) (workspaceComposition, string, error) {
	ref := strings.TrimSpace(workspace)
	if ref == "" {
		return workspaceComposition{}, "", os.ErrNotExist
	}
	meta, err := getJSON[workspaceComposition](ctx, s.rdb, workspaceCompositionMetaKey(ref))
	if err == nil {
		return meta, meta.ID, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return workspaceComposition{}, "", err
	}
	id, err := s.rdb.HGet(ctx, workspaceCompositionNameIndexKey(), ref).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return workspaceComposition{}, "", os.ErrNotExist
		}
		return workspaceComposition{}, "", err
	}
	meta, err = getJSON[workspaceComposition](ctx, s.rdb, workspaceCompositionMetaKey(id))
	if err != nil {
		return workspaceComposition{}, "", err
	}
	return meta, meta.ID, nil
}

func (s *Store) PutWorkspaceComposition(ctx context.Context, item workspaceComposition) error {
	id := strings.TrimSpace(item.ID)
	if id == "" {
		return fmt.Errorf("workspace id is required")
	}
	name := strings.TrimSpace(item.Name)
	if err := ValidateName("workspace", name); err != nil {
		return err
	}
	item.ID = id
	item.Name = name
	item.Version = workspaceCompositionVersion
	previous, err := getJSON[workspaceComposition](ctx, s.rdb, workspaceCompositionMetaKey(id))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := setJSON(ctx, s.rdb, workspaceCompositionMetaKey(id), item); err != nil {
		return err
	}
	if err := s.rdb.HSet(ctx, workspaceCompositionNameIndexKey(), item.Name, id).Err(); err != nil {
		return err
	}
	if strings.TrimSpace(previous.Name) != "" && previous.Name != item.Name {
		currentID, err := s.rdb.HGet(ctx, workspaceCompositionNameIndexKey(), previous.Name).Result()
		if err == nil && strings.TrimSpace(currentID) == id {
			if err := s.rdb.HDel(ctx, workspaceCompositionNameIndexKey(), previous.Name).Err(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) GetWorkspaceComposition(ctx context.Context, workspace string) (workspaceComposition, error) {
	meta, _, err := s.resolveWorkspaceComposition(ctx, workspace)
	return meta, err
}

func (s *Store) ListWorkspaceCompositions(ctx context.Context) ([]workspaceComposition, error) {
	items := make([]workspaceComposition, 0)
	var cursor uint64
	for {
		keys, next, err := s.rdb.Scan(ctx, cursor, "afs:{*}:workspace:composition:meta", 128).Result()
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			item, err := getJSON[workspaceComposition](ctx, s.rdb, key)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				return nil, err
			}
			items = append(items, item)
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	return items, nil
}

func (s *Store) DeleteWorkspaceComposition(ctx context.Context, workspace string) error {
	meta, id, err := s.resolveWorkspaceComposition(ctx, workspace)
	if err != nil {
		return err
	}
	var cursor uint64
	for {
		keys, next, err := s.rdb.Scan(ctx, cursor, workspaceCompositionPattern(id), 128).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err := s.rdb.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	if strings.TrimSpace(meta.Name) != "" {
		if err := s.rdb.HDel(ctx, workspaceCompositionNameIndexKey(), meta.Name).Err(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) PutWorkspaceBookmark(ctx context.Context, bookmark workspaceBookmark) error {
	workspaceID := strings.TrimSpace(bookmark.WorkspaceID)
	name := strings.TrimSpace(bookmark.Name)
	if workspaceID == "" {
		return fmt.Errorf("workspace id is required")
	}
	if err := ValidateName("bookmark", name); err != nil {
		return err
	}
	bookmark.WorkspaceID = workspaceID
	bookmark.Name = name
	if bookmark.CreatedAt.IsZero() {
		bookmark.CreatedAt = time.Now().UTC()
	}
	if err := setJSON(ctx, s.rdb, workspaceCompositionBookmarkKey(workspaceID, name), bookmark); err != nil {
		return err
	}
	return s.rdb.ZAdd(ctx, workspaceCompositionBookmarkIndexKey(workspaceID), redis.Z{
		Score:  float64(bookmark.CreatedAt.UTC().UnixMilli()),
		Member: name,
	}).Err()
}

func (s *Store) GetWorkspaceBookmark(ctx context.Context, workspace, name string) (workspaceBookmark, error) {
	_, id, err := s.resolveWorkspaceComposition(ctx, workspace)
	if err != nil {
		return workspaceBookmark{}, err
	}
	return getJSON[workspaceBookmark](ctx, s.rdb, workspaceCompositionBookmarkKey(id, strings.TrimSpace(name)))
}

func (s *Store) ListWorkspaceBookmarks(ctx context.Context, workspace string) ([]workspaceBookmark, error) {
	_, id, err := s.resolveWorkspaceComposition(ctx, workspace)
	if err != nil {
		return nil, err
	}
	names, err := s.rdb.ZRevRange(ctx, workspaceCompositionBookmarkIndexKey(id), 0, -1).Result()
	if err != nil {
		return nil, err
	}
	items := make([]workspaceBookmark, 0, len(names))
	for _, name := range names {
		bookmark, err := getJSON[workspaceBookmark](ctx, s.rdb, workspaceCompositionBookmarkKey(id, name))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		items = append(items, bookmark)
	}
	return items, nil
}

func (s *Service) CreateWorkspaceComposition(ctx context.Context, input createWorkspaceCompositionRequest) (workspaceCompositionDetail, error) {
	name := strings.TrimSpace(input.Name)
	if err := ValidateName("workspace", name); err != nil {
		return workspaceCompositionDetail{}, err
	}
	if _, err := s.store.GetWorkspaceComposition(ctx, name); err == nil {
		return workspaceCompositionDetail{}, fmt.Errorf("workspace %q already exists", name)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return workspaceCompositionDetail{}, err
	}
	mounts, err := s.normalizeWorkspaceCompositionMounts(ctx, input.Mounts)
	if err != nil {
		return workspaceCompositionDetail{}, err
	}
	id, err := newWorkspaceCompositionID()
	if err != nil {
		return workspaceCompositionDetail{}, err
	}
	now, err := s.store.Now(ctx)
	if err != nil {
		now = time.Now().UTC()
	}
	item := workspaceComposition{
		Version:      workspaceCompositionVersion,
		ID:           id,
		Name:         name,
		Description:  strings.TrimSpace(input.Description),
		DatabaseID:   firstNonEmpty(strings.TrimSpace(input.DatabaseID), s.catalogDatabaseID),
		DatabaseName: firstNonEmpty(strings.TrimSpace(input.DatabaseName), s.catalogDatabaseName),
		CloudAccount: strings.TrimSpace(input.CloudAccount),
		Mounts:       mounts,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.store.PutWorkspaceComposition(ctx, item); err != nil {
		return workspaceCompositionDetail{}, err
	}
	return s.workspaceCompositionDetail(ctx, item)
}

func (s *Service) ListWorkspaceCompositions(ctx context.Context) (workspaceCompositionListResponse, error) {
	items, err := s.store.ListWorkspaceCompositions(ctx)
	if err != nil {
		return workspaceCompositionListResponse{}, err
	}
	summaries := make([]workspaceCompositionSummary, 0, len(items))
	for _, item := range items {
		summary, err := s.workspaceCompositionSummary(ctx, item)
		if err != nil {
			return workspaceCompositionListResponse{}, err
		}
		summaries = append(summaries, summary)
	}
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].UpdatedAt == summaries[j].UpdatedAt {
			return strings.ToLower(summaries[i].Name) < strings.ToLower(summaries[j].Name)
		}
		return summaries[i].UpdatedAt > summaries[j].UpdatedAt
	})
	return workspaceCompositionListResponse{Items: summaries}, nil
}

func (s *Service) GetWorkspaceComposition(ctx context.Context, workspace string) (workspaceCompositionDetail, error) {
	item, err := s.store.GetWorkspaceComposition(ctx, workspace)
	if err != nil {
		return workspaceCompositionDetail{}, err
	}
	return s.workspaceCompositionDetail(ctx, item)
}

func (s *Service) UpdateWorkspaceComposition(ctx context.Context, workspace string, input updateWorkspaceCompositionRequest) (workspaceCompositionDetail, error) {
	item, err := s.store.GetWorkspaceComposition(ctx, workspace)
	if err != nil {
		return workspaceCompositionDetail{}, err
	}
	if name := strings.TrimSpace(input.Name); name != "" && name != item.Name {
		if err := ValidateName("workspace", name); err != nil {
			return workspaceCompositionDetail{}, err
		}
		if existing, err := s.store.GetWorkspaceComposition(ctx, name); err == nil && existing.ID != item.ID {
			return workspaceCompositionDetail{}, fmt.Errorf("workspace %q already exists", name)
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return workspaceCompositionDetail{}, err
		}
		item.Name = name
	}
	item.Description = strings.TrimSpace(input.Description)
	item.UpdatedAt = time.Now().UTC()
	if err := s.store.PutWorkspaceComposition(ctx, item); err != nil {
		return workspaceCompositionDetail{}, err
	}
	return s.workspaceCompositionDetail(ctx, item)
}

func (s *Service) DeleteWorkspaceComposition(ctx context.Context, workspace string) error {
	return s.store.DeleteWorkspaceComposition(ctx, workspace)
}

func (s *Service) ReplaceWorkspaceCompositionMounts(ctx context.Context, workspace string, mounts []workspaceCompositionMount) (workspaceCompositionDetail, error) {
	item, err := s.store.GetWorkspaceComposition(ctx, workspace)
	if err != nil {
		return workspaceCompositionDetail{}, err
	}
	normalized, err := s.normalizeWorkspaceCompositionMounts(ctx, mounts)
	if err != nil {
		return workspaceCompositionDetail{}, err
	}
	item.Mounts = normalized
	item.UpdatedAt = time.Now().UTC()
	if err := s.store.PutWorkspaceComposition(ctx, item); err != nil {
		return workspaceCompositionDetail{}, err
	}
	return s.workspaceCompositionDetail(ctx, item)
}

func (s *Service) AddWorkspaceCompositionMount(ctx context.Context, workspace string, mount workspaceCompositionMount) (workspaceCompositionDetail, error) {
	item, err := s.store.GetWorkspaceComposition(ctx, workspace)
	if err != nil {
		return workspaceCompositionDetail{}, err
	}
	next := append([]workspaceCompositionMount(nil), item.Mounts...)
	next = append(next, mount)
	return s.ReplaceWorkspaceCompositionMounts(ctx, item.ID, next)
}

func (s *Service) RemoveWorkspaceCompositionMount(ctx context.Context, workspace, volumeID string) (workspaceCompositionDetail, error) {
	item, err := s.store.GetWorkspaceComposition(ctx, workspace)
	if err != nil {
		return workspaceCompositionDetail{}, err
	}
	target := strings.TrimSpace(volumeID)
	if target == "" {
		return workspaceCompositionDetail{}, fmt.Errorf("volume id is required")
	}
	next := make([]workspaceCompositionMount, 0, len(item.Mounts))
	removed := false
	for _, mount := range item.Mounts {
		if strings.TrimSpace(mount.VolumeID) == target || strings.TrimSpace(mount.VolumeName) == target {
			removed = true
			continue
		}
		next = append(next, mount)
	}
	if !removed {
		return workspaceCompositionDetail{}, os.ErrNotExist
	}
	return s.ReplaceWorkspaceCompositionMounts(ctx, item.ID, next)
}

func (s *Service) CreateWorkspaceBookmark(ctx context.Context, workspace string, input createWorkspaceBookmarkRequest) (workspaceBookmark, error) {
	item, err := s.store.GetWorkspaceComposition(ctx, workspace)
	if err != nil {
		return workspaceBookmark{}, err
	}
	name := strings.TrimSpace(input.Name)
	if err := ValidateName("bookmark", name); err != nil {
		return workspaceBookmark{}, err
	}
	volumes := make([]workspaceBookmarkVolume, 0, len(item.Mounts))
	for _, mount := range item.Mounts {
		meta, err := s.store.GetWorkspaceMeta(ctx, mount.VolumeID)
		if err != nil {
			return workspaceBookmark{}, fmt.Errorf("volume %q: %w", mount.VolumeID, err)
		}
		volumes = append(volumes, workspaceBookmarkVolume{
			VolumeID:     workspaceStorageID(meta),
			VolumeName:   meta.Name,
			CheckpointID: meta.HeadSavepoint,
		})
	}
	bookmark := workspaceBookmark{
		WorkspaceID: item.ID,
		Name:        name,
		Description: strings.TrimSpace(input.Description),
		Volumes:     volumes,
		CreatedAt:   time.Now().UTC(),
	}
	if err := s.store.PutWorkspaceBookmark(ctx, bookmark); err != nil {
		return workspaceBookmark{}, err
	}
	item.LastActivityAt = bookmark.CreatedAt
	item.UpdatedAt = bookmark.CreatedAt
	_ = s.store.PutWorkspaceComposition(ctx, item)
	return bookmark, nil
}

func (s *Service) ListWorkspaceBookmarks(ctx context.Context, workspace string) (workspaceBookmarkListResponse, error) {
	items, err := s.store.ListWorkspaceBookmarks(ctx, workspace)
	if err != nil {
		return workspaceBookmarkListResponse{}, err
	}
	return workspaceBookmarkListResponse{Items: items}, nil
}

func (s *Service) RestoreWorkspaceBookmark(ctx context.Context, workspace, name string) (workspaceBookmark, error) {
	item, err := s.store.GetWorkspaceComposition(ctx, workspace)
	if err != nil {
		return workspaceBookmark{}, err
	}
	bookmark, err := s.store.GetWorkspaceBookmark(ctx, item.ID, name)
	if err != nil {
		return workspaceBookmark{}, err
	}
	for _, volume := range bookmark.Volumes {
		if strings.TrimSpace(volume.VolumeID) == "" || strings.TrimSpace(volume.CheckpointID) == "" {
			return workspaceBookmark{}, fmt.Errorf("bookmark %q has an incomplete volume reference", bookmark.Name)
		}
		if _, err := s.RestoreCheckpointWithResult(ctx, volume.VolumeID, volume.CheckpointID); err != nil {
			return workspaceBookmark{}, fmt.Errorf("restore volume %q checkpoint %q: %w", volume.VolumeID, volume.CheckpointID, err)
		}
	}
	now := time.Now().UTC()
	item.LastActivityAt = now
	item.UpdatedAt = now
	_ = s.store.PutWorkspaceComposition(ctx, item)
	return bookmark, nil
}

func (s *Service) normalizeWorkspaceCompositionMounts(ctx context.Context, mounts []workspaceCompositionMount) ([]workspaceCompositionMount, error) {
	normalized := make([]workspaceCompositionMount, 0, len(mounts))
	seen := map[string]struct{}{}
	for _, mount := range mounts {
		volumeRef := strings.TrimSpace(mount.VolumeID)
		if volumeRef == "" {
			return nil, fmt.Errorf("volume_id is required")
		}
		meta, err := s.store.GetWorkspaceMeta(ctx, volumeRef)
		if err != nil {
			return nil, fmt.Errorf("volume %q: %w", volumeRef, err)
		}
		mountPath, err := normalizeWorkspaceMountPath(mount.MountPath)
		if err != nil {
			return nil, err
		}
		volumeID := workspaceStorageID(meta)
		key := volumeID + "\x00" + mountPath
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, workspaceCompositionMount{
			VolumeID:      volumeID,
			VolumeName:    meta.Name,
			MountPath:     mountPath,
			Readonly:      mount.Readonly,
			VolumeTokenID: strings.TrimSpace(mount.VolumeTokenID),
		})
	}
	if err := validateWorkspaceMountConflicts(normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

func normalizeWorkspaceMountPath(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		value = "/"
	}
	if strings.Contains(value, "\x00") {
		return "", fmt.Errorf("mount_path contains an invalid character")
	}
	value = strings.ReplaceAll(value, "\\", "/")
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	clean := path.Clean(value)
	if clean == "." {
		clean = "/"
	}
	if clean != "/" && strings.HasPrefix(clean, "/../") {
		return "", fmt.Errorf("mount_path %q escapes the workspace root", raw)
	}
	for _, part := range strings.Split(strings.Trim(clean, "/"), "/") {
		if part == ".." {
			return "", fmt.Errorf("mount_path %q escapes the workspace root", raw)
		}
	}
	return clean, nil
}

func validateWorkspaceMountConflicts(mounts []workspaceCompositionMount) error {
	byPath := map[string]string{}
	for _, mount := range mounts {
		mountPath := strings.TrimSpace(mount.MountPath)
		volumeID := strings.TrimSpace(mount.VolumeID)
		if existing := strings.TrimSpace(byPath[mountPath]); existing != "" && existing != volumeID {
			return fmt.Errorf("mount_path %q is already used by volume %q", mountPath, existing)
		}
		byPath[mountPath] = volumeID
	}
	for i := range mounts {
		for j := i + 1; j < len(mounts); j++ {
			left := mounts[i]
			right := mounts[j]
			if left.MountPath == right.MountPath {
				continue
			}
			if left.VolumeID == right.VolumeID {
				continue
			}
			if isWorkspaceMountPathAncestor(left.MountPath, right.MountPath) || isWorkspaceMountPathAncestor(right.MountPath, left.MountPath) {
				return fmt.Errorf("mount paths %q and %q overlap", left.MountPath, right.MountPath)
			}
		}
	}
	return nil
}

func isWorkspaceMountPathAncestor(parent, child string) bool {
	parent = strings.TrimSuffix(parent, "/")
	child = strings.TrimSuffix(child, "/")
	if parent == "" {
		parent = "/"
	}
	if parent == child {
		return true
	}
	if parent == "/" {
		return child != "/"
	}
	return strings.HasPrefix(child, parent+"/")
}

func (s *Service) workspaceCompositionSummary(ctx context.Context, item workspaceComposition) (workspaceCompositionSummary, error) {
	mounted := make([]workspaceCompositionVolumeLabel, 0, len(item.Mounts))
	for _, mount := range item.Mounts {
		mounted = append(mounted, workspaceCompositionVolumeLabel{
			ID:        mount.VolumeID,
			Name:      mount.VolumeName,
			MountPath: mount.MountPath,
			Readonly:  mount.Readonly,
		})
	}
	return workspaceCompositionSummary{
		ID:             item.ID,
		Name:           item.Name,
		Description:    item.Description,
		DatabaseID:     item.DatabaseID,
		DatabaseName:   item.DatabaseName,
		CloudAccount:   item.CloudAccount,
		OwnerSubject:   item.OwnerSubject,
		OwnerLabel:     item.OwnerLabel,
		MountCount:     len(item.Mounts),
		MountedVolumes: mounted,
		LastActivityAt: formatOptionalTime(item.LastActivityAt),
		UpdatedAt:      item.UpdatedAt.Format(timeRFC3339),
	}, nil
}

func (s *Service) workspaceCompositionDetail(ctx context.Context, item workspaceComposition) (workspaceCompositionDetail, error) {
	bookmarks, err := s.store.ListWorkspaceBookmarks(ctx, item.ID)
	if err != nil {
		return workspaceCompositionDetail{}, err
	}
	return workspaceCompositionDetail{
		ID:             item.ID,
		Name:           item.Name,
		Description:    item.Description,
		DatabaseID:     item.DatabaseID,
		DatabaseName:   item.DatabaseName,
		CloudAccount:   item.CloudAccount,
		OwnerSubject:   item.OwnerSubject,
		OwnerLabel:     item.OwnerLabel,
		Mounts:         item.Mounts,
		Bookmarks:      bookmarks,
		CreatedAt:      item.CreatedAt.Format(timeRFC3339),
		UpdatedAt:      item.UpdatedAt.Format(timeRFC3339),
		LastActivityAt: formatOptionalTime(item.LastActivityAt),
	}, nil
}

func formatOptionalTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(timeRFC3339)
}

func (m *DatabaseManager) CreateResolvedWorkspaceComposition(ctx context.Context, input createWorkspaceCompositionRequest) (workspaceCompositionDetail, error) {
	profile, err := m.resolveTargetDatabase(ctx, input.DatabaseID)
	if err != nil {
		return workspaceCompositionDetail{}, err
	}
	return m.CreateWorkspaceComposition(ctx, profile.ID, input)
}

func (m *DatabaseManager) CreateWorkspaceComposition(ctx context.Context, databaseID string, input createWorkspaceCompositionRequest) (workspaceCompositionDetail, error) {
	service, profile, err := m.serviceFor(ctx, databaseID)
	if err != nil {
		return workspaceCompositionDetail{}, err
	}
	input.DatabaseID = profile.ID
	input.DatabaseName = profile.Name
	if strings.TrimSpace(input.CloudAccount) == "" {
		input.CloudAccount = quickstartCloudAccount(profile)
	}
	detail, err := service.CreateWorkspaceComposition(ctx, input)
	if err != nil {
		return workspaceCompositionDetail{}, err
	}
	detail.DatabaseID = profile.ID
	detail.DatabaseName = profile.Name
	if strings.TrimSpace(profile.OwnerSubject) != "" {
		detail.OwnerSubject = profile.OwnerSubject
		detail.OwnerLabel = profile.OwnerLabel
	}
	return detail, nil
}

func (m *DatabaseManager) ListAllWorkspaceCompositions(ctx context.Context) (workspaceCompositionListResponse, error) {
	subject := authSubjectFromContext(ctx)
	m.mu.Lock()
	visibleIDs := make([]string, 0, len(m.order))
	for _, id := range m.order {
		if profile, ok := m.profiles[id]; ok && !databaseProfileVisibleToSubject(profile, subject) {
			continue
		}
		visibleIDs = append(visibleIDs, id)
	}
	m.mu.Unlock()

	var (
		mu    sync.Mutex
		items = make([]workspaceCompositionSummary, 0)
		wg    sync.WaitGroup
	)
	for _, id := range visibleIDs {
		wg.Add(1)
		go func(databaseID string) {
			defer wg.Done()
			dbCtx, cancel := context.WithTimeout(ctx, perDatabaseListTimeout)
			defer cancel()
			response, err := m.ListWorkspaceCompositions(dbCtx, databaseID)
			if err != nil {
				return
			}
			mu.Lock()
			items = append(items, response.Items...)
			mu.Unlock()
		}(id)
	}
	wg.Wait()
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt == items[j].UpdatedAt {
			return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
		}
		return items[i].UpdatedAt > items[j].UpdatedAt
	})
	return workspaceCompositionListResponse{Items: items}, nil
}

func (m *DatabaseManager) ListWorkspaceCompositions(ctx context.Context, databaseID string) (workspaceCompositionListResponse, error) {
	service, profile, err := m.serviceFor(ctx, databaseID)
	if err != nil {
		return workspaceCompositionListResponse{}, err
	}
	response, err := service.ListWorkspaceCompositions(ctx)
	if err != nil {
		return workspaceCompositionListResponse{}, err
	}
	for index := range response.Items {
		stampWorkspaceCompositionSummary(&response.Items[index], profile)
	}
	response.Items = filterWorkspaceCompositionSummariesForCLIToken(ctx, response.Items)
	return response, nil
}

func (m *DatabaseManager) GetResolvedWorkspaceComposition(ctx context.Context, workspace string) (workspaceCompositionDetail, error) {
	service, profile, _, err := m.resolveWorkspaceComposition(ctx, workspace)
	if err != nil {
		return workspaceCompositionDetail{}, err
	}
	detail, err := service.GetWorkspaceComposition(ctx, workspace)
	if err != nil {
		return workspaceCompositionDetail{}, err
	}
	stampWorkspaceCompositionDetail(&detail, profile)
	return detail, nil
}

func (m *DatabaseManager) UpdateResolvedWorkspaceComposition(ctx context.Context, workspace string, input updateWorkspaceCompositionRequest) (workspaceCompositionDetail, error) {
	service, profile, route, err := m.resolveWorkspaceComposition(ctx, workspace)
	if err != nil {
		return workspaceCompositionDetail{}, err
	}
	detail, err := service.UpdateWorkspaceComposition(ctx, route.ID, input)
	if err != nil {
		return workspaceCompositionDetail{}, err
	}
	stampWorkspaceCompositionDetail(&detail, profile)
	return detail, nil
}

func (m *DatabaseManager) DeleteResolvedWorkspaceComposition(ctx context.Context, workspace string) error {
	service, _, route, err := m.resolveWorkspaceComposition(ctx, workspace)
	if err != nil {
		return err
	}
	return service.DeleteWorkspaceComposition(ctx, route.ID)
}

func (m *DatabaseManager) ReplaceResolvedWorkspaceCompositionMounts(ctx context.Context, workspace string, mounts []workspaceCompositionMount) (workspaceCompositionDetail, error) {
	service, profile, route, err := m.resolveWorkspaceComposition(ctx, workspace)
	if err != nil {
		return workspaceCompositionDetail{}, err
	}
	detail, err := service.ReplaceWorkspaceCompositionMounts(ctx, route.ID, mounts)
	if err != nil {
		return workspaceCompositionDetail{}, err
	}
	stampWorkspaceCompositionDetail(&detail, profile)
	return detail, nil
}

func (m *DatabaseManager) AddResolvedWorkspaceCompositionMount(ctx context.Context, workspace string, mount workspaceCompositionMount) (workspaceCompositionDetail, error) {
	service, profile, route, err := m.resolveWorkspaceComposition(ctx, workspace)
	if err != nil {
		return workspaceCompositionDetail{}, err
	}
	detail, err := service.AddWorkspaceCompositionMount(ctx, route.ID, mount)
	if err != nil {
		return workspaceCompositionDetail{}, err
	}
	stampWorkspaceCompositionDetail(&detail, profile)
	return detail, nil
}

func (m *DatabaseManager) RemoveResolvedWorkspaceCompositionMount(ctx context.Context, workspace, volumeID string) (workspaceCompositionDetail, error) {
	service, profile, route, err := m.resolveWorkspaceComposition(ctx, workspace)
	if err != nil {
		return workspaceCompositionDetail{}, err
	}
	detail, err := service.RemoveWorkspaceCompositionMount(ctx, route.ID, volumeID)
	if err != nil {
		return workspaceCompositionDetail{}, err
	}
	stampWorkspaceCompositionDetail(&detail, profile)
	return detail, nil
}

func (m *DatabaseManager) ListResolvedWorkspaceBookmarks(ctx context.Context, workspace string) (workspaceBookmarkListResponse, error) {
	service, _, route, err := m.resolveWorkspaceComposition(ctx, workspace)
	if err != nil {
		return workspaceBookmarkListResponse{}, err
	}
	return service.ListWorkspaceBookmarks(ctx, route.ID)
}

func (m *DatabaseManager) CreateResolvedWorkspaceBookmark(ctx context.Context, workspace string, input createWorkspaceBookmarkRequest) (workspaceBookmark, error) {
	service, _, route, err := m.resolveWorkspaceComposition(ctx, workspace)
	if err != nil {
		return workspaceBookmark{}, err
	}
	return service.CreateWorkspaceBookmark(ctx, route.ID, input)
}

func (m *DatabaseManager) RestoreResolvedWorkspaceBookmark(ctx context.Context, workspace, name string) (workspaceBookmark, error) {
	service, _, route, err := m.resolveWorkspaceComposition(ctx, workspace)
	if err != nil {
		return workspaceBookmark{}, err
	}
	return service.RestoreWorkspaceBookmark(ctx, route.ID, name)
}

type workspaceCompositionRoute struct {
	ID         string
	Name       string
	DatabaseID string
}

func (m *DatabaseManager) resolveWorkspaceComposition(ctx context.Context, workspace string) (*Service, databaseProfile, workspaceCompositionRoute, error) {
	ref := strings.TrimSpace(workspace)
	if ref == "" {
		return nil, databaseProfile{}, workspaceCompositionRoute{}, fmt.Errorf("workspace id is required")
	}
	subject := authSubjectFromContext(ctx)
	m.mu.Lock()
	ids := append([]string(nil), m.order...)
	profiles := make(map[string]databaseProfile, len(m.profiles))
	for id, profile := range m.profiles {
		profiles[id] = profile
	}
	m.mu.Unlock()

	var (
		matchService *Service
		matchProfile databaseProfile
		matchRoute   workspaceCompositionRoute
		matches      int
	)
	for _, databaseID := range ids {
		profile, ok := profiles[databaseID]
		if !ok || !databaseProfileVisibleToSubject(profile, subject) {
			continue
		}
		service, _, err := m.serviceFor(ctx, databaseID)
		if err != nil {
			continue
		}
		detail, err := service.GetWorkspaceComposition(ctx, ref)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, databaseProfile{}, workspaceCompositionRoute{}, err
		}
		if requireCLITokenWorkspaceCompositionAccess(ctx, databaseID, detail.ID, detail.Name, detail.Mounts) != nil {
			continue
		}
		matches++
		matchService = service
		matchProfile = profile
		matchRoute = workspaceCompositionRoute{
			ID:         detail.ID,
			Name:       detail.Name,
			DatabaseID: databaseID,
		}
	}
	if matches == 0 {
		return nil, databaseProfile{}, workspaceCompositionRoute{}, os.ErrNotExist
	}
	if matches > 1 {
		return nil, databaseProfile{}, workspaceCompositionRoute{}, fmt.Errorf("%w: workspace %q exists in multiple databases", ErrAmbiguousWorkspace, ref)
	}
	return matchService, matchProfile, matchRoute, nil
}

func stampWorkspaceCompositionSummary(summary *workspaceCompositionSummary, profile databaseProfile) {
	summary.DatabaseID = profile.ID
	summary.DatabaseName = profile.Name
	if v := strings.TrimSpace(profile.OwnerSubject); v != "" {
		summary.OwnerSubject = v
		summary.OwnerLabel = profile.OwnerLabel
	}
}

func stampWorkspaceCompositionDetail(detail *workspaceCompositionDetail, profile databaseProfile) {
	detail.DatabaseID = profile.ID
	detail.DatabaseName = profile.Name
	if v := strings.TrimSpace(profile.OwnerSubject); v != "" {
		detail.OwnerSubject = v
		detail.OwnerLabel = profile.OwnerLabel
	}
}
