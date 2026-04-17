package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/redis/agent-filesystem/internal/controlplane"
)

type httpControlPlaneClient struct {
	baseURL    string
	databaseID string
	client     *http.Client
	importer   *http.Client
}

type httpErrorResponse struct {
	Error string `json:"error"`
}

type httpRestoreCheckpointRequest struct {
	CheckpointID string `json:"checkpoint_id"`
}

type httpForkWorkspaceRequest struct {
	NewWorkspace string `json:"new_workspace"`
}

type httpSaveFromLiveRequest struct {
	CheckpointID string `json:"checkpoint_id"`
}

type httpSaveCheckpointRequest struct {
	ExpectedHead          string                `json:"expected_head"`
	CheckpointID          string                `json:"checkpoint_id"`
	Manifest              controlplane.Manifest `json:"manifest"`
	Blobs                 map[string][]byte     `json:"blobs"`
	FileCount             int                   `json:"file_count"`
	DirCount              int                   `json:"dir_count"`
	TotalBytes            int64                 `json:"total_bytes"`
	SkipWorkspaceRootSync bool                  `json:"skip_workspace_root_sync"`
}

type httpSaveCheckpointResponse struct {
	Saved bool `json:"saved"`
}

func newHTTPControlPlaneClient(ctx context.Context, cfg config) (*httpControlPlaneClient, string, error) {
	_ = ctx
	baseURL, err := normalizeControlPlaneURL(cfg.URL)
	if err != nil {
		return nil, "", err
	}

	client := &httpControlPlaneClient{
		baseURL:    baseURL,
		databaseID: strings.TrimSpace(cfg.DatabaseID),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		importer: &http.Client{
			Timeout: 15 * time.Minute,
		},
	}
	return client, client.databaseID, nil
}

func (c *httpControlPlaneClient) ListWorkspaceSummaries(ctx context.Context) (controlplane.WorkspaceListResponse, error) {
	var out controlplane.WorkspaceListResponse
	err := c.doJSON(ctx, http.MethodGet, "/v1/workspaces", nil, &out, http.StatusOK)
	return out, err
}

func (c *httpControlPlaneClient) GetWorkspace(ctx context.Context, workspace string) (controlplane.WorkspaceDetail, error) {
	var out controlplane.WorkspaceDetail
	path := c.workspacePath(workspace)
	err := c.doJSON(ctx, http.MethodGet, path, nil, &out, http.StatusOK)
	return out, err
}

func (c *httpControlPlaneClient) CreateWorkspace(ctx context.Context, input controlplane.CreateWorkspaceRequest) (controlplane.WorkspaceDetail, error) {
	var out controlplane.WorkspaceDetail
	databaseID, err := c.requireDatabaseID(ctx)
	if err != nil {
		return out, err
	}
	err = c.doJSON(ctx, http.MethodPost, c.scopedPathFor(databaseID, "workspaces"), input, &out, http.StatusCreated)
	return out, err
}

func (c *httpControlPlaneClient) ImportWorkspace(ctx context.Context, input controlplane.ImportWorkspaceRequest) (controlplane.ImportWorkspaceResponse, error) {
	var out controlplane.ImportWorkspaceResponse
	databaseID, err := c.requireDatabaseID(ctx)
	if err != nil {
		return out, err
	}
	err = c.doJSONWithClient(ctx, c.importer, http.MethodPost, c.scopedPathFor(databaseID, "workspaces:import"), input, &out, http.StatusCreated)
	return out, err
}

func (c *httpControlPlaneClient) DeleteWorkspace(ctx context.Context, workspace string) error {
	return c.doJSON(ctx, http.MethodDelete, c.workspacePath(workspace), nil, nil, http.StatusNoContent)
}

func (c *httpControlPlaneClient) CreateWorkspaceSession(ctx context.Context, workspace string, input controlplane.CreateWorkspaceSessionRequest) (controlplane.WorkspaceSession, error) {
	var out controlplane.WorkspaceSession
	path := c.clientWorkspacePath(workspace, "sessions")
	err := c.doJSON(ctx, http.MethodPost, path, input, &out, http.StatusCreated)
	return out, err
}

