package controlplane

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/redis/agent-filesystem/internal/uistatic"
)

type saveCheckpointHTTPResponse struct {
	Saved bool `json:"saved"`
}

type forkWorkspaceRequest struct {
	NewWorkspace string `json:"new_workspace"`
}

type saveCheckpointRequest struct {
	ExpectedHead          string            `json:"expected_head"`
	CheckpointID          string            `json:"checkpoint_id"`
	Manifest              Manifest          `json:"manifest"`
	Blobs                 map[string][]byte `json:"blobs"`
	DirCount              int               `json:"dir_count"`
	FileCount             int               `json:"file_count"`
	TotalBytes            int64             `json:"total_bytes"`
	SkipWorkspaceRootSync bool              `json:"skip_workspace_root_sync"`
}

func NewHandler(manager *DatabaseManager, allowOrigin string) http.Handler {
	root := http.NewServeMux()
	root.Handle("/v1/client/", http.StripPrefix("/v1/client", newClientMux(manager)))

	admin := newAdminMux(manager)

	// Serve embedded UI for non-API paths, falling back to index.html for SPA routes.
	uiFS, err := fs.Sub(uistatic.Content, "dist")
	if err != nil {
		// If the UI is not embedded (e.g. dev build), serve API only.
		root.Handle("/", admin)
		return cors(root, allowOrigin)
	}

	// Check if the embedded UI has real content (not just the placeholder).
	if _, err := fs.Stat(uiFS, "index.html"); err != nil {
		// No UI built — serve API only.
		root.Handle("/", admin)
		return cors(root, allowOrigin)
	}

	fileServer := http.FileServer(http.FS(uiFS))
	spaHandler := &spaFallbackHandler{fs: uiFS, fileServer: fileServer, admin: admin}
	root.Handle("/", spaHandler)
	return cors(root, allowOrigin)
}

// spaFallbackHandler serves static files from the embedded UI filesystem.
// API routes (/v1/, /healthz) are forwarded to the admin mux.
// Non-API paths that don't match a static file get index.html (SPA routing).
type spaFallbackHandler struct {
	fs         fs.FS
	fileServer http.Handler
	admin      http.Handler
}

func (h *spaFallbackHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Route API requests to the admin mux.
	if strings.HasPrefix(r.URL.Path, "/v1/") || r.URL.Path == "/healthz" {
		h.admin.ServeHTTP(w, r)
		return
	}

	// Try serving the file from the embedded FS.
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	if _, err := fs.Stat(h.fs, path); err == nil {
		h.fileServer.ServeHTTP(w, r)
		return
	}

	// SPA fallback: serve index.html for any unmatched route.
	r.URL.Path = "/"
	h.fileServer.ServeHTTP(w, r)
}

func NewAdminHandler(manager *DatabaseManager, allowOrigin string) http.Handler {
	return cors(newAdminMux(manager), allowOrigin)
}

func NewClientHandler(manager *DatabaseManager, allowOrigin string) http.Handler {
	return cors(newClientMux(manager), allowOrigin)
}

