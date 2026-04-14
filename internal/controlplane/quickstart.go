package controlplane

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	quickstartWorkspaceName = "getting-started"
	quickstartCheckpoint    = "initial"
	quickstartSource        = "quickstart"
)

// QuickstartRequest contains optional overrides for the quickstart flow.
type QuickstartRequest struct {
	RedisAddr     string `json:"redis_addr"`
	RedisPassword string `json:"redis_password"`
	RedisUsername  string `json:"redis_username"`
	RedisDB       int    `json:"redis_db"`
	RedisTLS      bool   `json:"redis_tls"`
}

// QuickstartResponse is returned on successful quickstart.
type QuickstartResponse struct {
	DatabaseID  string          `json:"database_id"`
	WorkspaceID string          `json:"workspace_id"`
	Workspace   workspaceDetail `json:"workspace"`
}

// Quickstart creates a database connection and a workspace pre-populated with
// sample content in a single call. It is designed for first-time onboarding.
func (m *DatabaseManager) Quickstart(ctx context.Context, input QuickstartRequest) (QuickstartResponse, error) {
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

	// Step 2: No usable database — create one. Use the Redis address from
	// the request, fall back to AFS_REDIS_ADDR env var, then to localhost:6379.
	redisAddr := strings.TrimSpace(input.RedisAddr)
	if redisAddr == "" {
		if envAddr := strings.TrimSpace(os.Getenv("AFS_REDIS_ADDR")); envAddr != "" {
			redisAddr = envAddr
		} else {
			redisAddr = "localhost:6379"
		}
	}

	dbReq := UpsertDatabaseRequest{
		Name:          "Local Development",
		Description:   "Auto-created by quickstart.",
		RedisAddr:     redisAddr,
		RedisUsername: input.RedisUsername,
		RedisPassword: input.RedisPassword,
		RedisDB:       input.RedisDB,
		RedisTLS:      input.RedisTLS,
	}

	dbRecord, err := m.UpsertDatabase(ctx, existingDBID, dbReq)
	if err != nil {
		return QuickstartResponse{}, fmt.Errorf("quickstart: connect database: %w", err)
	}

	return m.quickstartWithDatabase(ctx, dbRecord.ID)
}

// quickstartWithDatabase creates the getting-started workspace on an existing database.
func (m *DatabaseManager) quickstartWithDatabase(ctx context.Context, databaseID string) (QuickstartResponse, error) {
	// Check if workspace already exists — if so, just return it.
	existing, err := m.GetWorkspace(ctx, databaseID, quickstartWorkspaceName)
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

	// Create the workspace with seed data.
	service, profile, err := m.serviceFor(ctx, databaseID)
	if err != nil {
		return QuickstartResponse{}, fmt.Errorf("quickstart: open service: %w", err)
	}

	detail, err := createQuickstartWorkspace(ctx, service, profile)
	if err != nil {
		return QuickstartResponse{}, fmt.Errorf("quickstart: create workspace: %w", err)
	}

	if err := m.attachWorkspaceDetailIdentity(ctx, &detail, profile); err != nil {
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

// createQuickstartWorkspace builds a workspace pre-populated with sample files.
func createQuickstartWorkspace(ctx context.Context, service *Service, profile databaseProfile) (workspaceDetail, error) {
	workspace := quickstartWorkspaceName
	now := time.Now().UTC()

	// Build the seed manifest with embedded content.
	manifest, fileCount, dirCount, totalBytes := buildSeedManifest(workspace, now)

	manifestHash, err := HashManifest(manifest)
	if err != nil {
		return workspaceDetail{}, err
	}

	meta := WorkspaceMeta{
		Version:          formatVersion,
		Name:             workspace,
		Description:      "Sample workspace with example files to explore AFS features.",
		DatabaseID:       profile.ID,
		DatabaseName:     profile.Name,
		CloudAccount:     "Direct Redis",
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
		Author:       "afs",
		Workspace:    workspace,
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
	if err := SyncWorkspaceRoot(ctx, store, workspace, manifest); err != nil {
		return workspaceDetail{}, err
	}
	if err := store.Audit(ctx, workspace, "workspace_create", map[string]any{
		"checkpoint": quickstartCheckpoint,
		"source":     quickstartSource,
		"files":      fileCount,
		"dirs":       dirCount,
	}); err != nil {
		return workspaceDetail{}, err
	}

	return service.getWorkspace(ctx, workspace)
}

// buildSeedManifest constructs a Manifest containing the quickstart sample files.
func buildSeedManifest(workspace string, now time.Time) (Manifest, int, int, int64) {
	ms := now.UnixMilli()

	files := map[string]string{
		"/README.md":               seedReadme,
		"/docs/quickstart.md":      seedQuickstart,
		"/docs/architecture.md":    seedArchitecture,
		"/examples/hello.py":       seedHelloPy,
		"/examples/config.json":    seedConfigJSON,
		"/tests/test_hello.py":     seedTestHelloPy,
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
afs workspace use getting-started
afs up
# The workspace appears at ~/afs/getting-started/
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
# Select this workspace
afs workspace use getting-started

# Start the sync daemon — files appear at ~/afs/getting-started/
afs up
` + "```" + `

Your agent can now read and write files in that directory using normal file I/O.

## Step 2: Make changes

Have your agent edit a file:

` + "```bash" + `
echo "print('Modified by agent')" >> ~/afs/getting-started/examples/hello.py
` + "```" + `

AFS syncs the change to Redis automatically.

## Step 3: Create a checkpoint

` + "```bash" + `
afs checkpoint save --name "after-agent-edit" --note "Agent modified hello.py"
` + "```" + `

Or use the web UI — go to the Checkpoints tab and click **Save checkpoint**.

## Step 4: Restore if needed

` + "```bash" + `
# List checkpoints
afs checkpoint list

# Restore to a previous state
afs checkpoint restore initial
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

**1. Redis Module** — A native C module providing ` + "`FS.*`" + ` commands.
One Redis key = one complete filesystem with O(1) path lookups.

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
