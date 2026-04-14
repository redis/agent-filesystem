package main

import (
	"context"
	"time"

	"github.com/redis/agent-filesystem/internal/worktree"
)

type redisConfig struct {
	RedisAddr     string `json:"addr"`
	RedisUsername string `json:"username"`
	RedisPassword string `json:"password"`
	RedisDB       int    `json:"db"`
	RedisTLS      bool   `json:"tls"`
}

type mountSettings struct {
	MountBackend string `json:"backend"`
	ReadOnly     bool   `json:"readOnly"`
	AllowOther   bool   `json:"allowOther"`
	MountBin     string `json:"mountBin"`
	NFSBin       string `json:"nfsBin"`
	NFSHost      string `json:"nfsHost"`
	NFSPort      int    `json:"nfsPort"`
}

type logSettings struct {
	MountLog string `json:"mount"`
	SyncLog  string `json:"sync"`
}

type syncSettings struct {
	SyncFileSizeCapMB int `json:"fileSizeCapMB"`
}

type controlPlaneSettings struct {
	URL        string `json:"url,omitempty"`
	DatabaseID string `json:"databaseID,omitempty"`
}

// config captures the persisted CLI/runtime settings for the AFS surface.
// The JSON tags define the on-disk format.
type config struct {
	redisConfig          `json:"redis"`
	controlPlaneSettings `json:"controlPlane,omitempty"`
	ProductMode          string `json:"productMode,omitempty"`
	Mode                 string `json:"mode,omitempty"`
	CurrentWorkspace     string `json:"currentWorkspace"`
	CurrentWorkspaceID   string `json:"currentWorkspaceID,omitempty"`
	LocalPath            string `json:"localPath,omitempty"`
	mountSettings        `json:"mount"`
	logSettings          `json:"logs"`
	syncSettings         `json:"sync"`

	// Internal-only state; not persisted to afs.config.json.
	WorkRoot string `json:"-"`
	RedisKey string `json:"-"`
}

// state records the currently running AFS processes and surface state.
type state struct {
	StartedAt            time.Time `json:"started_at"`
	ProductMode          string    `json:"product_mode,omitempty"`
	ControlPlaneURL      string    `json:"control_plane_url,omitempty"`
	ControlPlaneDatabase string    `json:"control_plane_database,omitempty"`
	SessionID            string    `json:"session_id,omitempty"`
	RedisAddr            string    `json:"redis_addr"`
	RedisDB              int       `json:"redis_db"`
	CurrentWorkspace     string    `json:"current_workspace,omitempty"`
	CurrentWorkspaceID   string    `json:"current_workspace_id,omitempty"`
	MountedHeadSavepoint string    `json:"mounted_head_savepoint,omitempty"`
	MountPID             int       `json:"mount_pid"`
	MountBackend         string    `json:"mount_backend"`
	ReadOnly             bool      `json:"read_only"`
	MountEndpoint        string    `json:"mount_endpoint,omitempty"`
	LocalPath            string    `json:"local_path,omitempty"`
	CreatedLocalPath     bool      `json:"created_local_path,omitempty"`
	RedisKey             string    `json:"redis_key"`
	MountLog             string    `json:"mount_log"`
	MountBin             string    `json:"mount_bin"`
	ArchivePath          string    `json:"archive_path,omitempty"`

	// Sync daemon mode fields. Populated when `afs up` ran with mode=sync.
	Mode    string `json:"mode,omitempty"`
	SyncPID int    `json:"sync_pid,omitempty"`
	SyncLog string `json:"sync_log,omitempty"`
}

type importClient interface {
	Mkdir(ctx context.Context, path string) error
	Echo(ctx context.Context, path string, data []byte) error
	Ln(ctx context.Context, target, linkpath string) error
	Chmod(ctx context.Context, path string, mode uint32) error
	Chown(ctx context.Context, path string, uid, gid uint32) error
	Utimens(ctx context.Context, path string, atimeMs, mtimeMs int64) error
}

type importStats = worktree.ImportStats