func newAdminMux(manager *DatabaseManager) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, fmt.Errorf("%s not allowed", r.Method))
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	mux.HandleFunc("/v1/databases", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			response, err := manager.ListDatabases(r.Context())
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, response)
		case http.MethodPost:
			var input UpsertDatabaseRequest
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				writeError(w, fmt.Errorf("invalid request body: %w", err))
				return
			}
			response, err := manager.UpsertDatabase(r.Context(), "", input)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, response)
		default:
			writeError(w, fmt.Errorf("%s not allowed", r.Method))
		}
	})

	mux.HandleFunc("/v1/catalog/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, fmt.Errorf("%s not allowed", r.Method))
			return
		}
		response, err := manager.CatalogHealth(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, response)
	})

	mux.HandleFunc("/v1/catalog/reconcile", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, fmt.Errorf("%s not allowed", r.Method))
			return
		}
		response, err := manager.ReconcileCatalog(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, response)
	})

	mux.HandleFunc("/v1/cli", handleCLIDownload)

	mux.HandleFunc("/v1/quickstart", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, fmt.Errorf("%s not allowed", r.Method))
			return
		}
		var input QuickstartRequest
		if r.Body != nil {
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil && !errors.Is(err, io.EOF) {
				writeError(w, fmt.Errorf("invalid request body: %w", err))
				return
			}
		}
		response, err := manager.Quickstart(r.Context(), input)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, response)
	})

	mux.HandleFunc("/v1/workspaces", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, fmt.Errorf("%s not allowed", r.Method))
			return
		}
		response, err := manager.ListAllWorkspaceSummaries(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, response)
	})

	mux.HandleFunc("/v1/workspaces/", func(w http.ResponseWriter, r *http.Request) {
		workspacePath := strings.TrimPrefix(r.URL.Path, "/v1/workspaces/")
		workspacePath = strings.Trim(workspacePath, "/")
		if workspacePath == "" {
			writeError(w, os.ErrNotExist)
			return
		}
		handleResolvedWorkspaceRoute(w, r, manager, workspacePath)
	})

	mux.HandleFunc("/v1/databases/", func(w http.ResponseWriter, r *http.Request) {
		trimmed := strings.TrimPrefix(r.URL.Path, "/v1/databases/")
		trimmed = strings.Trim(trimmed, "/")
		if trimmed == "" {
			writeError(w, os.ErrNotExist)
			return
		}

		parts := strings.Split(trimmed, "/")
		databaseID := parts[0]
		rest := strings.Join(parts[1:], "/")

		if rest == "" {
			switch r.Method {
			case http.MethodPut:
				var input UpsertDatabaseRequest
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
					writeError(w, fmt.Errorf("invalid request body: %w", err))
					return
				}
				response, err := manager.UpsertDatabase(r.Context(), databaseID, input)
				if err != nil {
					writeError(w, err)
					return
				}
				writeJSON(w, http.StatusOK, response)
			case http.MethodDelete:
				if err := manager.DeleteDatabase(databaseID); err != nil {
					writeError(w, err)
					return
				}
				w.WriteHeader(http.StatusNoContent)
			default:
				writeError(w, fmt.Errorf("%s not allowed", r.Method))
			}
			return
		}

		switch {
		case rest == "activity":
			if r.Method != http.MethodGet {
				writeError(w, fmt.Errorf("%s not allowed", r.Method))
				return
			}
			limit, err := queryInt(r, "limit", 50)
			if err != nil {
				writeError(w, err)
				return
			}
			response, err := manager.ListGlobalActivity(r.Context(), databaseID, limit)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, response)
		case rest == "workspaces":
			switch r.Method {
			case http.MethodGet:
				response, err := manager.ListWorkspaceSummaries(r.Context(), databaseID)
				if err != nil {
					writeError(w, err)
					return
				}
				writeJSON(w, http.StatusOK, response)
			case http.MethodPost:
				var input CreateWorkspaceRequest
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
					writeError(w, fmt.Errorf("invalid request body: %w", err))
					return
				}
				response, err := manager.CreateWorkspace(r.Context(), databaseID, input)
				if err != nil {
					writeError(w, err)
					return
				}
				writeJSON(w, http.StatusCreated, response)
			default:
				writeError(w, fmt.Errorf("%s not allowed", r.Method))
			}
		case rest == "workspaces:import-local":
			if r.Method != http.MethodPost {
				writeError(w, fmt.Errorf("%s not allowed", r.Method))
				return
			}
			var input ImportLocalRequest
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				writeError(w, fmt.Errorf("invalid request body: %w", err))
				return
			}
			response, err := manager.ImportLocal(r.Context(), databaseID, input)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, response)
		case strings.HasPrefix(rest, "workspaces/"):
			workspacePath := strings.TrimPrefix(rest, "workspaces/")
			workspacePath = strings.Trim(workspacePath, "/")
			if workspacePath == "" {
				writeError(w, os.ErrNotExist)
				return
			}
			handleWorkspaceRoute(w, r, manager, databaseID, workspacePath)
		default:
			writeError(w, os.ErrNotExist)
		}
	})

	return mux
}

