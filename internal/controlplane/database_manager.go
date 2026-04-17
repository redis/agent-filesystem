package controlplane

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type databaseProfile struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	RedisAddr     string `json:"redis_addr"`
	RedisUsername string `json:"redis_username,omitempty"`
	RedisPassword string `json:"redis_password,omitempty"`
	RedisDB       int    `json:"redis_db"`
	RedisTLS      bool   `json:"redis_tls"`
	IsDefault     bool   `json:"is_default"`
}

type databaseRecord struct {
	ID                        string `json:"id"`
	Name                      string `json:"name"`
	Description               string `json:"description,omitempty"`
	RedisAddr                 string `json:"redis_addr"`
	RedisUsername             string `json:"redis_username,omitempty"`
	RedisDB                   int    `json:"redis_db"`
	RedisTLS                  bool   `json:"redis_tls"`
	IsDefault                 bool   `json:"is_default"`
	WorkspaceCount            int    `json:"workspace_count"`
	ActiveSessionCount        int    `json:"active_session_count"`
	ConnectionError           string `json:"connection_error,omitempty"`
	LastWorkspaceRefreshAt    string `json:"last_workspace_refresh_at,omitempty"`
	LastWorkspaceRefreshError string `json:"last_workspace_refresh_error,omitempty"`
	LastSessionReconcileAt    string `json:"last_session_reconcile_at,omitempty"`
	LastSessionReconcileError string `json:"last_session_reconcile_error,omitempty"`
}

var ErrAmbiguousDatabase = errors.New("control plane database is ambiguous")

type databaseListResponse struct {
	Items []databaseRecord `json:"items"`
}

type catalogHealthResponse struct {
	GeneratedAt string           `json:"generated_at"`
	Items       []databaseRecord `json:"items"`
}

type upsertDatabaseRequest struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	RedisAddr     string `json:"redis_addr"`
	RedisUsername string `json:"redis_username"`
	RedisPassword string `json:"redis_password"`
	RedisDB       int    `json:"redis_db"`
	RedisTLS      bool   `json:"redis_tls"`
}

type databaseRuntime struct {
	cfg     Config
	store   *Store
	closeFn func()
}

type DatabaseRecord = databaseRecord
type DatabaseListResponse = databaseListResponse
type UpsertDatabaseRequest = upsertDatabaseRequest

type DatabaseManager struct {
	mu       sync.Mutex
	catalog  *workspaceCatalog
	profiles map[string]databaseProfile
	order    []string
	runtimes map[string]*databaseRuntime
}

func OpenDatabaseManager(configPathOverride string) (*DatabaseManager, error) {
	catalog, err := openWorkspaceCatalog(configPathOverride)
	if err != nil {
		return nil, err
	}

	loadedProfiles, err := catalog.ListDatabaseProfiles(context.Background())
	if err != nil {
		_ = catalog.Close()
		return nil, err
	}

	manager := &DatabaseManager{
		catalog:  catalog,
		profiles: make(map[string]databaseProfile, len(loadedProfiles)),
		order:    make([]string, 0, len(loadedProfiles)),
		runtimes: make(map[string]*databaseRuntime),
	}
	for _, profile := range loadedProfiles {
		if err := validateDatabaseProfile(profile); err != nil {
			return nil, err
		}
		if _, exists := manager.profiles[profile.ID]; exists {
			return nil, fmt.Errorf("duplicate database id %q", profile.ID)
		}
		manager.profiles[profile.ID] = profile
		manager.order = append(manager.order, profile.ID)
	}
	manager.ensureDefaultDatabaseLocked()

	if err := manager.saveRegistryLocked(); err != nil {
		_ = manager.catalog.Close()
		return nil, err
	}
	if err := manager.refreshWorkspaceCatalog(context.Background()); err != nil {
		manager.Close()
		return nil, err
	}

	return manager, nil
}

func (m *DatabaseManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, runtime := range m.runtimes {
		runtime.closeFn()
		delete(m.runtimes, id)
	}
	if m.catalog != nil {
		_ = m.catalog.Close()
		m.catalog = nil
	}
}

func (m *DatabaseManager) ListDatabases(ctx context.Context) (databaseListResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	healthByDatabase := map[string]databaseCatalogHealth{}
	activeSessionsByDatabase := map[string]int{}
	if m.catalog != nil {
		var err error
		healthByDatabase, err = m.catalog.ListDatabaseHealth(ctx)
		if err != nil {
			return databaseListResponse{}, err
		}
		activeSessionsByDatabase, err = m.catalog.CountActiveSessionsByDatabase(ctx)
		if err != nil {
			return databaseListResponse{}, err
		}
	}

	items := make([]databaseRecord, 0, len(m.order))
	for _, id := range m.order {
		profile := m.profiles[id]
		record := databaseRecord{
			ID:            profile.ID,
			Name:          profile.Name,
			Description:   profile.Description,
			RedisAddr:     profile.RedisAddr,
			RedisUsername: profile.RedisUsername,
			RedisDB:       profile.RedisDB,
			RedisTLS:      profile.RedisTLS,
			IsDefault:     profile.IsDefault,
		}
		if health, ok := healthByDatabase[id]; ok {
			record.LastWorkspaceRefreshAt = health.LastWorkspaceRefreshAt
			record.LastWorkspaceRefreshError = health.LastWorkspaceRefreshError
			record.LastSessionReconcileAt = health.LastSessionReconcileAt
			record.LastSessionReconcileError = health.LastSessionReconcileError
		}
		record.ActiveSessionCount = activeSessionsByDatabase[id]

		workspaces, _, err := m.liveWorkspaceSummariesLocked(ctx, id)
		if err != nil {
			record.ConnectionError = err.Error()
			items = append(items, record)
			continue
		}
		record.WorkspaceCount = len(workspaces)
		items = append(items, record)
	}

	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	return databaseListResponse{Items: items}, nil
}

