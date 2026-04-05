package client

import "testing"

func TestGlobMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		pattern string
		str     string
		want    bool
	}{
		// Exact match
		{"hello", "hello", true},
		{"hello", "world", false},
		{"", "", true},
		{"", "a", false},

		// Star wildcard
		{"*", "", true},
		{"*", "anything", true},
		{"*.txt", "file.txt", true},
		{"*.txt", "file.go", false},
		{"*.txt", ".txt", true},
		{"file.*", "file.txt", true},
		{"file.*", "file.", true},
		{"*.*", "a.b", true},
		{"*.*", "ab", false},
		{"a*b", "ab", true},
		{"a*b", "aXb", true},
		{"a*b", "aXYZb", true},
		{"a*b", "aXYZc", false},

		// Question mark
		{"?", "a", true},
		{"?", "", false},
		{"?", "ab", false},
		{"file?.log", "file1.log", true},
		{"file?.log", "fileAB.log", false},

		// Character class
		{"[abc]", "a", true},
		{"[abc]", "b", true},
		{"[abc]", "d", false},
		{"[a-z]", "m", true},
		{"[a-z]", "A", false},
		{"[a-zA-Z]", "Z", true},
		{"[a-zA-Z_]", "_", true},
		{"[0-9]", "5", true},
		{"[0-9]", "a", false},

		// Negated class
		{"[!abc]", "d", true},
		{"[!abc]", "a", false},
		{"[^abc]", "d", true},
		{"[^abc]", "b", false},
		{"*.[!o]", "file.c", true},
		{"*.[!o]", "file.o", false},

		// Reverse range
		{"[z-a]", "m", true},

		// Escaped characters
		{"\\*", "*", true},
		{"\\*", "a", false},
		{"\\?", "?", true},
		{"\\?", "a", false},
		{"\\[a]", "[a]", true},

		// Combined patterns
		{"[Mm]akefile", "Makefile", true},
		{"[Mm]akefile", "makefile", true},
		{"[Mm]akefile", "xakefile", false},
		{"*.tar.gz", "archive.tar.gz", true},
		{"*.tar.gz", "archive.tar.bz2", false},

		// Multiple stars
		{"**", "anything", true},
		{"a**b", "ab", true},
		{"a**b", "aXb", true},

		// Edge cases
		{"[", "x", false},
		{"[]", "x", false},
	}

	for _, tt := range tests {
		got := globMatch(tt.pattern, tt.str)
		if got != tt.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", tt.pattern, tt.str, got, tt.want)
		}
	}
}
