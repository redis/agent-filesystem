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
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	afsclient "github.com/redis/agent-filesystem/mount/client"
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

const (
	workspaceSessionStateStarting = "starting"
	workspaceSessionStateActive   = "active"
	workspaceSessionStateStale    = "stale"
	workspaceSessionStateClosed   = "closed"
)

const (
	workspaceSessionHeartbeatInterval = 20 * time.Second
	workspaceSessionLeaseTTL          = 65 * time.Second
	workspaceSessionRecordTTL         = 24 * time.Hour
)

var ErrUnsupportedView = errors.New("control plane operation is not available for this workspace view")
var ErrWorkspaceConflict = errors.New("control plane workspace conflict")
var ErrAmbiguousWorkspace = errors.New("control plane workspace is ambiguous")

type capabilities struct {
	BrowseHead        bool `json:"browse_head"`
	BrowseCheckpoints bool `json:"browse_checkpoints"`
	BrowseWorkingCopy bool `json:"browse_working_copy"`
	EditWorkingCopy   bool `json:"edit_working_copy"`
	CreateCheckpoint  bool `json:"create_checkpoint"`
	RestoreCheckpoint bool `json:"restore_checkpoint"`
}

type workspaceSummary struct {
	ID                     string `json:"id"`
	Name                   string `json:"name"`
	CloudAccount           string `json:"cloud_account"`
	DatabaseID             string `json:"database_id"`
	DatabaseName           string `json:"database_name"`
	OwnerSubject           string `json:"owner_subject,omitempty"`
	OwnerLabel             string `json:"owner_label,omitempty"`
	DatabaseManagementType string `json:"database_management_type,omitempty"`
	DatabaseCanEdit        bool   `json:"database_can_edit"`
	DatabaseCanDelete      bool   `json:"database_can_delete"`
	RedisKey               string `json:"redis_key"`
	Status                 string `json:"status"`
	FileCount              int    `json:"file_count"`
	FolderCount            int    `json:"folder_count"`
	TotalBytes             int64  `json:"total_bytes"`
	CheckpointCount        int    `json:"checkpoint_count"`
	DraftState             string `json:"draft_state"`
	LastCheckpointAt       string `json:"last_checkpoint_at"`
	UpdatedAt              string `json:"updated_at"`
	Region                 string `json:"region"`
	Source                 string `json:"source"`
	TemplateSlug           string `json:"template_slug,omitempty"`
}

type workspaceListResponse struct {
	Items []workspaceSummary `json:"items"`
}

type checkpointSummary struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Author             string `json:"author,omitempty"`
	Description        string `json:"description,omitempty"`
	Note               string `json:"note,omitempty"`
	Kind               string `json:"kind,omitempty"`
	Source             string `json:"source,omitempty"`
	CreatedBy          string `json:"created_by,omitempty"`
	SessionID          string `json:"session_id,omitempty"`
	AgentID            string `json:"agent_id,omitempty"`
	AgentName          string `json:"agent_name,omitempty"`
	ParentCheckpointID string `json:"parent_checkpoint_id,omitempty"`
	ManifestHash       string `json:"manifest_hash,omitempty"`
	CreatedAt          string `json:"created_at"`
	FileCount          int    `json:"file_count"`
	FolderCount        int    `json:"folder_count"`
	TotalBytes         int64  `json:"total_bytes"`
	IsHead             bool   `json:"is_head"`
}

type checkpointDetail struct {
	WorkspaceID        string             `json:"workspace_id"`
	WorkspaceName      string             `json:"workspace_name,omitempty"`
	ID                 string             `json:"id"`
	Name               string             `json:"name"`
	Author             string             `json:"author,omitempty"`
	Description        string             `json:"description,omitempty"`
	Note               string             `json:"note,omitempty"`
	Kind               string             `json:"kind,omitempty"`
	Source             string             `json:"source,omitempty"`
	CreatedBy          string             `json:"created_by,omitempty"`
	SessionID          string             `json:"session_id,omitempty"`
	AgentID            string             `json:"agent_id,omitempty"`
	AgentName          string             `json:"agent_name,omitempty"`
	ParentCheckpointID string             `json:"parent_checkpoint_id,omitempty"`
	Parent             *checkpointSummary `json:"parent,omitempty"`
	ManifestHash       string             `json:"manifest_hash,omitempty"`
	CreatedAt          string             `json:"created_at"`
	FileCount          int                `json:"file_count"`
	FolderCount        int                `json:"folder_count"`
	TotalBytes         int64              `json:"total_bytes"`
	IsHead             bool               `json:"is_head"`
	ChangeSummary      DiffSummary        `json:"change_summary"`
}

type activityEvent struct {
	ID            string `json:"id"`
	WorkspaceID   string `json:"workspace_id,omitempty"`
	WorkspaceName string `json:"workspace_name,omitempty"`
	DatabaseID    string `json:"database_id,omitempty"`
	DatabaseName  string `json:"database_name,omitempty"`
	Actor         string `json:"actor"`
	CreatedAt     string `json:"created_at"`
	Detail        string `json:"detail"`
	Kind          string `json:"kind"`
	Scope         string `json:"scope"`
	Title         string `json:"title"`
}

type activityListRequest struct {
	Until string
	Limit int
}

type activityListResponse struct {
	Items      []activityEvent `json:"items"`
	NextCursor string          `json:"next_cursor,omitempty"`
}

type workspaceDetail struct {
	ID                     string                  `json:"id"`
	Name                   string                  `json:"name"`
	Description            string                  `json:"description,omitempty"`
	CloudAccount           string                  `json:"cloud_account"`
	DatabaseID             string                  `json:"database_id"`
	DatabaseName           string                  `json:"database_name"`
	DatabaseSupportsArrays *bool                   `json:"database_supports_arrays,omitempty"`
	OwnerSubject           string                  `json:"owner_subject,omitempty"`
	OwnerLabel             string                  `json:"owner_label,omitempty"`
	DatabaseManagementType string                  `json:"database_management_type,omitempty"`
	DatabaseCanEdit        bool                    `json:"database_can_edit"`
	DatabaseCanDelete      bool                    `json:"database_can_delete"`
	RedisKey               string                  `json:"redis_key"`
	Region                 string                  `json:"region"`
	Status                 string                  `json:"status"`
	Source                 string                  `json:"source"`
	TemplateSlug           string                  `json:"template_slug,omitempty"`
	CreatedAt              string                  `json:"created_at"`
	UpdatedAt              string                  `json:"updated_at"`
	DraftState             string                  `json:"draft_state"`
	HeadCheckpointID       string                  `json:"head_checkpoint_id"`
	Tags                   []string                `json:"tags,omitempty"`
	FileCount              int                     `json:"file_count"`
	FolderCount            int                     `json:"folder_count"`
	TotalBytes             int64                   `json:"total_bytes"`
	ContentStorage         workspaceContentStorage `json:"content_storage"`
	SearchIndex            workspaceSearchIndex    `json:"search_index"`
	CheckpointCount        int                     `json:"checkpoint_count"`
	Checkpoints            []checkpointSummary     `json:"checkpoints"`
	Activity               []activityEvent         `json:"activity"`
	Capabilities           capabilities            `json:"capabilities"`
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
	TemplateSlug string    `json:"template_slug,omitempty"`
}

type updateWorkspaceRequest struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	DatabaseName string `json:"database_name"`
	CloudAccount string `json:"cloud_account"`
	Region       string `json:"region"`
}

type restoreCheckpointRequest struct {
	CheckpointID string `json:"checkpoint_id"`
}

type RestoreCheckpointResult struct {
	Restored                bool   `json:"restored"`
	CheckpointID            string `json:"checkpoint_id"`
	SafetyCheckpointID      string `json:"safety_checkpoint_id,omitempty"`
	SafetyCheckpointCreated bool   `json:"safety_checkpoint_created"`
	WorkspaceID             string `json:"workspace_id,omitempty"`
	WorkspaceName           string `json:"workspace_name,omitempty"`
}

type createWorkspaceSessionRequest struct {
	AgentID         string `json:"agent_id,omitempty"`
	AgentName       string `json:"agent_name,omitempty"`
	SessionName     string `json:"session_name,omitempty"`
	ClientKind      string `json:"client_kind,omitempty"`
	AFSVersion      string `json:"afs_version,omitempty"`
	Hostname        string `json:"hostname,omitempty"`
	OperatingSystem string `json:"os,omitempty"`
	LocalPath       string `json:"local_path,omitempty"`
	Label           string `json:"label,omitempty"`
	Readonly        bool   `json:"readonly,omitempty"`
}

type workspaceSession struct {
	SessionID                string      `json:"session_id,omitempty"`
	Workspace                string      `json:"workspace"`
	DatabaseID               string      `json:"database_id,omitempty"`
	DatabaseName             string      `json:"database_name,omitempty"`
	RedisKey                 string      `json:"redis_key"`
	HeadCheckpointID         string      `json:"head_checkpoint_id"`
	Initialized              bool        `json:"initialized"`
	HeartbeatIntervalSeconds int         `json:"heartbeat_interval_seconds,omitempty"`
	LeaseExpiresAt           string      `json:"lease_expires_at,omitempty"`
	Redis                    RedisConfig `json:"redis"`
}