func (m *DatabaseManager) CatalogHealth(ctx context.Context) (catalogHealthResponse, error) {
	response, err := m.ListDatabases(ctx)
	if err != nil {
		return catalogHealthResponse{}, err
	}
	return catalogHealthResponse{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Items:       response.Items,
	}, nil
}

func (m *DatabaseManager) ReconcileCatalog(ctx context.Context) (catalogHealthResponse, error) {
	if err := m.refreshWorkspaceCatalog(ctx); err != nil {
		return catalogHealthResponse{}, err
	}
	if err := m.reconcileSessionCatalog(ctx); err != nil {
		return catalogHealthResponse{}, err
	}
	return m.CatalogHealth(ctx)
}

func (m *DatabaseManager) UpsertDatabase(ctx context.Context, id string, input upsertDatabaseRequest) (databaseRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	profile, isNew, err := m.buildProfileLocked(id, input)
	if err != nil {
		return databaseRecord{}, err
	}
	if err := validateDatabaseProfile(profile); err != nil {
		return databaseRecord{}, err
	}

	runtime, err := openDatabaseRuntime(ctx, profile)
	if err != nil {
		return databaseRecord{}, err
	}

	oldRuntime := m.runtimes[profile.ID]
	oldProfiles := cloneDatabaseProfiles(m.profiles)

	m.profiles[profile.ID] = profile
	if isNew {
		m.order = append(m.order, profile.ID)
	}
	m.runtimes[profile.ID] = runtime

	if err := m.saveRegistryLocked(); err != nil {
		runtime.closeFn()
		m.profiles = oldProfiles
		if oldRuntime != nil {
			m.runtimes[profile.ID] = oldRuntime
		} else {
			delete(m.runtimes, profile.ID)
		}
		if isNew {
			m.order = withoutValue(m.order, profile.ID)
		}
		return databaseRecord{}, err
	}

	if oldRuntime != nil {
		oldRuntime.closeFn()
	}

	record := databaseRecord{
		ID:            profile.ID,
		Name:          profile.Name,
		Description:   profile.Description,
		RedisAddr:     profile.RedisAddr,
		RedisUsername: profile.RedisUsername,
		RedisDB:       profile.RedisDB,
		RedisTLS:      profile.RedisTLS,
		IsDefault:     profile.IsDefault,
	}
	items, _, err := m.liveWorkspaceSummariesLocked(ctx, profile.ID)
	if err != nil {
		record.ConnectionError = err.Error()
	} else {
		record.WorkspaceCount = len(items)
	}
	return record, nil
}

func (m *DatabaseManager) DeleteDatabase(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.profiles[id]; !exists {
		return os.ErrNotExist
	}
	oldRuntime := m.runtimes[id]
	oldOrder := append([]string(nil), m.order...)
	oldProfiles := cloneDatabaseProfiles(m.profiles)

	delete(m.profiles, id)
	delete(m.runtimes, id)
	m.order = withoutValue(m.order, id)
	m.ensureDefaultDatabaseLocked()

	if err := m.saveRegistryLocked(); err != nil {
		m.profiles = oldProfiles
		if oldRuntime != nil {
			m.runtimes[id] = oldRuntime
		}
		m.order = oldOrder
		return err
	}

	if oldRuntime != nil {
		oldRuntime.closeFn()
	}
	if m.catalog != nil {
		if err := m.catalog.DeleteDatabaseWorkspaces(context.Background(), id); err != nil {
			return err
		}
	}
	return nil
}

func (m *DatabaseManager) ListWorkspaceSummaries(ctx context.Context, databaseID string) (workspaceListResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	items, _, err := m.liveWorkspaceSummariesLocked(ctx, databaseID)
	if err != nil {
		return workspaceListResponse{}, err
	}
	return workspaceListResponse{Items: items}, nil
}

func (m *DatabaseManager) SetDefaultDatabase(ctx context.Context, id string) (databaseRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	profile, exists := m.profiles[strings.TrimSpace(id)]
	if !exists {
		return databaseRecord{}, os.ErrNotExist
	}
	oldProfiles := cloneDatabaseProfiles(m.profiles)
	for databaseID, candidate := range m.profiles {
		candidate.IsDefault = databaseID == profile.ID
		m.profiles[databaseID] = candidate
	}
	if err := m.saveRegistryLocked(); err != nil {
		m.profiles = oldProfiles
		return databaseRecord{}, err
	}
	return m.databaseRecordLocked(ctx, profile.ID)
}

func (m *DatabaseManager) ListAllWorkspaceSummaries(ctx context.Context) (workspaceListResponse, error) {
	return m.listAllWorkspaceSummariesByFanout(ctx)
}

func (m *DatabaseManager) GetWorkspace(ctx context.Context, databaseID, workspace string) (workspaceDetail, error) {
	service, profile, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return workspaceDetail{}, err
	}
	detail, err := service.GetWorkspace(ctx, route.Name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			_ = m.deleteWorkspaceFromCatalog(ctx, route.DatabaseID, route)
		}
		return workspaceDetail{}, err
	}
	if err := m.attachWorkspaceDetailIdentity(ctx, &detail, profile); err != nil {
		return workspaceDetail{}, err
	}
	return detail, nil
}

