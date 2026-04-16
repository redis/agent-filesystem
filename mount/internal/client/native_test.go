package client

import (
	"context"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

type commandCountHook struct {
	track func(cmd redis.Cmder)
}

func (h *commandCountHook) DialHook(next redis.DialHook) redis.DialHook {
	return next
}

func (h *commandCountHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if h.track != nil {
			h.track(cmd)
		}
		return next(ctx, cmd)
	}
}

func (h *commandCountHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		if h.track != nil {
			for _, cmd := range cmds {
				h.track(cmd)
			}
		}
		return next(ctx, cmds)
	}
}

func countRootHMGet(cmd redis.Cmder, rootKey string, count *atomic.Int64) {
	args := cmd.Args()
	if len(args) < 2 {
		return
	}
	name, ok := args[0].(string)
	if !ok || !strings.EqualFold(name, "hmget") {
		return
	}
	key, ok := args[1].(string)
	if !ok || key != rootKey {
		return
	}
	count.Add(1)
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func setupTestRedis(t *testing.T) (*redis.Client, context.Context) {
	t.Helper()

	port := freeTCPPort(t)
	cmd := exec.Command(
		"redis-server",
		"--port", strconv.Itoa(port),
		"--save", "",
		"--appendonly", "no",
	)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start redis-server: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:" + strconv.Itoa(port)})
	t.Cleanup(func() { _ = rdb.Close() })

	deadline := time.Now().Add(5 * time.Second)
	for {
		if err := rdb.Ping(ctx).Err(); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("redis-server did not become ready")
		}
		time.Sleep(50 * time.Millisecond)
	}

	return rdb, ctx
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate port: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// ---------------------------------------------------------------------------
// Smoke test (original compat test, adapted)
// ---------------------------------------------------------------------------

func TestNativeBackendSmoke(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "smoke")

	if err := c.Mkdir(ctx, "/a/b"); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := c.Echo(ctx, "/a/b/file.txt", []byte("hello")); err != nil {
		t.Fatalf("echo: %v", err)
	}
	if err := c.EchoAppend(ctx, "/a/b/file.txt", []byte(" world")); err != nil {
		t.Fatalf("echo append: %v", err)
	}
	data, err := c.Cat(ctx, "/a/b/file.txt")
	if err != nil {
		t.Fatalf("cat: %v", err)
	}
	if string(data) != "hello world" {
		t.Fatalf("unexpected content: %q", string(data))
	}

	if err := c.Truncate(ctx, "/a/b/file.txt", 5); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	data, err = c.Cat(ctx, "/a/b/file.txt")
	if err != nil {
		t.Fatalf("cat after truncate: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("unexpected truncated content: %q", string(data))
	}

	if err := c.Ln(ctx, "../b/file.txt", "/a/link"); err != nil {
		t.Fatalf("ln: %v", err)
	}
	target, err := c.Readlink(ctx, "/a/link")
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if target != "../b/file.txt" {
		t.Fatalf("unexpected readlink target: %q", target)
	}

	if err := c.Mv(ctx, "/a/b/file.txt", "/a/b/file2.txt"); err != nil {
		t.Fatalf("mv: %v", err)
	}
	if _, err := c.Cat(ctx, "/a/b/file.txt"); err == nil {
		t.Fatal("expected old path to be missing after move")
	}
	if _, err := c.Cat(ctx, "/a/b/file2.txt"); err != nil {
		t.Fatalf("cat new path after move: %v", err)
	}

	entries, err := c.LsLong(ctx, "/a/b")
	if err != nil {
		t.Fatalf("ls long: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "file2.txt" {
		t.Fatalf("unexpected ls entries: %+v", entries)
	}

	info, err := c.Info(ctx)
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	if info.Files < 1 || info.Directories < 1 {
		t.Fatalf("unexpected info: %+v", info)
	}
}

func TestMutationsMarkRootDirtyButReadsDoNot(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "dirty")
	keys := newKeyBuilder("dirty")

	if err := c.Echo(ctx, "/note.txt", []byte("hello")); err != nil {
		t.Fatalf("Echo() returned error: %v", err)
	}
	rootDirty, err := rdb.Get(ctx, keys.rootDirty()).Result()
	if err != nil {
		t.Fatalf("Get(rootDirty) returned error: %v", err)
	}
	if rootDirty != "1" {
		t.Fatalf("rootDirty = %q, want %q", rootDirty, "1")
	}

	if err := rdb.Set(ctx, keys.rootDirty(), "0", 0).Err(); err != nil {
		t.Fatalf("Set(rootDirty) returned error: %v", err)
	}
	if _, err := c.Cat(ctx, "/note.txt"); err != nil {
		t.Fatalf("Cat() returned error: %v", err)
	}
	rootDirty, err = rdb.Get(ctx, keys.rootDirty()).Result()
	if err != nil {
		t.Fatalf("Get(rootDirty after cat) returned error: %v", err)
	}
	if rootDirty != "0" {
		t.Fatalf("rootDirty after cat = %q, want %q", rootDirty, "0")
	}
}

func TestFileWritesUpdateSearchFields(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "searchfields")

	if err := c.Echo(ctx, "/note.txt", []byte("hello world")); err != nil {
		t.Fatalf("Echo() returned error: %v", err)
	}
	if err := c.EchoAppend(ctx, "/note.txt", []byte("\nHELLO")); err != nil {
		t.Fatalf("EchoAppend() returned error: %v", err)
	}

	stat, err := c.Stat(ctx, "/note.txt")
	if err != nil {
		t.Fatalf("Stat() returned error: %v", err)
	}
	if stat == nil {
		t.Fatal("expected stat for /note.txt")
	}

	inodeKey := "afs:{searchfields}:inode:" + strconv.FormatUint(stat.Inode, 10)
	assertSearchFields := func(wantContent string) {
		t.Helper()

		want := buildFileSearchFields(wantContent)
		searchState, err := rdb.HGet(ctx, inodeKey, "search_state").Result()
		if err != nil {
			t.Fatalf("HGet(search_state) returned error: %v", err)
		}
		if searchState != want.SearchState {
			t.Fatalf("search_state = %q, want %q", searchState, want.SearchState)
		}

		grepGrams, err := rdb.HGet(ctx, inodeKey, "grep_grams_ci").Result()
		if err != nil {
			t.Fatalf("HGet(grep_grams_ci) returned error: %v", err)
		}
		if grepGrams != want.GrepGramsCI {
			t.Fatalf("grep_grams_ci = %q, want %q", grepGrams, want.GrepGramsCI)
		}
	}

	assertSearchFields("hello world\nHELLO")

	if err := c.Truncate(ctx, "/note.txt", 5); err != nil {
		t.Fatalf("Truncate() returned error: %v", err)
	}
	assertSearchFields("hello")
}

// ---------------------------------------------------------------------------
// Canonical inode storage verification
// ---------------------------------------------------------------------------

func TestCanonicalInodeStorageFormat(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "keytest")

	if err := c.Echo(ctx, "/hello.txt", []byte("world")); err != nil {
		t.Fatalf("echo: %v", err)
	}

	st, err := c.Stat(ctx, "/hello.txt")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if st == nil || st.Inode == 0 {
		t.Fatalf("expected stable inode id, got %+v", st)
	}

	inodeID := strconv.FormatUint(st.Inode, 10)
	inodeKey := "afs:{keytest}:inode:" + inodeID
	vals, err := rdb.HGetAll(ctx, inodeKey).Result()
	if err != nil {
		t.Fatalf("hgetall: %v", err)
	}
	if len(vals) == 0 {
		t.Fatalf("expected inode hash at %q, got empty", inodeKey)
	}
	if vals["type"] != "file" {
		t.Fatalf("expected type=file, got %q", vals["type"])
	}
	// Content is stored in a separate STRING key, not the inode HASH.
	if vals["content_ref"] != "ext" {
		t.Fatalf("expected content_ref=ext, got %q", vals["content_ref"])
	}
	contentKey := "afs:{keytest}:content:" + inodeID
	contentVal, err := rdb.Get(ctx, contentKey).Result()
	if err != nil {
		t.Fatalf("get content key: %v", err)
	}
	if contentVal != "world" {
		t.Fatalf("expected content=world in content key, got %q", contentVal)
	}
	if vals["parent"] != rootInodeID {
		t.Fatalf("expected parent=%s, got %q", rootInodeID, vals["parent"])
	}
	if vals["name"] != "hello.txt" {
		t.Fatalf("expected name=hello.txt, got %q", vals["name"])
	}
	if vals["path"] != "/hello.txt" {
		t.Fatalf("expected path=/hello.txt, got %q", vals["path"])
	}
	if vals["path_ancestors"] != "/hello.txt" {
		t.Fatalf("expected path_ancestors=/hello.txt, got %q", vals["path_ancestors"])
	}

	rootDirentsKey := "afs:{keytest}:dirents:" + rootInodeID
	childID, err := rdb.HGet(ctx, rootDirentsKey, "hello.txt").Result()
	if err != nil {
		t.Fatalf("hget dirent: %v", err)
	}
	if childID != inodeID {
		t.Fatalf("expected dirent hello.txt -> %s, got %q", inodeID, childID)
	}

	infoKey := "afs:{keytest}:info"
	infoVals, err := rdb.HGetAll(ctx, infoKey).Result()
	if err != nil {
		t.Fatalf("hgetall info: %v", err)
	}
	if infoVals["files"] != "1" {
		t.Fatalf("expected files=1, got %q", infoVals["files"])
	}
	if infoVals["directories"] != "1" {
		t.Fatalf("expected directories=1, got %q", infoVals["directories"])
	}
	if infoVals["schema_version"] != schemaVersion {
		t.Fatalf("expected schema_version=%s, got %q", schemaVersion, infoVals["schema_version"])
	}

	nextInode, err := rdb.Get(ctx, "afs:{keytest}:next_inode").Result()
	if err != nil {
		t.Fatalf("get next_inode: %v", err)
	}
	if nextInode != inodeID {
		t.Fatalf("expected next inode counter to be %s, got %q", inodeID, nextInode)
	}
}

