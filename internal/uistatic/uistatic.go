// Package uistatic embeds the compiled web UI assets.
//
// The dist/ directory is normally populated at build time by the Makefile
// (make embed-ui). A tracked placeholder file keeps the directory present so
// non-UI builds can still compile; the HTTP layer falls back to API-only mode
// when dist/index.html is not embedded.
package uistatic

import "embed"

//go:embed all:dist
var Content embed.FS