func (m *DatabaseManager) GetResolvedWorkspace(ctx context.Context, workspace string) (workspaceDetail, error) {
	service, profile, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return workspaceDetail{}, err
	}
	detail, err := service.GetWorkspace(ctx, route.Name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			_ = m.deleteWorkspaceFromCatalog(ctx, route.DatabaseID, route)
		}
		return workspaceDetail{}, err
	}
	if err := m.attachWorkspaceDetailIdentity(ctx, &detail, profile); err != nil {
		return workspaceDetail{}, err
	}
	return detail, nil
}

func (m *DatabaseManager) CreateWorkspace(ctx context.Context, databaseID string, input createWorkspaceRequest) (workspaceDetail, error) {
	service, profile, err := m.serviceFor(ctx, databaseID)
	if err != nil {
		return workspaceDetail{}, err
	}
	input.DatabaseID = profile.ID
	input.DatabaseName = profile.Name
	if strings.TrimSpace(input.CloudAccount) == "" {
		input.CloudAccount = "Direct Redis"
	}
	detail, err := service.CreateWorkspace(ctx, input)
	if err != nil {
		return workspaceDetail{}, err
	}
	if err := m.attachWorkspaceDetailIdentity(ctx, &detail, profile); err != nil {
		return workspaceDetail{}, err
	}
	return detail, nil
}

func (m *DatabaseManager) CreateResolvedWorkspace(ctx context.Context, input createWorkspaceRequest) (workspaceDetail, error) {
	profile, err := m.resolveTargetDatabase(ctx, input.DatabaseID)
	if err != nil {
		return workspaceDetail{}, err
	}
	return m.CreateWorkspace(ctx, profile.ID, input)
}

func (m *DatabaseManager) UpdateWorkspace(ctx context.Context, databaseID, workspace string, input updateWorkspaceRequest) (workspaceDetail, error) {
	service, profile, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return workspaceDetail{}, err
	}
	if strings.TrimSpace(input.DatabaseName) == "" {
		input.DatabaseName = profile.Name
	}
	if strings.TrimSpace(input.CloudAccount) == "" {
		input.CloudAccount = "Direct Redis"
	}
	detail, err := service.UpdateWorkspace(ctx, route.Name, input)
	if err != nil {
		return workspaceDetail{}, err
	}
	if err := m.attachWorkspaceDetailIdentity(ctx, &detail, profile); err != nil {
		return workspaceDetail{}, err
	}
	return detail, nil
}

func (m *DatabaseManager) UpdateResolvedWorkspace(ctx context.Context, workspace string, input updateWorkspaceRequest) (workspaceDetail, error) {
	service, profile, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return workspaceDetail{}, err
	}
	if strings.TrimSpace(input.DatabaseName) == "" {
		input.DatabaseName = profile.Name
	}
	if strings.TrimSpace(input.CloudAccount) == "" {
		input.CloudAccount = "Direct Redis"
	}
	detail, err := service.UpdateWorkspace(ctx, route.Name, input)
	if err != nil {
		return workspaceDetail{}, err
	}
	if err := m.attachWorkspaceDetailIdentity(ctx, &detail, profile); err != nil {
		return workspaceDetail{}, err
	}
	return detail, nil
}

func (m *DatabaseManager) DeleteWorkspace(ctx context.Context, databaseID, workspace string) error {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return err
	}
	if err := service.DeleteWorkspace(ctx, route.Name); err != nil {
		return err
	}
	return m.deleteWorkspaceFromCatalog(ctx, route.DatabaseID, route)
}

func (m *DatabaseManager) DeleteResolvedWorkspace(ctx context.Context, workspace string) error {
	service, _, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return err
	}
	if err := service.DeleteWorkspace(ctx, route.Name); err != nil {
		return err
	}
	return m.deleteWorkspaceFromCatalog(ctx, route.DatabaseID, route)
}

func (m *DatabaseManager) ListGlobalActivity(ctx context.Context, databaseID string, limit int) (activityListResponse, error) {
	service, _, err := m.serviceFor(ctx, databaseID)
	if err != nil {
		return activityListResponse{}, err
	}
	return service.ListGlobalActivity(ctx, limit)
}

func (m *DatabaseManager) ListWorkspaceActivity(ctx context.Context, databaseID, workspace string, limit int) (activityListResponse, error) {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return activityListResponse{}, err
	}
	return service.ListWorkspaceActivity(ctx, route.Name, limit)
}

func (m *DatabaseManager) ListResolvedWorkspaceActivity(ctx context.Context, workspace string, limit int) (activityListResponse, error) {
	service, _, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return activityListResponse{}, err
	}
	return service.ListWorkspaceActivity(ctx, route.Name, limit)
}

func (m *DatabaseManager) RestoreCheckpoint(ctx context.Context, databaseID, workspace, checkpointID string) error {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return err
	}
	if err := service.RestoreCheckpoint(ctx, route.Name, checkpointID); err != nil {
		return err
	}
	return m.refreshWorkspaceCatalogEntry(ctx, route.DatabaseID, route.Name)
}

func (m *DatabaseManager) RestoreResolvedCheckpoint(ctx context.Context, workspace, checkpointID string) error {
	service, _, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return err
	}
	if err := service.RestoreCheckpoint(ctx, route.Name, checkpointID); err != nil {
		return err
	}
	return m.refreshWorkspaceCatalogEntry(ctx, route.DatabaseID, route.Name)
}

