package controlplane

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Per-session file-change log. Append-only Redis Stream per workspace plus
// companion hashes for session rollups and last-writer lookups. Keep this
// aligned with docs/internals/versioned-filesystem.md.

// Change source tags. One value per origin so session logs can tell a
// live-sync FS write apart from an explicit checkpoint save, a restore, or
// the initial import seed.
const (
	ChangeSourceCheckpoint      = "checkpoint"
	ChangeSourceAgentSync       = "agent_sync"
	ChangeSourceMCP             = "mcp"
	ChangeSourceMount           = "mount"
	ChangeSourceVersionRestore  = "version_restore"
	ChangeSourceVersionUndelete = "version_undelete"
	ChangeSourceServerRestore   = "server_restore"
	ChangeSourceImport          = "import"
)

// Change ops. Derived from manifest-entry diffs.
const (
	ChangeOpPut     = "put"     // create or modify a file
	ChangeOpDelete  = "delete"  // remove a path
	ChangeOpMkdir   = "mkdir"   // create a directory
	ChangeOpRmdir   = "rmdir"   // remove a directory
	ChangeOpRename  = "rename"  // move a tracked file lineage to a new path
	ChangeOpSymlink = "symlink" // create/replace a symlink
	ChangeOpChmod   = "chmod"   // mode-only change on an existing entry
)

// changelogStreamMaxLen is the soft cap passed to XADD MAXLEN ~ per workspace.
// Older entries are trimmed probabilistically beyond this.
const changelogStreamMaxLen = 100000

// ChangeEntry is a single row in the workspace changelog stream. Fields with
// empty/zero values are elided at write time to keep stream entries compact.
type ChangeEntry struct {
	SessionID    string // session that caused the change; empty for server-initiated ops
	AgentID      string // stable agent identity across multiple sessions; optional
	User         string // authenticated principal (Clerk user, CLI token owner); optional
	Label        string // human-readable session label; optional
	AgentVersion string // client afs version; optional
	Op           string // one of ChangeOp*
	Path         string // workspace-relative path
	PrevPath     string // set on renames
	SizeBytes    int64  // final size after op
	DeltaBytes   int64  // signed change in size vs previous state
	ContentHash  string // post-op blob hash or manifest-entry hash proxy
	PrevHash     string // pre-op blob hash
	Mode         uint32 // final mode bits
	FileID       string // exact versioned-file lineage id when available
	VersionID    string // exact file version id produced by the operation when available
	CheckpointID string // set when op was part of a checkpoint save
	Source       string // one of ChangeSource*
}

func (e ChangeEntry) fields() map[string]any {
	fields := map[string]any{
		"op":     e.Op,
		"path":   e.Path,
		"source": e.Source,
		"ts_ms":  strconv.FormatInt(time.Now().UTC().UnixMilli(), 10),
	}
	if e.SessionID != "" {
		fields["session_id"] = e.SessionID
	}
	if e.AgentID != "" {
		fields["agent_id"] = e.AgentID
	}
	if e.User != "" {
		fields["user"] = e.User
	}
	if e.Label != "" {
		fields["label"] = e.Label
	}
	if e.AgentVersion != "" {
		fields["agent_version"] = e.AgentVersion
	}
	if e.PrevPath != "" {
		fields["prev_path"] = e.PrevPath
	}
	if e.SizeBytes != 0 {
		fields["size_bytes"] = strconv.FormatInt(e.SizeBytes, 10)
	}
	if e.DeltaBytes != 0 {
		fields["delta_bytes"] = strconv.FormatInt(e.DeltaBytes, 10)
	}
	if e.ContentHash != "" {
		fields["content_hash"] = e.ContentHash
	}
	if e.PrevHash != "" {
		fields["prev_hash"] = e.PrevHash
	}
	if e.Mode != 0 {
		fields["mode"] = strconv.FormatUint(uint64(e.Mode), 10)
	}
	if e.FileID != "" {
		fields["file_id"] = e.FileID
	}
	if e.VersionID != "" {
		fields["version_id"] = e.VersionID
	}
	if e.CheckpointID != "" {
		fields["checkpoint_id"] = e.CheckpointID
	}
	return fields
}