func TestRenamePreservesStableInode(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "rename")

	if err := c.Mkdir(ctx, "/src"); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := c.Mkdir(ctx, "/dst"); err != nil {
		t.Fatalf("mkdir dst: %v", err)
	}
	if err := c.Echo(ctx, "/src/file.txt", []byte("world")); err != nil {
		t.Fatalf("echo: %v", err)
	}
	srcDir, err := c.Stat(ctx, "/src")
	if err != nil {
		t.Fatalf("stat src dir: %v", err)
	}
	if srcDir == nil || srcDir.Inode == 0 {
		t.Fatalf("expected src dir inode, got %+v", srcDir)
	}

	before, err := c.Stat(ctx, "/src/file.txt")
	if err != nil {
		t.Fatalf("stat before rename: %v", err)
	}
	if before == nil || before.Inode == 0 {
		t.Fatalf("expected inode before rename, got %+v", before)
	}

	if err := c.Mv(ctx, "/src/file.txt", "/dst/renamed.txt"); err != nil {
		t.Fatalf("mv: %v", err)
	}

	after, err := c.Stat(ctx, "/dst/renamed.txt")
	if err != nil {
		t.Fatalf("stat after rename: %v", err)
	}
	if after == nil {
		t.Fatal("expected renamed file to exist")
	}
	if before.Inode != after.Inode {
		t.Fatalf("expected stable inode across rename: before=%d after=%d", before.Inode, after.Inode)
	}

	oldPath, err := c.Stat(ctx, "/src/file.txt")
	if err != nil {
		t.Fatalf("stat old path: %v", err)
	}
	if oldPath != nil {
		t.Fatalf("expected old path to be gone, got %+v", oldPath)
	}

	dstDir, err := c.Stat(ctx, "/dst")
	if err != nil {
		t.Fatalf("stat dst dir: %v", err)
	}
	if dstDir == nil || dstDir.Inode == 0 {
		t.Fatalf("expected dst dir inode, got %+v", dstDir)
	}

	inodeKey := "afs:{rename}:inode:" + strconv.FormatUint(after.Inode, 10)
	vals, err := rdb.HGetAll(ctx, inodeKey).Result()
	if err != nil {
		t.Fatalf("hgetall renamed inode: %v", err)
	}
	if vals["parent"] != strconv.FormatUint(dstDir.Inode, 10) {
		t.Fatalf("expected renamed inode parent=%d, got %q", dstDir.Inode, vals["parent"])
	}
	if vals["name"] != "renamed.txt" {
		t.Fatalf("expected renamed inode name=renamed.txt, got %q", vals["name"])
	}

	srcDirents := "afs:{rename}:dirents:" + strconv.FormatUint(srcDir.Inode, 10)
	if exists, err := rdb.HExists(ctx, srcDirents, "file.txt").Result(); err != nil {
		t.Fatalf("hexists old dirent: %v", err)
	} else if exists {
		t.Fatal("expected old dir entry to be removed")
	}

	dstDirents := "afs:{rename}:dirents:" + strconv.FormatUint(dstDir.Inode, 10)
	dstChild, err := rdb.HGet(ctx, dstDirents, "renamed.txt").Result()
	if err != nil {
		t.Fatalf("hget new dirent: %v", err)
	}
	if dstChild != strconv.FormatUint(after.Inode, 10) {
		t.Fatalf("expected new dirent to point at inode %d, got %q", after.Inode, dstChild)
	}
}

func TestCreateFileExclusive(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "create-exclusive")

	st, created, err := c.CreateFile(ctx, "/lock.txt", 0o640, true)
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	if !created {
		t.Fatal("expected file to be newly created")
	}
	if st == nil || st.Mode != 0o640 {
		t.Fatalf("unexpected stat after create: %+v", st)
	}

	if _, created, err := c.CreateFile(ctx, "/lock.txt", 0o600, false); err != nil {
		t.Fatalf("reopen create file: %v", err)
	} else if created {
		t.Fatal("expected existing file to be reused")
	}

	if _, _, err := c.CreateFile(ctx, "/lock.txt", 0o600, true); err == nil {
		t.Fatal("expected exclusive create on existing file to fail")
	}
}

func TestRenameReplacesExistingFile(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "rename-replace")

	if err := c.Echo(ctx, "/src.txt", []byte("src")); err != nil {
		t.Fatalf("echo src: %v", err)
	}
	if err := c.Echo(ctx, "/dst.txt", []byte("dst")); err != nil {
		t.Fatalf("echo dst: %v", err)
	}

	if err := c.Rename(ctx, "/src.txt", "/dst.txt", 0); err != nil {
		t.Fatalf("rename replace: %v", err)
	}

	if st, err := c.Stat(ctx, "/src.txt"); err != nil {
		t.Fatalf("stat old src: %v", err)
	} else if st != nil {
		t.Fatalf("expected old src to be removed, got %+v", st)
	}

	data, err := c.Cat(ctx, "/dst.txt")
	if err != nil {
		t.Fatalf("cat dst after replace: %v", err)
	}
	if string(data) != "src" {
		t.Fatalf("expected replaced content to come from src, got %q", string(data))
	}

	info, err := c.Info(ctx)
	if err != nil {
		t.Fatalf("info after replace: %v", err)
	}
	if info.Files != 1 {
		t.Fatalf("expected file count to remain 1 after replace, got %d", info.Files)
	}
}

func TestRenameNoreplace(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "rename-noreplace")

	if err := c.Echo(ctx, "/src.txt", []byte("src")); err != nil {
		t.Fatalf("echo src: %v", err)
	}
	if err := c.Echo(ctx, "/dst.txt", []byte("dst")); err != nil {
		t.Fatalf("echo dst: %v", err)
	}

	if err := c.Rename(ctx, "/src.txt", "/dst.txt", RenameNoreplace); err == nil {
		t.Fatal("expected rename noreplace to fail")
	}

	srcData, err := c.Cat(ctx, "/src.txt")
	if err != nil {
		t.Fatalf("cat src after noreplace: %v", err)
	}
	if string(srcData) != "src" {
		t.Fatalf("unexpected src content after noreplace: %q", string(srcData))
	}
	dstData, err := c.Cat(ctx, "/dst.txt")
	if err != nil {
		t.Fatalf("cat dst after noreplace: %v", err)
	}
	if string(dstData) != "dst" {
		t.Fatalf("unexpected dst content after noreplace: %q", string(dstData))
	}
}

func TestRenameReplacesEmptyDirectory(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "rename-empty-dir")

	if err := c.Mkdir(ctx, "/src/sub"); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := c.Echo(ctx, "/src/sub/file.txt", []byte("payload")); err != nil {
		t.Fatalf("echo nested file: %v", err)
	}
	if err := c.Mkdir(ctx, "/dst"); err != nil {
		t.Fatalf("mkdir dst: %v", err)
	}

	if err := c.Rename(ctx, "/src", "/dst", 0); err != nil {
		t.Fatalf("rename dir over empty dir: %v", err)
	}

	data, err := c.Cat(ctx, "/dst/sub/file.txt")
	if err != nil {
		t.Fatalf("cat moved nested file: %v", err)
	}
	if string(data) != "payload" {
		t.Fatalf("unexpected nested content after dir replace: %q", string(data))
	}
}

func TestRenameUpdatesIndexedPathsForDirectorySubtree(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "rename-indexed-paths")

	if err := c.Mkdir(ctx, "/src/sub"); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := c.Echo(ctx, "/src/sub/file.txt", []byte("payload")); err != nil {
		t.Fatalf("echo nested file: %v", err)
	}

	before, err := c.Stat(ctx, "/src/sub/file.txt")
	if err != nil {
		t.Fatalf("stat before rename: %v", err)
	}
	if before == nil || before.Inode == 0 {
		t.Fatalf("expected file inode before rename, got %+v", before)
	}

	if err := c.Rename(ctx, "/src", "/dst", 0); err != nil {
		t.Fatalf("rename src to dst: %v", err)
	}

	inodeKey := "afs:{rename-indexed-paths}:inode:" + strconv.FormatUint(before.Inode, 10)
	vals, err := rdb.HGetAll(ctx, inodeKey).Result()
	if err != nil {
		t.Fatalf("hgetall renamed inode: %v", err)
	}
	if vals["path"] != "/dst/sub/file.txt" {
		t.Fatalf("path = %q, want %q", vals["path"], "/dst/sub/file.txt")
	}
	if vals["path_ancestors"] != "/dst,/dst/sub,/dst/sub/file.txt" {
		t.Fatalf("path_ancestors = %q, want %q", vals["path_ancestors"], "/dst,/dst/sub,/dst/sub/file.txt")
	}
}

func TestDirectoryTimestampsChangeOnMutation(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "dir-times")

	if err := c.Mkdir(ctx, "/dir"); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	beforeCreate, err := c.Stat(ctx, "/dir")
	if err != nil {
		t.Fatalf("stat before create: %v", err)
	}

	time.Sleep(2 * time.Millisecond)
	if _, _, err := c.CreateFile(ctx, "/dir/a.txt", 0o644, true); err != nil {
		t.Fatalf("create file: %v", err)
	}
	afterCreate, err := c.Stat(ctx, "/dir")
	if err != nil {
		t.Fatalf("stat after create: %v", err)
	}
	if afterCreate.Mtime <= beforeCreate.Mtime || afterCreate.Ctime <= beforeCreate.Ctime {
		t.Fatalf("dir timestamps after create = mtime=%d ctime=%d, want > mtime=%d ctime=%d", afterCreate.Mtime, afterCreate.Ctime, beforeCreate.Mtime, beforeCreate.Ctime)
	}

	time.Sleep(2 * time.Millisecond)
	if err := c.Rm(ctx, "/dir/a.txt"); err != nil {
		t.Fatalf("remove file: %v", err)
	}
	afterRemove, err := c.Stat(ctx, "/dir")
	if err != nil {
		t.Fatalf("stat after remove: %v", err)
	}
	if afterRemove.Mtime <= afterCreate.Mtime || afterRemove.Ctime <= afterCreate.Ctime {
		t.Fatalf("dir timestamps after remove = mtime=%d ctime=%d, want > mtime=%d ctime=%d", afterRemove.Mtime, afterRemove.Ctime, afterCreate.Mtime, afterCreate.Ctime)
	}
}

