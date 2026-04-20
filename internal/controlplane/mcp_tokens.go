package controlplane

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	mcpAccessTokenPrefix = "afs_mcp"
)

var ErrMCPAccessTokenInvalid = errors.New("mcp access token is invalid or expired")

type mcpAccessTokenRecord struct {
	ID            string `json:"id"`
	Name          string `json:"name,omitempty"`
	OwnerSubject  string `json:"owner_subject,omitempty"`
	OwnerLabel    string `json:"owner_label,omitempty"`
	DatabaseID    string `json:"database_id"`
	WorkspaceID   string `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name,omitempty"`
	Readonly      bool   `json:"readonly,omitempty"`
	SecretHash    string `json:"-"`
	CreatedAt     string `json:"created_at"`
	LastUsedAt    string `json:"last_used_at,omitempty"`
	ExpiresAt     string `json:"expires_at,omitempty"`
	RevokedAt     string `json:"revoked_at,omitempty"`
}

type mcpAccessTokenResponse struct {
	ID            string `json:"id"`
	Name          string `json:"name,omitempty"`
	DatabaseID    string `json:"database_id"`
	WorkspaceID   string `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name,omitempty"`
	Readonly      bool   `json:"readonly,omitempty"`
	Token         string `json:"token,omitempty"`
	CreatedAt     string `json:"created_at"`
	LastUsedAt    string `json:"last_used_at,omitempty"`
	ExpiresAt     string `json:"expires_at,omitempty"`
	RevokedAt     string `json:"revoked_at,omitempty"`
}

