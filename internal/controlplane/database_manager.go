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
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	OwnerSubject   string `json:"owner_subject,omitempty"`
	OwnerLabel     string `json:"owner_label,omitempty"`
	ManagementType string `json:"management_type"`
	Purpose        string `json:"purpose,omitempty"`
	RedisAddr      string `json:"redis_addr"`
	RedisUsername  string `json:"redis_username,omitempty"`
	RedisPassword  string `json:"redis_password,omitempty"`
	RedisDB        int    `json:"redis_db"`
	RedisTLS       bool   `json:"redis_tls"`
	IsDefault      bool   `json:"is_default"`
}

type databaseRecord struct {
	ID                        string `json:"id"`
	Name                      string `json:"name"`
	Description               string `json:"description,omitempty"`
	OwnerSubject              string `json:"owner_subject,omitempty"`
	OwnerLabel                string `json:"owner_label,omitempty"`
	ManagementType            string `json:"management_type"`
	Purpose                   string `json:"purpose,omitempty"`
	CanEdit                   bool   `json:"can_edit"`
	CanDelete                 bool   `json:"can_delete"`
	CanCreateWorkspaces       bool   `json:"can_create_workspaces"`
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

	// AFS-specific aggregates (summed across all workspaces in this database)
	AFSTotalBytes int64 `json:"afs_total_bytes"`
	AFSFileCount  int   `json:"afs_file_count"`

	// Redis server stats (populated by the background poller). Pointer so the
	// JSON omits the whole block when we have never sampled (e.g. for an
	// unreachable DB on startup).
	Stats *RedisStats `json:"stats,omitempty"`
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

const (
	databaseManagementSystemManaged = "system-managed"
	databaseManagementUserManaged   = "user-managed"
	databasePurposeGeneral          = "general"
	databasePurposeOnboarding       = "onboarding"

	// freeTierWorkspaceLimit is the maximum number of workspaces a single user
	// may own on the shared onboarding database (free tier).
	freeTierWorkspaceLimit = 3
)

// ErrFreeTierLimitReached is returned when a user attempts to create a
// workspace on the shared onboarding database after reaching their free
// tier quota.
var ErrFreeTierLimitReached = errors.New("free tier workspace limit reached")

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
	catalog  catalogStore
	profiles map[string]databaseProfile
	order    []string
	runtimes map[string]*databaseRuntime

	// Stats cache populated by the background poller. Keyed by database ID.
	// Protected by statsMu so callers hitting ListDatabases don't contend
	// with the poller goroutine on mu.
	statsMu sync.RWMutex
	stats   map[string]RedisStats

	// Poller lifecycle
	pollerStop   chan struct{}
	pollerDoneWG sync.WaitGroup
}

// redisStatsPollInterval is how often the background poller re-samples each
// database's INFO snapshot. Cheap for small clusters; tunable via env if we
// ever need it.
const redisStatsPollInterval = 30 * time.Second

func OpenDatabaseManager(configPathOverride string) (*DatabaseManager, error) {
	catalog, err := openCatalogStore(configPathOverride)
	if err != nil {
		return nil, err
	}

	loadedProfiles, err := catalog.ListDatabaseProfiles(context.Background())
	if err != nil {
		_ = catalog.Close()
		return nil, err
	}

	manager := &DatabaseManager{
		catalog:    catalog,
		profiles:   make(map[string]databaseProfile, len(loadedProfiles)),
		order:      make([]string, 0, len(loadedProfiles)),
		runtimes:   make(map[string]*databaseRuntime),
		stats:      make(map[string]RedisStats),
		pollerStop: make(chan struct{}),
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

	// Kick off the background stats poller. Fire-and-forget; it self-throttles
	// and silently skips unavailable databases.
	manager.pollerDoneWG.Add(1)
	go manager.runStatsPoller()

	return manager, nil
}

// runStatsPoller periodically samples RedisStats for each known database and
// caches the result. Ticks on redisStatsPollInterval until Close stops it.
func (m *DatabaseManager) runStatsPoller() {
	defer m.pollerDoneWG.Done()

	// Prime the cache on startup so the first ListDatabases call already has
	// data (instead of making the user wait for the first tick).
	m.sampleAllDatabaseStats(context.Background())

	ticker := time.NewTicker(redisStatsPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.pollerStop:
			return
		case <-ticker.C:
			m.sampleAllDatabaseStats(context.Background())
		}
	}
}

// sampleAllDatabaseStats iterates the known profiles and refreshes each
// database's cached RedisStats. Errors are non-fatal: the prior sample stays
// in place so the UI doesn't flicker on transient failures.
func (m *DatabaseManager) sampleAllDatabaseStats(parent context.Context) {
	m.mu.Lock()
	ids := make([]string, len(m.order))
	copy(ids, m.order)
	m.mu.Unlock()

	for _, id := range ids {
		select {
		case <-m.pollerStop:
			return
		default:
		}

		ctx, cancel := context.WithTimeout(parent, 5*time.Second)
		stats, err := m.collectStatsFor(ctx, id)
		cancel()
		if err != nil {
			continue
		}

		m.statsMu.Lock()
		m.stats[id] = stats
		m.statsMu.Unlock()
	}
}

// collectStatsFor opens (or reuses) a runtime for the given database ID and
// issues one INFO+DBSIZE round-trip. The runtime is left in m.runtimes so a
// subsequent service request does not pay the connection cost again.
func (m *DatabaseManager) collectStatsFor(ctx context.Context, databaseID string) (RedisStats, error) {
	m.mu.Lock()
	profile, ok := m.profiles[databaseID]
	if !ok {
		m.mu.Unlock()
		return RedisStats{}, fmt.Errorf("unknown database %q", databaseID)
	}
	runtime, ok := m.runtimes[databaseID]
	if !ok {
		m.mu.Unlock()
		opened, err := openDatabaseRuntime(ctx, profile)
		if err != nil {
			return RedisStats{}, err
		}
		m.mu.Lock()
		// Someone may have raced us; keep the first winner.
		if existing, alreadyThere := m.runtimes[databaseID]; alreadyThere {
			opened.closeFn()
			runtime = existing
		} else {
			m.runtimes[databaseID] = opened
			runtime = opened
		}
	}
	m.mu.Unlock()

	return runtime.store.CollectRedisStats(ctx)
}

// statsFor returns the last cached RedisStats for a database, if any.
func (m *DatabaseManager) statsFor(databaseID string) (RedisStats, bool) {
	m.statsMu.RLock()
	defer m.statsMu.RUnlock()
	s, ok := m.stats[databaseID]
	return s, ok
}

func (m *DatabaseManager) Close() {
	// Signal the stats poller to stop and wait for it to exit so it doesn't
	// race with runtime teardown below.
	if m.pollerStop != nil {
		select {
		case <-m.pollerStop:
			// already closed
		default:
			close(m.pollerStop)
		}
		m.pollerDoneWG.Wait()
	}

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
	if err := m.ensureBootstrapDatabase(ctx); err != nil {
		return databaseListResponse{}, err
	}
	subject := authSubjectFromContext(ctx)

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
	defaultID := m.effectiveDefaultDatabaseIDLocked(subject)
	for _, id := range m.order {
		profile := m.profiles[id]
		if !databaseProfileVisibleToSubject(profile, subject) {
			continue
		}
		record := databaseRecord{
			ID:                  profile.ID,
			Name:                profile.Name,
			Description:         profile.Description,
			OwnerSubject:        profile.OwnerSubject,
			OwnerLabel:          profile.OwnerLabel,
			ManagementType:      normalizedDatabaseManagementType(profile.ManagementType),
			Purpose:             normalizedDatabasePurpose(profile.Purpose),
			CanEdit:             databaseProfileCanEdit(profile),
			CanDelete:           databaseProfileCanDelete(profile),
			CanCreateWorkspaces: databaseProfileCanCreateWorkspaces(profile),
			RedisAddr:           profile.RedisAddr,
			RedisUsername:       profile.RedisUsername,
			RedisDB:             profile.RedisDB,
			RedisTLS:            profile.RedisTLS,
			IsDefault:           id == defaultID,
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
			// Even without live workspace data we can still surface the last
			// cached stats sample if we have one.
			if stats, ok := m.statsFor(id); ok {
				s := stats
				record.Stats = &s
			}
			items = append(items, record)
			continue
		}
		record.WorkspaceCount = len(workspaces)

		// Aggregate AFS-specific footprint across this database's workspaces.
		var totalBytes int64
		var totalFiles int
		for _, ws := range workspaces {
			totalBytes += ws.TotalBytes
			totalFiles += ws.FileCount
		}
		record.AFSTotalBytes = totalBytes
		record.AFSFileCount = totalFiles

		if stats, ok := m.statsFor(id); ok {
			s := stats
			record.Stats = &s
		}
		items = append(items, record)
	}

	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	return databaseListResponse{Items: items}, nil
}

func (m *DatabaseManager) ensureBootstrapDatabase(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ensureBootstrapDatabaseLocked(ctx)
}

