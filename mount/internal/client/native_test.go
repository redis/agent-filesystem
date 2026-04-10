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
	if vals["content"] != "world" {
		t.Fatalf("expected content=world, got %q", vals["content"])
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
