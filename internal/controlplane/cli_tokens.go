package controlplane

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	cliAccessTokenPrefix = "afs_cli"

	cliScopeAccount         = "account"
	cliScopeWorkspacePrefix = "workspace:"

	cliCapabilityAccount = "account"
	cliCapabilityMountRO = "mount-ro"
	cliCapabilityMountRW = "mount-rw"
)

var ErrCLIAccessTokenInvalid = errors.New("cli access token is invalid or expired")

type cliAccessTokenRecord struct {
	ID            string `json:"id"`
	Name          string `json:"name,omitempty"`
	OwnerSubject  string `json:"owner_subject,omitempty"`
	OwnerLabel    string `json:"owner_label,omitempty"`
	DatabaseID    string `json:"database_id"`
	WorkspaceID   string `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name,omitempty"`
	Scope         string `json:"scope"`
	Capability    string `json:"capability,omitempty"`
	SecretHash    string `json:"-"`
	CreatedAt     string `json:"created_at"`
	LastUsedAt    string `json:"last_used_at,omitempty"`
	ExpiresAt     string `json:"expires_at,omitempty"`
	RevokedAt     string `json:"revoked_at,omitempty"`
}

type cliAccessTokenResponse struct {
	ID            string `json:"id"`
	Name          string `json:"name,omitempty"`
	OwnerSubject  string `json:"owner_subject,omitempty"`
	OwnerLabel    string `json:"owner_label,omitempty"`
	DatabaseID    string `json:"database_id,omitempty"`
	WorkspaceID   string `json:"workspace_id,omitempty"`
	WorkspaceName string `json:"workspace_name,omitempty"`
	Scope         string `json:"scope"`
	Capability    string `json:"capability,omitempty"`
	Token         string `json:"token,omitempty"`
	CreatedAt     string `json:"created_at"`
	LastUsedAt    string `json:"last_used_at,omitempty"`
	ExpiresAt     string `json:"expires_at,omitempty"`
	RevokedAt     string `json:"revoked_at,omitempty"`
}

type createCLIAccessTokenRequest struct {
	Name       string `json:"name"`
	Capability string `json:"capability,omitempty"`
	Readonly   bool   `json:"readonly,omitempty"`
	ExpiresAt  string `json:"expires_at,omitempty"`
}

type createCLIAccessTokenOptions struct {
	Name          string
	Subject       string
	Label         string
	DatabaseID    string
	WorkspaceID   string
	WorkspaceName string
	Scope         string
	Capability    string
	ExpiresAt     string
}

func (m *DatabaseManager) createCLIAccessTokenRecord(ctx context.Context, subject, label, databaseID, workspaceID, workspaceName string) (string, error) {
	response, err := m.createCLIAccessTokenRecordWithOptions(ctx, createCLIAccessTokenOptions{
		Subject:       subject,
		Label:         label,
		DatabaseID:    databaseID,
		WorkspaceID:   workspaceID,
		WorkspaceName: workspaceName,
		Scope:         cliScopeAccount,
		Capability:    cliCapabilityAccount,
	})
	if err != nil {
		return "", err
	}
	return response.Token, nil
}

func (m *DatabaseManager) CreateResolvedWorkspaceCLIAccessToken(ctx context.Context, workspace string, input createCLIAccessTokenRequest) (cliAccessTokenResponse, error) {
	if m == nil || m.catalog == nil {
		return cliAccessTokenResponse{}, fmt.Errorf("cli token storage is unavailable")
	}
	subject, label, err := m.requireOwnedSubject(ctx)
	if err != nil {
		return cliAccessTokenResponse{}, err
	}
	_, profile, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return cliAccessTokenResponse{}, err
	}
	if !databaseProfileVisibleToSubject(profile, subject) {
		return cliAccessTokenResponse{}, os.ErrNotExist
	}
	route, err = m.claimSharedWorkspaceForSubject(ctx, profile, route)
	if err != nil {
		return cliAccessTokenResponse{}, err
	}
	return m.createWorkspaceCLIAccessTokenRecord(ctx, subject, label, profile.ID, route.WorkspaceID, route.Name, input)
}

