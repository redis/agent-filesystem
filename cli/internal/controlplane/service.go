package controlplane

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/redis/go-redis/v9"
)

const (
	sourceBlank       = "blank"
	sourceGitImport   = "git-import"
	sourceCloudImport = "cloud-import"
)

const (
	SourceBlank       = sourceBlank
	SourceGitImport   = sourceGitImport
	SourceCloudImport = sourceCloudImport
)

var ErrUnsupportedView = errors.New("control plane operation is not available for this workspace view")
var ErrWorkspaceConflict = errors.New("control plane workspace conflict")

type capabilities struct {
	BrowseHead        bool `json:"browse_head"`
	BrowseCheckpoints bool `json:"browse_checkpoints"`
	BrowseWorkingCopy bool `json:"browse_working_copy"`
	EditWorkingCopy   bool `json:"edit_working_copy"`
	CreateCheckpoint  bool `json:"create_checkpoint"`
	RestoreCheckpoint bool `json:"restore_checkpoint"`
}

type workspaceSummary struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	CloudAccount     string `json:"cloud_account"`
	DatabaseID       string `json:"database_id"`
	DatabaseName     string `json:"database_name"`
	RedisKey         string `json:"redis_key"`
	Status           string `json:"status"`
	FileCount        int    `json:"file_count"`
	FolderCount      int    `json:"folder_count"`
	TotalBytes       int64  `json:"total_bytes"`
	CheckpointCount  int    `json:"checkpoint_count"`
	DraftState       string `json:"draft_state"`
	LastCheckpointAt string `json:"last_checkpoint_at"`
	UpdatedAt        string `json:"updated_at"`
	Region           string `json:"region"`
	Source           string `json:"source"`
}

type workspaceListResponse struct {
	Items []workspaceSummary `json:"items"`
}

type checkpointSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Author      string `json:"author,omitempty"`
	Note        string `json:"note,omitempty"`
	CreatedAt   string `json:"created_at"`
	FileCount   int    `json:"file_count"`
	FolderCount int    `json:"folder_count"`
	TotalBytes  int64  `json:"total_bytes"`
	IsHead      bool   `json:"is_head"`
}

type activityEvent struct {
	ID            string `json:"id"`
	WorkspaceID   string `json:"workspace_id,omitempty"`
	WorkspaceName string `json:"workspace_name,omitempty"`
	Actor         string `json:"actor"`
	CreatedAt     string `json:"created_at"`
	Detail        string `json:"detail"`
	Kind          string `json:"kind"`
	Scope         string `json:"scope"`
	Title         string `json:"title"`
}

type activityListResponse struct {
	Items []activityEvent `json:"items"`
}

type workspaceDetail struct {
	ID               string              `json:"id"`
	Name             string              `json:"name"`
	Description      string              `json:"description,omitempty"`
	CloudAccount     string              `json:"cloud_account"`
	DatabaseID       string              `json:"database_id"`
	DatabaseName     string              `json:"database_name"`
	RedisKey         string              `json:"redis_key"`
	Region           string              `json:"region"`
	Status           string              `json:"status"`
	Source           string              `json:"source"`
	CreatedAt        string              `json:"created_at"`
	UpdatedAt        string              `json:"updated_at"`
	DraftState       string              `json:"draft_state"`
	HeadCheckpointID string              `json:"head_checkpoint_id"`
	Tags             []string            `json:"tags,omitempty"`
	FileCount        int                 `json:"file_count"`
	FolderCount      int                 `json:"folder_count"`
	TotalBytes       int64               `json:"total_bytes"`
	CheckpointCount  int                 `json:"checkpoint_count"`
	Checkpoints      []checkpointSummary `json:"checkpoints"`
	Activity         []activityEvent     `json:"activity"`
	Capabilities     capabilities        `json:"capabilities"`
}

type treeItem struct {
	Path       string `json:"path"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Size       int64  `json:"size"`
	ModifiedAt string `json:"modified_at,omitempty"`
	Target     string `json:"target,omitempty"`
}

type treeResponse struct {
	WorkspaceID string     `json:"workspace_id"`
	View        string     `json:"view"`
	Path        string     `json:"path"`
	Items       []treeItem `json:"items"`
}

type fileContentResponse struct {
	WorkspaceID string `json:"workspace_id"`
	View        string `json:"view"`
	Path        string `json:"path"`
	Kind        string `json:"kind"`
	Revision    string `json:"revision"`
	Language    string `json:"language"`
	Encoding    string `json:"encoding"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	ModifiedAt  string `json:"modified_at,omitempty"`
	Binary      bool   `json:"binary"`
	Content     string `json:"content,omitempty"`
	Target      string `json:"target,omitempty"`
}

type sourceRef struct {
	Kind string `json:"kind"`
}

type createWorkspaceRequest struct {
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	DatabaseID   string    `json:"database_id"`
	DatabaseName string    `json:"database_name"`
	CloudAccount string    `json:"cloud_account"`
	Region       string    `json:"region"`
	Source       sourceRef `json:"source"`
}

type updateWorkspaceRequest struct {
	Description  string `json:"description"`
	DatabaseName string `json:"database_name"`
	CloudAccount string `json:"cloud_account"`
	Region       string `json:"region"`
}

type restoreCheckpointRequest struct {
	CheckpointID string `json:"checkpoint_id"`
}

type viewRef struct {
	Kind         string
	CheckpointID string
}

type Service struct {
	cfg   Config
	store *Store
}