// enqueueChangeEntries appends entries to the per-workspace changelog stream
// and updates the companion session-summary and path:last hashes. All writes
// are queued on the caller-provided pipeliner so they commit with the parent
// transaction when the caller runs Exec.
func enqueueChangeEntries(ctx context.Context, pipe redis.Pipeliner, storageID string, entries []ChangeEntry) {
	if len(entries) == 0 || pipe == nil {
		return
	}
	streamKey := changelogStreamKey(storageID)
	for _, entry := range entries {
		fields := entry.fields()
		pipe.XAdd(ctx, &redis.XAddArgs{
			Stream: streamKey,
			MaxLen: changelogStreamMaxLen,
			Approx: true,
			Values: fields,
		})
		enqueueEventFields(ctx, pipe, storageID, changeEventFields(fields))
		if entry.SessionID != "" {
			summaryKey := sessionSummaryKey(storageID, entry.SessionID)
			pipe.HIncrBy(ctx, summaryKey, "op_"+entry.Op, 1)
			if entry.DeltaBytes != 0 {
				pipe.HIncrBy(ctx, summaryKey, "delta_bytes", entry.DeltaBytes)
			}
			pipe.HSet(ctx, summaryKey, "last_op_at_ms", strconv.FormatInt(time.Now().UTC().UnixMilli(), 10))
		}
		pipe.HSet(ctx, pathLastKey(storageID, entry.Path), map[string]any{
			"session_id":   entry.SessionID,
			"op":           entry.Op,
			"content_hash": entry.ContentHash,
			"occurred_ms":  strconv.FormatInt(time.Now().UTC().UnixMilli(), 10),
		})
	}
}

// WriteChangeEntries is the exported non-transactional write path. Used by
// clients that hold a Redis connection directly (e.g. the sync daemon's
// uploader goroutine in cmd/afs) and by in-package import paths. Failures are
// logged but do NOT surface to the caller — observability must not block
// correctness.
func WriteChangeEntries(ctx context.Context, rdb redis.Cmdable, storageID string, entries []ChangeEntry) {
	writeChangeEntries(ctx, rdb, storageID, entries)
}

// writeChangeEntries is the non-transactional variant used by import paths
// that don't already hold a pipeliner. Failures are logged but do NOT surface
// to the caller — observability must not block correctness.
func writeChangeEntries(ctx context.Context, rdb redis.Cmdable, storageID string, entries []ChangeEntry) {
	if len(entries) == 0 || rdb == nil {
		return
	}
	pipe := rdb.Pipeline()
	enqueueChangeEntries(ctx, pipe, storageID, entries)
	if _, err := pipe.Exec(ctx); err != nil {
		slog.Warn("changelog write failed",
			"workspace", storageID,
			"entries", len(entries),
			"err", err)
		return
	}
	publishMonitorEvent(ctx, rdb, monitorEvent{
		Type:        "changes",
		WorkspaceID: strings.TrimSpace(storageID),
		Reason:      "changed",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	})
}

// manifestDiff computes the set of change entries that represent the
// transition from parent → child. Template fields (SessionID, User, Source,
// CheckpointID, etc) are applied to every emitted entry. Returns entries in
// deterministic path order for reproducible tests.
func manifestDiff(parent, child Manifest, template ChangeEntry) []ChangeEntry {
	parentEntries := parent.Entries
	childEntries := child.Entries
	if parentEntries == nil {
		parentEntries = map[string]ManifestEntry{}
	}
	if childEntries == nil {
		childEntries = map[string]ManifestEntry{}
	}

	paths := make(map[string]struct{}, len(parentEntries)+len(childEntries))
	for p := range parentEntries {
		paths[p] = struct{}{}
	}
	for p := range childEntries {
		paths[p] = struct{}{}
	}
	ordered := make([]string, 0, len(paths))
	for p := range paths {
		ordered = append(ordered, p)
	}
	// Sort so tests and logs are deterministic; cheap for typical manifest sizes.
	sort.Strings(ordered)

	out := make([]ChangeEntry, 0, len(ordered))
	for _, path := range ordered {
		if path == "/" {
			continue
		}
		prev, hadPrev := parentEntries[path]
		next, hasNext := childEntries[path]
		switch {
		case hadPrev && !hasNext:
			entry := template
			entry.Path = path
			entry.Op = deleteOpFor(prev.Type)
			entry.PrevHash = entryHash(prev)
			entry.DeltaBytes = -prev.Size
			entry.Mode = prev.Mode
			out = append(out, entry)
		case !hadPrev && hasNext:
			entry := template
			entry.Path = path
			entry.Op = createOpFor(next.Type)
			entry.ContentHash = entryHash(next)
			entry.SizeBytes = next.Size
			entry.DeltaBytes = next.Size
			entry.Mode = next.Mode
			out = append(out, entry)
		case hadPrev && hasNext:
			if manifestEntryEquivalent(prev, next) {
				continue
			}
			entry := template
			entry.Path = path
			entry.PrevHash = entryHash(prev)
			entry.ContentHash = entryHash(next)
			entry.SizeBytes = next.Size
			entry.DeltaBytes = next.Size - prev.Size
			entry.Mode = next.Mode
			if entryContentEquivalent(prev, next) {
				entry.Op = ChangeOpChmod
			} else {
				entry.Op = modifyOpFor(next.Type)
			}
			out = append(out, entry)
		}
	}
	return out
}

