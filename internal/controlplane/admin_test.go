package controlplane

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCloudAdminAllowlistAuthorizesAdminEndpoints(t *testing.T) {
	t.Setenv(ProductModeEnvVar, ProductModeCloud)
	t.Setenv(authAdminSubjectsEnvVar, "admin-user")

	manager, databaseID := newTestManager(t)
	createOwnedTestWorkspace(t, manager, databaseID, "alice-user", "Alice", 1, "alice-repo")
	createOwnedTestWorkspace(t, manager, databaseID, "bob-user", "Bob", 2, "bob-repo")

	auth := newTrustedHeaderTestAuth(t)
	server := httptest.NewServer(NewHandlerWithOptions(manager, HandlerOptions{
		AllowOrigin: "*",
		Auth:        auth,
	}))
	defer server.Close()

	configReq, err := http.NewRequest(http.MethodGet, server.URL+"/v1/auth/config", nil)
	if err != nil {
		t.Fatalf("NewRequest(auth config) returned error: %v", err)
	}
	configReq.Header.Set("X-Forwarded-User", "admin-user")
	configResp, err := http.DefaultClient.Do(configReq)
	if err != nil {
		t.Fatalf("GET /v1/auth/config returned error: %v", err)
	}
	defer configResp.Body.Close()
	var config authRuntimeConfigResponse
	if err := json.NewDecoder(configResp.Body).Decode(&config); err != nil {
		t.Fatalf("Decode(auth config) returned error: %v", err)
	}
	if config.User == nil || !config.User.IsAdmin {
		t.Fatalf("auth config user = %#v, want admin", config.User)
	}

	for _, path := range []string{
		"/v1/admin/overview",
		"/v1/admin/users",
		"/v1/admin/databases",
		"/v1/admin/workspaces",
		"/v1/admin/agents",
	} {
		req, err := http.NewRequest(http.MethodGet, server.URL+path, nil)
		if err != nil {
			t.Fatalf("NewRequest(%s) returned error: %v", path, err)
		}
		req.Header.Set("X-Forwarded-User", "admin-user")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET %s returned error: %v", path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s status = %d, want %d, body=%s", path, resp.StatusCode, http.StatusOK, body)
		}
	}

	req, err := http.NewRequest(http.MethodGet, server.URL+"/v1/admin/users", nil)
	if err != nil {
		t.Fatalf("NewRequest(admin users) returned error: %v", err)
	}
	req.Header.Set("X-Forwarded-User", "admin-user")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /v1/admin/users returned error: %v", err)
	}
	defer resp.Body.Close()
	var users adminUserListResponse
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		t.Fatalf("Decode(admin users) returned error: %v", err)
	}
	if !adminUsersContain(users.Items, "alice-user") || !adminUsersContain(users.Items, "bob-user") {
		t.Fatalf("admin users = %#v, want alice-user and bob-user", users.Items)
	}
}

func TestCloudAdminEndpointsRejectNonAdmin(t *testing.T) {
	t.Setenv(ProductModeEnvVar, ProductModeCloud)
	t.Setenv(authAdminSubjectsEnvVar, "admin-user")

	manager, _ := newTestManager(t)
	auth := newTrustedHeaderTestAuth(t)
	server := httptest.NewServer(NewHandlerWithOptions(manager, HandlerOptions{
		AllowOrigin: "*",
		Auth:        auth,
	}))
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/v1/admin/overview", nil)
	if err != nil {
		t.Fatalf("NewRequest(admin overview) returned error: %v", err)
	}
	req.Header.Set("X-Forwarded-User", "regular-user")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /v1/admin/overview returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /v1/admin/overview status = %d, want %d, body=%s", resp.StatusCode, http.StatusForbidden, body)
	}

	configReq, err := http.NewRequest(http.MethodGet, server.URL+"/v1/auth/config", nil)
	if err != nil {
		t.Fatalf("NewRequest(auth config) returned error: %v", err)
	}
	configReq.Header.Set("X-Forwarded-User", "regular-user")
	configResp, err := http.DefaultClient.Do(configReq)
	if err != nil {
		t.Fatalf("GET /v1/auth/config returned error: %v", err)
	}
	defer configResp.Body.Close()
	var config authRuntimeConfigResponse
	if err := json.NewDecoder(configResp.Body).Decode(&config); err != nil {
		t.Fatalf("Decode(auth config) returned error: %v", err)
	}
	if config.User != nil && config.User.IsAdmin {
		t.Fatalf("regular user auth config = %#v, want non-admin", config.User)
	}
}