type Capabilities = capabilities
type WorkspaceSummary = workspaceSummary
type WorkspaceListResponse = workspaceListResponse
type CheckpointSummary = checkpointSummary
type ActivityEvent = activityEvent
type ActivityListResponse = activityListResponse
type WorkspaceDetail = workspaceDetail
type TreeItem = treeItem
type TreeResponse = treeResponse
type FileContentResponse = fileContentResponse
type SourceRef = sourceRef
type CreateWorkspaceRequest = createWorkspaceRequest
type UpdateWorkspaceRequest = updateWorkspaceRequest

type SaveCheckpointRequest struct {
	Workspace    string
	ExpectedHead string
	CheckpointID string
	Manifest     Manifest
	Blobs        map[string][]byte
	FileCount    int
	DirCount     int
	TotalBytes   int64
}

func NewService(cfg Config, store *Store) *Service {
	return &Service{cfg: cfg, store: store}
}

func (s *Service) ListWorkspaceSummaries(ctx context.Context) (WorkspaceListResponse, error) {
	return s.listWorkspaceSummaries(ctx)
}

func (s *Service) GetWorkspace(ctx context.Context, workspace string) (WorkspaceDetail, error) {
	return s.getWorkspace(ctx, workspace)
}

func (s *Service) CreateWorkspace(ctx context.Context, input CreateWorkspaceRequest) (WorkspaceDetail, error) {
	return s.createWorkspace(ctx, input)
}

func (s *Service) UpdateWorkspace(ctx context.Context, workspace string, input UpdateWorkspaceRequest) (WorkspaceDetail, error) {
	return s.updateWorkspace(ctx, workspace, input)
}

func (s *Service) DeleteWorkspace(ctx context.Context, workspace string) error {
	return s.deleteWorkspace(ctx, workspace)
}

func (s *Service) ListCheckpoints(ctx context.Context, workspace string, limit int) ([]CheckpointSummary, error) {
	return s.listCheckpoints(ctx, workspace, limit)
}

func (s *Service) RestoreCheckpoint(ctx context.Context, workspace, checkpointID string) error {
	return s.restoreCheckpoint(ctx, workspace, checkpointID)
}

func (s *Service) SaveCheckpoint(ctx context.Context, input SaveCheckpointRequest) (bool, error) {
	return s.saveCheckpoint(ctx, input)
}

func (s *Service) ForkWorkspace(ctx context.Context, sourceWorkspace, newWorkspace string) error {
	return s.forkWorkspace(ctx, sourceWorkspace, newWorkspace)
}

func (s *Service) GetTree(ctx context.Context, workspace, rawView, rawPath string, depth int) (TreeResponse, error) {
	return s.getTree(ctx, workspace, rawView, rawPath, depth)
}

func (s *Service) GetFileContent(ctx context.Context, workspace, rawView, rawPath string) (FileContentResponse, error) {
	return s.getFileContent(ctx, workspace, rawView, rawPath)
}

func (s *Service) ListWorkspaceActivity(ctx context.Context, workspace string, limit int) (ActivityListResponse, error) {
	return s.listWorkspaceActivity(ctx, workspace, limit)
}

func (s *Service) ListGlobalActivity(ctx context.Context, limit int) (ActivityListResponse, error) {
	return s.listGlobalActivity(ctx, limit)
}

func ApplyWorkspaceMetaDefaults(cfg Config, meta WorkspaceMeta) WorkspaceMeta {
	return applyWorkspaceMetaDefaults(cfg, meta)
}

func WorkspaceTags(region, source string) []string {
	return workspaceTags(region, source)
}

func WorkspaceSource(meta WorkspaceMeta) string {
	return workspaceSource(meta)
}

func ManifestEquivalent(a, b Manifest) bool {
	return manifestEquivalent(a, b)
}

func ManifestBlobRefs(m Manifest) map[string]int64 {
	return manifestBlobRefs(m)
}

func (s *Service) listWorkspaceSummaries(ctx context.Context) (workspaceListResponse, error) {
	metas, err := s.store.ListWorkspaces(ctx)
	if err != nil {
		return workspaceListResponse{}, err
	}
	items := make([]workspaceSummary, 0, len(metas))
	for _, meta := range metas {
		summary, err := s.buildWorkspaceSummary(ctx, meta)
		if err != nil {
			return workspaceListResponse{}, err
		}
		items = append(items, summary)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt > items[j].UpdatedAt })
	return workspaceListResponse{Items: items}, nil
}

func (s *Service) getWorkspace(ctx context.Context, workspace string) (workspaceDetail, error) {
	meta, err := s.store.GetWorkspaceMeta(ctx, workspace)
	if err != nil {
		return workspaceDetail{}, err
	}
	meta = applyWorkspaceMetaDefaults(s.cfg, meta)
	checkpoints, err := s.store.ListSavepoints(ctx, workspace, 100)
	if err != nil {
		return workspaceDetail{}, err
	}
	headMeta, err := s.store.GetSavepointMeta(ctx, workspace, meta.HeadSavepoint)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return workspaceDetail{}, err
	}
	activity, err := s.listWorkspaceActivity(ctx, workspace, 25)
	if err != nil {
		return workspaceDetail{}, err
	}

	detail := workspaceDetail{
		ID:               meta.Name,
		Name:             meta.Name,
		Description:      meta.Description,
		CloudAccount:     meta.CloudAccount,
		DatabaseID:       meta.DatabaseID,
		DatabaseName:     meta.DatabaseName,
		RedisKey:         WorkspaceFSKey(meta.Name),
		Region:           meta.Region,
		Status:           workspaceStatus(meta),
		Source:           workspaceSource(meta),
		CreatedAt:        meta.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:        meta.UpdatedAt.UTC().Format(time.RFC3339),
		DraftState:       draftState(meta),
		HeadCheckpointID: meta.HeadSavepoint,
		Tags:             append([]string(nil), meta.Tags...),
		FileCount:        headMeta.FileCount,
		FolderCount:      headMeta.DirCount,
		TotalBytes:       headMeta.TotalBytes,
		CheckpointCount:  len(checkpoints),
		Checkpoints:      make([]checkpointSummary, 0, len(checkpoints)),
		Activity:         activity.Items,
		Capabilities:     defaultCapabilities(),
	}

	for _, checkpoint := range checkpoints {
		detail.Checkpoints = append(detail.Checkpoints, checkpointSummary{
			ID:          checkpoint.ID,
			Name:        checkpoint.Name,
			Author:      defaultString(checkpoint.Author, "afs"),
			Note:        checkpoint.Description,
			CreatedAt:   checkpoint.CreatedAt.UTC().Format(time.RFC3339),
			FileCount:   checkpoint.FileCount,
			FolderCount: checkpoint.DirCount,
			TotalBytes:  checkpoint.TotalBytes,
			IsHead:      checkpoint.ID == meta.HeadSavepoint,
		})
	}
	return detail, nil
}

