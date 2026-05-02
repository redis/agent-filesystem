package controlplane

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	quickstartWorkspaceName = "getting-started"
	quickstartCheckpoint    = "initial"
	quickstartSource        = "quickstart"
	quickstartLocalDBName   = "Local Development"
	quickstartCloudDBName   = "Starter Database"
	quickstartCloudDBID     = "afs-cloud"
)

// QuickstartRequest contains optional overrides for the quickstart flow.
type QuickstartRequest struct {
	RedisAddr     string `json:"redis_addr"`
	RedisPassword string `json:"redis_password"`
	RedisUsername string `json:"redis_username"`
	RedisDB       int    `json:"redis_db"`
	RedisTLS      bool   `json:"redis_tls"`
}

// QuickstartResponse is returned on successful quickstart.
type QuickstartResponse struct {
	DatabaseID  string          `json:"database_id"`
	WorkspaceID string          `json:"workspace_id"`
	Workspace   workspaceDetail `json:"workspace"`
}

// SeedGettingStarted ensures a `getting-started` workspace exists on the
// control plane, creating a default database if needed. It is idempotent and
// safe to call on every boot: if the workspace already exists (from an
// earlier boot or a prior Quickstart call), it returns without mutating
// anything. Intended for self-hosted deployments so a fresh `afs auth login
// --self-hosted` lands the user on a usable workspace without manual setup.
func (m *DatabaseManager) SeedGettingStarted(ctx context.Context) error {
	_, err := m.Quickstart(ctx, QuickstartRequest{})
	return err
}

// Quickstart creates a database connection and a workspace pre-populated with
// sample content in a single call. It is designed for first-time onboarding.
func (m *DatabaseManager) Quickstart(ctx context.Context, input QuickstartRequest) (QuickstartResponse, error) {
	if err := m.ensureBootstrapDatabase(ctx); err != nil {
		return QuickstartResponse{}, err
	}

	// Step 1: If a database already exists, reuse it instead of creating a new
	// one. This handles the Docker case where the control plane starts with
	// AFS_REDIS_ADDR=redis:6379 and auto-seeds a database profile.
	m.mu.Lock()
	existingDBID := ""
	if len(m.order) > 0 {
		existingDBID = m.order[0]
	}
	m.mu.Unlock()

	if existingDBID != "" {
		// Verify the connection still works and get the record.
		dbList, err := m.ListDatabases(ctx)
		if err == nil && len(dbList.Items) > 0 {
			dbRecord := dbList.Items[0]
			if dbRecord.ConnectionError == "" {
				return m.quickstartWithDatabase(ctx, dbRecord.ID)
			}
		}
	}

	// Step 2: No usable database — create one from explicit input, AFS_REDIS_*,
	// REDIS_URL, or finally localhost.
	dbReq := quickstartDatabaseRequest(input)

	dbRecord, err := m.UpsertDatabase(ctx, existingDBID, dbReq)
	if err != nil {
		return QuickstartResponse{}, fmt.Errorf("quickstart: connect database: %w", err)
	}

	return m.quickstartWithDatabase(ctx, dbRecord.ID)
}

func quickstartDatabaseRequest(input QuickstartRequest) UpsertDatabaseRequest {
	redisCfg := quickstartRedisConfig(input)
	name := quickstartLocalDBName
	description := "Auto-created by quickstart."

	if strings.TrimSpace(input.RedisAddr) == "" {
		if seeded, ok := bootstrapDatabaseProfileFromEnv(); ok {
			name = seeded.Name
			description = seeded.Description
		}
	}

	return UpsertDatabaseRequest{
		Name:          name,
		Description:   description,
		RedisAddr:     redisCfg.RedisAddr,
		RedisUsername: redisCfg.RedisUsername,
		RedisPassword: redisCfg.RedisPassword,
		RedisDB:       redisCfg.RedisDB,
		RedisTLS:      redisCfg.RedisTLS,
	}
}

