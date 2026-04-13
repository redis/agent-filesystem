package main

import (
	"fmt"
	"strings"
)

// Mode constants determine what `afs up` actually starts. The legacy default
// is "mount" — empty Mode is treated identically so that existing
// `afs.config.json` files keep their pre-sync behavior.
const (
	modeMount = "mount"
	modeSync  = "sync"
	modeNone  = "none"
)

// defaultSyncFileSizeCapMB is the per-file ceiling enforced by the sync
// uploader. The Redis backend stores file content as a single hash field with
// an implicit 512 MB cap; we set the default low enough that users get a
// clean error before they hit the wall.
// defaultSyncFileSizeCapMB is raised from 64 to 2048 (2 GB) now that
// chunked delta sync avoids loading entire files into memory.
const defaultSyncFileSizeCapMB = 2048

// effectiveMode returns the resolved Mode for the daemon. Empty resolves to
// sync (the recommended default). A pre-existing config that does not set
// Mode but DOES configure a mountpoint and backend is still honored as
// mount, so users who installed AFS before sync mode existed keep their
// mount-based setup on their next `afs up`.
//
// Any unrecognized value is reported as an error so the user notices a typo
// immediately rather than after a confusing fallback.
func effectiveMode(cfg config) (string, error) {
	m := strings.TrimSpace(cfg.Mode)
	switch m {
	case "":
		// Legacy heuristic: a config that was clearly built for mount mode
		// (explicit backend + mountpoint, no sync path) should keep mounting.
		// Anything else — including brand-new configs — defaults to sync.
		if isLegacyMountConfig(cfg) {
			return modeMount, nil
		}
		return modeSync, nil
	case modeMount, modeSync, modeNone:
		return m, nil
	default:
		return "", fmt.Errorf("unknown mode %q in config (expected one of: sync, mount, none)", cfg.Mode)
	}
}

// isLegacyMountConfig detects configs that were written before sync mode
// existed and explicitly wanted a FUSE/NFS mount. We err on the side of NOT
// migrating those users silently — flipping a live mount to sync mid-session
// could cause surprising writes.
func isLegacyMountConfig(cfg config) bool {
	if strings.TrimSpace(cfg.SyncLocalPath) != "" {
		return false
	}
	backend := strings.TrimSpace(cfg.MountBackend)
	if backend == "" || backend == mountBackendNone {
		return false
	}
	if strings.TrimSpace(cfg.Mountpoint) == "" {
		return false
	}
	return true
}

// syncSizeCapBytes returns the configured per-file size cap in bytes, falling
// back to the default if unset or non-positive.
func syncSizeCapBytes(cfg config) int64 {
	mb := cfg.SyncFileSizeCapMB
	if mb <= 0 {
		mb = defaultSyncFileSizeCapMB
	}
	return int64(mb) * 1024 * 1024
}