type workspaceSessionInfo struct {
	SessionID       string `json:"session_id"`
	Workspace       string `json:"workspace"`
	WorkspaceID     string `json:"workspace_id,omitempty"`
	WorkspaceName   string `json:"workspace_name,omitempty"`
	DatabaseID      string `json:"database_id,omitempty"`
	DatabaseName    string `json:"database_name,omitempty"`
	OwnerSubject    string `json:"owner_subject,omitempty"`
	OwnerLabel      string `json:"owner_label,omitempty"`
	AgentID         string `json:"agent_id,omitempty"`
	AgentName       string `json:"agent_name,omitempty"`
	SessionName     string `json:"session_name,omitempty"`
	ClientKind      string `json:"client_kind,omitempty"`
	AFSVersion      string `json:"afs_version,omitempty"`
	Hostname        string `json:"hostname,omitempty"`
	OperatingSystem string `json:"os,omitempty"`
	LocalPath       string `json:"local_path,omitempty"`
	Label           string `json:"label,omitempty"`
	Readonly        bool   `json:"readonly,omitempty"`
	State           string `json:"state"`
	StartedAt       string `json:"started_at"`
	LastSeenAt      string `json:"last_seen_at"`
	LeaseExpiresAt  string `json:"lease_expires_at"`
}

type workspaceSessionListResponse struct {
	Items []workspaceSessionInfo `json:"items"`
}

func workspaceSessionNames(input createWorkspaceSessionRequest) (agentName, sessionName, label string) {
	agentName = strings.TrimSpace(input.AgentName)
	sessionName = strings.TrimSpace(input.SessionName)
	legacyLabel := strings.TrimSpace(input.Label)
	if agentName == "" && sessionName == "" {
		agentName = legacyLabel
	}
	label = sessionName
	if label == "" {
		label = agentName
	}
	if label == "" {
		label = legacyLabel
	}
	return agentName, sessionName, label
}

func workspaceSessionRecordAgentName(record WorkspaceSessionRecord) string {
	if value := strings.TrimSpace(record.AgentName); value != "" {
		return value
	}
	if strings.TrimSpace(record.SessionName) == "" {
		return strings.TrimSpace(record.Label)
	}
	return ""
}

func workspaceSessionRecordSessionName(record WorkspaceSessionRecord) string {
	if value := strings.TrimSpace(record.SessionName); value != "" {
		return value
	}
	if agentName := strings.TrimSpace(record.AgentName); agentName != "" {
		label := strings.TrimSpace(record.Label)
		if label != "" && label != agentName {
			return label
		}
	}
	return ""
}

func workspaceSessionRecordLabel(record WorkspaceSessionRecord) string {
	if value := workspaceSessionRecordSessionName(record); value != "" {
		return value
	}
	if value := workspaceSessionRecordAgentName(record); value != "" {
		return value
	}
	return strings.TrimSpace(record.Label)
}

type viewRef struct {
	Kind         string
	CheckpointID string
}

type Service struct {
	cfg                 Config
	store               *Store
	catalog             catalogStore
	catalogDatabaseID   string
	catalogDatabaseName string
}

type WorkspaceSummary = workspaceSummary
type WorkspaceListResponse = workspaceListResponse
type CheckpointSummary = checkpointSummary
type CheckpointDetail = checkpointDetail
type ActivityEvent = activityEvent
type ActivityListRequest = activityListRequest
type ActivityListResponse = activityListResponse
type WorkspaceDetail = workspaceDetail
type TreeItem = treeItem
type TreeResponse = treeResponse
type FileContentResponse = fileContentResponse
type SourceRef = sourceRef
type CreateWorkspaceRequest = createWorkspaceRequest
type UpdateWorkspaceRequest = updateWorkspaceRequest
type CreateWorkspaceSessionRequest = createWorkspaceSessionRequest
type WorkspaceSession = workspaceSession
type WorkspaceSessionInfo = workspaceSessionInfo
type WorkspaceSessionListResponse = workspaceSessionListResponse

type SaveCheckpointRequest struct {
	Workspace             string
	ExpectedHead          string
	CheckpointID          string
	Description           string
	Kind                  string
	Source                string
	Author                string
	CreatedBy             string
	Manifest              Manifest
	Blobs                 map[string][]byte
	FileCount             int
	DirCount              int
	TotalBytes            int64
	SkipWorkspaceRootSync bool
	AllowUnchanged        bool
}

type SaveCheckpointFromLiveOptions struct {
	Description    string
	Kind           string
	Source         string
	Author         string
	CreatedBy      string
	AllowUnchanged bool
}

type workspaceUsageStats struct {
	FileCount   int
	FolderCount int
	TotalBytes  int64
}

func NewService(cfg Config, store *Store) *Service {
	return &Service{cfg: cfg, store: store}
}