func (s *Service) createWorkspace(ctx context.Context, input createWorkspaceRequest) (workspaceDetail, error) {
	workspace := strings.TrimSpace(input.Name)
	if err := ValidateName("workspace", workspace); err != nil {
		return workspaceDetail{}, err
	}
	exists, err := s.store.WorkspaceExists(ctx, workspace)
	if err != nil {
		return workspaceDetail{}, err
	}
	if exists {
		return workspaceDetail{}, fmt.Errorf("workspace %q already exists", workspace)
	}
	spec := workspaceCreateSpec{
		Description:  strings.TrimSpace(input.Description),
		DatabaseID:   strings.TrimSpace(input.DatabaseID),
		DatabaseName: strings.TrimSpace(input.DatabaseName),
		CloudAccount: strings.TrimSpace(input.CloudAccount),
		Region:       strings.TrimSpace(input.Region),
		Source:       strings.TrimSpace(input.Source.Kind),
		Tags:         workspaceTags(strings.TrimSpace(input.Region), strings.TrimSpace(input.Source.Kind)),
	}
	if err := createWorkspaceWithMetadata(ctx, s.cfg, s.store, workspace, spec); err != nil {
		return workspaceDetail{}, err
	}
	return s.getWorkspace(ctx, workspace)
}

func (s *Service) deleteWorkspace(ctx context.Context, workspace string) error {
	if err := ValidateName("workspace", workspace); err != nil {
		return err
	}
	exists, err := s.store.WorkspaceExists(ctx, workspace)
	if err != nil {
		return err
	}
	if !exists {
		return os.ErrNotExist
	}
	return s.store.DeleteWorkspace(ctx, workspace)
}

func (s *Service) updateWorkspace(ctx context.Context, workspace string, input updateWorkspaceRequest) (workspaceDetail, error) {
	if err := ValidateName("workspace", workspace); err != nil {
		return workspaceDetail{}, err
	}
	meta, err := s.store.GetWorkspaceMeta(ctx, workspace)
	if err != nil {
		return workspaceDetail{}, err
	}
	databaseName := strings.TrimSpace(input.DatabaseName)
	if databaseName == "" {
		return workspaceDetail{}, fmt.Errorf("database name is required")
	}
	cloudAccount := strings.TrimSpace(input.CloudAccount)
	if cloudAccount == "" {
		return workspaceDetail{}, fmt.Errorf("cloud account is required")
	}

	meta = applyWorkspaceMetaDefaults(s.cfg, meta)
	meta.Description = strings.TrimSpace(input.Description)
	meta.DatabaseName = databaseName
	meta.CloudAccount = cloudAccount
	meta.Region = strings.TrimSpace(input.Region)
	meta.Tags = workspaceTags(meta.Region, workspaceSource(meta))
	meta.UpdatedAt = time.Now().UTC()

	if err := s.store.PutWorkspaceMeta(ctx, meta); err != nil {
		return workspaceDetail{}, err
	}
	if err := s.store.Audit(ctx, workspace, "workspace_update", map[string]any{
		"database_name": meta.DatabaseName,
		"cloud_account": meta.CloudAccount,
		"region":        meta.Region,
	}); err != nil {
		return workspaceDetail{}, err
	}
	return s.getWorkspace(ctx, workspace)
}

func (s *Service) listCheckpoints(ctx context.Context, workspace string, limit int) ([]checkpointSummary, error) {
	if limit <= 0 {
		limit = 100
	}
	meta, err := s.store.GetWorkspaceMeta(ctx, workspace)
	if err != nil {
		return nil, err
	}
	checkpoints, err := s.store.ListSavepoints(ctx, workspace, int64(limit))
	if err != nil {
		return nil, err
	}
	items := make([]checkpointSummary, 0, len(checkpoints))
	for _, checkpoint := range checkpoints {
		items = append(items, checkpointSummary{
			ID:          checkpoint.ID,
			Name:        checkpoint.Name,
			Author:      defaultString(checkpoint.Author, "afs"),
			Note:        checkpoint.Description,
			CreatedAt:   checkpoint.CreatedAt.UTC().Format(time.RFC3339),
			FileCount:   checkpoint.FileCount,
			FolderCount: checkpoint.DirCount,
			TotalBytes:  checkpoint.TotalBytes,
			IsHead:      checkpoint.ID == meta.HeadSavepoint,
		})
	}
	return items, nil
}

