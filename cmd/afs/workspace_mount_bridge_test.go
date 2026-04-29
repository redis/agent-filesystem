package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/agent-filesystem/internal/controlplane"
	"github.com/redis/agent-filesystem/mount/client"
	"github.com/redis/go-redis/v9"
)

func TestEnsureMountWorkspaceRejectsMissingCurrentWorkspace(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)
	cfg := defaultConfig()
	cfg.RedisAddr = mr.Addr()
	cfg.WorkRoot = t.TempDir()
	cfg.CurrentWorkspace = "newfiles"

	store := newAFSStore(mustRedisClient(t, cfg))
	defer func() { _ = store.rdb.Close() }()

	_, err := ensureMountWorkspace(context.Background(), cfg, store)
	if err == nil {
		t.Fatal("ensureMountWorkspace() returned nil error, want missing workspace error")
	}
	if !strings.Contains(err.Error(), `workspace "newfiles" does not exist`) {
		t.Fatalf("ensureMountWorkspace() error = %q, want missing workspace message", err)
	}
}

func TestSeedWorkspaceMountKeyUsesWorkspaceHeadInsteadOfLocalTree(t *testing.T) {
	t.Helper()

	cfg, store, closeStore := seedWorkspaceMountBridgeFixture(t)
	defer closeStore()

	writeTestFile(t, filepath.Join(afsWorkspaceTreePath(cfg, "repo"), "main.go"), "package local\n")

	ctx := context.Background()
	mountKey, head, initialized, err := seedWorkspaceMountKey(ctx, store, "repo")
	if err != nil {
		t.Fatalf("seedWorkspaceMountKey() returned error: %v", err)
	}
	if !initialized {
		t.Fatal("expected first workspace mount open to initialize the live workspace root")
	}
	if head != "initial" {
		t.Fatalf("head = %q, want %q", head, "initial")
	}
	if mountKey != workspaceRedisKey("repo") {
		t.Fatalf("mountKey = %q, want %q", mountKey, workspaceRedisKey("repo"))
	}

	data, err := client.New(store.rdb, mountKey).Cat(ctx, "/main.go")
	if err != nil {
		t.Fatalf("Cat(/main.go) returned error: %v", err)
	}
	if string(data) != "package main\n" {
		t.Fatalf("mounted main.go = %q, want %q", string(data), "package main\n")
	}

	st, err := client.New(store.rdb, mountKey).Stat(ctx, "/main.go")
	if err != nil {
		t.Fatalf("Stat(/main.go) returned error: %v", err)
	}
	if st == nil || st.Inode == 0 {
		t.Fatalf("expected inode for /main.go, got %+v", st)
	}
}

func TestSeedWorkspaceMountKeyUsesCanonicalAFSKeysOnly(t *testing.T) {
	t.Helper()

	_, store, closeStore := seedWorkspaceMountBridgeFixture(t)
	defer closeStore()

	ctx := context.Background()
	mountKey, _, _, err := seedWorkspaceMountKey(ctx, store, "repo")
	if err != nil {
		t.Fatalf("seedWorkspaceMountKey() returned error: %v", err)
	}

	legacyKeys, err := store.rdb.Keys(ctx, "rfs:{"+mountKey+"}:*").Result()
	if err != nil {
		t.Fatalf("Keys(legacy mount prefix) returned error: %v", err)
	}
	if len(legacyKeys) != 0 {
		t.Fatalf("expected no legacy rfs mount keys, got %v", legacyKeys)
	}

	keys, err := store.rdb.Keys(ctx, "afs:{"+mountKey+"}:*").Result()
	if err != nil {
		t.Fatalf("Keys(afs mount prefix) returned error: %v", err)
	}
	if len(keys) == 0 {
		t.Fatal("expected canonical afs mount keys to exist")
	}

	for _, key := range keys {
		if key == "afs:{"+mountKey+"}:inode:/" {
			t.Fatalf("unexpected legacy path-keyed root inode key: %s", key)
		}
	}

	rootExists, err := store.rdb.Exists(ctx, "afs:{"+mountKey+"}:inode:1").Result()
	if err != nil {
		t.Fatalf("Exists(canonical root inode) returned error: %v", err)
	}
	if rootExists != 1 {
		t.Fatal("expected canonical root inode key afs:{<mount>}:inode:1")
	}
}