func (m *DatabaseManager) CreateWorkspaceCLIAccessToken(ctx context.Context, databaseID, workspace string, input createCLIAccessTokenRequest) (cliAccessTokenResponse, error) {
	if m == nil || m.catalog == nil {
		return cliAccessTokenResponse{}, fmt.Errorf("cli token storage is unavailable")
	}
	subject, label, err := m.requireOwnedSubject(ctx)
	if err != nil {
		return cliAccessTokenResponse{}, err
	}
	_, profile, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return cliAccessTokenResponse{}, err
	}
	if !databaseProfileVisibleToSubject(profile, subject) {
		return cliAccessTokenResponse{}, os.ErrNotExist
	}
	route, err = m.claimSharedWorkspaceForSubject(ctx, profile, route)
	if err != nil {
		return cliAccessTokenResponse{}, err
	}
	return m.createWorkspaceCLIAccessTokenRecord(ctx, subject, label, profile.ID, route.WorkspaceID, route.Name, input)
}

func (m *DatabaseManager) CreateResolvedWorkspaceCompositionCLIAccessToken(ctx context.Context, workspace string, input createCLIAccessTokenRequest) (cliAccessTokenResponse, error) {
	if m == nil || m.catalog == nil {
		return cliAccessTokenResponse{}, fmt.Errorf("cli token storage is unavailable")
	}
	subject, label, err := m.requireOwnedSubject(ctx)
	if err != nil {
		return cliAccessTokenResponse{}, err
	}
	_, profile, route, err := m.resolveWorkspaceComposition(ctx, workspace)
	if err != nil {
		return cliAccessTokenResponse{}, err
	}
	if !databaseProfileVisibleToSubject(profile, subject) {
		return cliAccessTokenResponse{}, os.ErrNotExist
	}
	return m.createWorkspaceCLIAccessTokenRecord(ctx, subject, label, profile.ID, route.ID, route.Name, input)
}

func (m *DatabaseManager) createWorkspaceCLIAccessTokenRecord(ctx context.Context, subject, label, databaseID, workspaceID, workspaceName string, input createCLIAccessTokenRequest) (cliAccessTokenResponse, error) {
	capabilityInput := strings.TrimSpace(input.Capability)
	if capabilityInput == "" && input.Readonly {
		capabilityInput = cliCapabilityMountRO
	}
	capability, err := normalizeCLIMountCapability(capabilityInput)
	if err != nil {
		return cliAccessTokenResponse{}, err
	}
	return m.createCLIAccessTokenRecordWithOptions(ctx, createCLIAccessTokenOptions{
		Name:          input.Name,
		Subject:       subject,
		Label:         label,
		DatabaseID:    databaseID,
		WorkspaceID:   workspaceID,
		WorkspaceName: workspaceName,
		Scope:         cliWorkspaceScope(workspaceID),
		Capability:    capability,
		ExpiresAt:     input.ExpiresAt,
	})
}

func (m *DatabaseManager) createCLIAccessTokenRecordWithOptions(ctx context.Context, options createCLIAccessTokenOptions) (cliAccessTokenResponse, error) {
	if m == nil || m.catalog == nil {
		return cliAccessTokenResponse{}, fmt.Errorf("cli token storage is unavailable")
	}
	tokenID, secret, err := newCLIAccessTokenParts()
	if err != nil {
		return cliAccessTokenResponse{}, err
	}
	now := time.Now().UTC()
	scope := normalizeCLITokenScope(options.Scope)
	capability := normalizeCLITokenCapability(scope, options.Capability)
	record := cliAccessTokenRecord{
		ID:            tokenID,
		Name:          strings.TrimSpace(options.Name),
		OwnerSubject:  strings.TrimSpace(options.Subject),
		OwnerLabel:    strings.TrimSpace(options.Label),
		DatabaseID:    strings.TrimSpace(options.DatabaseID),
		WorkspaceID:   strings.TrimSpace(options.WorkspaceID),
		WorkspaceName: strings.TrimSpace(options.WorkspaceName),
		Scope:         scope,
		Capability:    capability,
		SecretHash:    hashCLIAccessTokenSecret(secret),
		CreatedAt:     now.Format(timeRFC3339),
	}
	if expiresAt := strings.TrimSpace(options.ExpiresAt); expiresAt != "" {
		if _, err := time.Parse(timeRFC3339, expiresAt); err != nil {
			return cliAccessTokenResponse{}, fmt.Errorf("expires_at must be RFC3339: %w", err)
		}
		record.ExpiresAt = expiresAt
	}
	if err := m.catalog.CreateCLIAccessToken(ctx, record); err != nil {
		return cliAccessTokenResponse{}, err
	}
	response := cliAccessTokenResponseFromRecord(record)
	response.Token = formatCLIAccessToken(tokenID, secret)
	return response, nil
}