func (m *DatabaseManager) ListCheckpoints(ctx context.Context, databaseID, workspace string, limit int) ([]checkpointSummary, error) {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return nil, err
	}
	return service.ListCheckpoints(ctx, route.Name, limit)
}

func (m *DatabaseManager) ListResolvedCheckpoints(ctx context.Context, workspace string, limit int) ([]checkpointSummary, error) {
	service, _, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return nil, err
	}
	return service.ListCheckpoints(ctx, route.Name, limit)
}

func (m *DatabaseManager) SaveCheckpoint(ctx context.Context, databaseID, workspace string, input SaveCheckpointRequest) (bool, error) {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return false, err
	}
	input.Workspace = route.Name
	saved, err := service.SaveCheckpoint(ctx, input)
	if err != nil {
		return false, err
	}
	if saved {
		if err := m.refreshWorkspaceCatalogEntry(ctx, route.DatabaseID, route.Name); err != nil {
			return false, err
		}
	}
	return saved, nil
}

func (m *DatabaseManager) SaveResolvedCheckpoint(ctx context.Context, workspace string, input SaveCheckpointRequest) (bool, error) {
	service, _, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return false, err
	}
	input.Workspace = route.Name
	saved, err := service.SaveCheckpoint(ctx, input)
	if err != nil {
		return false, err
	}
	if saved {
		if err := m.refreshWorkspaceCatalogEntry(ctx, route.DatabaseID, route.Name); err != nil {
			return false, err
		}
	}
	return saved, nil
}

func (m *DatabaseManager) SaveResolvedCheckpointFromLive(ctx context.Context, workspace, checkpointID string) (bool, error) {
	service, _, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return false, fmt.Errorf("resolve workspace %q: %w", workspace, err)
	}
	saved, err := service.SaveCheckpointFromLive(ctx, route.Name, checkpointID)
	if err != nil {
		return false, err
	}
	if saved {
		if err := m.refreshWorkspaceCatalogEntry(ctx, route.DatabaseID, route.Name); err != nil {
			return false, err
		}
	}
	return saved, nil
}

func (m *DatabaseManager) SaveCheckpointFromLive(ctx context.Context, databaseID, workspace, checkpointID string) (bool, error) {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return false, fmt.Errorf("resolve workspace %q in database %q: %w", workspace, databaseID, err)
	}
	saved, err := service.SaveCheckpointFromLive(ctx, route.Name, checkpointID)
	if err != nil {
		return false, err
	}
	if saved {
		if err := m.refreshWorkspaceCatalogEntry(ctx, route.DatabaseID, route.Name); err != nil {
			return false, err
		}
	}
	return saved, nil
}

func (m *DatabaseManager) ForkWorkspace(ctx context.Context, databaseID, sourceWorkspace, newWorkspace string) error {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, sourceWorkspace)
	if err != nil {
		return err
	}
	if err := service.ForkWorkspace(ctx, route.Name, newWorkspace); err != nil {
		return err
	}
	return m.refreshWorkspaceCatalogEntry(ctx, route.DatabaseID, newWorkspace)
}

func (m *DatabaseManager) ForkResolvedWorkspace(ctx context.Context, sourceWorkspace, newWorkspace string) error {
	service, _, route, err := m.resolveWorkspace(ctx, sourceWorkspace)
	if err != nil {
		return err
	}
	if err := service.ForkWorkspace(ctx, route.Name, newWorkspace); err != nil {
		return err
	}
	return m.refreshWorkspaceCatalogEntry(ctx, route.DatabaseID, newWorkspace)
}

func (m *DatabaseManager) CreateWorkspaceSession(ctx context.Context, databaseID, workspace string, input createWorkspaceSessionRequest) (workspaceSession, error) {
	service, profile, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return workspaceSession{}, err
	}
	session, err := service.CreateWorkspaceSession(ctx, route.Name, input)
	if err != nil {
		return workspaceSession{}, err
	}
	session.DatabaseID = profile.ID
	session.DatabaseName = profile.Name
	session.Redis = RedisConfig{
		RedisAddr:     profile.RedisAddr,
		RedisUsername: profile.RedisUsername,
		RedisPassword: profile.RedisPassword,
		RedisDB:       profile.RedisDB,
		RedisTLS:      profile.RedisTLS,
	}
	return session, nil
}

func (m *DatabaseManager) CreateResolvedWorkspaceSession(ctx context.Context, workspace string, input createWorkspaceSessionRequest) (workspaceSession, error) {
	service, profile, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return workspaceSession{}, err
	}
	session, err := service.CreateWorkspaceSession(ctx, route.Name, input)
	if err != nil {
		return workspaceSession{}, err
	}
	session.DatabaseID = profile.ID
	session.DatabaseName = profile.Name
	session.Redis = RedisConfig{
		RedisAddr:     profile.RedisAddr,
		RedisUsername: profile.RedisUsername,
		RedisPassword: profile.RedisPassword,
		RedisDB:       profile.RedisDB,
		RedisTLS:      profile.RedisTLS,
	}
	return session, nil
}

func (m *DatabaseManager) ListWorkspaceSessions(ctx context.Context, databaseID, workspace string) (workspaceSessionListResponse, error) {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return workspaceSessionListResponse{}, err
	}
	return service.ListWorkspaceSessions(ctx, route.Name)
}

func (m *DatabaseManager) ListResolvedWorkspaceSessions(ctx context.Context, workspace string) (workspaceSessionListResponse, error) {
	service, _, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return workspaceSessionListResponse{}, err
	}
	return service.ListWorkspaceSessions(ctx, route.Name)
}