func TestSeedWorkspaceMountKeyKeepsExistingLiveWorkspaceRoot(t *testing.T) {
	t.Helper()

	_, store, closeStore := seedWorkspaceMountBridgeFixture(t)
	defer closeStore()

	ctx := context.Background()
	mountKey, _, initialized, err := seedWorkspaceMountKey(ctx, store, "repo")
	if err != nil {
		t.Fatalf("seedWorkspaceMountKey() returned error: %v", err)
	}
	if !initialized {
		t.Fatal("expected first workspace mount open to initialize the live workspace root")
	}

	fsClient := client.New(store.rdb, mountKey)
	if err := fsClient.Echo(ctx, "/live.txt", []byte("live change\n")); err != nil {
		t.Fatalf("Echo(/live.txt) returned error: %v", err)
	}

	secondKey, secondHead, initializedAgain, err := seedWorkspaceMountKey(ctx, store, "repo")
	if err != nil {
		t.Fatalf("second seedWorkspaceMountKey() returned error: %v", err)
	}
	if initializedAgain {
		t.Fatal("expected repeated workspace mount open to reuse the live workspace root")
	}
	if secondKey != mountKey {
		t.Fatalf("second mountKey = %q, want %q", secondKey, mountKey)
	}
	if secondHead != "initial" {
		t.Fatalf("second head = %q, want %q", secondHead, "initial")
	}

	data, err := fsClient.Cat(ctx, "/live.txt")
	if err != nil {
		t.Fatalf("Cat(/live.txt) returned error: %v", err)
	}
	if string(data) != "live change\n" {
		t.Fatalf("live.txt = %q, want %q", string(data), "live change\n")
	}
}

func TestSeedWorkspaceMountKeyRepairsWorkspaceRootWithoutReadyMarker(t *testing.T) {
	t.Helper()

	_, store, closeStore := seedWorkspaceMountBridgeFixture(t)
	defer closeStore()

	ctx := context.Background()
	fsClient := client.New(store.rdb, workspaceRedisKey("repo"))
	if err := fsClient.Echo(ctx, "/stale.txt", []byte("stale\n")); err != nil {
		t.Fatalf("Echo(/stale.txt) returned error: %v", err)
	}

	mountKey, head, initialized, err := seedWorkspaceMountKey(ctx, store, "repo")
	if err != nil {
		t.Fatalf("seedWorkspaceMountKey() returned error: %v", err)
	}
	if !initialized {
		t.Fatal("expected workspace mount open to repair an unmarked live workspace root")
	}
	if head != "initial" {
		t.Fatalf("head = %q, want %q", head, "initial")
	}

	staleStat, err := client.New(store.rdb, mountKey).Stat(ctx, "/stale.txt")
	if err != nil {
		t.Fatalf("Stat(/stale.txt) returned error: %v", err)
	}
	if staleStat != nil {
		t.Fatalf("expected stale.txt to be cleared during root repair, got %+v", staleStat)
	}

	data, err := client.New(store.rdb, mountKey).Cat(ctx, "/main.go")
	if err != nil {
		t.Fatalf("Cat(/main.go) returned error: %v", err)
	}
	if string(data) != "package main\n" {
		t.Fatalf("mounted main.go = %q, want %q", string(data), "package main\n")
	}

	rootHead, err := store.rdb.Get(ctx, "afs:{repo}:root_head_savepoint").Result()
	if err != nil {
		t.Fatalf("Get(root_head_savepoint) returned error: %v", err)
	}
	if rootHead != "initial" {
		t.Fatalf("root_head_savepoint = %q, want %q", rootHead, "initial")
	}
}

