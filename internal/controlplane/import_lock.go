package controlplane

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrImportInProgress is returned when a workspace is currently being imported
// and another operation (mount, fork, checkpoint, create) attempts to touch it.
var ErrImportInProgress = errors.New("workspace import in progress")

const (
	importLockTTL       = 30 * time.Second
	importLockHeartbeat = 10 * time.Second
)

// importLockReleaseScript deletes the key only if its value matches the caller's
// token. Prevents releasing a lock that has expired and been reacquired by
// someone else.
var importLockReleaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`)

// importLockRefreshScript extends the TTL only if the caller still holds the
// lock. Ensures a stale heartbeat doesn't resurrect a lock the holder lost.
var importLockRefreshScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return 0
`)

// ImportLock represents an acquired workspace import lock. It runs a background
// heartbeat until Release is called.
type ImportLock struct {
	rdb       *redis.Client
	key       string
	token     string
	workspace string

	cancel   context.CancelFunc
	done     chan struct{}
	onceStop sync.Once
	lostMu   sync.Mutex
	lost     error
}

// ImportLockKey returns the Redis key used for the workspace import lock.
func ImportLockKey(workspace string) string {
	return fmt.Sprintf("afs:{%s}:import_lock", workspace)
}

// AcquireImportLock acquires the per-workspace import lock. If another
// import is already in progress, it returns ErrImportInProgress.
func AcquireImportLock(ctx context.Context, store *Store, workspace string) (*ImportLock, error) {
	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(tokenBytes)

	lockWorkspace, err := store.resolveWorkspaceStorageOrRaw(ctx, workspace)
	if err != nil {
		return nil, err
	}
	key := ImportLockKey(lockWorkspace)
	ok, err := store.rdb.SetNX(ctx, key, token, importLockTTL).Result()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrImportInProgress
	}

	hbCtx, cancel := context.WithCancel(context.Background())
	lock := &ImportLock{
		rdb:       store.rdb,
		key:       key,
		token:     token,
		workspace: lockWorkspace,
		cancel:    cancel,
		done:      make(chan struct{}),
	}
	go lock.heartbeat(hbCtx)
	return lock, nil
}

// CheckImportLock returns ErrImportInProgress if the workspace currently has an
// active import lock. It is a read-only EXISTS check.
func CheckImportLock(ctx context.Context, store *Store, workspace string) error {
	lockWorkspace, err := store.resolveWorkspaceStorageOrRaw(ctx, workspace)
	if err != nil {
		return err
	}
	count, err := store.rdb.Exists(ctx, ImportLockKey(lockWorkspace)).Result()
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrImportInProgress
	}
	return nil
}

// Release cancels the heartbeat and deletes the lock key (only if still held).
// Safe to call multiple times.
func (l *ImportLock) Release(ctx context.Context) error {
	if l == nil {
		return nil
	}
	var releaseErr error
	l.onceStop.Do(func() {
		l.cancel()
		<-l.done
		_, err := importLockReleaseScript.Run(ctx, l.rdb, []string{l.key}, l.token).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			releaseErr = err
		}
	})
	return releaseErr
}

// Lost reports whether the heartbeat has observed that the lock was lost
// (e.g., Redis evicted the key or another process seized it). Callers should
// treat a lost lock as fatal to the current import.
func (l *ImportLock) Lost() error {
	if l == nil {
		return nil
	}
	l.lostMu.Lock()
	defer l.lostMu.Unlock()
	return l.lost
}

// Token returns the opaque value written to the lock key. Exposed for tests.
func (l *ImportLock) Token() string {
	if l == nil {
		return ""
	}
	return l.token
}

func (l *ImportLock) heartbeat(ctx context.Context) {
	defer close(l.done)
	ticker := time.NewTicker(importLockHeartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refreshCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			res, err := importLockRefreshScript.Run(refreshCtx, l.rdb, []string{l.key}, l.token, int64(importLockTTL/time.Millisecond)).Result()
			cancel()
			if err != nil && !errors.Is(err, redis.Nil) {
				l.markLost(fmt.Errorf("import lock heartbeat: %w", err))
				return
			}
			if n, ok := res.(int64); ok && n == 0 {
				l.markLost(fmt.Errorf("import lock for workspace %q was lost", l.workspace))
				return
			}
		}
	}
}

func (l *ImportLock) markLost(err error) {
	l.lostMu.Lock()
	defer l.lostMu.Unlock()
	if l.lost == nil {
		l.lost = err
	}
}