func (m *DatabaseManager) ensureBootstrapDatabaseLocked(ctx context.Context) error {
	profile, ok := bootstrapDatabaseProfileFromContext(ctx)
	if !ok {
		return nil
	}
	if err := validateDatabaseProfile(profile); err != nil {
		return err
	}

	changed := false
	if updated := m.normalizeBootstrapProfilesLocked(profile); updated {
		changed = true
	}
	if existing, exists := m.profiles[profile.ID]; exists {
		if existing != profile {
			m.profiles[profile.ID] = profile
			changed = true
		}
		if changed {
			m.ensureDefaultDatabaseLocked()
			return m.saveRegistryLocked()
		}
		return nil
	}

	m.profiles[profile.ID] = profile
	m.order = append(m.order, profile.ID)
	m.ensureDefaultDatabaseLocked()

	if err := m.saveRegistryLocked(); err != nil {
		delete(m.profiles, profile.ID)
		m.order = withoutValue(m.order, profile.ID)
		return err
	}
	return nil
}

func (m *DatabaseManager) normalizeBootstrapProfilesLocked(canonical databaseProfile) bool {
	changed := false
	for _, id := range append([]string(nil), m.order...) {
		if id == canonical.ID {
			continue
		}
		profile, exists := m.profiles[id]
		if !exists {
			continue
		}
		if !shouldPruneLegacyBootstrapProfile(profile, canonical) {
			continue
		}
		if runtime := m.runtimes[id]; runtime != nil {
			runtime.closeFn()
			delete(m.runtimes, id)
		}
		delete(m.profiles, id)
		m.order = withoutValue(m.order, id)
		changed = true
	}
	return changed
}

func shouldPruneLegacyBootstrapProfile(profile, canonical databaseProfile) bool {
	if normalizedDatabaseManagementType(profile.ManagementType) == databaseManagementSystemManaged &&
		normalizedDatabasePurpose(profile.Purpose) == databasePurposeOnboarding {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(profile.Name), strings.TrimSpace(canonical.Name)) {
		return true
	}
	return false
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

	profile, isNew, err := m.buildProfileLocked(ctx, id, input)
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
		ID:                  profile.ID,
		Name:                profile.Name,
		Description:         profile.Description,
		OwnerSubject:        profile.OwnerSubject,
		OwnerLabel:          profile.OwnerLabel,
		ManagementType:      normalizedDatabaseManagementType(profile.ManagementType),
		Purpose:             normalizedDatabasePurpose(profile.Purpose),
		CanEdit:             databaseProfileCanEdit(profile),
		CanDelete:           databaseProfileCanDelete(profile),
		CanCreateWorkspaces: databaseProfileCanCreateWorkspaces(profile),
		RedisAddr:           profile.RedisAddr,
		RedisUsername:       profile.RedisUsername,
		RedisDB:             profile.RedisDB,
		RedisTLS:            profile.RedisTLS,
		IsDefault:           profile.IsDefault,
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
	return m.DeleteDatabaseWithContext(context.Background(), id)
}

func (m *DatabaseManager) DeleteDatabaseWithContext(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	profile, exists := m.profiles[id]
	if !exists {
		return os.ErrNotExist
	}
	if !databaseProfileVisibleToSubject(profile, authSubjectFromContext(ctx)) {
		return os.ErrNotExist
	}
	if !databaseProfileCanDelete(profile) {
		return fmt.Errorf("database %q is managed by AFS Cloud and cannot be deleted", profile.Name)
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

	subject := authSubjectFromContext(ctx)
	profile, exists := m.profiles[strings.TrimSpace(id)]
	if !exists {
		return databaseRecord{}, os.ErrNotExist
	}
	if !databaseProfileVisibleToSubject(profile, subject) {
		return databaseRecord{}, os.ErrNotExist
	}
	oldProfiles := cloneDatabaseProfiles(m.profiles)
	for databaseID, candidate := range m.profiles {
		if !databaseProfileVisibleToSubject(candidate, subject) {
			continue
		}
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
	detail, err := service.GetWorkspace(ctx, route.WorkspaceID)
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

func (m *DatabaseManager) GetWorkspaceVersioningPolicy(ctx context.Context, databaseID, workspace string) (WorkspaceVersioningPolicy, error) {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return WorkspaceVersioningPolicy{}, err
	}
	return service.GetWorkspaceVersioningPolicy(ctx, route.WorkspaceID)
}

func (m *DatabaseManager) GetResolvedWorkspace(ctx context.Context, workspace string) (workspaceDetail, error) {
	service, profile, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return workspaceDetail{}, err
	}
	detail, err := service.GetWorkspace(ctx, route.WorkspaceID)
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

func (m *DatabaseManager) GetResolvedWorkspaceVersioningPolicy(ctx context.Context, workspace string) (WorkspaceVersioningPolicy, error) {
	service, _, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return WorkspaceVersioningPolicy{}, err
	}
	return service.GetWorkspaceVersioningPolicy(ctx, route.WorkspaceID)
}

func (m *DatabaseManager) CreateWorkspace(ctx context.Context, databaseID string, input createWorkspaceRequest) (workspaceDetail, error) {
	service, profile, err := m.serviceFor(ctx, databaseID)
	if err != nil {
		return workspaceDetail{}, err
	}
	if err := m.enforceFreeTierQuotaLocked(ctx, profile); err != nil {
		return workspaceDetail{}, err
	}
	input.DatabaseID = profile.ID
	input.DatabaseName = profile.Name
	if strings.TrimSpace(input.CloudAccount) == "" {
		input.CloudAccount = quickstartCloudAccount(profile)
	}
	detail, err := service.CreateWorkspace(ctx, input)
	if err != nil {
		return workspaceDetail{}, err
	}
	// On shared databases, attach per-user ownership before the catalog sync
	// so the new workspace counts toward the requesting user's free-tier
	// quota (and is visible to them, not other users).
	if strings.TrimSpace(profile.OwnerSubject) == "" {
		if subject := authSubjectFromContext(ctx); subject != "" {
			detail.OwnerSubject = subject
			detail.OwnerLabel = authIdentityLabelFromContext(ctx)
		}
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

// enforceFreeTierQuotaLocked checks whether the requesting user is allowed to
// create another workspace on the given database. Shared onboarding databases
// cap each user at freeTierWorkspaceLimit workspaces. Owned databases have no
// quota.
func (m *DatabaseManager) enforceFreeTierQuotaLocked(ctx context.Context, profile databaseProfile) error {
	if normalizedDatabasePurpose(profile.Purpose) != databasePurposeOnboarding {
		return nil
	}
	if m.catalog == nil {
		return nil
	}
	subject := authSubjectFromContext(ctx)
	if subject == "" {
		// Without an authenticated user we can't attribute the quota; let the
		// request through (legacy/local-dev behaviour).
		return nil
	}
	count, err := m.catalog.CountWorkspacesForOwner(ctx, profile.ID, subject)
	if err != nil {
		return err
	}
	if count >= freeTierWorkspaceLimit {
		return ErrFreeTierLimitReached
	}
	return nil
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
		input.CloudAccount = quickstartCloudAccount(profile)
	}
	detail, err := service.UpdateWorkspace(ctx, route.WorkspaceID, input)
	if err != nil {
		return workspaceDetail{}, err
	}
	if err := m.attachWorkspaceDetailIdentity(ctx, &detail, profile); err != nil {
		return workspaceDetail{}, err
	}
	return detail, nil
}

func (m *DatabaseManager) UpdateWorkspaceVersioningPolicy(ctx context.Context, databaseID, workspace string, policy WorkspaceVersioningPolicy) (WorkspaceVersioningPolicy, error) {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return WorkspaceVersioningPolicy{}, err
	}
	return service.UpdateWorkspaceVersioningPolicy(ctx, route.WorkspaceID, policy)
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
		input.CloudAccount = quickstartCloudAccount(profile)
	}
	detail, err := service.UpdateWorkspace(ctx, route.WorkspaceID, input)
	if err != nil {
		return workspaceDetail{}, err
	}
	if err := m.attachWorkspaceDetailIdentity(ctx, &detail, profile); err != nil {
		return workspaceDetail{}, err
	}
	return detail, nil
}

func (m *DatabaseManager) UpdateResolvedWorkspaceVersioningPolicy(ctx context.Context, workspace string, policy WorkspaceVersioningPolicy) (WorkspaceVersioningPolicy, error) {
	service, _, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return WorkspaceVersioningPolicy{}, err
	}
	return service.UpdateWorkspaceVersioningPolicy(ctx, route.WorkspaceID, policy)
}

func (m *DatabaseManager) DeleteWorkspace(ctx context.Context, databaseID, workspace string) error {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return err
	}
	if err := service.DeleteWorkspace(ctx, route.WorkspaceID); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return m.deleteWorkspaceFromCatalog(ctx, route.DatabaseID, route)
		}
		return err
	}
	return m.deleteWorkspaceFromCatalog(ctx, route.DatabaseID, route)
}

func (m *DatabaseManager) DeleteResolvedWorkspace(ctx context.Context, workspace string) error {
	service, _, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return err
	}
	if err := service.DeleteWorkspace(ctx, route.WorkspaceID); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return m.deleteWorkspaceFromCatalog(ctx, route.DatabaseID, route)
		}
		return err
	}
	return m.deleteWorkspaceFromCatalog(ctx, route.DatabaseID, route)
}

func (m *DatabaseManager) ListGlobalActivity(ctx context.Context, databaseID string, req activityListRequest) (activityListResponse, error) {
	service, _, err := m.serviceFor(ctx, databaseID)
	if err != nil {
		return activityListResponse{}, err
	}
	response, err := service.ListGlobalActivityPage(ctx, req)
	if err != nil {
		return activityListResponse{}, err
	}
	m.attachDatabaseToActivity(&response, databaseID)
	return response, nil
}

