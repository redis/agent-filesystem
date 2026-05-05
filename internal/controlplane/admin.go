package controlplane

import (
	"context"
	"os"
	"sort"
	"strings"
	"time"
)

type adminOverviewResponse struct {
	UserCount                int   `json:"user_count"`
	DatabaseCount            int   `json:"database_count"`
	WorkspaceCount           int   `json:"workspace_count"`
	AgentCount               int   `json:"agent_count"`
	ActiveAgentCount         int   `json:"active_agent_count"`
	StaleAgentCount          int   `json:"stale_agent_count"`
	UnavailableDatabaseCount int   `json:"unavailable_database_count"`
	TotalBytes               int64 `json:"total_bytes"`
	FileCount                int   `json:"file_count"`
}

type adminUserRecord struct {
	Subject           string   `json:"subject"`
	Label             string   `json:"label,omitempty"`
	DatabaseCount     int      `json:"database_count"`
	WorkspaceCount    int      `json:"workspace_count"`
	MCPTokenCount     int      `json:"mcp_token_count"`
	AgentSessionCount int      `json:"agent_session_count"`
	LastSeenAt        string   `json:"last_seen_at,omitempty"`
	Sources           []string `json:"sources"`
}

type adminUserListResponse struct {
	Items []adminUserRecord `json:"items"`
}

func isCloudAdminIdentity(identity AuthIdentity) bool {
	if ProductModeFromEnv() != ProductModeCloud {
		return false
	}
	subject := strings.TrimSpace(identity.Subject)
	if subject == "" {
		return false
	}
	switch strings.TrimSpace(identity.Provider) {
	case "cli-token", "mcp-token":
		return false
	}
	for _, candidate := range splitHeaderValues(os.Getenv(authAdminSubjectsEnvVar)) {
		if candidate == subject {
			return true
		}
	}
	return false
}

func adminUnscopedContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, authIdentityContextKey, AuthIdentity{})
}

func (m *DatabaseManager) AdminOverview(ctx context.Context) (adminOverviewResponse, error) {
	databases, err := m.AdminDatabases(ctx)
	if err != nil {
		return adminOverviewResponse{}, err
	}
	workspaces, err := m.AdminWorkspaces(ctx)
	if err != nil {
		return adminOverviewResponse{}, err
	}
	agents, err := m.AdminAgents(ctx)
	if err != nil {
		return adminOverviewResponse{}, err
	}
	users, err := m.AdminUsers(ctx)
	if err != nil {
		return adminOverviewResponse{}, err
	}

	response := adminOverviewResponse{
		UserCount:      len(users.Items),
		DatabaseCount:  len(databases.Items),
		WorkspaceCount: len(workspaces.Items),
		AgentCount:     len(agents.Items),
	}
	for _, database := range databases.Items {
		if strings.TrimSpace(database.ConnectionError) != "" {
			response.UnavailableDatabaseCount++
		}
	}
	for _, workspace := range workspaces.Items {
		response.TotalBytes += workspace.TotalBytes
		response.FileCount += workspace.FileCount
	}
	for _, agent := range agents.Items {
		switch strings.TrimSpace(agent.State) {
		case workspaceSessionStateStale:
			response.StaleAgentCount++
		default:
			response.ActiveAgentCount++
		}
	}
	return response, nil
}

func (m *DatabaseManager) AdminDatabases(ctx context.Context) (databaseListResponse, error) {
	return m.ListDatabases(adminUnscopedContext(ctx))
}

func (m *DatabaseManager) AdminWorkspaces(ctx context.Context) (workspaceListResponse, error) {
	return m.listAllWorkspaceSummariesByFanout(adminUnscopedContext(ctx))
}

func (m *DatabaseManager) AdminAgents(ctx context.Context) (workspaceSessionListResponse, error) {
	if m == nil {
		return workspaceSessionListResponse{Items: []workspaceSessionInfo{}}, nil
	}
	ctx = adminUnscopedContext(ctx)
	if err := m.reconcileListedAgentSessions(ctx, ""); err != nil {
		return workspaceSessionListResponse{}, err
	}

	m.mu.Lock()
	catalog := m.catalog
	profiles := make(map[string]databaseProfile, len(m.profiles))
	for id, profile := range m.profiles {
		profiles[id] = profile
	}
	m.mu.Unlock()
	if catalog == nil {
		return workspaceSessionListResponse{Items: []workspaceSessionInfo{}}, nil
	}

	records, err := catalog.ListAllSessions(ctx)
	if err != nil {
		return workspaceSessionListResponse{}, err
	}
	owners, err := m.adminWorkspaceOwners(ctx)
	if err != nil {
		return workspaceSessionListResponse{}, err
	}

	now := time.Now().UTC()
	items := make([]workspaceSessionInfo, 0, len(records))
	for _, record := range records {
		if sessionCatalogRecordLeaseExpired(record, now) {
			record = staleSessionCatalogRecord(record, now, "expired")
			_ = catalog.UpsertSession(ctx, record)
		}
		profile, ok := profiles[record.DatabaseID]
		if !ok {
			continue
		}
		databaseName := record.DatabaseID
		if strings.TrimSpace(profile.Name) != "" {
			databaseName = profile.Name
		}
		owner := owners[adminWorkspaceKey(record.DatabaseID, record.WorkspaceID)]
		items = append(items, workspaceSessionInfo{
			SessionID:       record.SessionID,
			Workspace:       defaultString(record.WorkspaceName, record.WorkspaceID),
			WorkspaceID:     record.WorkspaceID,
			WorkspaceName:   record.WorkspaceName,
			DatabaseID:      record.DatabaseID,
			DatabaseName:    databaseName,
			OwnerSubject:    owner.Subject,
			OwnerLabel:      owner.Label,
			AgentID:         record.AgentID,
			AgentName:       record.AgentName,
			SessionName:     record.SessionName,
			ClientKind:      record.ClientKind,
			AFSVersion:      record.AFSVersion,
			Hostname:        record.Hostname,
			OperatingSystem: record.OperatingSystem,
			LocalPath:       record.LocalPath,
			Label:           record.Label,
			Readonly:        record.Readonly,
			State:           record.State,
			StartedAt:       record.StartedAt,
			LastSeenAt:      record.LastSeenAt,
			LeaseExpiresAt:  record.LeaseExpiresAt,
		})
	}
	return workspaceSessionListResponse{Items: items}, nil
}

