package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const databaseRegistryVersion = 1

type databaseProfile struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	RedisAddr     string `json:"redis_addr"`
	RedisUsername string `json:"redis_username,omitempty"`
	RedisPassword string `json:"redis_password,omitempty"`
	RedisDB       int    `json:"redis_db"`
	RedisTLS      bool   `json:"redis_tls"`
}

type databaseRecord struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Description     string `json:"description,omitempty"`
	RedisAddr       string `json:"redis_addr"`
	RedisUsername   string `json:"redis_username,omitempty"`
	RedisPassword   string `json:"redis_password,omitempty"`
	RedisDB         int    `json:"redis_db"`
	RedisTLS        bool   `json:"redis_tls"`
	WorkspaceCount  int    `json:"workspace_count"`
	ConnectionError string `json:"connection_error,omitempty"`
}

type databaseListResponse struct {
	Items []databaseRecord `json:"items"`
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

type databaseRegistryFile struct {
	Version   int               `json:"version"`
	Databases []databaseProfile `json:"databases"`
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
	mu           sync.Mutex
	registryPath string
	catalog      *workspaceCatalog
	profiles     map[string]databaseProfile
	order        []string
	runtimes     map[string]*databaseRuntime
}

func OpenDatabaseManager(configPathOverride string) (*DatabaseManager, error) {
	seedCfg, seedPresent, err := LoadConfigWithPresence(configPathOverride)
	if err != nil {
		return nil, err
	}

	registryPath := databaseRegistryPath(configPathOverride)
	loadedProfiles, err := loadDatabaseProfiles(registryPath)
	if err != nil {
		return nil, err
	}
	if len(loadedProfiles) == 0 && seedPresent {
		loadedProfiles = []databaseProfile{seedDatabaseProfile(seedCfg)}
	}

	catalog, err := openWorkspaceCatalog(configPathOverride)
	if err != nil {
		return nil, err
	}

	manager := &DatabaseManager{
		registryPath: registryPath,
		catalog:      catalog,
		profiles:     make(map[string]databaseProfile, len(loadedProfiles)),
		order:        make([]string, 0, len(loadedProfiles)),
		runtimes:     make(map[string]*databaseRuntime),
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

	items := make([]databaseRecord, 0, len(m.order))
	for _, id := range m.order {
		profile := m.profiles[id]
		record := databaseRecord{
			ID:            profile.ID,
			Name:          profile.Name,
			Description:   profile.Description,
			RedisAddr:     profile.RedisAddr,
			RedisUsername: profile.RedisUsername,
			RedisPassword: profile.RedisPassword,
			RedisDB:       profile.RedisDB,
			RedisTLS:      profile.RedisTLS,
		}

		service, _, err := m.serviceForLocked(ctx, id)
		if err != nil {
			record.ConnectionError = err.Error()
			items = append(items, record)
			continue
		}

		workspaces, err := service.ListWorkspaceSummaries(ctx)
		if err != nil {
			record.ConnectionError = err.Error()
		} else {
			record.WorkspaceCount = len(workspaces.Items)
		}
		items = append(items, record)
	}

	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	return databaseListResponse{Items: items}, nil
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

	oldProfile, hadOldProfile := m.profiles[profile.ID]
	oldRuntime := m.runtimes[profile.ID]

	m.profiles[profile.ID] = profile
	if isNew {
		m.order = append(m.order, profile.ID)
	}
	m.runtimes[profile.ID] = runtime

	if err := m.saveRegistryLocked(); err != nil {
		runtime.closeFn()
		if hadOldProfile {
			m.profiles[profile.ID] = oldProfile
		} else {
			delete(m.profiles, profile.ID)
		}
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

	service := NewService(runtime.cfg, runtime.store)
	workspaces, err := service.ListWorkspaceSummaries(ctx)
	record := databaseRecord{
		ID:            profile.ID,
		Name:          profile.Name,
		Description:   profile.Description,
		RedisAddr:     profile.RedisAddr,
		RedisUsername: profile.RedisUsername,
		RedisPassword: profile.RedisPassword,
		RedisDB:       profile.RedisDB,
		RedisTLS:      profile.RedisTLS,
	}
	if err != nil {
		record.ConnectionError = err.Error()
	} else {
		record.WorkspaceCount = len(workspaces.Items)
		if m.catalog != nil {
			for index := range workspaces.Items {
				stampWorkspaceSummary(&workspaces.Items[index], profile)
			}
			if catalogErr := m.catalog.ReplaceDatabaseWorkspaces(ctx, profile.ID, workspaces.Items); catalogErr != nil {
				return databaseRecord{}, catalogErr
			}
		}
	}
	if err != nil && m.catalog != nil {
		if catalogErr := m.catalog.DeleteDatabaseWorkspaces(ctx, profile.ID); catalogErr != nil {
			return databaseRecord{}, catalogErr
		}
	}
	return record, nil
}

func (m *DatabaseManager) DeleteDatabase(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	profile, exists := m.profiles[id]
	if !exists {
		return os.ErrNotExist
	}
	oldRuntime := m.runtimes[id]
	oldOrder := append([]string(nil), m.order...)

	delete(m.profiles, id)
	delete(m.runtimes, id)
	m.order = withoutValue(m.order, id)

	if err := m.saveRegistryLocked(); err != nil {
		m.profiles[id] = profile
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
	service, profile, err := m.serviceFor(ctx, databaseID)
	if err != nil {
		return workspaceListResponse{}, err
	}
	response, err := service.ListWorkspaceSummaries(ctx)
	if err != nil {
		return workspaceListResponse{}, err
	}
	for index := range response.Items {
		stampWorkspaceSummary(&response.Items[index], profile)
	}
	return response, nil
}

func (m *DatabaseManager) ListAllWorkspaceSummaries(ctx context.Context) (workspaceListResponse, error) {
	if m.catalog != nil {
		items, err := m.catalog.ListWorkspaces(ctx)
		if err != nil {
			return workspaceListResponse{}, err
		}
		return workspaceListResponse{Items: items}, nil
	}
	return m.listAllWorkspaceSummariesByFanout(ctx)
}

func (m *DatabaseManager) GetWorkspace(ctx context.Context, databaseID, workspace string) (workspaceDetail, error) {
	service, profile, err := m.serviceFor(ctx, databaseID)
	if err != nil {
		return workspaceDetail{}, err
	}
	detail, err := service.GetWorkspace(ctx, workspace)
	if err != nil {
		return workspaceDetail{}, err
	}
	stampWorkspaceDetail(&detail, profile)
	return detail, nil
}

func (m *DatabaseManager) GetResolvedWorkspace(ctx context.Context, workspace string) (workspaceDetail, error) {
	service, profile, err := m.resolveWorkspaceService(ctx, workspace)
	if err != nil {
		return workspaceDetail{}, err
	}
	detail, err := service.GetWorkspace(ctx, workspace)
	if err != nil {
		return workspaceDetail{}, err
	}
	stampWorkspaceDetail(&detail, profile)
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
	stampWorkspaceDetail(&detail, profile)
	if err := m.syncWorkspaceCatalogSummary(ctx, workspaceSummaryFromDetail(detail)); err != nil {
		return workspaceDetail{}, err
	}
	return detail, nil
}

func (m *DatabaseManager) UpdateWorkspace(ctx context.Context, databaseID, workspace string, input updateWorkspaceRequest) (workspaceDetail, error) {
	service, profile, err := m.serviceFor(ctx, databaseID)
	if err != nil {
		return workspaceDetail{}, err
	}
	if strings.TrimSpace(input.DatabaseName) == "" {
		input.DatabaseName = profile.Name
	}
	if strings.TrimSpace(input.CloudAccount) == "" {
		input.CloudAccount = "Direct Redis"
	}
	detail, err := service.UpdateWorkspace(ctx, workspace, input)
	if err != nil {
		return workspaceDetail{}, err
	}
	stampWorkspaceDetail(&detail, profile)
	if err := m.syncWorkspaceCatalogSummary(ctx, workspaceSummaryFromDetail(detail)); err != nil {
		return workspaceDetail{}, err
	}
	return detail, nil
}

func (m *DatabaseManager) UpdateResolvedWorkspace(ctx context.Context, workspace string, input updateWorkspaceRequest) (workspaceDetail, error) {
	service, profile, err := m.resolveWorkspaceService(ctx, workspace)
	if err != nil {
		return workspaceDetail{}, err
	}
	if strings.TrimSpace(input.DatabaseName) == "" {
		input.DatabaseName = profile.Name
	}
	if strings.TrimSpace(input.CloudAccount) == "" {
		input.CloudAccount = "Direct Redis"
	}
	detail, err := service.UpdateWorkspace(ctx, workspace, input)
	if err != nil {
		return workspaceDetail{}, err
	}
	stampWorkspaceDetail(&detail, profile)
	if err := m.syncWorkspaceCatalogSummary(ctx, workspaceSummaryFromDetail(detail)); err != nil {
		return workspaceDetail{}, err
	}
	return detail, nil
}

func (m *DatabaseManager) DeleteWorkspace(ctx context.Context, databaseID, workspace string) error {
	service, _, err := m.serviceFor(ctx, databaseID)
	if err != nil {
		return err
	}
	if err := service.DeleteWorkspace(ctx, workspace); err != nil {
		return err
	}
	return m.deleteWorkspaceFromCatalog(ctx, databaseID, workspace)
}

func (m *DatabaseManager) DeleteResolvedWorkspace(ctx context.Context, workspace string) error {
	service, profile, err := m.resolveWorkspaceService(ctx, workspace)
	if err != nil {
		return err
	}
	if err := service.DeleteWorkspace(ctx, workspace); err != nil {
		return err
	}
	return m.deleteWorkspaceFromCatalog(ctx, profile.ID, workspace)
}

func (m *DatabaseManager) ListGlobalActivity(ctx context.Context, databaseID string, limit int) (activityListResponse, error) {
	service, _, err := m.serviceFor(ctx, databaseID)
	if err != nil {
		return activityListResponse{}, err
	}
	return service.ListGlobalActivity(ctx, limit)
}

func (m *DatabaseManager) ListWorkspaceActivity(ctx context.Context, databaseID, workspace string, limit int) (activityListResponse, error) {
	service, _, err := m.serviceFor(ctx, databaseID)
	if err != nil {
		return activityListResponse{}, err
	}
	return service.ListWorkspaceActivity(ctx, workspace, limit)
}

func (m *DatabaseManager) ListResolvedWorkspaceActivity(ctx context.Context, workspace string, limit int) (activityListResponse, error) {
	service, _, err := m.resolveWorkspaceService(ctx, workspace)
	if err != nil {
		return activityListResponse{}, err
	}
	return service.ListWorkspaceActivity(ctx, workspace, limit)
}

func (m *DatabaseManager) RestoreCheckpoint(ctx context.Context, databaseID, workspace, checkpointID string) error {
	service, _, err := m.serviceFor(ctx, databaseID)
	if err != nil {
		return err
	}
	if err := service.RestoreCheckpoint(ctx, workspace, checkpointID); err != nil {
		return err
	}
	return m.refreshWorkspaceCatalogEntry(ctx, databaseID, workspace)
}

func (m *DatabaseManager) RestoreResolvedCheckpoint(ctx context.Context, workspace, checkpointID string) error {
	service, profile, err := m.resolveWorkspaceService(ctx, workspace)
	if err != nil {
		return err
	}
	if err := service.RestoreCheckpoint(ctx, workspace, checkpointID); err != nil {
		return err
	}
	return m.refreshWorkspaceCatalogEntry(ctx, profile.ID, workspace)
}

func (m *DatabaseManager) ListCheckpoints(ctx context.Context, databaseID, workspace string, limit int) ([]checkpointSummary, error) {
	service, _, err := m.serviceFor(ctx, databaseID)
	if err != nil {
		return nil, err
	}
	return service.ListCheckpoints(ctx, workspace, limit)
}

func (m *DatabaseManager) ListResolvedCheckpoints(ctx context.Context, workspace string, limit int) ([]checkpointSummary, error) {
	service, _, err := m.resolveWorkspaceService(ctx, workspace)
	if err != nil {
		return nil, err
	}
	return service.ListCheckpoints(ctx, workspace, limit)
}

func (m *DatabaseManager) SaveCheckpoint(ctx context.Context, databaseID, workspace string, input SaveCheckpointRequest) (bool, error) {
	service, _, err := m.serviceFor(ctx, databaseID)
	if err != nil {
		return false, err
	}
	input.Workspace = workspace
	saved, err := service.SaveCheckpoint(ctx, input)
	if err != nil {
		return false, err
	}
	if saved {
		if err := m.refreshWorkspaceCatalogEntry(ctx, databaseID, workspace); err != nil {
			return false, err
		}
	}
	return saved, nil
}

func (m *DatabaseManager) SaveResolvedCheckpoint(ctx context.Context, workspace string, input SaveCheckpointRequest) (bool, error) {
	service, profile, err := m.resolveWorkspaceService(ctx, workspace)
	if err != nil {
		return false, err
	}
	input.Workspace = workspace
	saved, err := service.SaveCheckpoint(ctx, input)
	if err != nil {
		return false, err
	}
	if saved {
		if err := m.refreshWorkspaceCatalogEntry(ctx, profile.ID, workspace); err != nil {
			return false, err
		}
	}
	return saved, nil
}

func (m *DatabaseManager) ForkWorkspace(ctx context.Context, databaseID, sourceWorkspace, newWorkspace string) error {
	service, _, err := m.serviceFor(ctx, databaseID)
	if err != nil {
		return err
	}
	if err := service.ForkWorkspace(ctx, sourceWorkspace, newWorkspace); err != nil {
		return err
	}
	return m.refreshWorkspaceCatalogEntry(ctx, databaseID, newWorkspace)
}

func (m *DatabaseManager) ForkResolvedWorkspace(ctx context.Context, sourceWorkspace, newWorkspace string) error {
	service, profile, err := m.resolveWorkspaceService(ctx, sourceWorkspace)
	if err != nil {
		return err
	}
	if err := service.ForkWorkspace(ctx, sourceWorkspace, newWorkspace); err != nil {
		return err
	}
	return m.refreshWorkspaceCatalogEntry(ctx, profile.ID, newWorkspace)
}

func (m *DatabaseManager) CreateWorkspaceSession(ctx context.Context, databaseID, workspace string, input createWorkspaceSessionRequest) (workspaceSession, error) {
	service, profile, err := m.serviceFor(ctx, databaseID)
	if err != nil {
		return workspaceSession{}, err
	}
	session, err := service.CreateWorkspaceSession(ctx, workspace, input)
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
	service, profile, err := m.resolveWorkspaceService(ctx, workspace)
	if err != nil {
		return workspaceSession{}, err
	}
	session, err := service.CreateWorkspaceSession(ctx, workspace, input)
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
	service, _, err := m.serviceFor(ctx, databaseID)
	if err != nil {
		return workspaceSessionListResponse{}, err
	}
	return service.ListWorkspaceSessions(ctx, workspace)
}

func (m *DatabaseManager) ListResolvedWorkspaceSessions(ctx context.Context, workspace string) (workspaceSessionListResponse, error) {
	service, _, err := m.resolveWorkspaceService(ctx, workspace)
	if err != nil {
		return workspaceSessionListResponse{}, err
	}
	return service.ListWorkspaceSessions(ctx, workspace)
}

func (m *DatabaseManager) HeartbeatWorkspaceSession(ctx context.Context, databaseID, workspace, sessionID string) (workspaceSessionInfo, error) {
	service, _, err := m.serviceFor(ctx, databaseID)
	if err != nil {
		return workspaceSessionInfo{}, err
	}
	return service.HeartbeatWorkspaceSession(ctx, workspace, sessionID)
}

func (m *DatabaseManager) HeartbeatResolvedWorkspaceSession(ctx context.Context, workspace, sessionID string) (workspaceSessionInfo, error) {
	service, _, err := m.resolveWorkspaceService(ctx, workspace)
	if err != nil {
		return workspaceSessionInfo{}, err
	}
	return service.HeartbeatWorkspaceSession(ctx, workspace, sessionID)
}

func (m *DatabaseManager) CloseWorkspaceSession(ctx context.Context, databaseID, workspace, sessionID string) error {
	service, _, err := m.serviceFor(ctx, databaseID)
	if err != nil {
		return err
	}
	return service.CloseWorkspaceSession(ctx, workspace, sessionID)
}

func (m *DatabaseManager) CloseResolvedWorkspaceSession(ctx context.Context, workspace, sessionID string) error {
	service, _, err := m.resolveWorkspaceService(ctx, workspace)
	if err != nil {
		return err
	}
	return service.CloseWorkspaceSession(ctx, workspace, sessionID)
}

func (m *DatabaseManager) GetTree(ctx context.Context, databaseID, workspace, rawView, rawPath string, depth int) (treeResponse, error) {
	service, _, err := m.serviceFor(ctx, databaseID)
	if err != nil {
		return treeResponse{}, err
	}
	return service.GetTree(ctx, workspace, rawView, rawPath, depth)
}

func (m *DatabaseManager) GetResolvedTree(ctx context.Context, workspace, rawView, rawPath string, depth int) (treeResponse, error) {
	service, _, err := m.resolveWorkspaceService(ctx, workspace)
	if err != nil {
		return treeResponse{}, err
	}
	return service.GetTree(ctx, workspace, rawView, rawPath, depth)
}

func (m *DatabaseManager) GetFileContent(ctx context.Context, databaseID, workspace, rawView, rawPath string) (fileContentResponse, error) {
	service, _, err := m.serviceFor(ctx, databaseID)
	if err != nil {
		return fileContentResponse{}, err
	}
	return service.GetFileContent(ctx, workspace, rawView, rawPath)
}

func (m *DatabaseManager) GetResolvedFileContent(ctx context.Context, workspace, rawView, rawPath string) (fileContentResponse, error) {
	service, _, err := m.resolveWorkspaceService(ctx, workspace)
	if err != nil {
		return fileContentResponse{}, err
	}
	return service.GetFileContent(ctx, workspace, rawView, rawPath)
}

func (m *DatabaseManager) serviceFor(ctx context.Context, databaseID string) (*Service, databaseProfile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.serviceForLocked(ctx, databaseID)
}

func (m *DatabaseManager) resolveWorkspaceService(ctx context.Context, workspace string) (*Service, databaseProfile, error) {
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
	return NewService(runtime.cfg, runtime.store), profile, nil
}

func (m *DatabaseManager) resolveWorkspaceServiceLocked(ctx context.Context, workspace string) (*Service, databaseProfile, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return nil, databaseProfile{}, fmt.Errorf("workspace id is required")
	}

	if m.catalog != nil {
		routes, err := m.catalog.ResolveWorkspace(ctx, workspace)
		if err != nil {
			return nil, databaseProfile{}, err
		}
		switch len(routes) {
		case 1:
			return m.serviceForLocked(ctx, routes[0].DatabaseID)
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
			return nil, databaseProfile{}, fmt.Errorf("%w: workspace %q exists in multiple databases: %s", ErrAmbiguousWorkspace, workspace, strings.Join(labels, ", "))
		}
	}

	var (
		matchService *Service
		matchProfile databaseProfile
		matches      []databaseProfile
	)

	for _, id := range m.order {
		service, profile, err := m.serviceForLocked(ctx, id)
		if err != nil {
			return nil, databaseProfile{}, err
		}
		exists, err := service.store.WorkspaceExists(ctx, workspace)
		if err != nil {
			return nil, databaseProfile{}, err
		}
		if !exists {
			continue
		}
		if matchService == nil {
			matchService = service
			matchProfile = profile
		}
		matches = append(matches, profile)
	}

	switch len(matches) {
	case 0:
		return nil, databaseProfile{}, os.ErrNotExist
	case 1:
		return matchService, matchProfile, nil
	default:
		labels := make([]string, 0, len(matches))
		for _, profile := range matches {
			label := profile.ID
			if profile.Name != "" && profile.Name != profile.ID {
				label = profile.Name + " (" + profile.ID + ")"
			}
			labels = append(labels, label)
		}
		sort.Strings(labels)
		return nil, databaseProfile{}, fmt.Errorf("%w: workspace %q exists in multiple databases: %s", ErrAmbiguousWorkspace, workspace, strings.Join(labels, ", "))
	}
}

func (m *DatabaseManager) listAllWorkspaceSummariesByFanout(ctx context.Context) (workspaceListResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	items := make([]workspaceSummary, 0)
	for _, id := range m.order {
		service, profile, err := m.serviceForLocked(ctx, id)
		if err != nil {
			return workspaceListResponse{}, err
		}
		response, err := service.ListWorkspaceSummaries(ctx)
		if err != nil {
			return workspaceListResponse{}, err
		}
		for index := range response.Items {
			stampWorkspaceSummary(&response.Items[index], profile)
			items = append(items, response.Items[index])
		}
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
		service, profile, err := m.serviceForLocked(ctx, id)
		if err != nil {
			continue
		}
		response, err := service.ListWorkspaceSummaries(ctx)
		if err != nil {
			continue
		}
		for index := range response.Items {
			stampWorkspaceSummary(&response.Items[index], profile)
		}
		if err := m.catalog.ReplaceDatabaseWorkspaces(ctx, id, response.Items); err != nil {
			return err
		}
	}
	return nil
}

func (m *DatabaseManager) syncWorkspaceCatalogSummary(ctx context.Context, summary workspaceSummary) error {
	if m.catalog == nil {
		return nil
	}
	return m.catalog.UpsertWorkspace(ctx, summary)
}

func (m *DatabaseManager) deleteWorkspaceFromCatalog(ctx context.Context, databaseID, workspace string) error {
	if m.catalog == nil {
		return nil
	}
	return m.catalog.DeleteWorkspace(ctx, databaseID, workspace)
}

func (m *DatabaseManager) refreshWorkspaceCatalogEntry(ctx context.Context, databaseID, workspace string) error {
	service, profile, err := m.serviceFor(ctx, databaseID)
	if err != nil {
		return err
	}
	detail, err := service.GetWorkspace(ctx, workspace)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return m.deleteWorkspaceFromCatalog(ctx, databaseID, workspace)
		}
		return err
	}
	stampWorkspaceDetail(&detail, profile)
	return m.syncWorkspaceCatalogSummary(ctx, workspaceSummaryFromDetail(detail))
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

	return databaseProfile{
		ID:            resolvedID,
		Name:          name,
		Description:   strings.TrimSpace(input.Description),
		RedisAddr:     normalizeRedisAddr(input.RedisAddr),
		RedisUsername: strings.TrimSpace(input.RedisUsername),
		RedisPassword: input.RedisPassword,
		RedisDB:       input.RedisDB,
		RedisTLS:      input.RedisTLS,
	}, isNew, nil
}

func (m *DatabaseManager) saveRegistryLocked() error {
	payload := databaseRegistryFile{
		Version:   databaseRegistryVersion,
		Databases: make([]databaseProfile, 0, len(m.order)),
	}
	for _, id := range m.order {
		if profile, exists := m.profiles[id]; exists {
			payload.Databases = append(payload.Databases, profile)
		}
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.registryPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(m.registryPath, append(data, '\n'), 0o600)
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

func loadDatabaseProfiles(registryPath string) ([]databaseProfile, error) {
	data, err := os.ReadFile(registryPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var payload databaseRegistryFile
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	return payload.Databases, nil
}

func databaseRegistryPath(configPathOverride string) string {
	cfgPath := configPath(configPathOverride)
	return filepath.Join(filepath.Dir(cfgPath), "afs.databases.json")
}

func seedDatabaseProfile(cfg Config) databaseProfile {
	id, name := activeDatabaseIdentity(cfg)
	return databaseProfile{
		ID:            id,
		Name:          name,
		RedisAddr:     strings.TrimSpace(cfg.RedisAddr),
		RedisUsername: strings.TrimSpace(cfg.RedisUsername),
		RedisPassword: cfg.RedisPassword,
		RedisDB:       cfg.RedisDB,
		RedisTLS:      cfg.RedisTLS,
	}
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