func (m *DatabaseManager) ListWorkspaceActivity(ctx context.Context, databaseID, workspace string, req activityListRequest) (activityListResponse, error) {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return activityListResponse{}, err
	}
	response, err := service.ListWorkspaceActivityPage(ctx, route.WorkspaceID, req)
	if err != nil {
		return activityListResponse{}, err
	}
	m.attachDatabaseToActivity(&response, route.DatabaseID)
	return response, nil
}

func (m *DatabaseManager) ListResolvedWorkspaceActivity(ctx context.Context, workspace string, req activityListRequest) (activityListResponse, error) {
	service, _, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return activityListResponse{}, err
	}
	response, err := service.ListWorkspaceActivityPage(ctx, route.WorkspaceID, req)
	if err != nil {
		return activityListResponse{}, err
	}
	m.attachDatabaseToActivity(&response, route.DatabaseID)
	return response, nil
}

func (m *DatabaseManager) ListGlobalEvents(ctx context.Context, databaseID string, req EventListRequest) (EventListResponse, error) {
	service, _, err := m.serviceFor(ctx, databaseID)
	if err != nil {
		return EventListResponse{}, err
	}
	response, err := service.ListGlobalEvents(ctx, req)
	if err != nil {
		return EventListResponse{}, err
	}
	m.attachDatabaseToEvents(&response, databaseID)
	return response, nil
}

func (m *DatabaseManager) ListWorkspaceEvents(ctx context.Context, databaseID, workspace string, req EventListRequest) (EventListResponse, error) {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return EventListResponse{}, err
	}
	response, err := service.ListWorkspaceEvents(ctx, route.WorkspaceID, req)
	if err != nil {
		return EventListResponse{}, err
	}
	m.attachDatabaseToEvents(&response, route.DatabaseID)
	return response, nil
}

func (m *DatabaseManager) ListResolvedWorkspaceEvents(ctx context.Context, workspace string, req EventListRequest) (EventListResponse, error) {
	service, _, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return EventListResponse{}, err
	}
	response, err := service.ListWorkspaceEvents(ctx, route.WorkspaceID, req)
	if err != nil {
		return EventListResponse{}, err
	}
	m.attachDatabaseToEvents(&response, route.DatabaseID)
	return response, nil
}

func (m *DatabaseManager) ListGlobalChangelog(ctx context.Context, databaseID string, req ChangelogListRequest) (ChangelogListResponse, error) {
	service, _, err := m.serviceFor(ctx, databaseID)
	if err != nil {
		return ChangelogListResponse{}, err
	}
	response, err := service.ListGlobalChangelog(ctx, req)
	if err != nil {
		return ChangelogListResponse{}, err
	}
	m.attachDatabaseToChangelog(&response, databaseID)
	return response, nil
}

func (m *DatabaseManager) RestoreCheckpoint(ctx context.Context, databaseID, workspace, checkpointID string) error {
	_, err := m.RestoreCheckpointWithResult(ctx, databaseID, workspace, checkpointID)
	return err
}

func (m *DatabaseManager) RestoreCheckpointWithResult(ctx context.Context, databaseID, workspace, checkpointID string) (RestoreCheckpointResult, error) {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return RestoreCheckpointResult{}, err
	}
	result, err := service.RestoreCheckpointWithResult(ctx, route.WorkspaceID, checkpointID)
	if err != nil {
		return RestoreCheckpointResult{}, err
	}
	if err := m.refreshWorkspaceCatalogEntry(ctx, route.DatabaseID, route.WorkspaceID); err != nil {
		return RestoreCheckpointResult{}, err
	}
	return result, nil
}

func (m *DatabaseManager) RestoreResolvedCheckpoint(ctx context.Context, workspace, checkpointID string) error {
	_, err := m.RestoreResolvedCheckpointWithResult(ctx, workspace, checkpointID)
	return err
}

func (m *DatabaseManager) RestoreResolvedCheckpointWithResult(ctx context.Context, workspace, checkpointID string) (RestoreCheckpointResult, error) {
	service, _, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return RestoreCheckpointResult{}, err
	}
	result, err := service.RestoreCheckpointWithResult(ctx, route.WorkspaceID, checkpointID)
	if err != nil {
		return RestoreCheckpointResult{}, err
	}
	if err := m.refreshWorkspaceCatalogEntry(ctx, route.DatabaseID, route.WorkspaceID); err != nil {
		return RestoreCheckpointResult{}, err
	}
	return result, nil
}

func (m *DatabaseManager) ListCheckpoints(ctx context.Context, databaseID, workspace string, limit int) ([]checkpointSummary, error) {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return nil, err
	}
	return service.ListCheckpoints(ctx, route.WorkspaceID, limit)
}

func (m *DatabaseManager) ListResolvedCheckpoints(ctx context.Context, workspace string, limit int) ([]checkpointSummary, error) {
	service, _, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return nil, err
	}
	return service.ListCheckpoints(ctx, route.WorkspaceID, limit)
}

func (m *DatabaseManager) GetCheckpoint(ctx context.Context, databaseID, workspace, checkpointID string) (checkpointDetail, error) {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return checkpointDetail{}, err
	}
	return service.GetCheckpoint(ctx, route.WorkspaceID, checkpointID)
}

func (m *DatabaseManager) GetResolvedCheckpoint(ctx context.Context, workspace, checkpointID string) (checkpointDetail, error) {
	service, _, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return checkpointDetail{}, err
	}
	return service.GetCheckpoint(ctx, route.WorkspaceID, checkpointID)
}

func (m *DatabaseManager) DiffWorkspace(ctx context.Context, databaseID, workspace, baseView, headView string) (WorkspaceDiffResponse, error) {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return WorkspaceDiffResponse{}, err
	}
	return service.DiffWorkspace(ctx, route.WorkspaceID, baseView, headView)
}

func (m *DatabaseManager) DiffResolvedWorkspace(ctx context.Context, workspace, baseView, headView string) (WorkspaceDiffResponse, error) {
	service, _, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return WorkspaceDiffResponse{}, err
	}
	return service.DiffWorkspace(ctx, route.WorkspaceID, baseView, headView)
}

func (m *DatabaseManager) SaveCheckpoint(ctx context.Context, databaseID, workspace string, input SaveCheckpointRequest) (bool, error) {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return false, err
	}
	input.Workspace = route.WorkspaceID
	saved, err := service.SaveCheckpoint(ctx, input)
	if err != nil {
		return false, err
	}
	if saved {
		if err := m.refreshWorkspaceCatalogEntry(ctx, route.DatabaseID, route.WorkspaceID); err != nil {
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
	input.Workspace = route.WorkspaceID
	saved, err := service.SaveCheckpoint(ctx, input)
	if err != nil {
		return false, err
	}
	if saved {
		if err := m.refreshWorkspaceCatalogEntry(ctx, route.DatabaseID, route.WorkspaceID); err != nil {
			return false, err
		}
	}
	return saved, nil
}

func (m *DatabaseManager) SaveResolvedCheckpointFromLive(ctx context.Context, workspace, checkpointID string) (bool, error) {
	return m.SaveResolvedCheckpointFromLiveWithOptions(ctx, workspace, checkpointID, SaveCheckpointFromLiveOptions{})
}

func (m *DatabaseManager) SaveResolvedCheckpointFromLiveWithOptions(ctx context.Context, workspace, checkpointID string, options SaveCheckpointFromLiveOptions) (bool, error) {
	service, _, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return false, fmt.Errorf("resolve workspace %q: %w", workspace, err)
	}
	saved, err := service.SaveCheckpointFromLiveWithOptions(ctx, route.WorkspaceID, checkpointID, options)
	if err != nil {
		return false, err
	}
	if saved {
		if err := m.refreshWorkspaceCatalogEntry(ctx, route.DatabaseID, route.WorkspaceID); err != nil {
			return false, err
		}
	}
	return saved, nil
}

func (m *DatabaseManager) SaveCheckpointFromLive(ctx context.Context, databaseID, workspace, checkpointID string) (bool, error) {
	return m.SaveCheckpointFromLiveWithOptions(ctx, databaseID, workspace, checkpointID, SaveCheckpointFromLiveOptions{})
}

func (m *DatabaseManager) SaveCheckpointFromLiveWithOptions(ctx context.Context, databaseID, workspace, checkpointID string, options SaveCheckpointFromLiveOptions) (bool, error) {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return false, fmt.Errorf("resolve workspace %q in database %q: %w", workspace, databaseID, err)
	}
	saved, err := service.SaveCheckpointFromLiveWithOptions(ctx, route.WorkspaceID, checkpointID, options)
	if err != nil {
		return false, err
	}
	if saved {
		if err := m.refreshWorkspaceCatalogEntry(ctx, route.DatabaseID, route.WorkspaceID); err != nil {
			return false, err
		}
	}
	return saved, nil
}

// ListChangelog reads per-session file-change entries for a workspace.
func (m *DatabaseManager) ListChangelog(ctx context.Context, databaseID, workspace string, req ChangelogListRequest) (ChangelogListResponse, error) {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return ChangelogListResponse{}, err
	}
	return service.ListChangelog(ctx, route.WorkspaceID, req)
}

// GetSessionChangelogSummary returns the per-session rollup (op counts, delta bytes).
func (m *DatabaseManager) GetSessionChangelogSummary(ctx context.Context, databaseID, workspace, sessionID string) (SessionChangelogSummary, error) {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return SessionChangelogSummary{}, err
	}
	return service.GetSessionChangelogSummary(ctx, route.WorkspaceID, sessionID)
}