func quickstartRedisConfig(input QuickstartRequest) RedisConfig {
	cfg := RedisConfig{
		RedisAddr:     strings.TrimSpace(input.RedisAddr),
		RedisUsername: strings.TrimSpace(input.RedisUsername),
		RedisPassword: input.RedisPassword,
		RedisDB:       input.RedisDB,
		RedisTLS:      input.RedisTLS,
	}
	if cfg.RedisAddr != "" {
		return cfg
	}

	if envCfg, ok := redisConfigFromAFSEnv(); ok {
		cfg = envCfg
		return cfg
	}

	if parsed, ok := redisConfigFromURL(strings.TrimSpace(os.Getenv("REDIS_URL"))); ok {
		return parsed
	}

	cfg.RedisAddr = "localhost:6379"
	return cfg
}

func redisConfigFromAFSEnv() (RedisConfig, bool) {
	envAddr := strings.TrimSpace(os.Getenv("AFS_REDIS_ADDR"))
	if envAddr == "" {
		return RedisConfig{}, false
	}

	cfg := RedisConfig{
		RedisAddr:     envAddr,
		RedisUsername: strings.TrimSpace(os.Getenv("AFS_REDIS_USERNAME")),
		RedisPassword: os.Getenv("AFS_REDIS_PASSWORD"),
	}
	if envDB := strings.TrimSpace(os.Getenv("AFS_REDIS_DB")); envDB != "" {
		if parsedDB, err := strconv.Atoi(envDB); err == nil {
			cfg.RedisDB = parsedDB
		}
	}
	if envTLS := strings.TrimSpace(os.Getenv("AFS_REDIS_TLS")); envTLS != "" {
		cfg.RedisTLS = envTLS == "1" || strings.EqualFold(envTLS, "true")
	}
	return cfg, true
}

func bootstrapDatabaseProfileFromEnv() (databaseProfile, bool) {
	return bootstrapDatabaseProfileFromContext(context.Background())
}

func bootstrapDatabaseProfileFromContext(_ context.Context) (databaseProfile, bool) {
	if cfg, ok := redisConfigFromAFSEnv(); ok {
		return databaseProfile{
			ID:             "local-development",
			Name:           quickstartLocalDBName,
			Description:    "Configured from AFS_REDIS_* environment variables.",
			ManagementType: databaseManagementUserManaged,
			RedisAddr:      cfg.RedisAddr,
			RedisUsername:  cfg.RedisUsername,
			RedisPassword:  cfg.RedisPassword,
			RedisDB:        cfg.RedisDB,
			RedisTLS:       cfg.RedisTLS,
			IsDefault:      true,
		}, true
	}

	if cfg, ok := redisConfigFromURL(strings.TrimSpace(os.Getenv("REDIS_URL"))); ok {
		return databaseProfile{
			ID:             quickstartCloudDBID,
			Name:           quickstartCloudDBName,
			Description:    "Managed Redis Cloud data plane for hosted Agent Filesystem.",
			OwnerLabel:     "Starter Database",
			ManagementType: databaseManagementSystemManaged,
			Purpose:        databasePurposeOnboarding,
			RedisAddr:      cfg.RedisAddr,
			RedisUsername:  cfg.RedisUsername,
			RedisPassword:  cfg.RedisPassword,
			RedisDB:        cfg.RedisDB,
			RedisTLS:       cfg.RedisTLS,
			IsDefault:      true,
		}, true
	}

	return databaseProfile{}, false
}

func bootstrapDatabaseProfileFromConfigPath(configPathOverride string) (databaseProfile, bool) {
	cfg, present, err := LoadConfigWithPresence(configPathOverride)
	if err != nil || !present || strings.TrimSpace(cfg.RedisAddr) == "" {
		return databaseProfile{}, false
	}
	return databaseProfile{
		ID:             "local-development",
		Name:           quickstartLocalDBName,
		Description:    "Configured from afs.config.json.",
		ManagementType: databaseManagementUserManaged,
		RedisAddr:      cfg.RedisAddr,
		RedisUsername:  cfg.RedisUsername,
		RedisPassword:  cfg.RedisPassword,
		RedisDB:        cfg.RedisDB,
		RedisTLS:       cfg.RedisTLS,
		IsDefault:      true,
	}, true
}