func newClientMux(manager *DatabaseManager) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, fmt.Errorf("%s not allowed", r.Method))
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	mux.HandleFunc("/databases/", func(w http.ResponseWriter, r *http.Request) {
		trimmed := strings.TrimPrefix(r.URL.Path, "/databases/")
		trimmed = strings.Trim(trimmed, "/")
		if trimmed == "" {
			writeError(w, os.ErrNotExist)
			return
		}
		parts := strings.Split(trimmed, "/")
		databaseID := parts[0]
		rest := strings.Join(parts[1:], "/")
		switch {
		case strings.HasSuffix(rest, "/sessions"):
			if manager == nil {
				writeError(w, os.ErrNotExist)
				return
			}
			workspacePath := strings.TrimSuffix(rest, "/sessions")
			workspacePath = strings.TrimPrefix(workspacePath, "workspaces/")
			workspacePath = strings.Trim(workspacePath, "/")
			if workspacePath == "" {
				writeError(w, os.ErrNotExist)
				return
			}
			if r.Method != http.MethodPost {
				writeError(w, fmt.Errorf("%s not allowed", r.Method))
				return
			}
			var input createWorkspaceSessionRequest
			if r.Body != nil {
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil && !errors.Is(err, io.EOF) {
					writeError(w, fmt.Errorf("invalid request body: %w", err))
					return
				}
			}
			response, err := manager.CreateWorkspaceSession(r.Context(), databaseID, workspacePath, input)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, response)
		case strings.Contains(rest, "/sessions/") && strings.HasSuffix(rest, "/heartbeat"):
			parts := strings.Split(strings.Trim(rest, "/"), "/")
			if len(parts) != 5 || parts[0] != "workspaces" || parts[2] != "sessions" || parts[4] != "heartbeat" {
				writeError(w, os.ErrNotExist)
				return
			}
			if r.Method != http.MethodPost {
				writeError(w, fmt.Errorf("%s not allowed", r.Method))
				return
			}
			response, err := manager.HeartbeatWorkspaceSession(r.Context(), databaseID, parts[1], parts[3])
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, response)
		case strings.Contains(rest, "/sessions/"):
			parts := strings.Split(strings.Trim(rest, "/"), "/")
			if len(parts) != 4 || parts[0] != "workspaces" || parts[2] != "sessions" {
				writeError(w, os.ErrNotExist)
				return
			}
			if r.Method != http.MethodDelete {
				writeError(w, fmt.Errorf("%s not allowed", r.Method))
				return
			}
			if err := manager.CloseWorkspaceSession(r.Context(), databaseID, parts[1], parts[3]); err != nil {
				writeError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			writeError(w, os.ErrNotExist)
		}
	})
	mux.HandleFunc("/workspaces/", func(w http.ResponseWriter, r *http.Request) {
		workspacePath := strings.TrimPrefix(r.URL.Path, "/workspaces/")
		workspacePath = strings.Trim(workspacePath, "/")
		switch {
		case strings.HasSuffix(workspacePath, "/sessions"):
			if manager == nil {
				writeError(w, os.ErrNotExist)
				return
			}
			workspace := strings.TrimSuffix(workspacePath, "/sessions")
			workspace = strings.Trim(workspace, "/")
			if workspace == "" {
				writeError(w, os.ErrNotExist)
				return
			}
			if r.Method != http.MethodPost {
				writeError(w, fmt.Errorf("%s not allowed", r.Method))
				return
			}
			var input createWorkspaceSessionRequest
			if r.Body != nil {
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil && !errors.Is(err, io.EOF) {
					writeError(w, fmt.Errorf("invalid request body: %w", err))
					return
				}
			}
			response, err := manager.CreateResolvedWorkspaceSession(r.Context(), workspace, input)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, response)
		case strings.Contains(workspacePath, "/sessions/") && strings.HasSuffix(workspacePath, "/heartbeat"):
			parts := strings.Split(strings.Trim(workspacePath, "/"), "/")
			if len(parts) != 4 || parts[1] != "sessions" || parts[3] != "heartbeat" {
				writeError(w, os.ErrNotExist)
				return
			}
			if r.Method != http.MethodPost {
				writeError(w, fmt.Errorf("%s not allowed", r.Method))
				return
			}
			response, err := manager.HeartbeatResolvedWorkspaceSession(r.Context(), parts[0], parts[2])
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, response)
		case strings.Contains(workspacePath, "/sessions/"):
			parts := strings.Split(strings.Trim(workspacePath, "/"), "/")
			if len(parts) != 3 || parts[1] != "sessions" {
				writeError(w, os.ErrNotExist)
				return
			}
			if r.Method != http.MethodDelete {
				writeError(w, fmt.Errorf("%s not allowed", r.Method))
				return
			}
			if err := manager.CloseResolvedWorkspaceSession(r.Context(), parts[0], parts[2]); err != nil {
				writeError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			writeError(w, os.ErrNotExist)
		}
	})
	return mux
}