func TestRenameTouchesBothParentDirectories(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "rename-dir-times")

	if err := c.Mkdir(ctx, "/src"); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := c.Mkdir(ctx, "/dst"); err != nil {
		t.Fatalf("mkdir dst: %v", err)
	}
	if _, _, err := c.CreateFile(ctx, "/src/a.txt", 0o644, true); err != nil {
		t.Fatalf("create file: %v", err)
	}

	srcBefore, err := c.Stat(ctx, "/src")
	if err != nil {
		t.Fatalf("stat src before rename: %v", err)
	}
	dstBefore, err := c.Stat(ctx, "/dst")
	if err != nil {
		t.Fatalf("stat dst before rename: %v", err)
	}

	time.Sleep(2 * time.Millisecond)
	if err := c.Rename(ctx, "/src/a.txt", "/dst/a.txt", 0); err != nil {
		t.Fatalf("rename: %v", err)
	}

	srcAfter, err := c.Stat(ctx, "/src")
	if err != nil {
		t.Fatalf("stat src after rename: %v", err)
	}
	dstAfter, err := c.Stat(ctx, "/dst")
	if err != nil {
		t.Fatalf("stat dst after rename: %v", err)
	}
	if srcAfter.Mtime <= srcBefore.Mtime || srcAfter.Ctime <= srcBefore.Ctime {
		t.Fatalf("src dir timestamps after rename = mtime=%d ctime=%d, want > mtime=%d ctime=%d", srcAfter.Mtime, srcAfter.Ctime, srcBefore.Mtime, srcBefore.Ctime)
	}
	if dstAfter.Mtime <= dstBefore.Mtime || dstAfter.Ctime <= dstBefore.Ctime {
		t.Fatalf("dst dir timestamps after rename = mtime=%d ctime=%d, want > mtime=%d ctime=%d", dstAfter.Mtime, dstAfter.Ctime, dstBefore.Mtime, dstBefore.Ctime)
	}
}

func TestRenameRejectsNonEmptyDirectoryReplace(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "rename-dir-not-empty")

	if err := c.Mkdir(ctx, "/src"); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := c.Mkdir(ctx, "/dst"); err != nil {
		t.Fatalf("mkdir dst: %v", err)
	}
	if err := c.Echo(ctx, "/dst/file.txt", []byte("keep")); err != nil {
		t.Fatalf("echo dst file: %v", err)
	}

	if err := c.Rename(ctx, "/src", "/dst", 0); err == nil {
		t.Fatal("expected rename over non-empty directory to fail")
	}
}

func TestFileLocksConflictAndUnlockAll(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "locks")

	if err := c.Echo(ctx, "/file.txt", []byte("content")); err != nil {
		t.Fatalf("echo: %v", err)
	}
	st, err := c.Stat(ctx, "/file.txt")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	lockA := &FileLock{Start: 0, End: 99, Type: syscall.F_WRLCK, PID: 1001}
	lockB := &FileLock{Start: 50, End: 120, Type: syscall.F_RDLCK, PID: 1002}

	if err := c.Setlk(ctx, st.Inode, "handle-a", lockA, false); err != nil {
		t.Fatalf("setlk handle-a: %v", err)
	}

	conflict, err := c.Getlk(ctx, st.Inode, "handle-b", lockB)
	if err != nil {
		t.Fatalf("getlk handle-b: %v", err)
	}
	if conflict == nil || conflict.Type != syscall.F_WRLCK {
		t.Fatalf("expected conflicting write lock, got %+v", conflict)
	}

	if err := c.Setlk(ctx, st.Inode, "handle-b", lockB, false); err == nil {
		t.Fatal("expected conflicting lock to fail")
	}

	if err := c.UnlockAll(ctx, st.Inode, "handle-a"); err != nil {
		t.Fatalf("unlock all handle-a: %v", err)
	}
	if err := c.Setlk(ctx, st.Inode, "handle-b", lockB, false); err != nil {
		t.Fatalf("setlk handle-b after unlock: %v", err)
	}
}

func TestFileLocksBlockingWait(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "locks-wait")

	if err := c.Echo(ctx, "/file.txt", []byte("content")); err != nil {
		t.Fatalf("echo: %v", err)
	}
	st, err := c.Stat(ctx, "/file.txt")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	first := &FileLock{Start: 0, End: 99, Type: syscall.F_WRLCK, PID: 1}
	second := &FileLock{Start: 0, End: 99, Type: syscall.F_WRLCK, PID: 2}

	if err := c.Setlk(ctx, st.Inode, "handle-a", first, false); err != nil {
		t.Fatalf("setlk handle-a: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		done <- c.Setlk(waitCtx, st.Inode, "handle-b", second, true)
	}()

	time.Sleep(100 * time.Millisecond)
	if err := c.UnlockAll(ctx, st.Inode, "handle-a"); err != nil {
		t.Fatalf("unlock all handle-a: %v", err)
	}

	if err := <-done; err != nil {
		t.Fatalf("blocking setlk failed: %v", err)
	}
}

func TestInodeRangeIO(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "range-io")

	if err := c.Echo(ctx, "/file.txt", []byte("hello")); err != nil {
		t.Fatalf("echo: %v", err)
	}
	st, err := c.Stat(ctx, "/file.txt")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	inodeKey := "afs:{range-io}:inode:" + strconv.FormatUint(st.Inode, 10)
	assertSearchFields := func(wantContent string) {
		t.Helper()

		want := buildFileSearchFields(wantContent)
		searchState, err := rdb.HGet(ctx, inodeKey, "search_state").Result()
		if err != nil {
			t.Fatalf("HGet(search_state) returned error: %v", err)
		}
		if searchState != want.SearchState {
			t.Fatalf("search_state = %q, want %q", searchState, want.SearchState)
		}

		grepGrams, err := rdb.HGet(ctx, inodeKey, "grep_grams_ci").Result()
		if err != nil {
			t.Fatalf("HGet(grep_grams_ci) returned error: %v", err)
		}
		if grepGrams != want.GrepGramsCI {
			t.Fatalf("grep_grams_ci = %q, want %q", grepGrams, want.GrepGramsCI)
		}
	}

	chunk, err := c.ReadInodeAt(ctx, st.Inode, 1, 3)
	if err != nil {
		t.Fatalf("read inode at: %v", err)
	}
	if string(chunk) != "ell" {
		t.Fatalf("read inode at = %q, want ell", string(chunk))
	}

	if err := c.WriteInodeAt(ctx, st.Inode, []byte("y"), 1); err != nil {
		t.Fatalf("write inode at: %v", err)
	}
	data, err := c.Cat(ctx, "/file.txt")
	if err != nil {
		t.Fatalf("cat after write: %v", err)
	}
	if string(data) != "hyllo" {
		t.Fatalf("content after write = %q", string(data))
	}
	assertSearchFields("hyllo")

	if err := c.TruncateInode(ctx, st.Inode, 3); err != nil {
		t.Fatalf("truncate inode: %v", err)
	}
	data, err = c.Cat(ctx, "/file.txt")
	if err != nil {
		t.Fatalf("cat after truncate: %v", err)
	}
	if string(data) != "hyl" {
		t.Fatalf("content after truncate = %q", string(data))
	}
	assertSearchFields("hyl")
}