func NewServiceWithCatalog(cfg Config, store *Store, catalog catalogStore, databaseID, databaseName string) *Service {
	return &Service{
		cfg:                 cfg,
		store:               store,
		catalog:             catalog,
		catalogDatabaseID:   strings.TrimSpace(databaseID),
		catalogDatabaseName: strings.TrimSpace(databaseName),
	}
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

func (s *Service) GetWorkspaceVersioningPolicy(ctx context.Context, workspace string) (WorkspaceVersioningPolicy, error) {
	return s.store.GetWorkspaceVersioningPolicy(ctx, workspace)
}

func (s *Service) UpdateWorkspaceVersioningPolicy(ctx context.Context, workspace string, policy WorkspaceVersioningPolicy) (WorkspaceVersioningPolicy, error) {
	normalized := NormalizeWorkspaceVersioningPolicy(policy)
	if err := ValidateWorkspaceVersioningPolicy(normalized); err != nil {
		return WorkspaceVersioningPolicy{}, err
	}
	if err := s.store.PutWorkspaceVersioningPolicy(ctx, workspace, normalized); err != nil {
		return WorkspaceVersioningPolicy{}, err
	}
	return normalized, nil
}

func (s *Service) ListCheckpoints(ctx context.Context, workspace string, limit int) ([]CheckpointSummary, error) {
	return s.listCheckpoints(ctx, workspace, limit)
}

func (s *Service) GetCheckpoint(ctx context.Context, workspace, checkpointID string) (CheckpointDetail, error) {
	return s.getCheckpoint(ctx, workspace, checkpointID)
}

func (s *Service) RestoreCheckpoint(ctx context.Context, workspace, checkpointID string) error {
	_, err := s.restoreCheckpoint(ctx, workspace, checkpointID)
	return err
}

func (s *Service) RestoreCheckpointWithResult(ctx context.Context, workspace, checkpointID string) (RestoreCheckpointResult, error) {
	return s.restoreCheckpoint(ctx, workspace, checkpointID)
}

func (s *Service) SaveCheckpoint(ctx context.Context, input SaveCheckpointRequest) (bool, error) {
	return s.saveCheckpoint(ctx, input)
}

// ListChangelog reads a page of workspace file-change entries. See
// ChangelogListRequest for filter options.
func (s *Service) ListChangelog(ctx context.Context, workspace string, req ChangelogListRequest) (ChangelogListResponse, error) {
	_, storageID, err := s.store.resolveWorkspaceMeta(ctx, workspace)
	if err != nil {
		return ChangelogListResponse{}, err
	}
	return s.store.ListChangelog(ctx, storageID, req)
}

func (s *Service) ListGlobalChangelog(ctx context.Context, req ChangelogListRequest) (ChangelogListResponse, error) {
	return s.listGlobalChangelog(ctx, req)
}

// GetSessionChangelogSummary reads the per-session rollup hash.
func (s *Service) GetSessionChangelogSummary(ctx context.Context, workspace, sessionID string) (SessionChangelogSummary, error) {
	if strings.TrimSpace(sessionID) == "" {
		return SessionChangelogSummary{}, fmt.Errorf("session id is required")
	}
	_, storageID, err := s.store.resolveWorkspaceMeta(ctx, workspace)
	if err != nil {
		return SessionChangelogSummary{}, err
	}
	return s.store.GetSessionChangelogSummary(ctx, storageID, sessionID)
}

// GetPathLastWriter reads the companion path:last hash for a single path.
func (s *Service) GetPathLastWriter(ctx context.Context, workspace, path string) (PathLastWriter, error) {
	if strings.TrimSpace(path) == "" {
		return PathLastWriter{}, fmt.Errorf("path is required")
	}
	_, storageID, err := s.store.resolveWorkspaceMeta(ctx, workspace)
	if err != nil {
		return PathLastWriter{}, err
	}
	return s.store.GetPathLastWriter(ctx, storageID, path)
}

func (s *Service) SaveCheckpointFromLive(ctx context.Context, workspace, checkpointID string) (bool, error) {
	return s.saveCheckpointFromLive(ctx, workspace, checkpointID, SaveCheckpointFromLiveOptions{})
}

// SaveCheckpointFromLiveWithOptions builds a manifest from the live workspace
// root in Redis and saves it as a new checkpoint with caller-provided metadata.
func (s *Service) SaveCheckpointFromLiveWithOptions(ctx context.Context, workspace, checkpointID string, options SaveCheckpointFromLiveOptions) (bool, error) {
	return s.saveCheckpointFromLive(ctx, workspace, checkpointID, options)
}

// saveCheckpointFromLive encapsulates the full server-side flow so that remote
// clients can create checkpoints without direct Redis access.
func (s *Service) saveCheckpointFromLive(ctx context.Context, workspace, checkpointID string, options SaveCheckpointFromLiveOptions) (bool, error) {
	if err := ValidateName("workspace", workspace); err != nil {
		return false, fmt.Errorf("save-from-live validate: %w", err)
	}
	if err := ValidateName("checkpoint", checkpointID); err != nil {
		return false, fmt.Errorf("save-from-live validate checkpoint: %w", err)
	}

	meta, err := s.store.GetWorkspaceMeta(ctx, workspace)
	if err != nil {
		return false, fmt.Errorf("save-from-live get workspace meta %q: %w", workspace, err)
	}
	storageID := workspaceStorageID(meta)

	// Check dirty state — if the workspace root is known-clean, skip.
	if dirty, known, err := WorkspaceRootDirtyState(ctx, s.store, storageID); err != nil {
		return false, fmt.Errorf("save-from-live dirty state: %w", err)
	} else if known && !dirty && !options.AllowUnchanged {
		return false, nil
	}

	// Ensure workspace root is materialized.
	if _, _, _, err := EnsureWorkspaceRoot(ctx, s.store, storageID); err != nil {
		return false, fmt.Errorf("save-from-live ensure workspace root %q (head=%q): %w", workspace, meta.HeadSavepoint, err)
	}

	// Build manifest from the live workspace root.
	manifest, blobs, fileCount, dirCount, totalBytes, err := BuildManifestFromWorkspaceRoot(ctx, s.store.rdb, storageID, checkpointID)
	if err != nil {
		return false, fmt.Errorf("save-from-live build manifest: %w", err)
	}

	saved, err := s.saveCheckpoint(ctx, SaveCheckpointRequest{
		Workspace:             storageID,
		ExpectedHead:          meta.HeadSavepoint,
		CheckpointID:          checkpointID,
		Description:           options.Description,
		Kind:                  options.Kind,
		Source:                options.Source,
		Author:                options.Author,
		CreatedBy:             options.CreatedBy,
		Manifest:              manifest,
		Blobs:                 blobs,
		FileCount:             fileCount,
		DirCount:              dirCount,
		TotalBytes:            totalBytes,
		SkipWorkspaceRootSync: true,
		AllowUnchanged:        options.AllowUnchanged,
	})
	if err != nil {
		return false, fmt.Errorf("save-from-live save checkpoint: %w", err)
	}
	if !saved {
		if err := MarkWorkspaceRootClean(ctx, s.store, storageID, meta.HeadSavepoint); err != nil {
			return false, fmt.Errorf("save-from-live mark clean: %w", err)
		}
	}
	return saved, nil
}

func (s *Service) ForkWorkspace(ctx context.Context, sourceWorkspace, newWorkspace string) error {
	return s.forkWorkspace(ctx, sourceWorkspace, newWorkspace)
}

func (s *Service) CreateWorkspaceSession(ctx context.Context, workspace string, input CreateWorkspaceSessionRequest) (WorkspaceSession, error) {
	redisKey, headSavepoint, initialized, err := EnsureWorkspaceRoot(ctx, s.store, workspace)
	if err != nil {
		return WorkspaceSession{}, err
	}
	meta, err := s.store.GetWorkspaceMeta(ctx, workspace)
	if err != nil {
		return WorkspaceSession{}, err
	}
	storageID := workspaceStorageID(meta)
	session := WorkspaceSession{
		Workspace:        storageID,
		RedisKey:         redisKey,
		HeadCheckpointID: headSavepoint,
		Initialized:      initialized,
		Redis:            s.cfg.RedisConfig,
	}
	if !shouldTrackWorkspaceSession(input) {
		return session, nil
	}

	now, err := s.store.Now(ctx)
	if err != nil {
		return WorkspaceSession{}, err
	}
	if err := s.reapExpiredWorkspaceSessions(ctx, workspace, now); err != nil {
		return WorkspaceSession{}, err
	}
	sessionID, err := newWorkspaceSessionID()
	if err != nil {
		return WorkspaceSession{}, err
	}
	agentName, sessionName, label := workspaceSessionNames(input)
	record := WorkspaceSessionRecord{
		SessionID:       sessionID,
		Workspace:       workspace,
		AgentID:         strings.TrimSpace(input.AgentID),
		AgentName:       agentName,
		SessionName:     sessionName,
		ClientKind:      strings.TrimSpace(input.ClientKind),
		AFSVersion:      strings.TrimSpace(input.AFSVersion),
		Hostname:        strings.TrimSpace(input.Hostname),
		OperatingSystem: strings.TrimSpace(input.OperatingSystem),
		LocalPath:       strings.TrimSpace(input.LocalPath),
		Label:           label,
		Readonly:        input.Readonly,
		State:           workspaceSessionStateStarting,
		StartedAt:       now,
		LastSeenAt:      now,
		LeaseExpiresAt:  now.Add(workspaceSessionLeaseTTL),
	}
	if err := s.store.PutWorkspaceSession(ctx, record); err != nil {
		return WorkspaceSession{}, err
	}
	if err := s.syncWorkspaceSessionCatalog(ctx, workspace, record, ""); err != nil {
		return WorkspaceSession{}, err
	}
	_ = s.store.Audit(ctx, workspace, "session_start", map[string]any{
		"session_id":  record.SessionID,
		"client_kind": defaultString(record.ClientKind, "sync"),
		"hostname":    record.Hostname,
	})
	s.publishSessionMonitorEvent(ctx, workspace, record, "started")

	session.SessionID = record.SessionID
	session.HeartbeatIntervalSeconds = int(workspaceSessionHeartbeatInterval / time.Second)
	session.LeaseExpiresAt = record.LeaseExpiresAt.UTC().Format(time.RFC3339)
	return session, nil
}

func (s *Service) UpsertWorkspaceSession(ctx context.Context, workspace, sessionID string, input CreateWorkspaceSessionRequest) (WorkspaceSessionInfo, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return WorkspaceSessionInfo{}, fmt.Errorf("session id is required")
	}
	if _, _, _, err := EnsureWorkspaceRoot(ctx, s.store, workspace); err != nil {
		return WorkspaceSessionInfo{}, err
	}
	now, err := s.store.Now(ctx)
	if err != nil {
		return WorkspaceSessionInfo{}, err
	}
	if err := s.reapExpiredWorkspaceSessions(ctx, workspace, now); err != nil {
		return WorkspaceSessionInfo{}, err
	}
	record, err := s.store.GetWorkspaceSession(ctx, workspace, sessionID)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return WorkspaceSessionInfo{}, err
	}
	if errors.Is(err, os.ErrNotExist) {
		record = WorkspaceSessionRecord{
			SessionID: sessionID,
			Workspace: workspace,
			StartedAt: now,
		}
	}
	record.ClientKind = strings.TrimSpace(input.ClientKind)
	record.AgentID = strings.TrimSpace(input.AgentID)
	agentName, sessionName, label := workspaceSessionNames(input)
	if agentName != "" {
		record.AgentName = agentName
	}
	if sessionName != "" {
		record.SessionName = sessionName
	}
	if label != "" {
		record.Label = label
	}
	record.AFSVersion = strings.TrimSpace(input.AFSVersion)
	record.Hostname = strings.TrimSpace(input.Hostname)
	record.OperatingSystem = strings.TrimSpace(input.OperatingSystem)
	record.LocalPath = strings.TrimSpace(input.LocalPath)
	record.Readonly = input.Readonly
	record.State = workspaceSessionStateActive
	record.LastSeenAt = now
	record.LeaseExpiresAt = now.Add(workspaceSessionLeaseTTL)
	if err := s.store.PutWorkspaceSession(ctx, record); err != nil {
		return WorkspaceSessionInfo{}, err
	}
	if err := s.syncWorkspaceSessionCatalog(ctx, workspace, record, ""); err != nil {
		return WorkspaceSessionInfo{}, err
	}
	s.publishSessionMonitorEvent(ctx, workspace, record, "upserted")
	return workspaceSessionInfoFromRecord(record), nil
}

func (s *Service) ListWorkspaceSessions(ctx context.Context, workspace string) (WorkspaceSessionListResponse, error) {
	now, err := s.store.Now(ctx)
	if err != nil {
		return WorkspaceSessionListResponse{}, err
	}
	if err := s.reapExpiredWorkspaceSessions(ctx, workspace, now); err != nil {
		return WorkspaceSessionListResponse{}, err
	}
	if items, ok, err := s.listWorkspaceSessionsFromCatalog(ctx, workspace); err != nil {
		return WorkspaceSessionListResponse{}, err
	} else if ok {
		return WorkspaceSessionListResponse{Items: items}, nil
	}
	records, err := s.store.ListWorkspaceSessions(ctx, workspace)
	if err != nil {
		return WorkspaceSessionListResponse{}, err
	}
	items := make([]workspaceSessionInfo, 0, len(records))
	for _, record := range records {
		items = append(items, workspaceSessionInfoFromRecord(record))
	}
	return WorkspaceSessionListResponse{Items: items}, nil
}

func (s *Service) HeartbeatWorkspaceSession(ctx context.Context, workspace, sessionID string, input ...CreateWorkspaceSessionRequest) (WorkspaceSessionInfo, error) {
	record, err := s.store.GetWorkspaceSession(ctx, workspace, sessionID)
	if err != nil {
		return WorkspaceSessionInfo{}, err
	}
	if record.State == workspaceSessionStateClosed {
		return WorkspaceSessionInfo{}, os.ErrNotExist
	}
	now, err := s.store.Now(ctx)
	if err != nil {
		return WorkspaceSessionInfo{}, err
	}
	record.State = workspaceSessionStateActive
	if len(input) > 0 && (shouldTrackWorkspaceSession(input[0]) || input[0].Readonly) {
		record = workspaceSessionRecordWithHeartbeatMetadata(record, input[0])
	}
	record.LastSeenAt = now
	record.LeaseExpiresAt = now.Add(workspaceSessionLeaseTTL)
	if err := s.store.PutWorkspaceSession(ctx, record); err != nil {
		return WorkspaceSessionInfo{}, err
	}
	if err := s.syncWorkspaceSessionCatalog(ctx, workspace, record, ""); err != nil {
		return WorkspaceSessionInfo{}, err
	}
	s.publishSessionMonitorEvent(ctx, workspace, record, "heartbeat")
	return workspaceSessionInfoFromRecord(record), nil
}

