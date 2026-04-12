package main

import "testing"

// TestCellWidthEmojis locks down the width accounting for the emoji and
// marker glyphs we actually render. Box layout depends on these being
// correct — ✅ takes two cells in a terminal even though it's one rune, and
// ✓ / ✗ render single-width in every monospaced font we've tested.
func TestCellWidthEmojis(t *testing.T) {
	cases := map[rune]int{
		'✅': 2,
		'❌': 2,
		'✓':  1,
		'✗':  1,
		'●':  1,
		'○':  1,
		'A':  1,
		' ':  1,
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