func TestInodeWriteRefreshesPathCacheBeforeMetadataUpdate(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := NewWithCache(rdb, "range-cache", time.Hour)

	st, created, err := c.CreateFile(ctx, "/cached.txt", 0o644, false)
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	if !created {
		t.Fatal("expected cached.txt to be created")
	}
	if st == nil {
		t.Fatal("expected stat result for created file")
	}

	initial, err := c.Stat(ctx, "/cached.txt")
	if err != nil {
		t.Fatalf("stat before write: %v", err)
	}
	if initial == nil || initial.Size != 0 {
		t.Fatalf("expected zero-sized cached stat before write, got %+v", initial)
	}

	if err := c.WriteInodeAt(ctx, st.Inode, []byte("hello"), 0); err != nil {
		t.Fatalf("write inode at: %v", err)
	}

	afterWrite, err := c.Stat(ctx, "/cached.txt")
	if err != nil {
		t.Fatalf("stat after write: %v", err)
	}
	if afterWrite == nil || afterWrite.Size != 5 {
		t.Fatalf("expected immediate path stat size 5 after inode write, got %+v", afterWrite)
	}

	now := time.Now().UnixMilli()
	if err := c.Utimens(ctx, "/cached.txt", now, now); err != nil {
		t.Fatalf("utimens after write: %v", err)
	}

	afterMeta, err := c.Stat(ctx, "/cached.txt")
	if err != nil {
		t.Fatalf("stat after metadata update: %v", err)
	}
	if afterMeta == nil || afterMeta.Size != 5 {
		t.Fatalf("expected metadata update to preserve size 5, got %+v", afterMeta)
	}

	data, err := c.Cat(ctx, "/cached.txt")
	if err != nil {
		t.Fatalf("cat after metadata update: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("content after metadata update = %q, want hello", string(data))
	}

	size, err := rdb.HGet(ctx, "afs:{range-cache}:inode:"+strconv.FormatUint(st.Inode, 10), "size").Result()
	if err != nil {
		t.Fatalf("hget size: %v", err)
	}
	if size != "5" {
		t.Fatalf("redis inode size = %q, want 5", size)
	}
}

func TestWarmPathCachePreloadsDeepExactPaths(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	raw := NewWithCache(rdb, "warm-path-cache", time.Hour)
	c, ok := raw.(*nativeClient)
	if !ok {
		t.Fatalf("client type = %T, want *nativeClient", raw)
	}

	if err := c.Echo(ctx, "/projects/example/session/subagents/agent-1.jsonl", []byte("hello")); err != nil {
		t.Fatalf("echo deep file: %v", err)
	}

	c.cache.InvalidateAll()
	for _, p := range []string{
		"/projects",
		"/projects/example",
		"/projects/example/session",
		"/projects/example/session/subagents",
		"/projects/example/session/subagents/agent-1.jsonl",
	} {
		if _, ok := c.cache.Get(p); ok {
			t.Fatalf("expected cold cache miss for %s", p)
		}
	}

	if err := c.WarmPathCache(ctx); err != nil {
		t.Fatalf("WarmPathCache() returned error: %v", err)
	}

	for _, p := range []string{
		"/",
		"/projects",
		"/projects/example",
		"/projects/example/session",
		"/projects/example/session/subagents",
		"/projects/example/session/subagents/agent-1.jsonl",
	} {
		if _, ok := c.cache.Get(p); !ok {
			t.Fatalf("expected warmed cache hit for %s", p)
		}
	}
}

func TestResolvePathUsesCachedRootInode(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	raw := NewWithCache(rdb, "warm-root-cache", time.Hour)
	c, ok := raw.(*nativeClient)
	if !ok {
		t.Fatalf("client type = %T, want *nativeClient", raw)
	}

	if err := c.Echo(ctx, "/projects/example/session/subagents/agent-1.jsonl", []byte("hello")); err != nil {
		t.Fatalf("echo deep file: %v", err)
	}
	if err := c.WarmPathCache(ctx); err != nil {
		t.Fatalf("WarmPathCache() returned error: %v", err)
	}

	rootCached, ok := c.cache.Get("/")
	if !ok {
		t.Fatal("expected warmed cache hit for /")
	}
	c.cache.InvalidateAll()
	c.cache.Set("/", rootCached)

	var rootLoads atomic.Int64
	rdb.AddHook(&commandCountHook{
		track: func(cmd redis.Cmder) {
			countRootHMGet(cmd, c.keys.inode(rootInodeID), &rootLoads)
		},
	})

	if _, err := c.Stat(ctx, "/projects/example/session/subagents/agent-1.jsonl"); err != nil {
		t.Fatalf("stat deep file: %v", err)
	}
	if got := rootLoads.Load(); got != 0 {
		t.Fatalf("expected cached root inode to avoid redis HMGETs, got %d", got)
	}
}

func TestHashTagInKeys(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "htag")

	if err := c.Echo(ctx, "/test.txt", []byte("data")); err != nil {
		t.Fatalf("echo: %v", err)
	}

	// Scan for all keys matching our pattern
	var allKeys []string
	var cursor uint64
	for {
		keys, next, err := rdb.Scan(ctx, cursor, "afs:{htag}:*", 100).Result()
		if err != nil {
			t.Fatalf("scan: %v", err)
		}
		allKeys = append(allKeys, keys...)
		cursor = next
		if cursor == 0 {
			break
		}
	}

	if len(allKeys) == 0 {
		t.Fatal("expected keys matching afs:{htag}:*, got none")
	}
	for _, k := range allKeys {
		if !strings.Contains(k, "{htag}") {
			t.Errorf("key %q does not contain {htag} hash tag", k)
		}
	}
}

// ---------------------------------------------------------------------------
// Text-processing commands
// ---------------------------------------------------------------------------

func TestHeadTailLines(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "text")

	content := "line1\nline2\nline3\nline4\nline5\n"
	if err := c.Echo(ctx, "/file.txt", []byte(content)); err != nil {
		t.Fatalf("echo: %v", err)
	}

	// Head
	h, err := c.Head(ctx, "/file.txt", 3)
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	if h != "line1\nline2\nline3\n" {
		t.Fatalf("head(3) = %q", h)
	}

	// Tail
	tl, err := c.Tail(ctx, "/file.txt", 2)
	if err != nil {
		t.Fatalf("tail: %v", err)
	}
	if tl != "line4\nline5\n" {
		t.Fatalf("tail(2) = %q", tl)
	}

	// Lines (1-indexed)
	l, err := c.Lines(ctx, "/file.txt", 2, 4)
	if err != nil {
		t.Fatalf("lines: %v", err)
	}
	if l != "line2\nline3\nline4\n" {
		t.Fatalf("lines(2,4) = %q", l)
	}

	// Lines end=-1 means EOF
	l2, err := c.Lines(ctx, "/file.txt", 3, -1)
	if err != nil {
		t.Fatalf("lines to EOF: %v", err)
	}
	if l2 != "line3\nline4\nline5\n" {
		t.Fatalf("lines(3,-1) = %q", l2)
	}

	// Head with n > total lines
	h2, err := c.Head(ctx, "/file.txt", 100)
	if err != nil {
		t.Fatalf("head overflow: %v", err)
	}
	if h2 != content {
		t.Fatalf("head(100) = %q, want %q", h2, content)
	}
}

func TestWarmReadHelpersIssueSingleRedisRead(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	fsKey := "read-hot"
	c := NewWithCache(rdb, fsKey, time.Hour)

	content := "line1\nline2\nline3\n"
	if err := c.Echo(ctx, "/docs/note.txt", []byte(content)); err != nil {
		t.Fatalf("echo: %v", err)
	}
	if _, err := c.Stat(ctx, "/docs/note.txt"); err != nil {
		t.Fatalf("warm stat: %v", err)
	}

	var h countHook
	rdb.AddHook(&hookGate{gate: &h, fsKey: fsKey})

	assertSingleCommand := func(name string, run func() error) {
		t.Helper()
		before := h.total.Load()
		if err := run(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := h.total.Load() - before; got != 1 {
			t.Fatalf("%s command count = %d, want 1", name, got)
		}
	}

	assertSingleCommand("Cat", func() error {
		_, err := c.Cat(ctx, "/docs/note.txt")
		return err
	})
	assertSingleCommand("Head", func() error {
		_, err := c.Head(ctx, "/docs/note.txt", 2)
		return err
	})
	assertSingleCommand("Tail", func() error {
		_, err := c.Tail(ctx, "/docs/note.txt", 2)
		return err
	})
	assertSingleCommand("Lines", func() error {
		_, err := c.Lines(ctx, "/docs/note.txt", 2, 3)
		return err
	})
	assertSingleCommand("Wc", func() error {
		_, err := c.Wc(ctx, "/docs/note.txt")
		return err
	})
}

func TestWc(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "wc")

	if err := c.Echo(ctx, "/file.txt", []byte("hello world\nfoo bar baz\n")); err != nil {
		t.Fatalf("echo: %v", err)
	}

	wc, err := c.Wc(ctx, "/file.txt")
	if err != nil {
		t.Fatalf("wc: %v", err)
	}
	if wc.Lines != 2 {
		t.Fatalf("lines = %d, want 2", wc.Lines)
	}
	if wc.Words != 5 {
		t.Fatalf("words = %d, want 5", wc.Words)
	}
	if wc.Chars != 24 {
		t.Fatalf("chars = %d, want 24", wc.Chars)
	}

	// No trailing newline
	if err := c.Echo(ctx, "/notrail.txt", []byte("one two")); err != nil {
		t.Fatalf("echo: %v", err)
	}
	wc2, err := c.Wc(ctx, "/notrail.txt")
	if err != nil {
		t.Fatalf("wc: %v", err)
	}
	if wc2.Lines != 1 {
		t.Fatalf("lines = %d, want 1", wc2.Lines)
	}
}

func TestInsert(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "insert")

	if err := c.Echo(ctx, "/file.txt", []byte("line1\nline2\nline3\n")); err != nil {
		t.Fatalf("echo: %v", err)
	}

	// Insert after line 1
	if err := c.Insert(ctx, "/file.txt", 1, "inserted"); err != nil {
		t.Fatalf("insert: %v", err)
	}
	data, _ := c.Cat(ctx, "/file.txt")
	if string(data) != "line1\ninserted\nline2\nline3\n" {
		t.Fatalf("after insert(1): %q", string(data))
	}

	// Prepend (line 0)
	if err := c.Echo(ctx, "/prepend.txt", []byte("B\n")); err != nil {
		t.Fatalf("echo: %v", err)
	}
	if err := c.Insert(ctx, "/prepend.txt", 0, "A"); err != nil {
		t.Fatalf("insert prepend: %v", err)
	}
	data, _ = c.Cat(ctx, "/prepend.txt")
	if string(data) != "A\nB\n" {
		t.Fatalf("after prepend: %q", string(data))
	}

	// Append (line -1)
	if err := c.Echo(ctx, "/append.txt", []byte("X\n")); err != nil {
		t.Fatalf("echo: %v", err)
	}
	if err := c.Insert(ctx, "/append.txt", -1, "Y"); err != nil {
		t.Fatalf("insert append: %v", err)
	}
	data, _ = c.Cat(ctx, "/append.txt")
	if string(data) != "X\nY\n" {
		t.Fatalf("after append: %q", string(data))
	}
}