// ListAllCLIAccessTokens returns every active CLI access token owned by the
// caller's subject. Mirrors the MCP token list shape so the dashboard can
// merge both into one view.
func (m *DatabaseManager) ListAllCLIAccessTokens(ctx context.Context) ([]cliAccessTokenResponse, error) {
	if m == nil || m.catalog == nil {
		return nil, fmt.Errorf("cli token storage is unavailable")
	}
	subject := authSubjectFromContext(ctx)
	items, err := m.catalog.ListAllCLIAccessTokens(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]cliAccessTokenResponse, 0, len(items))
	for _, item := range items {
		ownerSubject := strings.TrimSpace(item.OwnerSubject)
		if subject != "" && ownerSubject != "" && ownerSubject != subject {
			continue
		}
		// Bootstrap onboarding tokens (account scope, no workspace) are
		// internal credentials; hide them from the dashboard list.
		if isAccountCLIScope(item.Scope) {
			continue
		}
		out = append(out, cliAccessTokenResponseFromRecord(item))
	}
	return out, nil
}

// RevokeCLIAccessToken marks a CLI token revoked. Scoped to the caller's
// subject so users cannot revoke tokens belonging to others.
func (m *DatabaseManager) RevokeCLIAccessToken(ctx context.Context, tokenID string) error {
	if m == nil || m.catalog == nil {
		return fmt.Errorf("cli token storage is unavailable")
	}
	tokenID = strings.TrimSpace(tokenID)
	if tokenID == "" {
		return os.ErrNotExist
	}
	record, err := m.catalog.GetCLIAccessToken(ctx, tokenID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(record.RevokedAt) != "" {
		return os.ErrNotExist
	}
	subject := authSubjectFromContext(ctx)
	if subject != "" && strings.TrimSpace(record.OwnerSubject) != "" && strings.TrimSpace(record.OwnerSubject) != subject {
		return os.ErrNotExist
	}
	return m.catalog.RevokeCLIAccessTokenByID(ctx, tokenID, time.Now().UTC().Format(timeRFC3339))
}

func (m *DatabaseManager) AuthenticateCLIAccessToken(ctx context.Context, rawToken string) (cliAccessTokenRecord, error) {
	if m == nil || m.catalog == nil {
		return cliAccessTokenRecord{}, ErrCLIAccessTokenInvalid
	}
	tokenID, secret, err := parseCLIAccessToken(rawToken)
	if err != nil {
		return cliAccessTokenRecord{}, ErrCLIAccessTokenInvalid
	}
	record, err := m.catalog.GetCLIAccessToken(ctx, tokenID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cliAccessTokenRecord{}, ErrCLIAccessTokenInvalid
		}
		return cliAccessTokenRecord{}, err
	}
	if strings.TrimSpace(record.SecretHash) == "" || record.SecretHash != hashCLIAccessTokenSecret(secret) {
		return cliAccessTokenRecord{}, ErrCLIAccessTokenInvalid
	}
	now := time.Now().UTC()
	if strings.TrimSpace(record.RevokedAt) != "" {
		return cliAccessTokenRecord{}, ErrCLIAccessTokenInvalid
	}
	if expiresAt := strings.TrimSpace(record.ExpiresAt); expiresAt != "" {
		expiry, err := time.Parse(timeRFC3339, expiresAt)
		if err != nil || !now.Before(expiry) {
			return cliAccessTokenRecord{}, ErrCLIAccessTokenInvalid
		}
	}
	if err := m.catalog.TouchCLIAccessToken(ctx, tokenID, now.Format(timeRFC3339)); err != nil {
		return cliAccessTokenRecord{}, err
	}
	record.LastUsedAt = now.Format(timeRFC3339)
	record.Scope = normalizeCLITokenScope(record.Scope)
	record.Capability = normalizeCLITokenCapability(record.Scope, record.Capability)
	return record, nil
}

func newCLIAccessTokenParts() (string, string, error) {
	idRaw := make([]byte, 8)
	if _, err := rand.Read(idRaw); err != nil {
		return "", "", err
	}
	secretRaw := make([]byte, 24)
	if _, err := rand.Read(secretRaw); err != nil {
		return "", "", err
	}
	return hex.EncodeToString(idRaw), hex.EncodeToString(secretRaw), nil
}

func formatCLIAccessToken(id, secret string) string {
	return cliAccessTokenPrefix + "_" + strings.TrimSpace(id) + "_" + strings.TrimSpace(secret)
}

func parseCLIAccessToken(raw string) (string, string, error) {
	trimmed := strings.TrimSpace(raw)
	parts := strings.Split(trimmed, "_")
	if len(parts) != 4 || parts[0] != "afs" || parts[1] != "cli" {
		return "", "", fmt.Errorf("invalid cli token format")
	}
	if strings.TrimSpace(parts[2]) == "" || strings.TrimSpace(parts[3]) == "" {
		return "", "", fmt.Errorf("invalid cli token format")
	}
	return strings.TrimSpace(parts[2]), strings.TrimSpace(parts[3]), nil
}

