package main

import (
	"testing"
	"time"
)

func TestTakeRenameCandidateForLocalFileFallsBackToSingleSameDirCandidate(t *testing.T) {
	t.Helper()

	r := &reconciler{
		renameCandidates: map[string]renameCandidate{
			"file:inode:1:2": {
				path: "tracked/old.txt",
				entry: SyncEntry{
					Type:          "file",
					LocalIdentity: "1:2",
					LocalHash:     "oldhash",
					Size:          10,
				},
				recordedAt: time.Now().UTC(),
			},
		},
	}

	candidate, ok := r.takeRenameCandidateForLocalFile("tracked/new.txt", "", "newhash", 24, false)
	if !ok {
		t.Fatal("takeRenameCandidateForLocalFile() = no match, want fallback candidate")
	}
	if candidate.path != "tracked/old.txt" {
		t.Fatalf("candidate.path = %q, want tracked/old.txt", candidate.path)
	}
	if len(r.renameCandidates) != 0 {
		t.Fatalf("len(renameCandidates) = %d, want 0 after consuming fallback", len(r.renameCandidates))
	}
}

func TestTakeRenameCandidateForLocalFileRejectsAmbiguousFallback(t *testing.T) {
	t.Helper()

	r := &reconciler{
		renameCandidates: map[string]renameCandidate{
			"one": {
				path:       "tracked/old-a.txt",
				entry:      SyncEntry{Type: "file"},
				recordedAt: time.Now().UTC(),
			},
			"two": {
				path:       "tracked/old-b.txt",
				entry:      SyncEntry{Type: "file"},
				recordedAt: time.Now().UTC(),
			},
		},
	}

	if _, ok := r.takeRenameCandidateForLocalFile("tracked/new.txt", "", "newhash", 24, false); ok {
		t.Fatal("takeRenameCandidateForLocalFile() = fallback match, want ambiguity rejection")
	}
	if len(r.renameCandidates) != 2 {
		t.Fatalf("len(renameCandidates) = %d, want 2 when fallback is ambiguous", len(r.renameCandidates))
	}
}

func TestTakeRenameCandidateForLocalSymlinkFallsBackToSingleSameDirCandidate(t *testing.T) {
	t.Helper()

	r := &reconciler{
		renameCandidates: map[string]renameCandidate{
			"symlink:inode:1:2": {
				path: "tracked/old-link",
				entry: SyncEntry{
					Type:          "symlink",
					LocalIdentity: "1:2",
					Target:        "target.txt",
				},
				recordedAt: time.Now().UTC(),
			},
		},
	}

	candidate, ok := r.takeRenameCandidateForLocalSymlink("tracked/new-link", "", "updated.txt", false)
	if !ok {
		t.Fatal("takeRenameCandidateForLocalSymlink() = no match, want fallback candidate")
	}
	if candidate.path != "tracked/old-link" {
		t.Fatalf("candidate.path = %q, want tracked/old-link", candidate.path)
	}
	if len(r.renameCandidates) != 0 {
		t.Fatalf("len(renameCandidates) = %d, want 0 after consuming fallback", len(r.renameCandidates))
	}
}
