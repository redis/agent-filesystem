package client

import (
	"context"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

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

// ---------------------------------------------------------------------------
// Raw key format verification
// ---------------------------------------------------------------------------

func TestRawPathKeyFormat(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := New(rdb, "keytest")

	if err := c.Echo(ctx, "/hello.txt", []byte("world")); err != nil {
		t.Fatalf("echo: %v", err)
	}

	// Verify the inode key uses raw path, not base64
	inodeKey := "rfs:{keytest}:inode:/hello.txt"
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

	// Verify children set key format
	childrenKey := "rfs:{keytest}:children:/"
	members, err := rdb.SMembers(ctx, childrenKey).Result()
	if err != nil {
		t.Fatalf("smembers: %v", err)
	}
	found := false
	for _, m := range members {
		if m == "hello.txt" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected 'hello.txt' in children set, got %v", members)
	}

	// Verify info key format
	infoKey := "rfs:{keytest}:info"
	infoVals, err := rdb.HGetAll(ctx, infoKey).Result()
	if err != nil {
		t.Fatalf("hgetall info: %v", err)
	}
	if infoVals["files"] != "1" {
		t.Fatalf("expected files=1, got %q", infoVals["files"])
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
		keys, next, err := rdb.Scan(ctx, cursor, "rfs:{htag}:*", 100).Result()
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
		t.Fatal("expected keys matching rfs:{htag}:*, got none")
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