func (s *Service) restoreCheckpoint(ctx context.Context, workspace, checkpointID string) error {
	if err := ValidateName("workspace", workspace); err != nil {
		return err
	}
	if err := ValidateName("checkpoint", checkpointID); err != nil {
		return err
	}
	exists, err := s.store.SavepointExists(ctx, workspace, checkpointID)
	if err != nil {
		return err
	}
	if !exists {
		return os.ErrNotExist
	}
	if err := s.store.MoveWorkspaceHead(ctx, workspace, checkpointID, time.Now().UTC()); err != nil {
		if err == redis.TxFailedErr {
			return ErrWorkspaceConflict
		}
		return err
	}
	manifestValue, err := s.store.GetManifest(ctx, workspace, checkpointID)
	if err != nil {
		return err
	}
	if err := SyncWorkspaceRoot(ctx, s.store, workspace, manifestValue); err != nil {
		return err
	}
	return s.store.Audit(ctx, workspace, "checkpoint_restore", map[string]any{
		"checkpoint": checkpointID,
		"mode":       "canonical-only",
	})
}

func (s *Service) saveCheckpoint(ctx context.Context, input SaveCheckpointRequest) (bool, error) {
	if err := ValidateName("workspace", input.Workspace); err != nil {
		return false, err
	}
	if err := ValidateName("checkpoint", input.CheckpointID); err != nil {
		return false, err
	}
	if err := ValidateName("checkpoint", input.ExpectedHead); err != nil {
		return false, err
	}

	headManifest, err := s.store.GetManifest(ctx, input.Workspace, input.ExpectedHead)
	if err != nil {
		return false, err
	}
	if manifestEquivalent(headManifest, input.Manifest) {
		return false, nil
	}

	now := time.Now().UTC()
	manifestHash, err := HashManifest(input.Manifest)
	if err != nil {
		return false, err
	}
	if err := s.store.SaveBlobs(ctx, input.Workspace, input.Blobs); err != nil {
		return false, err
	}

	savepointMeta := SavepointMeta{
		Version:         formatVersion,
		ID:              input.CheckpointID,
		Name:            input.CheckpointID,
		Description:     "",
		Author:          "afs",
		Workspace:       input.Workspace,
		ParentSavepoint: input.ExpectedHead,
		ManifestHash:    manifestHash,
		CreatedAt:       now,
		FileCount:       input.FileCount,
		DirCount:        input.DirCount,
		TotalBytes:      input.TotalBytes,
	}

	err = s.store.rdb.Watch(ctx, func(tx *redis.Tx) error {
		current, err := getJSON[WorkspaceMeta](ctx, tx, workspaceMetaKey(input.Workspace))
		if err != nil {
			return err
		}
		if current.HeadSavepoint != input.ExpectedHead {
			return ErrWorkspaceConflict
		}
		exists, err := tx.Exists(ctx, savepointMetaKey(input.Workspace, input.CheckpointID)).Result()
		if err != nil {
			return err
		}
		if exists > 0 {
			return fmt.Errorf("savepoint %q already exists", input.CheckpointID)
		}

		updatedRefs := map[string]blobRef{}
		for blobID, size := range manifestBlobRefs(input.Manifest) {
			ref, err := getJSON[blobRef](ctx, tx, blobRefKey(input.Workspace, blobID))
			if err != nil {
				if !errors.Is(err, os.ErrNotExist) {
					return err
				}
				ref = blobRef{
					BlobID:    blobID,
					Size:      size,
					CreatedAt: now,
				}
			}
			ref.RefCount++
			if ref.Size == 0 {
				ref.Size = size
			}
			updatedRefs[blobID] = ref
		}

		current.HeadSavepoint = input.CheckpointID
		current.UpdatedAt = now
		current.DirtyHint = false

		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			if err := setJSON(ctx, pipe, savepointMetaKey(input.Workspace, input.CheckpointID), savepointMeta); err != nil {
				return err
			}
			if err := setJSON(ctx, pipe, savepointManifestKey(input.Workspace, input.CheckpointID), input.Manifest); err != nil {
				return err
			}
			if err := setJSON(ctx, pipe, workspaceMetaKey(input.Workspace), current); err != nil {
				return err
			}
			pipe.ZAdd(ctx, workspaceSavepointsKey(input.Workspace), redis.Z{
				Score:  float64(now.UnixMilli()),
				Member: input.CheckpointID,
			})
			for blobID, ref := range updatedRefs {
				if err := setJSON(ctx, pipe, blobRefKey(input.Workspace, blobID), ref); err != nil {
					return err
				}
			}
			return nil
		})
		return err
	}, workspaceMetaKey(input.Workspace))
	if err != nil {
		if errors.Is(err, ErrWorkspaceConflict) || err == redis.TxFailedErr {
			return false, ErrWorkspaceConflict
		}
		return false, err
	}
	if err := SyncWorkspaceRoot(ctx, s.store, input.Workspace, input.Manifest); err != nil {
		return false, err
	}

	if err := s.store.Audit(ctx, input.Workspace, "save", map[string]any{
		"savepoint": input.CheckpointID,
		"parent":    savepointMeta.ParentSavepoint,
		"files":     input.FileCount,
		"dirs":      input.DirCount,
		"bytes":     input.TotalBytes,
	}); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) forkWorkspace(ctx context.Context, sourceWorkspace, newWorkspace string) error {
	if err := ValidateName("workspace", sourceWorkspace); err != nil {
		return err
	}
	if err := ValidateName("workspace", newWorkspace); err != nil {
		return err
	}
	exists, err := s.store.WorkspaceExists(ctx, newWorkspace)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("workspace %q already exists", newWorkspace)
	}

	sourceMeta, err := s.store.GetWorkspaceMeta(ctx, sourceWorkspace)
	if err != nil {
		return err
	}
	sourceMeta = applyWorkspaceMetaDefaults(s.cfg, sourceMeta)
	sourceManifest, err := s.store.GetManifest(ctx, sourceWorkspace, sourceMeta.HeadSavepoint)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	newManifest := cloneManifest(sourceManifest)
	newManifest.Workspace = newWorkspace
	newManifest.Savepoint = initialCheckpointName
	manifestHash, err := HashManifest(newManifest)
	if err != nil {
		return err
	}

	blobs := map[string][]byte{}
	for blobID := range manifestBlobRefs(sourceManifest) {
		data, err := s.store.GetBlob(ctx, sourceWorkspace, blobID)
		if err != nil {
			return err
		}
		blobs[blobID] = data
	}
	if err := s.store.SaveBlobs(ctx, newWorkspace, blobs); err != nil {
		return err
	}
	if err := s.store.AddBlobRefs(ctx, newWorkspace, newManifest, now); err != nil {
		return err
	}

	stats := manifestStats(newManifest)
	workspaceMeta := WorkspaceMeta{
		Version:          formatVersion,
		Name:             newWorkspace,
		Description:      sourceMeta.Description,
		DatabaseID:       sourceMeta.DatabaseID,
		DatabaseName:     sourceMeta.DatabaseName,
		CloudAccount:     sourceMeta.CloudAccount,
		Region:           sourceMeta.Region,
		Source:           workspaceSource(sourceMeta),
		Tags:             append([]string(nil), sourceMeta.Tags...),
		CreatedAt:        now,
		UpdatedAt:        now,
		HeadSavepoint:    initialCheckpointName,
		DefaultSavepoint: initialCheckpointName,
	}
	checkpointMeta := SavepointMeta{
		Version:      formatVersion,
		ID:           initialCheckpointName,
		Name:         initialCheckpointName,
		Description:  "Forked from " + sourceWorkspace + ".",
		Author:       "afs",
		Workspace:    newWorkspace,
		ManifestHash: manifestHash,
		CreatedAt:    now,
		FileCount:    stats.FileCount,
		DirCount:     stats.DirCount,
		TotalBytes:   stats.TotalBytes,
	}

	if err := s.store.PutWorkspaceMeta(ctx, workspaceMeta); err != nil {
		return err
	}
	if err := s.store.PutSavepoint(ctx, checkpointMeta, newManifest); err != nil {
		return err
	}
	if err := SyncWorkspaceRoot(ctx, s.store, newWorkspace, newManifest); err != nil {
		return err
	}
	return s.store.Audit(ctx, newWorkspace, "workspace_fork", map[string]any{
		"source_workspace":  sourceWorkspace,
		"source_checkpoint": sourceMeta.HeadSavepoint,
	})
}

