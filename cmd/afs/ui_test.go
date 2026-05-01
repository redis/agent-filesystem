package main

import (
	"errors"
	"strings"
	"testing"
)

// TestCellWidthEmojis locks down the width accounting for the emoji and
// marker glyphs we actually render. Plain table layout depends on these being
// correct — ✅ takes two cells in a terminal even though it's one rune, and
// ✓ / ✗ render single-width in every monospaced font we've tested.
func TestCellWidthEmojis(t *testing.T) {
	cases := map[rune]int{
		'✅': 2,
		'❌': 2,
		'✓': 1,
		'✗': 1,
		'●': 1,
		'○': 1,
		'A': 1,
		' ': 1,
	}
	for r, want := range cases {
		if got := cellWidth(r); got != want {
			t.Errorf("cellWidth(%q) = %d, want %d", r, got, want)
		}
	}
}

func TestRuneWidthHandlesEmoji(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"hello", 5},
		{"✓ ok", 4},
		{"✅ done", 7}, // ✅ (2) + space (1) + done (4)
		{"\033[32m✓\033[0m ok", 4},
	}
	for _, tc := range cases {
		if got := runeWidth(tc.input); got != tc.want {
			t.Errorf("runeWidth(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

// TestMarkerSuccessConstant pins the marker identity so a future find-replace
// doesn't silently swap the emoji for a different glyph that breaks the
// cellWidth table.
func TestMarkerSuccessConstant(t *testing.T) {
	if markerSuccess != "✅" {
		t.Errorf("markerSuccess = %q, want %q", markerSuccess, "✅")
	}
}

func TestFormatCLIErrorUsesPlainSectionFormat(t *testing.T) {
	t.Helper()

	got := formatCLIError(errors.New(`mount blocked for workspace "smoke": local path "/Users/rowantrollope/afs" is already populated and the remote workspace is not empty
Use an empty directory, import the local directory into a new workspace, or move conflicting files aside first`))

	want := `
Error

Mount blocked for workspace "smoke": local path "/Users/rowantrollope/afs" is already populated and the remote workspace is not empty.

Use an empty directory, import the local directory into a new workspace, or move conflicting files aside first.

`
	if got != want {
		t.Fatalf("formatCLIError() = %q, want %q", got, want)
	}
}

func TestFormatCLIErrorPreservesUsageBlocks(t *testing.T) {
	t.Helper()

	got := formatCLIError(errors.New("unknown flag \"--wat\"\n\nUsage:\n  afs ws mount [<workspace> <directory>]"))
	for _, want := range []string{
		"\nError\n\nUnknown flag \"--wat\".\n\nUsage:\n  afs ws mount [<workspace> <directory>]\n\n",
		"Usage:\n  afs ws mount",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatCLIError() = %q, want substring %q", got, want)
		}
	}
}