func workspaceSessionRecordWithHeartbeatMetadata(record WorkspaceSessionRecord, input CreateWorkspaceSessionRequest) WorkspaceSessionRecord {
	if value := strings.TrimSpace(input.ClientKind); value != "" {
		record.ClientKind = value
	}
	if value := strings.TrimSpace(input.AgentID); value != "" {
		record.AgentID = value
	}
	agentName, sessionName, label := workspaceSessionNames(input)
	if agentName != "" {
		record.AgentName = agentName
	}
	if sessionName != "" {
		record.SessionName = sessionName
	}
	if label != "" {
		record.Label = label
	}
	if value := strings.TrimSpace(input.AFSVersion); value != "" {
		record.AFSVersion = value
	}
	if value := strings.TrimSpace(input.Hostname); value != "" {
		record.Hostname = value
	}
	if value := strings.TrimSpace(input.OperatingSystem); value != "" {
		record.OperatingSystem = value
	}
	if value := strings.TrimSpace(input.LocalPath); value != "" {
		record.LocalPath = value
	}
	record.Readonly = input.Readonly
	return record
}

func (s *Service) CloseWorkspaceSession(ctx context.Context, workspace, sessionID string) error {
	record, err := s.store.GetWorkspaceSession(ctx, workspace, sessionID)
	if err != nil {
		return err
	}
	now, err := s.store.Now(ctx)
	if err != nil {
		return err
	}
	record.State = workspaceSessionStateClosed
	record.LastSeenAt = now
	record.LeaseExpiresAt = now
	if err := s.store.RemoveWorkspaceSessionPresence(ctx, workspace, sessionID); err != nil {
		return err
	}
	if err := s.store.PutWorkspaceSessionWithTTL(ctx, record, workspaceSessionRecordTTL); err != nil {
		return err
	}
	if err := s.syncWorkspaceSessionCatalog(ctx, workspace, record, "explicit"); err != nil {
		return err
	}
	_ = s.store.Audit(ctx, workspace, "session_close", map[string]any{
		"session_id":  record.SessionID,
		"client_kind": defaultString(record.ClientKind, "sync"),
		"hostname":    record.Hostname,
	})
	s.publishSessionMonitorEvent(ctx, workspace, record, "closed")
	return nil
}

func (s *Service) GetTree(ctx context.Context, workspace, rawView, rawPath string, depth int) (TreeResponse, error) {
	return s.getTree(ctx, workspace, rawView, rawPath, depth)
}

func (s *Service) GetFileContent(ctx context.Context, workspace, rawView, rawPath string) (FileContentResponse, error) {
	return s.getFileContent(ctx, workspace, rawView, rawPath)
}

func (s *Service) ListWorkspaceActivity(ctx context.Context, workspace string, limit int) (ActivityListResponse, error) {
	return s.listWorkspaceActivity(ctx, workspace, activityListRequest{Limit: limit})
}

func (s *Service) ListWorkspaceActivityPage(ctx context.Context, workspace string, req ActivityListRequest) (ActivityListResponse, error) {
	return s.listWorkspaceActivity(ctx, workspace, req)
}

func (s *Service) ListGlobalActivity(ctx context.Context, limit int) (ActivityListResponse, error) {
	return s.listGlobalActivity(ctx, activityListRequest{Limit: limit})
}

func (s *Service) ListGlobalActivityPage(ctx context.Context, req ActivityListRequest) (ActivityListResponse, error) {
	return s.listGlobalActivity(ctx, req)
}

func (s *Service) ListWorkspaceEvents(ctx context.Context, workspace string, req EventListRequest) (EventListResponse, error) {
	return s.listWorkspaceEvents(ctx, workspace, req)
}

func (s *Service) ListGlobalEvents(ctx context.Context, req EventListRequest) (EventListResponse, error) {
	return s.listGlobalEvents(ctx, req)
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

func shouldTrackWorkspaceSession(input createWorkspaceSessionRequest) bool {
	return strings.TrimSpace(input.AgentID) != "" ||
		strings.TrimSpace(input.AgentName) != "" ||
		strings.TrimSpace(input.SessionName) != "" ||
		strings.TrimSpace(input.Label) != "" ||
		strings.TrimSpace(input.ClientKind) != "" ||
		strings.TrimSpace(input.Hostname) != "" ||
		strings.TrimSpace(input.OperatingSystem) != "" ||
		strings.TrimSpace(input.LocalPath) != "" ||
		strings.TrimSpace(input.AFSVersion) != ""
}

func workspaceSessionInfoFromRecord(record WorkspaceSessionRecord) workspaceSessionInfo {
	return workspaceSessionInfo{
		SessionID:       record.SessionID,
		Workspace:       record.Workspace,
		AgentID:         record.AgentID,
		AgentName:       workspaceSessionRecordAgentName(record),
		SessionName:     workspaceSessionRecordSessionName(record),
		ClientKind:      record.ClientKind,
		AFSVersion:      record.AFSVersion,
		Hostname:        record.Hostname,
		OperatingSystem: record.OperatingSystem,
		LocalPath:       record.LocalPath,
		Label:           workspaceSessionRecordLabel(record),
		Readonly:        record.Readonly,
		State:           record.State,
		StartedAt:       record.StartedAt.UTC().Format(time.RFC3339),
		LastSeenAt:      record.LastSeenAt.UTC().Format(time.RFC3339),
		LeaseExpiresAt:  record.LeaseExpiresAt.UTC().Format(time.RFC3339),
	}
}

const timeRFC3339 = time.RFC3339

func newWorkspaceSessionID() (string, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return "sess_" + hex.EncodeToString(raw[:]), nil
}

func (s *Service) reapExpiredWorkspaceSessions(ctx context.Context, workspace string, now time.Time) error {
	records, err := s.store.ListWorkspaceSessions(ctx, workspace)
	if err != nil {
		return err
	}
	for _, record := range records {
		alive, err := s.store.WorkspaceSessionLeaseAlive(ctx, workspace, record.SessionID)
		if err != nil {
			return err
		}
		if alive && !record.LeaseExpiresAt.Before(now) {
			continue
		}
		if record.State == workspaceSessionStateClosed {
			if err := s.store.RemoveWorkspaceSessionPresence(ctx, workspace, record.SessionID); err != nil {
				return err
			}
			continue
		}
		record.State = workspaceSessionStateStale
		record.LastSeenAt = now
		if err := s.store.RemoveWorkspaceSessionPresence(ctx, workspace, record.SessionID); err != nil {
			return err
		}
		if err := s.store.PutWorkspaceSessionWithTTL(ctx, record, workspaceSessionRecordTTL); err != nil {
			return err
		}
		if err := s.syncWorkspaceSessionCatalog(ctx, workspace, record, "expired"); err != nil {
			return err
		}
		_ = s.store.Audit(ctx, workspace, "session_stale", map[string]any{
			"session_id":  record.SessionID,
			"client_kind": defaultString(record.ClientKind, "sync"),
			"hostname":    record.Hostname,
		})
		s.publishSessionMonitorEvent(ctx, workspace, record, "stale")
	}
	return nil
}

func (s *Service) listWorkspaceSessionsFromCatalog(ctx context.Context, workspace string) ([]workspaceSessionInfo, bool, error) {
	if s.catalog == nil {
		return nil, false, nil
	}
	route, meta, ok, err := s.resolveWorkspaceCatalogRoute(ctx, workspace)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}
	records, err := s.catalog.ListSessionsForWorkspace(ctx, route.WorkspaceID)
	if err != nil {
		return nil, false, err
	}
	items := make([]workspaceSessionInfo, 0, len(records))
	now := time.Now().UTC()
	for _, record := range records {
		if sessionCatalogRecordLeaseExpired(record, now) {
			_ = s.catalog.UpsertSession(ctx, staleSessionCatalogRecord(record, now, "expired"))
			continue
		}
		items = append(items, workspaceSessionInfo{
			SessionID:       record.SessionID,
			Workspace:       defaultString(record.WorkspaceName, meta.Name),
			AgentID:         record.AgentID,
			AgentName:       record.AgentName,
			SessionName:     record.SessionName,
			ClientKind:      record.ClientKind,
			AFSVersion:      record.AFSVersion,
			Hostname:        record.Hostname,
			OperatingSystem: record.OperatingSystem,
			LocalPath:       record.LocalPath,
			Label:           record.Label,
			Readonly:        record.Readonly,
			State:           record.State,
			StartedAt:       record.StartedAt,
			LastSeenAt:      record.LastSeenAt,
			LeaseExpiresAt:  record.LeaseExpiresAt,
		})
	}
	return items, true, nil
}

func (s *Service) syncWorkspaceSessionCatalog(ctx context.Context, workspace string, record WorkspaceSessionRecord, closeReason string) error {
	if s.catalog == nil {
		return nil
	}
	route, meta, ok, err := s.resolveWorkspaceCatalogRoute(ctx, workspace)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	return s.catalog.UpsertSession(ctx, workspaceSessionCatalogRecord(route, meta, record, closeReason))
}