func (s *Service) getTree(ctx context.Context, workspace, rawView, rawPath string, depth int) (treeResponse, error) {
	if depth <= 0 {
		depth = 1
	}
	view, err := parseViewRef(rawView)
	if err != nil {
		return treeResponse{}, err
	}
	normalizedPath, err := normalizeManifestPath(rawPath)
	if err != nil {
		return treeResponse{}, err
	}
	_, checkpoint, manifestValue, err := s.resolveManifestForView(ctx, workspace, view)
	if err != nil {
		return treeResponse{}, err
	}
	entry, ok := manifestValue.Entries[normalizedPath]
	if !ok {
		return treeResponse{}, os.ErrNotExist
	}
	if entry.Type != "dir" {
		return treeResponse{}, fmt.Errorf("path %q is not a directory", normalizedPath)
	}
	items := make([]treeItem, 0)
	for manifestPath, child := range manifestValue.Entries {
		if manifestPath == normalizedPath || manifestPath == "/" {
			continue
		}
		if !strings.HasPrefix(manifestPath, normalizedPathPrefix(normalizedPath)) {
			continue
		}
		relativeDepth := manifestRelativeDepth(normalizedPath, manifestPath)
		if relativeDepth <= 0 || relativeDepth > depth {
			continue
		}
		if depth == 1 && manifestParentPath(manifestPath) != normalizedPath {
			continue
		}
		items = append(items, treeItem{
			Path:       manifestPath,
			Name:       manifestItemName(manifestPath),
			Kind:       child.Type,
			Size:       child.Size,
			ModifiedAt: manifestTimestamp(child),
			Target:     child.Target,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Kind != items[j].Kind {
			if items[i].Kind == "dir" {
				return true
			}
			if items[j].Kind == "dir" {
				return false
			}
		}
		return items[i].Path < items[j].Path
	})
	return treeResponse{
		WorkspaceID: workspace,
		View:        viewName(view, checkpoint.ID),
		Path:        normalizedPath,
		Items:       items,
	}, nil
}

func (s *Service) getFileContent(ctx context.Context, workspace, rawView, rawPath string) (fileContentResponse, error) {
	view, err := parseViewRef(rawView)
	if err != nil {
		return fileContentResponse{}, err
	}
	normalizedPath, err := normalizeManifestPath(rawPath)
	if err != nil {
		return fileContentResponse{}, err
	}
	_, checkpoint, manifestValue, err := s.resolveManifestForView(ctx, workspace, view)
	if err != nil {
		return fileContentResponse{}, err
	}
	entry, ok := manifestValue.Entries[normalizedPath]
	if !ok {
		return fileContentResponse{}, os.ErrNotExist
	}
	if entry.Type == "dir" {
		return fileContentResponse{}, fmt.Errorf("path %q is a directory", normalizedPath)
	}

	response := fileContentResponse{
		WorkspaceID: workspace,
		View:        viewName(view, checkpoint.ID),
		Path:        normalizedPath,
		Kind:        entry.Type,
		Revision:    fmt.Sprintf("%s:%s", checkpoint.ManifestHash, normalizedPath),
		Language:    language(normalizedPath),
		Encoding:    "utf-8",
		ContentType: contentType(normalizedPath, entry.Type),
		Size:        entry.Size,
		ModifiedAt:  manifestTimestamp(entry),
	}

	switch entry.Type {
	case "symlink":
		response.Target = entry.Target
		response.Content = entry.Target
		return response, nil
	case "file":
		data, err := ManifestEntryData(entry, func(blobID string) ([]byte, error) {
			return s.store.GetBlob(ctx, workspace, blobID)
		})
		if err != nil {
			return fileContentResponse{}, err
		}
		if isBinary(data) {
			response.Binary = true
			response.Encoding = ""
			return response, nil
		}
		response.Content = string(data)
		return response, nil
	default:
		return fileContentResponse{}, fmt.Errorf("unsupported manifest entry type %q", entry.Type)
	}
}

func (s *Service) listWorkspaceActivity(ctx context.Context, workspace string, limit int) (activityListResponse, error) {
	if limit <= 0 {
		limit = 50
	}
	meta, err := s.store.GetWorkspaceMeta(ctx, workspace)
	if err != nil {
		return activityListResponse{}, err
	}
	records, err := s.store.ListAudit(ctx, workspace, int64(limit))
	if err != nil {
		return activityListResponse{}, err
	}
	items := make([]activityEvent, 0, len(records))
	for _, record := range records {
		items = append(items, activityFromAudit(meta.Name, record))
	}
	return activityListResponse{Items: items}, nil
}

func (s *Service) listGlobalActivity(ctx context.Context, limit int) (activityListResponse, error) {
	if limit <= 0 {
		limit = 50
	}
	metas, err := s.store.ListWorkspaces(ctx)
	if err != nil {
		return activityListResponse{}, err
	}
	items := make([]activityEvent, 0, len(metas))
	for _, meta := range metas {
		records, err := s.store.ListAudit(ctx, meta.Name, int64(limit))
		if err != nil {
			return activityListResponse{}, err
		}
		for _, record := range records {
			items = append(items, activityFromAudit(meta.Name, record))
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt > items[j].CreatedAt })
	if len(items) > limit {
		items = items[:limit]
	}
	return activityListResponse{Items: items}, nil
}

func (s *Service) buildWorkspaceSummary(ctx context.Context, meta WorkspaceMeta) (workspaceSummary, error) {
	meta = applyWorkspaceMetaDefaults(s.cfg, meta)
	checkpoints, err := s.store.ListSavepoints(ctx, meta.Name, 0)
	if err != nil {
		return workspaceSummary{}, err
	}
	headMeta, err := s.store.GetSavepointMeta(ctx, meta.Name, meta.HeadSavepoint)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return workspaceSummary{}, err
	}
	lastCheckpointAt := meta.UpdatedAt.UTC().Format(time.RFC3339)
	if len(checkpoints) > 0 {
		lastCheckpointAt = checkpoints[0].CreatedAt.UTC().Format(time.RFC3339)
	}
	return workspaceSummary{
		ID:               meta.Name,
		Name:             meta.Name,
		CloudAccount:     meta.CloudAccount,
		DatabaseID:       meta.DatabaseID,
		DatabaseName:     meta.DatabaseName,
		RedisKey:         WorkspaceFSKey(meta.Name),
		Status:           workspaceStatus(meta),
		FileCount:        headMeta.FileCount,
		FolderCount:      headMeta.DirCount,
		TotalBytes:       headMeta.TotalBytes,
		CheckpointCount:  len(checkpoints),
		DraftState:       draftState(meta),
		LastCheckpointAt: lastCheckpointAt,
		UpdatedAt:        meta.UpdatedAt.UTC().Format(time.RFC3339),
		Region:           meta.Region,
		Source:           workspaceSource(meta),
	}, nil
}

