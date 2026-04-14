package searchindex

import (
	"bytes"
	"strings"
	"testing"
)

func TestBuildFileFieldsReady(t *testing.T) {
	fields := BuildFileFields([]byte("Hello"))
	if fields.SearchState != StateReady {
		t.Fatalf("SearchState = %q, want %q", fields.SearchState, StateReady)
	}
	if got, want := fields.GrepGramsCI, strings.Join(QueryTermsForLiteral("Hello"), " "); got != want {
		t.Fatalf("GrepGramsCI = %q, want %q", got, want)
	}
}

func TestBuildFileFieldsBinary(t *testing.T) {
	fields := BuildFileFields([]byte("ab\x00cd"))
	if fields.SearchState != StateBinary {
		t.Fatalf("SearchState = %q, want %q", fields.SearchState, StateBinary)
	}
	if fields.GrepGramsCI != "" {
		t.Fatalf("GrepGramsCI = %q, want empty", fields.GrepGramsCI)
	}
}

func TestBuildFileFieldsLarge(t *testing.T) {
	fields := BuildFileFields(bytes.Repeat([]byte("x"), MaxIndexedBytes+1))
	if fields.SearchState != StateLarge {
		t.Fatalf("SearchState = %q, want %q", fields.SearchState, StateLarge)
	}
	if fields.GrepGramsCI != "" {
		t.Fatalf("GrepGramsCI = %q, want empty", fields.GrepGramsCI)
	}
}

func TestEscapeTagValue(t *testing.T) {
	if got, want := EscapeTagValue("/src/app.go"), `\/src\/app\.go`; got != want {
		t.Fatalf("EscapeTagValue() = %q, want %q", got, want)
	}
}
