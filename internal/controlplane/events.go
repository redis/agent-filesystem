package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const eventStreamMaxLen = 200000

type eventListRequest struct {
	Kind      string
	SessionID string
	Path      string
	Since     string
	Until     string
	Limit     int
	Reverse   bool
}

type eventListResponse struct {
	Items      []eventEntry `json:"items"`
	NextCursor string       `json:"next_cursor,omitempty"`
}

type eventEntry struct {
	ID            string            `json:"id"`
	WorkspaceID   string            `json:"workspace_id,omitempty"`
	WorkspaceName string            `json:"workspace_name,omitempty"`
	DatabaseID    string            `json:"database_id,omitempty"`
	DatabaseName  string            `json:"database_name,omitempty"`
	CreatedAt     string            `json:"created_at,omitempty"`
	Kind          string            `json:"kind"`
	Op            string            `json:"op"`
	Source        string            `json:"source,omitempty"`
	Actor         string            `json:"actor,omitempty"`
	SessionID     string            `json:"session_id,omitempty"`
	User          string            `json:"user,omitempty"`
	Label         string            `json:"label,omitempty"`
	AgentVersion  string            `json:"agent_version,omitempty"`
	Hostname      string            `json:"hostname,omitempty"`
	Path          string            `json:"path,omitempty"`
	PrevPath      string            `json:"prev_path,omitempty"`
	SizeBytes     int64             `json:"size_bytes,omitempty"`
	DeltaBytes    int64             `json:"delta_bytes,omitempty"`
	ContentHash   string            `json:"content_hash,omitempty"`
	PrevHash      string            `json:"prev_hash,omitempty"`
	Mode          uint32            `json:"mode,omitempty"`
	CheckpointID  string            `json:"checkpoint_id,omitempty"`
	Extras        map[string]string `json:"extras,omitempty"`
}

type EventListRequest = eventListRequest
type EventListResponse = eventListResponse
type EventEntry = eventEntry

func workspaceEventsKey(workspace string) string {
	return fmt.Sprintf("afs:{%s}:workspace:events", workspace)
}

func WorkspaceEventsKey(workspace string) string {
	return workspaceEventsKey(workspace)
}

func enqueueEventFields(ctx context.Context, pipe redis.Pipeliner, storageID string, fields map[string]any) {
	if pipe == nil || len(fields) == 0 {
		return
	}
	pipe.XAdd(ctx, &redis.XAddArgs{
		Stream: workspaceEventsKey(storageID),
		MaxLen: eventStreamMaxLen,
		Approx: true,
		Values: fields,
	})
}

func auditEventFields(values map[string]any) map[string]any {
	op := auditFieldString(values, "op")
	kind, eventOp := auditEventKindAndOp(op)
	fields := map[string]any{
		"ts_ms":     auditFieldString(values, "ts_ms"),
		"workspace": auditFieldString(values, "workspace"),
		"kind":      kind,
		"op":        eventOp,
		"actor":     "afs",
		"source":    "server",
	}
	if checkpointID := firstNonEmpty(auditFieldString(values, "checkpoint"), auditFieldString(values, "savepoint")); checkpointID != "" {
		fields["checkpoint_id"] = checkpointID
	}
	if sessionID := strings.TrimSpace(auditFieldString(values, "session_id")); sessionID != "" {
		fields["session_id"] = sessionID
	}
	if hostname := strings.TrimSpace(auditFieldString(values, "hostname")); hostname != "" {
		fields["hostname"] = hostname
	}
	extras := map[string]string{}
	for key, value := range values {
		switch key {
		case "ts_ms", "workspace", "op", "checkpoint", "savepoint", "session_id", "hostname":
			continue
		default:
			text := strings.TrimSpace(fmt.Sprint(value))
			if text != "" {
				extras[key] = text
			}
		}
	}
	if len(extras) > 0 {
		if data, err := json.Marshal(extras); err == nil {
			fields["extras"] = string(data)
		}
	}
	return fields
}