func (s *Service) resolveManifestForView(ctx context.Context, workspace string, view viewRef) (WorkspaceMeta, SavepointMeta, Manifest, error) {
	meta, err := s.store.GetWorkspaceMeta(ctx, workspace)
	if err != nil {
		return WorkspaceMeta{}, SavepointMeta{}, Manifest{}, err
	}

	savepointID := meta.HeadSavepoint
	switch view.Kind {
	case "head":
	case "checkpoint":
		savepointID = view.CheckpointID
	case "working-copy":
		return WorkspaceMeta{}, SavepointMeta{}, Manifest{}, ErrUnsupportedView
	default:
		return WorkspaceMeta{}, SavepointMeta{}, Manifest{}, fmt.Errorf("unsupported workspace view %q", view.Kind)
	}

	checkpoint, err := s.store.GetSavepointMeta(ctx, workspace, savepointID)
	if err != nil {
		return WorkspaceMeta{}, SavepointMeta{}, Manifest{}, err
	}
	manifestValue, err := s.store.GetManifest(ctx, workspace, savepointID)
	if err != nil {
		return WorkspaceMeta{}, SavepointMeta{}, Manifest{}, err
	}
	return applyWorkspaceMetaDefaults(s.cfg, meta), checkpoint, manifestValue, nil
}

type workspaceCreateSpec struct {
	Description  string
	DatabaseID   string
	DatabaseName string
	CloudAccount string
	Region       string
	Source       string
	Tags         []string
}