func (c *httpControlPlaneClient) HeartbeatWorkspaceSession(ctx context.Context, workspace, sessionID string) (controlplane.WorkspaceSessionInfo, error) {
	var out controlplane.WorkspaceSessionInfo
	path := c.clientWorkspacePath(workspace, "sessions", sessionID, "heartbeat")
	err := c.doJSON(ctx, http.MethodPost, path, nil, &out, http.StatusOK)
	return out, err
}

func (c *httpControlPlaneClient) CloseWorkspaceSession(ctx context.Context, workspace, sessionID string) error {
	return c.doJSON(ctx, http.MethodDelete, c.clientWorkspacePath(workspace, "sessions", sessionID), nil, nil, http.StatusNoContent)
}

func (c *httpControlPlaneClient) ListCheckpoints(ctx context.Context, workspace string, limit int) ([]controlplane.CheckpointSummary, error) {
	rel := c.workspacePath(workspace, "checkpoints")
	if limit > 0 {
		rel += "?limit=" + strconv.Itoa(limit)
	}
	var out []controlplane.CheckpointSummary
	err := c.doJSON(ctx, http.MethodGet, rel, nil, &out, http.StatusOK)
	return out, err
}

func (c *httpControlPlaneClient) RestoreCheckpoint(ctx context.Context, workspace, checkpointID string) error {
	return c.doJSON(ctx, http.MethodPost, c.workspacePath(workspace)+":restore", httpRestoreCheckpointRequest{
		CheckpointID: checkpointID,
	}, nil, http.StatusNoContent)
}

func (c *httpControlPlaneClient) SaveCheckpoint(ctx context.Context, input controlplane.SaveCheckpointRequest) (bool, error) {
	var out httpSaveCheckpointResponse
	err := c.doJSON(ctx, http.MethodPost, c.workspacePath(input.Workspace, "checkpoints"), httpSaveCheckpointRequest{
		ExpectedHead:          input.ExpectedHead,
		CheckpointID:          input.CheckpointID,
		Manifest:              input.Manifest,
		Blobs:                 input.Blobs,
		FileCount:             input.FileCount,
		DirCount:              input.DirCount,
		TotalBytes:            input.TotalBytes,
		SkipWorkspaceRootSync: input.SkipWorkspaceRootSync,
	}, &out, http.StatusCreated)
	return out.Saved, err
}

func (c *httpControlPlaneClient) SaveCheckpointFromLive(ctx context.Context, workspace, checkpointID string) (bool, error) {
	var out httpSaveCheckpointResponse
	err := c.doJSON(ctx, http.MethodPost, c.workspacePath(workspace)+":save-from-live", httpSaveFromLiveRequest{
		CheckpointID: checkpointID,
	}, &out, http.StatusCreated)
	return out.Saved, err
}

func (c *httpControlPlaneClient) ForkWorkspace(ctx context.Context, sourceWorkspace, newWorkspace string) error {
	return c.doJSON(ctx, http.MethodPost, c.workspacePath(sourceWorkspace)+":fork", httpForkWorkspaceRequest{
		NewWorkspace: newWorkspace,
	}, nil, http.StatusNoContent)
}

func (c *httpControlPlaneClient) listDatabases(ctx context.Context) (controlplane.DatabaseListResponse, error) {
	var out controlplane.DatabaseListResponse
	err := c.doJSON(ctx, http.MethodGet, "/v1/databases", nil, &out, http.StatusOK)
	return out, err
}

func (c *httpControlPlaneClient) scopedPath(parts ...string) string {
	return c.scopedPathFor(c.databaseID, parts...)
}

func (c *httpControlPlaneClient) scopedPathFor(databaseID string, parts ...string) string {
	escaped := make([]string, 0, len(parts)+2)
	escaped = append(escaped, "/v1/databases", url.PathEscape(databaseID))
	for _, part := range parts {
		escaped = append(escaped, url.PathEscape(part))
	}
	return strings.Join(escaped, "/")
}

