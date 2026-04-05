package main

import (
	"fmt"
	"regexp"
	"time"
)

const (
	rafFormatVersion   = 1
	rafInlineThreshold = 4 * 1024
)

var rafNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type rafLocalState struct {
	Version        int       `json:"version"`
	Workspace      string    `json:"workspace"`
	Session        string    `json:"session"`
	HeadSavepoint  string    `json:"head_savepoint"`
	Dirty          bool      `json:"dirty"`
	MaterializedAt time.Time `json:"materialized_at"`
	LastScanAt     time.Time `json:"last_scan_at"`
	ArchivedAt     time.Time `json:"archived_at,omitempty"`
}

type workspaceMeta struct {
	Version          int       `json:"version"`
	Name             string    `json:"name"`
	CreatedAt        time.Time `json:"created_at"`
	DefaultSession   string    `json:"default_session"`
	DefaultSavepoint string    `json:"default_savepoint"`
}

type sessionMeta struct {
	Version                 int       `json:"version"`
	Workspace               string    `json:"workspace"`
	Name                    string    `json:"name"`
	HeadSavepoint           string    `json:"head_savepoint"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
	DirtyHint               bool      `json:"dirty_hint"`
	LastMaterializedAt      time.Time `json:"last_materialized_at"`
	LastKnownMaterializedAt string    `json:"last_materialized_host,omitempty"`
}

type savepointMeta struct {
	Version         int       `json:"version"`
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Workspace       string    `json:"workspace"`
	Session         string    `json:"session"`
	ParentSavepoint string    `json:"parent_savepoint,omitempty"`
	ManifestHash    string    `json:"manifest_hash"`
	CreatedAt       time.Time `json:"created_at"`
	FileCount       int       `json:"file_count"`
	DirCount        int       `json:"dir_count"`
	TotalBytes      int64     `json:"total_bytes"`
}

type manifest struct {
	Version   int                      `json:"version"`
	Workspace string                   `json:"workspace"`
	Savepoint string                   `json:"savepoint"`
	Entries   map[string]manifestEntry `json:"entries"`
}

type manifestEntry struct {
	Type    string `json:"type"`
	Mode    uint32 `json:"mode"`
	MtimeMs int64  `json:"mtime_ms"`
	Size    int64  `json:"size"`
	BlobID  string `json:"blob_id,omitempty"`
	Inline  string `json:"inline,omitempty"`
	Target  string `json:"target,omitempty"`
}

type blobRef struct {
	BlobID    string    `json:"blob_id"`
	Size      int64     `json:"size"`
	RefCount  int64     `json:"ref_count"`
	CreatedAt time.Time `json:"created_at"`
}

type manifestStats struct {
	FileCount  int
	DirCount   int
	TotalBytes int64
}

type workspaceBlobStats struct {
	Count int
	Bytes int64
}

func validateRAFName(kind, value string) error {
	if value == "" {
		return fmt.Errorf("%s name is required", kind)
	}
	if !rafNamePattern.MatchString(value) {
		return fmt.Errorf("%s name %q is invalid; use letters, numbers, dot, dash, and underscore", kind, value)
	}
	return nil
}