func createWorkspaceWithMetadata(ctx context.Context, cfg Config, store *Store, workspace string, spec workspaceCreateSpec) error {
	now := time.Now().UTC()
	metaDefaults := applyWorkspaceMetaDefaults(cfg, WorkspaceMeta{
		Name:         workspace,
		Description:  spec.Description,
		DatabaseID:   spec.DatabaseID,
		DatabaseName: spec.DatabaseName,
		CloudAccount: spec.CloudAccount,
		Region:       spec.Region,
		Source:       spec.Source,
		Tags:         append([]string(nil), spec.Tags...),
	})
	rootManifest := Manifest{
		Version:   formatVersion,
		Workspace: workspace,
		Savepoint: initialCheckpointName,
		Entries: map[string]ManifestEntry{
			"/": {
				Type:    "dir",
				Mode:    0o755,
				MtimeMs: now.UnixMilli(),
				Size:    0,
			},
		},
	}
	manifestHash, err := HashManifest(rootManifest)
	if err != nil {
		return err
	}
	workspaceMeta := WorkspaceMeta{
		Version:          formatVersion,
		Name:             workspace,
		Description:      metaDefaults.Description,
		DatabaseID:       metaDefaults.DatabaseID,
		DatabaseName:     metaDefaults.DatabaseName,
		CloudAccount:     metaDefaults.CloudAccount,
		Region:           metaDefaults.Region,
		Source:           workspaceSource(metaDefaults),
		Tags:             append([]string(nil), metaDefaults.Tags...),
		CreatedAt:        now,
		UpdatedAt:        now,
		HeadSavepoint:    initialCheckpointName,
		DefaultSavepoint: initialCheckpointName,
	}
	checkpointMeta := SavepointMeta{
		Version:      formatVersion,
		ID:           initialCheckpointName,
		Name:         initialCheckpointName,
		Description:  "Initial workspace state.",
		Author:       "afs",
		Workspace:    workspace,
		ManifestHash: manifestHash,
		CreatedAt:    now,
		FileCount:    0,
		DirCount:     0,
		TotalBytes:   0,
	}
	if err := store.PutWorkspaceMeta(ctx, workspaceMeta); err != nil {
		return err
	}
	if err := store.PutSavepoint(ctx, checkpointMeta, rootManifest); err != nil {
		return err
	}
	if err := SyncWorkspaceRoot(ctx, store, workspace, rootManifest); err != nil {
		return err
	}
	return store.Audit(ctx, workspace, "workspace_create", map[string]any{
		"checkpoint": initialCheckpointName,
		"source":     workspaceMeta.Source,
	})
}

func applyWorkspaceMetaDefaults(cfg Config, meta WorkspaceMeta) WorkspaceMeta {
	defaultDatabaseName := strings.TrimSpace(cfg.RedisAddr)
	if defaultDatabaseName == "" {
		defaultDatabaseName = "direct-redis"
	}
	defaultDatabaseID := "redis-" + slugify(fmt.Sprintf("%s-%d", defaultDatabaseName, cfg.RedisDB))
	if defaultDatabaseID == "redis-" {
		defaultDatabaseID = "redis-direct"
	}
	if strings.TrimSpace(meta.DatabaseID) == "" {
		meta.DatabaseID = defaultDatabaseID
	}
	if strings.TrimSpace(meta.DatabaseName) == "" {
		meta.DatabaseName = defaultDatabaseName
	}
	if strings.TrimSpace(meta.CloudAccount) == "" {
		meta.CloudAccount = "Direct Redis"
	}
	if strings.TrimSpace(meta.Source) == "" {
		meta.Source = sourceBlank
	}
	if meta.Tags == nil {
		meta.Tags = workspaceTags(strings.TrimSpace(meta.Region), strings.TrimSpace(meta.Source))
	}
	return meta
}

func workspaceTags(region, source string) []string {
	tags := make([]string, 0, 2)
	if region != "" {
		tags = append(tags, region)
	}
	switch source {
	case sourceGitImport:
		tags = append(tags, "Git import")
	case sourceCloudImport:
		tags = append(tags, "Redis Cloud import")
	case sourceBlank:
		tags = append(tags, "Blank workspace")
	}
	return tags
}

func workspaceSource(meta WorkspaceMeta) string {
	switch strings.TrimSpace(meta.Source) {
	case sourceGitImport, sourceCloudImport, sourceBlank:
		return strings.TrimSpace(meta.Source)
	default:
		return sourceBlank
	}
}

func workspaceStatus(meta WorkspaceMeta) string {
	if meta.DirtyHint {
		return "attention"
	}
	return "healthy"
}

func draftState(meta WorkspaceMeta) string {
	if meta.DirtyHint {
		return "dirty"
	}
	return "clean"
}

func defaultCapabilities() capabilities {
	return capabilities{
		BrowseHead:        true,
		BrowseCheckpoints: true,
		BrowseWorkingCopy: false,
		EditWorkingCopy:   false,
		CreateCheckpoint:  false,
		RestoreCheckpoint: true,
	}
}

func parseViewRef(raw string) (viewRef, error) {
	view := strings.TrimSpace(raw)
	if view == "" || view == "head" {
		return viewRef{Kind: "head"}, nil
	}
	if strings.HasPrefix(view, "checkpoint:") {
		checkpointID := strings.TrimPrefix(view, "checkpoint:")
		if err := ValidateName("checkpoint", checkpointID); err != nil {
			return viewRef{}, err
		}
		return viewRef{Kind: "checkpoint", CheckpointID: checkpointID}, nil
	}
	if view == "working-copy" {
		return viewRef{Kind: "working-copy"}, nil
	}
	return viewRef{}, fmt.Errorf("unsupported workspace view %q", view)
}

func viewName(view viewRef, checkpointID string) string {
	if view.Kind == "checkpoint" {
		return "checkpoint:" + checkpointID
	}
	return view.Kind
}

func normalizeManifestPath(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "/", nil
	}
	cleaned := path.Clean("/" + strings.TrimPrefix(trimmed, "/"))
	if !strings.HasPrefix(cleaned, "/") {
		return "", fmt.Errorf("invalid path %q", raw)
	}
	return cleaned, nil
}

func normalizedPathPrefix(p string) string {
	if p == "/" {
		return "/"
	}
	return p + "/"
}