func TestReplace(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "replace")

	if err := c.Echo(ctx, "/file.txt", []byte("foo bar foo baz foo")); err != nil {
		t.Fatalf("echo: %v", err)
	}

	// Replace first occurrence
	n, err := c.Replace(ctx, "/file.txt", "foo", "qux", false)
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 replacement, got %d", n)
	}
	data, _ := c.Cat(ctx, "/file.txt")
	if string(data) != "qux bar foo baz foo" {
		t.Fatalf("after replace first: %q", string(data))
	}

	// Replace all
	n, err = c.Replace(ctx, "/file.txt", "foo", "X", true)
	if err != nil {
		t.Fatalf("replace all: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 replacements, got %d", n)
	}
	data, _ = c.Cat(ctx, "/file.txt")
	if string(data) != "qux bar X baz X" {
		t.Fatalf("after replace all: %q", string(data))
	}

	// No match
	n, err = c.Replace(ctx, "/file.txt", "NOPE", "Y", true)
	if err != nil {
		t.Fatalf("replace nomatch: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 replacements, got %d", n)
	}
}

func TestDeleteLines(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "dellines")

	if err := c.Echo(ctx, "/file.txt", []byte("a\nb\nc\nd\ne\n")); err != nil {
		t.Fatalf("echo: %v", err)
	}

	n, err := c.DeleteLines(ctx, "/file.txt", 2, 4)
	if err != nil {
		t.Fatalf("delete lines: %v", err)
	}
	if n != 3 {
		t.Fatalf("expected 3 deleted, got %d", n)
	}
	data, _ := c.Cat(ctx, "/file.txt")
	if string(data) != "a\ne\n" {
		t.Fatalf("after delete lines 2-4: %q", string(data))
	}
}

// ---------------------------------------------------------------------------
// Recursive/walk commands
// ---------------------------------------------------------------------------

func TestCpFile(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "cpfile")

	if err := c.Echo(ctx, "/src.txt", []byte("content")); err != nil {
		t.Fatalf("echo: %v", err)
	}
	if err := c.Chmod(ctx, "/src.txt", 0o600); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	if err := c.Cp(ctx, "/src.txt", "/dst.txt", false); err != nil {
		t.Fatalf("cp: %v", err)
	}

	data, err := c.Cat(ctx, "/dst.txt")
	if err != nil {
		t.Fatalf("cat dst: %v", err)
	}
	if string(data) != "content" {
		t.Fatalf("unexpected content: %q", string(data))
	}

	st, err := c.Stat(ctx, "/dst.txt")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if st.Mode != 0o600 {
		t.Fatalf("mode = %o, want 600", st.Mode)
	}
}

func TestCpDirectory(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "cpdir")

	if err := c.Mkdir(ctx, "/src/sub"); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := c.Echo(ctx, "/src/a.txt", []byte("A")); err != nil {
		t.Fatalf("echo: %v", err)
	}
	if err := c.Echo(ctx, "/src/sub/b.txt", []byte("B")); err != nil {
		t.Fatalf("echo: %v", err)
	}
	if err := c.Ln(ctx, "a.txt", "/src/link"); err != nil {
		t.Fatalf("ln: %v", err)
	}

	if err := c.Cp(ctx, "/src", "/dst", true); err != nil {
		t.Fatalf("cp recursive: %v", err)
	}

	// Verify files were copied
	data, err := c.Cat(ctx, "/dst/a.txt")
	if err != nil {
		t.Fatalf("cat: %v", err)
	}
	if string(data) != "A" {
		t.Fatalf("content = %q", string(data))
	}

	data, err = c.Cat(ctx, "/dst/sub/b.txt")
	if err != nil {
		t.Fatalf("cat sub: %v", err)
	}
	if string(data) != "B" {
		t.Fatalf("sub content = %q", string(data))
	}

	// Verify symlink was copied
	tgt, err := c.Readlink(ctx, "/dst/link")
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if tgt != "a.txt" {
		t.Fatalf("symlink target = %q", tgt)
	}
}

func TestCpDirNonRecursiveError(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "cpnorecurse")

	if err := c.Mkdir(ctx, "/dir"); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	err := c.Cp(ctx, "/dir", "/dir2", false)
	if err == nil {
		t.Fatal("expected error for cp dir without recursive")
	}
	if !strings.Contains(err.Error(), "directory") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTree(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "tree")

	if err := c.Mkdir(ctx, "/a/b"); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := c.Echo(ctx, "/a/b/file.txt", []byte("x")); err != nil {
		t.Fatalf("echo: %v", err)
	}
	if err := c.Echo(ctx, "/a/top.txt", []byte("y")); err != nil {
		t.Fatalf("echo: %v", err)
	}

	entries, err := c.Tree(ctx, "/a", 0)
	if err != nil {
		t.Fatalf("tree: %v", err)
	}
	if len(entries) < 3 {
		t.Fatalf("expected at least 3 entries, got %d: %+v", len(entries), entries)
	}
	// First entry should be /a at depth 0
	if entries[0].Path != "/a" || entries[0].Type != "dir" || entries[0].Depth != 0 {
		t.Fatalf("root entry: %+v", entries[0])
	}

	// Test depth limiting
	entries2, err := c.Tree(ctx, "/a", 1)
	if err != nil {
		t.Fatalf("tree depth 1: %v", err)
	}
	for _, e := range entries2 {
		if e.Depth > 1 {
			t.Fatalf("depth exceeded: %+v", e)
		}
	}
}

func TestFind(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "find")

	if err := c.Mkdir(ctx, "/docs"); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := c.Echo(ctx, "/docs/readme.md", []byte("R")); err != nil {
		t.Fatalf("echo: %v", err)
	}
	if err := c.Echo(ctx, "/docs/notes.txt", []byte("N")); err != nil {
		t.Fatalf("echo: %v", err)
	}
	if err := c.Echo(ctx, "/file.md", []byte("F")); err != nil {
		t.Fatalf("echo: %v", err)
	}

	// Find all .md files
	matches, err := c.Find(ctx, "/", "*.md", "")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d: %v", len(matches), matches)
	}

	// Find only files
	matches2, err := c.Find(ctx, "/", "*.md", "file")
	if err != nil {
		t.Fatalf("find file: %v", err)
	}
	if len(matches2) != 2 {
		t.Fatalf("expected 2 file matches, got %d: %v", len(matches2), matches2)
	}

	// Find dirs
	matches3, err := c.Find(ctx, "/", "docs", "dir")
	if err != nil {
		t.Fatalf("find dir: %v", err)
	}
	if len(matches3) != 1 || matches3[0] != "/docs" {
		t.Fatalf("find dir result: %v", matches3)
	}
}

func TestGrep(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "grep")

	if err := c.Echo(ctx, "/log.txt", []byte("INFO: started\nERROR: disk full\nINFO: retrying\n")); err != nil {
		t.Fatalf("echo: %v", err)
	}

	// Case-sensitive
	matches, err := c.Grep(ctx, "/", "*ERROR*", false)
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d: %+v", len(matches), matches)
	}
	if matches[0].LineNum != 2 {
		t.Fatalf("line = %d, want 2", matches[0].LineNum)
	}
	if matches[0].Line != "ERROR: disk full" {
		t.Fatalf("line = %q", matches[0].Line)
	}

	// NOCASE
	matches2, err := c.Grep(ctx, "/", "*error*", true)
	if err != nil {
		t.Fatalf("grep nocase: %v", err)
	}
	if len(matches2) != 1 {
		t.Fatalf("expected 1 nocase match, got %d: %+v", len(matches2), matches2)
	}
}

func TestGrepBinaryDetection(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "grepbin")

	// File with NUL byte
	if err := c.Echo(ctx, "/binary.dat", []byte("hello\x00world")); err != nil {
		t.Fatalf("echo: %v", err)
	}

	matches, err := c.Grep(ctx, "/", "*hello*", false)
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 binary match, got %d", len(matches))
	}
	if matches[0].LineNum != 0 {
		t.Fatalf("binary match line = %d, want 0", matches[0].LineNum)
	}
	if matches[0].Line != "Binary file matches" {
		t.Fatalf("binary match line = %q", matches[0].Line)
	}
}

func TestGrepSingleFile(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "grepsingle")

	if err := c.Echo(ctx, "/file.txt", []byte("foo\nbar\nbaz\n")); err != nil {
		t.Fatalf("echo: %v", err)
	}

	matches, err := c.Grep(ctx, "/file.txt", "*ba*", false)
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d: %+v", len(matches), matches)
	}
}

// ---------------------------------------------------------------------------
// Fix 1 — createFileIfMissing HSETNX race behavior
//
// These tests exercise the lost-race path in createFileIfMissing. Two
// goroutines race into CreateFile on the same path, and both are released
// from a barrier at the same instant. Redis is single-threaded, so exactly
// one HSETNX wins. The loser must:
//   (a) DEL the orphan inode it optimistically wrote,
//   (b) reverse the files / total_data_bytes counter bumps,
//   (c) report the existing file (or "already exists" under O_EXCL).
// Without any of those, Info() counters would drift or orphan keys would
// survive in Redis.
// ---------------------------------------------------------------------------

