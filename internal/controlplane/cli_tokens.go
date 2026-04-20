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
	cliAccessTokenPrefix = "afs_cli"
)

var ErrCLIAccessTokenInvalid = errors.New("cli access token is invalid or expired")

type cliAccessTokenRecord struct {
	ID            string `json:"id"`
	OwnerSubject  string `json:"owner_subject,omitempty"`
	OwnerLabel    string `json:"owner_label,omitempty"`
	DatabaseID    string `json:"database_id"`
	WorkspaceID   string `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name,omitempty"`
	SecretHash    string `json:"-"`
	CreatedAt     string `json:"created_at"`
	LastUsedAt    string `json:"last_used_at,omitempty"`
	ExpiresAt     string `json:"expires_at,omitempty"`
	RevokedAt     string `json:"revoked_at,omitempty"`
}

func (m *DatabaseManager) createCLIAccessTokenRecord(ctx context.Context, subject, label, databaseID, workspaceID, workspaceName string) (string, error) {
	if m == nil || m.catalog == nil {
		return "", fmt.Errorf("cli token storage is unavailable")
	}
	tokenID, secret, err := newCLIAccessTokenParts()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	record := cliAccessTokenRecord{
		ID:            tokenID,
		OwnerSubject:  strings.TrimSpace(subject),
		OwnerLabel:    strings.TrimSpace(label),
		DatabaseID:    strings.TrimSpace(databaseID),
		WorkspaceID:   strings.TrimSpace(workspaceID),
		WorkspaceName: strings.TrimSpace(workspaceName),
		SecretHash:    hashCLIAccessTokenSecret(secret),
		CreatedAt:     now.Format(timeRFC3339),
	}
	if err := m.catalog.CreateCLIAccessToken(ctx, record); err != nil {
		return "", err
	}
	return formatCLIAccessToken(tokenID, secret), nil
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
