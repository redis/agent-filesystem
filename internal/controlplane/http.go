package controlplane

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
)

func NewHandler(manager *DatabaseManager, allowOrigin string) http.Handler {
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

	return cors(mux, allowOrigin)
}

func handleWorkspaceRoute(
	w http.ResponseWriter,
	r *http.Request,
	manager *DatabaseManager,
	databaseID string,
	workspacePath string,
) {
	switch {
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
