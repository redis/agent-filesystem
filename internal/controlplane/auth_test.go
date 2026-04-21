package controlplane

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLoadAuthConfigFromEnvDefaultsToNone(t *testing.T) {
	t.Setenv(authModeEnvVar, "")

	cfg, err := LoadAuthConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadAuthConfigFromEnv() returned error: %v", err)
	}
	if cfg.Mode != AuthModeNone {
		t.Fatalf("Mode = %q, want %q", cfg.Mode, AuthModeNone)
	}
}

func TestLoadAuthConfigFromEnvClerk(t *testing.T) {
	t.Setenv(authModeEnvVar, string(AuthModeClerk))
	t.Setenv(authClerkSecretKeyEnvVar, "sk_test_123")
	t.Setenv(authClerkPublishableKeyEnvVar, "pk_test_123")

	cfg, err := LoadAuthConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadAuthConfigFromEnv() returned error: %v", err)
	}
	if cfg.Mode != AuthModeClerk {
		t.Fatalf("Mode = %q, want %q", cfg.Mode, AuthModeClerk)
	}
	if cfg.ClerkSecretKey != "sk_test_123" {
		t.Fatalf("ClerkSecretKey = %q, want sk_test_123", cfg.ClerkSecretKey)
	}
	if cfg.ClerkPublishableKey != "pk_test_123" {
		t.Fatalf("ClerkPublishableKey = %q, want pk_test_123", cfg.ClerkPublishableKey)
	}
}