func handleWorkspaceRoute(
	w http.ResponseWriter,
	r *http.Request,
	manager *DatabaseManager,
	databaseID string,
	workspacePath string,
) {
	switch {
	case strings.HasSuffix(workspacePath, ":fork"):
		workspace := strings.TrimSuffix(workspacePath, ":fork")
		if r.Method != http.MethodPost {
			writeError(w, fmt.Errorf("%s not allowed", r.Method))
			return
		}
		var input forkWorkspaceRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if err := manager.ForkWorkspace(r.Context(), databaseID, workspace, input.NewWorkspace); err != nil {
			writeError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case strings.HasSuffix(workspacePath, ":restore"):
		workspace := strings.TrimSuffix(workspacePath, ":restore")
		if r.Method != http.MethodPost {
			writeError(w, fmt.Errorf("%s not allowed", r.Method))
			return
		}
		var input restoreCheckpointRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if err := manager.RestoreCheckpoint(r.Context(), databaseID, workspace, input.CheckpointID); err != nil {
			writeError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case strings.HasSuffix(workspacePath, "/checkpoints"):
		workspace := strings.TrimSuffix(workspacePath, "/checkpoints")
		switch r.Method {
		case http.MethodGet:
			limit, err := queryInt(r, "limit", 100)
			if err != nil {
				writeError(w, err)
				return
			}
			response, err := manager.ListCheckpoints(r.Context(), databaseID, workspace, limit)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, response)
		case http.MethodPost:
			var input saveCheckpointRequest
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				writeError(w, fmt.Errorf("invalid request body: %w", err))
				return
			}
			saved, err := manager.SaveCheckpoint(r.Context(), databaseID, workspace, SaveCheckpointRequest{
				Workspace:             workspace,
				ExpectedHead:          input.ExpectedHead,
				CheckpointID:          input.CheckpointID,
				Manifest:              input.Manifest,
				Blobs:                 input.Blobs,
				FileCount:             input.FileCount,
				DirCount:              input.DirCount,
				TotalBytes:            input.TotalBytes,
				SkipWorkspaceRootSync: input.SkipWorkspaceRootSync,
			})
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, saveCheckpointHTTPResponse{Saved: saved})
		default:
			writeError(w, fmt.Errorf("%s not allowed", r.Method))
		}
	case strings.HasSuffix(workspacePath, "/tree"):
		workspace := strings.TrimSuffix(workspacePath, "/tree")
		if r.Method != http.MethodGet {
			writeError(w, fmt.Errorf("%s not allowed", r.Method))
			return
		}
		depth, err := queryInt(r, "depth", 1)
		if err != nil {
			writeError(w, err)
			return
		}
		response, err := manager.GetTree(
			r.Context(),
			databaseID,
			workspace,
			r.URL.Query().Get("view"),
			r.URL.Query().Get("path"),
			depth,
		)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, response)
	case strings.HasSuffix(workspacePath, "/files/content"):
		workspace := strings.TrimSuffix(workspacePath, "/files/content")
		if r.Method != http.MethodGet {
			writeError(w, fmt.Errorf("%s not allowed", r.Method))
			return
		}
		response, err := manager.GetFileContent(
			r.Context(),
			databaseID,
			workspace,
			r.URL.Query().Get("view"),
			r.URL.Query().Get("path"),
		)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, response)
	case strings.HasSuffix(workspacePath, "/activity"):
		workspace := strings.TrimSuffix(workspacePath, "/activity")
		if r.Method != http.MethodGet {
			writeError(w, fmt.Errorf("%s not allowed", r.Method))
			return
		}
		limit, err := queryInt(r, "limit", 50)
		if err != nil {
			writeError(w, err)
			return
		}
		response, err := manager.ListWorkspaceActivity(r.Context(), databaseID, workspace, limit)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, response)
	case strings.HasSuffix(workspacePath, "/sessions"):
		workspace := strings.TrimSuffix(workspacePath, "/sessions")
		if r.Method != http.MethodGet {
			writeError(w, fmt.Errorf("%s not allowed", r.Method))
			return
		}
		response, err := manager.ListWorkspaceSessions(r.Context(), databaseID, workspace)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, response)
	default:
		workspace := workspacePath
		switch r.Method {
		case http.MethodGet:
			response, err := manager.GetWorkspace(r.Context(), databaseID, workspace)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, response)
		case http.MethodPut:
			var input UpdateWorkspaceRequest
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				writeError(w, fmt.Errorf("invalid request body: %w", err))
				return
			}
			response, err := manager.UpdateWorkspace(r.Context(), databaseID, workspace, input)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, response)
		case http.MethodDelete:
			if err := manager.DeleteWorkspace(r.Context(), databaseID, workspace); err != nil {
				writeError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			writeError(w, fmt.Errorf("%s not allowed", r.Method))
		}
	}
}