func TestSaveWorkspaceRootCheckpointSavesLiveWorkspaceChangesIntoWorkspace(t *testing.T) {
	t.Helper()

	cfg, store, closeStore := seedWorkspaceMountBridgeFixture(t)
	defer closeStore()

	ctx := context.Background()
	mountKey, head, _, err := seedWorkspaceMountKey(ctx, store, "repo")
	if err != nil {
		t.Fatalf("seedWorkspaceMountKey() returned error: %v", err)
	}

	fsClient := client.New(store.rdb, mountKey)
	if err := fsClient.Echo(ctx, "/mounted.txt", []byte("hello from mount\n")); err != nil {
		t.Fatalf("Echo(/mounted.txt) returned error: %v", err)
	}

	saved, err := saveWorkspaceRootCheckpoint(ctx, store, "repo", head, "mounted-save")
	if err != nil {
		t.Fatalf("saveWorkspaceRootCheckpoint() returned error: %v", err)
	}
	if !saved {
		t.Fatal("expected mounted workspace sync to create a new savepoint")
	}

	workspaceMeta, err := store.getWorkspaceMeta(ctx, "repo")
	if err != nil {
		t.Fatalf("getWorkspaceMeta(repo) returned error: %v", err)
	}
	if workspaceMeta.HeadSavepoint == head {
		t.Fatalf("HeadSavepoint = %q, want a new savepoint after mounted edits", workspaceMeta.HeadSavepoint)
	}

	manifest, err := store.getManifest(ctx, "repo", workspaceMeta.HeadSavepoint)
	if err != nil {
		t.Fatalf("getManifest(new head) returned error: %v", err)
	}
	entry, ok := manifest.Entries["/mounted.txt"]
	if !ok {
		t.Fatal("expected /mounted.txt in saved workspace manifest")
	}
	data, err := controlplane.ManifestEntryData(entry, func(blobID string) ([]byte, error) {
		return store.getBlob(ctx, "repo", blobID)
	})
	if err != nil {
		t.Fatalf("ManifestEntryData(/mounted.txt) returned error: %v", err)
	}
	if string(data) != "hello from mount\n" {
		t.Fatalf("saved /mounted.txt = %q, want %q", string(data), "hello from mount\n")
	}
	if _, err := os.Stat(afsWorkspaceTreePath(cfg, "repo")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no local workspace tree after mounted sync, stat err = %v", err)
	}
}

func TestSaveWorkspaceRootCheckpointIgnoresMountedSystemArtifacts(t *testing.T) {
	t.Helper()

	cfg, store, closeStore := seedWorkspaceMountBridgeFixture(t)
	defer closeStore()

	ctx := context.Background()
	mountKey, head, _, err := seedWorkspaceMountKey(ctx, store, "repo")
	if err != nil {
		t.Fatalf("seedWorkspaceMountKey() returned error: %v", err)
	}

	fsClient := client.New(store.rdb, mountKey)
	if err := fsClient.Touch(ctx, "/.nfs-check"); err != nil {
		t.Fatalf("Touch(/.nfs-check) returned error: %v", err)
	}
	if err := fsClient.Echo(ctx, "/._root.txt", []byte("artifact")); err != nil {
		t.Fatalf("Echo(/._root.txt) returned error: %v", err)
	}

	saved, err := saveWorkspaceRootCheckpoint(ctx, store, "repo", head, "artifact-only")
	if err != nil {
		t.Fatalf("saveWorkspaceRootCheckpoint() returned error: %v", err)
	}
	if saved {
		t.Fatal("expected mounted system artifacts to be ignored during sync")
	}

	workspaceMeta, err := store.getWorkspaceMeta(ctx, "repo")
	if err != nil {
		t.Fatalf("getWorkspaceMeta(repo) returned error: %v", err)
	}
	if workspaceMeta.HeadSavepoint != head {
		t.Fatalf("HeadSavepoint = %q, want %q when only ignored mount artifacts changed", workspaceMeta.HeadSavepoint, head)
	}
	if _, err := os.Stat(afsWorkspaceTreePath(cfg, "repo")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no local workspace tree after artifact-only sync, stat err = %v", err)
	}
}

