package main

import (
	"fmt"
	"strings"
)

// Mode constants determine the local runtime kind.
const (
	modeMount = "mount"
	modeSync  = "sync"
	modeNone  = "none"
)

// defaultSyncFileSizeCapMB is raised from 64 to 2048 (2 GB) now that
// chunked delta sync avoids loading entire files into memory.
const defaultSyncFileSizeCapMB = 2048

// effectiveMode returns the resolved Mode for the daemon. Empty resolves to
// sync. Any unrecognized value is reported as an error so the user notices a
// typo immediately rather than after a confusing fallback.
func effectiveMode(cfg config) (string, error) {
	m := strings.TrimSpace(cfg.Mode)
	switch m {
	case "":
		return modeSync, nil
	case modeMount, modeSync, modeNone:
		return m, nil
	default:
		return "", fmt.Errorf("unknown mode %q in config (expected one of: sync, mount, none)", cfg.Mode)
	}
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