// quickstartWithDatabase creates the getting-started workspace on an existing database.
func (m *DatabaseManager) quickstartWithDatabase(ctx context.Context, databaseID string) (QuickstartResponse, error) {
	service, profile, err := m.serviceFor(ctx, databaseID)
	if err != nil {
		return QuickstartResponse{}, fmt.Errorf("quickstart: open service: %w", err)
	}

	workspaceName := quickstartWorkspaceNameFor(profile, authSubjectFromContext(ctx))
	if profile.Purpose == databasePurposeOnboarding && authSubjectFromContext(ctx) != "" {
		summaries, err := m.ListWorkspaceSummaries(ctx, databaseID)
		if err != nil {
			return QuickstartResponse{}, fmt.Errorf("quickstart: list workspaces: %w", err)
		}
		for _, item := range summaries.Items {
			if item.Name != workspaceName {
				continue
			}
			existing, err := m.GetWorkspace(ctx, databaseID, item.ID)
			if err != nil {
				return QuickstartResponse{}, fmt.Errorf("quickstart: load workspace: %w", err)
			}
			return QuickstartResponse{
				DatabaseID:  databaseID,
				WorkspaceID: existing.ID,
				Workspace:   existing,
			}, nil
		}
	} else {
		existing, err := m.GetWorkspace(ctx, databaseID, workspaceName)
		if err == nil {
			return QuickstartResponse{
				DatabaseID:  databaseID,
				WorkspaceID: existing.ID,
				Workspace:   existing,
			}, nil
		}
		if !isNotFound(err) {
			return QuickstartResponse{}, fmt.Errorf("quickstart: check workspace: %w", err)
		}
	}

	detail, err := createQuickstartWorkspace(ctx, service, profile, workspaceName)
	if err != nil {
		return QuickstartResponse{}, fmt.Errorf("quickstart: create workspace: %w", err)
	}

	if err := m.attachWorkspaceDetailIdentity(ctx, &detail, profile); err != nil {
		return QuickstartResponse{}, err
	}
	if _, err := m.syncWorkspaceCatalogSummary(ctx, workspaceSummaryFromDetail(detail)); err != nil {
		return QuickstartResponse{}, err
	}

	return QuickstartResponse{
		DatabaseID:  databaseID,
		WorkspaceID: detail.ID,
		Workspace:   detail,
	}, nil
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	if os.IsNotExist(err) {
		return true
	}
	return strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "does not exist")
}

func quickstartWorkspaceNameFor(profile databaseProfile, subject string) string {
	return quickstartWorkspaceName
}

// createQuickstartWorkspace builds a workspace pre-populated with sample files.
func createQuickstartWorkspace(ctx context.Context, service *Service, profile databaseProfile, workspace string) (workspaceDetail, error) {
	now := time.Now().UTC()
	workspaceID, err := newOpaqueWorkspaceID()
	if err != nil {
		return workspaceDetail{}, err
	}

	// Build the seed manifest with embedded content.
	manifest, fileCount, dirCount, totalBytes := buildSeedManifest(workspaceID, now)

	manifestHash, err := HashManifest(manifest)
	if err != nil {
		return workspaceDetail{}, err
	}

	meta := WorkspaceMeta{
		Version:          formatVersion,
		ID:               workspaceID,
		Name:             workspace,
		Description:      "Sample workspace with example files to explore AFS features.",
		DatabaseID:       profile.ID,
		DatabaseName:     profile.Name,
		CloudAccount:     quickstartCloudAccount(profile),
		Source:           quickstartSource,
		Tags:             []string{"Quickstart"},
		CreatedAt:        now,
		UpdatedAt:        now,
		HeadSavepoint:    quickstartCheckpoint,
		DefaultSavepoint: quickstartCheckpoint,
	}

	checkpoint := SavepointMeta{
		Version:      formatVersion,
		ID:           quickstartCheckpoint,
		Name:         quickstartCheckpoint,
		Description:  "Initial quickstart snapshot with sample files.",
		Kind:         CheckpointKindSystem,
		Source:       CheckpointSourceQuickstart,
		Author:       "afs",
		Workspace:    workspaceID,
		ManifestHash: manifestHash,
		CreatedAt:    now,
		FileCount:    fileCount,
		DirCount:     dirCount,
		TotalBytes:   totalBytes,
	}

	store := service.store
	if err := store.PutWorkspaceMeta(ctx, meta); err != nil {
		return workspaceDetail{}, err
	}
	if err := store.PutSavepoint(ctx, checkpoint, manifest); err != nil {
		return workspaceDetail{}, err
	}
	if err := SyncWorkspaceRoot(ctx, store, workspaceID, manifest); err != nil {
		return workspaceDetail{}, err
	}
	if err := store.Audit(ctx, workspaceID, "workspace_create", map[string]any{
		"checkpoint": quickstartCheckpoint,
		"source":     quickstartSource,
		"files":      fileCount,
		"dirs":       dirCount,
	}); err != nil {
		return workspaceDetail{}, err
	}

	return service.getWorkspace(ctx, workspaceID)
}

