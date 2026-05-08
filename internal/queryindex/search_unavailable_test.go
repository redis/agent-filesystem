package queryindex

import (
	"errors"
	"testing"
)

func TestIsSearchUnavailableTreatsNonZeroDBAsUnavailable(t *testing.T) {
	t.Helper()

	err := errors.New("ERR Cannot create index on db != 0")
	if !isSearchUnavailable(err) {
		t.Fatalf("isSearchUnavailable(%q) = false, want true", err)
	}
}
