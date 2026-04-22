package controlplane

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

const onboardingTokenTTL = 15 * time.Minute

var ErrOnboardingTokenInvalid = errors.New("onboarding token is invalid or expired")

type onboardingTokenResponse struct {
	Token         string `json:"token"`
	DatabaseID    string `json:"database_id"`
	WorkspaceID   string `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name"`
	ExpiresAt     string `json:"expires_at"`
}

type onboardingExchangeRequest struct {
	Token string `json:"token"`
}

type onboardingExchangeResponse struct {
	DatabaseID    string `json:"database_id"`
	WorkspaceID   string `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name"`
	AccessToken   string `json:"access_token,omitempty"`
	Account       string `json:"account,omitempty"`
}

func (m *DatabaseManager) CreateResolvedOnboardingToken(ctx context.Context, workspace string) (onboardingTokenResponse, error) {
	if m == nil || m.catalog == nil {
		return onboardingTokenResponse{}, fmt.Errorf("onboarding token storage is unavailable")
	}

	_, profile, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return onboardingTokenResponse{}, err
	}
	return m.createOnboardingTokenRecord(ctx, profile.ID, route.WorkspaceID, route.Name)
}

func (m *DatabaseManager) CreateOnboardingToken(ctx context.Context, databaseID, workspace string) (onboardingTokenResponse, error) {
	if m == nil || m.catalog == nil {
		return onboardingTokenResponse{}, fmt.Errorf("onboarding token storage is unavailable")
	}

	_, profile, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return onboardingTokenResponse{}, err
	}
	return m.createOnboardingTokenRecord(ctx, profile.ID, route.WorkspaceID, route.Name)
}

func (m *DatabaseManager) createOnboardingTokenRecord(ctx context.Context, databaseID, workspaceID, workspaceName string) (onboardingTokenResponse, error) {
	token, err := newOnboardingToken()
	if err != nil {
		return onboardingTokenResponse{}, err
	}
	now := time.Now().UTC()
	expiresAt := now.Add(onboardingTokenTTL)
	ownerSubject := authSubjectFromContext(ctx)
	ownerLabel := ownerSubject
	if identity, ok := AuthIdentityFromContext(ctx); ok {
		ownerLabel = firstNonEmpty(identity.Name, identity.Email, ownerSubject)
	}
	record := onboardingTokenRecord{
		Token:         token,
		OwnerSubject:  ownerSubject,
		OwnerLabel:    ownerLabel,
		DatabaseID:    databaseID,
		WorkspaceID:   workspaceID,
		WorkspaceName: workspaceName,
		CreatedAt:     now.Format(timeRFC3339),
		ExpiresAt:     expiresAt.Format(timeRFC3339),
	}
	if err := m.catalog.CreateOnboardingToken(ctx, record); err != nil {
		return onboardingTokenResponse{}, err
	}

	return onboardingTokenResponse{
		Token:         record.Token,
		DatabaseID:    record.DatabaseID,
		WorkspaceID:   record.WorkspaceID,
		WorkspaceName: record.WorkspaceName,
		ExpiresAt:     record.ExpiresAt,
	}, nil
}

func (m *DatabaseManager) ExchangeOnboardingToken(ctx context.Context, token string) (onboardingExchangeResponse, error) {
	if m == nil || m.catalog == nil {
		return onboardingExchangeResponse{}, fmt.Errorf("onboarding token storage is unavailable")
	}

	record, err := m.catalog.ConsumeOnboardingToken(ctx, strings.TrimSpace(token), time.Now().UTC().Format(timeRFC3339))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return onboardingExchangeResponse{}, ErrOnboardingTokenInvalid
		}
		return onboardingExchangeResponse{}, err
	}
	var accessToken string
	if strings.TrimSpace(record.OwnerSubject) != "" {
		accessToken, err = m.createCLIAccessTokenRecord(ctx, record.OwnerSubject, record.OwnerLabel, record.DatabaseID, record.WorkspaceID, record.WorkspaceName)
		if err != nil {
			return onboardingExchangeResponse{}, err
		}
	}

	return onboardingExchangeResponse{
		DatabaseID:    record.DatabaseID,
		WorkspaceID:   record.WorkspaceID,
		WorkspaceName: record.WorkspaceName,
		AccessToken:   accessToken,
		Account:       record.OwnerLabel,
	}, nil
}

func newOnboardingToken() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return "afs_otk_" + hex.EncodeToString(raw[:]), nil
}
