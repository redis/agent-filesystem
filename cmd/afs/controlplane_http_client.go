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
	authToken  string
	client     *http.Client
	importer   *http.Client
}

type httpErrorResponse struct {
	Error string `json:"error"`
}

type httpRestoreCheckpointRequest struct {
	CheckpointID string `json:"checkpoint_id"`
}

type httpRestoreFileVersionRequest struct {
	Path      string `json:"path"`
	VersionID string `json:"version_id,omitempty"`
	FileID    string `json:"file_id,omitempty"`
	Ordinal   int64  `json:"ordinal,omitempty"`
}

type httpDiffFileVersionsRequest struct {
	Path string                              `json:"path"`
	From controlplane.FileVersionDiffOperand `json:"from"`
	To   controlplane.FileVersionDiffOperand `json:"to"`
}

type httpUndeleteFileVersionRequest struct {
	Path      string `json:"path"`
	VersionID string `json:"version_id,omitempty"`
	FileID    string `json:"file_id,omitempty"`
	Ordinal   int64  `json:"ordinal,omitempty"`
}

type httpForkWorkspaceRequest struct {
	NewWorkspace string `json:"new_workspace"`
}

type httpSaveFromLiveRequest struct {
	CheckpointID   string `json:"checkpoint_id"`
	Description    string `json:"description,omitempty"`
	Kind           string `json:"kind,omitempty"`
	Source         string `json:"source,omitempty"`
	Author         string `json:"author,omitempty"`
	AllowUnchanged bool   `json:"allow_unchanged,omitempty"`
}

type httpSaveCheckpointRequest struct {
	ExpectedHead          string                `json:"expected_head"`
	CheckpointID          string                `json:"checkpoint_id"`
	Description           string                `json:"description,omitempty"`
	Kind                  string                `json:"kind,omitempty"`
	Source                string                `json:"source,omitempty"`
	Author                string                `json:"author,omitempty"`
	Manifest              controlplane.Manifest `json:"manifest"`
	Blobs                 map[string][]byte     `json:"blobs"`
	FileCount             int                   `json:"file_count"`
	DirCount              int                   `json:"dir_count"`
	TotalBytes            int64                 `json:"total_bytes"`
	SkipWorkspaceRootSync bool                  `json:"skip_workspace_root_sync"`
	AllowUnchanged        bool                  `json:"allow_unchanged,omitempty"`
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
	productMode, err := effectiveProductMode(cfg)
	if err != nil {
		return nil, "", err
	}

	client := &httpControlPlaneClient{
		baseURL:    baseURL,
		databaseID: strings.TrimSpace(cfg.DatabaseID),
		authToken:  "",
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		importer: &http.Client{
			Timeout: 15 * time.Minute,
		},
	}
	if productMode == productModeCloud {
		client.authToken = strings.TrimSpace(cfg.AuthToken)
	}
	return client, client.databaseID, nil
}

// Ping makes a cheap GET to /healthz. Used to verify a control plane URL
// during login before persisting it to config.
func (c *httpControlPlaneClient) Ping(ctx context.Context) error {
	return c.doJSON(ctx, http.MethodGet, "/healthz", nil, nil, http.StatusOK)
}