func (m *DatabaseManager) AdminUsers(ctx context.Context) (adminUserListResponse, error) {
	databases, err := m.AdminDatabases(ctx)
	if err != nil {
		return adminUserListResponse{}, err
	}
	workspaces, err := m.AdminWorkspaces(ctx)
	if err != nil {
		return adminUserListResponse{}, err
	}
	agents, err := m.AdminAgents(ctx)
	if err != nil {
		return adminUserListResponse{}, err
	}

	records := make(map[string]*adminUserRecord)
	upsert := func(subject, label, source string) *adminUserRecord {
		subject = strings.TrimSpace(subject)
		if subject == "" {
			return nil
		}
		record, ok := records[subject]
		if !ok {
			record = &adminUserRecord{Subject: subject}
			records[subject] = record
		}
		if strings.TrimSpace(record.Label) == "" {
			record.Label = strings.TrimSpace(label)
		}
		if source != "" && !stringSliceContains(record.Sources, source) {
			record.Sources = append(record.Sources, source)
		}
		return record
	}

	for _, database := range databases.Items {
		if record := upsert(database.OwnerSubject, database.OwnerLabel, "database"); record != nil {
			record.DatabaseCount++
		}
	}
	for _, workspace := range workspaces.Items {
		if record := upsert(workspace.OwnerSubject, workspace.OwnerLabel, "workspace"); record != nil {
			record.WorkspaceCount++
		}
	}
	for _, agent := range agents.Items {
		if record := upsert(agent.OwnerSubject, agent.OwnerLabel, "agent"); record != nil {
			record.AgentSessionCount++
			if compareRFC3339(agent.LastSeenAt, record.LastSeenAt) > 0 {
				record.LastSeenAt = agent.LastSeenAt
			}
		}
	}
	if m != nil && m.catalog != nil {
		tokens, err := m.catalog.ListAllMCPAccessTokens(ctx)
		if err != nil {
			return adminUserListResponse{}, err
		}
		for _, token := range tokens {
			if strings.TrimSpace(token.RevokedAt) != "" {
				continue
			}
			if record := upsert(token.OwnerSubject, token.OwnerLabel, "mcp-token"); record != nil {
				record.MCPTokenCount++
				if compareRFC3339(token.LastUsedAt, record.LastSeenAt) > 0 {
					record.LastSeenAt = token.LastUsedAt
				}
			}
		}
	}

	items := make([]adminUserRecord, 0, len(records))
	for _, record := range records {
		sort.Strings(record.Sources)
		items = append(items, *record)
	}
	sort.Slice(items, func(i, j int) bool {
		left := strings.ToLower(defaultString(items[i].Label, items[i].Subject))
		right := strings.ToLower(defaultString(items[j].Label, items[j].Subject))
		if left == right {
			return items[i].Subject < items[j].Subject
		}
		return left < right
	})
	return adminUserListResponse{Items: items}, nil
}

func (m *DatabaseManager) adminWorkspaceOwners(ctx context.Context) (map[string]catalogOwnerInfo, error) {
	workspaces, err := m.AdminWorkspaces(ctx)
	if err != nil {
		return nil, err
	}
	owners := make(map[string]catalogOwnerInfo, len(workspaces.Items))
	for _, workspace := range workspaces.Items {
		owners[adminWorkspaceKey(workspace.DatabaseID, workspace.ID)] = catalogOwnerInfo{
			Subject: strings.TrimSpace(workspace.OwnerSubject),
			Label:   strings.TrimSpace(workspace.OwnerLabel),
		}
	}
	return owners, nil
}

func adminWorkspaceKey(databaseID, workspaceID string) string {
	return strings.TrimSpace(databaseID) + "\x00" + strings.TrimSpace(workspaceID)
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func compareRFC3339(left, right string) int {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" && right == "" {
		return 0
	}
	if left == "" {
		return -1
	}
	if right == "" {
		return 1
	}
	leftTime, leftErr := time.Parse(timeRFC3339, left)
	rightTime, rightErr := time.Parse(timeRFC3339, right)
	if leftErr == nil && rightErr == nil {
		switch {
		case leftTime.After(rightTime):
			return 1
		case leftTime.Before(rightTime):
			return -1
		default:
			return 0
		}
	}
	return strings.Compare(left, right)
}