func TestCreateFileRaceLoserCleansUpOrphan(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "create-race")

	type raceResult struct {
		stat    *StatResult
		created bool
		err     error
	}
	start := make(chan struct{})
	results := make(chan raceResult, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			st, created, err := c.CreateFile(ctx, "/race.txt", 0o644, false)
			results <- raceResult{stat: st, created: created, err: err}
		}()
	}
	close(start)

	var gotCreated, gotExisting int
	var firstInode uint64
	for i := 0; i < 2; i++ {
		r := <-results
		if r.err != nil {
			t.Fatalf("unexpected CreateFile error: %v", r.err)
		}
		if r.stat == nil {
			t.Fatalf("CreateFile returned nil stat")
		}
		if firstInode == 0 {
			firstInode = r.stat.Inode
		} else if r.stat.Inode != firstInode {
			t.Fatalf("both callers must see the same winning inode ID; got %d and %d",
				firstInode, r.stat.Inode)
		}
		if r.created {
			gotCreated++
		} else {
			gotExisting++
		}
	}
	if gotCreated != 1 {
		t.Fatalf("expected exactly 1 created=true result, got %d", gotCreated)
	}
	if gotExisting != 1 {
		t.Fatalf("expected exactly 1 created=false result, got %d", gotExisting)
	}

	// Info.files must be 1, not 2. If the loser did not compensate its
	// optimistic HIncrBy(files, +1), this would be 2.
	info, err := c.Info(ctx)
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	if info.Files != 1 {
		t.Fatalf("Info.Files = %d, want 1 (loser counters not compensated)", info.Files)
	}
	if info.TotalDataBytes != 0 {
		t.Fatalf("Info.TotalDataBytes = %d, want 0", info.TotalDataBytes)
	}

	// Redis should hold exactly two inode keys: the root and the winner's
	// /race.txt. An extra key means the loser's DEL on its orphan inode
	// never ran.
	keys, err := rdb.Keys(ctx, "afs:{create-race}:inode:*").Result()
	if err != nil {
		t.Fatalf("keys: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 inode keys (root + /race.txt), got %d: %v", len(keys), keys)
	}
}

func TestCreateFileRaceLoserCompensatesContentBytes(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "create-race-bytes")

	// Echo() hits createFileIfMissing via createFile with non-empty
	// content, exercising the total_data_bytes compensation branch on
	// the lost-race path.
	const payload = "hello world"
	type raceResult struct{ err error }
	start := make(chan struct{})
	results := make(chan raceResult, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			err := c.Echo(ctx, "/bytes.txt", []byte(payload))
			results <- raceResult{err: err}
		}()
	}
	close(start)
	for i := 0; i < 2; i++ {
		r := <-results
		// Echo overwrites on existing-file path, so both goroutines should
		// succeed regardless of who won the race.
		if r.err != nil {
			t.Fatalf("echo: %v", r.err)
		}
	}

	info, err := c.Info(ctx)
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	if info.Files != 1 {
		t.Fatalf("Info.Files = %d, want 1", info.Files)
	}
	if info.TotalDataBytes != int64(len(payload)) {
		t.Fatalf("Info.TotalDataBytes = %d, want %d (loser content-delta not compensated)",
			info.TotalDataBytes, len(payload))
	}

	keys, err := rdb.Keys(ctx, "afs:{create-race-bytes}:inode:*").Result()
	if err != nil {
		t.Fatalf("keys: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 inode keys (root + /bytes.txt), got %d: %v", len(keys), keys)
	}
}

func TestCreateFileRaceExclusiveLoserReturnsError(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "create-race-excl")

	type raceResult struct {
		stat    *StatResult
		created bool
		err     error
	}
	start := make(chan struct{})
	results := make(chan raceResult, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			st, created, err := c.CreateFile(ctx, "/excl.txt", 0o600, true)
			results <- raceResult{stat: st, created: created, err: err}
		}()
	}
	close(start)

	var wins, errs int
	for i := 0; i < 2; i++ {
		r := <-results
		if r.err != nil {
			if !strings.Contains(r.err.Error(), "already exists") {
				t.Fatalf("exclusive loser error = %v, want 'already exists'", r.err)
			}
			errs++
			continue
		}
		if !r.created {
			t.Fatalf("exclusive winner must see created=true")
		}
		wins++
	}
	if wins != 1 {
		t.Fatalf("expected 1 exclusive winner, got %d", wins)
	}
	if errs != 1 {
		t.Fatalf("expected 1 exclusive loser, got %d", errs)
	}

	info, err := c.Info(ctx)
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	if info.Files != 1 {
		t.Fatalf("Info.Files = %d, want 1", info.Files)
	}
	keys, err := rdb.Keys(ctx, "afs:{create-race-excl}:inode:*").Result()
	if err != nil {
		t.Fatalf("keys: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 inode keys, got %d: %v", len(keys), keys)
	}
}

func TestCreateFileOverExistingDirectoryPreservesError(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "create-over-dir")

	if err := c.Mkdir(ctx, "/thing"); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// A CreateFile against an existing directory must fail with a
	// "not a file" error — callers in the NFS layer string-match on this.
	_, _, err := c.CreateFile(ctx, "/thing", 0o644, false)
	if err == nil {
		t.Fatal("expected CreateFile over a directory to fail")
	}
	if !strings.Contains(err.Error(), "not a file") {
		t.Fatalf("expected 'not a file' error, got %v", err)
	}

	// The failed create must leave counters untouched. If the lost-race
	// compensation path doesn't cover this branch, Info.Files would be -1.
	info, err := c.Info(ctx)
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	if info.Files != 0 {
		t.Fatalf("Info.Files = %d, want 0 (counter leaked on dir collision)", info.Files)
	}
	// ensureRoot seeds directories=1 for the root inode, Mkdir("/thing") adds
	// another +1 via queueCreateInfo, so the expected post-state is 2. The
	// failed CreateFile must NOT touch the directories counter either way.
	if info.Directories != 2 {
		t.Fatalf("Info.Directories = %d, want 2 (root + /thing)", info.Directories)
	}
	if info.TotalDataBytes != 0 {
		t.Fatalf("Info.TotalDataBytes = %d, want 0", info.TotalDataBytes)
	}
}

// ---------------------------------------------------------------------------
// Fix 2 — batched SetAttrs
//
// These tests cover the new Client.SetAttrs fast path: sparse updates,
// no-op skip, cache-in-place behavior, and parity with the sequential
// Chmod / Chown / Utimens methods it replaces.
// ---------------------------------------------------------------------------

func ptrU32(v uint32) *uint32 { return &v }
func ptrI64(v int64) *int64   { return &v }

// readInodeFields is a small test helper that HMGets selected fields
// off a file's inode hash directly, bypassing the client's cache.
func readInodeFields(t *testing.T, rdb *redis.Client, ctx context.Context, fsKey, inodePath string, fields ...string) map[string]string {
	t.Helper()
	c := New(rdb, fsKey)
	st, err := c.Stat(ctx, inodePath)
	if err != nil {
		t.Fatalf("stat %s: %v", inodePath, err)
	}
	if st == nil {
		t.Fatalf("stat %s: nil", inodePath)
	}
	key := "afs:{" + fsKey + "}:inode:" + strconv.FormatUint(st.Inode, 10)
	vals, err := rdb.HMGet(ctx, key, fields...).Result()
	if err != nil {
		t.Fatalf("hmget %s: %v", key, err)
	}
	out := make(map[string]string, len(fields))
	for i, name := range fields {
		if vals[i] == nil {
			out[name] = ""
			continue
		}
		if s, ok := vals[i].(string); ok {
			out[name] = s
		}
	}
	return out
}

func TestSetAttrsPartialFieldsOnly(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	const fsKey = "setattrs-partial"
	c := New(rdb, fsKey)

	// Seed a file with known initial mode/uid/gid and a distinctive
	// mtime we can later verify is untouched by a mode-only update.
	if _, _, err := c.CreateFile(ctx, "/f.txt", 0o644, false); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := c.Chown(ctx, "/f.txt", 1000, 1000); err != nil {
		t.Fatalf("chown seed: %v", err)
	}
	const sentinelMs int64 = 1600000000000
	if err := c.Utimens(ctx, "/f.txt", sentinelMs, sentinelMs); err != nil {
		t.Fatalf("utimens seed: %v", err)
	}

	cases := []struct {
		name       string
		upd        AttrUpdate
		wantFields map[string]string
	}{
		{
			name: "mode_only",
			upd:  AttrUpdate{Mode: ptrU32(0o600)},
			wantFields: map[string]string{
				"mode": strconv.FormatUint(0o600, 10),
				"uid":  "1000",
				"gid":  "1000",
			},
		},
		{
			name: "uid_only",
			upd:  AttrUpdate{UID: ptrU32(2000)},
			wantFields: map[string]string{
				"mode": strconv.FormatUint(0o600, 10),
				"uid":  "2000",
				"gid":  "1000",
			},
		},
		{
			name: "gid_only",
			upd:  AttrUpdate{GID: ptrU32(2001)},
			wantFields: map[string]string{
				"mode": strconv.FormatUint(0o600, 10),
				"uid":  "2000",
				"gid":  "2001",
			},
		},
		{
			name: "atime_only",
			upd:  AttrUpdate{AtimeMs: ptrI64(1700000000000)},
			wantFields: map[string]string{
				"atime_ms": "1700000000000",
				"mtime_ms": strconv.FormatInt(sentinelMs, 10),
			},
		},
		{
			name: "mtime_only",
			upd:  AttrUpdate{MtimeMs: ptrI64(1700000001000)},
			wantFields: map[string]string{
				"atime_ms": "1700000000000",
				"mtime_ms": "1700000001000",
			},
		},
		{
			name: "mode_and_uid",
			upd:  AttrUpdate{Mode: ptrU32(0o640), UID: ptrU32(3000)},
			wantFields: map[string]string{
				"mode": strconv.FormatUint(0o640, 10),
				"uid":  "3000",
				"gid":  "2001",
			},
		},
		{
			name: "all_five",
			upd: AttrUpdate{
				Mode:    ptrU32(0o755),
				UID:     ptrU32(4000),
				GID:     ptrU32(4001),
				AtimeMs: ptrI64(1800000000000),
				MtimeMs: ptrI64(1800000001000),
			},
			wantFields: map[string]string{
				"mode":     strconv.FormatUint(0o755, 10),
				"uid":      "4000",
				"gid":      "4001",
				"atime_ms": "1800000000000",
				"mtime_ms": "1800000001000",
			},
		},
	}
	// Note: these cases are NOT independent — each one builds on the
	// previous via the running "current state" of /f.txt. This is
	// intentional so that "uid_only" verifies the mode from "mode_only"
	// stayed untouched. If we wanted fully independent cases we'd need
	// to seed a fresh file per case, which would also be fine; this is
	// terser and exercises the partial-field HSet behavior head-on.
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := c.SetAttrs(ctx, "/f.txt", tc.upd); err != nil {
				t.Fatalf("SetAttrs: %v", err)
			}
			names := make([]string, 0, len(tc.wantFields))
			for n := range tc.wantFields {
				names = append(names, n)
			}
			got := readInodeFields(t, rdb, ctx, fsKey, "/f.txt", names...)
			for _, n := range names {
				if got[n] != tc.wantFields[n] {
					t.Errorf("field %q = %q, want %q", n, got[n], tc.wantFields[n])
				}
			}
		})
	}
}

