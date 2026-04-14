// Package uistatic embeds the compiled web UI assets (ui/dist/).
//
// The dist/ directory is populated at build time by the Makefile (make embed-ui).
// It is gitignored since it is a build artifact.
package uistatic

import "embed"

//go:embed all:dist
var Content embed.FS