// GetPathLastWriter returns the last-writer metadata for a single path.
func (m *DatabaseManager) GetPathLastWriter(ctx context.Context, databaseID, workspace, path string) (PathLastWriter, error) {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return PathLastWriter{}, err
	}
	return service.GetPathLastWriter(ctx, route.WorkspaceID, path)
}

func (m *DatabaseManager) ForkWorkspace(ctx context.Context, databaseID, sourceWorkspace, newWorkspace string) error {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, sourceWorkspace)
	if err != nil {
		return err
	}
	if err := service.ForkWorkspace(ctx, route.WorkspaceID, newWorkspace); err != nil {
		return err
	}
	return m.refreshWorkspaceCatalogEntry(ctx, route.DatabaseID, newWorkspace)
}

func (m *DatabaseManager) ForkResolvedWorkspace(ctx context.Context, sourceWorkspace, newWorkspace string) error {
	service, _, route, err := m.resolveWorkspace(ctx, sourceWorkspace)
	if err != nil {
		return err
	}
	if err := service.ForkWorkspace(ctx, route.WorkspaceID, newWorkspace); err != nil {
		return err
	}
	return m.refreshWorkspaceCatalogEntry(ctx, route.DatabaseID, newWorkspace)
}

func (m *DatabaseManager) CreateWorkspaceSession(ctx context.Context, databaseID, workspace string, input createWorkspaceSessionRequest) (workspaceSession, error) {
	service, profile, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return workspaceSession{}, err
	}
	session, err := service.CreateWorkspaceSession(ctx, route.WorkspaceID, input)
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

func (m *DatabaseManager) UpsertWorkspaceSession(ctx context.Context, databaseID, workspace, sessionID string, input createWorkspaceSessionRequest) (workspaceSessionInfo, error) {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return workspaceSessionInfo{}, err
	}
	return service.UpsertWorkspaceSession(ctx, route.WorkspaceID, sessionID, input)
}

func (m *DatabaseManager) CreateResolvedWorkspaceSession(ctx context.Context, workspace string, input createWorkspaceSessionRequest) (workspaceSession, error) {
	service, profile, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return workspaceSession{}, err
	}
	session, err := service.CreateWorkspaceSession(ctx, route.WorkspaceID, input)
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
	return service.ListWorkspaceSessions(ctx, route.WorkspaceID)
}

func (m *DatabaseManager) ListResolvedWorkspaceSessions(ctx context.Context, workspace string) (workspaceSessionListResponse, error) {
	service, _, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return workspaceSessionListResponse{}, err
	}
	return service.ListWorkspaceSessions(ctx, route.WorkspaceID)
}

func (m *DatabaseManager) ListAgentSessions(ctx context.Context, databaseID string) (workspaceSessionListResponse, error) {
	if err := m.reconcileListedAgentSessions(ctx, databaseID); err != nil {
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
	subject := authSubjectFromContext(ctx)

	records, err := catalog.ListSessions(ctx, databaseID)
	if err != nil {
		return workspaceSessionListResponse{}, err
	}

	now := time.Now().UTC()
	items := make([]workspaceSessionInfo, 0, len(records))
	workspaceOwnersByDatabase := make(map[string]map[string]catalogOwnerInfo)
	for _, record := range records {
		if sessionCatalogRecordLeaseExpired(record, now) {
			_ = catalog.UpsertSession(ctx, staleSessionCatalogRecord(record, now, "expired"))
			continue
		}
		profile, ok := profiles[record.DatabaseID]
		if !ok {
			continue
		}
		if !databaseProfileVisibleToSubject(profile, subject) {
			continue
		}
		if subject != "" && strings.TrimSpace(profile.OwnerSubject) == "" {
			owners, ok := workspaceOwnersByDatabase[record.DatabaseID]
			if !ok {
				owners, err = catalog.ListWorkspaceOwners(ctx, record.DatabaseID)
				if err != nil {
					return workspaceSessionListResponse{}, err
				}
				workspaceOwnersByDatabase[record.DatabaseID] = owners
			}
			owner, ok := owners[record.WorkspaceID]
			if !ok || strings.TrimSpace(owner.Subject) != subject {
				continue
			}
		}
		databaseName := record.DatabaseID
		if strings.TrimSpace(profile.Name) != "" {
			databaseName = profile.Name
		}
		items = append(items, workspaceSessionInfo{
			SessionID:       record.SessionID,
			Workspace:       defaultString(record.WorkspaceName, record.WorkspaceID),
			WorkspaceID:     record.WorkspaceID,
			WorkspaceName:   record.WorkspaceName,
			DatabaseID:      record.DatabaseID,
			DatabaseName:    databaseName,
			AgentID:         record.AgentID,
			ClientKind:      record.ClientKind,
			AFSVersion:      record.AFSVersion,
			Hostname:        record.Hostname,
			OperatingSystem: record.OperatingSystem,
			LocalPath:       record.LocalPath,
			Readonly:        record.Readonly,
			State:           record.State,
			StartedAt:       record.StartedAt,
			LastSeenAt:      record.LastSeenAt,
			LeaseExpiresAt:  record.LeaseExpiresAt,
		})
	}

	return workspaceSessionListResponse{Items: items}, nil
}

func (m *DatabaseManager) reconcileListedAgentSessions(ctx context.Context, databaseID string) error {
	m.mu.Lock()
	catalog := m.catalog
	profiles := make(map[string]databaseProfile, len(m.profiles))
	for id, profile := range m.profiles {
		profiles[id] = profile
	}
	m.mu.Unlock()

	if catalog == nil {
		return nil
	}

	targetDatabaseID := strings.TrimSpace(databaseID)
	subject := authSubjectFromContext(ctx)
	targets, err := catalog.ListSessionReconcileTargets(ctx)
	if err != nil {
		return err
	}

	workspacesByDatabase := make(map[string][]string)
	for _, target := range targets {
		resolvedDatabaseID := strings.TrimSpace(target.DatabaseID)
		workspace := strings.TrimSpace(target.WorkspaceName)
		if resolvedDatabaseID == "" || workspace == "" {
			continue
		}
		if targetDatabaseID != "" && resolvedDatabaseID != targetDatabaseID {
			continue
		}
		profile, ok := profiles[resolvedDatabaseID]
		if !ok || !databaseProfileVisibleToSubject(profile, subject) {
			continue
		}
		workspacesByDatabase[resolvedDatabaseID] = append(workspacesByDatabase[resolvedDatabaseID], workspace)
	}

	databaseIDs := make([]string, 0, len(workspacesByDatabase))
	for id := range workspacesByDatabase {
		databaseIDs = append(databaseIDs, id)
	}
	sort.Strings(databaseIDs)

	for _, resolvedDatabaseID := range databaseIDs {
		profile := profiles[resolvedDatabaseID]
		reconcileAt := time.Now().UTC()
		service, _, err := m.serviceFor(ctx, resolvedDatabaseID)
		if err != nil {
			_ = catalog.RecordSessionReconcile(ctx, resolvedDatabaseID, profile.Name, reconcileAt, err)
			continue
		}

		workspaces := workspacesByDatabase[resolvedDatabaseID]
		sort.Strings(workspaces)

		var reconcileErr error
		for _, workspace := range workspaces {
			if _, err := service.ListWorkspaceSessions(ctx, workspace); err != nil {
				reconcileErr = err
				break
			}
		}
		_ = catalog.RecordSessionReconcile(ctx, resolvedDatabaseID, profile.Name, reconcileAt, reconcileErr)
	}

	return nil
}

func (m *DatabaseManager) ListAllActivity(ctx context.Context, req activityListRequest) (activityListResponse, error) {
	m.mu.Lock()
	order := append([]string(nil), m.order...)
	profiles := make(map[string]databaseProfile, len(m.profiles))
	for id, profile := range m.profiles {
		profiles[id] = profile
	}
	m.mu.Unlock()
	subject := authSubjectFromContext(ctx)

	limit := req.Limit
	if limit <= 0 {
		limit = 50
		req.Limit = limit
	}
	items := make([]activityEvent, 0, limit)
	for _, databaseID := range order {
		if profile, ok := profiles[databaseID]; ok && !databaseProfileVisibleToSubject(profile, subject) {
			continue
		}
		service, _, err := m.serviceFor(ctx, databaseID)
		if err != nil {
			continue
		}
		response, err := service.ListGlobalActivityPage(ctx, req)
		if err != nil {
			continue
		}
		for _, item := range response.Items {
			item.DatabaseID = databaseID
			item.DatabaseName = profiles[databaseID].Name
			items = append(items, item)
		}
	}

	sort.Slice(items, func(i, j int) bool { return compareActivityEvents(items[i], items[j]) > 0 })
	if len(items) > limit {
		items = items[:limit]
	}

	response := activityListResponse{Items: items}
	if len(items) > 0 {
		response.NextCursor = items[len(items)-1].ID
	}
	return response, nil
}

func (m *DatabaseManager) ListAllEvents(ctx context.Context, req EventListRequest) (EventListResponse, error) {
	m.mu.Lock()
	order := append([]string(nil), m.order...)
	profiles := make(map[string]databaseProfile, len(m.profiles))
	for id, profile := range m.profiles {
		profiles[id] = profile
	}
	m.mu.Unlock()
	subject := authSubjectFromContext(ctx)

	req = normalizeEventListRequest(req)
	items := make([]eventEntry, 0, req.Limit)
	for _, databaseID := range order {
		profile, ok := profiles[databaseID]
		if ok && !databaseProfileVisibleToSubject(profile, subject) {
			continue
		}
		service, _, err := m.serviceFor(ctx, databaseID)
		if err != nil {
			continue
		}
		response, err := service.ListGlobalEvents(ctx, req)
		if err != nil {
			continue
		}
		for _, item := range response.Items {
			item.DatabaseID = databaseID
			item.DatabaseName = profile.Name
			items = append(items, item)
		}
	}

	sort.Slice(items, func(i, j int) bool {
		comparison := compareRedisStreamIDs(items[i].ID, items[j].ID)
		if req.Reverse {
			return comparison > 0
		}
		return comparison < 0
	})
	if len(items) > req.Limit {
		items = items[:req.Limit]
	}

	response := EventListResponse{Items: items}
	if len(items) > 0 {
		response.NextCursor = items[len(items)-1].ID
	}
	return response, nil
}

func (m *DatabaseManager) ListAllChangelog(ctx context.Context, req ChangelogListRequest) (ChangelogListResponse, error) {
	m.mu.Lock()
	order := append([]string(nil), m.order...)
	profiles := make(map[string]databaseProfile, len(m.profiles))
	for id, profile := range m.profiles {
		profiles[id] = profile
	}
	m.mu.Unlock()
	subject := authSubjectFromContext(ctx)

	limit := req.Limit
	if limit <= 0 {
		limit = 100
		req.Limit = limit
	}
	if limit > 1000 {
		limit = 1000
		req.Limit = limit
	}
	entries := make([]ChangelogEntryRow, 0, limit)
	for _, databaseID := range order {
		profile, ok := profiles[databaseID]
		if ok && !databaseProfileVisibleToSubject(profile, subject) {
			continue
		}
		service, _, err := m.serviceFor(ctx, databaseID)
		if err != nil {
			continue
		}
		response, err := service.ListGlobalChangelog(ctx, req)
		if err != nil {
			continue
		}
		for _, entry := range response.Entries {
			entry.DatabaseID = databaseID
			entry.DatabaseName = profile.Name
			entries = append(entries, entry)
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		comparison := compareChangelogRows(entries[i], entries[j])
		if req.Reverse {
			return comparison > 0
		}
		return comparison < 0
	})
	if len(entries) > limit {
		entries = entries[:limit]
	}
	response := ChangelogListResponse{Entries: entries}
	if len(entries) > 0 {
		response.NextCursor = entries[len(entries)-1].ID
	}
	return response, nil
}

func (m *DatabaseManager) attachDatabaseToActivity(response *activityListResponse, databaseID string) {
	m.mu.Lock()
	profile, ok := m.profiles[databaseID]
	m.mu.Unlock()
	if !ok {
		return
	}
	for i := range response.Items {
		response.Items[i].DatabaseID = databaseID
		response.Items[i].DatabaseName = profile.Name
	}
}

func (m *DatabaseManager) attachDatabaseToChangelog(response *ChangelogListResponse, databaseID string) {
	m.mu.Lock()
	profile, ok := m.profiles[databaseID]
	m.mu.Unlock()
	if !ok {
		return
	}
	for i := range response.Entries {
		response.Entries[i].DatabaseID = databaseID
		response.Entries[i].DatabaseName = profile.Name
	}
}

func (m *DatabaseManager) attachDatabaseToEvents(response *EventListResponse, databaseID string) {
	m.mu.Lock()
	profile, ok := m.profiles[databaseID]
	m.mu.Unlock()
	if !ok {
		return
	}
	for i := range response.Items {
		response.Items[i].DatabaseID = databaseID
		response.Items[i].DatabaseName = profile.Name
	}
}

func (m *DatabaseManager) HeartbeatWorkspaceSession(ctx context.Context, databaseID, workspace, sessionID string) (workspaceSessionInfo, error) {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return workspaceSessionInfo{}, err
	}
	return service.HeartbeatWorkspaceSession(ctx, route.WorkspaceID, sessionID)
}

