package queryvector

import (
	"errors"
	"testing"
)

func TestIsVectorUnavailableTreatsNonZeroDBAsUnavailable(t *testing.T) {
	t.Helper()

	err := errors.New("ERR Cannot create index on db != 0")
	if !isVectorUnavailable(err) {
		t.Fatalf("isVectorUnavailable(%q) = false, want true", err)
	}
}
