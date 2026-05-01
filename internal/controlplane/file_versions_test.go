package controlplane

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redis/agent-filesystem/mount/client"
)

func TestStoreFileLineageDeleteAndRecreateAllocatesNewLineage(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	service, _, err := manager.serviceFor(context.Background(), databaseID)
	if err != nil {
		t.Fatalf("serviceFor() returned error: %v", err)
	}

	createdAt := time.Now().UTC().Add(-time.Minute)
	first, err := service.store.CreateFileLineage(context.Background(), "repo", "/src/main.go", createdAt)
	if err != nil {
		t.Fatalf("CreateFileLineage(first) returned error: %v", err)
	}
	if first.State != FileLineageStateLive {
		t.Fatalf("first.State = %q, want %q", first.State, FileLineageStateLive)
	}

	resolved, err := service.store.ResolveLiveFileLineageByPath(context.Background(), "repo", "/src/main.go")
	if err != nil {
		t.Fatalf("ResolveLiveFileLineageByPath() returned error: %v", err)
	}
	if resolved.FileID != first.FileID {
		t.Fatalf("resolved.FileID = %q, want %q", resolved.FileID, first.FileID)
	}

	deleted, err := service.store.DeleteFileLineage(context.Background(), "repo", first.FileID, time.Now().UTC())
	if err != nil {
		t.Fatalf("DeleteFileLineage() returned error: %v", err)
	}
	if deleted.State != FileLineageStateDeleted {
		t.Fatalf("deleted.State = %q, want %q", deleted.State, FileLineageStateDeleted)
	}
	if _, err := service.store.ResolveLiveFileLineageByPath(context.Background(), "repo", "/src/main.go"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ResolveLiveFileLineageByPath(after delete) error = %v, want os.ErrNotExist", err)
	}

	second, err := service.store.CreateFileLineage(context.Background(), "repo", "/src/main.go", time.Now().UTC())
	if err != nil {
		t.Fatalf("CreateFileLineage(second) returned error: %v", err)
	}
	if second.FileID == first.FileID {
		t.Fatalf("second.FileID = %q, want a new lineage id", second.FileID)
	}

	storedFirst, err := service.store.GetFileLineage(context.Background(), "repo", first.FileID)
	if err != nil {
		t.Fatalf("GetFileLineage(first) returned error: %v", err)
	}
	if storedFirst.State != FileLineageStateDeleted {
		t.Fatalf("storedFirst.State = %q, want %q", storedFirst.State, FileLineageStateDeleted)
	}
}

func TestStoreAppendFileVersionAllocatesOrdinalsAndIndexes(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	service, _, err := manager.serviceFor(context.Background(), databaseID)
	if err != nil {
		t.Fatalf("serviceFor() returned error: %v", err)
	}

	lineage, err := service.store.CreateFileLineage(context.Background(), "repo", "/src/main.go", time.Now().UTC())
	if err != nil {
		t.Fatalf("CreateFileLineage() returned error: %v", err)
	}

	first, err := service.store.AppendFileVersion(context.Background(), "repo", lineage.FileID, FileVersion{
		Path:        "/src/main.go",
		Op:          ChangeOpPut,
		Kind:        FileVersionKindFile,
		ContentHash: "hash-1",
		SizeBytes:   11,
	})
	if err != nil {
		t.Fatalf("AppendFileVersion(first) returned error: %v", err)
	}
	second, err := service.store.AppendFileVersion(context.Background(), "repo", lineage.FileID, FileVersion{
		Path:        "/src/main.go",
		Op:          ChangeOpPut,
		Kind:        FileVersionKindFile,
		ContentHash: "hash-2",
		PrevHash:    "hash-1",
		SizeBytes:   12,
		DeltaBytes:  1,
	})
	if err != nil {
		t.Fatalf("AppendFileVersion(second) returned error: %v", err)
	}

	if first.Ordinal != 1 {
		t.Fatalf("first.Ordinal = %d, want 1", first.Ordinal)
	}
	if second.Ordinal != 2 {
		t.Fatalf("second.Ordinal = %d, want 2", second.Ordinal)
	}

	lookedUp, err := service.store.GetFileVersion(context.Background(), "repo", second.VersionID)
	if err != nil {
		t.Fatalf("GetFileVersion() returned error: %v", err)
	}
	if lookedUp.FileID != lineage.FileID {
		t.Fatalf("lookedUp.FileID = %q, want %q", lookedUp.FileID, lineage.FileID)
	}
	if lookedUp.ContentHash != "hash-2" {
		t.Fatalf("lookedUp.ContentHash = %q, want %q", lookedUp.ContentHash, "hash-2")
	}

	ascending, err := service.store.ListFileVersions(context.Background(), "repo", lineage.FileID, true)
	if err != nil {
		t.Fatalf("ListFileVersions(asc) returned error: %v", err)
	}
	if len(ascending) != 2 {
		t.Fatalf("len(ascending) = %d, want 2", len(ascending))
	}
	if ascending[0].Ordinal != 1 || ascending[1].Ordinal != 2 {
		t.Fatalf("ascending ordinals = [%d %d], want [1 2]", ascending[0].Ordinal, ascending[1].Ordinal)
	}

	descending, err := service.store.ListFileVersions(context.Background(), "repo", lineage.FileID, false)
	if err != nil {
		t.Fatalf("ListFileVersions(desc) returned error: %v", err)
	}
	if len(descending) != 2 {
		t.Fatalf("len(descending) = %d, want 2", len(descending))
	}
	if descending[0].Ordinal != 2 || descending[1].Ordinal != 1 {
		t.Fatalf("descending ordinals = [%d %d], want [2 1]", descending[0].Ordinal, descending[1].Ordinal)
	}

	pathHistory, err := service.store.ListPathHistoryVersionIDs(context.Background(), "repo", "/src/main.go")
	if err != nil {
		t.Fatalf("ListPathHistoryVersionIDs() returned error: %v", err)
	}
	if len(pathHistory) != 2 {
		t.Fatalf("len(pathHistory) = %d, want 2", len(pathHistory))
	}
	if pathHistory[0] != first.VersionID || pathHistory[1] != second.VersionID {
		t.Fatalf("pathHistory = %#v, want [%q %q]", pathHistory, first.VersionID, second.VersionID)
	}
}

func TestStoreAppendFileVersionConcurrentOrdinalsRemainUnique(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	service, _, err := manager.serviceFor(context.Background(), databaseID)
	if err != nil {
		t.Fatalf("serviceFor() returned error: %v", err)
	}

	lineage, err := service.store.CreateFileLineage(context.Background(), "repo", "/src/main.go", time.Now().UTC())
	if err != nil {
		t.Fatalf("CreateFileLineage() returned error: %v", err)
	}

	const writers = 8
	results := make([]FileVersion, writers)
	errorsOut := make([]error, writers)
	var wg sync.WaitGroup
	wg.Add(writers)
	for i := 0; i < writers; i++ {
		go func(index int) {
			defer wg.Done()
			results[index], errorsOut[index] = service.store.AppendFileVersion(context.Background(), "repo", lineage.FileID, FileVersion{
				Path:        "/src/main.go",
				Op:          ChangeOpPut,
				Kind:        FileVersionKindFile,
				ContentHash: "hash",
			})
		}(i)
	}
	wg.Wait()

	ordinals := make([]int, 0, writers)
	for i, err := range errorsOut {
		if err != nil {
			t.Fatalf("AppendFileVersion(%d) returned error: %v", i, err)
		}
		ordinals = append(ordinals, int(results[i].Ordinal))
	}
	sort.Ints(ordinals)
	for i, ordinal := range ordinals {
		want := i + 1
		if ordinal != want {
			t.Fatalf("sorted ordinals[%d] = %d, want %d; full=%v", i, ordinal, want, ordinals)
		}
	}
}