func manifestParentPath(p string) string {
	if p == "/" {
		return "/"
	}
	parent := path.Dir(p)
	if parent == "." {
		return "/"
	}
	return parent
}

func manifestItemName(p string) string {
	if p == "/" {
		return "/"
	}
	return path.Base(p)
}

func manifestRelativeDepth(parentPath, childPath string) int {
	if parentPath == childPath {
		return 0
	}
	parentSegments := pathSegments(parentPath)
	childSegments := pathSegments(childPath)
	if len(childSegments) < len(parentSegments) {
		return -1
	}
	for i := range parentSegments {
		if parentSegments[i] != childSegments[i] {
			return -1
		}
	}
	return len(childSegments) - len(parentSegments)
}

func pathSegments(p string) []string {
	trimmed := strings.Trim(strings.TrimSpace(p), "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func manifestTimestamp(entry ManifestEntry) string {
	if entry.MtimeMs == 0 {
		return ""
	}
	return time.UnixMilli(entry.MtimeMs).UTC().Format(time.RFC3339)
}

func cloneManifest(source Manifest) Manifest {
	cloned := Manifest{
		Version:   source.Version,
		Workspace: source.Workspace,
		Savepoint: source.Savepoint,
		Entries:   make(map[string]ManifestEntry, len(source.Entries)),
	}
	for p, entry := range source.Entries {
		cloned.Entries[p] = entry
	}
	return cloned
}

func manifestEquivalent(a, b Manifest) bool {
	if len(a.Entries) != len(b.Entries) {
		return false
	}
	for p, left := range a.Entries {
		right, ok := b.Entries[p]
		if !ok {
			return false
		}
		if !manifestEntryEquivalent(left, right) {
			return false
		}
	}
	return true
}

func manifestEntryEquivalent(a, b ManifestEntry) bool {
	if a.Type != b.Type || a.Mode != b.Mode || a.Size != b.Size || a.BlobID != b.BlobID || a.Inline != b.Inline || a.Target != b.Target {
		return false
	}
	if a.Type == "symlink" || a.Type == "dir" {
		return true
	}
	return a.MtimeMs == b.MtimeMs
}

func manifestBlobRefs(m Manifest) map[string]int64 {
	refs := map[string]int64{}
	for _, entry := range m.Entries {
		if entry.BlobID == "" {
			continue
		}
		refs[entry.BlobID] = entry.Size
	}
	return refs
}

type manifestStatTotals struct {
	FileCount  int
	DirCount   int
	TotalBytes int64
}

func manifestStats(m Manifest) manifestStatTotals {
	var stats manifestStatTotals
	for p, entry := range m.Entries {
		if p == "/" {
			continue
		}
		switch entry.Type {
		case "file":
			stats.FileCount++
			stats.TotalBytes += entry.Size
		case "dir":
			stats.DirCount++
		}
	}
	return stats
}

func language(p string) string {
	switch strings.ToLower(path.Ext(p)) {
	case ".md":
		return "markdown"
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "javascript"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".sh":
		return "shell"
	case ".py":
		return "python"
	default:
		return "text"
	}
}

func contentType(p, kind string) string {
	if kind == "symlink" {
		return "text/plain"
	}
	switch strings.ToLower(path.Ext(p)) {
	case ".md":
		return "text/markdown"
	case ".json":
		return "application/json"
	case ".yaml", ".yml":
		return "application/yaml"
	default:
		return "text/plain"
	}
}

func isBinary(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	if !utf8.Valid(data) {
		return true
	}
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return false
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func activityFromAudit(workspace string, record auditRecord) activityEvent {
	title := "Workspace event"
	detail := "AFS recorded a workspace event."
	kind := record.Op
	scope := "workspace"

	switch record.Op {
	case "workspace_create":
		title = "Created workspace"
		detail = fmt.Sprintf("Initialized workspace %s with checkpoint %s.", workspace, defaultString(record.Fields["checkpoint"], initialCheckpointName))
	case "import":
		title = "Imported workspace"
		detail = fmt.Sprintf("Imported content into %s from %s.", workspace, defaultString(record.Fields["source"], "a local directory"))
	case "save":
		title = "Created checkpoint " + defaultString(record.Fields["savepoint"], "savepoint")
		detail = fmt.Sprintf("Captured %s files and %s directories into a new checkpoint.", defaultString(record.Fields["files"], "0"), defaultString(record.Fields["dirs"], "0"))
		kind = "checkpoint.created"
		scope = "checkpoint"
	case "checkpoint_restore":
		title = "Restored checkpoint " + defaultString(record.Fields["checkpoint"], "checkpoint")
		detail = "Moved the workspace head to a saved checkpoint."
		kind = "checkpoint.restored"
		scope = "checkpoint"
	case "workspace_fork":
		title = "Forked workspace"
		detail = fmt.Sprintf("Created this workspace from %s at checkpoint %s.", defaultString(record.Fields["source_workspace"], "another workspace"), defaultString(record.Fields["source_checkpoint"], initialCheckpointName))
	case "run_start":
		title = "Started command"
		detail = fmt.Sprintf("Ran %s inside the materialized workspace.", defaultString(record.Fields["argv"], "a command"))
		kind = "process.started"
		scope = "process"
	case "run_exit":
		title = "Finished command"
		detail = fmt.Sprintf("Process exited with code %s.", defaultString(record.Fields["exit_code"], "0"))
		kind = "process.finished"
		scope = "process"
	}

	return activityEvent{
		ID:            record.ID,
		WorkspaceID:   workspace,
		WorkspaceName: workspace,
		Actor:         "afs",
		CreatedAt:     record.CreatedAt.UTC().Format(time.RFC3339),
		Detail:        detail,
		Kind:          kind,
		Scope:         scope,
		Title:         title,
	}
}