func (s *Service) resolveWorkspaceCatalogRoute(ctx context.Context, workspace string) (workspaceCatalogRoute, WorkspaceMeta, bool, error) {
	if s.catalog == nil {
		return workspaceCatalogRoute{}, WorkspaceMeta{}, false, nil
	}
	meta, err := s.store.GetWorkspaceMeta(ctx, workspace)
	if err != nil {
		return workspaceCatalogRoute{}, WorkspaceMeta{}, false, err
	}
	meta = applyWorkspaceMetaDefaults(s.cfg, meta)
	databaseID := meta.DatabaseID
	if strings.TrimSpace(s.catalogDatabaseID) != "" {
		databaseID = s.catalogDatabaseID
	}
	route, err := s.catalog.ResolveWorkspaceInDatabase(ctx, databaseID, workspace)
	if err == nil {
		return route, meta, true, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return workspaceCatalogRoute{}, WorkspaceMeta{}, false, err
	}
	detail, err := s.getWorkspace(ctx, workspace)
	if err != nil {
		return workspaceCatalogRoute{}, WorkspaceMeta{}, false, err
	}
	summary := workspaceSummaryFromDetail(detail)
	if strings.TrimSpace(s.catalogDatabaseID) != "" {
		summary.DatabaseID = s.catalogDatabaseID
	}
	if strings.TrimSpace(s.catalogDatabaseName) != "" {
		summary.DatabaseName = s.catalogDatabaseName
	}
	summary, err = s.catalog.UpsertWorkspace(ctx, summary)
	if err != nil {
		return workspaceCatalogRoute{}, WorkspaceMeta{}, false, err
	}
	return workspaceCatalogRoute{
		DatabaseID:  summary.DatabaseID,
		WorkspaceID: summary.ID,
		Name:        summary.Name,
	}, meta, true, nil
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
	storageID := workspaceStorageID(meta)
	checkpoints, err := s.store.ListSavepoints(ctx, storageID, 100)
	if err != nil {
		return workspaceDetail{}, err
	}
	headMeta, err := s.store.GetSavepointMeta(ctx, storageID, meta.HeadSavepoint)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return workspaceDetail{}, err
	}
	stats := s.currentWorkspaceStats(ctx, storageID, headMeta)
	contentStorage, err := inspectWorkspaceContentStorage(ctx, s.store, storageID)
	if err != nil {
		return workspaceDetail{}, err
	}
	databaseSupportsArrays, err := inspectDatabaseArraySupport(ctx, s.store)
	if err != nil {
		return workspaceDetail{}, err
	}
	searchIndex, err := inspectWorkspaceSearchIndex(ctx, s.store.rdb, storageID)
	if err != nil {
		return workspaceDetail{}, err
	}
	dirty, err := s.workspaceDirtyState(ctx, storageID, meta)
	if err != nil {
		return workspaceDetail{}, err
	}
	activity, err := s.listWorkspaceActivity(ctx, storageID, activityListRequest{Limit: 25})
	if err != nil {
		return workspaceDetail{}, err
	}

	detail := workspaceDetail{
		ID:                     storageID,
		Name:                   meta.Name,
		Description:            meta.Description,
		CloudAccount:           meta.CloudAccount,
		DatabaseID:             meta.DatabaseID,
		DatabaseName:           meta.DatabaseName,
		DatabaseSupportsArrays: databaseSupportsArrays,
		RedisKey:               WorkspaceFSKey(storageID),
		Region:                 meta.Region,
		Status:                 workspaceStatus(dirty),
		Source:                 workspaceSource(meta),
		CreatedAt:              meta.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:              meta.UpdatedAt.UTC().Format(time.RFC3339),
		DraftState:             draftState(dirty),
		HeadCheckpointID:       meta.HeadSavepoint,
		Tags:                   append([]string(nil), meta.Tags...),
		FileCount:              stats.FileCount,
		FolderCount:            stats.FolderCount,
		TotalBytes:             stats.TotalBytes,
		ContentStorage:         contentStorage,
		SearchIndex:            searchIndex,
		CheckpointCount:        len(checkpoints),
		Checkpoints:            make([]checkpointSummary, 0, len(checkpoints)),
		Activity:               activity.Items,
		Capabilities:           defaultCapabilities(),
	}

	for _, checkpoint := range checkpoints {
		detail.Checkpoints = append(detail.Checkpoints, checkpointSummaryFromMeta(checkpoint, meta.HeadSavepoint))
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
	detail, err := s.getWorkspace(ctx, workspace)
	if err != nil {
		return workspaceDetail{}, err
	}
	detail.TemplateSlug = strings.TrimSpace(input.TemplateSlug)
	return detail, nil
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
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = strings.TrimSpace(meta.Name)
	}
	if name != meta.Name {
		if err := ValidateName("workspace", name); err != nil {
			return workspaceDetail{}, err
		}
		exists, err := s.store.WorkspaceExists(ctx, name)
		if err != nil {
			return workspaceDetail{}, err
		}
		if exists {
			return workspaceDetail{}, fmt.Errorf("workspace %q already exists", name)
		}
	}

	meta = applyWorkspaceMetaDefaults(s.cfg, meta)
	meta.Name = name
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
		"name":          meta.Name,
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
		items = append(items, checkpointSummaryFromMeta(checkpoint, meta.HeadSavepoint))
	}
	return items, nil
}

func checkpointSummaryFromMeta(checkpoint SavepointMeta, headSavepoint string) checkpointSummary {
	return checkpointSummary{
		ID:                 checkpoint.ID,
		Name:               checkpoint.Name,
		Author:             defaultString(checkpoint.Author, "afs"),
		Description:        checkpoint.Description,
		Note:               checkpoint.Description,
		Kind:               checkpointKind(checkpoint.Kind),
		Source:             checkpointSource(checkpoint.Source),
		CreatedBy:          checkpoint.CreatedBy,
		SessionID:          checkpoint.SessionID,
		AgentID:            checkpoint.AgentID,
		AgentName:          checkpoint.AgentName,
		ParentCheckpointID: checkpoint.ParentSavepoint,
		ManifestHash:       checkpoint.ManifestHash,
		CreatedAt:          checkpoint.CreatedAt.UTC().Format(time.RFC3339),
		FileCount:          checkpoint.FileCount,
		FolderCount:        checkpoint.DirCount,
		TotalBytes:         checkpoint.TotalBytes,
		IsHead:             checkpoint.ID == headSavepoint,
	}
}

func (s *Service) getCheckpoint(ctx context.Context, workspace, checkpointID string) (checkpointDetail, error) {
	if err := ValidateName("workspace", workspace); err != nil {
		return checkpointDetail{}, err
	}
	if err := ValidateName("checkpoint", checkpointID); err != nil {
		return checkpointDetail{}, err
	}
	meta, err := s.store.GetWorkspaceMeta(ctx, workspace)
	if err != nil {
		return checkpointDetail{}, err
	}
	meta = applyWorkspaceMetaDefaults(s.cfg, meta)
	storageID := workspaceStorageID(meta)
	checkpoint, err := s.store.GetSavepointMeta(ctx, storageID, checkpointID)
	if err != nil {
		return checkpointDetail{}, err
	}
	manifestValue, err := s.store.GetManifest(ctx, storageID, checkpointID)
	if err != nil {
		return checkpointDetail{}, err
	}

	var parent *checkpointSummary
	parentManifest := Manifest{}
	if checkpoint.ParentSavepoint != "" {
		if parentMeta, err := s.store.GetSavepointMeta(ctx, storageID, checkpoint.ParentSavepoint); err == nil {
			parentSummary := checkpointSummaryFromMeta(parentMeta, meta.HeadSavepoint)
			parent = &parentSummary
		} else if !errors.Is(err, os.ErrNotExist) {
			return checkpointDetail{}, err
		}
		if m, err := s.store.GetManifest(ctx, storageID, checkpoint.ParentSavepoint); err == nil {
			parentManifest = m
		} else if !errors.Is(err, os.ErrNotExist) {
			return checkpointDetail{}, err
		}
	}
	changes := diffManifests(parentManifest, manifestValue)
	return checkpointDetailFromMeta(meta, checkpoint, parent, summarizeDiffEntries(changes)), nil
}

func checkpointDetailFromMeta(meta WorkspaceMeta, checkpoint SavepointMeta, parent *checkpointSummary, summary DiffSummary) checkpointDetail {
	return checkpointDetail{
		WorkspaceID:        workspaceStorageID(meta),
		WorkspaceName:      meta.Name,
		ID:                 checkpoint.ID,
		Name:               checkpoint.Name,
		Author:             defaultString(checkpoint.Author, "afs"),
		Description:        checkpoint.Description,
		Note:               checkpoint.Description,
		Kind:               checkpointKind(checkpoint.Kind),
		Source:             checkpointSource(checkpoint.Source),
		CreatedBy:          checkpoint.CreatedBy,
		SessionID:          checkpoint.SessionID,
		AgentID:            checkpoint.AgentID,
		AgentName:          checkpoint.AgentName,
		ParentCheckpointID: checkpoint.ParentSavepoint,
		Parent:             parent,
		ManifestHash:       checkpoint.ManifestHash,
		CreatedAt:          checkpoint.CreatedAt.UTC().Format(time.RFC3339),
		FileCount:          checkpoint.FileCount,
		FolderCount:        checkpoint.DirCount,
		TotalBytes:         checkpoint.TotalBytes,
		IsHead:             checkpoint.ID == meta.HeadSavepoint,
		ChangeSummary:      summary,
	}
}