func hashCLIAccessTokenSecret(secret string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(secret)))
	return hex.EncodeToString(sum[:])
}

func cliWorkspaceScope(workspaceID string) string {
	return cliScopeWorkspacePrefix + strings.TrimSpace(workspaceID)
}

func normalizeCLIMountCapability(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "rw", "write", "read-write", cliCapabilityMountRW:
		return cliCapabilityMountRW, nil
	case "ro", "read", "read-only", "readonly", cliCapabilityMountRO:
		return cliCapabilityMountRO, nil
	default:
		return "", fmt.Errorf("unsupported cli token capability %q (expected mount-ro or mount-rw)", raw)
	}
}

func normalizeCLITokenScope(scope string) string {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return cliScopeAccount
	}
	return scope
}

func normalizeCLITokenCapability(scope, capability string) string {
	capability = strings.ToLower(strings.TrimSpace(capability))
	switch capability {
	case "":
		if isAccountCLIScope(scope) {
			return cliCapabilityAccount
		}
		return cliCapabilityMountRW
	case "rw", "write", "read-write":
		return cliCapabilityMountRW
	case "ro", "read", "read-only", "readonly":
		return cliCapabilityMountRO
	default:
		return capability
	}
}

func isAccountCLIScope(scope string) bool {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "", cliScopeAccount, "org", "organization":
		return true
	default:
		return false
	}
}

func isWorkspaceCLIScope(scope string) bool {
	return strings.HasPrefix(strings.TrimSpace(scope), cliScopeWorkspacePrefix)
}

func isMountOnlyCLICapability(capability string) bool {
	switch normalizeCLITokenCapability(cliWorkspaceScope("workspace"), capability) {
	case cliCapabilityMountRO, cliCapabilityMountRW:
		return true
	default:
		return false
	}
}

func cliCapabilityReadonly(capability string) bool {
	return normalizeCLITokenCapability(cliWorkspaceScope("workspace"), capability) == cliCapabilityMountRO
}

func cliAccessTokenResponseFromRecord(record cliAccessTokenRecord) cliAccessTokenResponse {
	scope := normalizeCLITokenScope(record.Scope)
	return cliAccessTokenResponse{
		ID:            record.ID,
		Name:          record.Name,
		OwnerSubject:  record.OwnerSubject,
		OwnerLabel:    record.OwnerLabel,
		DatabaseID:    record.DatabaseID,
		WorkspaceID:   record.WorkspaceID,
		WorkspaceName: record.WorkspaceName,
		Scope:         scope,
		Capability:    normalizeCLITokenCapability(scope, record.Capability),
		CreatedAt:     record.CreatedAt,
		LastUsedAt:    record.LastUsedAt,
		ExpiresAt:     record.ExpiresAt,
		RevokedAt:     record.RevokedAt,
	}
}

func cliTokenAllowsHTTPPath(identity AuthIdentity, method, path string) bool {
	if strings.TrimSpace(identity.Provider) != "cli-token" {
		return true
	}
	if !isWorkspaceCLIScope(identity.Scope) || !isMountOnlyCLICapability(identity.Capability) {
		return true
	}
	path = strings.TrimSpace(path)
	if method == http.MethodGet && path == "/v1/workspaces" {
		return true
	}
	if method == http.MethodGet && (path == "/v2/workspaces" || strings.HasPrefix(path, "/v2/workspaces/")) {
		return true
	}
	return strings.HasPrefix(path, "/v1/client/") ||
		strings.HasPrefix(path, "/workspaces/") ||
		strings.HasPrefix(path, "/databases/")
}

func cliTokenAllowsWorkspace(identity AuthIdentity, databaseID, workspaceID, workspaceName string) bool {
	if strings.TrimSpace(identity.Provider) != "cli-token" {
		return true
	}
	scope := normalizeCLITokenScope(identity.Scope)
	if isAccountCLIScope(scope) {
		return true
	}
	if !isWorkspaceCLIScope(scope) {
		return false
	}
	if scopedDatabase := strings.TrimSpace(identity.ScopedDatabaseID); scopedDatabase != "" && scopedDatabase != strings.TrimSpace(databaseID) {
		return false
	}
	target := strings.TrimSpace(strings.TrimPrefix(scope, cliScopeWorkspacePrefix))
	if target == "" {
		return false
	}
	if target == strings.TrimSpace(workspaceID) || target == strings.TrimSpace(workspaceName) {
		return true
	}
	if _, ok := identity.WorkspaceMountCapabilities[strings.TrimSpace(workspaceID)]; ok {
		return true
	}
	return false
}

