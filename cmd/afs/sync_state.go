package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// syncStateVersion is bumped whenever the on-disk SyncState format changes in
// an incompatible way. v2 adds ChunkSize/ChunkHashes to SyncEntry for
// chunk-level delta sync. v1 entries have zero-value ChunkSize (= inline).
const syncStateVersion = 3

// SyncEntry is the per-path record the reconciler maintains. It records what
// the daemon last knew about both sides — local hash/mtime and the corresponding
// remote (Redis live root) hash/mtime — so subsequent events can be classified
// as no-op (matches stored hash), upload (local diverged from stored), download
// (remote diverged from stored), or conflict (both sides moved off stored).
type SyncEntry struct {
	Type          string    `json:"type"` // "file" | "dir" | "symlink"
	Mode          uint32    `json:"mode"`
	Size          int64     `json:"size"`
	LocalHash     string    `json:"local_hash,omitempty"`
	LocalMtimeMs  int64     `json:"local_mtime_ms"`
	RemoteHash    string    `json:"remote_hash,omitempty"`
	RemoteMtimeMs int64     `json:"remote_mtime_ms"`
	Target        string    `json:"target,omitempty"`
	LastSyncedAt  time.Time `json:"last_synced_at"`
	// Chunked sync fields (v2). ChunkSize==0 means inline (not chunked).
	ChunkSize   int      `json:"chunk_size,omitempty"`
	ChunkHashes []string `json:"chunk_hashes,omitempty"`
	// Version counter (v3) — monotonically increasing per state write.
	Version uint64 `json:"version"`
	// Deleted marks the entry as a tombstone (v3). The entry is kept so
	// buildPlan can distinguish "intentionally deleted" from "never seen".
	Deleted bool `json:"deleted,omitempty"`
}

// SyncState is the persisted view of every path the daemon has ever observed
// for a workspace. Keys are workspace-relative POSIX paths (no leading slash,
// no "."). Directories are recorded so a missing entry on disk during the next
// reconcile loop unambiguously means "deleted locally" rather than "never seen".
type SyncState struct {
	Version      int                  `json:"version"`
	Workspace    string               `json:"workspace"`
	LocalPath    string               `json:"local_path"`
	LastStreamID string               `json:"last_stream_id,omitempty"`
	Entries      map[string]SyncEntry `json:"entries"`
	UpdatedAt    time.Time            `json:"updated_at"`
	// NextVersion is the monotonic counter used by nextVersion().
	NextVersion uint64 `json:"next_version"`
}

// newSyncState returns an empty state ready to be populated by a reconciler
// pass.
func newSyncState(workspace, localPath string) *SyncState {
	return &SyncState{
		Version:   syncStateVersion,
		Workspace: workspace,
		LocalPath: localPath,
		Entries:   make(map[string]SyncEntry),
	}
}

func syncStateEntryCounts(st *SyncState) (live, deleted int) {
	if st == nil {
		return 0, 0
	}
	for _, entry := range st.Entries {
		if entry.Deleted {
			deleted++
			continue
		}
		live++
	}
	return live, deleted
}

// syncStateDir returns the directory holding sync state files for the current
// process (one JSON file per workspace).
func syncStateDir() string {
	return filepath.Join(stateDir(), "sync")
}

// syncStatePath returns the JSON file path for one workspace.
func syncStatePath(workspace string) string {
	return filepath.Join(syncStateDir(), workspace+".json")
}

// loadSyncState reads the persisted state for a workspace from disk. A
// missing file is reported as os.ErrNotExist so callers can distinguish "first
// run" from "broken file".
func loadSyncState(workspace string) (*SyncState, error) {
	raw, err := os.ReadFile(syncStatePath(workspace))
	if err != nil {
		return nil, err
	}
	var st SyncState
	if err := json.Unmarshal(raw, &st); err != nil {
		return nil, fmt.Errorf("parse sync state %s: %w", syncStatePath(workspace), err)
	}
	if st.Entries == nil {
		st.Entries = make(map[string]SyncEntry)
	}
	if st.Workspace == "" {
		st.Workspace = workspace
	}
	return &st, nil
}

