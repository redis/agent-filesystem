package main

import "testing"

func TestParseVersioningSetArgs(t *testing.T) {
	parsed, err := parseVersioningSetArgs([]string{
		"repo",
		"--mode", "paths",
		"--include", "src/**",
		"--include=docs/**",
		"--exclude", "**/*.log",
		"--max-versions-per-file", "10",
		"--max-age-days=30",
		"--max-total-bytes", "4096",
		"--large-file-cutoff-bytes=8192",
	})
	if err != nil {
		t.Fatalf("parseVersioningSetArgs() returned error: %v", err)
	}
	if parsed.workspace != "repo" {
		t.Fatalf("workspace = %q, want repo", parsed.workspace)
	}
	if parsed.mode != "paths" {
		t.Fatalf("mode = %q, want paths", parsed.mode)
	}
	if len(parsed.includeGlobs) != 2 || parsed.includeGlobs[0] != "src/**" || parsed.includeGlobs[1] != "docs/**" {
		t.Fatalf("includeGlobs = %#v, want [src/** docs/**]", parsed.includeGlobs)
	}
	if len(parsed.excludeGlobs) != 1 || parsed.excludeGlobs[0] != "**/*.log" {
		t.Fatalf("excludeGlobs = %#v, want [**/*.log]", parsed.excludeGlobs)
	}
	if parsed.maxVersionsPerFile == nil || *parsed.maxVersionsPerFile != 10 {
		t.Fatalf("maxVersionsPerFile = %#v, want 10", parsed.maxVersionsPerFile)
	}
	if parsed.maxAgeDays == nil || *parsed.maxAgeDays != 30 {
		t.Fatalf("maxAgeDays = %#v, want 30", parsed.maxAgeDays)
	}
	if parsed.maxTotalBytes == nil || *parsed.maxTotalBytes != 4096 {
		t.Fatalf("maxTotalBytes = %#v, want 4096", parsed.maxTotalBytes)
	}
	if parsed.largeFileCutoffBytes == nil || *parsed.largeFileCutoffBytes != 8192 {
		t.Fatalf("largeFileCutoffBytes = %#v, want 8192", parsed.largeFileCutoffBytes)
	}
}

func TestParseVersioningSetArgsRejectsBadInput(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "unknown flag", args: []string{"--wat"}},
		{name: "too many workspaces", args: []string{"a", "b"}},
		{name: "missing mode value", args: []string{"--mode"}},
		{name: "bad integer", args: []string{"--max-age-days", "abc"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parseVersioningSetArgs(test.args); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}