func quickstartCloudAccount(profile databaseProfile) string {
	name := strings.TrimSpace(profile.Name)
	if name == "" || strings.EqualFold(name, quickstartLocalDBName) {
		return "Direct Redis"
	}
	return name
}

// buildSeedManifest constructs a Manifest containing the quickstart sample files.
func buildSeedManifest(workspace string, now time.Time) (Manifest, int, int, int64) {
	ms := now.UnixMilli()

	files := map[string]string{
		"/README.md":            seedReadme,
		"/docs/quickstart.md":   seedQuickstart,
		"/docs/architecture.md": seedArchitecture,
		"/examples/hello.py":    seedHelloPy,
		"/examples/config.json": seedConfigJSON,
		"/tests/test_hello.py":  seedTestHelloPy,
	}

	dirs := []string{"/", "/docs", "/examples", "/tests"}

	entries := make(map[string]ManifestEntry, len(files)+len(dirs))
	var totalBytes int64

	for _, d := range dirs {
		entries[d] = ManifestEntry{
			Type:    "dir",
			Mode:    0o755,
			MtimeMs: ms,
			Size:    0,
		}
	}

	for path, content := range files {
		raw := []byte(content)
		size := int64(len(raw))
		totalBytes += size
		entries[path] = ManifestEntry{
			Type:    "file",
			Mode:    0o644,
			MtimeMs: ms,
			Size:    size,
			Inline:  base64.StdEncoding.EncodeToString(raw),
		}
	}

	manifest := Manifest{
		Version:   formatVersion,
		Workspace: workspace,
		Savepoint: quickstartCheckpoint,
		Entries:   entries,
	}

	return manifest, len(files), len(dirs), totalBytes
}

// ── Seed file content ──────────────────────────────────────────────────────────

const seedReadme = `# Welcome to Agent Filesystem

This is a sample workspace to help you explore AFS — fast, durable workspaces
for AI agents, backed by Redis.

## What's in this workspace

` + "```" + `
getting-started/
├── README.md              ← You are here
├── docs/
│   ├── quickstart.md      ← How to connect an agent
│   └── architecture.md    ← How AFS works under the hood
├── examples/
│   ├── hello.py           ← A simple script an agent can modify
│   └── config.json        ← Example structured data
└── tests/
    └── test_hello.py      ← Example test file
` + "```" + `

## Try these things

1. **Browse files** — Click through the file tree on the left to explore.
2. **Create a checkpoint** — Go to the Checkpoints tab and save a snapshot.
3. **Connect an agent** — Mount this workspace locally or use MCP tools.
4. **Edit and compare** — Have an agent modify ` + "`examples/hello.py`" + `, then
   compare the workspace state against your checkpoint.

## Connect an agent

**Option A: Mount locally**
` + "```bash" + `
afs ws mount getting-started ~/getting-started
# The workspace appears at ~/getting-started/
` + "```" + `

**Option B: MCP tools**
Add this to your agent's MCP configuration:
` + "```json" + `
{
  "mcpServers": {
    "agent-filesystem": {
      "command": "afs",
      "args": ["mcp"]
    }
  }
}
` + "```" + `

## Learn more

- Open ` + "`docs/quickstart.md`" + ` for a step-by-step walkthrough
- Open ` + "`docs/architecture.md`" + ` to understand how workspaces, checkpoints, and blobs work
- Visit the [Agent Guide](/agent-guide) in the web UI for full documentation
`