func handleResolvedWorkspaceRoute(
	w http.ResponseWriter,
	r *http.Request,
	manager *DatabaseManager,
	workspacePath string,
) {
	switch {
	case strings.HasSuffix(workspacePath, ":fork"):
		workspace := strings.TrimSuffix(workspacePath, ":fork")
		if r.Method != http.MethodPost {
			writeError(w, fmt.Errorf("%s not allowed", r.Method))
			return
		}
		var input forkWorkspaceRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if err := manager.ForkResolvedWorkspace(r.Context(), workspace, input.NewWorkspace); err != nil {
			writeError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case strings.HasSuffix(workspacePath, ":restore"):
		workspace := strings.TrimSuffix(workspacePath, ":restore")
		if r.Method != http.MethodPost {
			writeError(w, fmt.Errorf("%s not allowed", r.Method))
			return
		}
		var input restoreCheckpointRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if err := manager.RestoreResolvedCheckpoint(r.Context(), workspace, input.CheckpointID); err != nil {
			writeError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case strings.HasSuffix(workspacePath, "/checkpoints"):
		workspace := strings.TrimSuffix(workspacePath, "/checkpoints")
		switch r.Method {
		case http.MethodGet:
			limit, err := queryInt(r, "limit", 100)
			if err != nil {
				writeError(w, err)
				return
			}
			response, err := manager.ListResolvedCheckpoints(r.Context(), workspace, limit)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, response)
		case http.MethodPost:
			var input saveCheckpointRequest
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				writeError(w, fmt.Errorf("invalid request body: %w", err))
				return
			}
			saved, err := manager.SaveResolvedCheckpoint(r.Context(), workspace, SaveCheckpointRequest{
				Workspace:             workspace,
				ExpectedHead:          input.ExpectedHead,
				CheckpointID:          input.CheckpointID,
				Manifest:              input.Manifest,
				Blobs:                 input.Blobs,
				FileCount:             input.FileCount,
				DirCount:              input.DirCount,
				TotalBytes:            input.TotalBytes,
				SkipWorkspaceRootSync: input.SkipWorkspaceRootSync,
			})
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, saveCheckpointHTTPResponse{Saved: saved})
		default:
			writeError(w, fmt.Errorf("%s not allowed", r.Method))
		}
	case strings.HasSuffix(workspacePath, "/tree"):
		workspace := strings.TrimSuffix(workspacePath, "/tree")
		if r.Method != http.MethodGet {
			writeError(w, fmt.Errorf("%s not allowed", r.Method))
			return
		}
		depth, err := queryInt(r, "depth", 1)
		if err != nil {
			writeError(w, err)
			return
		}
		response, err := manager.GetResolvedTree(
			r.Context(),
			workspace,
			r.URL.Query().Get("view"),
			r.URL.Query().Get("path"),
			depth,
		)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, response)
	case strings.HasSuffix(workspacePath, "/files/content"):
		workspace := strings.TrimSuffix(workspacePath, "/files/content")
		if r.Method != http.MethodGet {
			writeError(w, fmt.Errorf("%s not allowed", r.Method))
			return
		}
		response, err := manager.GetResolvedFileContent(
			r.Context(),
			workspace,
			r.URL.Query().Get("view"),
			r.URL.Query().Get("path"),
		)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, response)
	case strings.HasSuffix(workspacePath, "/activity"):
		workspace := strings.TrimSuffix(workspacePath, "/activity")
		if r.Method != http.MethodGet {
			writeError(w, fmt.Errorf("%s not allowed", r.Method))
			return
		}
		limit, err := queryInt(r, "limit", 50)
		if err != nil {
			writeError(w, err)
			return
		}
		response, err := manager.ListResolvedWorkspaceActivity(r.Context(), workspace, limit)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, response)
	case strings.HasSuffix(workspacePath, "/sessions"):
		workspace := strings.TrimSuffix(workspacePath, "/sessions")
		if r.Method != http.MethodGet {
			writeError(w, fmt.Errorf("%s not allowed", r.Method))
			return
		}
		response, err := manager.ListResolvedWorkspaceSessions(r.Context(), workspace)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, response)
	default:
		workspace := workspacePath
		switch r.Method {
		case http.MethodGet:
			response, err := manager.GetResolvedWorkspace(r.Context(), workspace)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, response)
		case http.MethodPut:
			var input UpdateWorkspaceRequest
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				writeError(w, fmt.Errorf("invalid request body: %w", err))
				return
			}
			response, err := manager.UpdateResolvedWorkspace(r.Context(), workspace, input)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, response)
		case http.MethodDelete:
			if err := manager.DeleteResolvedWorkspace(r.Context(), workspace); err != nil {
				writeError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			writeError(w, fmt.Errorf("%s not allowed", r.Method))
		}
	}
}

