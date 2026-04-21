// Package version exposes the build-time version metadata for the AFS
// binaries. Values are populated three ways, in order of precedence:
//
//  1. Linker flags — set by the Makefile / release pipeline via
//     -ldflags "-X github.com/redis/agent-filesystem/internal/version.Version=…".
//  2. runtime/debug.BuildInfo — populated automatically when the binary was
//     produced by `go install`/`go build` from a module with VCS info. Used
//     as a fallback so `go install github.com/redis/agent-filesystem/...`
//     still yields a sensible version.
//  3. Hardcoded "dev" defaults — for bare `go run` / IDE builds where
//     neither ldflags nor VCS info are present.
package version

import (
	"fmt"
	"runtime/debug"
	"strings"
	"sync"
)

// These vars can be overridden at build time via
// -ldflags "-X github.com/redis/agent-filesystem/internal/version.Version=v0.5.0".
var (
	Version   = "dev"
	Commit    = ""
	BuildDate = ""
)

var (
	resolveOnce sync.Once
	resolved    Info
)

// Info is the resolved build metadata exposed to callers.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit,omitempty"`
	BuildDate string `json:"build_date,omitempty"`
}

// Get returns the resolved version info, combining ldflags overrides with
// BuildInfo fallbacks. Safe to call from any goroutine; resolves once and
// caches the result.
func Get() Info {
	resolveOnce.Do(func() {
		resolved = Info{
			Version:   strings.TrimSpace(Version),
			Commit:    strings.TrimSpace(Commit),
			BuildDate: strings.TrimSpace(BuildDate),
		}

		// If ldflags didn't populate the fields, pull what we can from
		// debug.BuildInfo. This covers `go install` from an end-user's shell.
		bi, ok := debug.ReadBuildInfo()
		if !ok {
			return
		}
		if resolved.Version == "dev" && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			resolved.Version = bi.Main.Version
		}
		if resolved.Commit == "" {
			for _, setting := range bi.Settings {
				switch setting.Key {
				case "vcs.revision":
					if len(setting.Value) >= 7 {
						resolved.Commit = setting.Value[:7]
					} else {
						resolved.Commit = setting.Value
					}
				case "vcs.time":
					if resolved.BuildDate == "" {
						resolved.BuildDate = setting.Value
					}
				case "vcs.modified":
					if setting.Value == "true" && !strings.Contains(resolved.Version, "-dirty") {
						resolved.Version += "-dirty"
					}
				}
			}
		}
	})
	return resolved
}

// Short returns a compact display version: just the Version, e.g. "v0.5.2".
func Short() string {
	return Get().Version
}

// String returns the long-form display string: "v0.5.2 (a1b2c3d, 2026-04-21)".
// Commit and build date are omitted if unknown, so `dev` just returns "dev".
func String() string {
	info := Get()
	switch {
	case info.Commit == "" && info.BuildDate == "":
		return info.Version
	case info.BuildDate == "":
		return fmt.Sprintf("%s (%s)", info.Version, info.Commit)
	case info.Commit == "":
		return fmt.Sprintf("%s (%s)", info.Version, info.BuildDate)
	default:
		return fmt.Sprintf("%s (%s, %s)", info.Version, info.Commit, info.BuildDate)
	}
}