// manifestSeedEntries emits one create-style entry per path in the manifest.
// Used by import paths to populate the changelog from day one.
func manifestSeedEntries(manifest Manifest, template ChangeEntry) []ChangeEntry {
	return manifestDiff(Manifest{}, manifest, template)
}

func annotateChangeEntriesWithVersions(entries []ChangeEntry, versionsByPath map[string]*FileVersion) []ChangeEntry {
	if len(entries) == 0 || len(versionsByPath) == 0 {
		return entries
	}
	for i := range entries {
		version := versionsByPath[entries[i].Path]
		if version == nil {
			continue
		}
		entries[i].FileID = version.FileID
		entries[i].VersionID = version.VersionID
	}
	return entries
}

func createOpFor(entryType string) string {
	switch entryType {
	case "dir":
		return ChangeOpMkdir
	case "symlink":
		return ChangeOpSymlink
	default:
		return ChangeOpPut
	}
}

func modifyOpFor(entryType string) string {
	switch entryType {
	case "dir":
		return ChangeOpMkdir
	case "symlink":
		return ChangeOpSymlink
	default:
		return ChangeOpPut
	}
}

func deleteOpFor(entryType string) string {
	if entryType == "dir" {
		return ChangeOpRmdir
	}
	return ChangeOpDelete
}

// entryHash returns a stable string identity for a manifest entry's content.
// Prefers blob ID, falls back to target (symlink) or inline content marker.
func entryHash(e ManifestEntry) string {
	if e.BlobID != "" {
		return e.BlobID
	}
	if e.Target != "" {
		return "symlink:" + e.Target
	}
	if e.Inline != "" {
		return "inline:" + e.Inline
	}
	return ""
}

func entryContentEquivalent(a, b ManifestEntry) bool {
	return a.Type == b.Type && a.Size == b.Size && a.BlobID == b.BlobID && a.Inline == b.Inline && a.Target == b.Target
}

// Key builders.

func changelogStreamKey(workspace string) string {
	return fmt.Sprintf("afs:{%s}:workspace:changelog", workspace)
}

func sessionSummaryKey(workspace, sessionID string) string {
	return fmt.Sprintf("afs:{%s}:session:%s:summary", workspace, sessionID)
}

func pathLastKey(workspace, path string) string {
	return fmt.Sprintf("afs:{%s}:path:last:%s", workspace, strings.TrimPrefix(path, "/"))
}

// Exported for tests and internal readers.
func ChangelogStreamKey(workspace string) string { return changelogStreamKey(workspace) }
func SessionSummaryKey(workspace, sessionID string) string {
	return sessionSummaryKey(workspace, sessionID)
}
func PathLastKey(workspace, path string) string { return pathLastKey(workspace, path) }

// Session identity plumbed via context rather than function signatures — the
// call chain from HTTP handler → DatabaseManager → Service is too wide to
// thread a SessionID param through cleanly.

type changeSessionContextKey struct{}

// ChangeSessionContext carries the session-identity bits used to tag
// changelog entries emitted by the current request.
type ChangeSessionContext struct {
	SessionID string
}

// WithChangeSessionContext returns a new context carrying the given session
// context. HTTP handlers attach this after parsing the X-AFS-Session-Id
// header; service-layer code reads it at apply time.
func WithChangeSessionContext(ctx context.Context, sc ChangeSessionContext) context.Context {
	if sc.SessionID == "" {
		return ctx
	}
	return context.WithValue(ctx, changeSessionContextKey{}, sc)
}

// ChangeSessionContextFromContext extracts the session context if one was
// attached. Returns a zero value and false otherwise.
func ChangeSessionContextFromContext(ctx context.Context) (ChangeSessionContext, bool) {
	if ctx == nil {
		return ChangeSessionContext{}, false
	}
	sc, ok := ctx.Value(changeSessionContextKey{}).(ChangeSessionContext)
	return sc, ok
}

// SessionIDHeader is the HTTP header carrying the current session identity
// from the agent CLI to the control plane.
const SessionIDHeader = "X-AFS-Session-Id"

// AgentIDHeader is the optional HTTP header carrying a stable caller identity
// for hosted or custom agent clients that cannot persist CLI config.
const AgentIDHeader = "X-AFS-Agent-Id"