func TestTrustedHeaderAuthProtectsAdminRoutes(t *testing.T) {
	manager, _ := newTestManager(t)
	auth, err := NewAuthHandler(AuthConfig{
		Mode:              AuthModeTrustedHeader,
		TrustedUserHeader: "X-Forwarded-User",
	})
	if err != nil {
		t.Fatalf("NewAuthHandler() returned error: %v", err)
	}

	server := httptest.NewServer(NewHandlerWithOptions(manager, HandlerOptions{
		AllowOrigin: "*",
		Auth:        auth,
	}))
	defer server.Close()

	resp, err := http.Get(server.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /healthz status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	resp, err = http.Get(server.URL + "/v1/workspaces")
	if err != nil {
		t.Fatalf("GET /v1/workspaces returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("GET /v1/workspaces status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	req, err := http.NewRequest(http.MethodGet, server.URL+"/v1/workspaces", nil)
	if err != nil {
		t.Fatalf("NewRequest() returned error: %v", err)
	}
	req.Header.Set("X-Forwarded-User", "rowan@example.com")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("authorized GET /v1/workspaces returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("authorized GET /v1/workspaces status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestCLIExchangeTokenAuthenticatesProtectedRoutes(t *testing.T) {
	manager, _ := newTestManager(t)
	auth, err := NewAuthHandler(AuthConfig{
		Mode:              AuthModeTrustedHeader,
		TrustedUserHeader: "X-Forwarded-User",
		TrustedNameHeader: "X-Forwarded-Name",
	})
	if err != nil {
		t.Fatalf("NewAuthHandler() returned error: %v", err)
	}

	server := httptest.NewServer(NewHandlerWithOptions(manager, HandlerOptions{
		AllowOrigin: "*",
		Auth:        auth,
	}))
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/workspaces/repo/onboarding-token", nil)
	if err != nil {
		t.Fatalf("NewRequest(onboarding token) returned error: %v", err)
	}
	req.Header.Set("X-Forwarded-User", "rowan@example.com")
	req.Header.Set("X-Forwarded-Name", "Rowan")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST onboarding token returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST onboarding token status = %d, want %d, body=%s", resp.StatusCode, http.StatusCreated, body)
	}

	var token onboardingTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		t.Fatalf("Decode(onboarding token) returned error: %v", err)
	}
	if strings.TrimSpace(token.Token) == "" {
		t.Fatal("expected onboarding token to be populated")
	}

	resp, err = http.Post(server.URL+"/v1/auth/exchange", "application/json", strings.NewReader(fmtJSON(t, onboardingExchangeRequest{
		Token: token.Token,
	})))
	if err != nil {
		t.Fatalf("POST auth exchange returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST auth exchange status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, body)
	}

	var exchange onboardingExchangeResponse
	if err := json.NewDecoder(resp.Body).Decode(&exchange); err != nil {
		t.Fatalf("Decode(onboarding exchange) returned error: %v", err)
	}
	if strings.TrimSpace(exchange.AccessToken) == "" {
		t.Fatal("expected exchange access token to be populated")
	}

	req, err = http.NewRequest(http.MethodGet, server.URL+"/v1/workspaces", nil)
	if err != nil {
		t.Fatalf("NewRequest(workspaces) returned error: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+exchange.AccessToken)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("authorized GET /v1/workspaces returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("authorized GET /v1/workspaces status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, body)
	}
}

func TestAuthConfigEndpointIsPublic(t *testing.T) {
	manager, _ := newTestManager(t)
	auth, err := NewAuthHandler(AuthConfig{
		Mode:              AuthModeTrustedHeader,
		TrustedUserHeader: "X-Forwarded-User",
	})
	if err != nil {
		t.Fatalf("NewAuthHandler() returned error: %v", err)
	}

	server := httptest.NewServer(NewHandlerWithOptions(manager, HandlerOptions{
		AllowOrigin: "*",
		Auth:        auth,
	}))
	defer server.Close()

	resp, err := http.Get(server.URL + "/v1/auth/config")
	if err != nil {
		t.Fatalf("GET /v1/auth/config returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/auth/config status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestAuthConfigReportsProductMode(t *testing.T) {
	manager, _ := newTestManager(t)
	auth, err := NewAuthHandler(AuthConfig{Mode: AuthModeNone})
	if err != nil {
		t.Fatalf("NewAuthHandler() returned error: %v", err)
	}

	server := httptest.NewServer(NewHandlerWithOptions(manager, HandlerOptions{
		AllowOrigin: "*",
		Auth:        auth,
	}))
	defer server.Close()

	// Default (unset env var) is self-hosted.
	t.Setenv(ProductModeEnvVar, "")
	body := fetchAuthConfig(t, server.URL)
	if body.ProductMode != ProductModeSelfHosted {
		t.Errorf("default product_mode = %q, want %q", body.ProductMode, ProductModeSelfHosted)
	}

	// Explicit cloud opt-in.
	t.Setenv(ProductModeEnvVar, ProductModeCloud)
	body = fetchAuthConfig(t, server.URL)
	if body.ProductMode != ProductModeCloud {
		t.Errorf("cloud product_mode = %q, want %q", body.ProductMode, ProductModeCloud)
	}
}

func fetchAuthConfig(t *testing.T, baseURL string) authRuntimeConfigResponse {
	t.Helper()
	resp, err := http.Get(baseURL + "/v1/auth/config")
	if err != nil {
		t.Fatalf("GET /v1/auth/config returned error: %v", err)
	}
	defer resp.Body.Close()
	var decoded authRuntimeConfigResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode /v1/auth/config: %v", err)
	}
	return decoded
}

func TestClerkAuthProtectsAdminRoutes(t *testing.T) {
	manager, _ := newTestManager(t)
	auth, err := NewAuthHandler(AuthConfig{
		Mode:                AuthModeClerk,
		ClerkJWTKey:         "-----BEGIN PUBLIC KEY-----\nMFwwDQYJKoZIhvcNAQEBBQADSwAwSAJBAKq8QyFfJOLLmObAun1vDLteA94ppIqh\napMI2vlA38nSxrdbidKdvUSsfx8bVsgcuyo6edSxnl2xe50Tzw9uQWkCAwEAAQ==\n-----END PUBLIC KEY-----",
		ClerkPublishableKey: "pk_test_123",
	})
	if err != nil {
		t.Fatalf("NewAuthHandler() returned error: %v", err)
	}
	auth.clerkAuthenticate = func(r *http.Request) (*AuthIdentity, error) {
		token := clerkSessionTokenFromRequest(r)
		if strings.TrimSpace(token) == "" {
			return nil, ErrUnauthorized
		}
		return &AuthIdentity{
			Subject:  "user_123",
			Provider: string(AuthModeClerk),
		}, nil
	}

	server := httptest.NewServer(NewHandlerWithOptions(manager, HandlerOptions{
		AllowOrigin: "*",
		Auth:        auth,
	}))
	defer server.Close()

	resp, err := http.Get(server.URL + "/v1/workspaces")
	if err != nil {
		t.Fatalf("GET /v1/workspaces returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("GET /v1/workspaces status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	req, err := http.NewRequest(http.MethodGet, server.URL+"/v1/workspaces", nil)
	if err != nil {
		t.Fatalf("NewRequest() returned error: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: clerkSessionCookieName, Value: "session_token"})
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("authorized GET /v1/workspaces returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("authorized GET /v1/workspaces status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	req, err = http.NewRequest(http.MethodGet, server.URL+"/v1/auth/config", nil)
	if err != nil {
		t.Fatalf("NewRequest(auth config) returned error: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: clerkSessionCookieName, Value: "session_token"})
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /v1/auth/config returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/auth/config status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var payload authRuntimeConfigResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("Decode(auth config) returned error: %v", err)
	}
	if payload.ClerkPublishableKey != "pk_test_123" {
		t.Fatalf("ClerkPublishableKey = %q, want pk_test_123", payload.ClerkPublishableKey)
	}
	if !payload.Authenticated {
		t.Fatal("Authenticated = false, want true")
	}
	if payload.User == nil || payload.User.Subject != "user_123" {
		t.Fatalf("User = %#v, want subject user_123", payload.User)
	}
}

func TestTrustedHeaderScopesOwnedDatabasesAndWorkspaces(t *testing.T) {
	manager, databaseID := newTestManager(t)
	auth, err := NewAuthHandler(AuthConfig{
		Mode:              AuthModeTrustedHeader,
		TrustedUserHeader: "X-Forwarded-User",
	})
	if err != nil {
		t.Fatalf("NewAuthHandler() returned error: %v", err)
	}

	baseProfile := manager.profiles[databaseID]
	aliceCtx := context.WithValue(context.Background(), authIdentityContextKey, AuthIdentity{
		Subject: "alice@example.com",
		Name:    "Alice",
		Email:   "alice@example.com",
	})
	bobCtx := context.WithValue(context.Background(), authIdentityContextKey, AuthIdentity{
		Subject: "bob@example.com",
		Name:    "Bob",
		Email:   "bob@example.com",
	})

	aliceDatabase, err := manager.UpsertDatabase(aliceCtx, "", upsertDatabaseRequest{
		Name:      "Alice DB",
		RedisAddr: baseProfile.RedisAddr,
		RedisDB:   1,
	})
	if err != nil {
		t.Fatalf("UpsertDatabase(alice) returned error: %v", err)
	}
	if aliceDatabase.OwnerSubject != "alice@example.com" {
		t.Fatalf("aliceDatabase.OwnerSubject = %q, want alice@example.com", aliceDatabase.OwnerSubject)
	}
	if aliceDatabase.OwnerLabel != "Alice" {
		t.Fatalf("aliceDatabase.OwnerLabel = %q, want Alice", aliceDatabase.OwnerLabel)
	}

	bobDatabase, err := manager.UpsertDatabase(bobCtx, "", upsertDatabaseRequest{
		Name:      "Bob DB",
		RedisAddr: baseProfile.RedisAddr,
		RedisDB:   2,
	})
	if err != nil {
		t.Fatalf("UpsertDatabase(bob) returned error: %v", err)
	}

	if _, err := manager.CreateWorkspace(aliceCtx, aliceDatabase.ID, createWorkspaceRequest{
		Name:   "alice-repo",
		Source: sourceRef{Kind: SourceBlank},
	}); err != nil {
		t.Fatalf("CreateWorkspace(alice) returned error: %v", err)
	}
	if _, err := manager.CreateWorkspace(bobCtx, bobDatabase.ID, createWorkspaceRequest{
		Name:   "bob-repo",
		Source: sourceRef{Kind: SourceBlank},
	}); err != nil {
		t.Fatalf("CreateWorkspace(bob) returned error: %v", err)
	}

	server := httptest.NewServer(NewHandlerWithOptions(manager, HandlerOptions{
		AllowOrigin: "*",
		Auth:        auth,
	}))
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/v1/databases", nil)
	if err != nil {
		t.Fatalf("NewRequest(databases) returned error: %v", err)
	}
	req.Header.Set("X-Forwarded-User", "alice@example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /v1/databases returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /v1/databases status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, body)
	}

	var databases databaseListResponse
	if err := json.NewDecoder(resp.Body).Decode(&databases); err != nil {
		t.Fatalf("Decode(databases) returned error: %v", err)
	}
	if len(databases.Items) != 2 {
		t.Fatalf("len(databases.items) = %d, want 2 (shared default + alice-owned)", len(databases.Items))
	}
	for _, item := range databases.Items {
		if item.ID == bobDatabase.ID {
			t.Fatalf("alice should not see bob-owned database: %#v", databases.Items)
		}
	}

	req, err = http.NewRequest(http.MethodGet, server.URL+"/v1/workspaces", nil)
	if err != nil {
		t.Fatalf("NewRequest(workspaces) returned error: %v", err)
	}
	req.Header.Set("X-Forwarded-User", "alice@example.com")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /v1/workspaces returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /v1/workspaces status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, body)
	}

	var workspaces workspaceListResponse
	if err := json.NewDecoder(resp.Body).Decode(&workspaces); err != nil {
		t.Fatalf("Decode(workspaces) returned error: %v", err)
	}
	seen := map[string]bool{}
	for _, item := range workspaces.Items {
		seen[item.Name] = true
	}
	if !seen["repo"] {
		t.Fatalf("expected alice to keep seeing shared repo, got %#v", workspaces.Items)
	}
	if !seen["alice-repo"] {
		t.Fatalf("expected alice to see alice-repo, got %#v", workspaces.Items)
	}
	if seen["bob-repo"] {
		t.Fatalf("alice should not see bob-repo, got %#v", workspaces.Items)
	}

	req, err = http.NewRequest(http.MethodGet, server.URL+"/v1/databases/"+bobDatabase.ID+"/workspaces", nil)
	if err != nil {
		t.Fatalf("NewRequest(bob workspaces) returned error: %v", err)
	}
	req.Header.Set("X-Forwarded-User", "alice@example.com")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET bob workspaces returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET bob workspaces status = %d, want %d, body=%s", resp.StatusCode, http.StatusNotFound, body)
	}
}

func TestTrustedHeaderDeveloperResetDeletesOwnedDatabases(t *testing.T) {
	manager, databaseID := newTestManager(t)
	auth, err := NewAuthHandler(AuthConfig{
		Mode:              AuthModeTrustedHeader,
		TrustedUserHeader: "X-Forwarded-User",
	})
	if err != nil {
		t.Fatalf("NewAuthHandler() returned error: %v", err)
	}

	baseProfile := manager.profiles[databaseID]
	aliceCtx := context.WithValue(context.Background(), authIdentityContextKey, AuthIdentity{
		Subject:  "alice@example.com",
		Name:     "Alice",
		Provider: string(AuthModeTrustedHeader),
	})
	aliceDatabase, err := manager.UpsertDatabase(aliceCtx, "", upsertDatabaseRequest{
		Name:      "Alice DB",
		RedisAddr: baseProfile.RedisAddr,
		RedisDB:   1,
	})
	if err != nil {
		t.Fatalf("UpsertDatabase(alice) returned error: %v", err)
	}
	if _, err := manager.CreateWorkspace(aliceCtx, aliceDatabase.ID, createWorkspaceRequest{
		Name:   "alice-repo",
		Source: sourceRef{Kind: SourceBlank},
	}); err != nil {
		t.Fatalf("CreateWorkspace(alice) returned error: %v", err)
	}

	server := httptest.NewServer(NewHandlerWithOptions(manager, HandlerOptions{
		AllowOrigin: "*",
		Auth:        auth,
	}))
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/account/developer/reset", nil)
	if err != nil {
		t.Fatalf("NewRequest(reset) returned error: %v", err)
	}
	req.Header.Set("X-Forwarded-User", "alice@example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/account/developer/reset returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /v1/account/developer/reset status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, body)
	}

	var payload accountResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("Decode(reset payload) returned error: %v", err)
	}
	if payload.DeletedDatabaseCount != 1 {
		t.Fatalf("DeletedDatabaseCount = %d, want 1", payload.DeletedDatabaseCount)
	}
	if payload.DeletedWorkspaceCount != 1 {
		t.Fatalf("DeletedWorkspaceCount = %d, want 1", payload.DeletedWorkspaceCount)
	}

	req, err = http.NewRequest(http.MethodGet, server.URL+"/v1/databases", nil)
	if err != nil {
		t.Fatalf("NewRequest(databases after reset) returned error: %v", err)
	}
	req.Header.Set("X-Forwarded-User", "alice@example.com")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /v1/databases after reset returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /v1/databases after reset status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, body)
	}

	var databases databaseListResponse
	if err := json.NewDecoder(resp.Body).Decode(&databases); err != nil {
		t.Fatalf("Decode(databases after reset) returned error: %v", err)
	}
	for _, item := range databases.Items {
		if item.ID == aliceDatabase.ID {
			t.Fatalf("alice-owned database still visible after reset: %#v", databases.Items)
		}
	}
}

func TestTrustedHeaderDeveloperResetClearsStarterWorkspaceWithoutDeletingOnboardingDatabase(t *testing.T) {
	manager, databaseID := newTestManager(t)
	auth, err := NewAuthHandler(AuthConfig{
		Mode:              AuthModeTrustedHeader,
		TrustedUserHeader: "X-Forwarded-User",
	})
	if err != nil {
		t.Fatalf("NewAuthHandler() returned error: %v", err)
	}

	manager.mu.Lock()
	baseProfile := manager.profiles[databaseID]
	manager.profiles[quickstartCloudDBID] = databaseProfile{
		ID:             quickstartCloudDBID,
		Name:           quickstartCloudDBName,
		Description:    "Shared AFS Cloud onboarding database.",
		ManagementType: databaseManagementSystemManaged,
		Purpose:        databasePurposeOnboarding,
		RedisAddr:      baseProfile.RedisAddr,
		RedisUsername:  baseProfile.RedisUsername,
		RedisPassword:  baseProfile.RedisPassword,
		RedisDB:        baseProfile.RedisDB,
		RedisTLS:       baseProfile.RedisTLS,
		IsDefault:      true,
	}
	manager.order = append(manager.order, quickstartCloudDBID)
	manager.mu.Unlock()

	aliceCtx := context.WithValue(context.Background(), authIdentityContextKey, AuthIdentity{
		Subject: "alice@example.com",
		Name:    "Alice",
		Email:   "alice@example.com",
	})
	service, profile, err := manager.serviceFor(aliceCtx, quickstartCloudDBID)
	if err != nil {
		t.Fatalf("serviceFor(afs-cloud) returned error: %v", err)
	}
	starterName := quickstartWorkspaceNameFor(profile, "alice@example.com")
	starter, err := createQuickstartWorkspace(aliceCtx, service, profile, starterName)
	if err != nil {
		t.Fatalf("createQuickstartWorkspace() returned error: %v", err)
	}

	now := time.Now().UTC()
	if err := manager.catalog.UpsertSession(context.Background(), sessionCatalogRecord{
		SessionID:       "starter-agent",
		WorkspaceID:     starter.ID,
		DatabaseID:      quickstartCloudDBID,
		WorkspaceName:   starter.Name,
		ClientKind:      "sync",
		AFSVersion:      "test",
		Hostname:        "mac-mini",
		OperatingSystem: "darwin",
		LocalPath:       "/tmp/getting-started",
		State:           workspaceSessionStateActive,
		StartedAt:       now.Format(timeRFC3339),
		LastSeenAt:      now.Format(timeRFC3339),
		LeaseExpiresAt:  now.Add(time.Hour).Format(timeRFC3339),
		UpdatedAt:       now.Format(timeRFC3339),
	}); err != nil {
		t.Fatalf("UpsertSession(starter) returned error: %v", err)
	}

	manager.mu.Lock()
	profile = manager.profiles[quickstartCloudDBID]
	profile.OwnerSubject = "alice@example.com"
	manager.profiles[quickstartCloudDBID] = profile
	manager.mu.Unlock()

	server := httptest.NewServer(NewHandlerWithOptions(manager, HandlerOptions{
		AllowOrigin: "*",
		Auth:        auth,
	}))
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/account/developer/reset", nil)
	if err != nil {
		t.Fatalf("NewRequest(reset) returned error: %v", err)
	}
	req.Header.Set("X-Forwarded-User", "alice@example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/account/developer/reset returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /v1/account/developer/reset status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, body)
	}

	var payload accountResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("Decode(reset payload) returned error: %v", err)
	}
	if payload.DeletedDatabaseCount != 0 {
		t.Fatalf("DeletedDatabaseCount = %d, want 0 for system-managed onboarding db", payload.DeletedDatabaseCount)
	}
	if payload.DeletedWorkspaceCount != 1 {
		t.Fatalf("DeletedWorkspaceCount = %d, want 1 starter workspace", payload.DeletedWorkspaceCount)
	}

	req, err = http.NewRequest(http.MethodGet, server.URL+"/v1/workspaces", nil)
	if err != nil {
		t.Fatalf("NewRequest(workspaces after reset) returned error: %v", err)
	}
	req.Header.Set("X-Forwarded-User", "alice@example.com")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /v1/workspaces after reset returned error: %v", err)
	}
	defer resp.Body.Close()
	var workspaces workspaceListResponse
	if err := json.NewDecoder(resp.Body).Decode(&workspaces); err != nil {
		t.Fatalf("Decode(workspaces after reset) returned error: %v", err)
	}
	for _, item := range workspaces.Items {
		if item.Name == starter.Name {
			t.Fatalf("starter workspace still visible after reset: %#v", workspaces.Items)
		}
	}

	req, err = http.NewRequest(http.MethodGet, server.URL+"/v1/agents", nil)
	if err != nil {
		t.Fatalf("NewRequest(agents after reset) returned error: %v", err)
	}
	req.Header.Set("X-Forwarded-User", "alice@example.com")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /v1/agents after reset returned error: %v", err)
	}
	defer resp.Body.Close()
	var agents workspaceSessionListResponse
	if err := json.NewDecoder(resp.Body).Decode(&agents); err != nil {
		t.Fatalf("Decode(agents after reset) returned error: %v", err)
	}
	for _, item := range agents.Items {
		if item.WorkspaceName == starter.Name {
			t.Fatalf("starter agent still visible after reset: %#v", agents.Items)
		}
	}
}

func TestDeleteCurrentIdentityUsesClerkDeleter(t *testing.T) {
	auth, err := NewAuthHandler(AuthConfig{
		Mode:                AuthModeClerk,
		ClerkJWTKey:         "-----BEGIN PUBLIC KEY-----\nMFwwDQYJKoZIhvcNAQEBBQADSwAwSAJBAKq8QyFfJOLLmObAun1vDLteA94ppIqh\napMI2vlA38nSxrdbidKdvUSsfx8bVsgcuyo6edSxnl2xe50Tzw9uQWkCAwEAAQ==\n-----END PUBLIC KEY-----",
		ClerkPublishableKey: "pk_test_123",
	})
	if err != nil {
		t.Fatalf("NewAuthHandler() returned error: %v", err)
	}

	deletedSubject := ""
	auth.clerkDeleteUser = func(_ context.Context, subject string) error {
		deletedSubject = subject
		return nil
	}

	ctx := context.WithValue(context.Background(), authIdentityContextKey, AuthIdentity{
		Subject:  "user_123",
		Provider: string(AuthModeClerk),
	})
	if err := auth.DeleteCurrentIdentity(ctx); err != nil {
		t.Fatalf("DeleteCurrentIdentity() returned error: %v", err)
	}
	if deletedSubject != "user_123" {
		t.Fatalf("deletedSubject = %q, want user_123", deletedSubject)
	}
}