func (m *DatabaseManager) HeartbeatWorkspaceSession(ctx context.Context, databaseID, workspace, sessionID string) (workspaceSessionInfo, error) {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return workspaceSessionInfo{}, err
	}
	return service.HeartbeatWorkspaceSession(ctx, route.Name, sessionID)
}

func (m *DatabaseManager) HeartbeatResolvedWorkspaceSession(ctx context.Context, workspace, sessionID string) (workspaceSessionInfo, error) {
	service, _, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return workspaceSessionInfo{}, err
	}
	return service.HeartbeatWorkspaceSession(ctx, route.Name, sessionID)
}

func (m *DatabaseManager) CloseWorkspaceSession(ctx context.Context, databaseID, workspace, sessionID string) error {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return err
	}
	return service.CloseWorkspaceSession(ctx, route.Name, sessionID)
}

func (m *DatabaseManager) CloseResolvedWorkspaceSession(ctx context.Context, workspace, sessionID string) error {
	service, _, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return err
	}
	return service.CloseWorkspaceSession(ctx, route.Name, sessionID)
}

func (m *DatabaseManager) GetTree(ctx context.Context, databaseID, workspace, rawView, rawPath string, depth int) (treeResponse, error) {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return treeResponse{}, err
	}
	return service.GetTree(ctx, route.Name, rawView, rawPath, depth)
}

func (m *DatabaseManager) GetResolvedTree(ctx context.Context, workspace, rawView, rawPath string, depth int) (treeResponse, error) {
	service, _, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return treeResponse{}, err
	}
	return service.GetTree(ctx, route.Name, rawView, rawPath, depth)
}

func (m *DatabaseManager) GetFileContent(ctx context.Context, databaseID, workspace, rawView, rawPath string) (fileContentResponse, error) {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return fileContentResponse{}, err
	}
	return service.GetFileContent(ctx, route.Name, rawView, rawPath)
}

func (m *DatabaseManager) GetResolvedFileContent(ctx context.Context, workspace, rawView, rawPath string) (fileContentResponse, error) {
	service, _, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return fileContentResponse{}, err
	}
	return service.GetFileContent(ctx, route.Name, rawView, rawPath)
}

func (m *DatabaseManager) serviceFor(ctx context.Context, databaseID string) (*Service, databaseProfile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.serviceForLocked(ctx, databaseID)
}

func (m *DatabaseManager) resolveWorkspace(ctx context.Context, workspace string) (*Service, databaseProfile, workspaceCatalogRoute, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.resolveWorkspaceServiceLocked(ctx, workspace)
}

func (m *DatabaseManager) serviceForLocked(ctx context.Context, databaseID string) (*Service, databaseProfile, error) {
	profile, exists := m.profiles[databaseID]
	if !exists {
		return nil, databaseProfile{}, os.ErrNotExist
	}
	runtime, exists := m.runtimes[databaseID]
	if !exists {
		var err error
		runtime, err = openDatabaseRuntime(ctx, profile)
		if err != nil {
			return nil, databaseProfile{}, err
		}
		m.runtimes[databaseID] = runtime
	}
	return NewServiceWithCatalog(runtime.cfg, runtime.store, m.catalog, profile.ID, profile.Name), profile, nil
}

func (m *DatabaseManager) resolveWorkspaceServiceLocked(ctx context.Context, workspace string) (*Service, databaseProfile, workspaceCatalogRoute, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return nil, databaseProfile{}, workspaceCatalogRoute{}, fmt.Errorf("workspace id is required")
	}

	if m.catalog != nil {
		routes, err := m.catalog.ResolveWorkspace(ctx, workspace)
		if err != nil {
			return nil, databaseProfile{}, workspaceCatalogRoute{}, err
		}
		switch len(routes) {
		case 1:
			service, profile, err := m.serviceForLocked(ctx, routes[0].DatabaseID)
			return service, profile, routes[0], err
		case 0:
			// Fall back to a scan so out-of-band changes can still be discovered.
		default:
			labels := make([]string, 0, len(routes))
			for _, route := range routes {
				profile := m.profiles[route.DatabaseID]
				label := route.DatabaseID
				if profile.Name != "" && profile.Name != route.DatabaseID {
					label = profile.Name + " (" + route.DatabaseID + ")"
				}
				labels = append(labels, label)
			}
			sort.Strings(labels)
			return nil, databaseProfile{}, workspaceCatalogRoute{}, fmt.Errorf("%w: workspace %q exists in multiple databases: %s", ErrAmbiguousWorkspace, workspace, strings.Join(labels, ", "))
		}
	}

	var (
		matchService *Service
		matchProfile databaseProfile
		matchRoute   workspaceCatalogRoute
		matches      []workspaceCatalogRoute
	)

	for _, id := range m.order {
		service, profile, err := m.serviceForLocked(ctx, id)
		if err != nil {
			return nil, databaseProfile{}, workspaceCatalogRoute{}, err
		}
		exists, err := service.store.WorkspaceExists(ctx, workspace)
		if err != nil {
			return nil, databaseProfile{}, workspaceCatalogRoute{}, err
		}
		if !exists {
			continue
		}
		route := workspaceCatalogRoute{
			DatabaseID:  profile.ID,
			WorkspaceID: workspace,
			Name:        workspace,
		}
		if matchService == nil {
			matchService = service
			matchProfile = profile
			matchRoute = route
		}
		matches = append(matches, route)
	}

	switch len(matches) {
	case 0:
		return nil, databaseProfile{}, workspaceCatalogRoute{}, os.ErrNotExist
	case 1:
		return matchService, matchProfile, matchRoute, nil
	default:
		labels := make([]string, 0, len(matches))
		for _, route := range matches {
			profile := m.profiles[route.DatabaseID]
			label := route.DatabaseID
			if profile.Name != "" && profile.Name != profile.ID {
				label = profile.Name + " (" + profile.ID + ")"
			}
			labels = append(labels, label)
		}
		sort.Strings(labels)
		return nil, databaseProfile{}, workspaceCatalogRoute{}, fmt.Errorf("%w: workspace %q exists in multiple databases: %s", ErrAmbiguousWorkspace, workspace, strings.Join(labels, ", "))
	}
}