// handleCLIDownload serves the afs CLI binary from the same directory as the
// control plane binary. This lets new users install with a single curl command.
// On macOS the binary is ad-hoc code-signed before serving so that Gatekeeper
// does not kill it on the client side.
func handleCLIDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, fmt.Errorf("%s not allowed", r.Method))
		return
	}

	target, err := normalizeCLITarget(r.URL.Query().Get("os"), r.URL.Query().Get("arch"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid CLI target: " + err.Error(),
		})
		return
	}

	binaryPath, cleanupBuild, err := resolveCLIBinaryForTarget(target)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "CLI binary not found: " + err.Error(),
		})
		return
	}
	if cleanupBuild != nil {
		defer cleanupBuild()
	}

	servePath, cleanupSign, err := ensureCodeSigned(binaryPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to prepare CLI binary: " + err.Error(),
		})
		return
	}
	if cleanupSign != nil {
		defer cleanupSign()
	}

	info, err := os.Stat(servePath)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "CLI binary not available",
		})
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, target.Filename))
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	http.ServeFile(w, r, servePath)
}

// findCLIBinary looks for the afs binary next to the running control plane binary,
// then falls back to well-known locations.
func findCLIBinary() (string, error) {
	// Try sibling of this executable.
	exe, err := executablePath()
	if err == nil {
		sibling := filepath.Join(filepath.Dir(exe), "afs")
		if info, err := os.Stat(sibling); err == nil && !info.IsDir() {
			return sibling, nil
		}
	}

	// Try well-known paths (Docker, /usr/local/bin).
	for _, path := range []string{"/usr/local/bin/afs", "/usr/bin/afs"} {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}

	return "", fmt.Errorf("afs binary not found next to control plane or in /usr/local/bin")
}

