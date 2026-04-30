package controlplane

import (
	"encoding/base64"
	"testing"
)

func TestDiffManifestsDetectsCreateUpdateDeleteAndRename(t *testing.T) {
	t.Helper()

	base := Manifest{Entries: map[string]ManifestEntry{
		"/":              {Type: "dir", Mode: 0o755},
		"/delete.md":     {Type: "file", Mode: 0o644, Size: 6, Inline: base64.StdEncoding.EncodeToString([]byte("delete"))},
		"/rename-old.md": {Type: "file", Mode: 0o644, Size: 6, Inline: base64.StdEncoding.EncodeToString([]byte("rename"))},
		"/update.md":     {Type: "file", Mode: 0o644, Size: 3, Inline: base64.StdEncoding.EncodeToString([]byte("old"))},
	}}
	head := Manifest{Entries: map[string]ManifestEntry{
		"/":              {Type: "dir", Mode: 0o755},
		"/create.md":     {Type: "file", Mode: 0o644, Size: 6, Inline: base64.StdEncoding.EncodeToString([]byte("create"))},
		"/rename-new.md": {Type: "file", Mode: 0o644, Size: 6, Inline: base64.StdEncoding.EncodeToString([]byte("rename"))},
		"/update.md":     {Type: "file", Mode: 0o644, Size: 3, Inline: base64.StdEncoding.EncodeToString([]byte("new"))},
	}}

	entries := diffManifests(base, head)
	byOp := map[string]DiffEntry{}
	for _, entry := range entries {
		byOp[entry.Op] = entry
	}
	if got := len(entries); got != 4 {
		t.Fatalf("len(diff entries) = %d, want 4: %#v", got, entries)
	}
	if byOp[DiffOpCreate].Path != "/create.md" {
		t.Fatalf("create entry = %#v, want /create.md", byOp[DiffOpCreate])
	}
	if byOp[DiffOpUpdate].Path != "/update.md" {
		t.Fatalf("update entry = %#v, want /update.md", byOp[DiffOpUpdate])
	}
	if byOp[DiffOpDelete].Path != "/delete.md" {
		t.Fatalf("delete entry = %#v, want /delete.md", byOp[DiffOpDelete])
	}
	rename := byOp[DiffOpRename]
	if rename.PreviousPath != "/rename-old.md" || rename.Path != "/rename-new.md" {
		t.Fatalf("rename entry = %#v, want rename-old -> rename-new", rename)
	}
}

func TestSummarizeDiffEntriesCountsBytes(t *testing.T) {
	t.Helper()

	summary := summarizeDiffEntries([]DiffEntry{
		{Op: DiffOpCreate, DeltaBytes: 10},
		{Op: DiffOpUpdate, DeltaBytes: -3},
		{Op: DiffOpDelete, DeltaBytes: -7},
		{Op: DiffOpRename},
		{Op: DiffOpMetadata},
	})
	if summary.Total != 5 || summary.Created != 1 || summary.Updated != 1 || summary.Deleted != 1 || summary.Renamed != 1 || summary.MetadataChanged != 1 {
		t.Fatalf("summary counts = %+v, want one of each", summary)
	}
	if summary.BytesAdded != 10 || summary.BytesRemoved != 10 {
		t.Fatalf("summary bytes = +%d/-%d, want +10/-10", summary.BytesAdded, summary.BytesRemoved)
	}
}