func auditFieldString(values map[string]any, key string) string {
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func auditEventKindAndOp(op string) (string, string) {
	switch op {
	case "workspace_create":
		return "workspace", "create"
	case "import":
		return "workspace", "import"
	case "workspace_fork":
		return "workspace", "fork"
	case "workspace_update":
		return "workspace", "update"
	case "save":
		return "checkpoint", "save"
	case "checkpoint_restore":
		return "checkpoint", "restore"
	case "session_start":
		return "session", "start"
	case "session_close":
		return "session", "close"
	case "session_stale":
		return "session", "stale"
	case "run_start":
		return "process", "start"
	case "run_exit":
		return "process", "exit"
	default:
		return "workspace", op
	}
}

func changeEventFields(fields map[string]any) map[string]any {
	event := map[string]any{
		"ts_ms":  fmt.Sprint(fields["ts_ms"]),
		"kind":   "file",
		"op":     fmt.Sprint(fields["op"]),
		"source": fmt.Sprint(fields["source"]),
	}
	copyStringField(event, fields, "session_id")
	copyStringField(event, fields, "agent_id")
	copyStringField(event, fields, "user")
	copyStringField(event, fields, "label")
	copyStringField(event, fields, "agent_version")
	copyStringField(event, fields, "path")
	copyStringField(event, fields, "prev_path")
	copyStringField(event, fields, "size_bytes")
	copyStringField(event, fields, "delta_bytes")
	copyStringField(event, fields, "content_hash")
	copyStringField(event, fields, "prev_hash")
	copyStringField(event, fields, "mode")
	copyStringField(event, fields, "checkpoint_id")
	event["actor"] = firstNonEmpty(
		stringField(fields, "label"),
		stringField(fields, "agent_id"),
		stringField(fields, "user"),
		stringField(fields, "session_id"),
		"afs",
	)
	return event
}

func copyStringField(dst, src map[string]any, key string) {
	value := strings.TrimSpace(stringField(src, key))
	if value != "" {
		dst[key] = value
	}
}

func stringField(fields map[string]any, key string) string {
	if value, ok := fields[key]; ok {
		return fmt.Sprint(value)
	}
	return ""
}

func (s *Store) ListEvents(ctx context.Context, storageID string, req EventListRequest) (EventListResponse, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	start := "-"
	end := "+"
	if strings.TrimSpace(req.Since) != "" {
		start = "(" + strings.TrimSpace(req.Since)
	}
	if strings.TrimSpace(req.Until) != "" {
		end = "(" + strings.TrimSpace(req.Until)
	}
	fetch := int64(limit)
	if req.Kind != "" || req.SessionID != "" || req.Path != "" {
		fetch = int64(limit) * 4
		if fetch > 4000 {
			fetch = 4000
		}
	}
	var (
		msgs []redis.XMessage
		err  error
	)
	if req.Reverse {
		msgs, err = s.rdb.XRevRangeN(ctx, workspaceEventsKey(storageID), end, start, fetch).Result()
	} else {
		msgs, err = s.rdb.XRangeN(ctx, workspaceEventsKey(storageID), start, end, fetch).Result()
	}
	if err != nil {
		return EventListResponse{}, err
	}
	items := make([]eventEntry, 0, len(msgs))
	for _, msg := range msgs {
		item := eventFromStreamMessage(msg)
		if req.Kind != "" && item.Kind != req.Kind {
			continue
		}
		if req.SessionID != "" && item.SessionID != req.SessionID {
			continue
		}
		if req.Path != "" && item.Path != req.Path {
			continue
		}
		items = append(items, item)
		if len(items) >= limit {
			break
		}
	}
	response := EventListResponse{Items: items}
	if len(items) > 0 {
		response.NextCursor = items[len(items)-1].ID
	}
	return response, nil
}

func eventFromStreamMessage(msg redis.XMessage) eventEntry {
	item := eventEntry{ID: msg.ID}
	getField := func(key string) string {
		if value, ok := msg.Values[key]; ok {
			return fmt.Sprint(value)
		}
		return ""
	}
	if raw := getField("ts_ms"); raw != "" {
		if ms, err := strconv.ParseInt(raw, 10, 64); err == nil {
			item.CreatedAt = time.UnixMilli(ms).UTC().Format(time.RFC3339)
		}
	}
	item.WorkspaceName = getField("workspace")
	item.Kind = getField("kind")
	item.Op = getField("op")
	item.Source = getField("source")
	item.Actor = getField("actor")
	item.SessionID = getField("session_id")
	item.User = getField("user")
	item.Label = getField("label")
	item.AgentVersion = getField("agent_version")
	item.Hostname = getField("hostname")
	item.Path = getField("path")
	item.PrevPath = getField("prev_path")
	if raw := getField("size_bytes"); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
			item.SizeBytes = n
		}
	}
	if raw := getField("delta_bytes"); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
			item.DeltaBytes = n
		}
	}
	item.ContentHash = getField("content_hash")
	item.PrevHash = getField("prev_hash")
	if raw := getField("mode"); raw != "" {
		if n, err := strconv.ParseUint(raw, 10, 32); err == nil {
			item.Mode = uint32(n)
		}
	}
	item.CheckpointID = getField("checkpoint_id")
	if raw := getField("extras"); raw != "" {
		extras := map[string]string{}
		if err := json.Unmarshal([]byte(raw), &extras); err == nil && len(extras) > 0 {
			item.Extras = extras
		}
	}
	return item
}

func (s *Service) listWorkspaceEvents(ctx context.Context, workspace string, req EventListRequest) (EventListResponse, error) {
	meta, err := s.store.GetWorkspaceMeta(ctx, workspace)
	if err != nil {
		return EventListResponse{}, err
	}
	storageID := workspaceStorageID(meta)
	response, err := s.store.ListEvents(ctx, storageID, normalizeEventListRequest(req))
	if err != nil {
		return EventListResponse{}, err
	}
	for i := range response.Items {
		response.Items[i].WorkspaceID = storageID
		response.Items[i].WorkspaceName = meta.Name
	}
	return response, nil
}

func (s *Service) listGlobalEvents(ctx context.Context, req EventListRequest) (EventListResponse, error) {
	req = normalizeEventListRequest(req)
	metas, err := s.store.ListWorkspaces(ctx)
	if err != nil {
		return EventListResponse{}, err
	}
	items := make([]eventEntry, 0, req.Limit)
	for _, meta := range metas {
		storageID := workspaceStorageID(meta)
		page, err := s.store.ListEvents(ctx, storageID, req)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return EventListResponse{}, err
		}
		for _, item := range page.Items {
			item.WorkspaceID = storageID
			item.WorkspaceName = meta.Name
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

func normalizeEventListRequest(req EventListRequest) EventListRequest {
	req.Kind = strings.TrimSpace(req.Kind)
	req.SessionID = strings.TrimSpace(req.SessionID)
	req.Path = strings.TrimSpace(req.Path)
	req.Since = strings.TrimSpace(req.Since)
	req.Until = strings.TrimSpace(req.Until)
	if req.Limit <= 0 {
		req.Limit = 100
	}
	if req.Limit > 1000 {
		req.Limit = 1000
	}
	return req
}