func (s *Service) restoreCheckpoint(ctx context.Context, workspace, checkpointID string) (RestoreCheckpointResult, error) {
	if err := ValidateName("workspace", workspace); err != nil {
		return RestoreCheckpointResult{}, err
	}
	if err := ValidateName("checkpoint", checkpointID); err != nil {
		return RestoreCheckpointResult{}, err
	}
	meta, err := s.store.GetWorkspaceMeta(ctx, workspace)
	if err != nil {
		return RestoreCheckpointResult{}, err
	}
	storageID := workspaceStorageID(meta)
	result := RestoreCheckpointResult{
		Restored:      true,
		CheckpointID:  checkpointID,
		WorkspaceID:   storageID,
		WorkspaceName: meta.Name,
	}
	if err := CheckImportLock(ctx, s.store, storageID); err != nil {
		return RestoreCheckpointResult{}, err
	}
	exists, err := s.store.SavepointExists(ctx, storageID, checkpointID)
	if err != nil {
		return RestoreCheckpointResult{}, err
	}
	if !exists {
		return RestoreCheckpointResult{}, os.ErrNotExist
	}
	safetyCheckpointID, saved, err := s.createRestoreSafetyCheckpoint(ctx, storageID, checkpointID)
	if err != nil {
		return RestoreCheckpointResult{}, err
	}
	if saved {
		result.SafetyCheckpointID = safetyCheckpointID
		result.SafetyCheckpointCreated = true
		meta, err = s.store.GetWorkspaceMeta(ctx, storageID)
		if err != nil {
			return RestoreCheckpointResult{}, err
		}
	}
	// Capture the current manifest before moving head so we can diff against
	// the restore target for the changelog.
	priorManifest, err := s.store.GetManifest(ctx, storageID, meta.HeadSavepoint)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return RestoreCheckpointResult{}, err
	}
	if err := s.store.MoveWorkspaceHead(ctx, storageID, checkpointID, time.Now().UTC()); err != nil {
		if err == redis.TxFailedErr {
			return RestoreCheckpointResult{}, ErrWorkspaceConflict
		}
		return RestoreCheckpointResult{}, err
	}
	manifestValue, err := s.store.GetManifest(ctx, storageID, checkpointID)
	if err != nil {
		return RestoreCheckpointResult{}, err
	}
	if err := SyncWorkspaceRoot(ctx, s.store, storageID, manifestValue); err != nil {
		return RestoreCheckpointResult{}, err
	}
	if err := afsclient.PublishInvalidation(ctx, s.store.rdb, WorkspaceFSKey(storageID), afsclient.InvalidateEvent{
		Origin: "control-plane",
		Op:     afsclient.InvalidateOpRootReplace,
		Paths:  []string{"/"},
	}); err != nil {
		return RestoreCheckpointResult{}, err
	}
	template := s.buildChangelogTemplate(ctx, storageID, checkpointID, ChangeSourceServerRestore)
	versionsByPath, err := s.store.RecordManifestVersionChangesWithResults(ctx, storageID, priorManifest, manifestValue, FileVersionMutationMetadata{
		Source:       "checkpoint_restore",
		CheckpointID: checkpointID,
	})
	if err != nil {
		return RestoreCheckpointResult{}, err
	}
	writeChangeEntries(ctx, s.store.rdb, storageID, annotateChangeEntriesWithVersions(manifestDiff(priorManifest, manifestValue, template), versionsByPath))
	fields := map[string]any{
		"checkpoint": checkpointID,
		"mode":       "canonical-only",
	}
	if result.SafetyCheckpointCreated {
		fields["safety_checkpoint"] = result.SafetyCheckpointID
	}
	return result, s.store.Audit(ctx, storageID, "checkpoint_restore", fields)
}

func (s *Service) createRestoreSafetyCheckpoint(ctx context.Context, workspace, checkpointID string) (string, bool, error) {
	safetyCheckpointID := restoreSafetyCheckpointName()
	saved, err := s.saveCheckpointFromLive(ctx, workspace, safetyCheckpointID, SaveCheckpointFromLiveOptions{
		Description: fmt.Sprintf("Safety checkpoint before restoring %s.", checkpointID),
		Kind:        CheckpointKindSafety,
		Source:      CheckpointSourceServer,
		Author:      "afs",
	})
	if err != nil {
		return "", false, fmt.Errorf("restore safety checkpoint: %w", err)
	}
	return safetyCheckpointID, saved, nil
}

func restoreSafetyCheckpointName() string {
	return "restore-safety-" + time.Now().UTC().Format("20060102-150405.000000000")
}

func checkpointKind(kind string) string {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return CheckpointKindManual
	}
	return kind
}

func checkpointSource(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return CheckpointSourceServer
	}
	return source
}

func checkpointAuthorFromIdentity(identity AuthIdentity) string {
	if value := strings.TrimSpace(identity.Name); value != "" {
		return value
	}
	if value := strings.TrimSpace(identity.Email); value != "" {
		return value
	}
	return strings.TrimSpace(identity.Subject)
}