func (m *DatabaseManager) listAllWorkspaceSummariesByFanout(ctx context.Context) (workspaceListResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	items := make([]workspaceSummary, 0)
	for _, id := range m.order {
		summaries, _, err := m.liveWorkspaceSummariesLocked(ctx, id)
		if err != nil {
			continue
		}
		items = append(items, summaries...)
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt == items[j].UpdatedAt {
			if items[i].Name == items[j].Name {
				return items[i].DatabaseID < items[j].DatabaseID
			}
			return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
		}
		return items[i].UpdatedAt > items[j].UpdatedAt
	})
	return workspaceListResponse{Items: items}, nil
}

func (m *DatabaseManager) refreshWorkspaceCatalog(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.catalog == nil {
		return nil
	}
	if err := m.catalog.PruneDatabases(ctx, m.order); err != nil {
		return err
	}
	for _, id := range m.order {
		_, _, err := m.liveWorkspaceSummariesLocked(ctx, id)
		if err != nil {
			continue
		}
	}
	return nil
}

func (m *DatabaseManager) liveWorkspaceSummariesLocked(ctx context.Context, databaseID string) ([]workspaceSummary, databaseProfile, error) {
	profile, exists := m.profiles[databaseID]
	if !exists {
		return nil, databaseProfile{}, os.ErrNotExist
	}
	service, _, err := m.serviceForLocked(ctx, databaseID)
	if err != nil {
		if m.catalog != nil {
			_ = m.catalog.RecordWorkspaceRefresh(ctx, profile.ID, profile.Name, time.Time{}, err)
		}
		return nil, databaseProfile{}, err
	}
	response, err := service.ListWorkspaceSummaries(ctx)
	if err != nil {
		if m.catalog != nil {
			_ = m.catalog.RecordWorkspaceRefresh(ctx, profile.ID, profile.Name, time.Time{}, err)
		}
		return nil, databaseProfile{}, err
	}
	for index := range response.Items {
		stampWorkspaceSummary(&response.Items[index], profile)
	}
	if m.catalog != nil {
		synced, err := m.catalog.ReplaceDatabaseWorkspaces(ctx, profile.ID, response.Items)
		if err != nil {
			_ = m.catalog.RecordWorkspaceRefresh(ctx, profile.ID, profile.Name, time.Time{}, err)
			return nil, databaseProfile{}, err
		}
		_ = m.catalog.RecordWorkspaceRefresh(ctx, profile.ID, profile.Name, time.Now().UTC(), nil)
		return synced, profile, nil
	}
	return response.Items, profile, nil
}

func (m *DatabaseManager) reconcileSessionCatalog(ctx context.Context) error {
	m.mu.Lock()
	order := append([]string(nil), m.order...)
	profiles := make(map[string]databaseProfile, len(m.profiles))
	for id, profile := range m.profiles {
		profiles[id] = profile
	}
	catalog := m.catalog
	m.mu.Unlock()

	if catalog == nil {
		return nil
	}
	targets, err := catalog.ListSessionReconcileTargets(ctx)
	if err != nil {
		return err
	}
	workspacesByDatabase := make(map[string]map[string]struct{})
	for _, target := range targets {
		if strings.TrimSpace(target.DatabaseID) == "" || strings.TrimSpace(target.WorkspaceName) == "" {
			continue
		}
		if workspacesByDatabase[target.DatabaseID] == nil {
			workspacesByDatabase[target.DatabaseID] = make(map[string]struct{})
		}
		workspacesByDatabase[target.DatabaseID][target.WorkspaceName] = struct{}{}
	}

	for _, databaseID := range order {
		profile := profiles[databaseID]
		reconcileAt := time.Now().UTC()
		service, _, err := m.serviceFor(ctx, databaseID)
		if err != nil {
			_ = catalog.RecordSessionReconcile(ctx, databaseID, profile.Name, reconcileAt, err)
			continue
		}
		var reconcileErr error
		for workspace := range workspacesByDatabase[databaseID] {
			if _, err := service.ListWorkspaceSessions(ctx, workspace); err != nil {
				reconcileErr = err
				break
			}
		}
		_ = catalog.RecordSessionReconcile(ctx, databaseID, profile.Name, reconcileAt, reconcileErr)
	}
	return nil
}

func (m *DatabaseManager) syncWorkspaceCatalogSummary(ctx context.Context, summary workspaceSummary) (workspaceSummary, error) {
	if m.catalog == nil {
		return summary, nil
	}
	return m.catalog.UpsertWorkspace(ctx, summary)
}

func (m *DatabaseManager) deleteWorkspaceFromCatalog(ctx context.Context, databaseID string, route workspaceCatalogRoute) error {
	if m.catalog == nil {
		return nil
	}
	if strings.TrimSpace(route.WorkspaceID) != "" {
		return m.catalog.DeleteWorkspace(ctx, databaseID, route.WorkspaceID)
	}
	return m.catalog.DeleteWorkspaceByName(ctx, databaseID, route.Name)
}