func TestRecordFileVersionMutationConflictsOnStaleBeforeSnapshot(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	service, _, err := manager.serviceFor(context.Background(), databaseID)
	if err != nil {
		t.Fatalf("serviceFor() returned error: %v", err)
	}
	if err := service.store.PutWorkspaceVersioningPolicy(context.Background(), "repo", WorkspaceVersioningPolicy{
		Mode: WorkspaceVersioningModeAll,
	}); err != nil {
		t.Fatalf("PutWorkspaceVersioningPolicy() returned error: %v", err)
	}

	before := VersionedFileSnapshot{
		Path:    "/src/main.go",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("base\n"),
	}
	if _, err := service.store.RecordFileVersionMutation(context.Background(), "repo", VersionedFileSnapshot{Path: "/src/main.go"}, before, FileVersionMutationMetadata{Source: ChangeSourceMCP}); err != nil {
		t.Fatalf("RecordFileVersionMutation(seed) returned error: %v", err)
	}

	firstAfter := VersionedFileSnapshot{
		Path:    "/src/main.go",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("first\n"),
	}
	secondAfter := VersionedFileSnapshot{
		Path:    "/src/main.go",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("second\n"),
	}

	firstResult, err := service.store.RecordFileVersionMutation(context.Background(), "repo", before, firstAfter, FileVersionMutationMetadata{Source: ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation(first writer) returned error: %v", err)
	}
	if _, err := service.store.RecordFileVersionMutation(context.Background(), "repo", before, secondAfter, FileVersionMutationMetadata{Source: ChangeSourceMCP}); !errors.Is(err, ErrWorkspaceConflict) {
		t.Fatalf("RecordFileVersionMutation(stale writer) error = %v, want ErrWorkspaceConflict", err)
	}

	history, err := service.GetFileHistory(context.Background(), "repo", "/src/main.go", false)
	if err != nil {
		t.Fatalf("GetFileHistory() returned error: %v", err)
	}
	if len(history.Lineages) != 1 || len(history.Lineages[0].Versions) != 2 {
		t.Fatalf("history.Lineages = %#v, want seed plus one winning update", history.Lineages)
	}
	if history.Lineages[0].Versions[1].VersionID != firstResult.VersionID {
		t.Fatalf("winning version = %q, want %q", history.Lineages[0].Versions[1].VersionID, firstResult.VersionID)
	}
}

func TestImportWorkspaceSeedsInitialFileVersionsWhenPolicyEnabled(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	service, _, err := manager.serviceFor(context.Background(), databaseID)
	if err != nil {
		t.Fatalf("serviceFor() returned error: %v", err)
	}

	manifest := Manifest{
		Version:   FormatVersion,
		Workspace: "placeholder",
		Savepoint: InitialCheckpointName,
		Entries: map[string]ManifestEntry{
			"/":              {Type: "dir", Mode: 0o755},
			"/notes/todo.md": {Type: "file", Mode: 0o644, Size: int64(len("seeded\n")), Inline: base64.StdEncoding.EncodeToString([]byte("seeded\n"))},
		},
	}

	response, err := service.importWorkspace(context.Background(), ImportWorkspaceRequest{
		Name:       "history-import",
		Manifest:   manifest,
		FileCount:  1,
		DirCount:   1,
		TotalBytes: int64(len("seeded\n")),
		VersioningPolicy: &WorkspaceVersioningPolicy{
			Mode: WorkspaceVersioningModeAll,
		},
	})
	if err != nil {
		t.Fatalf("importWorkspace() returned error: %v", err)
	}

	lineage, err := service.store.ResolveLiveFileLineageByPath(context.Background(), response.WorkspaceID, "/notes/todo.md")
	if err != nil {
		t.Fatalf("ResolveLiveFileLineageByPath() returned error: %v", err)
	}
	versions, err := service.store.ListFileVersions(context.Background(), response.WorkspaceID, lineage.FileID, true)
	if err != nil {
		t.Fatalf("ListFileVersions() returned error: %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("len(versions) = %d, want 1", len(versions))
	}
	if versions[0].Ordinal != 1 {
		t.Fatalf("versions[0].Ordinal = %d, want 1", versions[0].Ordinal)
	}
	if versions[0].Source != ChangeSourceImport {
		t.Fatalf("versions[0].Source = %q, want %q", versions[0].Source, ChangeSourceImport)
	}
}

func TestRecordFileVersionMutationConcurrentWritersProduceSingleWinner(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	service, _, err := manager.serviceFor(context.Background(), databaseID)
	if err != nil {
		t.Fatalf("serviceFor() returned error: %v", err)
	}
	if err := service.store.PutWorkspaceVersioningPolicy(context.Background(), "repo", WorkspaceVersioningPolicy{
		Mode: WorkspaceVersioningModeAll,
	}); err != nil {
		t.Fatalf("PutWorkspaceVersioningPolicy() returned error: %v", err)
	}

	before := VersionedFileSnapshot{
		Path:    "/src/race.go",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("base\n"),
	}
	seed, err := service.store.RecordFileVersionMutation(context.Background(), "repo", VersionedFileSnapshot{Path: "/src/race.go"}, before, FileVersionMutationMetadata{Source: ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation(seed) returned error: %v", err)
	}

	const writers = 6
	start := make(chan struct{})
	var wg sync.WaitGroup
	type result struct {
		version *FileVersion
		err     error
	}
	results := make([]result, writers)

	for index := 0; index < writers; index++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			version, err := service.store.RecordFileVersionMutation(context.Background(), "repo", before, VersionedFileSnapshot{
				Path:    "/src/race.go",
				Exists:  true,
				Kind:    "file",
				Mode:    0o644,
				Content: []byte("writer-" + string(rune('a'+i)) + "\n"),
			}, FileVersionMutationMetadata{Source: ChangeSourceMCP})
			results[i] = result{version: version, err: err}
		}(index)
	}

	close(start)
	wg.Wait()

	successes := 0
	for _, outcome := range results {
		if outcome.err == nil {
			successes++
			continue
		}
		if !errors.Is(outcome.err, ErrWorkspaceConflict) {
			t.Fatalf("concurrent writer error = %v, want nil or ErrWorkspaceConflict", outcome.err)
		}
	}
	if successes != 1 {
		t.Fatalf("successes = %d, want 1", successes)
	}

	versions, err := service.store.ListFileVersions(context.Background(), "repo", seed.FileID, true)
	if err != nil {
		t.Fatalf("ListFileVersions() returned error: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("len(versions) = %d, want 2", len(versions))
	}
	if versions[0].Ordinal != 1 || versions[1].Ordinal != 2 {
		t.Fatalf("ordinals = [%d %d], want [1 2]", versions[0].Ordinal, versions[1].Ordinal)
	}
}

func TestRestoreCheckpointCreatesFileVersionsWhenPolicyEnabled(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	service, _, err := manager.serviceFor(context.Background(), databaseID)
	if err != nil {
		t.Fatalf("serviceFor() returned error: %v", err)
	}
	if err := service.store.PutWorkspaceVersioningPolicy(context.Background(), "repo", WorkspaceVersioningPolicy{
		Mode: WorkspaceVersioningModeAll,
	}); err != nil {
		t.Fatalf("PutWorkspaceVersioningPolicy() returned error: %v", err)
	}

	meta, err := service.store.GetWorkspaceMeta(context.Background(), "repo")
	if err != nil {
		t.Fatalf("GetWorkspaceMeta() returned error: %v", err)
	}
	original, err := service.store.GetManifest(context.Background(), workspaceStorageID(meta), meta.HeadSavepoint)
	if err != nil {
		t.Fatalf("GetManifest(original) returned error: %v", err)
	}

	edited := cloneManifest(original)
	edited.Savepoint = "edited"
	edited.Entries["/README.md"] = ManifestEntry{
		Type:   "file",
		Mode:   0o644,
		Size:   int64(len("changed\n")),
		Inline: base64.StdEncoding.EncodeToString([]byte("changed\n")),
	}
	editedHash, err := HashManifest(edited)
	if err != nil {
		t.Fatalf("HashManifest(edited) returned error: %v", err)
	}
	if err := service.store.PutSavepoint(context.Background(), SavepointMeta{
		Version:         FormatVersion,
		ID:              "edited",
		Name:            "edited",
		Workspace:       workspaceStorageID(meta),
		ParentSavepoint: meta.HeadSavepoint,
		ManifestHash:    editedHash,
		CreatedAt:       time.Now().UTC(),
		FileCount:       2,
		DirCount:        2,
		TotalBytes:      int64(len("changed\n") + len("package main\n")),
	}, edited); err != nil {
		t.Fatalf("PutSavepoint(edited) returned error: %v", err)
	}
	meta.HeadSavepoint = "edited"
	if err := service.store.PutWorkspaceMeta(context.Background(), meta); err != nil {
		t.Fatalf("PutWorkspaceMeta() returned error: %v", err)
	}
	if err := SyncWorkspaceRoot(context.Background(), service.store, workspaceStorageID(meta), edited); err != nil {
		t.Fatalf("SyncWorkspaceRoot(edited) returned error: %v", err)
	}

	if _, err := service.restoreCheckpoint(context.Background(), "repo", "snapshot"); err != nil {
		t.Fatalf("restoreCheckpoint() returned error: %v", err)
	}

	lineage, err := service.store.ResolveLiveFileLineageByPath(context.Background(), "repo", "/README.md")
	if err != nil {
		t.Fatalf("ResolveLiveFileLineageByPath() returned error: %v", err)
	}
	versions, err := service.store.ListFileVersions(context.Background(), "repo", lineage.FileID, true)
	if err != nil {
		t.Fatalf("ListFileVersions() returned error: %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("len(versions) = %d, want 1", len(versions))
	}
	if versions[0].Source != "checkpoint_restore" {
		t.Fatalf("versions[0].Source = %q, want %q", versions[0].Source, "checkpoint_restore")
	}
	changelog, err := manager.ListChangelog(context.Background(), databaseID, "repo", ChangelogListRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListChangelog() returned error: %v", err)
	}
	foundLinkedRow := false
	for _, entry := range changelog.Entries {
		if entry.Path == "/README.md" && entry.FileID != "" && entry.VersionID != "" {
			foundLinkedRow = true
			break
		}
	}
	if !foundLinkedRow {
		t.Fatalf("checkpoint restore changelog missing file/version linkage: %+v", changelog.Entries)
	}
}

func TestServiceGetFileHistoryGroupsRecreatedPathLineages(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	service, _, err := manager.serviceFor(context.Background(), databaseID)
	if err != nil {
		t.Fatalf("serviceFor() returned error: %v", err)
	}
	if err := service.store.PutWorkspaceVersioningPolicy(context.Background(), "repo", WorkspaceVersioningPolicy{
		Mode: WorkspaceVersioningModeAll,
	}); err != nil {
		t.Fatalf("PutWorkspaceVersioningPolicy() returned error: %v", err)
	}

	before := VersionedFileSnapshot{Path: "/notes/todo.md"}
	firstAfter := VersionedFileSnapshot{Path: "/notes/todo.md", Exists: true, Kind: "file", Mode: 0o644, Content: []byte("one\n")}
	firstVersion, err := service.store.RecordFileVersionMutation(context.Background(), "repo", before, firstAfter, FileVersionMutationMetadata{Source: ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation(create #1) returned error: %v", err)
	}
	if _, err := service.store.RecordFileVersionMutation(context.Background(), "repo", firstAfter, before, FileVersionMutationMetadata{Source: ChangeSourceMCP}); err != nil {
		t.Fatalf("RecordFileVersionMutation(delete) returned error: %v", err)
	}
	secondAfter := VersionedFileSnapshot{Path: "/notes/todo.md", Exists: true, Kind: "file", Mode: 0o644, Content: []byte("two\n")}
	secondVersion, err := service.store.RecordFileVersionMutation(context.Background(), "repo", before, secondAfter, FileVersionMutationMetadata{Source: ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation(create #2) returned error: %v", err)
	}

	history, err := service.getFileHistory(context.Background(), "repo", "/notes/todo.md", false)
	if err != nil {
		t.Fatalf("getFileHistory() returned error: %v", err)
	}
	if len(history.Lineages) != 2 {
		t.Fatalf("len(history.Lineages) = %d, want 2", len(history.Lineages))
	}
	if history.Lineages[0].FileID != firstVersion.FileID {
		t.Fatalf("history.Lineages[0].FileID = %q, want %q", history.Lineages[0].FileID, firstVersion.FileID)
	}
	if len(history.Lineages[0].Versions) != 2 {
		t.Fatalf("len(history.Lineages[0].Versions) = %d, want 2", len(history.Lineages[0].Versions))
	}
	if history.Lineages[1].FileID != secondVersion.FileID {
		t.Fatalf("history.Lineages[1].FileID = %q, want %q", history.Lineages[1].FileID, secondVersion.FileID)
	}
	if len(history.Lineages[1].Versions) != 1 {
		t.Fatalf("len(history.Lineages[1].Versions) = %d, want 1", len(history.Lineages[1].Versions))
	}
}

func TestServiceGetFileHistoryPageUsesStableCursorOrdering(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	service, _, err := manager.serviceFor(context.Background(), databaseID)
	if err != nil {
		t.Fatalf("serviceFor() returned error: %v", err)
	}
	if err := service.store.PutWorkspaceVersioningPolicy(context.Background(), "repo", WorkspaceVersioningPolicy{
		Mode: WorkspaceVersioningModeAll,
	}); err != nil {
		t.Fatalf("PutWorkspaceVersioningPolicy() returned error: %v", err)
	}

	version1, err := service.store.RecordFileVersionMutation(context.Background(), "repo", VersionedFileSnapshot{Path: "/notes/page.txt"}, VersionedFileSnapshot{
		Path:    "/notes/page.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("one\n"),
	}, FileVersionMutationMetadata{Source: ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation(create) returned error: %v", err)
	}
	version2, err := service.store.RecordFileVersionMutation(context.Background(), "repo", VersionedFileSnapshot{
		Path:    "/notes/page.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("one\n"),
	}, VersionedFileSnapshot{
		Path:    "/notes/page.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("two\n"),
	}, FileVersionMutationMetadata{Source: ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation(update) returned error: %v", err)
	}
	version3, err := service.store.RecordFileVersionMutation(context.Background(), "repo", VersionedFileSnapshot{
		Path:    "/notes/page.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("two\n"),
	}, VersionedFileSnapshot{
		Path:    "/notes/page.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("three\n"),
	}, FileVersionMutationMetadata{Source: ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation(update #2) returned error: %v", err)
	}

	firstPage, err := service.GetFileHistoryPage(context.Background(), "repo", FileHistoryRequest{
		Path:        "/notes/page.txt",
		NewestFirst: false,
		Limit:       2,
	})
	if err != nil {
		t.Fatalf("GetFileHistoryPage(first page) returned error: %v", err)
	}
	if len(firstPage.Lineages) != 1 || len(firstPage.Lineages[0].Versions) != 2 {
		t.Fatalf("firstPage.Lineages = %#v, want one lineage with two versions", firstPage.Lineages)
	}
	if firstPage.Lineages[0].Versions[0].VersionID != version1.VersionID || firstPage.Lineages[0].Versions[1].VersionID != version2.VersionID {
		t.Fatalf("first page versions = %#v, want [%q %q]", firstPage.Lineages[0].Versions, version1.VersionID, version2.VersionID)
	}
	expectedCursor, err := fileHistoryCursorForVersion(*version2)
	if err != nil {
		t.Fatalf("fileHistoryCursorForVersion() returned error: %v", err)
	}
	if firstPage.NextCursor != expectedCursor {
		t.Fatalf("firstPage.NextCursor = %q, want %q", firstPage.NextCursor, expectedCursor)
	}

	secondPage, err := service.GetFileHistoryPage(context.Background(), "repo", FileHistoryRequest{
		Path:        "/notes/page.txt",
		NewestFirst: false,
		Limit:       2,
		Cursor:      firstPage.NextCursor,
	})
	if err != nil {
		t.Fatalf("GetFileHistoryPage(second page) returned error: %v", err)
	}
	if secondPage.NextCursor != "" {
		t.Fatalf("secondPage.NextCursor = %q, want empty", secondPage.NextCursor)
	}
	if len(secondPage.Lineages) != 1 || len(secondPage.Lineages[0].Versions) != 1 {
		t.Fatalf("secondPage.Lineages = %#v, want one lineage with one version", secondPage.Lineages)
	}
	if secondPage.Lineages[0].Versions[0].VersionID != version3.VersionID {
		t.Fatalf("second page version = %q, want %q", secondPage.Lineages[0].Versions[0].VersionID, version3.VersionID)
	}
}

func TestServiceGetFileHistoryPageDescCursorAcrossLineages(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	service, _, err := manager.serviceFor(context.Background(), databaseID)
	if err != nil {
		t.Fatalf("serviceFor() returned error: %v", err)
	}
	if err := service.store.PutWorkspaceVersioningPolicy(context.Background(), "repo", WorkspaceVersioningPolicy{
		Mode: WorkspaceVersioningModeAll,
	}); err != nil {
		t.Fatalf("PutWorkspaceVersioningPolicy() returned error: %v", err)
	}

	before := VersionedFileSnapshot{Path: "/notes/recreated-page.txt"}
	firstVersion, err := service.store.RecordFileVersionMutation(context.Background(), "repo", before, VersionedFileSnapshot{
		Path:    "/notes/recreated-page.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("one\n"),
	}, FileVersionMutationMetadata{Source: ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation(create #1) returned error: %v", err)
	}
	secondVersion, err := service.store.RecordFileVersionMutation(context.Background(), "repo", VersionedFileSnapshot{
		Path:    "/notes/recreated-page.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("one\n"),
	}, VersionedFileSnapshot{
		Path:    "/notes/recreated-page.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("two\n"),
	}, FileVersionMutationMetadata{Source: ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation(update #1) returned error: %v", err)
	}
	if _, err := service.store.RecordFileVersionMutation(context.Background(), "repo", VersionedFileSnapshot{
		Path:    "/notes/recreated-page.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("two\n"),
	}, before, FileVersionMutationMetadata{Source: ChangeSourceMCP}); err != nil {
		t.Fatalf("RecordFileVersionMutation(delete) returned error: %v", err)
	}
	latestVersion, err := service.store.RecordFileVersionMutation(context.Background(), "repo", before, VersionedFileSnapshot{
		Path:    "/notes/recreated-page.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("three\n"),
	}, FileVersionMutationMetadata{Source: ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation(create #2) returned error: %v", err)
	}

	page, err := service.GetFileHistoryPage(context.Background(), "repo", FileHistoryRequest{
		Path:        "/notes/recreated-page.txt",
		NewestFirst: true,
		Limit:       2,
	})
	if err != nil {
		t.Fatalf("GetFileHistoryPage(desc first page) returned error: %v", err)
	}
	if len(page.Lineages) != 2 {
		t.Fatalf("len(page.Lineages) = %d, want 2", len(page.Lineages))
	}
	if page.Lineages[0].FileID != latestVersion.FileID || len(page.Lineages[0].Versions) != 1 {
		t.Fatalf("page.Lineages[0] = %#v, want latest lineage first", page.Lineages[0])
	}
	if page.Lineages[1].FileID != firstVersion.FileID || len(page.Lineages[1].Versions) != 1 {
		t.Fatalf("page.Lineages[1] = %#v, want older lineage partial page", page.Lineages[1])
	}
	if page.Lineages[1].Versions[0].Op != ChangeOpDelete {
		t.Fatalf("older lineage first page version = %#v, want delete tombstone first", page.Lineages[1].Versions[0])
	}

	page2, err := service.GetFileHistoryPage(context.Background(), "repo", FileHistoryRequest{
		Path:        "/notes/recreated-page.txt",
		NewestFirst: true,
		Limit:       2,
		Cursor:      page.NextCursor,
	})
	if err != nil {
		t.Fatalf("GetFileHistoryPage(desc second page) returned error: %v", err)
	}
	if len(page2.Lineages) != 1 || page2.Lineages[0].FileID != firstVersion.FileID || len(page2.Lineages[0].Versions) != 2 {
		t.Fatalf("page2.Lineages = %#v, want remainder of original lineage", page2.Lineages)
	}
	if page2.Lineages[0].Versions[0].VersionID != secondVersion.VersionID || page2.Lineages[0].Versions[1].VersionID != firstVersion.VersionID {
		t.Fatalf("page2 versions = %#v, want update then original create", page2.Lineages[0].Versions)
	}
}

func TestStoreRecordFileVersionMutationRenamePreservesLineage(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	service, _, err := manager.serviceFor(context.Background(), databaseID)
	if err != nil {
		t.Fatalf("serviceFor() returned error: %v", err)
	}
	if err := service.store.PutWorkspaceVersioningPolicy(context.Background(), "repo", WorkspaceVersioningPolicy{
		Mode: WorkspaceVersioningModeAll,
	}); err != nil {
		t.Fatalf("PutWorkspaceVersioningPolicy() returned error: %v", err)
	}

	created, err := service.store.RecordFileVersionMutation(context.Background(), "repo", VersionedFileSnapshot{Path: "/notes/old.txt"}, VersionedFileSnapshot{
		Path:    "/notes/old.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("rename me\n"),
	}, FileVersionMutationMetadata{Source: ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation(create) returned error: %v", err)
	}

	renamed, err := service.store.RecordFileVersionMutation(context.Background(), "repo", VersionedFileSnapshot{
		Path:    "/notes/old.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("rename me\n"),
	}, VersionedFileSnapshot{
		Path:    "/notes/new.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("rename me\n"),
	}, FileVersionMutationMetadata{Source: ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation(rename) returned error: %v", err)
	}
	if renamed.FileID != created.FileID {
		t.Fatalf("renamed.FileID = %q, want %q", renamed.FileID, created.FileID)
	}
	if renamed.Op != ChangeOpRename {
		t.Fatalf("renamed.Op = %q, want %q", renamed.Op, ChangeOpRename)
	}
	if renamed.PrevPath != "/notes/old.txt" || renamed.Path != "/notes/new.txt" {
		t.Fatalf("renamed paths = prev %q path %q, want old/new", renamed.PrevPath, renamed.Path)
	}

	lineage, err := service.store.ResolveLiveFileLineageByPath(context.Background(), "repo", "/notes/new.txt")
	if err != nil {
		t.Fatalf("ResolveLiveFileLineageByPath(new) returned error: %v", err)
	}
	if lineage.FileID != created.FileID {
		t.Fatalf("lineage.FileID = %q, want %q", lineage.FileID, created.FileID)
	}
	if lineage.CurrentPath != "/notes/new.txt" {
		t.Fatalf("lineage.CurrentPath = %q, want /notes/new.txt", lineage.CurrentPath)
	}
	if _, err := service.store.ResolveLiveFileLineageByPath(context.Background(), "repo", "/notes/old.txt"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ResolveLiveFileLineageByPath(old) error = %v, want os.ErrNotExist", err)
	}

	oldHistory, err := service.GetFileHistory(context.Background(), "repo", "/notes/old.txt", false)
	if err != nil {
		t.Fatalf("GetFileHistory(old) returned error: %v", err)
	}
	if len(oldHistory.Lineages) != 1 || len(oldHistory.Lineages[0].Versions) != 2 {
		t.Fatalf("oldHistory.Lineages = %#v, want single lineage with create+rename", oldHistory.Lineages)
	}
	if oldHistory.Lineages[0].Versions[1].VersionID != renamed.VersionID {
		t.Fatalf("old history rename version = %q, want %q", oldHistory.Lineages[0].Versions[1].VersionID, renamed.VersionID)
	}

	newHistory, err := service.GetFileHistory(context.Background(), "repo", "/notes/new.txt", false)
	if err != nil {
		t.Fatalf("GetFileHistory(new) returned error: %v", err)
	}
	if len(newHistory.Lineages) != 1 || len(newHistory.Lineages[0].Versions) != 1 {
		t.Fatalf("newHistory.Lineages = %#v, want single lineage page for new path", newHistory.Lineages)
	}
	if newHistory.Lineages[0].Versions[0].VersionID != renamed.VersionID {
		t.Fatalf("new history version = %q, want %q", newHistory.Lineages[0].Versions[0].VersionID, renamed.VersionID)
	}
}

func TestStoreRecordFileVersionMutationRenameConflictsOnStaleBeforeSnapshot(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	service, _, err := manager.serviceFor(context.Background(), databaseID)
	if err != nil {
		t.Fatalf("serviceFor() returned error: %v", err)
	}
	if err := service.store.PutWorkspaceVersioningPolicy(context.Background(), "repo", WorkspaceVersioningPolicy{
		Mode: WorkspaceVersioningModeAll,
	}); err != nil {
		t.Fatalf("PutWorkspaceVersioningPolicy() returned error: %v", err)
	}

	if _, err := service.store.RecordFileVersionMutation(context.Background(), "repo", VersionedFileSnapshot{Path: "/notes/race.txt"}, VersionedFileSnapshot{
		Path:    "/notes/race.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("rename race\n"),
	}, FileVersionMutationMetadata{Source: ChangeSourceMCP}); err != nil {
		t.Fatalf("RecordFileVersionMutation(create) returned error: %v", err)
	}

	before := VersionedFileSnapshot{
		Path:    "/notes/race.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("rename race\n"),
	}
	if _, err := service.store.RecordFileVersionMutation(context.Background(), "repo", before, VersionedFileSnapshot{
		Path:    "/notes/current.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("rename race\n"),
	}, FileVersionMutationMetadata{Source: ChangeSourceMCP}); err != nil {
		t.Fatalf("RecordFileVersionMutation(first rename) returned error: %v", err)
	}

	if _, err := service.store.RecordFileVersionMutation(context.Background(), "repo", before, VersionedFileSnapshot{
		Path:    "/notes/stale.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("rename race\n"),
	}, FileVersionMutationMetadata{Source: ChangeSourceMCP}); !errors.Is(err, ErrWorkspaceConflict) {
		t.Fatalf("RecordFileVersionMutation(stale rename) error = %v, want ErrWorkspaceConflict", err)
	}
}

func TestStoreRecordFileVersionMutationSymlinkTargetChange(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	service, _, err := manager.serviceFor(context.Background(), databaseID)
	if err != nil {
		t.Fatalf("serviceFor() returned error: %v", err)
	}
	if err := service.store.PutWorkspaceVersioningPolicy(context.Background(), "repo", WorkspaceVersioningPolicy{
		Mode: WorkspaceVersioningModeAll,
	}); err != nil {
		t.Fatalf("PutWorkspaceVersioningPolicy() returned error: %v", err)
	}

	first, err := service.store.RecordFileVersionMutation(context.Background(), "repo", VersionedFileSnapshot{Path: "/notes/link"}, VersionedFileSnapshot{
		Path:   "/notes/link",
		Exists: true,
		Kind:   "symlink",
		Mode:   0o777,
		Target: "first.txt",
	}, FileVersionMutationMetadata{Source: ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation(create symlink) returned error: %v", err)
	}
	second, err := service.store.RecordFileVersionMutation(context.Background(), "repo", VersionedFileSnapshot{
		Path:   "/notes/link",
		Exists: true,
		Kind:   "symlink",
		Mode:   0o777,
		Target: "first.txt",
	}, VersionedFileSnapshot{
		Path:   "/notes/link",
		Exists: true,
		Kind:   "symlink",
		Mode:   0o777,
		Target: "second.txt",
	}, FileVersionMutationMetadata{Source: ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation(update symlink) returned error: %v", err)
	}
	if second.Op != ChangeOpSymlink {
		t.Fatalf("second.Op = %q, want %q", second.Op, ChangeOpSymlink)
	}
	if second.Target != "second.txt" {
		t.Fatalf("second.Target = %q, want %q", second.Target, "second.txt")
	}

	history, err := service.GetFileHistory(context.Background(), "repo", "/notes/link", false)
	if err != nil {
		t.Fatalf("GetFileHistory() returned error: %v", err)
	}
	if len(history.Lineages) != 1 || len(history.Lineages[0].Versions) != 2 {
		t.Fatalf("history.Lineages = %#v, want one lineage with two versions", history.Lineages)
	}
	if history.Lineages[0].Versions[0].VersionID != first.VersionID || history.Lineages[0].Versions[1].VersionID != second.VersionID {
		t.Fatalf("symlink history = %#v, want [first second]", history.Lineages[0].Versions)
	}
}

func TestForkWorkspacePreservesFileHistoryAndPolicy(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	service, _, err := manager.serviceFor(context.Background(), databaseID)
	if err != nil {
		t.Fatalf("serviceFor() returned error: %v", err)
	}
	policy := WorkspaceVersioningPolicy{
		Mode:         WorkspaceVersioningModePaths,
		IncludeGlobs: []string{"notes/**"},
		ExcludeGlobs: []string{"notes/tmp/**"},
	}
	if err := service.store.PutWorkspaceVersioningPolicy(context.Background(), "repo", policy); err != nil {
		t.Fatalf("PutWorkspaceVersioningPolicy() returned error: %v", err)
	}

	version1, err := service.store.RecordFileVersionMutation(context.Background(), "repo", VersionedFileSnapshot{Path: "/notes/fork.txt"}, VersionedFileSnapshot{
		Path:    "/notes/fork.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("one\n"),
	}, FileVersionMutationMetadata{Source: ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation(create) returned error: %v", err)
	}
	version2, err := service.store.RecordFileVersionMutation(context.Background(), "repo", VersionedFileSnapshot{
		Path:    "/notes/fork.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("one\n"),
	}, VersionedFileSnapshot{
		Path:    "/notes/fork.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("two\n"),
	}, FileVersionMutationMetadata{Source: ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation(update) returned error: %v", err)
	}

	if err := service.ForkWorkspace(context.Background(), "repo", "repo-copy"); err != nil {
		t.Fatalf("ForkWorkspace() returned error: %v", err)
	}

	forkPolicy, err := service.store.GetWorkspaceVersioningPolicy(context.Background(), "repo-copy")
	if err != nil {
		t.Fatalf("GetWorkspaceVersioningPolicy(repo-copy) returned error: %v", err)
	}
	if !reflect.DeepEqual(forkPolicy, policy) {
		t.Fatalf("fork policy = %+v, want %+v", forkPolicy, policy)
	}

	forkHistory, err := service.GetFileHistory(context.Background(), "repo-copy", "/notes/fork.txt", false)
	if err != nil {
		t.Fatalf("GetFileHistory(repo-copy) returned error: %v", err)
	}
	if len(forkHistory.Lineages) != 1 || len(forkHistory.Lineages[0].Versions) != 2 {
		t.Fatalf("forkHistory.Lineages = %#v, want one lineage with two versions", forkHistory.Lineages)
	}
	if forkHistory.Lineages[0].FileID != version1.FileID {
		t.Fatalf("fork lineage file_id = %q, want %q", forkHistory.Lineages[0].FileID, version1.FileID)
	}
	if forkHistory.Lineages[0].Versions[0].VersionID != version1.VersionID || forkHistory.Lineages[0].Versions[1].VersionID != version2.VersionID {
		t.Fatalf("fork history versions = %#v, want original version ids", forkHistory.Lineages[0].Versions)
	}

	forkVersion1, err := service.GetFileVersionContent(context.Background(), "repo-copy", version1.VersionID)
	if err != nil {
		t.Fatalf("GetFileVersionContent(repo-copy, version1) returned error: %v", err)
	}
	if forkVersion1.Content != "one\n" {
		t.Fatalf("fork version1 content = %q, want %q", forkVersion1.Content, "one\n")
	}

	version3, err := service.store.RecordFileVersionMutation(context.Background(), "repo-copy", VersionedFileSnapshot{
		Path:    "/notes/fork.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("two\n"),
	}, VersionedFileSnapshot{
		Path:    "/notes/fork.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("three\n"),
	}, FileVersionMutationMetadata{Source: ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation(fork update) returned error: %v", err)
	}
	if version3.Ordinal != 3 {
		t.Fatalf("fork version ordinal = %d, want 3", version3.Ordinal)
	}

	parentHistory, err := service.GetFileHistory(context.Background(), "repo", "/notes/fork.txt", false)
	if err != nil {
		t.Fatalf("GetFileHistory(repo) returned error: %v", err)
	}
	if len(parentHistory.Lineages) != 1 || len(parentHistory.Lineages[0].Versions) != 2 {
		t.Fatalf("parentHistory.Lineages = %#v, want original lineage unchanged", parentHistory.Lineages)
	}

	updatedForkHistory, err := service.GetFileHistory(context.Background(), "repo-copy", "/notes/fork.txt", false)
	if err != nil {
		t.Fatalf("GetFileHistory(repo-copy updated) returned error: %v", err)
	}
	if len(updatedForkHistory.Lineages) != 1 || len(updatedForkHistory.Lineages[0].Versions) != 3 {
		t.Fatalf("updatedForkHistory.Lineages = %#v, want three fork versions", updatedForkHistory.Lineages)
	}
	if updatedForkHistory.Lineages[0].Versions[2].VersionID != version3.VersionID {
		t.Fatalf("fork latest version_id = %q, want %q", updatedForkHistory.Lineages[0].Versions[2].VersionID, version3.VersionID)
	}
}

func TestServiceGetFileVersionContentReadsHistoricalBlob(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	service, _, err := manager.serviceFor(context.Background(), databaseID)
	if err != nil {
		t.Fatalf("serviceFor() returned error: %v", err)
	}
	if err := service.store.PutWorkspaceVersioningPolicy(context.Background(), "repo", WorkspaceVersioningPolicy{
		Mode: WorkspaceVersioningModeAll,
	}); err != nil {
		t.Fatalf("PutWorkspaceVersioningPolicy() returned error: %v", err)
	}

	version, err := service.store.RecordFileVersionMutation(context.Background(), "repo", VersionedFileSnapshot{Path: "/notes/readme.md"}, VersionedFileSnapshot{
		Path:    "/notes/readme.md",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("history body\n"),
	}, FileVersionMutationMetadata{Source: ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation() returned error: %v", err)
	}

	content, err := service.getFileVersionContent(context.Background(), "repo", version.VersionID)
	if err != nil {
		t.Fatalf("getFileVersionContent() returned error: %v", err)
	}
	if content.Content != "history body\n" {
		t.Fatalf("content.Content = %q, want %q", content.Content, "history body\n")
	}
	if content.Ordinal != 1 {
		t.Fatalf("content.Ordinal = %d, want 1", content.Ordinal)
	}
}

func TestServiceDiffFileVersionsAgainstHeadAndWorkingCopy(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	service, _, err := manager.serviceFor(context.Background(), databaseID)
	if err != nil {
		t.Fatalf("serviceFor() returned error: %v", err)
	}
	if err := service.store.PutWorkspaceVersioningPolicy(context.Background(), "repo", WorkspaceVersioningPolicy{
		Mode: WorkspaceVersioningModeAll,
	}); err != nil {
		t.Fatalf("PutWorkspaceVersioningPolicy() returned error: %v", err)
	}

	version, err := service.store.RecordFileVersionMutation(context.Background(), "repo", VersionedFileSnapshot{Path: "/README.md"}, VersionedFileSnapshot{
		Path:    "/README.md",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("historical readme\n"),
	}, FileVersionMutationMetadata{Source: ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation() returned error: %v", err)
	}

	headContent, err := service.getFileContent(context.Background(), "repo", "head", "/README.md")
	if err != nil {
		t.Fatalf("getFileContent(head) returned error: %v", err)
	}

	headDiff, err := service.DiffFileVersions(context.Background(), "repo", "/README.md", FileVersionDiffOperand{
		VersionID: version.VersionID,
	}, FileVersionDiffOperand{
		Ref: "head",
	})
	if err != nil {
		t.Fatalf("DiffFileVersions(head) returned error: %v", err)
	}
	if headDiff.Binary {
		t.Fatal("headDiff.Binary = true, want false")
	}
	if !strings.Contains(headDiff.Diff, "historical readme\n") {
		t.Fatalf("headDiff.Diff = %q, want historical content", headDiff.Diff)
	}
	if !strings.Contains(headDiff.Diff, headContent.Content) {
		t.Fatalf("headDiff.Diff = %q, want head content %q", headDiff.Diff, headContent.Content)
	}

	fsKey, _, _, err := EnsureWorkspaceRoot(context.Background(), service.store, "repo")
	if err != nil {
		t.Fatalf("EnsureWorkspaceRoot() returned error: %v", err)
	}
	if err := client.New(service.store.rdb, fsKey).Echo(context.Background(), "/README.md", []byte("working copy readme\n")); err != nil {
		t.Fatalf("Echo() returned error: %v", err)
	}
	workingCopyDiff, err := service.DiffFileVersions(context.Background(), "repo", "/README.md", FileVersionDiffOperand{
		FileID:  version.FileID,
		Ordinal: version.Ordinal,
	}, FileVersionDiffOperand{
		Ref: "working-copy",
	})
	if err != nil {
		t.Fatalf("DiffFileVersions(working-copy) returned error: %v", err)
	}
	if !strings.Contains(workingCopyDiff.Diff, "working copy readme\n") {
		t.Fatalf("workingCopyDiff.Diff = %q, want working-copy content", workingCopyDiff.Diff)
	}
}

func TestServiceDiffFileVersionsHeadFallsBackToWorkingCopyForLiveOnlyPath(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	service, _, err := manager.serviceFor(context.Background(), databaseID)
	if err != nil {
		t.Fatalf("serviceFor() returned error: %v", err)
	}
	if err := service.store.PutWorkspaceVersioningPolicy(context.Background(), "repo", WorkspaceVersioningPolicy{
		Mode: WorkspaceVersioningModeAll,
	}); err != nil {
		t.Fatalf("PutWorkspaceVersioningPolicy() returned error: %v", err)
	}

	version, err := service.store.RecordFileVersionMutation(context.Background(), "repo", VersionedFileSnapshot{Path: "/drafts/live-only.txt"}, VersionedFileSnapshot{
		Path:    "/drafts/live-only.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("historical draft\n"),
	}, FileVersionMutationMetadata{Source: ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation() returned error: %v", err)
	}

	fsKey, _, _, err := EnsureWorkspaceRoot(context.Background(), service.store, "repo")
	if err != nil {
		t.Fatalf("EnsureWorkspaceRoot() returned error: %v", err)
	}
	fsClient := client.New(service.store.rdb, fsKey)
	if err := fsClient.Mkdir(context.Background(), "/drafts"); err != nil {
		t.Fatalf("Mkdir() returned error: %v", err)
	}
	if err := fsClient.EchoCreate(context.Background(), "/drafts/live-only.txt", []byte("live draft\n"), 0o644); err != nil {
		t.Fatalf("EchoCreate() returned error: %v", err)
	}

	diff, err := service.DiffFileVersions(context.Background(), "repo", "/drafts/live-only.txt", FileVersionDiffOperand{
		VersionID: version.VersionID,
	}, FileVersionDiffOperand{
		Ref: "head",
	})
	if err != nil {
		t.Fatalf("DiffFileVersions(head fallback) returned error: %v", err)
	}
	if !strings.Contains(diff.Diff, "historical draft\n") || !strings.Contains(diff.Diff, "live draft\n") {
		t.Fatalf("diff.Diff = %q, want historical and live working-copy content", diff.Diff)
	}
}

func TestServiceRestoreFileVersionCreatesNewLatestVersion(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	service, _, err := manager.serviceFor(context.Background(), databaseID)
	if err != nil {
		t.Fatalf("serviceFor() returned error: %v", err)
	}
	if err := service.store.PutWorkspaceVersioningPolicy(context.Background(), "repo", WorkspaceVersioningPolicy{
		Mode: WorkspaceVersioningModeAll,
	}); err != nil {
		t.Fatalf("PutWorkspaceVersioningPolicy() returned error: %v", err)
	}

	version, err := service.store.RecordFileVersionMutation(context.Background(), "repo", VersionedFileSnapshot{Path: "/notes/restore.txt"}, VersionedFileSnapshot{
		Path:    "/notes/restore.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("restore me\n"),
	}, FileVersionMutationMetadata{Source: ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation() returned error: %v", err)
	}

	response, err := service.RestoreFileVersion(context.Background(), "repo", "/notes/restore.txt", FileVersionSelector{
		VersionID: version.VersionID,
	})
	if err != nil {
		t.Fatalf("RestoreFileVersion() returned error: %v", err)
	}
	if response.RestoredFromVersionID != version.VersionID {
		t.Fatalf("response.RestoredFromVersionID = %q, want %q", response.RestoredFromVersionID, version.VersionID)
	}
	if response.VersionID == "" || response.VersionID == version.VersionID {
		t.Fatalf("response.VersionID = %q, want new version id", response.VersionID)
	}

	fsKey, _, _, err := EnsureWorkspaceRoot(context.Background(), service.store, "repo")
	if err != nil {
		t.Fatalf("EnsureWorkspaceRoot() returned error: %v", err)
	}
	content, err := client.New(service.store.rdb, fsKey).Cat(context.Background(), "/notes/restore.txt")
	if err != nil {
		t.Fatalf("Cat() returned error: %v", err)
	}
	if string(content) != "restore me\n" {
		t.Fatalf("content = %q, want %q", string(content), "restore me\n")
	}

	versions, err := service.store.ListFileVersions(context.Background(), "repo", version.FileID, true)
	if err != nil {
		t.Fatalf("ListFileVersions() returned error: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("len(versions) = %d, want 2", len(versions))
	}
	if versions[1].Source != ChangeSourceVersionRestore {
		t.Fatalf("versions[1].Source = %q, want %q", versions[1].Source, ChangeSourceVersionRestore)
	}
}

func TestServiceUndeleteFileVersionRevivesDeletedLineage(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	service, _, err := manager.serviceFor(context.Background(), databaseID)
	if err != nil {
		t.Fatalf("serviceFor() returned error: %v", err)
	}
	if err := service.store.PutWorkspaceVersioningPolicy(context.Background(), "repo", WorkspaceVersioningPolicy{
		Mode: WorkspaceVersioningModeAll,
	}); err != nil {
		t.Fatalf("PutWorkspaceVersioningPolicy() returned error: %v", err)
	}

	createVersion, err := service.store.RecordFileVersionMutation(context.Background(), "repo", VersionedFileSnapshot{Path: "/notes/deleted.txt"}, VersionedFileSnapshot{
		Path:    "/notes/deleted.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("deleted body\n"),
	}, FileVersionMutationMetadata{Source: ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation(create) returned error: %v", err)
	}
	if _, err := service.store.RecordFileVersionMutation(context.Background(), "repo", VersionedFileSnapshot{
		Path:    "/notes/deleted.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("deleted body\n"),
	}, VersionedFileSnapshot{
		Path: "/notes/deleted.txt",
	}, FileVersionMutationMetadata{Source: ChangeSourceMCP}); err != nil {
		t.Fatalf("RecordFileVersionMutation(delete) returned error: %v", err)
	}

	response, err := service.UndeleteFileVersion(context.Background(), "repo", "/notes/deleted.txt", FileVersionSelector{})
	if err != nil {
		t.Fatalf("UndeleteFileVersion() returned error: %v", err)
	}
	if response.UndeletedFromVersionID != createVersion.VersionID {
		t.Fatalf("response.UndeletedFromVersionID = %q, want %q", response.UndeletedFromVersionID, createVersion.VersionID)
	}

	lineage, err := service.store.ResolveLiveFileLineageByPath(context.Background(), "repo", "/notes/deleted.txt")
	if err != nil {
		t.Fatalf("ResolveLiveFileLineageByPath() returned error: %v", err)
	}
	if lineage.FileID != createVersion.FileID {
		t.Fatalf("lineage.FileID = %q, want %q", lineage.FileID, createVersion.FileID)
	}

	fsKey, _, _, err := EnsureWorkspaceRoot(context.Background(), service.store, "repo")
	if err != nil {
		t.Fatalf("EnsureWorkspaceRoot() returned error: %v", err)
	}
	content, err := client.New(service.store.rdb, fsKey).Cat(context.Background(), "/notes/deleted.txt")
	if err != nil {
		t.Fatalf("Cat() returned error: %v", err)
	}
	if string(content) != "deleted body\n" {
		t.Fatalf("content = %q, want %q", string(content), "deleted body\n")
	}

	versions, err := service.store.ListFileVersions(context.Background(), "repo", createVersion.FileID, true)
	if err != nil {
		t.Fatalf("ListFileVersions() returned error: %v", err)
	}
	if len(versions) != 3 {
		t.Fatalf("len(versions) = %d, want 3", len(versions))
	}
	if versions[2].Source != ChangeSourceVersionUndelete {
		t.Fatalf("versions[2].Source = %q, want %q", versions[2].Source, ChangeSourceVersionUndelete)
	}
}

func TestStoreRetentionTrimsOldestVersionsPerFile(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	service, _, err := manager.serviceFor(context.Background(), databaseID)
	if err != nil {
		t.Fatalf("serviceFor() returned error: %v", err)
	}
	if err := service.store.PutWorkspaceVersioningPolicy(context.Background(), "repo", WorkspaceVersioningPolicy{
		Mode:               WorkspaceVersioningModeAll,
		MaxVersionsPerFile: 2,
	}); err != nil {
		t.Fatalf("PutWorkspaceVersioningPolicy() returned error: %v", err)
	}

	first, err := service.store.RecordFileVersionMutation(context.Background(), "repo", VersionedFileSnapshot{Path: "/notes/retain.txt"}, VersionedFileSnapshot{
		Path:    "/notes/retain.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("one\n"),
	}, FileVersionMutationMetadata{Source: ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation(first) returned error: %v", err)
	}
	second, err := service.store.RecordFileVersionMutation(context.Background(), "repo", VersionedFileSnapshot{
		Path:    "/notes/retain.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("one\n"),
	}, VersionedFileSnapshot{
		Path:    "/notes/retain.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("two\n"),
	}, FileVersionMutationMetadata{Source: ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation(second) returned error: %v", err)
	}
	third, err := service.store.RecordFileVersionMutation(context.Background(), "repo", VersionedFileSnapshot{
		Path:    "/notes/retain.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("two\n"),
	}, VersionedFileSnapshot{
		Path:    "/notes/retain.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("three\n"),
	}, FileVersionMutationMetadata{Source: ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation(third) returned error: %v", err)
	}

	if _, err := service.store.GetFileVersion(context.Background(), "repo", first.VersionID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("GetFileVersion(first) error = %v, want os.ErrNotExist", err)
	}

	versions, err := service.store.ListFileVersions(context.Background(), "repo", third.FileID, true)
	if err != nil {
		t.Fatalf("ListFileVersions() returned error: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("len(versions) = %d, want 2", len(versions))
	}
	if versions[0].VersionID != second.VersionID || versions[1].VersionID != third.VersionID {
		t.Fatalf("retained versions = [%q %q], want [%q %q]", versions[0].VersionID, versions[1].VersionID, second.VersionID, third.VersionID)
	}

	history, err := service.GetFileHistory(context.Background(), "repo", "/notes/retain.txt", true)
	if err != nil {
		t.Fatalf("GetFileHistory() returned error: %v", err)
	}
	if len(history.Lineages) != 1 || len(history.Lineages[0].Versions) != 2 {
		t.Fatalf("history lineages = %+v, want one lineage with two versions", history.Lineages)
	}
}

func TestStoreRetentionTrimsVersionsOlderThanAgeLimit(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	service, _, err := manager.serviceFor(context.Background(), databaseID)
	if err != nil {
		t.Fatalf("serviceFor() returned error: %v", err)
	}
	if err := service.store.PutWorkspaceVersioningPolicy(context.Background(), "repo", WorkspaceVersioningPolicy{
		Mode:       WorkspaceVersioningModeAll,
		MaxAgeDays: 7,
	}); err != nil {
		t.Fatalf("PutWorkspaceVersioningPolicy() returned error: %v", err)
	}

	lineage, err := service.store.CreateFileLineage(context.Background(), "repo", "/notes/aged.txt", time.Now().UTC().Add(-30*24*time.Hour))
	if err != nil {
		t.Fatalf("CreateFileLineage() returned error: %v", err)
	}

	oldest, err := service.store.AppendFileVersion(context.Background(), "repo", lineage.FileID, FileVersion{
		Path:      "/notes/aged.txt",
		Op:        ChangeOpPut,
		CreatedAt: time.Now().UTC().Add(-20 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("AppendFileVersion(oldest) returned error: %v", err)
	}
	old, err := service.store.AppendFileVersion(context.Background(), "repo", lineage.FileID, FileVersion{
		Path:      "/notes/aged.txt",
		Op:        ChangeOpPut,
		CreatedAt: time.Now().UTC().Add(-10 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("AppendFileVersion(old) returned error: %v", err)
	}
	current, err := service.store.AppendFileVersion(context.Background(), "repo", lineage.FileID, FileVersion{
		Path:      "/notes/aged.txt",
		Op:        ChangeOpPut,
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("AppendFileVersion(current) returned error: %v", err)
	}

	if _, err := service.store.GetFileVersion(context.Background(), "repo", oldest.VersionID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("GetFileVersion(oldest) error = %v, want os.ErrNotExist", err)
	}
	if _, err := service.store.GetFileVersion(context.Background(), "repo", old.VersionID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("GetFileVersion(old) error = %v, want os.ErrNotExist", err)
	}

	versions, err := service.store.ListFileVersions(context.Background(), "repo", lineage.FileID, true)
	if err != nil {
		t.Fatalf("ListFileVersions() returned error: %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("len(versions) = %d, want 1", len(versions))
	}
	if versions[0].VersionID != current.VersionID {
		t.Fatalf("versions[0].VersionID = %q, want %q", versions[0].VersionID, current.VersionID)
	}
}

func TestStoreRetentionTrimsWorkspaceBudgetAcrossLineages(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	service, _, err := manager.serviceFor(context.Background(), databaseID)
	if err != nil {
		t.Fatalf("serviceFor() returned error: %v", err)
	}
	if err := service.store.PutWorkspaceVersioningPolicy(context.Background(), "repo", WorkspaceVersioningPolicy{
		Mode:          WorkspaceVersioningModeAll,
		MaxTotalBytes: 10,
	}); err != nil {
		t.Fatalf("PutWorkspaceVersioningPolicy() returned error: %v", err)
	}

	first, err := service.store.RecordFileVersionMutation(context.Background(), "repo", VersionedFileSnapshot{Path: "/notes/a.txt"}, VersionedFileSnapshot{
		Path:    "/notes/a.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("one\n"),
	}, FileVersionMutationMetadata{Source: ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation(first) returned error: %v", err)
	}
	time.Sleep(2 * time.Millisecond)

	second, err := service.store.RecordFileVersionMutation(context.Background(), "repo", VersionedFileSnapshot{Path: "/notes/b.txt"}, VersionedFileSnapshot{
		Path:    "/notes/b.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("two\n"),
	}, FileVersionMutationMetadata{Source: ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation(second) returned error: %v", err)
	}
	time.Sleep(2 * time.Millisecond)

	third, err := service.store.RecordFileVersionMutation(context.Background(), "repo", VersionedFileSnapshot{
		Path:    "/notes/a.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("one\n"),
	}, VersionedFileSnapshot{
		Path:    "/notes/a.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("three\n"),
	}, FileVersionMutationMetadata{Source: ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation(third) returned error: %v", err)
	}

	if _, err := service.store.GetFileVersion(context.Background(), "repo", first.VersionID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("GetFileVersion(first) error = %v, want os.ErrNotExist", err)
	}

	versionsA, err := service.store.ListFileVersions(context.Background(), "repo", third.FileID, true)
	if err != nil {
		t.Fatalf("ListFileVersions(a) returned error: %v", err)
	}
	if len(versionsA) != 1 || versionsA[0].VersionID != third.VersionID {
		t.Fatalf("versions for a = %+v, want only %q", versionsA, third.VersionID)
	}

	versionsB, err := service.store.ListFileVersions(context.Background(), "repo", second.FileID, true)
	if err != nil {
		t.Fatalf("ListFileVersions(b) returned error: %v", err)
	}
	if len(versionsB) != 1 || versionsB[0].VersionID != second.VersionID {
		t.Fatalf("versions for b = %+v, want only %q", versionsB, second.VersionID)
	}

	_, storageID, err := service.store.resolveWorkspaceMeta(context.Background(), "repo")
	if err != nil {
		t.Fatalf("resolveWorkspaceMeta() returned error: %v", err)
	}
	totalBytes, err := service.store.rdb.Get(context.Background(), workspaceVersionBytesKey(storageID)).Int64()
	if err != nil {
		t.Fatalf("Get(version bytes) returned error: %v", err)
	}
	if totalBytes != 10 {
		t.Fatalf("workspace version bytes = %d, want 10", totalBytes)
	}
}

func TestStoreLargeFileCutoffOmitsHistoricalBlobContent(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	service, _, err := manager.serviceFor(context.Background(), databaseID)
	if err != nil {
		t.Fatalf("serviceFor() returned error: %v", err)
	}
	if err := service.store.PutWorkspaceVersioningPolicy(context.Background(), "repo", WorkspaceVersioningPolicy{
		Mode:                 WorkspaceVersioningModeAll,
		LargeFileCutoffBytes: 4,
	}); err != nil {
		t.Fatalf("PutWorkspaceVersioningPolicy() returned error: %v", err)
	}

	version, err := service.store.RecordFileVersionMutation(context.Background(), "repo", VersionedFileSnapshot{Path: "/notes/large.txt"}, VersionedFileSnapshot{
		Path:    "/notes/large.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("oversized\n"),
	}, FileVersionMutationMetadata{Source: ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation() returned error: %v", err)
	}
	if version.BlobID != "" {
		t.Fatalf("version.BlobID = %q, want empty for guardrailed content", version.BlobID)
	}

	content, err := service.GetFileVersionContent(context.Background(), "repo", version.VersionID)
	if err != nil {
		t.Fatalf("GetFileVersionContent() returned error: %v", err)
	}
	if !content.Binary {
		t.Fatalf("content.Binary = %v, want true", content.Binary)
	}
	if content.Content != "" {
		t.Fatalf("content.Content = %q, want empty", content.Content)
	}

	_, err = service.RestoreFileVersion(context.Background(), "repo", "/notes/large.txt", FileVersionSelector{VersionID: version.VersionID})
	if err == nil || !strings.Contains(err.Error(), "content is unavailable") {
		t.Fatalf("RestoreFileVersion() error = %v, want unavailable-content failure", err)
	}
}