func (c *httpControlPlaneClient) clientScopedPath(parts ...string) string {
	return c.clientScopedPathFor(c.databaseID, parts...)
}

func (c *httpControlPlaneClient) clientScopedPathFor(databaseID string, parts ...string) string {
	escaped := make([]string, 0, len(parts)+3)
	escaped = append(escaped, "/v1/client/databases", url.PathEscape(databaseID))
	for _, part := range parts {
		escaped = append(escaped, url.PathEscape(part))
	}
	return strings.Join(escaped, "/")
}

func (c *httpControlPlaneClient) workspacePath(workspace string, more ...string) string {
	return c.unscopedPath("/v1/workspaces", append([]string{workspace}, more...)...)
}

func (c *httpControlPlaneClient) clientWorkspacePath(workspace string, more ...string) string {
	return c.unscopedPath("/v1/client/workspaces", append([]string{workspace}, more...)...)
}

func (c *httpControlPlaneClient) unscopedPath(prefix string, parts ...string) string {
	escaped := make([]string, 0, len(parts)+1)
	escaped = append(escaped, prefix)
	for _, part := range parts {
		escaped = append(escaped, url.PathEscape(part))
	}
	return strings.Join(escaped, "/")
}

func (c *httpControlPlaneClient) hasScopedDatabase() bool {
	return strings.TrimSpace(c.databaseID) != ""
}

func (c *httpControlPlaneClient) requireDatabaseID(ctx context.Context) (string, error) {
	if c.hasScopedDatabase() {
		return c.databaseID, nil
	}
	list, err := c.listDatabases(ctx)
	if err != nil {
		return "", err
	}
	switch len(list.Items) {
	case 0:
		return "", fmt.Errorf("control plane at %s returned no databases", c.baseURL)
	case 1:
		return list.Items[0].ID, nil
	default:
		return "", fmt.Errorf("control plane at %s has %d databases; this operation requires a database choice, so set controlPlane.databaseID or run '%s config set --control-plane-database <id>'", c.baseURL, len(list.Items), filepath.Base(os.Args[0]))
	}
}

func (c *httpControlPlaneClient) doJSON(ctx context.Context, method, rel string, requestBody any, out any, okStatuses ...int) error {
	return c.doJSONWithClient(ctx, c.client, method, rel, requestBody, out, okStatuses...)
}

func (c *httpControlPlaneClient) doJSONWithClient(ctx context.Context, httpClient *http.Client, method, rel string, requestBody any, out any, okStatuses ...int) error {
	var body io.Reader
	if requestBody != nil {
		encoded, err := json.Marshal(requestBody)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+rel, body)
	if err != nil {
		return err
	}
	if requestBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if httpClient == nil {
		httpClient = c.client
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if !containsStatus(okStatuses, resp.StatusCode) {
		return decodeControlPlaneHTTPError(resp)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func containsStatus(allowed []int, got int) bool {
	for _, status := range allowed {
		if status == got {
			return true
		}
	}
	return false
}

func decodeControlPlaneHTTPError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	message := strings.TrimSpace(resp.Status)
	var payload httpErrorResponse
	if err := json.Unmarshal(body, &payload); err == nil && strings.TrimSpace(payload.Error) != "" {
		message = strings.TrimSpace(payload.Error)
	} else if strings.TrimSpace(string(body)) != "" {
		message = strings.TrimSpace(string(body))
	}

	switch resp.StatusCode {
	case http.StatusNotFound:
		return fmt.Errorf("%w: %s", os.ErrNotExist, message)
	case http.StatusConflict:
		return fmt.Errorf("%w: %s", controlplane.ErrWorkspaceConflict, message)
	case http.StatusNotImplemented:
		return fmt.Errorf("%w: %s", controlplane.ErrUnsupportedView, message)
	default:
		return errors.New(message)
	}
}