func TestSelfHostedIgnoresAdminAllowlist(t *testing.T) {
	t.Setenv(ProductModeEnvVar, ProductModeSelfHosted)
	t.Setenv(authAdminSubjectsEnvVar, "admin-user")

	manager, _ := newTestManager(t)
	auth := newTrustedHeaderTestAuth(t)
	server := httptest.NewServer(NewHandlerWithOptions(manager, HandlerOptions{
		AllowOrigin: "*",
		Auth:        auth,
	}))
	defer server.Close()

	configReq, err := http.NewRequest(http.MethodGet, server.URL+"/v1/auth/config", nil)
	if err != nil {
		t.Fatalf("NewRequest(auth config) returned error: %v", err)
	}
	configReq.Header.Set("X-Forwarded-User", "admin-user")
	configResp, err := http.DefaultClient.Do(configReq)
	if err != nil {
		t.Fatalf("GET /v1/auth/config returned error: %v", err)
	}
	defer configResp.Body.Close()
	var config authRuntimeConfigResponse
	if err := json.NewDecoder(configResp.Body).Decode(&config); err != nil {
		t.Fatalf("Decode(auth config) returned error: %v", err)
	}
	if config.User != nil && config.User.IsAdmin {
		t.Fatalf("self-hosted auth config = %#v, want non-admin", config.User)
	}

	req, err := http.NewRequest(http.MethodGet, server.URL+"/v1/admin/overview", nil)
	if err != nil {
		t.Fatalf("NewRequest(admin overview) returned error: %v", err)
	}
	req.Header.Set("X-Forwarded-User", "admin-user")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /v1/admin/overview returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /v1/admin/overview status = %d, want %d, body=%s", resp.StatusCode, http.StatusForbidden, body)
	}
}

func TestAdminEndpointsDoNotChangeUserScopedLists(t *testing.T) {
	t.Setenv(ProductModeEnvVar, ProductModeCloud)
	t.Setenv(authAdminSubjectsEnvVar, "admin-user")

	manager, databaseID := newTestManager(t)
	createOwnedTestWorkspace(t, manager, databaseID, "alice-user", "Alice", 1, "alice-repo")
	createOwnedTestWorkspace(t, manager, databaseID, "bob-user", "Bob", 2, "bob-repo")

	auth := newTrustedHeaderTestAuth(t)
	server := httptest.NewServer(NewHandlerWithOptions(manager, HandlerOptions{
		AllowOrigin: "*",
		Auth:        auth,
	}))
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/v1/workspaces", nil)
	if err != nil {
		t.Fatalf("NewRequest(workspaces) returned error: %v", err)
	}
	req.Header.Set("X-Forwarded-User", "alice-user")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /v1/workspaces returned error: %v", err)
	}
	defer resp.Body.Close()
	var workspaces workspaceListResponse
	if err := json.NewDecoder(resp.Body).Decode(&workspaces); err != nil {
		t.Fatalf("Decode(workspaces) returned error: %v", err)
	}
	if !workspaceListContains(workspaces.Items, "alice-repo") {
		t.Fatalf("alice scoped workspaces = %#v, want alice-repo", workspaces.Items)
	}
	if workspaceListContains(workspaces.Items, "bob-repo") {
		t.Fatalf("alice scoped workspaces = %#v, must not include bob-repo", workspaces.Items)
	}
}

func newTrustedHeaderTestAuth(t *testing.T) *AuthHandler {
	t.Helper()
	auth, err := NewAuthHandler(AuthConfig{
		Mode:              AuthModeTrustedHeader,
		TrustedUserHeader: "X-Forwarded-User",
		TrustedNameHeader: "X-Forwarded-Name",
	})
	if err != nil {
		t.Fatalf("NewAuthHandler() returned error: %v", err)
	}
	return auth
}

func createOwnedTestWorkspace(t *testing.T, manager *DatabaseManager, baseDatabaseID, subject, label string, redisDB int, workspaceName string) {
	t.Helper()

	manager.mu.Lock()
	baseProfile := manager.profiles[baseDatabaseID]
	manager.mu.Unlock()

	ctx := context.WithValue(context.Background(), authIdentityContextKey, AuthIdentity{
		Subject:  subject,
		Name:     label,
		Provider: string(AuthModeTrustedHeader),
	})
	database, err := manager.UpsertDatabase(ctx, "", upsertDatabaseRequest{
		Name:      label + " DB",
		RedisAddr: baseProfile.RedisAddr,
		RedisDB:   redisDB,
	})
	if err != nil {
		t.Fatalf("UpsertDatabase(%s) returned error: %v", subject, err)
	}
	if _, err := manager.CreateWorkspace(ctx, database.ID, createWorkspaceRequest{
		Name:   workspaceName,
		Source: sourceRef{Kind: SourceBlank},
	}); err != nil {
		t.Fatalf("CreateWorkspace(%s) returned error: %v", workspaceName, err)
	}
}

func adminUsersContain(items []adminUserRecord, subject string) bool {
	for _, item := range items {
		if item.Subject == subject {
			return true
		}
	}
	return false
}

func workspaceListContains(items []workspaceSummary, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}