func newAnonymousHTTPControlPlaneClient(baseURL string) (*httpControlPlaneClient, error) {
	normalized, err := normalizeControlPlaneURL(baseURL)
	if err != nil {
		return nil, err
	}
	return &httpControlPlaneClient{
		baseURL: normalized,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		importer: &http.Client{
			Timeout: 15 * time.Minute,
		},
	}, nil
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

func (c *httpControlPlaneClient) GetWorkspaceVersioningPolicy(ctx context.Context, workspace string) (controlplane.WorkspaceVersioningPolicy, error) {
	var out controlplane.WorkspaceVersioningPolicy
	err := c.doJSON(ctx, http.MethodGet, c.workspacePath(workspace, "versioning"), nil, &out, http.StatusOK)
	return out, err
}

func (c *httpControlPlaneClient) GetFileHistory(ctx context.Context, workspace, rawPath string, newestFirst bool) (controlplane.FileHistoryResponse, error) {
	return c.GetFileHistoryPage(ctx, workspace, controlplane.FileHistoryRequest{
		Path:        rawPath,
		NewestFirst: newestFirst,
	})
}

func (c *httpControlPlaneClient) GetFileHistoryPage(ctx context.Context, workspace string, req controlplane.FileHistoryRequest) (controlplane.FileHistoryResponse, error) {
	var out controlplane.FileHistoryResponse
	params := url.Values{}
	params.Set("path", req.Path)
	params.Set("direction", ternaryString(req.NewestFirst, "desc", "asc"))
	if req.Limit > 0 {
		params.Set("limit", strconv.Itoa(req.Limit))
	}
	if strings.TrimSpace(req.Cursor) != "" {
		params.Set("cursor", strings.TrimSpace(req.Cursor))
	}
	rel := c.workspacePath(workspace, "files", "history") + "?" + params.Encode()
	err := c.doJSON(ctx, http.MethodGet, rel, nil, &out, http.StatusOK)
	return out, err
}

func (c *httpControlPlaneClient) GetFileVersionContent(ctx context.Context, workspace, versionID string) (controlplane.FileVersionContentResponse, error) {
	var out controlplane.FileVersionContentResponse
	params := url.Values{}
	params.Set("version_id", versionID)
	rel := c.workspacePath(workspace, "files", "version-content") + "?" + params.Encode()
	err := c.doJSON(ctx, http.MethodGet, rel, nil, &out, http.StatusOK)
	return out, err
}

func (c *httpControlPlaneClient) GetFileVersionContentAtOrdinal(ctx context.Context, workspace, fileID string, ordinal int64) (controlplane.FileVersionContentResponse, error) {
	var out controlplane.FileVersionContentResponse
	params := url.Values{}
	params.Set("file_id", fileID)
	params.Set("ordinal", strconv.FormatInt(ordinal, 10))
	rel := c.workspacePath(workspace, "files", "version-content") + "?" + params.Encode()
	err := c.doJSON(ctx, http.MethodGet, rel, nil, &out, http.StatusOK)
	return out, err
}

func (c *httpControlPlaneClient) DiffFileVersions(ctx context.Context, workspace, rawPath string, from, to controlplane.FileVersionDiffOperand) (controlplane.FileVersionDiffResponse, error) {
	var out controlplane.FileVersionDiffResponse
	err := c.doJSON(ctx, http.MethodPost, c.workspacePath(workspace, "files", "diff"), httpDiffFileVersionsRequest{
		Path: rawPath,
		From: from,
		To:   to,
	}, &out, http.StatusOK)
	return out, err
}

func (c *httpControlPlaneClient) RestoreFileVersion(ctx context.Context, workspace, rawPath string, selector controlplane.FileVersionSelector) (controlplane.FileVersionRestoreResponse, error) {
	var out controlplane.FileVersionRestoreResponse
	err := c.doJSON(ctx, http.MethodPost, c.workspacePath(workspace)+":restore-version", httpRestoreFileVersionRequest{
		Path:      rawPath,
		VersionID: selector.VersionID,
		FileID:    selector.FileID,
		Ordinal:   selector.Ordinal,
	}, &out, http.StatusOK)
	return out, err
}

func (c *httpControlPlaneClient) UndeleteFileVersion(ctx context.Context, workspace, rawPath string, selector controlplane.FileVersionSelector) (controlplane.FileVersionUndeleteResponse, error) {
	var out controlplane.FileVersionUndeleteResponse
	err := c.doJSON(ctx, http.MethodPost, c.workspacePath(workspace)+":undelete", httpUndeleteFileVersionRequest{
		Path:      rawPath,
		VersionID: selector.VersionID,
		FileID:    selector.FileID,
		Ordinal:   selector.Ordinal,
	}, &out, http.StatusOK)
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

func (c *httpControlPlaneClient) UpdateWorkspaceVersioningPolicy(ctx context.Context, workspace string, policy controlplane.WorkspaceVersioningPolicy) (controlplane.WorkspaceVersioningPolicy, error) {
	var out controlplane.WorkspaceVersioningPolicy
	err := c.doJSON(ctx, http.MethodPut, c.workspacePath(workspace, "versioning"), policy, &out, http.StatusOK)
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
	path := c.clientWorkspaceSessionPath(workspace, "sessions", sessionID, "heartbeat")
	err := c.doJSON(ctx, http.MethodPost, path, nil, &out, http.StatusOK)
	return out, err
}

func (c *httpControlPlaneClient) CloseWorkspaceSession(ctx context.Context, workspace, sessionID string) error {
	return c.doJSON(ctx, http.MethodDelete, c.clientWorkspaceSessionPath(workspace, "sessions", sessionID), nil, nil, http.StatusNoContent)
}

func (c *httpControlPlaneClient) ListChangelog(ctx context.Context, workspace string, req controlplane.ChangelogListRequest) (controlplane.ChangelogListResponse, error) {
	databaseID, err := c.requireDatabaseID(ctx)
	if err != nil {
		return controlplane.ChangelogListResponse{}, err
	}
	rel := c.scopedPathFor(databaseID, "workspaces", workspace, "changes")
	params := url.Values{}
	if req.Limit > 0 {
		params.Set("limit", strconv.Itoa(req.Limit))
	}
	if strings.TrimSpace(req.SessionID) != "" {
		params.Set("session_id", req.SessionID)
	}
	if strings.TrimSpace(req.Since) != "" {
		params.Set("since", req.Since)
	}
	if strings.TrimSpace(req.Until) != "" {
		params.Set("until", req.Until)
	}
	if req.Reverse {
		params.Set("direction", "desc")
	}
	if encoded := params.Encode(); encoded != "" {
		rel += "?" + encoded
	}
	var out controlplane.ChangelogListResponse
	if err := c.doJSON(ctx, http.MethodGet, rel, nil, &out, http.StatusOK); err != nil {
		return controlplane.ChangelogListResponse{}, err
	}
	return out, nil
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

func (c *httpControlPlaneClient) GetCheckpoint(ctx context.Context, workspace, checkpointID string) (controlplane.CheckpointDetail, error) {
	var out controlplane.CheckpointDetail
	err := c.doJSON(ctx, http.MethodGet, c.workspacePath(workspace, "checkpoints", checkpointID), nil, &out, http.StatusOK)
	return out, err
}

func (c *httpControlPlaneClient) GetTree(ctx context.Context, workspace, view, treePath string, depth int) (controlplane.TreeResponse, error) {
	params := url.Values{}
	if strings.TrimSpace(view) != "" {
		params.Set("view", strings.TrimSpace(view))
	}
	if strings.TrimSpace(treePath) != "" {
		params.Set("path", strings.TrimSpace(treePath))
	}
	if depth > 0 {
		params.Set("depth", strconv.Itoa(depth))
	}
	rel := c.workspacePath(workspace, "tree")
	if encoded := params.Encode(); encoded != "" {
		rel += "?" + encoded
	}
	var out controlplane.TreeResponse
	err := c.doJSON(ctx, http.MethodGet, rel, nil, &out, http.StatusOK)
	return out, err
}

func (c *httpControlPlaneClient) GetFileContent(ctx context.Context, workspace, view, filePath string) (controlplane.FileContentResponse, error) {
	params := url.Values{}
	if strings.TrimSpace(view) != "" {
		params.Set("view", strings.TrimSpace(view))
	}
	if strings.TrimSpace(filePath) != "" {
		params.Set("path", strings.TrimSpace(filePath))
	}
	rel := c.workspacePath(workspace, "files", "content")
	if encoded := params.Encode(); encoded != "" {
		rel += "?" + encoded
	}
	var out controlplane.FileContentResponse
	err := c.doJSON(ctx, http.MethodGet, rel, nil, &out, http.StatusOK)
	return out, err
}

func (c *httpControlPlaneClient) DiffWorkspace(ctx context.Context, workspace, baseView, headView string) (controlplane.WorkspaceDiffResponse, error) {
	params := url.Values{}
	if strings.TrimSpace(baseView) != "" {
		params.Set("base", strings.TrimSpace(baseView))
	}
	if strings.TrimSpace(headView) != "" {
		params.Set("head", strings.TrimSpace(headView))
	}
	rel := c.workspacePath(workspace, "diff")
	if encoded := params.Encode(); encoded != "" {
		rel += "?" + encoded
	}
	var out controlplane.WorkspaceDiffResponse
	err := c.doJSON(ctx, http.MethodGet, rel, nil, &out, http.StatusOK)
	return out, err
}

func (c *httpControlPlaneClient) RestoreCheckpoint(ctx context.Context, workspace, checkpointID string) error {
	_, err := c.RestoreCheckpointWithResult(ctx, workspace, checkpointID)
	return err
}

func (c *httpControlPlaneClient) RestoreCheckpointWithResult(ctx context.Context, workspace, checkpointID string) (controlplane.RestoreCheckpointResult, error) {
	var out controlplane.RestoreCheckpointResult
	err := c.doJSON(ctx, http.MethodPost, c.workspacePath(workspace)+":restore", httpRestoreCheckpointRequest{
		CheckpointID: checkpointID,
	}, &out, http.StatusOK)
	return out, err
}

func (c *httpControlPlaneClient) SaveCheckpoint(ctx context.Context, input controlplane.SaveCheckpointRequest) (bool, error) {
	var out httpSaveCheckpointResponse
	err := c.doJSON(ctx, http.MethodPost, c.workspacePath(input.Workspace, "checkpoints"), httpSaveCheckpointRequest{
		ExpectedHead:          input.ExpectedHead,
		CheckpointID:          input.CheckpointID,
		Description:           input.Description,
		Kind:                  input.Kind,
		Source:                input.Source,
		Author:                input.Author,
		Manifest:              input.Manifest,
		Blobs:                 input.Blobs,
		FileCount:             input.FileCount,
		DirCount:              input.DirCount,
		TotalBytes:            input.TotalBytes,
		SkipWorkspaceRootSync: input.SkipWorkspaceRootSync,
		AllowUnchanged:        input.AllowUnchanged,
	}, &out, http.StatusCreated)
	return out.Saved, err
}

func (c *httpControlPlaneClient) SaveCheckpointFromLive(ctx context.Context, workspace, checkpointID string) (bool, error) {
	return c.SaveCheckpointFromLiveWithOptions(ctx, workspace, checkpointID, controlplane.SaveCheckpointFromLiveOptions{})
}

func (c *httpControlPlaneClient) SaveCheckpointFromLiveWithOptions(ctx context.Context, workspace, checkpointID string, options controlplane.SaveCheckpointFromLiveOptions) (bool, error) {
	if strings.TrimSpace(options.Kind) == "" {
		options.Kind = controlplane.CheckpointKindManual
	}
	if strings.TrimSpace(options.Source) == "" {
		options.Source = controlplane.CheckpointSourceCLI
	}
	var out httpSaveCheckpointResponse
	err := c.doJSON(ctx, http.MethodPost, c.workspacePath(workspace)+":save-from-live", httpSaveFromLiveRequest{
		CheckpointID:   checkpointID,
		Description:    options.Description,
		Kind:           options.Kind,
		Source:         options.Source,
		Author:         options.Author,
		AllowUnchanged: options.AllowUnchanged,
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

func (c *httpControlPlaneClient) exchangeOnboardingToken(ctx context.Context, token string) (authExchangeResponse, error) {
	var out authExchangeResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/auth/exchange", map[string]string{
		"token": strings.TrimSpace(token),
	}, &out, http.StatusOK)
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

func (c *httpControlPlaneClient) clientWorkspaceSessionPath(workspace string, more ...string) string {
	if c.hasScopedDatabase() {
		return c.clientScopedPath(append([]string{"workspaces", workspace}, more...)...)
	}
	return c.clientWorkspacePath(workspace, more...)
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
	if strings.TrimSpace(c.authToken) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.authToken))
	}
	if sid := sessionIDFromContext(ctx); sid != "" {
		req.Header.Set(controlplane.SessionIDHeader, sid)
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