type createMCPAccessTokenRequest struct {
	Name      string `json:"name"`
	Readonly  bool   `json:"readonly,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

func (m *DatabaseManager) CreateResolvedMCPAccessToken(ctx context.Context, workspace string, input createMCPAccessTokenRequest) (mcpAccessTokenResponse, error) {
	if m == nil || m.catalog == nil {
		return mcpAccessTokenResponse{}, fmt.Errorf("mcp token storage is unavailable")
	}
	subject, label, err := m.requireOwnedSubject(ctx)
	if err != nil {
		return mcpAccessTokenResponse{}, err
	}
	_, profile, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return mcpAccessTokenResponse{}, err
	}
	if !databaseProfileVisibleToSubject(profile, subject) {
		return mcpAccessTokenResponse{}, os.ErrNotExist
	}
	return m.createMCPAccessTokenRecord(ctx, subject, label, profile.ID, route.WorkspaceID, route.Name, input)
}

func (m *DatabaseManager) CreateMCPAccessToken(ctx context.Context, databaseID, workspace string, input createMCPAccessTokenRequest) (mcpAccessTokenResponse, error) {
	if m == nil || m.catalog == nil {
		return mcpAccessTokenResponse{}, fmt.Errorf("mcp token storage is unavailable")
	}
	subject, label, err := m.requireOwnedSubject(ctx)
	if err != nil {
		return mcpAccessTokenResponse{}, err
	}
	_, profile, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return mcpAccessTokenResponse{}, err
	}
	if !databaseProfileVisibleToSubject(profile, subject) {
		return mcpAccessTokenResponse{}, os.ErrNotExist
	}
	return m.createMCPAccessTokenRecord(ctx, subject, label, profile.ID, route.WorkspaceID, route.Name, input)
}

func (m *DatabaseManager) createMCPAccessTokenRecord(ctx context.Context, subject, label, databaseID, workspaceID, workspaceName string, input createMCPAccessTokenRequest) (mcpAccessTokenResponse, error) {
	tokenID, secret, err := newMCPAccessTokenParts()
	if err != nil {
		return mcpAccessTokenResponse{}, err
	}
	now := time.Now().UTC()
	record := mcpAccessTokenRecord{
		ID:            tokenID,
		Name:          strings.TrimSpace(input.Name),
		OwnerSubject:  strings.TrimSpace(subject),
		OwnerLabel:    strings.TrimSpace(label),
		DatabaseID:    strings.TrimSpace(databaseID),
		WorkspaceID:   strings.TrimSpace(workspaceID),
		WorkspaceName: strings.TrimSpace(workspaceName),
		Readonly:      input.Readonly,
		SecretHash:    hashMCPAccessTokenSecret(secret),
		CreatedAt:     now.Format(timeRFC3339),
	}
	if expiresAt := strings.TrimSpace(input.ExpiresAt); expiresAt != "" {
		if _, err := time.Parse(timeRFC3339, expiresAt); err != nil {
			return mcpAccessTokenResponse{}, fmt.Errorf("expires_at must be RFC3339: %w", err)
		}
		record.ExpiresAt = expiresAt
	}
	if err := m.catalog.CreateMCPAccessToken(ctx, record); err != nil {
		return mcpAccessTokenResponse{}, err
	}
	response := mcpAccessTokenResponseFromRecord(record)
	response.Token = formatMCPAccessToken(tokenID, secret)
	return response, nil
}

func (m *DatabaseManager) ListResolvedMCPAccessTokens(ctx context.Context, workspace string) ([]mcpAccessTokenResponse, error) {
	if m == nil || m.catalog == nil {
		return nil, fmt.Errorf("mcp token storage is unavailable")
	}
	subject := authSubjectFromContext(ctx)
	_, profile, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return nil, err
	}
	if !databaseProfileVisibleToSubject(profile, subject) {
		return nil, os.ErrNotExist
	}
	items, err := m.catalog.ListMCPAccessTokens(ctx, profile.ID, route.WorkspaceID)
	if err != nil {
		return nil, err
	}
	return mcpAccessTokenResponses(items), nil
}

func (m *DatabaseManager) ListMCPAccessTokens(ctx context.Context, databaseID, workspace string) ([]mcpAccessTokenResponse, error) {
	if m == nil || m.catalog == nil {
		return nil, fmt.Errorf("mcp token storage is unavailable")
	}
	subject := authSubjectFromContext(ctx)
	_, profile, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return nil, err
	}
	if !databaseProfileVisibleToSubject(profile, subject) {
		return nil, os.ErrNotExist
	}
	items, err := m.catalog.ListMCPAccessTokens(ctx, profile.ID, route.WorkspaceID)
	if err != nil {
		return nil, err
	}
	return mcpAccessTokenResponses(items), nil
}

func (m *DatabaseManager) RevokeResolvedMCPAccessToken(ctx context.Context, workspace, tokenID string) error {
	if m == nil || m.catalog == nil {
		return fmt.Errorf("mcp token storage is unavailable")
	}
	subject := authSubjectFromContext(ctx)
	_, profile, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return err
	}
	if !databaseProfileVisibleToSubject(profile, subject) {
		return os.ErrNotExist
	}
	return m.catalog.RevokeMCPAccessToken(ctx, strings.TrimSpace(tokenID), profile.ID, route.WorkspaceID, time.Now().UTC().Format(timeRFC3339))
}

func (m *DatabaseManager) RevokeMCPAccessToken(ctx context.Context, databaseID, workspace, tokenID string) error {
	if m == nil || m.catalog == nil {
		return fmt.Errorf("mcp token storage is unavailable")
	}
	subject := authSubjectFromContext(ctx)
	_, profile, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return err
	}
	if !databaseProfileVisibleToSubject(profile, subject) {
		return os.ErrNotExist
	}
	return m.catalog.RevokeMCPAccessToken(ctx, strings.TrimSpace(tokenID), profile.ID, route.WorkspaceID, time.Now().UTC().Format(timeRFC3339))
}

func (m *DatabaseManager) AuthenticateMCPAccessToken(ctx context.Context, rawToken string) (mcpAccessTokenRecord, error) {
	if m == nil || m.catalog == nil {
		return mcpAccessTokenRecord{}, ErrMCPAccessTokenInvalid
	}
	tokenID, secret, err := parseMCPAccessToken(rawToken)
	if err != nil {
		return mcpAccessTokenRecord{}, ErrMCPAccessTokenInvalid
	}
	record, err := m.catalog.GetMCPAccessToken(ctx, tokenID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return mcpAccessTokenRecord{}, ErrMCPAccessTokenInvalid
		}
		return mcpAccessTokenRecord{}, err
	}
	if strings.TrimSpace(record.SecretHash) == "" || record.SecretHash != hashMCPAccessTokenSecret(secret) {
		return mcpAccessTokenRecord{}, ErrMCPAccessTokenInvalid
	}
	now := time.Now().UTC()
	if revoked := strings.TrimSpace(record.RevokedAt); revoked != "" {
		return mcpAccessTokenRecord{}, ErrMCPAccessTokenInvalid
	}
	if expiresAt := strings.TrimSpace(record.ExpiresAt); expiresAt != "" {
		expiry, err := time.Parse(timeRFC3339, expiresAt)
		if err != nil || !now.Before(expiry) {
			return mcpAccessTokenRecord{}, ErrMCPAccessTokenInvalid
		}
	}
	if err := m.catalog.TouchMCPAccessToken(ctx, tokenID, now.Format(timeRFC3339)); err != nil {
		return mcpAccessTokenRecord{}, err
	}
	record.LastUsedAt = now.Format(timeRFC3339)
	return record, nil
}

func mcpAccessTokenResponseFromRecord(record mcpAccessTokenRecord) mcpAccessTokenResponse {
	return mcpAccessTokenResponse{
		ID:            record.ID,
		Name:          record.Name,
		DatabaseID:    record.DatabaseID,
		WorkspaceID:   record.WorkspaceID,
		WorkspaceName: record.WorkspaceName,
		Readonly:      record.Readonly,
		CreatedAt:     record.CreatedAt,
		LastUsedAt:    record.LastUsedAt,
		ExpiresAt:     record.ExpiresAt,
		RevokedAt:     record.RevokedAt,
	}
}

func mcpAccessTokenResponses(records []mcpAccessTokenRecord) []mcpAccessTokenResponse {
	out := make([]mcpAccessTokenResponse, 0, len(records))
	for _, record := range records {
		out = append(out, mcpAccessTokenResponseFromRecord(record))
	}
	return out
}

func (m *DatabaseManager) requireOwnedSubject(ctx context.Context) (string, string, error) {
	identity, ok := AuthIdentityFromContext(ctx)
	if !ok || strings.TrimSpace(identity.Subject) == "" {
		return "", "", ErrUnauthorized
	}
	return strings.TrimSpace(identity.Subject), firstNonEmpty(identity.Name, identity.Email, identity.Subject), nil
}

func newMCPAccessTokenParts() (string, string, error) {
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

func formatMCPAccessToken(id, secret string) string {
	return mcpAccessTokenPrefix + "_" + strings.TrimSpace(id) + "_" + strings.TrimSpace(secret)
}

func parseMCPAccessToken(raw string) (string, string, error) {
	trimmed := strings.TrimSpace(raw)
	parts := strings.Split(trimmed, "_")
	if len(parts) != 4 || parts[0] != "afs" || parts[1] != "mcp" {
		return "", "", fmt.Errorf("invalid mcp token format")
	}
	if strings.TrimSpace(parts[2]) == "" || strings.TrimSpace(parts[3]) == "" {
		return "", "", fmt.Errorf("invalid mcp token format")
	}
	return strings.TrimSpace(parts[2]), strings.TrimSpace(parts[3]), nil
}

func hashMCPAccessTokenSecret(secret string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(secret)))
	return hex.EncodeToString(sum[:])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