func requireCLITokenWorkspaceAccess(ctx context.Context, databaseID, workspaceID, workspaceName string) error {
	identity, ok := AuthIdentityFromContext(ctx)
	if !ok || cliTokenAllowsWorkspace(identity, databaseID, workspaceID, workspaceName) {
		return nil
	}
	return os.ErrNotExist
}

func filterWorkspaceSummariesForCLIToken(ctx context.Context, items []workspaceSummary) []workspaceSummary {
	identity, ok := AuthIdentityFromContext(ctx)
	if !ok || strings.TrimSpace(identity.Provider) != "cli-token" || !isWorkspaceCLIScope(identity.Scope) {
		return items
	}
	filtered := make([]workspaceSummary, 0, len(items))
	for _, item := range items {
		if cliTokenAllowsWorkspace(identity, item.DatabaseID, item.ID, item.Name) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func cliTokenAllowsWorkspaceComposition(identity AuthIdentity, databaseID, workspaceID, workspaceName string, mounts []workspaceCompositionMount) bool {
	if strings.TrimSpace(identity.Provider) != "cli-token" {
		return true
	}
	scope := normalizeCLITokenScope(identity.Scope)
	if isAccountCLIScope(scope) {
		return true
	}
	if !isWorkspaceCLIScope(scope) {
		return false
	}
	if scopedDatabase := strings.TrimSpace(identity.ScopedDatabaseID); scopedDatabase != "" && scopedDatabase != strings.TrimSpace(databaseID) {
		return false
	}
	target := strings.TrimSpace(strings.TrimPrefix(scope, cliScopeWorkspacePrefix))
	if target == "" {
		return false
	}
	if target == strings.TrimSpace(workspaceID) || target == strings.TrimSpace(workspaceName) {
		return true
	}
	for _, mount := range mounts {
		if target == strings.TrimSpace(mount.VolumeID) || target == strings.TrimSpace(mount.VolumeName) {
			return true
		}
	}
	return false
}

func cliTokenAllowsWorkspaceCompositionSummary(identity AuthIdentity, item workspaceCompositionSummary) bool {
	mounts := make([]workspaceCompositionMount, 0, len(item.MountedVolumes))
	for _, mounted := range item.MountedVolumes {
		mounts = append(mounts, workspaceCompositionMount{
			VolumeID:   mounted.ID,
			VolumeName: mounted.Name,
		})
	}
	return cliTokenAllowsWorkspaceComposition(identity, item.DatabaseID, item.ID, item.Name, mounts)
}

func filterWorkspaceCompositionSummariesForCLIToken(ctx context.Context, items []workspaceCompositionSummary) []workspaceCompositionSummary {
	identity, ok := AuthIdentityFromContext(ctx)
	if !ok || strings.TrimSpace(identity.Provider) != "cli-token" || !isWorkspaceCLIScope(identity.Scope) {
		return items
	}
	filtered := make([]workspaceCompositionSummary, 0, len(items))
	for _, item := range items {
		if cliTokenAllowsWorkspaceCompositionSummary(identity, item) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func requireCLITokenWorkspaceCompositionAccess(ctx context.Context, databaseID, workspaceID, workspaceName string, mounts []workspaceCompositionMount) error {
	identity, ok := AuthIdentityFromContext(ctx)
	if !ok || cliTokenAllowsWorkspaceComposition(identity, databaseID, workspaceID, workspaceName, mounts) {
		return nil
	}
	return os.ErrNotExist
}

func applyCLITokenSessionPolicy(ctx context.Context, route workspaceCatalogRoute, input createWorkspaceSessionRequest) (createWorkspaceSessionRequest, error) {
	identity, ok := AuthIdentityFromContext(ctx)
	if !ok || strings.TrimSpace(identity.Provider) != "cli-token" {
		return input, nil
	}
	if !cliTokenAllowsWorkspace(identity, route.DatabaseID, route.WorkspaceID, route.Name) {
		return input, os.ErrNotExist
	}
	if capability, ok := identity.WorkspaceMountCapabilities[strings.TrimSpace(route.WorkspaceID)]; ok && cliCapabilityReadonly(capability) {
		input.Readonly = true
	}
	if cliCapabilityReadonly(identity.Capability) {
		input.Readonly = true
	}
	return input, nil
}