const seedQuickstart = `# Quickstart Guide

Get an AI agent working with AFS in under 5 minutes.

## Prerequisites

- AFS binary installed (you already have this if you can see this file)
- Redis running (AFS stores everything in Redis)

## Step 1: Mount the workspace

` + "```bash" + `
# Mount this workspace; files appear at ~/getting-started/
afs ws mount getting-started ~/getting-started
` + "```" + `

Your agent can now read and write files in that directory using normal file I/O.

## Step 2: Make changes

Have your agent edit a file:

` + "```bash" + `
echo "print('Modified by agent')" >> ~/getting-started/examples/hello.py
` + "```" + `

AFS syncs the change to Redis automatically.

## Step 3: Create a checkpoint

` + "```bash" + `
afs cp create getting-started after-agent-edit --description "Agent modified hello.py"
` + "```" + `

Or use the web UI — go to the Checkpoints tab and click **Save checkpoint**.

## Step 4: Restore if needed

` + "```bash" + `
# List checkpoints
afs cp list getting-started

# Restore to a previous state
afs cp restore getting-started initial
` + "```" + `

## Using MCP instead

If your agent supports MCP (Model Context Protocol), it can manage workspaces
directly without mounting:

` + "```json" + `
{
  "mcpServers": {
    "agent-filesystem": {
      "command": "afs",
      "args": ["mcp"]
    }
  }
}
` + "```" + `

The agent gets tools like ` + "`workspace_read`" + `, ` + "`workspace_write`" + `,
` + "`checkpoint_save`" + `, and ` + "`checkpoint_restore`" + `.
`

const seedArchitecture = `# AFS Architecture

## Three layers

**1. Redis-backed workspace store** — Redis holds workspace metadata,
manifests, blobs, checkpoints, and activity.

**2. Mount + Sync** — A Go daemon that exposes a Redis-backed workspace
as a normal directory on your machine. Two modes:
- **Sync mode** (default): local directory synced with Redis
- **Live mount**: direct NFS/FUSE mount of the Redis filesystem

**3. Control Plane + Web UI** — HTTP API and React dashboard for managing
workspaces, checkpoints, databases, and agent sessions.

## Key concepts

### Workspaces
An isolated filesystem namespace. Each workspace has its own file tree,
checkpoint history, and activity log. Workspaces are identified by name
(e.g., ` + "`getting-started`" + `).

### Checkpoints
Immutable snapshots of a workspace at a point in time. Like git commits
but for the entire workspace. Create them before risky operations and
restore if things go wrong.

### Manifests
A checkpoint is stored as a manifest — a map from file paths to metadata
(size, mode, modification time) plus content (inline for small files,
blob references for large ones).

### Blobs
File content larger than ~4KB is stored as a separate blob in Redis,
referenced by its content hash. Identical files across checkpoints share
the same blob (content-addressable storage).

## Data flow

` + "```" + `
Agent writes file → local disk → sync daemon → Redis workspace
                                              → checkpoint (on demand)
` + "```" + `

The Redis workspace is the source of truth. The local directory is a
synchronized cache that can be recreated from any checkpoint.
`

const seedHelloPy = `"""A simple script that an agent can modify and test."""


def greet(name: str) -> str:
    """Return a greeting message."""
    return f"Hello, {name}! Welcome to Agent Filesystem."


def add(a: int, b: int) -> int:
    """Add two numbers."""
    return a + b


if __name__ == "__main__":
    print(greet("Agent"))
    print(f"2 + 3 = {add(2, 3)}")
`

const seedConfigJSON = `{
  "project": "getting-started",
  "version": "1.0.0",
  "description": "Sample configuration for the AFS quickstart workspace.",
  "settings": {
    "auto_checkpoint": true,
    "sync_interval_ms": 500,
    "max_file_size_mb": 50
  },
  "tags": ["sample", "quickstart"]
}
`

const seedTestHelloPy = `"""Tests for examples/hello.py — an agent can run and extend these."""

import sys
import os

# Add parent directory to path so we can import the example.
sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "examples"))

from hello import greet, add


def test_greet_default():
    result = greet("World")
    assert "Hello, World!" in result
    assert "Agent Filesystem" in result


def test_greet_custom_name():
    result = greet("Alice")
    assert "Alice" in result


def test_add():
    assert add(2, 3) == 5
    assert add(-1, 1) == 0
    assert add(0, 0) == 0


if __name__ == "__main__":
    test_greet_default()
    test_greet_custom_name()
    test_add()
    print("All tests passed!")
`