func (s *Service) checkpointAttribution(ctx context.Context, storageID string, input SaveCheckpointRequest) SavepointMeta {
	meta := SavepointMeta{
		Kind:      checkpointKind(input.Kind),
		Source:    checkpointSource(input.Source),
		Author:    strings.TrimSpace(input.Author),
		CreatedBy: strings.TrimSpace(input.CreatedBy),
	}
	if identity, ok := AuthIdentityFromContext(ctx); ok {
		if meta.CreatedBy == "" {
			meta.CreatedBy = strings.TrimSpace(identity.Subject)
		}
		if meta.Author == "" {
			meta.Author = checkpointAuthorFromIdentity(identity)
		}
	}
	sc, ok := ChangeSessionContextFromContext(ctx)
	if ok && strings.TrimSpace(sc.SessionID) != "" {
		meta.SessionID = strings.TrimSpace(sc.SessionID)
		if s != nil && s.store != nil {
			record, err := s.store.GetWorkspaceSession(ctx, storageID, meta.SessionID)
			if err == nil {
				meta.AgentID = strings.TrimSpace(record.AgentID)
				meta.AgentName = workspaceSessionRecordAgentName(record)
				if meta.AgentName == "" {
					meta.AgentName = strings.TrimSpace(record.Hostname)
				}
				if meta.Author == "" {
					meta.Author = firstNonEmpty(meta.AgentName, meta.AgentID)
				}
			}
		}
	}
	if meta.Author == "" {
		meta.Author = "afs"
	}
	return meta
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
	meta, err := s.store.GetWorkspaceMeta(ctx, input.Workspace)
	if err != nil {
		return false, err
	}
	storageID := workspaceStorageID(meta)
	if err := CheckImportLock(ctx, s.store, storageID); err != nil {
		return false, err
	}

	headManifest, err := s.store.GetManifest(ctx, storageID, input.ExpectedHead)
	if err != nil {
		return false, err
	}
	if manifestEquivalent(headManifest, input.Manifest) && !input.AllowUnchanged {
		return false, nil
	}

	changelogTemplate := s.buildChangelogTemplate(ctx, storageID, input.CheckpointID, ChangeSourceCheckpoint)
	changelogEntries := manifestDiff(headManifest, input.Manifest, changelogTemplate)

	now := time.Now().UTC()
	manifestHash, err := HashManifest(input.Manifest)
	if err != nil {
		return false, err
	}
	if err := s.store.SaveBlobs(ctx, storageID, input.Blobs); err != nil {
		return false, err
	}

	attribution := s.checkpointAttribution(ctx, storageID, input)
	savepointMeta := SavepointMeta{
		Version:         formatVersion,
		ID:              input.CheckpointID,
		Name:            input.CheckpointID,
		Description:     strings.TrimSpace(input.Description),
		Kind:            attribution.Kind,
		Source:          attribution.Source,
		Author:          attribution.Author,
		CreatedBy:       attribution.CreatedBy,
		SessionID:       attribution.SessionID,
		AgentID:         attribution.AgentID,
		AgentName:       attribution.AgentName,
		Workspace:       storageID,
		ParentSavepoint: input.ExpectedHead,
		ManifestHash:    manifestHash,
		CreatedAt:       now,
		FileCount:       input.FileCount,
		DirCount:        input.DirCount,
		TotalBytes:      input.TotalBytes,
	}

	err = s.store.rdb.Watch(ctx, func(tx *redis.Tx) error {
		current, err := getJSON[WorkspaceMeta](ctx, tx, workspaceMetaKey(storageID))
		if err != nil {
			return err
		}
		if current.HeadSavepoint != input.ExpectedHead {
			return ErrWorkspaceConflict
		}
		exists, err := tx.Exists(ctx, savepointMetaKey(storageID, input.CheckpointID)).Result()
		if err != nil {
			return err
		}
		if exists > 0 {
			return fmt.Errorf("savepoint %q already exists", input.CheckpointID)
		}

		updatedRefs := map[string]blobRef{}
		for blobID, size := range manifestBlobRefs(input.Manifest) {
			ref, err := getJSON[blobRef](ctx, tx, blobRefKey(storageID, blobID))
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
			if err := setJSON(ctx, pipe, savepointMetaKey(storageID, input.CheckpointID), savepointMeta); err != nil {
				return err
			}
			if err := setJSON(ctx, pipe, savepointManifestKey(storageID, input.CheckpointID), input.Manifest); err != nil {
				return err
			}
			if err := setJSON(ctx, pipe, workspaceMetaKey(storageID), current); err != nil {
				return err
			}
			pipe.ZAdd(ctx, workspaceSavepointsKey(storageID), redis.Z{
				Score:  float64(now.UnixMilli()),
				Member: input.CheckpointID,
			})
			for blobID, ref := range updatedRefs {
				if err := setJSON(ctx, pipe, blobRefKey(storageID, blobID), ref); err != nil {
					return err
				}
			}
			enqueueChangeEntries(ctx, pipe, storageID, changelogEntries)
			return nil
		})
		return err
	}, workspaceMetaKey(storageID))
	if err != nil {
		if errors.Is(err, ErrWorkspaceConflict) || err == redis.TxFailedErr {
			return false, ErrWorkspaceConflict
		}
		return false, err
	}
	if !input.SkipWorkspaceRootSync {
		if err := SyncWorkspaceRoot(ctx, s.store, storageID, input.Manifest); err != nil {
			return false, err
		}
	} else {
		if err := MarkWorkspaceRootClean(ctx, s.store, storageID, input.CheckpointID); err != nil {
			return false, err
		}
	}

	if err := s.store.Audit(ctx, storageID, "save", map[string]any{
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
	sourceMeta, err := s.store.GetWorkspaceMeta(ctx, sourceWorkspace)
	if err != nil {
		return err
	}
	sourceStorageID := workspaceStorageID(sourceMeta)
	if err := CheckImportLock(ctx, s.store, sourceStorageID); err != nil {
		return err
	}
	if err := CheckImportLock(ctx, s.store, newWorkspace); err != nil {
		return err
	}
	exists, err := s.store.WorkspaceExists(ctx, newWorkspace)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("workspace %q already exists", newWorkspace)
	}

	sourceMeta = applyWorkspaceMetaDefaults(s.cfg, sourceMeta)
	sourceManifest, err := s.store.GetManifest(ctx, sourceStorageID, sourceMeta.HeadSavepoint)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	newWorkspaceID, err := newOpaqueWorkspaceID()
	if err != nil {
		return err
	}
	newManifest := cloneManifest(sourceManifest)
	newManifest.Workspace = newWorkspaceID
	newManifest.Savepoint = initialCheckpointName
	manifestHash, err := HashManifest(newManifest)
	if err != nil {
		return err
	}

	blobs := map[string][]byte{}
	for blobID := range manifestBlobRefs(sourceManifest) {
		data, err := s.store.GetBlob(ctx, sourceStorageID, blobID)
		if err != nil {
			return err
		}
		blobs[blobID] = data
	}
	stats := manifestStats(newManifest)
	workspaceMeta := WorkspaceMeta{
		Version:          formatVersion,
		ID:               newWorkspaceID,
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
		Version:         formatVersion,
		ID:              initialCheckpointName,
		Name:            initialCheckpointName,
		Description:     "Forked from " + sourceWorkspace + ".",
		Kind:            CheckpointKindFork,
		Source:          CheckpointSourceServer,
		Author:          "afs",
		Workspace:       newWorkspaceID,
		ParentSavepoint: sourceMeta.HeadSavepoint,
		ManifestHash:    manifestHash,
		CreatedAt:       now,
		FileCount:       stats.FileCount,
		DirCount:        stats.DirCount,
		TotalBytes:      stats.TotalBytes,
	}

	if err := s.store.PutWorkspaceMeta(ctx, workspaceMeta); err != nil {
		return err
	}
	if err := s.store.SaveBlobs(ctx, newWorkspaceID, blobs); err != nil {
		return err
	}
	if err := s.store.AddBlobRefs(ctx, newWorkspaceID, newManifest, now); err != nil {
		return err
	}
	if err := s.store.PutSavepoint(ctx, checkpointMeta, newManifest); err != nil {
		return err
	}
	if err := s.store.CloneFileVersionHistory(ctx, sourceStorageID, newWorkspaceID); err != nil {
		return err
	}
	if err := SyncWorkspaceRoot(ctx, s.store, newWorkspaceID, newManifest); err != nil {
		return err
	}
	return s.store.Audit(ctx, newWorkspaceID, "workspace_fork", map[string]any{
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
	if view.Kind == "working-copy" {
		return s.getWorkingCopyTree(ctx, workspace, normalizedPath, depth)
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
	if view.Kind == "working-copy" {
		return s.getWorkingCopyFileContent(ctx, workspace, normalizedPath)
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

func (s *Service) getWorkingCopyTree(ctx context.Context, workspace, normalizedPath string, depth int) (treeResponse, error) {
	if _, _, _, err := EnsureWorkspaceRoot(ctx, s.store, workspace); err != nil {
		return treeResponse{}, err
	}
	meta, err := s.store.GetWorkspaceMeta(ctx, workspace)
	if err != nil {
		return treeResponse{}, err
	}
	fsClient := afsclient.New(s.store.rdb, WorkspaceFSKey(workspaceStorageID(meta)))
	stat, err := fsClient.Stat(ctx, normalizedPath)
	if err != nil {
		return treeResponse{}, err
	}
	if stat == nil {
		return treeResponse{}, os.ErrNotExist
	}
	if stat.Type != "dir" {
		return treeResponse{}, fmt.Errorf("path %q is not a directory", normalizedPath)
	}

	items := make([]treeItem, 0)
	if err := appendWorkingCopyTreeItems(ctx, fsClient, normalizedPath, depth, &items); err != nil {
		return treeResponse{}, err
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
		View:        "working-copy",
		Path:        normalizedPath,
		Items:       items,
	}, nil
}

func (s *Service) getWorkingCopyFileContent(ctx context.Context, workspace, normalizedPath string) (fileContentResponse, error) {
	if _, _, _, err := EnsureWorkspaceRoot(ctx, s.store, workspace); err != nil {
		return fileContentResponse{}, err
	}
	meta, err := s.store.GetWorkspaceMeta(ctx, workspace)
	if err != nil {
		return fileContentResponse{}, err
	}
	fsClient := afsclient.New(s.store.rdb, WorkspaceFSKey(workspaceStorageID(meta)))
	stat, err := fsClient.Stat(ctx, normalizedPath)
	if err != nil {
		return fileContentResponse{}, err
	}
	if stat == nil {
		return fileContentResponse{}, os.ErrNotExist
	}
	if stat.Type == "dir" {
		return fileContentResponse{}, fmt.Errorf("path %q is a directory", normalizedPath)
	}

	response := fileContentResponse{
		WorkspaceID: workspace,
		View:        "working-copy",
		Path:        normalizedPath,
		Kind:        stat.Type,
		Revision:    workingCopyRevision(stat, normalizedPath),
		Language:    language(normalizedPath),
		Encoding:    "utf-8",
		ContentType: contentType(normalizedPath, stat.Type),
		Size:        stat.Size,
		ModifiedAt:  workingCopyTimestamp(stat.Mtime),
	}

	switch stat.Type {
	case "symlink":
		target, err := fsClient.Readlink(ctx, normalizedPath)
		if err != nil {
			return fileContentResponse{}, err
		}
		response.Target = target
		response.Content = target
		return response, nil
	case "file":
		data, err := fsClient.Cat(ctx, normalizedPath)
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
		return fileContentResponse{}, fmt.Errorf("unsupported working-copy entry type %q", stat.Type)
	}
}

func appendWorkingCopyTreeItems(ctx context.Context, fsClient afsclient.Client, currentPath string, depth int, out *[]treeItem) error {
	entries, err := fsClient.LsLong(ctx, currentPath)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		childPath := path.Join(currentPath, entry.Name)
		item := treeItem{
			Path:       childPath,
			Name:       entry.Name,
			Kind:       entry.Type,
			Size:       entry.Size,
			ModifiedAt: workingCopyTimestamp(entry.Mtime),
		}
		if entry.Type == "symlink" {
			target, err := fsClient.Readlink(ctx, childPath)
			if err != nil {
				return err
			}
			item.Target = target
		}
		*out = append(*out, item)

		if depth > 1 && entry.Type == "dir" {
			if err := appendWorkingCopyTreeItems(ctx, fsClient, childPath, depth-1, out); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) listWorkspaceActivity(ctx context.Context, workspace string, req activityListRequest) (activityListResponse, error) {
	limit := req.Limit
	if limit <= 0 {
		req.Limit = 50
		limit = req.Limit
	}
	meta, err := s.store.GetWorkspaceMeta(ctx, workspace)
	if err != nil {
		return activityListResponse{}, err
	}
	page, err := s.store.listAuditPage(ctx, workspace, req)
	if err != nil {
		return activityListResponse{}, err
	}
	items := make([]activityEvent, 0, len(page.Items))
	for _, record := range page.Items {
		items = append(items, activityFromAudit(meta.Name, record))
	}
	return activityListResponse{Items: items, NextCursor: page.NextCursor}, nil
}

func (s *Service) listGlobalActivity(ctx context.Context, req activityListRequest) (activityListResponse, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 50
		req.Limit = limit
	}
	metas, err := s.store.ListWorkspaces(ctx)
	if err != nil {
		return activityListResponse{}, err
	}
	items := make([]activityEvent, 0, len(metas))
	for _, meta := range metas {
		page, err := s.store.listAuditPage(ctx, workspaceStorageID(meta), req)
		if err != nil {
			return activityListResponse{}, err
		}
		for _, record := range page.Items {
			items = append(items, activityFromAudit(meta.Name, record))
		}
	}
	sort.Slice(items, func(i, j int) bool { return compareActivityEvents(items[i], items[j]) > 0 })
	if len(items) > limit {
		items = items[:limit]
	}
	response := activityListResponse{Items: items}
	if len(items) > 0 {
		response.NextCursor = items[len(items)-1].ID
	}
	return response, nil
}

func (s *Service) listGlobalChangelog(ctx context.Context, req ChangelogListRequest) (ChangelogListResponse, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 100
		req.Limit = limit
	}
	if limit > 1000 {
		limit = 1000
		req.Limit = limit
	}
	metas, err := s.store.ListWorkspaces(ctx)
	if err != nil {
		return ChangelogListResponse{}, err
	}
	entries := make([]ChangelogEntryRow, 0, limit)
	for _, meta := range metas {
		page, err := s.store.ListChangelog(ctx, workspaceStorageID(meta), req)
		if err != nil {
			return ChangelogListResponse{}, err
		}
		for _, entry := range page.Entries {
			entry.WorkspaceID = defaultString(meta.ID, meta.Name)
			entry.WorkspaceName = meta.Name
			entries = append(entries, entry)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		comparison := compareChangelogRows(entries[i], entries[j])
		if req.Reverse {
			return comparison > 0
		}
		return comparison < 0
	})
	if len(entries) > limit {
		entries = entries[:limit]
	}
	response := ChangelogListResponse{Entries: entries}
	if len(entries) > 0 {
		response.NextCursor = entries[len(entries)-1].ID
	}
	return response, nil
}

func compareChangelogRows(left, right ChangelogEntryRow) int {
	if comparison := compareRedisStreamIDs(left.ID, right.ID); comparison != 0 {
		return comparison
	}
	if comparison := strings.Compare(left.WorkspaceID, right.WorkspaceID); comparison != 0 {
		return comparison
	}
	return strings.Compare(left.Path, right.Path)
}

func compareActivityEvents(left, right activityEvent) int {
	if comparison := compareRedisStreamIDs(left.ID, right.ID); comparison != 0 {
		return comparison
	}
	if comparison := strings.Compare(left.WorkspaceID, right.WorkspaceID); comparison != 0 {
		return comparison
	}
	return strings.Compare(left.Kind, right.Kind)
}

func compareRedisStreamIDs(left, right string) int {
	leftMS, leftSeq := parseRedisStreamID(left)
	rightMS, rightSeq := parseRedisStreamID(right)
	if leftMS < rightMS {
		return -1
	}
	if leftMS > rightMS {
		return 1
	}
	if leftSeq < rightSeq {
		return -1
	}
	if leftSeq > rightSeq {
		return 1
	}
	return strings.Compare(left, right)
}

func parseRedisStreamID(id string) (int64, int64) {
	ms, seq, ok := strings.Cut(id, "-")
	if !ok {
		n, _ := strconv.ParseInt(id, 10, 64)
		return n, 0
	}
	msValue, _ := strconv.ParseInt(ms, 10, 64)
	seqValue, _ := strconv.ParseInt(seq, 10, 64)
	return msValue, seqValue
}

func (s *Service) buildWorkspaceSummary(ctx context.Context, meta WorkspaceMeta) (workspaceSummary, error) {
	meta = applyWorkspaceMetaDefaults(s.cfg, meta)
	storageID := workspaceStorageID(meta)
	checkpoints, err := s.store.ListSavepoints(ctx, storageID, 0)
	if err != nil {
		return workspaceSummary{}, err
	}
	headMeta, err := s.store.GetSavepointMeta(ctx, storageID, meta.HeadSavepoint)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return workspaceSummary{}, err
	}
	stats := s.currentWorkspaceStats(ctx, storageID, headMeta)
	dirty, err := s.workspaceDirtyState(ctx, storageID, meta)
	if err != nil {
		return workspaceSummary{}, err
	}
	lastCheckpointAt := meta.UpdatedAt.UTC().Format(time.RFC3339)
	if len(checkpoints) > 0 {
		lastCheckpointAt = checkpoints[0].CreatedAt.UTC().Format(time.RFC3339)
	}
	return workspaceSummary{
		ID:               storageID,
		Name:             meta.Name,
		CloudAccount:     meta.CloudAccount,
		DatabaseID:       meta.DatabaseID,
		DatabaseName:     meta.DatabaseName,
		RedisKey:         WorkspaceFSKey(storageID),
		Status:           workspaceStatus(dirty),
		FileCount:        stats.FileCount,
		FolderCount:      stats.FolderCount,
		TotalBytes:       stats.TotalBytes,
		CheckpointCount:  len(checkpoints),
		DraftState:       draftState(dirty),
		LastCheckpointAt: lastCheckpointAt,
		UpdatedAt:        meta.UpdatedAt.UTC().Format(time.RFC3339),
		Region:           meta.Region,
		Source:           workspaceSource(meta),
	}, nil
}

func (s *Service) workspaceDirtyState(ctx context.Context, storageID string, meta WorkspaceMeta) (bool, error) {
	if s == nil || s.store == nil {
		return meta.DirtyHint, nil
	}
	dirty, known, err := WorkspaceRootDirtyState(ctx, s.store, storageID)
	if err != nil {
		return false, err
	}
	if known {
		return dirty, nil
	}
	return meta.DirtyHint, nil
}

func (s *Service) currentWorkspaceStats(ctx context.Context, workspace string, fallback SavepointMeta) workspaceUsageStats {
	stats := workspaceUsageStats{
		FileCount:   fallback.FileCount,
		FolderCount: fallback.DirCount,
		TotalBytes:  fallback.TotalBytes,
	}
	if s == nil || s.store == nil || s.store.rdb == nil {
		return stats
	}
	if _, _, _, err := EnsureWorkspaceRoot(ctx, s.store, workspace); err != nil {
		return stats
	}

	meta, err := s.store.GetWorkspaceMeta(ctx, workspace)
	if err != nil {
		return stats
	}
	info, err := afsclient.New(s.store.rdb, WorkspaceFSKey(workspaceStorageID(meta))).Info(ctx)
	if err != nil || info == nil {
		return stats
	}

	folderCount := int(info.Directories)
	if folderCount > 0 {
		folderCount--
	}
	return workspaceUsageStats{
		FileCount:   int(info.Files),
		FolderCount: folderCount,
		TotalBytes:  info.TotalDataBytes,
	}
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

	storageID := workspaceStorageID(meta)
	checkpoint, err := s.store.GetSavepointMeta(ctx, storageID, savepointID)
	if err != nil {
		return WorkspaceMeta{}, SavepointMeta{}, Manifest{}, err
	}
	manifestValue, err := s.store.GetManifest(ctx, storageID, savepointID)
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
	workspaceID, err := newOpaqueWorkspaceID()
	if err != nil {
		return err
	}
	if err := CheckImportLock(ctx, store, workspaceID); err != nil {
		return err
	}
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
		Workspace: workspaceID,
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
		ID:               workspaceID,
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
		Kind:         CheckpointKindSystem,
		Source:       CheckpointSourceServer,
		Author:       "afs",
		Workspace:    workspaceID,
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
	if err := SyncWorkspaceRoot(ctx, store, workspaceID, rootManifest); err != nil {
		return err
	}
	return store.Audit(ctx, workspaceID, "workspace_create", map[string]any{
		"checkpoint": initialCheckpointName,
		"source":     workspaceMeta.Source,
	})
}

func applyWorkspaceMetaDefaults(cfg Config, meta WorkspaceMeta) WorkspaceMeta {
	defaultDatabaseID, defaultDatabaseName := activeDatabaseIdentity(cfg)
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

func activeDatabaseIdentity(cfg Config) (databaseID string, databaseName string) {
	databaseName = strings.TrimSpace(cfg.RedisAddr)
	if databaseName == "" {
		databaseName = "direct-redis"
	}
	databaseID = "redis-" + slugify(fmt.Sprintf("%s-%d", databaseName, cfg.RedisDB))
	if databaseID == "redis-" {
		databaseID = "redis-direct"
	}
	return databaseID, databaseName
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

func workspaceStatus(dirty bool) string {
	if dirty {
		return "attention"
	}
	return "healthy"
}

func draftState(dirty bool) string {
	if dirty {
		return "dirty"
	}
	return "clean"
}

func defaultCapabilities() capabilities {
	return capabilities{
		BrowseHead:        true,
		BrowseCheckpoints: true,
		BrowseWorkingCopy: true,
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

func workingCopyTimestamp(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}

func workingCopyRevision(stat *afsclient.StatResult, normalizedPath string) string {
	if stat == nil {
		return "working-copy:" + normalizedPath
	}
	return fmt.Sprintf("working-copy:%d:%d:%d:%s", stat.Ctime, stat.Mtime, stat.Size, normalizedPath)
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
	case "session_start":
		title = "Connected client"
		detail = fmt.Sprintf("%s connected from %s.", defaultString(record.Fields["client_kind"], "client"), defaultString(record.Fields["hostname"], "an unknown host"))
		kind = "session.started"
		scope = "session"
	case "session_close":
		title = "Disconnected client"
		detail = fmt.Sprintf("%s disconnected from %s.", defaultString(record.Fields["client_kind"], "client"), defaultString(record.Fields["hostname"], "an unknown host"))
		kind = "session.closed"
		scope = "session"
	case "session_stale":
		title = "Client connection expired"
		detail = fmt.Sprintf("%s at %s stopped heartbeating and was marked stale.", defaultString(record.Fields["client_kind"], "client"), defaultString(record.Fields["hostname"], "an unknown host"))
		kind = "session.stale"
		scope = "session"
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
