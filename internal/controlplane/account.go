package controlplane

import (
	"context"
	"strings"
)

type accountResponse struct {
	Subject               string `json:"subject,omitempty"`
	Provider              string `json:"provider"`
	CanDeleteIdentity     bool   `json:"can_delete_identity"`
	CanResetData          bool   `json:"can_reset_data"`
	OwnedDatabaseCount    int    `json:"owned_database_count"`
	OwnedWorkspaceCount   int    `json:"owned_workspace_count"`
	DeletedDatabaseCount  int    `json:"deleted_database_count,omitempty"`
	DeletedWorkspaceCount int    `json:"deleted_workspace_count,omitempty"`
	IdentityDeleted       bool   `json:"identity_deleted,omitempty"`
}

func (m *DatabaseManager) Account(ctx context.Context) (accountResponse, error) {
	subject := authSubjectFromContext(ctx)
	response := accountResponse{
		Subject:      subject,
		Provider:     accountProvider(ctx),
		CanResetData: strings.TrimSpace(subject) != "",
	}
	if strings.TrimSpace(subject) == "" {
		return response, nil
	}

	databaseIDs, workspaceCount, err := m.subjectOwnedDatabases(ctx, subject)
	if err != nil {
		return accountResponse{}, err
	}
	response.OwnedDatabaseCount = len(databaseIDs)
	response.OwnedWorkspaceCount = workspaceCount
	return response, nil
}

func (m *DatabaseManager) ResetAccountData(ctx context.Context) (accountResponse, error) {
	subject := authSubjectFromContext(ctx)
	if strings.TrimSpace(subject) == "" {
		return accountResponse{}, ErrUnauthorized
	}

	databaseIDs, workspaceCount, err := m.subjectOwnedDatabases(ctx, subject)
	if err != nil {
		return accountResponse{}, err
	}

	for _, databaseID := range databaseIDs {
		if err := m.DeleteDatabaseWithContext(ctx, databaseID); err != nil {
			return accountResponse{}, err
		}
	}

	return accountResponse{
		Subject:               subject,
		Provider:              accountProvider(ctx),
		CanResetData:          true,
		OwnedDatabaseCount:    0,
		OwnedWorkspaceCount:   0,
		DeletedDatabaseCount:  len(databaseIDs),
		DeletedWorkspaceCount: workspaceCount,
	}, nil
}

func (m *DatabaseManager) subjectOwnedDatabases(ctx context.Context, subject string) ([]string, int, error) {
	resolvedSubject := strings.TrimSpace(subject)
	if resolvedSubject == "" {
		return nil, 0, nil
	}

	m.mu.Lock()
	ownedDatabaseIDs := make([]string, 0, len(m.order))
	ownedDatabaseSet := make(map[string]struct{}, len(m.order))
	for _, databaseID := range m.order {
		profile := m.profiles[databaseID]
		if strings.TrimSpace(profile.OwnerSubject) != resolvedSubject {
			continue
		}
		if !databaseProfileCanDelete(profile) {
			continue
		}
		ownedDatabaseIDs = append(ownedDatabaseIDs, databaseID)
		ownedDatabaseSet[databaseID] = struct{}{}
	}
	catalog := m.catalog
	m.mu.Unlock()

	if len(ownedDatabaseIDs) == 0 || catalog == nil {
		return ownedDatabaseIDs, 0, nil
	}

	workspaces, err := catalog.ListWorkspaces(ctx)
	if err != nil {
		return nil, 0, err
	}
	workspaceCount := 0
	for _, workspace := range workspaces {
		if _, ok := ownedDatabaseSet[workspace.DatabaseID]; ok {
			workspaceCount++
		}
	}
	return ownedDatabaseIDs, workspaceCount, nil
}

func accountProvider(ctx context.Context) string {
	identity, ok := AuthIdentityFromContext(ctx)
	if !ok {
		return string(AuthModeNone)
	}
	if value := strings.TrimSpace(identity.Provider); value != "" {
		return value
	}
	return string(AuthModeNone)
}
