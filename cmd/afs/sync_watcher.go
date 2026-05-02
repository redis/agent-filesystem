package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// LocalEvent is what the watcher emits onto its output channel. The watcher
// does not read content or hash; that work belongs to the reconciler when it
// is ready to act. KindHint is just a debugging hint about what fsnotify saw
// most recently for the path.
type LocalEvent struct {
	Path     string // workspace-relative POSIX path (no leading slash)
	KindHint string // "create" | "write" | "remove" | "rename" | "chmod"
}

// syncWatcher wraps fsnotify with: recursive directory bookkeeping (one watch
// per dir), per-path debouncing/coalescing, and a baseline ignore filter so
// .DS_Store et al. never reach the reconciler.
//
// Lifecycle: the output channel is never closed. Consumers must use ctx
// cancellation to stop reading. Closing would race with in-flight debounce
// timer goroutines that have already passed the stopped check.
type syncWatcher struct {
	root     string
	ignore   *syncIgnore
	debounce time.Duration

	w *fsnotify.Watcher

	mu      sync.Mutex
	pending map[string]*pendingEvent
	dirs    map[string]struct{} // absolute paths currently watched
	stopped bool                // protected by mu; flush is a no-op once true

	out chan LocalEvent
}

type pendingEvent struct {
	kindHint string
	timer    *time.Timer
}

// newSyncWatcher creates and starts a watcher rooted at the given absolute
// directory. It installs watches on every existing subdirectory before
// returning, so the caller can immediately walk the tree without missing
// in-flight events. The returned channel emits one LocalEvent per coalesced
// event after the debounce window expires.
func newSyncWatcher(root string, ignore *syncIgnore, debounce time.Duration) (*syncWatcher, error) {
	if debounce <= 0 {
		debounce = 100 * time.Millisecond
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("fsnotify: %w", err)
	}
	sw := &syncWatcher{
		root:     filepath.Clean(root),
		ignore:   ignore,
		debounce: debounce,
		w:        w,
		pending:  make(map[string]*pendingEvent),
		dirs:     make(map[string]struct{}),
		out:      make(chan LocalEvent, 1024),
	}
	if err := sw.addRecursive(sw.root); err != nil {
		_ = w.Close()
		return nil, err
	}
	return sw, nil
}

// Events returns the read-only channel of coalesced events. It stays open
// until run returns.
func (s *syncWatcher) Events() <-chan LocalEvent {
	return s.out
}

// Close stops the underlying fsnotify watcher and cancels any pending timers.
// Safe to call more than once.
func (s *syncWatcher) Close() error {
	s.mu.Lock()
	s.stopped = true
	for _, p := range s.pending {
		if p.timer != nil {
			p.timer.Stop()
		}
	}
	s.pending = nil
	s.mu.Unlock()
	if s.w != nil {
		return s.w.Close()
	}
	return nil
}

func (s *syncWatcher) resetRecursive(root string) error {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return nil
	}
	s.dirs = make(map[string]struct{})
	s.mu.Unlock()
	return s.addRecursive(root)
}

// run pumps fsnotify events until the underlying channels close or ctx is
// cancelled. Errors are logged to stderr; recoverable failures (e.g. a
// directory disappearing) are ignored.
//
// The output channel is intentionally NOT closed when run returns. Closing
// would race with in-flight debounce timer goroutines whose flush() calls
// could send on a now-closed channel. Consumers detect shutdown via ctx.
func (s *syncWatcher) run(ctx context.Context) {
	defer func() {
		s.mu.Lock()
		s.stopped = true
		for _, p := range s.pending {
			if p.timer != nil {
				p.timer.Stop()
			}
		}
		s.pending = nil
		s.mu.Unlock()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-s.w.Events:
			if !ok {
				return
			}
			s.handleEvent(ev)
		case err, ok := <-s.w.Errors:
			if !ok {
				return
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "afs sync: watcher error: %v\n", err)
			}
		}
	}
}