func (m *DatabaseManager) refreshWorkspaceCatalogEntry(ctx context.Context, databaseID, workspace string) error {
	service, profile, err := m.serviceFor(ctx, databaseID)
	if err != nil {
		return err
	}
	detail, err := service.GetWorkspace(ctx, workspace)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return m.deleteWorkspaceFromCatalog(ctx, databaseID, workspaceCatalogRoute{
				DatabaseID:  databaseID,
				WorkspaceID: workspace,
				Name:        workspace,
			})
		}
		return err
	}
	if err := m.attachWorkspaceDetailIdentity(ctx, &detail, profile); err != nil {
		return err
	}
	_, err = m.syncWorkspaceCatalogSummary(ctx, workspaceSummaryFromDetail(detail))
	return err
}

func (m *DatabaseManager) resolveScopedWorkspace(ctx context.Context, databaseID, workspace string) (*Service, databaseProfile, workspaceCatalogRoute, error) {
	service, profile, err := m.serviceFor(ctx, databaseID)
	if err != nil {
		return nil, databaseProfile{}, workspaceCatalogRoute{}, err
	}

	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return nil, databaseProfile{}, workspaceCatalogRoute{}, fmt.Errorf("workspace id is required")
	}

	if m.catalog != nil {
		route, err := m.catalog.ResolveWorkspaceInDatabase(ctx, profile.ID, workspace)
		if err == nil {
			return service, profile, route, nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, databaseProfile{}, workspaceCatalogRoute{}, err
		}
	}

	exists, err := service.store.WorkspaceExists(ctx, workspace)
	if err != nil {
		return nil, databaseProfile{}, workspaceCatalogRoute{}, err
	}
	if !exists {
		return nil, databaseProfile{}, workspaceCatalogRoute{}, os.ErrNotExist
	}
	return service, profile, workspaceCatalogRoute{
		DatabaseID:  profile.ID,
		WorkspaceID: workspace,
		Name:        workspace,
	}, nil
}

func (m *DatabaseManager) attachWorkspaceSummaryIdentity(ctx context.Context, summary *workspaceSummary, profile databaseProfile) error {
	if summary == nil {
		return nil
	}
	stampWorkspaceSummary(summary, profile)
	synced, err := m.syncWorkspaceCatalogSummary(ctx, *summary)
	if err != nil {
		return err
	}
	*summary = synced
	return nil
}

func (m *DatabaseManager) attachWorkspaceDetailIdentity(ctx context.Context, detail *workspaceDetail, profile databaseProfile) error {
	if detail == nil {
		return nil
	}
	stampWorkspaceDetail(detail, profile)
	synced, err := m.syncWorkspaceCatalogSummary(ctx, workspaceSummaryFromDetail(*detail))
	if err != nil {
		return err
	}
	detail.ID = synced.ID
	return nil
}

func (m *DatabaseManager) buildProfileLocked(id string, input upsertDatabaseRequest) (databaseProfile, bool, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return databaseProfile{}, false, fmt.Errorf("database name is required")
	}

	resolvedID := strings.TrimSpace(id)
	isNew := false
	if resolvedID == "" {
		resolvedID = uniqueDatabaseIDLocked(m.profiles, databaseProfile{
			Name:      name,
			RedisAddr: strings.TrimSpace(input.RedisAddr),
			RedisDB:   input.RedisDB,
		})
		isNew = true
	} else if _, exists := m.profiles[resolvedID]; !exists {
		isNew = true
	}

	password := input.RedisPassword
	if !isNew && strings.TrimSpace(password) == "" {
		if existing, exists := m.profiles[resolvedID]; exists {
			password = existing.RedisPassword
		}
	}

	return databaseProfile{
		ID:            resolvedID,
		Name:          name,
		Description:   strings.TrimSpace(input.Description),
		RedisAddr:     normalizeRedisAddr(input.RedisAddr),
		RedisUsername: strings.TrimSpace(input.RedisUsername),
		RedisPassword: password,
		RedisDB:       input.RedisDB,
		RedisTLS:      input.RedisTLS,
		IsDefault:     !isNew && m.profiles[resolvedID].IsDefault,
	}, isNew, nil
}

func (m *DatabaseManager) saveRegistryLocked() error {
	if m.catalog == nil {
		return errors.New("database registry catalog is unavailable")
	}
	m.ensureDefaultDatabaseLocked()
	profiles := make([]databaseProfile, 0, len(m.order))
	for _, id := range m.order {
		if profile, exists := m.profiles[id]; exists {
			profiles = append(profiles, profile)
		}
	}
	return m.catalog.ReplaceDatabaseProfiles(context.Background(), profiles)
}