func (m *DatabaseManager) HeartbeatResolvedWorkspaceSession(ctx context.Context, workspace, sessionID string) (workspaceSessionInfo, error) {
	service, _, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return workspaceSessionInfo{}, err
	}
	return service.HeartbeatWorkspaceSession(ctx, route.WorkspaceID, sessionID)
}

func (m *DatabaseManager) CloseWorkspaceSession(ctx context.Context, databaseID, workspace, sessionID string) error {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return err
	}
	return service.CloseWorkspaceSession(ctx, route.WorkspaceID, sessionID)
}

func (m *DatabaseManager) CloseResolvedWorkspaceSession(ctx context.Context, workspace, sessionID string) error {
	service, _, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return err
	}
	return service.CloseWorkspaceSession(ctx, route.WorkspaceID, sessionID)
}

func (m *DatabaseManager) GetTree(ctx context.Context, databaseID, workspace, rawView, rawPath string, depth int) (treeResponse, error) {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return treeResponse{}, err
	}
	return service.GetTree(ctx, route.WorkspaceID, rawView, rawPath, depth)
}

func (m *DatabaseManager) GetResolvedTree(ctx context.Context, workspace, rawView, rawPath string, depth int) (treeResponse, error) {
	service, _, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return treeResponse{}, err
	}
	return service.GetTree(ctx, route.WorkspaceID, rawView, rawPath, depth)
}

func (m *DatabaseManager) GetFileHistory(ctx context.Context, databaseID, workspace, rawPath string, newestFirst bool) (FileHistoryResponse, error) {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return FileHistoryResponse{}, err
	}
	return service.GetFileHistory(ctx, route.WorkspaceID, rawPath, newestFirst)
}

func (m *DatabaseManager) GetFileHistoryPage(ctx context.Context, databaseID, workspace string, req FileHistoryRequest) (FileHistoryResponse, error) {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return FileHistoryResponse{}, err
	}
	return service.GetFileHistoryPage(ctx, route.WorkspaceID, req)
}

func (m *DatabaseManager) GetResolvedFileHistory(ctx context.Context, workspace, rawPath string, newestFirst bool) (FileHistoryResponse, error) {
	service, _, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return FileHistoryResponse{}, err
	}
	return service.GetFileHistory(ctx, route.WorkspaceID, rawPath, newestFirst)
}

func (m *DatabaseManager) GetResolvedFileHistoryPage(ctx context.Context, workspace string, req FileHistoryRequest) (FileHistoryResponse, error) {
	service, _, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return FileHistoryResponse{}, err
	}
	return service.GetFileHistoryPage(ctx, route.WorkspaceID, req)
}

func (m *DatabaseManager) GetFileContent(ctx context.Context, databaseID, workspace, rawView, rawPath string) (fileContentResponse, error) {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return fileContentResponse{}, err
	}
	return service.GetFileContent(ctx, route.WorkspaceID, rawView, rawPath)
}

func (m *DatabaseManager) GetFileVersionContent(ctx context.Context, databaseID, workspace, versionID string) (FileVersionContentResponse, error) {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return FileVersionContentResponse{}, err
	}
	return service.GetFileVersionContent(ctx, route.WorkspaceID, versionID)
}

func (m *DatabaseManager) GetFileVersionContentAtOrdinal(ctx context.Context, databaseID, workspace, fileID string, ordinal int64) (FileVersionContentResponse, error) {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return FileVersionContentResponse{}, err
	}
	return service.GetFileVersionContentAtOrdinal(ctx, route.WorkspaceID, fileID, ordinal)
}

func (m *DatabaseManager) GetResolvedFileContent(ctx context.Context, workspace, rawView, rawPath string) (fileContentResponse, error) {
	service, _, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return fileContentResponse{}, err
	}
	return service.GetFileContent(ctx, route.WorkspaceID, rawView, rawPath)
}

func (m *DatabaseManager) GetResolvedFileVersionContent(ctx context.Context, workspace, versionID string) (FileVersionContentResponse, error) {
	service, _, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return FileVersionContentResponse{}, err
	}
	return service.GetFileVersionContent(ctx, route.WorkspaceID, versionID)
}

func (m *DatabaseManager) GetResolvedFileVersionContentAtOrdinal(ctx context.Context, workspace, fileID string, ordinal int64) (FileVersionContentResponse, error) {
	service, _, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return FileVersionContentResponse{}, err
	}
	return service.GetFileVersionContentAtOrdinal(ctx, route.WorkspaceID, fileID, ordinal)
}

func (m *DatabaseManager) DiffFileVersions(ctx context.Context, databaseID, workspace, rawPath string, from, to FileVersionDiffOperand) (FileVersionDiffResponse, error) {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return FileVersionDiffResponse{}, err
	}
	return service.DiffFileVersions(ctx, route.WorkspaceID, rawPath, from, to)
}

func (m *DatabaseManager) DiffResolvedFileVersions(ctx context.Context, workspace, rawPath string, from, to FileVersionDiffOperand) (FileVersionDiffResponse, error) {
	service, _, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return FileVersionDiffResponse{}, err
	}
	return service.DiffFileVersions(ctx, route.WorkspaceID, rawPath, from, to)
}

func (m *DatabaseManager) RestoreFileVersion(ctx context.Context, databaseID, workspace, rawPath string, selector FileVersionSelector) (FileVersionRestoreResponse, error) {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return FileVersionRestoreResponse{}, err
	}
	return service.RestoreFileVersion(ctx, route.WorkspaceID, rawPath, selector)
}