// saveSyncState atomically writes the state to disk via tmp+rename so a crash
// mid-write cannot leave a half-written file behind.
func saveSyncState(st *SyncState) error {
	if st == nil {
		return errors.New("saveSyncState: nil state")
	}
	if err := os.MkdirAll(syncStateDir(), 0o700); err != nil {
		return err
	}
	st.UpdatedAt = time.Now().UTC()
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	target := syncStatePath(st.Workspace)
	tmp, err := os.CreateTemp(syncStateDir(), "."+st.Workspace+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, target)
}

// removeSyncState deletes the persisted state file for explicit unmount/delete
// cleanup.
func removeSyncState(workspace string) error {
	err := os.Remove(syncStatePath(workspace))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// stateWriter wraps a SyncState with a sync.Mutex and a debounced writer
// goroutine. All reconciler mutations go through this so we never lose updates
// from concurrent goroutines and never block the hot path on disk I/O.
type stateWriter struct {
	mu       sync.Mutex
	state    *SyncState
	dirty    bool
	debounce time.Duration

	flushCh chan struct{}
	doneCh  chan struct{}
}

func newStateWriter(st *SyncState, debounce time.Duration) *stateWriter {
	if debounce <= 0 {
		debounce = time.Second
	}
	return &stateWriter{
		state:    st,
		debounce: debounce,
		flushCh:  make(chan struct{}, 1),
		doneCh:   make(chan struct{}),
	}
}

// nextVersion returns the next monotonic version and increments the counter.
// Must be called with w.mu held.
func (w *stateWriter) nextVersion() uint64 {
	v := w.state.NextVersion
	w.state.NextVersion++
	return v
}

// run loops until ctx is done, persisting the latest state at most once per
// debounce window after a markDirty call.
func (w *stateWriter) run(stop <-chan struct{}) {
	defer close(w.doneCh)
	for {
		select {
		case <-stop:
			w.flushNow()
			return
		case <-w.flushCh:
			timer := time.NewTimer(w.debounce)
			select {
			case <-stop:
				timer.Stop()
				w.flushNow()
				return
			case <-timer.C:
				w.flushNow()
			}
		}
	}
}

func (w *stateWriter) markDirty() {
	w.mu.Lock()
	w.dirty = true
	w.mu.Unlock()
	select {
	case w.flushCh <- struct{}{}:
	default:
	}
}

func (w *stateWriter) flushNow() {
	w.mu.Lock()
	if !w.dirty {
		w.mu.Unlock()
		return
	}
	clone := cloneSyncState(w.state)
	w.dirty = false
	w.mu.Unlock()
	if err := saveSyncState(clone); err != nil {
		// Log via stderr; the daemon will retry on next markDirty.
		fmt.Fprintf(os.Stderr, "afs sync: failed to persist state for %s: %v\n", clone.Workspace, err)
	}
}

// updateStreamID persists the latest Redis Stream cursor so the next
// reconnect or restart can resume from this position.
func (w *stateWriter) updateStreamID(id string) {
	w.mu.Lock()
	w.state.LastStreamID = id
	w.dirty = true
	w.mu.Unlock()
	w.markDirty()
}

// lastStreamID returns the persisted stream cursor.
func (w *stateWriter) lastStreamID() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.state.LastStreamID
}

// snapshot returns a defensive copy of the current state. Used by tests and
// the status command.
func (w *stateWriter) snapshot() *SyncState {
	w.mu.Lock()
	defer w.mu.Unlock()
	return cloneSyncState(w.state)
}

func cloneSyncState(st *SyncState) *SyncState {
	if st == nil {
		return nil
	}
	out := *st
	out.Entries = make(map[string]SyncEntry, len(st.Entries))
	for k, v := range st.Entries {
		out.Entries[k] = v
	}
	return &out
}