// ensureCodeSigned returns a path to a binary with a valid ad-hoc code
// signature. On non-macOS platforms or when the binary already verifies, the
// original path is returned. On macOS with an invalid signature, a temp copy
// is created and signed; the caller must invoke the returned cleanup func.
// If signing fails (e.g. structurally invalid binary), the original is
// returned as-is so non-macOS clients can still use it.
func ensureCodeSigned(binaryPath string) (servePath string, cleanup func(), _ error) {
	if runtime.GOOS != "darwin" {
		return binaryPath, nil, nil
	}

	// Check if the binary already has a valid signature.
	if err := exec.Command("codesign", "-v", binaryPath).Run(); err == nil {
		return binaryPath, nil, nil
	}

	// Copy to a temp file and ad-hoc sign it.
	tmp, err := os.CreateTemp("", "afs-signed-*")
	if err != nil {
		// Can't create temp — serve the original and hope for the best.
		return binaryPath, nil, nil
	}
	tmpPath := tmp.Name()

	src, err := os.Open(binaryPath)
	if err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return binaryPath, nil, nil
	}
	if _, err := io.Copy(tmp, src); err != nil {
		src.Close()
		tmp.Close()
		os.Remove(tmpPath)
		return binaryPath, nil, nil
	}
	src.Close()
	tmp.Close()

	if err := os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		return binaryPath, nil, nil
	}

	if err := exec.Command("codesign", "-s", "-", "--force", tmpPath).Run(); err != nil {
		// Signing failed (binary may be structurally invalid). Serve the
		// original — it will work on Linux and the install script can
		// re-sign on the client side if needed.
		os.Remove(tmpPath)
		return binaryPath, nil, nil
	}

	return tmpPath, func() { os.Remove(tmpPath) }, nil
}

func cors(next http.Handler, allowOrigin string) http.Handler {
	if strings.TrimSpace(allowOrigin) == "" {
		allowOrigin = "*"
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func queryInt(r *http.Request, key string, fallback int) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s value %q", key, raw)
	}
	return value, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case err == nil:
		status = http.StatusInternalServerError
	case errors.Is(err, os.ErrNotExist):
		status = http.StatusNotFound
	case errors.Is(err, ErrAmbiguousWorkspace):
		status = http.StatusBadRequest
	case errors.Is(err, ErrWorkspaceConflict):
		status = http.StatusConflict
	case errors.Is(err, ErrUnsupportedView):
		status = http.StatusNotImplemented
	case strings.Contains(strings.ToLower(err.Error()), "already exists"):
		status = http.StatusConflict
	case strings.Contains(strings.ToLower(err.Error()), "invalid"),
		strings.Contains(strings.ToLower(err.Error()), "unsupported"),
		strings.Contains(strings.ToLower(err.Error()), "required"),
		strings.Contains(strings.ToLower(err.Error()), "not a directory"),
		strings.Contains(strings.ToLower(err.Error()), "is a directory"):
		status = http.StatusBadRequest
	case strings.Contains(strings.ToLower(err.Error()), "not allowed"):
		status = http.StatusMethodNotAllowed
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
