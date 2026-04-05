package qmd

import "time"

const (
	ToolVersion   = "0.1.0"
	SchemaVersion = "rfs_qmd_v1"
)

// SearchHit is one indexed document match.
type SearchHit struct {
	DocID   string
	Path    string
	Type    string
	Content string
	Size    int64
	MtimeMS int64
	CtimeMS int64
	Score   float64
}

// QueryOptions controls FT.SEARCH execution.
type QueryOptions struct {
	Limit  int
	Offset int
}

// ParsedQuery holds DSL parsing output.
type ParsedQuery struct {
	TextQuery  string
	PathPrefix string
	TypeFilter string
	MinSize    *int64
	MaxSize    *int64
	MinMtimeMS *int64
	MaxMtimeMS *int64
	MinCtimeMS *int64
	MaxCtimeMS *int64
}

// WatchEvent is a top-result change from watch mode.
type WatchEvent struct {
	At   time.Time
	Hits []SearchHit
}