func TestObservedMountWritesRecordOrderedVersionHistory(t *testing.T) {
	t.Helper()

	_, store, closeStore := seedWorkspaceMountBridgeFixture(t)
	defer closeStore()

	ctx := context.Background()
	mountKey, _, _, err := seedWorkspaceMountKey(ctx, store, "repo")
	if err != nil {
		t.Fatalf("seedWorkspaceMountKey() returned error: %v", err)
	}
	if err := store.cp.PutWorkspaceVersioningPolicy(ctx, "repo", controlplane.WorkspaceVersioningPolicy{
		Mode: controlplane.WorkspaceVersioningModeAll,
	}); err != nil {
		t.Fatalf("PutWorkspaceVersioningPolicy() returned error: %v", err)
	}

	fsClient := client.NewWithObserver(store.rdb, mountKey, controlplane.NewMountVersionObserver(store.rdb))
	service := controlplane.NewService(controlplane.Config{}, store.cp)

	if err := fsClient.Echo(ctx, "/history.txt", []byte("one\n")); err != nil {
		t.Fatalf("Echo(create) returned error: %v", err)
	}
	if err := fsClient.Echo(ctx, "/history.txt", []byte("two\n")); err != nil {
		t.Fatalf("Echo(update) returned error: %v", err)
	}
	if err := fsClient.Chmod(ctx, "/history.txt", 0o755); err != nil {
		t.Fatalf("Chmod() returned error: %v", err)
	}
	if err := fsClient.Rename(ctx, "/history.txt", "/renamed.txt", 0); err != nil {
		t.Fatalf("Rename() returned error: %v", err)
	}
	if err := fsClient.Rm(ctx, "/renamed.txt"); err != nil {
		t.Fatalf("Rm() returned error: %v", err)
	}
	if err := fsClient.Echo(ctx, "/renamed.txt", []byte("reborn\n")); err != nil {
		t.Fatalf("Echo(recreate) returned error: %v", err)
	}

	oldHistory, err := service.GetFileHistory(ctx, "repo", "/history.txt", false)
	if err != nil {
		t.Fatalf("GetFileHistory(/history.txt) returned error: %v", err)
	}
	if len(oldHistory.Lineages) != 1 {
		t.Fatalf("len(oldHistory.Lineages) = %d, want 1", len(oldHistory.Lineages))
	}
	oldVersions := oldHistory.Lineages[0].Versions
	if len(oldVersions) != 4 {
		t.Fatalf("len(oldVersions) = %d, want 4", len(oldVersions))
	}
	oldOps := []string{
		oldVersions[0].Op,
		oldVersions[1].Op,
		oldVersions[2].Op,
		oldVersions[3].Op,
	}
	if !reflect.DeepEqual(oldOps, []string{
		controlplane.ChangeOpPut,
		controlplane.ChangeOpPut,
		controlplane.ChangeOpChmod,
		controlplane.ChangeOpRename,
	}) {
		t.Fatalf("old path ops = %#v", oldOps)
	}
	for index, version := range oldVersions {
		if version.Ordinal != int64(index+1) {
			t.Fatalf("oldVersions[%d].Ordinal = %d, want %d", index, version.Ordinal, index+1)
		}
		if version.Source != controlplane.ChangeSourceMount {
			t.Fatalf("oldVersions[%d].Source = %q, want %q", index, version.Source, controlplane.ChangeSourceMount)
		}
	}
	if oldVersions[3].PrevPath != "/history.txt" || oldVersions[3].Path != "/renamed.txt" {
		t.Fatalf("rename version = %+v, want prev_path=/history.txt path=/renamed.txt", oldVersions[3])
	}

	newHistory, err := service.GetFileHistory(ctx, "repo", "/renamed.txt", false)
	if err != nil {
		t.Fatalf("GetFileHistory(/renamed.txt) returned error: %v", err)
	}
	if len(newHistory.Lineages) != 2 {
		t.Fatalf("len(newHistory.Lineages) = %d, want 2", len(newHistory.Lineages))
	}
	deletedLineage := newHistory.Lineages[0]
	recreatedLineage := newHistory.Lineages[1]
	if deletedLineage.FileID == recreatedLineage.FileID {
		t.Fatalf("expected recreate to allocate a new lineage, got shared file_id %q", deletedLineage.FileID)
	}
	if deletedLineage.State != controlplane.FileLineageStateDeleted {
		t.Fatalf("deletedLineage.State = %q, want %q", deletedLineage.State, controlplane.FileLineageStateDeleted)
	}
	if recreatedLineage.State != controlplane.FileLineageStateLive {
		t.Fatalf("recreatedLineage.State = %q, want %q", recreatedLineage.State, controlplane.FileLineageStateLive)
	}
	if len(deletedLineage.Versions) != 2 {
		t.Fatalf("len(deletedLineage.Versions) = %d, want 2", len(deletedLineage.Versions))
	}
	if len(recreatedLineage.Versions) != 1 {
		t.Fatalf("len(recreatedLineage.Versions) = %d, want 1", len(recreatedLineage.Versions))
	}
	if deletedLineage.Versions[0].Op != controlplane.ChangeOpRename || deletedLineage.Versions[1].Op != controlplane.ChangeOpDelete {
		t.Fatalf("deleted lineage ops = [%q %q], want [%q %q]",
			deletedLineage.Versions[0].Op,
			deletedLineage.Versions[1].Op,
			controlplane.ChangeOpRename,
			controlplane.ChangeOpDelete,
		)
	}
	if recreatedLineage.Versions[0].Op != controlplane.ChangeOpPut {
		t.Fatalf("recreatedLineage.Versions[0].Op = %q, want %q", recreatedLineage.Versions[0].Op, controlplane.ChangeOpPut)
	}
	if recreatedLineage.Versions[0].Ordinal != 1 {
		t.Fatalf("recreatedLineage.Versions[0].Ordinal = %d, want 1", recreatedLineage.Versions[0].Ordinal)
	}

	content, err := service.GetFileVersionContent(ctx, "repo", recreatedLineage.Versions[0].VersionID)
	if err != nil {
		t.Fatalf("GetFileVersionContent(recreated) returned error: %v", err)
	}
	if content.Content != "reborn\n" {
		t.Fatalf("recreated version content = %q, want %q", content.Content, "reborn\n")
	}
}