// ChangelogListRequest parameterizes a changelog read. All fields optional.
type ChangelogListRequest struct {
	SessionID string // if set, entries are filtered to this session
	Path      string // if set, entries are filtered to this path or prev_path
	Since     string // entry ID to read after (exclusive) — start of the range
	Until     string // entry ID to read up to (exclusive) — end of the range
	Limit     int    // hard cap on entries returned; default 100, max 1000
	Reverse   bool   // if true, read newest-first via XREVRANGE
}

// ChangelogListResponse wraps a page of changelog entries.
type ChangelogListResponse struct {
	Entries    []ChangelogEntryRow `json:"entries"`
	NextCursor string              `json:"next_cursor,omitempty"`
}

// ChangelogEntryRow is the wire shape of one stream entry. Fields mirror the
// ChangeEntry with the addition of an ID (Redis stream entry ID) and
// occurred_at timestamp parsed from the ts_ms field.
type ChangelogEntryRow struct {
	ID            string `json:"id"`
	OccurredAt    string `json:"occurred_at,omitempty"`
	WorkspaceID   string `json:"workspace_id,omitempty"`
	WorkspaceName string `json:"workspace_name,omitempty"`
	DatabaseID    string `json:"database_id,omitempty"`
	DatabaseName  string `json:"database_name,omitempty"`
	SessionID     string `json:"session_id,omitempty"`
	AgentID       string `json:"agent_id,omitempty"`
	User          string `json:"user,omitempty"`
	Label         string `json:"label,omitempty"`
	AgentVersion  string `json:"agent_version,omitempty"`
	Op            string `json:"op"`
	Path          string `json:"path"`
	PrevPath      string `json:"prev_path,omitempty"`
	SizeBytes     int64  `json:"size_bytes,omitempty"`
	DeltaBytes    int64  `json:"delta_bytes,omitempty"`
	ContentHash   string `json:"content_hash,omitempty"`
	PrevHash      string `json:"prev_hash,omitempty"`
	Mode          uint32 `json:"mode,omitempty"`
	FileID        string `json:"file_id,omitempty"`
	VersionID     string `json:"version_id,omitempty"`
	CheckpointID  string `json:"checkpoint_id,omitempty"`
	Source        string `json:"source,omitempty"`
}

func rowFromStreamMessage(msg redis.XMessage) ChangelogEntryRow {
	row := ChangelogEntryRow{ID: msg.ID}
	getField := func(key string) string {
		if v, ok := msg.Values[key]; ok {
			return fmt.Sprint(v)
		}
		return ""
	}
	if raw := getField("ts_ms"); raw != "" {
		if ms, err := strconv.ParseInt(raw, 10, 64); err == nil {
			row.OccurredAt = time.UnixMilli(ms).UTC().Format(time.RFC3339)
		}
	}
	row.SessionID = getField("session_id")
	row.AgentID = getField("agent_id")
	row.User = getField("user")
	row.Label = getField("label")
	row.AgentVersion = getField("agent_version")
	row.Op = getField("op")
	row.Path = getField("path")
	row.PrevPath = getField("prev_path")
	row.FileID = getField("file_id")
	row.VersionID = getField("version_id")
	if raw := getField("size_bytes"); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
			row.SizeBytes = n
		}
	}
	if raw := getField("delta_bytes"); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
			row.DeltaBytes = n
		}
	}
	row.ContentHash = getField("content_hash")
	row.PrevHash = getField("prev_hash")
	if raw := getField("mode"); raw != "" {
		if n, err := strconv.ParseUint(raw, 10, 32); err == nil {
			row.Mode = uint32(n)
		}
	}
	row.CheckpointID = getField("checkpoint_id")
	row.Source = getField("source")
	return row
}

// ListChangelog reads a page of entries from a workspace's changelog stream.
// Optional session-ID filter is applied in-memory; for well-bounded session
// reads this is fine since sessions are short-lived.
func (s *Store) ListChangelog(ctx context.Context, storageID string, req ChangelogListRequest) (ChangelogListResponse, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	stream := changelogStreamKey(storageID)
	// Since/Until are exclusive cursors (the "(" prefix tells Redis streams to
	// exclude the given ID). Using exclusive bounds means the cursor returned
	// in NextCursor can be passed back verbatim for the next page without
	// re-emitting the row it points at.
	start := "-"
	end := "+"
	if req.Since != "" {
		start = "(" + req.Since
	}
	if req.Until != "" {
		end = "(" + req.Until
	}
	// Over-fetch when filtering so we have enough rows after in-memory filters.
	fetch := int64(limit)
	if req.SessionID != "" || req.Path != "" {
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
		msgs, err = s.rdb.XRevRangeN(ctx, stream, end, start, fetch).Result()
	} else {
		msgs, err = s.rdb.XRangeN(ctx, stream, start, end, fetch).Result()
	}
	if err != nil {
		return ChangelogListResponse{}, err
	}
	entries := make([]ChangelogEntryRow, 0, len(msgs))
	for _, m := range msgs {
		row := rowFromStreamMessage(m)
		if req.SessionID != "" && row.SessionID != req.SessionID {
			continue
		}
		if req.Path != "" && row.Path != req.Path && row.PrevPath != req.Path {
			continue
		}
		entries = append(entries, row)
		if len(entries) >= limit {
			break
		}
	}
	resp := ChangelogListResponse{Entries: entries}
	if len(entries) > 0 {
		resp.NextCursor = entries[len(entries)-1].ID
	}
	return resp, nil
}