func openDatabaseRuntime(ctx context.Context, profile databaseProfile) (*databaseRuntime, error) {
	cfg := profileToConfig(profile)
	store, closeFn, err := OpenStore(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &databaseRuntime{
		cfg:     cfg,
		store:   store,
		closeFn: closeFn,
	}, nil
}

func profileToConfig(profile databaseProfile) Config {
	return Config{
		RedisConfig: RedisConfig{
			RedisAddr:     profile.RedisAddr,
			RedisUsername: profile.RedisUsername,
			RedisPassword: profile.RedisPassword,
			RedisDB:       profile.RedisDB,
			RedisTLS:      profile.RedisTLS,
		},
	}
}

func normalizeRedisAddr(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.TrimPrefix(trimmed, "redis://")
	trimmed = strings.TrimPrefix(trimmed, "rediss://")
	return strings.TrimSpace(trimmed)
}

func validateDatabaseProfile(profile databaseProfile) error {
	if strings.TrimSpace(profile.ID) == "" {
		return fmt.Errorf("database id is required")
	}
	if strings.TrimSpace(profile.Name) == "" {
		return fmt.Errorf("database name is required")
	}
	if !namePattern.MatchString(profile.ID) {
		return fmt.Errorf("invalid database id %q", profile.ID)
	}
	if profile.RedisDB < 0 {
		return fmt.Errorf("redis db must be >= 0")
	}
	if _, _, err := splitAddr(normalizeRedisAddr(profile.RedisAddr)); err != nil {
		return err
	}
	return nil
}

func (m *DatabaseManager) resolveTargetDatabase(ctx context.Context, requestedID string) (databaseProfile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.resolveTargetDatabaseLocked(requestedID)
}

func (m *DatabaseManager) resolveTargetDatabaseLocked(requestedID string) (databaseProfile, error) {
	resolvedID := strings.TrimSpace(requestedID)
	if resolvedID != "" {
		profile, exists := m.profiles[resolvedID]
		if !exists {
			return databaseProfile{}, os.ErrNotExist
		}
		return profile, nil
	}

	switch len(m.order) {
	case 0:
		return databaseProfile{}, fmt.Errorf("no databases are configured")
	case 1:
		return m.profiles[m.order[0]], nil
	}

	for _, id := range m.order {
		if profile, exists := m.profiles[id]; exists && profile.IsDefault {
			return profile, nil
		}
	}

	labels := make([]string, 0, len(m.order))
	for _, id := range m.order {
		if profile, exists := m.profiles[id]; exists {
			labels = append(labels, fmt.Sprintf("%s (%s)", profile.Name, profile.ID))
		}
	}
	sort.Strings(labels)
	return databaseProfile{}, fmt.Errorf("%w: select a database or set a default database first: %s", ErrAmbiguousDatabase, strings.Join(labels, ", "))
}

func (m *DatabaseManager) ensureDefaultDatabaseLocked() {
	if len(m.order) == 0 {
		return
	}

	defaultID := ""
	for _, id := range m.order {
		profile, exists := m.profiles[id]
		if !exists || !profile.IsDefault {
			continue
		}
		if defaultID == "" {
			defaultID = id
			continue
		}
		profile.IsDefault = false
		m.profiles[id] = profile
	}
	if defaultID != "" {
		return
	}

	profile := m.profiles[m.order[0]]
	profile.IsDefault = true
	m.profiles[profile.ID] = profile
}

func (m *DatabaseManager) databaseRecordLocked(ctx context.Context, id string) (databaseRecord, error) {
	profile, exists := m.profiles[id]
	if !exists {
		return databaseRecord{}, os.ErrNotExist
	}

	record := databaseRecord{
		ID:            profile.ID,
		Name:          profile.Name,
		Description:   profile.Description,
		RedisAddr:     profile.RedisAddr,
		RedisUsername: profile.RedisUsername,
		RedisDB:       profile.RedisDB,
		RedisTLS:      profile.RedisTLS,
		IsDefault:     profile.IsDefault,
	}
	if m.catalog != nil {
		healthByDatabase, err := m.catalog.ListDatabaseHealth(ctx)
		if err != nil {
			return databaseRecord{}, err
		}
		activeSessionsByDatabase, err := m.catalog.CountActiveSessionsByDatabase(ctx)
		if err != nil {
			return databaseRecord{}, err
		}
		if health, ok := healthByDatabase[id]; ok {
			record.LastWorkspaceRefreshAt = health.LastWorkspaceRefreshAt
			record.LastWorkspaceRefreshError = health.LastWorkspaceRefreshError
			record.LastSessionReconcileAt = health.LastSessionReconcileAt
			record.LastSessionReconcileError = health.LastSessionReconcileError
		}
		record.ActiveSessionCount = activeSessionsByDatabase[id]
	}
	items, _, err := m.liveWorkspaceSummariesLocked(ctx, id)
	if err != nil {
		record.ConnectionError = err.Error()
		return record, nil
	}
	record.WorkspaceCount = len(items)
	return record, nil
}

func uniqueDatabaseIDLocked(existing map[string]databaseProfile, profile databaseProfile) string {
	base := slugify(profile.Name)
	if base == "" {
		base = slugify(profile.RedisAddr)
	}
	if base == "" {
		base = "database"
	}
	if profile.RedisDB > 0 {
		base = base + "-" + strconv.Itoa(profile.RedisDB)
	}
	candidate := base
	index := 2
	for {
		if _, exists := existing[candidate]; !exists {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d", base, index)
		index++
	}
}

func stampWorkspaceSummary(summary *workspaceSummary, profile databaseProfile) {
	summary.DatabaseID = profile.ID
	summary.DatabaseName = profile.Name
}

func stampWorkspaceDetail(detail *workspaceDetail, profile databaseProfile) {
	detail.DatabaseID = profile.ID
	detail.DatabaseName = profile.Name
}

func withoutValue(values []string, target string) []string {
	next := make([]string, 0, len(values))
	for _, value := range values {
		if value != target {
			next = append(next, value)
		}
	}
	return next
}

func cloneDatabaseProfiles(input map[string]databaseProfile) map[string]databaseProfile {
	cloned := make(map[string]databaseProfile, len(input))
	for id, profile := range input {
		cloned[id] = profile
	}
	return cloned
}