func TestSetAttrsEmptyIsNoOp(t *testing.T) {
	t.Parallel()
	rdb, _ := setupTestRedis(t)
	const fsKey = "setattrs-noop"

	// Count ALL Redis commands issued while SetAttrs runs, not just a
	// specific op. An all-nil AttrUpdate must not hit Redis at all.
	var ops atomic.Int64
	hook := &commandCountHook{track: func(cmd redis.Cmder) {
		ops.Add(1)
	}}
	rdb.AddHook(hook)

	c := New(rdb, fsKey)
	ctx := context.Background()
	if _, _, err := c.CreateFile(ctx, "/f.txt", 0o644, false); err != nil {
		t.Fatalf("create: %v", err)
	}

	ops.Store(0)
	if err := c.SetAttrs(ctx, "/f.txt", AttrUpdate{}); err != nil {
		t.Fatalf("SetAttrs empty: %v", err)
	}
	if got := ops.Load(); got != 0 {
		t.Fatalf("SetAttrs{} issued %d Redis commands, want 0", got)
	}
}

func TestSetAttrsCacheUpdatesInPlace(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	const fsKey = "setattrs-cache"

	// Count HMGET commands against the inode hash. If SetAttrs updates
	// the cache in place, a subsequent Stat must NOT issue any HMGET;
	// it'll hit the warm cache entry.
	var hmgetCount atomic.Int64
	hook := &commandCountHook{track: func(cmd redis.Cmder) {
		args := cmd.Args()
		if len(args) < 2 {
			return
		}
		name, _ := args[0].(string)
		if !strings.EqualFold(name, "hmget") {
			return
		}
		hmgetCount.Add(1)
	}}
	rdb.AddHook(hook)

	c := NewWithCache(rdb, fsKey, time.Hour)
	if _, _, err := c.CreateFile(ctx, "/f.txt", 0o644, false); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Warm the cache with a Stat once so subsequent reads hit the
	// path-cache fast path.
	if _, err := c.Stat(ctx, "/f.txt"); err != nil {
		t.Fatalf("stat warm: %v", err)
	}

	hmgetCount.Store(0)
	if err := c.SetAttrs(ctx, "/f.txt", AttrUpdate{Mode: ptrU32(0o600)}); err != nil {
		t.Fatalf("SetAttrs: %v", err)
	}
	// Immediately Stat again — must be a cache hit, zero HMGETs.
	st, err := c.Stat(ctx, "/f.txt")
	if err != nil {
		t.Fatalf("stat after SetAttrs: %v", err)
	}
	if st == nil || st.Mode != 0o600 {
		t.Fatalf("stat after SetAttrs: got mode=%o, want 0o600", st.Mode)
	}
	if got := hmgetCount.Load(); got != 0 {
		t.Fatalf("post-SetAttrs Stat issued %d HMGETs, want 0 (cache should be updated in place)", got)
	}
}

func TestSetAttrsParityWithChmodChownUtimens(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)

	// Set up two workspaces. On the first, apply Chmod+Chown+Utimens
	// sequentially. On the second, apply the same mutations through one
	// SetAttrs call. The resulting inode state must match exactly on the
	// four mutable metadata fields.
	const fsSeq = "setattrs-seq"
	const fsBatch = "setattrs-batch"
	cSeq := New(rdb, fsSeq)
	cBatch := New(rdb, fsBatch)

	const (
		seedMode uint32 = 0o644
		newMode  uint32 = 0o600
		newUID   uint32 = 5000
		newGID   uint32 = 5001
	)
	const newAtime int64 = 1900000000000
	const newMtime int64 = 1900000001000

	for _, c := range []Client{cSeq, cBatch} {
		if _, _, err := c.CreateFile(ctx, "/p.txt", seedMode, false); err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	if err := cSeq.Chmod(ctx, "/p.txt", newMode); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if err := cSeq.Chown(ctx, "/p.txt", newUID, newGID); err != nil {
		t.Fatalf("chown: %v", err)
	}
	if err := cSeq.Utimens(ctx, "/p.txt", newAtime, newMtime); err != nil {
		t.Fatalf("utimens: %v", err)
	}

	if err := cBatch.SetAttrs(ctx, "/p.txt", AttrUpdate{
		Mode:    ptrU32(newMode),
		UID:     ptrU32(newUID),
		GID:     ptrU32(newGID),
		AtimeMs: ptrI64(newAtime),
		MtimeMs: ptrI64(newMtime),
	}); err != nil {
		t.Fatalf("SetAttrs: %v", err)
	}

	seqFields := readInodeFields(t, rdb, ctx, fsSeq, "/p.txt",
		"mode", "uid", "gid", "atime_ms", "mtime_ms")
	batchFields := readInodeFields(t, rdb, ctx, fsBatch, "/p.txt",
		"mode", "uid", "gid", "atime_ms", "mtime_ms")
	for _, key := range []string{"mode", "uid", "gid", "atime_ms", "mtime_ms"} {
		if seqFields[key] != batchFields[key] {
			t.Errorf("field %q: seq=%q batch=%q", key, seqFields[key], batchFields[key])
		}
	}
	if batchFields["mode"] != strconv.FormatUint(uint64(newMode), 10) {
		t.Errorf("batch mode = %q, want %d", batchFields["mode"], newMode)
	}
	if batchFields["uid"] != strconv.FormatUint(uint64(newUID), 10) {
		t.Errorf("batch uid = %q, want %d", batchFields["uid"], newUID)
	}
}

func TestSetAttrsSingleRoundTrip(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)

	// Count HSET commands against any inode hash under this workspace.
	// SetAttrs must issue exactly ONE HSET per mutation, even when the
	// update covers mode+uid+gid+atime+mtime together. The three-method
	// sequence used to require three HSETs.
	const fsKey = "setattrs-rtt"
	var hsetCount atomic.Int64
	hook := &commandCountHook{track: func(cmd redis.Cmder) {
		args := cmd.Args()
		if len(args) < 2 {
			return
		}
		name, _ := args[0].(string)
		if !strings.EqualFold(name, "hset") {
			return
		}
		key, _ := args[1].(string)
		if !strings.Contains(key, ":inode:") {
			return
		}
		hsetCount.Add(1)
	}}
	rdb.AddHook(hook)

	c := New(rdb, fsKey)
	if _, _, err := c.CreateFile(ctx, "/rtt.txt", 0o644, false); err != nil {
		t.Fatalf("create: %v", err)
	}

	hsetCount.Store(0)
	if err := c.SetAttrs(ctx, "/rtt.txt", AttrUpdate{
		Mode:    ptrU32(0o600),
		UID:     ptrU32(6000),
		GID:     ptrU32(6001),
		AtimeMs: ptrI64(1910000000000),
		MtimeMs: ptrI64(1910000001000),
	}); err != nil {
		t.Fatalf("SetAttrs: %v", err)
	}
	if got := hsetCount.Load(); got != 1 {
		t.Fatalf("SetAttrs issued %d HSETs against an inode hash, want 1", got)
	}
}

// ---------------------------------------------------------------------------
// Perf regression guards: command-count assertions
//
// These tests pin the number of Redis commands per hot-path operation so any
// future refactor that accidentally re-adds a round trip shows up as a
// clear failure rather than a gradual bench drift.
// ---------------------------------------------------------------------------

// countHook sums every command (including commands issued inside pipelines).
type countHook struct {
	total atomic.Int64
}

func (h *countHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h *countHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		h.total.Add(1)
		return next(ctx, cmd)
	}
}
func (h *countHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		h.total.Add(int64(len(cmds)))
		return next(ctx, cmds)
	}
}

