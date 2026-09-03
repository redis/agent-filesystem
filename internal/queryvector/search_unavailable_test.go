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

func TestIsUnknownSearchIndexRecognizesRedisSearch810Error(t *testing.T) {
	err := errors.New("SEARCH_INDEX_NOT_FOUND Index not found: afs:qvec:{workspace}:v1")
	if !isUnknownSearchIndex(err) {
		t.Fatalf("isUnknownSearchIndex(%q) = false, want true", err)
	}
}