// SessionChangelogSummary is the HGETALL result from a session summary hash,
// decoded into the fields the UI actually needs.
type SessionChangelogSummary struct {
	SessionID  string         `json:"session_id"`
	OpCounts   map[string]int `json:"op_counts"`
	DeltaBytes int64          `json:"delta_bytes"`
	LastOpAt   string         `json:"last_op_at,omitempty"`
}

// GetSessionChangelogSummary reads the per-session rollup hash.
func (s *Store) GetSessionChangelogSummary(ctx context.Context, storageID, sessionID string) (SessionChangelogSummary, error) {
	out := SessionChangelogSummary{SessionID: sessionID, OpCounts: map[string]int{}}
	values, err := s.rdb.HGetAll(ctx, sessionSummaryKey(storageID, sessionID)).Result()
	if err != nil {
		return out, err
	}
	for k, v := range values {
		switch {
		case strings.HasPrefix(k, "op_"):
			if n, err := strconv.Atoi(v); err == nil {
				out.OpCounts[strings.TrimPrefix(k, "op_")] = n
			}
		case k == "delta_bytes":
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				out.DeltaBytes = n
			}
		case k == "last_op_at_ms":
			if ms, err := strconv.ParseInt(v, 10, 64); err == nil {
				out.LastOpAt = time.UnixMilli(ms).UTC().Format(time.RFC3339)
			}
		}
	}
	return out, nil
}

// PathLastWriter is the HGETALL result from a path:last hash.
type PathLastWriter struct {
	Path        string `json:"path"`
	SessionID   string `json:"session_id,omitempty"`
	Op          string `json:"op,omitempty"`
	ContentHash string `json:"content_hash,omitempty"`
	OccurredAt  string `json:"occurred_at,omitempty"`
}

// GetPathLastWriter reads the companion path:last hash for a single path.
func (s *Store) GetPathLastWriter(ctx context.Context, storageID, path string) (PathLastWriter, error) {
	out := PathLastWriter{Path: path}
	values, err := s.rdb.HGetAll(ctx, pathLastKey(storageID, path)).Result()
	if err != nil {
		return out, err
	}
	out.SessionID = values["session_id"]
	out.Op = values["op"]
	out.ContentHash = values["content_hash"]
	if raw := values["occurred_ms"]; raw != "" {
		if ms, err := strconv.ParseInt(raw, 10, 64); err == nil {
			out.OccurredAt = time.UnixMilli(ms).UTC().Format(time.RFC3339)
		}
	}
	return out, nil
}

// buildChangelogTemplate assembles the common fields attached to every entry
// emitted by the current request: session identity (from context + session
// record lookup) and authenticated user (from the auth identity).
// checkpointID and source are supplied per call site.
//
// Failure to resolve the session record is non-fatal — we emit with whatever
// we have. The session record may be absent if the agent is running without
// an up session (e.g. one-shot imports) or if the session has already closed.
func (s *Service) buildChangelogTemplate(ctx context.Context, storageID, checkpointID, source string) ChangeEntry {
	template := ChangeEntry{
		Source:       source,
		CheckpointID: checkpointID,
	}
	if identity, ok := AuthIdentityFromContext(ctx); ok {
		template.User = identity.Subject
	}
	sc, ok := ChangeSessionContextFromContext(ctx)
	if !ok || sc.SessionID == "" {
		return template
	}
	template.SessionID = sc.SessionID
	if s == nil || s.store == nil {
		return template
	}
	record, err := s.store.GetWorkspaceSession(ctx, storageID, sc.SessionID)
	if err != nil {
		// Stale/closed session — still tag with SessionID so the stream row
		// is attributable; just miss the richer label/version fields.
		return template
	}
	template.AgentID = record.AgentID
	template.Label = record.Label
	template.AgentVersion = record.AFSVersion
	return template
}
