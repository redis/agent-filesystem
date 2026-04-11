package controlplane

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestStore(t *testing.T) (*Store, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = rdb.Close()
	})
	return NewStore(rdb), mr
}

func TestAcquireImportLockHoldsExclusively(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	first, err := AcquireImportLock(ctx, store, "demo")
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer first.Release(ctx)

	if _, err := AcquireImportLock(ctx, store, "demo"); !errors.Is(err, ErrImportInProgress) {
		t.Fatalf("second acquire: got %v, want ErrImportInProgress", err)
	}
	if err := CheckImportLock(ctx, store, "demo"); !errors.Is(err, ErrImportInProgress) {
		t.Fatalf("check while held: got %v, want ErrImportInProgress", err)
	}
}

func TestImportLockReleaseAllowsReAcquire(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	first, err := AcquireImportLock(ctx, store, "demo")
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if err := first.Release(ctx); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := CheckImportLock(ctx, store, "demo"); err != nil {
		t.Fatalf("check after release: %v", err)
	}
	second, err := AcquireImportLock(ctx, store, "demo")
	if err != nil {
		t.Fatalf("reacquire: %v", err)
	}
	_ = second.Release(ctx)
}

func TestImportLockReleaseDoesNotDeleteForeignToken(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	first, err := AcquireImportLock(ctx, store, "demo")
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	// Forge a different token on the same key.
	if err := store.rdb.Set(ctx, ImportLockKey("demo"), "other", 30*time.Second).Err(); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	if err := first.Release(ctx); err != nil {
		t.Fatalf("release: %v", err)
	}
	// Lock should still be present because the token didn't match.
	val, err := store.rdb.Get(ctx, ImportLockKey("demo")).Result()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if val != "other" {
		t.Fatalf("key value = %q, want %q", val, "other")
	}
}

func TestImportLockExpiresAfterTTL(t *testing.T) {
	store, mr := newTestStore(t)
	ctx := context.Background()

	first, err := AcquireImportLock(ctx, store, "demo")
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	// Stop the heartbeat so TTL can expire without being refreshed.
	_ = first.Release(ctx)
	if _, err := AcquireImportLock(ctx, store, "demo"); err != nil {
		t.Fatalf("reacquire: %v", err)
	}

	// Fast-forward past the TTL.
	mr.FastForward(importLockTTL + time.Second)
	if err := CheckImportLock(ctx, store, "demo"); err != nil {
		t.Fatalf("expected lock to expire, got err: %v", err)
	}
}