func TestObservedMountRangeAndTextWritesRecordVersionHistory(t *testing.T) {
	t.Helper()

	_, store, closeStore := seedWorkspaceMountBridgeFixture(t)
	defer closeStore()

	ctx := context.Background()
	mountKey, _, _, err := seedWorkspaceMountKey(ctx, store, "repo")
	if err != nil {
		t.Fatalf("seedWorkspaceMountKey() returned error: %v", err)
	}
	if err := store.cp.PutWorkspaceVersioningPolicy(ctx, "repo", controlplane.WorkspaceVersioningPolicy{
		Mode: controlplane.WorkspaceVersioningModeAll,
	}); err != nil {
		t.Fatalf("PutWorkspaceVersioningPolicy() returned error: %v", err)
	}

	fsClient := client.NewWithObserver(store.rdb, mountKey, controlplane.NewMountVersionObserver(store.rdb))
	service := controlplane.NewService(controlplane.Config{}, store.cp)

	if err := fsClient.Echo(ctx, "/range.txt", []byte("one\ntwo\nthree\n")); err != nil {
		t.Fatalf("Echo(create) returned error: %v", err)
	}
	stat, err := fsClient.Stat(ctx, "/range.txt")
	if err != nil {
		t.Fatalf("Stat() returned error: %v", err)
	}
	if stat == nil {
		t.Fatal("expected stat for /range.txt")
	}
	if err := fsClient.WriteInodeAtPath(ctx, stat.Inode, "/range.txt", []byte("ONE"), 0); err != nil {
		t.Fatalf("WriteInodeAtPath() returned error: %v", err)
	}
	if err := fsClient.Insert(ctx, "/range.txt", 1, "middle"); err != nil {
		t.Fatalf("Insert() returned error: %v", err)
	}
	replaced, err := fsClient.Replace(ctx, "/range.txt", "two", "TWO", false)
	if err != nil {
		t.Fatalf("Replace() returned error: %v", err)
	}
	if replaced != 1 {
		t.Fatalf("Replace() count = %d, want 1", replaced)
	}
	deleted, err := fsClient.DeleteLines(ctx, "/range.txt", 4, 4)
	if err != nil {
		t.Fatalf("DeleteLines() returned error: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("DeleteLines() count = %d, want 1", deleted)
	}

	history, err := service.GetFileHistory(ctx, "repo", "/range.txt", false)
	if err != nil {
		t.Fatalf("GetFileHistory(/range.txt) returned error: %v", err)
	}
	if len(history.Lineages) != 1 {
		t.Fatalf("len(history.Lineages) = %d, want 1", len(history.Lineages))
	}
	versions := history.Lineages[0].Versions
	if len(versions) != 5 {
		t.Fatalf("len(versions) = %d, want 5", len(versions))
	}
	for index, version := range versions {
		if version.Ordinal != int64(index+1) {
			t.Fatalf("versions[%d].Ordinal = %d, want %d", index, version.Ordinal, index+1)
		}
		if version.Source != controlplane.ChangeSourceMount {
			t.Fatalf("versions[%d].Source = %q, want %q", index, version.Source, controlplane.ChangeSourceMount)
		}
		if version.Op != controlplane.ChangeOpPut {
			t.Fatalf("versions[%d].Op = %q, want %q", index, version.Op, controlplane.ChangeOpPut)
		}
	}
	content, err := service.GetFileVersionContent(ctx, "repo", versions[len(versions)-1].VersionID)
	if err != nil {
		t.Fatalf("GetFileVersionContent(final) returned error: %v", err)
	}
	if content.Content != "ONE\nmiddle\nTWO\n" {
		t.Fatalf("final range/text content = %q, want %q", content.Content, "ONE\nmiddle\nTWO\n")
	}
}

func TestObservedMountSymlinkLifecycleRecordsVersionHistory(t *testing.T) {
	t.Helper()

	_, store, closeStore := seedWorkspaceMountBridgeFixture(t)
	defer closeStore()

	ctx := context.Background()
	mountKey, _, _, err := seedWorkspaceMountKey(ctx, store, "repo")
	if err != nil {
		t.Fatalf("seedWorkspaceMountKey() returned error: %v", err)
	}
	if err := store.cp.PutWorkspaceVersioningPolicy(ctx, "repo", controlplane.WorkspaceVersioningPolicy{
		Mode: controlplane.WorkspaceVersioningModeAll,
	}); err != nil {
		t.Fatalf("PutWorkspaceVersioningPolicy() returned error: %v", err)
	}

	fsClient := client.NewWithObserver(store.rdb, mountKey, controlplane.NewMountVersionObserver(store.rdb))
	service := controlplane.NewService(controlplane.Config{}, store.cp)

	if err := fsClient.Ln(ctx, "alpha.txt", "/link.txt"); err != nil {
		t.Fatalf("Ln(create) returned error: %v", err)
	}
	if err := fsClient.Rm(ctx, "/link.txt"); err != nil {
		t.Fatalf("Rm(delete symlink) returned error: %v", err)
	}
	if err := fsClient.Ln(ctx, "beta.txt", "/link.txt"); err != nil {
		t.Fatalf("Ln(recreate) returned error: %v", err)
	}

	history, err := service.GetFileHistory(ctx, "repo", "/link.txt", false)
	if err != nil {
		t.Fatalf("GetFileHistory(/link.txt) returned error: %v", err)
	}
	if len(history.Lineages) != 2 {
		t.Fatalf("len(history.Lineages) = %d, want 2", len(history.Lineages))
	}
	firstLineage := history.Lineages[0]
	secondLineage := history.Lineages[1]
	if firstLineage.State != controlplane.FileLineageStateDeleted {
		t.Fatalf("firstLineage.State = %q, want %q", firstLineage.State, controlplane.FileLineageStateDeleted)
	}
	if secondLineage.State != controlplane.FileLineageStateLive {
		t.Fatalf("secondLineage.State = %q, want %q", secondLineage.State, controlplane.FileLineageStateLive)
	}
	if len(firstLineage.Versions) != 2 {
		t.Fatalf("len(firstLineage.Versions) = %d, want 2", len(firstLineage.Versions))
	}
	if len(secondLineage.Versions) != 1 {
		t.Fatalf("len(secondLineage.Versions) = %d, want 1", len(secondLineage.Versions))
	}
	if firstLineage.Versions[0].Kind != controlplane.FileVersionKindSymlink || secondLineage.Versions[0].Kind != controlplane.FileVersionKindSymlink {
		t.Fatalf("expected symlink versions, got first=%q second=%q", firstLineage.Versions[0].Kind, secondLineage.Versions[0].Kind)
	}
	if firstLineage.Versions[0].Target != "alpha.txt" {
		t.Fatalf("first symlink target = %q, want %q", firstLineage.Versions[0].Target, "alpha.txt")
	}
	if secondLineage.Versions[0].Target != "beta.txt" {
		t.Fatalf("second symlink target = %q, want %q", secondLineage.Versions[0].Target, "beta.txt")
	}
}

type failingMutationObserver struct {
	err error
}

func (o failingMutationObserver) RecordMutation(_ context.Context, _ string, _, _ client.VersionedSnapshot) error {
	return o.err
}

func TestObservedMountWritesSurfaceVersioningConflicts(t *testing.T) {
	t.Helper()

	_, store, closeStore := seedWorkspaceMountBridgeFixture(t)
	defer closeStore()

	ctx := context.Background()
	mountKey, _, _, err := seedWorkspaceMountKey(ctx, store, "repo")
	if err != nil {
		t.Fatalf("seedWorkspaceMountKey() returned error: %v", err)
	}

	fsClient := client.NewWithObserver(store.rdb, mountKey, failingMutationObserver{err: controlplane.ErrWorkspaceConflict})
	err = fsClient.Echo(ctx, "/conflict.txt", []byte("content\n"))
	if !errors.Is(err, controlplane.ErrWorkspaceConflict) {
		t.Fatalf("Echo() error = %v, want ErrWorkspaceConflict", err)
	}
}

func seedWorkspaceMountBridgeFixture(t *testing.T) (config, *afsStore, func()) {
	t.Helper()

	mr := miniredis.RunT(t)
	cfg := defaultConfig()
	cfg.RedisAddr = mr.Addr()
	cfg.MountBackend = mountBackendNone
	cfg.WorkRoot = t.TempDir()
	cfg.CurrentWorkspace = "repo"
	saveTempConfig(t, cfg)

	loadedCfg, store, closeStore, err := openAFSStore(context.Background())
	if err != nil {
		t.Fatalf("openAFSStore() returned error: %v", err)
	}

	sourceDir := t.TempDir()
	writeTestFile(t, filepath.Join(sourceDir, "main.go"), "package main\n")
	seedWorkspaceFromDirectory(t, store, "repo", "initial", sourceDir)
	return loadedCfg, store, closeStore
}

func mustRedisClient(t *testing.T, cfg config) *redis.Client {
	t.Helper()

	rdb := redis.NewClient(buildRedisOptions(cfg, 4))
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		_ = rdb.Close()
		t.Fatalf("Ping() returned error: %v", err)
	}
	return rdb
}
