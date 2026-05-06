package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const monitorEventsChannel = "afs:monitor:events"

type monitorEvent struct {
	Type          string `json:"type"`
	DatabaseID    string `json:"database_id,omitempty"`
	DatabaseName  string `json:"database_name,omitempty"`
	WorkspaceID   string `json:"workspace_id,omitempty"`
	WorkspaceName string `json:"workspace_name,omitempty"`
	OwnerSubject  string `json:"owner_subject,omitempty"`
	SessionID     string `json:"session_id,omitempty"`
	Reason        string `json:"reason,omitempty"`
	CreatedAt     string `json:"created_at"`
}

func newMonitorEvent(eventType, reason string, meta WorkspaceMeta) monitorEvent {
	databaseID := strings.TrimSpace(meta.DatabaseID)
	databaseName := strings.TrimSpace(meta.DatabaseName)
	return monitorEvent{
		Type:          strings.TrimSpace(eventType),
		DatabaseID:    databaseID,
		DatabaseName:  databaseName,
		WorkspaceID:   WorkspaceStorageID(meta),
		WorkspaceName: strings.TrimSpace(meta.Name),
		Reason:        strings.TrimSpace(reason),
		CreatedAt:     time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func (s *Store) publishMonitorEvent(ctx context.Context, event monitorEvent) {
	publishMonitorEvent(ctx, s.rdb, event)
}

func publishMonitorEvent(ctx context.Context, rdb redis.Cmdable, event monitorEvent) {
	if rdb == nil {
		return
	}
	if strings.TrimSpace(event.Type) == "" {
		return
	}
	if strings.TrimSpace(event.CreatedAt) == "" {
		event.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	publisher, ok := rdb.(interface {
		Publish(context.Context, string, interface{}) *redis.IntCmd
	})
	if !ok {
		return
	}
	_ = publisher.Publish(ctx, monitorEventsChannel, data).Err()
}

func (s *Service) publishSessionMonitorEvent(ctx context.Context, workspace string, record WorkspaceSessionRecord, reason string) {
	meta, _, err := s.store.resolveWorkspaceMeta(ctx, workspace)
	if err != nil {
		return
	}
	if strings.TrimSpace(meta.DatabaseID) == "" {
		meta.DatabaseID = strings.TrimSpace(s.catalogDatabaseID)
	}
	if strings.TrimSpace(meta.DatabaseName) == "" {
		meta.DatabaseName = strings.TrimSpace(s.catalogDatabaseName)
	}
	event := newMonitorEvent("agents", reason, meta)
	event.SessionID = strings.TrimSpace(record.SessionID)
	s.store.publishMonitorEvent(ctx, event)
}

type monitorSubscriptionTarget struct {
	databaseID string
	profile    databaseProfile
	rdb        *redis.Client
}

func (m *DatabaseManager) monitorSubscriptionTargets(ctx context.Context) []monitorSubscriptionTarget {
	m.mu.Lock()
	order := append([]string(nil), m.order...)
	profiles := make(map[string]databaseProfile, len(m.profiles))
	for id, profile := range m.profiles {
		profiles[id] = profile
	}
	m.mu.Unlock()

	targets := make([]monitorSubscriptionTarget, 0, len(order))
	for _, databaseID := range order {
		profile, ok := profiles[databaseID]
		if !ok || !databaseProfileVisibleToSubject(profile, authSubjectFromContext(ctx)) {
			continue
		}
		service, resolvedProfile, err := m.serviceFor(ctx, databaseID)
		if err != nil {
			continue
		}
		targets = append(targets, monitorSubscriptionTarget{
			databaseID: databaseID,
			profile:    resolvedProfile,
			rdb:        service.store.rdb,
		})
	}
	return targets
}

func (m *DatabaseManager) subscribeMonitorEvents(ctx context.Context) (<-chan monitorEvent, func()) {
	streamCtx, cancel := context.WithCancel(ctx)
	out := make(chan monitorEvent, 32)
	targets := m.monitorSubscriptionTargets(streamCtx)
	subject := authSubjectFromContext(streamCtx)

	var wg sync.WaitGroup
	for _, target := range targets {
		pubsub := target.rdb.Subscribe(streamCtx, monitorEventsChannel)
		if _, err := pubsub.Receive(streamCtx); err != nil {
			_ = pubsub.Close()
			continue
		}

		wg.Add(1)
		go func(target monitorSubscriptionTarget, pubsub *redis.PubSub) {
			defer wg.Done()
			defer pubsub.Close()
			messages := pubsub.Channel()

			for {
				select {
				case <-streamCtx.Done():
					return
				case msg, ok := <-messages:
					if !ok {
						return
					}
					var event monitorEvent
					if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
						continue
					}
					event.DatabaseID = target.databaseID
					event.DatabaseName = target.profile.Name
					if !m.monitorEventVisibleToSubject(streamCtx, event, target.profile, subject) {
						continue
					}
					select {
					case out <- event:
					case <-streamCtx.Done():
						return
					}
				}
			}
		}(target, pubsub)
	}

	done := make(chan struct{})
	go func() {
		<-streamCtx.Done()
		wg.Wait()
		close(out)
		close(done)
	}()

	return out, func() {
		cancel()
		<-done
	}
}

func (m *DatabaseManager) monitorEventVisibleToSubject(ctx context.Context, event monitorEvent, profile databaseProfile, subject string) bool {
	if !databaseProfileVisibleToSubject(profile, subject) {
		return false
	}
	if strings.TrimSpace(subject) == "" || strings.TrimSpace(profile.OwnerSubject) != "" {
		return true
	}
	if m.catalog == nil {
		return true
	}
	workspaceID := strings.TrimSpace(event.WorkspaceID)
	if workspaceID == "" {
		return false
	}
	if owner := strings.TrimSpace(event.OwnerSubject); owner != "" {
		return owner == subject
	}
	owners, err := m.catalog.ListWorkspaceOwners(ctx, strings.TrimSpace(event.DatabaseID))
	if err != nil {
		return false
	}
	owner, ok := owners[workspaceID]
	return ok && strings.TrimSpace(owner.Subject) == subject
}

func (m *DatabaseManager) publishWorkspaceMonitorEvent(ctx context.Context, service *Service, profile databaseProfile, detail workspaceDetail, reason string) {
	if service == nil || service.store == nil {
		return
	}
	event := monitorEvent{
		Type:          "workspaces",
		DatabaseID:    strings.TrimSpace(profile.ID),
		DatabaseName:  strings.TrimSpace(profile.Name),
		WorkspaceID:   strings.TrimSpace(detail.ID),
		WorkspaceName: strings.TrimSpace(detail.Name),
		OwnerSubject:  strings.TrimSpace(detail.OwnerSubject),
		Reason:        strings.TrimSpace(reason),
		CreatedAt:     time.Now().UTC().Format(time.RFC3339Nano),
	}
	service.store.publishMonitorEvent(ctx, event)
}

func (m *DatabaseManager) publishWorkspaceRouteMonitorEvent(ctx context.Context, service *Service, profile databaseProfile, route workspaceCatalogRoute, reason string) {
	m.publishWorkspaceMonitorEvent(ctx, service, profile, workspaceDetail{
		ID:           strings.TrimSpace(route.WorkspaceID),
		Name:         strings.TrimSpace(route.Name),
		OwnerSubject: strings.TrimSpace(route.OwnerSubject),
	}, reason)
}

func (m *DatabaseManager) publishMCPTokenMonitorEvent(ctx context.Context, record mcpAccessTokenRecord, reason string) {
	event := monitorEvent{
		Type:          "mcp-tokens",
		DatabaseID:    strings.TrimSpace(record.DatabaseID),
		WorkspaceID:   strings.TrimSpace(record.WorkspaceID),
		WorkspaceName: strings.TrimSpace(record.WorkspaceName),
		OwnerSubject:  strings.TrimSpace(record.OwnerSubject),
		Reason:        strings.TrimSpace(reason),
		CreatedAt:     time.Now().UTC().Format(time.RFC3339Nano),
	}
	if strings.TrimSpace(record.DatabaseID) != "" {
		service, profile, err := m.serviceFor(ctx, record.DatabaseID)
		if err != nil {
			return
		}
		event.DatabaseName = profile.Name
		service.store.publishMonitorEvent(ctx, event)
		return
	}
	for _, target := range m.monitorSubscriptionTargets(ctx) {
		event.DatabaseID = target.databaseID
		event.DatabaseName = target.profile.Name
		publishMonitorEvent(ctx, target.rdb, event)
	}
}

func handleMonitorStream(w http.ResponseWriter, r *http.Request, manager *DatabaseManager) {
	if r.Method != http.MethodGet {
		writeError(w, fmt.Errorf("%s not allowed", r.Method))
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, fmt.Errorf("streaming is not supported"))
		return
	}

	events, closeFn := manager.subscribeMonitorEvents(r.Context())
	defer closeFn()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	writeSSE(w, "ready", monitorEvent{
		Type:      "ready",
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	})
	flusher.Flush()

	keepAlive := time.NewTicker(25 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepAlive.C:
			_, _ = fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
		case event, ok := <-events:
			if !ok {
				return
			}
			writeSSE(w, "monitor", event)
			flusher.Flush()
		}
	}
}

func writeSSE(w http.ResponseWriter, eventName string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "event: %s\n", eventName)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
}