// perfHotPathCommandCounts runs one warm-path SetAttrs and one warm-path
// legacy Chmod+Chown+Utimens sequence on identically-prepared workspaces,
// and returns the total Redis command counts for each. This is the
// building block for the command-count regression guards below: we rely
// on the invariant that SetAttrs ALWAYS issues strictly fewer commands
// than the legacy three-method sequence, regardless of what ancillary
// commands (PUBLISH, SET rootDirty, etc.) the future adds.
func perfHotPathCommandCounts(t *testing.T) (batchCmds, legacyCmds int64, batchFS, legacyFS string) {
	t.Helper()
	rdb, ctx := setupTestRedis(t)
	batchFS = "perf-batch"
	legacyFS = "perf-legacy"

	// Shared warmth setup for both clients: create a file, stat it to
	// prime the path cache, then call one throwaway Chmod to prime
	// markRootDirty's debounce window. After this, any subsequent
	// mutation should NOT incur a SET rootDirty hit on top of its HSet.
	warm := func(fsKey string) Client {
		c := NewWithCache(rdb, fsKey, time.Hour)
		if _, _, err := c.CreateFile(ctx, "/f.txt", 0o644, false); err != nil {
			t.Fatalf("create %s: %v", fsKey, err)
		}
		if _, err := c.Stat(ctx, "/f.txt"); err != nil {
			t.Fatalf("warm stat %s: %v", fsKey, err)
		}
		if err := c.Chmod(ctx, "/f.txt", 0o644); err != nil {
			t.Fatalf("prime markRootDirty %s: %v", fsKey, err)
		}
		return c
	}

	cBatch := warm(batchFS)
	cLegacy := warm(legacyFS)

	var batchHook countHook
	var legacyHook countHook

	// Install hooks AFTER warmth so we only count the hot-path commands.
	// commandCountHook uses per-call track fns; we use a raw countHook
	// here to avoid threading a filter closure.
	rdb.AddHook(&hookGate{gate: &batchHook, fsKey: batchFS})
	rdb.AddHook(&hookGate{gate: &legacyHook, fsKey: legacyFS})

	// 5-field SetAttrs: the batched fast path.
	if err := cBatch.SetAttrs(ctx, "/f.txt", AttrUpdate{
		Mode:    ptrU32(0o600),
		UID:     ptrU32(6000),
		GID:     ptrU32(6001),
		AtimeMs: ptrI64(1920000000000),
		MtimeMs: ptrI64(1920000001000),
	}); err != nil {
		t.Fatalf("SetAttrs: %v", err)
	}

	// Equivalent update via the legacy three-method sequence.
	if err := cLegacy.Chmod(ctx, "/f.txt", 0o600); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if err := cLegacy.Chown(ctx, "/f.txt", 6000, 6001); err != nil {
		t.Fatalf("chown: %v", err)
	}
	if err := cLegacy.Utimens(ctx, "/f.txt", 1920000000000, 1920000001000); err != nil {
		t.Fatalf("utimens: %v", err)
	}

	return batchHook.total.Load(), legacyHook.total.Load(), batchFS, legacyFS
}

// hookGate filters commands to only those touching keys in fsKey's
// curly-brace hash tag. This keeps the two per-workspace hooks from
// counting each other.
type hookGate struct {
	gate  *countHook
	fsKey string
}

func (h *hookGate) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h *hookGate) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if h.matches(cmd) {
			h.gate.total.Add(1)
		}
		return next(ctx, cmd)
	}
}
func (h *hookGate) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		for _, cmd := range cmds {
			if h.matches(cmd) {
				h.gate.total.Add(1)
			}
		}
		return next(ctx, cmds)
	}
}
func (h *hookGate) matches(cmd redis.Cmder) bool {
	args := cmd.Args()
	// PUBLISH and SUBSCRIBE style commands carry the channel in args[1].
	// Everything else (HSet, HIncrBy, SET, INCR, ...) also has the key
	// at args[1]. A substring match on "{fsKey}" picks up both cases
	// because the channel name includes the same hash tag.
	tag := "{" + h.fsKey + "}"
	for _, a := range args {
		s, ok := a.(string)
		if !ok {
			continue
		}
		if strings.Contains(s, tag) {
			return true
		}
	}
	return false
}

func TestSetAttrsCommandsStrictlyLessThanLegacy(t *testing.T) {
	t.Parallel()
	batchCmds, legacyCmds, _, _ := perfHotPathCommandCounts(t)
	if batchCmds >= legacyCmds {
		t.Fatalf("SetAttrs fast path issued %d commands, legacy issued %d; "+
			"fast path must be strictly less", batchCmds, legacyCmds)
	}
	// Empirically: batch=2 (HSet + PUBLISH), legacy=6 (3× HSet + 3× PUBLISH)
	// when markRootDirty is throttled. Pin these so a regression that
	// silently adds a round trip trips the assertion.
	const maxBatchCmds = 3
	if batchCmds > maxBatchCmds {
		t.Fatalf("SetAttrs fast path issued %d commands, want <= %d "+
			"(regression: extra round trip on the SETATTR hot path)",
			batchCmds, maxBatchCmds)
	}
	t.Logf("perf: SetAttrs=%d commands, legacy=%d commands (%.1fx reduction)",
		batchCmds, legacyCmds, float64(legacyCmds)/float64(batchCmds))
}

func TestCreateFileCommandCountIsBounded(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	const fsKey = "createfile-opcount"
	c := NewWithCache(rdb, fsKey, time.Hour)

	// Warm the root cache so ensureParents/resolvePath on the parent
	// ("/") hits the cache and does not contribute any RTTs. This
	// matches the NFS hot path where the mount is already up.
	if _, err := c.Stat(ctx, "/"); err != nil {
		t.Fatalf("warm stat /: %v", err)
	}

	var h countHook
	rdb.AddHook(&hookGate{gate: &h, fsKey: fsKey})

	if _, _, err := c.CreateFile(ctx, "/first.txt", 0o644, false); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Post Fix 1, an uncontended CreateFile against a warm root cache
	// issues roughly:
	//   INCR nextInode                                         (1)
	//   pipeline: HSETNX dirents + SET content:{id} + HSet inode
	//             + HSet touchtimes + HIncrBy files            (5)
	//   PUBLISH invalidate (InvalidateOpDir for parent listing) (1)
	//   PUBLISH invalidate (InvalidateOpInode for new file)     (1)
	//   SET rootDirty "1"                                       (1)
	// = 9 commands.
	//
	// The extra SET is for the external content key (content_ref="ext"
	// storage model). Pre-fix the WATCH/MULTI sequence was 3+ more.
	const maxCommands = 12
	if got := h.total.Load(); got > maxCommands {
		t.Fatalf("CreateFile issued %d Redis commands, want <= %d (regression)", got, maxCommands)
	}
	t.Logf("perf: CreateFile=%d commands (warm root cache)", h.total.Load())
}

// ---------------------------------------------------------------------------
// Wall-clock benchmarks (go test -bench)
//
// These run against a local redis-server on a free port, so the absolute
// numbers approximate "warm local Redis" semantics, not the slow-path
// remote-Redis case the perf docs cover. The *ratio* between pre- and
// post-fix is what matters: BenchmarkSetAttrsBatched vs. the
// BenchmarkLegacyChmodChownUtimens baseline is a direct measurement of
// how much the 3->1 RTT collapse saves per CREATE/SETATTR.
// ---------------------------------------------------------------------------

func setupBenchRedis(b *testing.B) (*redis.Client, context.Context) {
	b.Helper()
	port := freeTCPPortBench(b)
	cmd := exec.Command(
		"redis-server",
		"--port", strconv.Itoa(port),
		"--save", "",
		"--appendonly", "no",
	)
	if err := cmd.Start(); err != nil {
		b.Fatalf("start redis-server: %v", err)
	}
	b.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	b.Cleanup(cancel)

	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:" + strconv.Itoa(port)})
	b.Cleanup(func() { _ = rdb.Close() })

	deadline := time.Now().Add(5 * time.Second)
	for {
		if err := rdb.Ping(ctx).Err(); err == nil {
			break
		}
		if time.Now().After(deadline) {
			b.Fatal("redis-server did not become ready")
		}
		time.Sleep(50 * time.Millisecond)
	}
	return rdb, ctx
}

func freeTCPPortBench(b *testing.B) int {
	b.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatalf("allocate port: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func BenchmarkCreateFile(b *testing.B) {
	rdb, ctx := setupBenchRedis(b)
	c := New(rdb, "bench-create")
	_ = c.Mkdir(ctx, "/d")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p := "/d/f-" + strconv.Itoa(i) + ".txt"
		if _, _, err := c.CreateFile(ctx, p, 0o644, false); err != nil {
			b.Fatalf("create: %v", err)
		}
	}
}

func BenchmarkSetAttrsBatched(b *testing.B) {
	rdb, ctx := setupBenchRedis(b)
	c := NewWithCache(rdb, "bench-setattrs-batch", time.Hour)
	if _, _, err := c.CreateFile(ctx, "/f.txt", 0o644, false); err != nil {
		b.Fatalf("seed: %v", err)
	}
	// Warm cache + prime throttle so we're measuring the steady-state
	// SETATTR hot path after the first tick.
	if _, err := c.Stat(ctx, "/f.txt"); err != nil {
		b.Fatalf("warm stat: %v", err)
	}
	if err := c.Chmod(ctx, "/f.txt", 0o644); err != nil {
		b.Fatalf("prime: %v", err)
	}

	mode := uint32(0o600)
	uid := uint32(6000)
	gid := uint32(6001)
	// Use a monotonically-increasing mtime so no iteration hits the
	// no-op skip and we measure a real HSET each loop.
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		atime := int64(1920000000000 + i)
		mtime := int64(1920000001000 + i)
		if err := c.SetAttrs(ctx, "/f.txt", AttrUpdate{
			Mode:    &mode,
			UID:     &uid,
			GID:     &gid,
			AtimeMs: &atime,
			MtimeMs: &mtime,
		}); err != nil {
			b.Fatalf("SetAttrs: %v", err)
		}
	}
}

func BenchmarkLegacyChmodChownUtimens(b *testing.B) {
	rdb, ctx := setupBenchRedis(b)
	c := NewWithCache(rdb, "bench-setattrs-legacy", time.Hour)
	if _, _, err := c.CreateFile(ctx, "/f.txt", 0o644, false); err != nil {
		b.Fatalf("seed: %v", err)
	}
	if _, err := c.Stat(ctx, "/f.txt"); err != nil {
		b.Fatalf("warm stat: %v", err)
	}
	if err := c.Chmod(ctx, "/f.txt", 0o644); err != nil {
		b.Fatalf("prime: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := c.Chmod(ctx, "/f.txt", 0o600); err != nil {
			b.Fatalf("chmod: %v", err)
		}
		if err := c.Chown(ctx, "/f.txt", 6000, 6001); err != nil {
			b.Fatalf("chown: %v", err)
		}
		atime := int64(1920000000000 + i)
		mtime := int64(1920000001000 + i)
		if err := c.Utimens(ctx, "/f.txt", atime, mtime); err != nil {
			b.Fatalf("utimens: %v", err)
		}
	}
}