func (s *syncWatcher) handleEvent(ev fsnotify.Event) {
	abs, err := filepath.Abs(ev.Name)
	if err != nil {
		return
	}
	rel, err := filepath.Rel(s.root, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return
	}
	rel = filepath.ToSlash(rel)
	isDir := false
	info, statErr := os.Lstat(abs)
	if statErr == nil {
		isDir = info.IsDir()
	}

	// Adjust dir watches when directories appear or disappear.
	if isDir && ev.Op&fsnotify.Create != 0 {
		// New directory: walk it and install watches plus emit synthetic
		// events for any pre-existing children, in case the user dropped a
		// populated tree under the watch root.
		_ = s.addRecursive(abs)
		_ = filepath.WalkDir(abs, func(p string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if p == abs {
				return nil
			}
			rel2, err := filepath.Rel(s.root, p)
			if err != nil {
				return nil
			}
			rel2 = filepath.ToSlash(rel2)
			if s.ignore.shouldIgnore(rel2, d.IsDir()) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			s.coalesce(rel2, "create")
			return nil
		})
	}
	if ev.Op&fsnotify.Remove != 0 || ev.Op&fsnotify.Rename != 0 {
		s.removeWatch(abs)
	}

	if s.ignore.shouldIgnore(rel, isDir) {
		return
	}

	kind := opName(ev.Op)
	s.coalesce(rel, kind)
}

// coalesce starts (or extends) a debounce timer for rel. When the timer
// fires, the path is emitted on the output channel exactly once.
func (s *syncWatcher) coalesce(rel, kind string) {
	s.mu.Lock()
	if s.stopped || s.pending == nil {
		s.mu.Unlock()
		return
	}
	if existing, ok := s.pending[rel]; ok {
		if existing.timer != nil {
			existing.timer.Stop()
		}
		existing.kindHint = kind
		existing.timer = time.AfterFunc(s.debounce, func() {
			s.flush(rel)
		})
		s.mu.Unlock()
		return
	}
	pe := &pendingEvent{kindHint: kind}
	pe.timer = time.AfterFunc(s.debounce, func() {
		s.flush(rel)
	})
	s.pending[rel] = pe
	s.mu.Unlock()
}

func (s *syncWatcher) flush(rel string) {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	pe, ok := s.pending[rel]
	if !ok {
		s.mu.Unlock()
		return
	}
	delete(s.pending, rel)
	out := s.out
	s.mu.Unlock()
	if out == nil {
		return
	}
	select {
	case out <- LocalEvent{Path: rel, KindHint: pe.kindHint}:
	default:
		// Output channel is saturated; drop this event but log so we can
		// catch it in tests. The reconciler does a full sweep on backlog
		// recovery so a dropped event is recoverable, just expensive.
		fmt.Fprintf(os.Stderr, "afs sync: watcher backpressure, dropped %s\n", rel)
	}
}

// addRecursive walks a directory and installs an fsnotify watch on every
// subdirectory found. Errors on individual subdirs are logged but do not
// abort the walk.
func (s *syncWatcher) addRecursive(root string) error {
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if !d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(s.root, p)
		if relErr == nil {
			rel = filepath.ToSlash(rel)
			if s.ignore.shouldIgnore(rel, true) {
				return filepath.SkipDir
			}
		}
		s.mu.Lock()
		if _, already := s.dirs[p]; already {
			s.mu.Unlock()
			return nil
		}
		s.mu.Unlock()
		if err := s.w.Add(p); err != nil {
			fmt.Fprintf(os.Stderr, "afs sync: cannot watch %s: %v\n", p, err)
			return nil
		}
		s.mu.Lock()
		s.dirs[p] = struct{}{}
		s.mu.Unlock()
		return nil
	})
}

// removeWatch drops the fsnotify watch for an absolute directory path. We
// also drop any descendant watches we know about because Linux fsnotify
// auto-removes them but darwin/kqueue may not.
func (s *syncWatcher) removeWatch(abs string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.dirs[abs]; ok {
		_ = s.w.Remove(abs)
		delete(s.dirs, abs)
	}
	prefix := abs + string(filepath.Separator)
	for d := range s.dirs {
		if strings.HasPrefix(d, prefix) {
			_ = s.w.Remove(d)
			delete(s.dirs, d)
		}
	}
}

func opName(op fsnotify.Op) string {
	switch {
	case op&fsnotify.Create != 0:
		return "create"
	case op&fsnotify.Write != 0:
		return "write"
	case op&fsnotify.Remove != 0:
		return "remove"
	case op&fsnotify.Rename != 0:
		return "rename"
	case op&fsnotify.Chmod != 0:
		return "chmod"
	default:
		return "unknown"
	}
}
