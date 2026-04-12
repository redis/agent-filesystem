package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// syncLogger provides structured, colored log output for the sync daemon.
// In background mode (verbose=false) only errors are emitted. In interactive
// mode (verbose=true) every filesystem event is logged with direction arrows
// and color coding so the user can watch the sync in real time.
type syncLogger struct {
	verbose bool
}

func newSyncLogger(verbose bool) *syncLogger {
	return &syncLogger{verbose: verbose}
}

// syncLog direction indicators
const (
	logUpload   = "↑" // local → remote
	logDownload = "↓" // remote → local
	logLocal    = "●" // local event detected
	logConflict = "⚡"
	logSkip     = "·"
	logError    = "✗"
)

// Upload logs a successful local→remote push.
func (l *syncLogger) Upload(path string) {
	if !l.verbose {
		return
	}
	l.emit(ansiGreen, logUpload, "uploaded", path)
}

// Download logs a successful remote→local pull.
func (l *syncLogger) Download(path string) {
	if !l.verbose {
		return
	}
	l.emit(ansiCyan, logDownload, "downloaded", path)
}

// LocalChange logs detection of a local filesystem change.
func (l *syncLogger) LocalChange(path, kind string) {
	if !l.verbose {
		return
	}
	l.emit(ansiYellow, logLocal, kind, path)
}

// RemoteChange logs detection of a remote change via pub/sub.
func (l *syncLogger) RemoteChange(path, op string) {
	if !l.verbose {
		return
	}
	l.emit(ansiCyan, logDownload, "remote "+op, path)
}

// Mkdir logs directory creation (either direction).
func (l *syncLogger) Mkdir(path, direction string) {
	if !l.verbose {
		return
	}
	arrow := logDownload
	color := ansiCyan
	if direction == "upload" {
		arrow = logUpload
		color = ansiGreen
	}
	l.emit(color, arrow, "mkdir", path)
}

// Delete logs a delete propagation (either direction).
func (l *syncLogger) Delete(path, direction string) {
	if !l.verbose {
		return
	}
	arrow := logDownload
	color := ansiCyan
	if direction == "upload" {
		arrow = logUpload
		color = ansiGreen
	}
	l.emit(color, arrow, "delete", path)
}

// Symlink logs a symlink sync (either direction).
func (l *syncLogger) Symlink(path, target, direction string) {
	if !l.verbose {
		return
	}
	arrow := logDownload
	color := ansiCyan
	if direction == "upload" {
		arrow = logUpload
		color = ansiGreen
	}
	l.emit(color, arrow, "symlink → "+target, path)
}

// Conflict logs a conflict detection.
func (l *syncLogger) Conflict(path, conflictCopy string) {
	l.emit(ansiYellow, logConflict, "conflict", path+" → "+conflictCopy)
}

// Skip logs a skipped file (echo suppression, unchanged, etc).
func (l *syncLogger) Skip(path, reason string) {
	if !l.verbose {
		return
	}
	l.emit(ansiDim, logSkip, reason, path)
}

// Err logs an error. Always emitted regardless of verbose.
func (l *syncLogger) Err(context, detail string) {
	l.emit(ansiRed, logError, context, detail)
}

// Info logs an informational message. Only in verbose mode.
func (l *syncLogger) Info(msg string) {
	if !l.verbose {
		return
	}
	ts := time.Now().Format("15:04:05")
	if colorTerm {
		fmt.Fprintf(os.Stderr, "%s%s%s %s%s%s %s\n",
			ansiDim, ts, ansiReset,
			ansiDim, "ℹ", ansiReset,
			msg)
	} else {
		fmt.Fprintf(os.Stderr, "%s   %s\n", ts, msg)
	}
}

func (l *syncLogger) emit(color, arrow, action, path string) {
	ts := time.Now().Format("15:04:05")
	// Pad action to 12 chars for alignment.
	padded := action
	if len(padded) < 12 {
		padded += strings.Repeat(" ", 12-len(padded))
	}
	if colorTerm {
		fmt.Fprintf(os.Stderr, "%s%s%s %s%s%s %s%-12s%s %s\n",
			ansiDim, ts, ansiReset,
			color, arrow, ansiReset,
			color, action, ansiReset,
			path)
	} else {
		fmt.Fprintf(os.Stderr, "%s %s %s %s\n", ts, arrow, padded, path)
	}
}