func (m *DatabaseManager) RestoreResolvedFileVersion(ctx context.Context, workspace, rawPath string, selector FileVersionSelector) (FileVersionRestoreResponse, error) {
	service, _, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return FileVersionRestoreResponse{}, err
	}
	return service.RestoreFileVersion(ctx, route.WorkspaceID, rawPath, selector)
}

func (m *DatabaseManager) UndeleteFileVersion(ctx context.Context, databaseID, workspace, rawPath string, selector FileVersionSelector) (FileVersionUndeleteResponse, error) {
	service, _, route, err := m.resolveScopedWorkspace(ctx, databaseID, workspace)
	if err != nil {
		return FileVersionUndeleteResponse{}, err
	}
	return service.UndeleteFileVersion(ctx, route.WorkspaceID, rawPath, selector)
}

func (m *DatabaseManager) UndeleteResolvedFileVersion(ctx context.Context, workspace, rawPath string, selector FileVersionSelector) (FileVersionUndeleteResponse, error) {
	service, _, route, err := m.resolveWorkspace(ctx, workspace)
	if err != nil {
		return FileVersionUndeleteResponse{}, err
	}
	return service.UndeleteFileVersion(ctx, route.WorkspaceID, rawPath, selector)
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
	if !databaseProfileVisibleToSubject(profile, authSubjectFromContext(ctx)) {
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

// serviceFor is the mutex-aware counterpart of serviceForLocked: it holds
// m.mu only for the map reads and, on a runtime cache miss, releases the
// mutex during the Redis dial so a slow/unreachable backend does not block
// unrelated callers. Two callers racing for the same first-dial resolve
// cleanly — the loser closes its extra runtime.
func (m *DatabaseManager) serviceFor(ctx context.Context, databaseID string) (*Service, databaseProfile, error) {
	m.mu.Lock()
	profile, exists := m.profiles[databaseID]
	if !exists {
		m.mu.Unlock()
		return nil, databaseProfile{}, os.ErrNotExist
	}
	if !databaseProfileVisibleToSubject(profile, authSubjectFromContext(ctx)) {
		m.mu.Unlock()
		return nil, databaseProfile{}, os.ErrNotExist
	}
	catalog := m.catalog
	if runtime, ok := m.runtimes[databaseID]; ok {
		m.mu.Unlock()
		return NewServiceWithCatalog(runtime.cfg, runtime.store, catalog, profile.ID, profile.Name), profile, nil
	}
	m.mu.Unlock()

	// Cache miss — dial without holding m.mu.
	opened, err := openDatabaseRuntime(ctx, profile)
	if err != nil {
		return nil, databaseProfile{}, err
	}

	m.mu.Lock()
	runtime, exists := m.runtimes[databaseID]
	if exists {
		// Another goroutine won the race; discard our extra.
		m.mu.Unlock()
		if opened.closeFn != nil {
			opened.closeFn()
		}
	} else {
		m.runtimes[databaseID] = opened
		runtime = opened
		m.mu.Unlock()
	}
	return NewServiceWithCatalog(runtime.cfg, runtime.store, catalog, profile.ID, profile.Name), profile, nil
}

func (m *DatabaseManager) resolveWorkspaceServiceLocked(ctx context.Context, workspace string) (*Service, databaseProfile, workspaceCatalogRoute, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return nil, databaseProfile{}, workspaceCatalogRoute{}, fmt.Errorf("workspace id is required")
	}
	subject := authSubjectFromContext(ctx)

	if m.catalog != nil {
		routes, err := m.catalog.ResolveWorkspace(ctx, workspace)
		if err != nil {
			return nil, databaseProfile{}, workspaceCatalogRoute{}, err
		}
		filteredRoutes := make([]workspaceCatalogRoute, 0, len(routes))
		for _, route := range routes {
			profile, ok := m.profiles[route.DatabaseID]
			if !ok || !databaseProfileVisibleToSubject(profile, authSubjectFromContext(ctx)) {
				continue
			}
			if subject != "" && strings.TrimSpace(route.OwnerSubject) != "" && strings.TrimSpace(route.OwnerSubject) != subject {
				continue
			}
			filteredRoutes = append(filteredRoutes, route)
		}
		switch len(filteredRoutes) {
		case 1:
			service, profile, err := m.serviceForLocked(ctx, filteredRoutes[0].DatabaseID)
			return service, profile, filteredRoutes[0], err
		case 0:
			// Fall back to a scan so out-of-band changes can still be discovered.
		default:
			seen := make(map[string]struct{}, len(filteredRoutes))
			labels := make([]string, 0, len(filteredRoutes))
			for _, route := range filteredRoutes {
				profile := m.profiles[route.DatabaseID]
				label := route.WorkspaceID
				if profile.Name != "" && profile.Name != route.DatabaseID {
					label = label + " (" + profile.Name + ")"
				} else if route.DatabaseID != "" {
					label = label + " (" + route.DatabaseID + ")"
				}
				if _, ok := seen[label]; ok {
					continue
				}
				seen[label] = struct{}{}
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
		if profile, ok := m.profiles[id]; ok && !databaseProfileVisibleToSubject(profile, authSubjectFromContext(ctx)) {
			continue
		}
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
		meta, err := service.store.GetWorkspaceMeta(ctx, workspace)
		if err != nil {
			return nil, databaseProfile{}, workspaceCatalogRoute{}, err
		}
		route := workspaceCatalogRoute{
			DatabaseID:  profile.ID,
			WorkspaceID: workspaceStorageID(meta),
			Name:        meta.Name,
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
		seen := make(map[string]struct{}, len(matches))
		labels := make([]string, 0, len(matches))
		for _, route := range matches {
			profile := m.profiles[route.DatabaseID]
			label := route.WorkspaceID
			if profile.Name != "" && profile.Name != profile.ID {
				label = label + " (" + profile.Name + ")"
			} else if profile.ID != "" {
				label = label + " (" + profile.ID + ")"
			}
			if _, ok := seen[label]; ok {
				continue
			}
			seen[label] = struct{}{}
			labels = append(labels, label)
		}
		sort.Strings(labels)
		return nil, databaseProfile{}, workspaceCatalogRoute{}, fmt.Errorf("%w: workspace %q exists in multiple databases: %s", ErrAmbiguousWorkspace, workspace, strings.Join(labels, ", "))
	}
}

// perDatabaseListTimeout bounds how long a single database can stall a
// /v1/workspaces call before we fall back to the catalog snapshot. Hosted
// Redis can occasionally take a few seconds to answer large list calls, so
// keep this high enough to avoid transient drops while still preventing one
// dead backend from hanging the whole page.
const perDatabaseListTimeout = 5 * time.Second

func (m *DatabaseManager) listAllWorkspaceSummariesByFanout(ctx context.Context) (workspaceListResponse, error) {
	subject := authSubjectFromContext(ctx)

	// Snapshot the visible databases under the manager mutex, then release it
	// so the actual Redis I/O below does not block unrelated callers (the
	// stats poller, workspace creates, session reconciles). The old code held
	// m.mu for the entire fan-out, which serialized every list behind the
	// slowest backend.
	m.mu.Lock()
	visibleIDs := make([]string, 0, len(m.order))
	for _, id := range m.order {
		if profile, ok := m.profiles[id]; ok && !databaseProfileVisibleToSubject(profile, subject) {
			continue
		}
		visibleIDs = append(visibleIDs, id)
	}
	m.mu.Unlock()

	catalogFallbacks := map[string][]workspaceSummary{}
	if m.catalog != nil {
		if cached, err := m.catalog.ListWorkspaces(ctx); err == nil {
			for _, item := range cached {
				if strings.TrimSpace(item.DatabaseID) == "" {
					continue
				}
				catalogFallbacks[item.DatabaseID] = append(catalogFallbacks[item.DatabaseID], item)
			}
		}
	}

	var (
		mu    sync.Mutex
		items = make([]workspaceSummary, 0)
		wg    sync.WaitGroup
	)
	for _, id := range visibleIDs {
		wg.Add(1)
		go func(databaseID string) {
			defer wg.Done()
			dbCtx, cancel := context.WithTimeout(ctx, perDatabaseListTimeout)
			defer cancel()
			summaries, err := m.liveWorkspaceSummaries(dbCtx, databaseID)
			if err != nil {
				if fallback, ok := catalogFallbacks[databaseID]; ok {
					mu.Lock()
					items = append(items, fallback...)
					mu.Unlock()
				}
				return
			}
			mu.Lock()
			items = append(items, summaries...)
			mu.Unlock()
		}(id)
	}
	wg.Wait()

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

// liveWorkspaceSummaries fetches summaries for one database without holding
// m.mu across Redis I/O. Used by the concurrent fan-out in
// listAllWorkspaceSummariesByFanout. The catalog writes below
// (ListWorkspaceOwners, ReplaceDatabaseWorkspaces, RecordWorkspaceRefresh)
// do not require m.mu — the catalog is initialised once at construction and
// its own storage backend handles concurrency.
func (m *DatabaseManager) liveWorkspaceSummaries(ctx context.Context, databaseID string) ([]workspaceSummary, error) {
	service, profile, err := m.serviceFor(ctx, databaseID)
	if err != nil {
		if m.catalog != nil && !errors.Is(err, os.ErrNotExist) {
			_ = m.catalog.RecordWorkspaceRefresh(ctx, databaseID, "", time.Time{}, err)
		}
		return nil, err
	}
	return m.liveWorkspaceSummariesUnlocked(ctx, service, profile)
}

// liveWorkspaceSummariesUnlocked is the I/O-only tail shared between the
// locked and unlocked variants. Assumes the caller already resolved the
// service + profile.
func (m *DatabaseManager) liveWorkspaceSummariesUnlocked(ctx context.Context, service *Service, profile databaseProfile) ([]workspaceSummary, error) {
	response, err := service.ListWorkspaceSummaries(ctx)
	if err != nil {
		if m.catalog != nil {
			_ = m.catalog.RecordWorkspaceRefresh(ctx, profile.ID, profile.Name, time.Time{}, err)
		}
		return nil, err
	}

	subject := authSubjectFromContext(ctx)
	sharedDB := strings.TrimSpace(profile.OwnerSubject) == ""
	if sharedDB && m.catalog != nil {
		if owners, ownerErr := m.catalog.ListWorkspaceOwners(ctx, profile.ID); ownerErr == nil {
			for index := range response.Items {
				workspaceID := response.Items[index].ID
				if existing, ok := owners[workspaceID]; ok && strings.TrimSpace(existing.Subject) != "" {
					response.Items[index].OwnerSubject = existing.Subject
					response.Items[index].OwnerLabel = existing.Label
					continue
				}
				if subject != "" {
					response.Items[index].OwnerSubject = subject
					response.Items[index].OwnerLabel = authIdentityLabelFromContext(ctx)
				}
			}
		}
	}

	for index := range response.Items {
		stampWorkspaceSummary(&response.Items[index], profile)
	}
	if m.catalog != nil {
		synced, err := m.catalog.ReplaceDatabaseWorkspaces(ctx, profile.ID, response.Items)
		if err != nil {
			_ = m.catalog.RecordWorkspaceRefresh(ctx, profile.ID, profile.Name, time.Time{}, err)
			return nil, err
		}
		_ = m.catalog.RecordWorkspaceRefresh(ctx, profile.ID, profile.Name, time.Now().UTC(), nil)
		if sharedDB && subject != "" {
			filtered := make([]workspaceSummary, 0, len(synced))
			for _, item := range synced {
				if strings.TrimSpace(item.OwnerSubject) == subject {
					filtered = append(filtered, item)
				}
			}
			return filtered, nil
		}
		return synced, nil
	}
	return response.Items, nil
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

	// For shared databases (no profile owner), attach per-workspace ownership
	// from the catalog so subsequent stamping/syncing preserves it. Workspaces
	// with no existing catalog row are claimed by the requesting user — this
	// is how new free-tier workspaces become owned at first list.
	subject := authSubjectFromContext(ctx)
	sharedDB := strings.TrimSpace(profile.OwnerSubject) == ""
	if sharedDB && m.catalog != nil {
		owners, ownerErr := m.catalog.ListWorkspaceOwners(ctx, profile.ID)
		if ownerErr == nil {
			for index := range response.Items {
				workspaceID := response.Items[index].ID
				if existing, ok := owners[workspaceID]; ok && strings.TrimSpace(existing.Subject) != "" {
					response.Items[index].OwnerSubject = existing.Subject
					response.Items[index].OwnerLabel = existing.Label
					continue
				}
				if subject != "" {
					response.Items[index].OwnerSubject = subject
					response.Items[index].OwnerLabel = authIdentityLabelFromContext(ctx)
				}
			}
		}
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
		// Filter shared-DB results to the requesting user. Owned databases
		// already enforce visibility at the database level.
		if sharedDB && subject != "" {
			filtered := make([]workspaceSummary, 0, len(synced))
			for _, item := range synced {
				if strings.TrimSpace(item.OwnerSubject) == subject {
					filtered = append(filtered, item)
				}
			}
			return filtered, profile, nil
		}
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
		if err := m.catalog.DeleteWorkspace(ctx, databaseID, route.WorkspaceID); err != nil {
			return err
		}
		return m.catalog.DeleteSessionsForWorkspace(ctx, route.WorkspaceID)
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
		routes, err := m.catalog.ResolveWorkspace(ctx, workspace)
		if err != nil {
			return nil, databaseProfile{}, workspaceCatalogRoute{}, err
		}
		filteredRoutes := make([]workspaceCatalogRoute, 0, len(routes))
		subject := authSubjectFromContext(ctx)
		for _, route := range routes {
			if route.DatabaseID != profile.ID {
				continue
			}
			if subject != "" && strings.TrimSpace(route.OwnerSubject) != "" && strings.TrimSpace(route.OwnerSubject) != subject {
				continue
			}
			filteredRoutes = append(filteredRoutes, route)
		}
		switch len(filteredRoutes) {
		case 1:
			return service, profile, filteredRoutes[0], nil
		case 0:
			// Fall back to direct Redis discovery.
		default:
			return nil, databaseProfile{}, workspaceCatalogRoute{}, fmt.Errorf("%w: workspace %q is ambiguous in database %q", ErrAmbiguousWorkspace, workspace, profile.ID)
		}
	}

	exists, err := service.store.WorkspaceExists(ctx, workspace)
	if err != nil {
		return nil, databaseProfile{}, workspaceCatalogRoute{}, err
	}
	if !exists {
		return nil, databaseProfile{}, workspaceCatalogRoute{}, os.ErrNotExist
	}
	meta, err := service.store.GetWorkspaceMeta(ctx, workspace)
	if err != nil {
		return nil, databaseProfile{}, workspaceCatalogRoute{}, err
	}
	return service, profile, workspaceCatalogRoute{
		DatabaseID:  profile.ID,
		WorkspaceID: workspaceStorageID(meta),
		Name:        meta.Name,
	}, nil
}

func (m *DatabaseManager) attachWorkspaceSummaryIdentity(ctx context.Context, summary *workspaceSummary, profile databaseProfile) error {
	if summary == nil {
		return nil
	}
	if strings.TrimSpace(summary.OwnerSubject) == "" {
		if subject := authSubjectFromContext(ctx); subject != "" {
			summary.OwnerSubject = subject
			summary.OwnerLabel = authIdentityLabelFromContext(ctx)
		}
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
	if strings.TrimSpace(detail.OwnerSubject) == "" {
		if subject := authSubjectFromContext(ctx); subject != "" {
			detail.OwnerSubject = subject
			detail.OwnerLabel = authIdentityLabelFromContext(ctx)
		}
	}
	stampWorkspaceDetail(detail, profile)
	synced, err := m.syncWorkspaceCatalogSummary(ctx, workspaceSummaryFromDetail(*detail))
	if err != nil {
		return err
	}
	detail.ID = synced.ID
	if strings.TrimSpace(detail.TemplateSlug) == "" {
		detail.TemplateSlug = synced.TemplateSlug
	}
	return nil
}

func (m *DatabaseManager) buildProfileLocked(ctx context.Context, id string, input upsertDatabaseRequest) (databaseProfile, bool, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return databaseProfile{}, false, fmt.Errorf("database name is required")
	}
	if strings.EqualFold(name, quickstartCloudDBName) && strings.TrimSpace(id) != quickstartCloudDBID {
		return databaseProfile{}, false, fmt.Errorf("database name %q is reserved", quickstartCloudDBName)
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
	if !isNew {
		if existing, exists := m.profiles[resolvedID]; exists {
			if !databaseProfileVisibleToSubject(existing, authSubjectFromContext(ctx)) {
				return databaseProfile{}, false, os.ErrNotExist
			}
			if !databaseProfileCanEdit(existing) {
				return databaseProfile{}, false, fmt.Errorf("database %q is managed by AFS Cloud and cannot be edited", existing.Name)
			}
		}
	}

	password := input.RedisPassword
	if !isNew && strings.TrimSpace(password) == "" {
		if existing, exists := m.profiles[resolvedID]; exists {
			password = existing.RedisPassword
		}
	}

	return databaseProfile{
		ID:             resolvedID,
		Name:           name,
		Description:    strings.TrimSpace(input.Description),
		OwnerSubject:   ownerSubjectForDatabaseProfile(ctx, isNew, m.profiles[resolvedID]),
		OwnerLabel:     ownerLabelForDatabaseProfile(ctx, isNew, m.profiles[resolvedID]),
		ManagementType: managementTypeForDatabaseProfile(isNew, m.profiles[resolvedID]),
		Purpose:        purposeForDatabaseProfile(isNew, m.profiles[resolvedID]),
		RedisAddr:      normalizeRedisAddr(input.RedisAddr),
		RedisUsername:  strings.TrimSpace(input.RedisUsername),
		RedisPassword:  password,
		RedisDB:        input.RedisDB,
		RedisTLS:       input.RedisTLS,
		IsDefault:      !isNew && m.profiles[resolvedID].IsDefault,
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
	return m.resolveTargetDatabaseLocked(ctx, requestedID)
}

func (m *DatabaseManager) resolveTargetDatabaseLocked(ctx context.Context, requestedID string) (databaseProfile, error) {
	resolvedID := strings.TrimSpace(requestedID)
	subject := authSubjectFromContext(ctx)
	if resolvedID != "" {
		profile, exists := m.profiles[resolvedID]
		if !exists {
			return databaseProfile{}, os.ErrNotExist
		}
		if !databaseProfileVisibleToSubject(profile, subject) {
			return databaseProfile{}, os.ErrNotExist
		}
		return profile, nil
	}

	visibleOrder := make([]string, 0, len(m.order))
	for _, id := range m.order {
		profile, exists := m.profiles[id]
		if !exists || !databaseProfileVisibleToSubject(profile, subject) {
			continue
		}
		visibleOrder = append(visibleOrder, id)
	}

	switch len(visibleOrder) {
	case 0:
		return databaseProfile{}, fmt.Errorf("no databases are configured")
	case 1:
		return m.profiles[visibleOrder[0]], nil
	}

	if defaultID := m.effectiveDefaultDatabaseIDLocked(subject); defaultID != "" {
		if profile, exists := m.profiles[defaultID]; exists {
			for _, id := range visibleOrder {
				if id == defaultID {
					return profile, nil
				}
			}
		}
	}

	labels := make([]string, 0, len(visibleOrder))
	for _, id := range visibleOrder {
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

	firstByScope := make(map[string]string)
	defaultByScope := make(map[string]string)
	for _, id := range m.order {
		profile, exists := m.profiles[id]
		if !exists {
			continue
		}
		scope := databaseDefaultScopeKey(profile)
		if _, recorded := firstByScope[scope]; !recorded {
			firstByScope[scope] = id
		}
		if !profile.IsDefault {
			continue
		}
		if _, recorded := defaultByScope[scope]; !recorded {
			defaultByScope[scope] = id
			continue
		}
		profile.IsDefault = false
		m.profiles[id] = profile
	}
	for scope, id := range firstByScope {
		if _, exists := defaultByScope[scope]; exists {
			continue
		}
		profile := m.profiles[id]
		profile.IsDefault = true
		m.profiles[id] = profile
	}
}

func databaseDefaultScopeKey(profile databaseProfile) string {
	if owner := strings.TrimSpace(profile.OwnerSubject); owner != "" {
		return "subject:" + owner
	}
	return "global"
}

func (m *DatabaseManager) databaseRecordLocked(ctx context.Context, id string) (databaseRecord, error) {
	profile, exists := m.profiles[id]
	if !exists {
		return databaseRecord{}, os.ErrNotExist
	}
	defaultID := m.effectiveDefaultDatabaseIDLocked(authSubjectFromContext(ctx))

	record := databaseRecord{
		ID:                  profile.ID,
		Name:                profile.Name,
		Description:         profile.Description,
		OwnerSubject:        profile.OwnerSubject,
		OwnerLabel:          profile.OwnerLabel,
		ManagementType:      normalizedDatabaseManagementType(profile.ManagementType),
		Purpose:             normalizedDatabasePurpose(profile.Purpose),
		CanEdit:             databaseProfileCanEdit(profile),
		CanDelete:           databaseProfileCanDelete(profile),
		CanCreateWorkspaces: databaseProfileCanCreateWorkspaces(profile),
		RedisAddr:           profile.RedisAddr,
		RedisUsername:       profile.RedisUsername,
		RedisDB:             profile.RedisDB,
		RedisTLS:            profile.RedisTLS,
		IsDefault:           id == defaultID,
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
	// For owned databases (user-managed), workspaces inherit the database
	// owner. For shared databases (no profile owner), preserve any existing
	// per-workspace owner that the caller already populated (e.g. from the
	// catalog) so we don't overwrite per-user ownership during list calls.
	if v := strings.TrimSpace(profile.OwnerSubject); v != "" {
		summary.OwnerSubject = v
		summary.OwnerLabel = profile.OwnerLabel
	}
	summary.DatabaseManagementType = normalizedDatabaseManagementType(profile.ManagementType)
	summary.DatabaseCanEdit = databaseProfileCanEdit(profile)
	summary.DatabaseCanDelete = databaseProfileCanDelete(profile)
}

func (m *DatabaseManager) effectiveDefaultDatabaseIDLocked(subject string) string {
	visibleOrder := make([]string, 0, len(m.order))
	creatableVisibleOrder := make([]string, 0, len(m.order))
	for _, id := range m.order {
		profile, exists := m.profiles[id]
		if !exists || !databaseProfileVisibleToSubject(profile, subject) {
			continue
		}
		visibleOrder = append(visibleOrder, id)
		if databaseProfileCanCreateWorkspaces(profile) {
			creatableVisibleOrder = append(creatableVisibleOrder, id)
		}
	}
	if len(visibleOrder) == 0 {
		return ""
	}
	if len(visibleOrder) == 1 {
		return visibleOrder[0]
	}
	if len(creatableVisibleOrder) == 1 {
		return creatableVisibleOrder[0]
	}

	subject = strings.TrimSpace(subject)
	if subject != "" {
		for _, id := range creatableVisibleOrder {
			profile := m.profiles[id]
			if strings.TrimSpace(profile.OwnerSubject) == subject && profile.IsDefault {
				return id
			}
		}
	}
	for _, id := range creatableVisibleOrder {
		if m.profiles[id].IsDefault {
			return id
		}
	}
	for _, id := range visibleOrder {
		if m.profiles[id].IsDefault {
			return id
		}
	}
	if subject != "" {
		for _, id := range visibleOrder {
			profile := m.profiles[id]
			if strings.TrimSpace(profile.OwnerSubject) == subject &&
				normalizedDatabaseManagementType(profile.ManagementType) == databaseManagementSystemManaged {
				return id
			}
		}
	}
	if len(creatableVisibleOrder) > 0 {
		return creatableVisibleOrder[0]
	}
	return visibleOrder[0]
}

func stampWorkspaceDetail(detail *workspaceDetail, profile databaseProfile) {
	detail.DatabaseID = profile.ID
	detail.DatabaseName = profile.Name
	// Same per-user-owner preservation as stampWorkspaceSummary — owned
	// databases stamp their owner; shared databases keep whatever owner the
	// caller already attached.
	if v := strings.TrimSpace(profile.OwnerSubject); v != "" {
		detail.OwnerSubject = v
		detail.OwnerLabel = profile.OwnerLabel
	}
	detail.DatabaseManagementType = normalizedDatabaseManagementType(profile.ManagementType)
	detail.DatabaseCanEdit = databaseProfileCanEdit(profile)
	detail.DatabaseCanDelete = databaseProfileCanDelete(profile)
}

func normalizedDatabaseManagementType(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "", databaseManagementUserManaged:
		return databaseManagementUserManaged
	case databaseManagementSystemManaged:
		return databaseManagementSystemManaged
	default:
		return databaseManagementUserManaged
	}
}

func normalizedDatabasePurpose(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "", databasePurposeGeneral:
		return databasePurposeGeneral
	case databasePurposeOnboarding:
		return databasePurposeOnboarding
	default:
		return databasePurposeGeneral
	}
}

func databaseProfileCanEdit(profile databaseProfile) bool {
	return normalizedDatabaseManagementType(profile.ManagementType) != databaseManagementSystemManaged
}

func databaseProfileCanDelete(profile databaseProfile) bool {
	return normalizedDatabaseManagementType(profile.ManagementType) != databaseManagementSystemManaged
}

func databaseProfileCanCreateWorkspaces(profile databaseProfile) bool {
	// Free-tier quotas are enforced at create time (per-user) rather than as
	// a blanket block on onboarding databases. The UI uses this flag to
	// decide whether the "Add workspace" button is enabled, so we always
	// return true and let the create handler reject quota violations with
	// ErrFreeTierLimitReached.
	_ = profile
	return true
}

func ownerSubjectForDatabaseProfile(ctx context.Context, isNew bool, existing databaseProfile) string {
	if !isNew {
		return strings.TrimSpace(existing.OwnerSubject)
	}
	if identity, ok := AuthIdentityFromContext(ctx); ok {
		return strings.TrimSpace(identity.Subject)
	}
	return ""
}

func ownerLabelForDatabaseProfile(ctx context.Context, isNew bool, existing databaseProfile) string {
	if !isNew {
		return strings.TrimSpace(existing.OwnerLabel)
	}
	if identity, ok := AuthIdentityFromContext(ctx); ok {
		if value := strings.TrimSpace(identity.Name); value != "" {
			return value
		}
		if value := strings.TrimSpace(identity.Email); value != "" {
			return value
		}
		if value := strings.TrimSpace(identity.Subject); value != "" {
			return value
		}
	}
	return ""
}

func managementTypeForDatabaseProfile(isNew bool, existing databaseProfile) string {
	if !isNew {
		return normalizedDatabaseManagementType(existing.ManagementType)
	}
	return databaseManagementUserManaged
}

func purposeForDatabaseProfile(isNew bool, existing databaseProfile) string {
	if !isNew {
		return normalizedDatabasePurpose(existing.Purpose)
	}
	return databasePurposeGeneral
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

func authSubjectFromContext(ctx context.Context) string {
	if identity, ok := AuthIdentityFromContext(ctx); ok {
		return strings.TrimSpace(identity.Subject)
	}
	return ""
}

func authIdentityLabelFromContext(ctx context.Context) string {
	identity, ok := AuthIdentityFromContext(ctx)
	if !ok {
		return ""
	}
	if v := strings.TrimSpace(identity.Name); v != "" {
		return v
	}
	if v := strings.TrimSpace(identity.Email); v != "" {
		return v
	}
	return strings.TrimSpace(identity.Subject)
}

func databaseProfileVisibleToSubject(profile databaseProfile, subject string) bool {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return true
	}
	ownerSubject := strings.TrimSpace(profile.OwnerSubject)
	if ownerSubject == "" {
		return true
	}
	return ownerSubject == subject
}
