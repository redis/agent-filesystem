package nfsfs

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/agent-filesystem/mount/internal/client"
	"github.com/redis/go-redis/v9"
	nfs "github.com/willscott/go-nfs"
)

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

func TestOpenFileCreateIsImmediate(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := client.New(rdb, "nfs-create")
	fs := New(c, false)

	fh, err := fs.OpenFile("/created.txt", os.O_RDWR|os.O_CREATE, 0o640)
	if err != nil {
		t.Fatalf("open create: %v", err)
	}
	defer func() { _ = fh.Close() }()

	st, err := c.Stat(ctx, "/created.txt")
	if err != nil {
		t.Fatalf("stat created file: %v", err)
	}
	if st == nil {
		t.Fatal("expected created file to exist before close")
	}
	if st.Mode != 0o640 {
		t.Fatalf("mode = %o, want 640", st.Mode)
	}
}

func TestOpenFileExclusiveCreateFailsWhenPresent(t *testing.T) {
	t.Parallel()
	rdb, _ := setupTestRedis(t)
	c := client.New(rdb, "nfs-exclusive")
	fs := New(c, false)

	if _, err := fs.OpenFile("/exists.txt", os.O_RDWR|os.O_CREATE, 0o644); err != nil {
		t.Fatalf("seed create: %v", err)
	}

	if _, err := fs.OpenFile("/exists.txt", os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644); !errors.Is(err, os.ErrExist) {
		t.Fatalf("expected os.ErrExist, got %v", err)
	}
}

func TestRenameReplacesExistingFile(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := client.New(rdb, "nfs-rename")
	fs := New(c, false)

	if err := c.Echo(ctx, "/src.txt", []byte("src")); err != nil {
		t.Fatalf("echo src: %v", err)
	}
	if err := c.Echo(ctx, "/dst.txt", []byte("dst")); err != nil {
		t.Fatalf("echo dst: %v", err)
	}

	if err := fs.Rename("/src.txt", "/dst.txt"); err != nil {
		t.Fatalf("rename: %v", err)
	}

	data, err := c.Cat(ctx, "/dst.txt")
	if err != nil {
		t.Fatalf("cat renamed dst: %v", err)
	}
	if string(data) != "src" {
		t.Fatalf("expected dst content from src, got %q", string(data))
	}
}

func TestWriteIsVisibleBeforeClose(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := client.New(rdb, "nfs-range")
	fs := New(c, false)

	fh, err := fs.OpenFile("/range.txt", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatalf("open create: %v", err)
	}
	defer func() { _ = fh.Close() }()

	if _, err := fh.Write([]byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}
	data, err := c.Cat(ctx, "/range.txt")
	if err != nil {
		t.Fatalf("cat before close: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("expected immediate write visibility, got %q", string(data))
	}

	if _, err := fh.Seek(1, io.SeekStart); err != nil {
		t.Fatalf("seek: %v", err)
	}
	buf := make([]byte, 3)
	n, err := fh.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("read: %v", err)
	}
	if string(buf[:n]) != "ell" {
		t.Fatalf("read after seek = %q, want ell", string(buf[:n]))
	}
}

func TestWriteThenMetadataUpdatePreservesVisibleSize(t *testing.T) {
	t.Parallel()
	rdb, _ := setupTestRedis(t)
	c := client.NewWithCache(rdb, "nfs-cache-size", time.Hour)
	fs := New(c, false)

	fh, err := fs.OpenFile("/size.txt", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatalf("open create: %v", err)
	}
	defer func() { _ = fh.Close() }()

	if _, err := fh.Write([]byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}

	info, err := fs.Stat("/size.txt")
	if err != nil {
		t.Fatalf("stat after write: %v", err)
	}
	if info.Size() != 5 {
		t.Fatalf("size after write = %d, want 5", info.Size())
	}

	now := time.Now()
	if err := fs.Chtimes("/size.txt", now, now); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	info, err = fs.Stat("/size.txt")
	if err != nil {
		t.Fatalf("stat after chtimes: %v", err)
	}
	if info.Size() != 5 {
		t.Fatalf("size after chtimes = %d, want 5", info.Size())
	}

	buf := make([]byte, 5)
	n, err := fh.ReadAt(buf, 0)
	if err != nil && err != io.EOF {
		t.Fatalf("readat after chtimes: %v", err)
	}
	if string(buf[:n]) != "hello" {
		t.Fatalf("readat after chtimes = %q, want hello", string(buf[:n]))
	}
}

func TestLockingReportsDisabled(t *testing.T) {
	t.Parallel()
	rdb, _ := setupTestRedis(t)
	c := client.New(rdb, "nfs-lock-disabled")
	fs := New(c, false)

	fh, err := fs.OpenFile("/lock.txt", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatalf("open create: %v", err)
	}
	defer func() { _ = fh.Close() }()

	if err := fh.Lock(); err == nil {
		t.Fatal("expected explicit nolock error")
	}
}

// ---------------------------------------------------------------------------
// Fix 2 — nfsfs.FS.SetAttrs (batched SETATTR fast path)
// ---------------------------------------------------------------------------

func TestFSSetAttrsFlowsThroughToClient(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := client.NewWithCache(rdb, "nfs-setattrs-flow", time.Hour)
	fs := New(c, false)

	if _, _, err := c.CreateFile(ctx, "/f.txt", 0o644, false); err != nil {
		t.Fatalf("seed: %v", err)
	}

	mode := os.FileMode(0o600)
	uid := 5000
	gid := 5001
	atime := time.UnixMilli(1700000000000)
	mtime := time.UnixMilli(1700000001000)
	if err := fs.SetAttrs("/f.txt", &mode, &uid, &gid, &atime, &mtime); err != nil {
		t.Fatalf("SetAttrs: %v", err)
	}

	st, err := c.Stat(ctx, "/f.txt")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if st == nil {
		t.Fatal("stat nil")
	}
	if st.Mode != 0o600 {
		t.Errorf("mode = %o, want 600", st.Mode)
	}
	if st.UID != 5000 {
		t.Errorf("uid = %d, want 5000", st.UID)
	}
	if st.GID != 5001 {
		t.Errorf("gid = %d, want 5001", st.GID)
	}
	if st.Atime != 1700000000000 {
		t.Errorf("atime_ms = %d, want 1700000000000", st.Atime)
	}
	if st.Mtime != 1700000001000 {
		t.Errorf("mtime_ms = %d, want 1700000001000", st.Mtime)
	}
}

func TestFSSetAttrsShadowAppleDouble(t *testing.T) {
	t.Parallel()
	rdb, _ := setupTestRedis(t)
	c := client.New(rdb, "nfs-setattrs-shadow")
	fs := New(c, false)

	// Seed a shadow "._x.txt" file via the normal OpenFile path.
	fh, err := fs.OpenFile("/._x.txt", os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatalf("open shadow: %v", err)
	}
	if _, err := fh.Write([]byte("shadow payload")); err != nil {
		t.Fatalf("write shadow: %v", err)
	}
	_ = fh.Close()

	newMode := os.FileMode(0o600)
	newAtime := time.UnixMilli(1710000000000)
	newMtime := time.UnixMilli(1710000001000)

	// SetAttrs on a shadow path must mutate the in-memory entry and
	// must not hit Redis. We verify by reading the shadow metadata back
	// via fs.Stat.
	if err := fs.SetAttrs("/._x.txt", &newMode, nil, nil, &newAtime, &newMtime); err != nil {
		t.Fatalf("SetAttrs shadow: %v", err)
	}

	info, err := fs.Stat("/._x.txt")
	if err != nil {
		t.Fatalf("stat shadow: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("shadow mode = %o, want 600", info.Mode().Perm())
	}
	if !info.ModTime().Equal(newMtime) {
		t.Errorf("shadow mtime = %v, want %v", info.ModTime(), newMtime)
	}
}

func TestFSSetAttrsShadowNotExist(t *testing.T) {
	t.Parallel()
	rdb, _ := setupTestRedis(t)
	c := client.New(rdb, "nfs-setattrs-shadow-missing")
	fs := New(c, false)

	newMode := os.FileMode(0o600)
	err := fs.SetAttrs("/._missing.txt", &newMode, nil, nil, nil, nil)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("SetAttrs missing shadow: got %v, want os.ErrNotExist", err)
	}
}

func TestFSSetAttrsReadOnly(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	// Seed using a read-write client, then mount read-only.
	cRW := client.New(rdb, "nfs-setattrs-ro")
	if _, _, err := cRW.CreateFile(ctx, "/f.txt", 0o644, false); err != nil {
		t.Fatalf("seed: %v", err)
	}

	cRO := client.New(rdb, "nfs-setattrs-ro")
	fs := New(cRO, true)
	newMode := os.FileMode(0o600)
	if err := fs.SetAttrs("/f.txt", &newMode, nil, nil, nil, nil); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("SetAttrs on read-only FS: got %v, want os.ErrPermission", err)
	}
}

func TestFSSetAttrsEmptyIsNoOp(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := client.New(rdb, "nfs-setattrs-empty")
	fs := New(c, false)
	if _, _, err := c.CreateFile(ctx, "/f.txt", 0o644, false); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Passing all-nil pointers must no-op on the real path; just verify
	// it returns nil without mutating anything.
	if err := fs.SetAttrs("/f.txt", nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("SetAttrs empty: %v", err)
	}

	st, err := c.Stat(ctx, "/f.txt")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if st.Mode != 0o644 {
		t.Errorf("mode drifted to %o after empty SetAttrs, want 0o644", st.Mode)
	}
}

// TestSetFileAttributesApplyDispatchesToBatchSetAttrer exercises the fast
// path in third_party/go-nfs: SetFileAttributes.Apply must notice that our
// nfsfs.FS implements BatchSetAttrer and dispatch a single SetAttrs call
// instead of the legacy Chmod / Lchown / Chtimes sequence. We verify by
// counting HSET commands against inode keys — the fast path is one HSET,
// the legacy path would be three.
func TestSetFileAttributesApplyDispatchesToBatchSetAttrer(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)

	const fsKey = "nfs-batchsetattrs"
	var inodeHSets atomic.Int64
	rdb.AddHook(&hsetCounterHook{
		filter: ":inode:",
		count:  &inodeHSets,
	})

	c := client.New(rdb, fsKey)
	fs := New(c, false)
	if _, _, err := c.CreateFile(ctx, "/apply.txt", 0o644, false); err != nil {
		t.Fatalf("seed: %v", err)
	}

	mode := uint32(0o600)
	uid := uint32(7000)
	gid := uint32(7001)
	// Pick atime/mtime that are definitely different from the seed mtime
	// set by CreateFile so the no-op skip doesn't hide the HSET.
	atime := time.UnixMilli(2000000000000)
	mtime := time.UnixMilli(2000000001000)
	sfa := &nfs.SetFileAttributes{
		SetMode:  &mode,
		SetUID:   &uid,
		SetGID:   &gid,
		SetAtime: &atime,
		SetMtime: &mtime,
	}

	inodeHSets.Store(0)
	if err := sfa.Apply(fs, fs, "/apply.txt"); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := inodeHSets.Load(); got != 1 {
		t.Fatalf("SetFileAttributes.Apply issued %d HSETs against inode keys, want 1 (fast path)", got)
	}

	// Double-check the values actually landed.
	st, err := c.Stat(ctx, "/apply.txt")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if st.Mode != 0o600 {
		t.Errorf("mode = %o, want 600", st.Mode)
	}
	if st.UID != 7000 {
		t.Errorf("uid = %d, want 7000", st.UID)
	}
	if st.GID != 7001 {
		t.Errorf("gid = %d, want 7001", st.GID)
	}
	if st.Atime != 2000000000000 {
		t.Errorf("atime_ms = %d, want 2000000000000", st.Atime)
	}
	if st.Mtime != 2000000001000 {
		t.Errorf("mtime_ms = %d, want 2000000001000", st.Mtime)
	}
}

// TestSetFileAttributesApplyEmptyDiffIsFreeRider verifies the no-op skip
// ("candidate 7" free-rider win): when SetFileAttributes.Apply receives a
// request whose mode/uid/gid/atime/mtime already match the current state,
// it must not touch Redis at all. Historically that would have been 1-3
// redundant round trips.
func TestSetFileAttributesApplyEmptyDiffIsFreeRider(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)

	const fsKey = "nfs-batchsetattrs-noop"
	var inodeHSets atomic.Int64
	rdb.AddHook(&hsetCounterHook{
		filter: ":inode:",
		count:  &inodeHSets,
	})

	c := client.New(rdb, fsKey)
	fs := New(c, false)
	if _, _, err := c.CreateFile(ctx, "/noop.txt", 0o644, false); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Read the seed state. Then construct a SetFileAttributes whose
	// fields match exactly. Apply must skip the call entirely.
	st, err := c.Stat(ctx, "/noop.txt")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	currentMode := uint32(st.Mode)
	currentUID := uint32(st.UID)
	currentGID := uint32(st.GID)
	currentAtime := time.UnixMilli(st.Atime)
	currentMtime := time.UnixMilli(st.Mtime)
	sfa := &nfs.SetFileAttributes{
		SetMode:  &currentMode,
		SetUID:   &currentUID,
		SetGID:   &currentGID,
		SetAtime: &currentAtime,
		SetMtime: &currentMtime,
	}

	inodeHSets.Store(0)
	if err := sfa.Apply(fs, fs, "/noop.txt"); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := inodeHSets.Load(); got != 0 {
		t.Fatalf("Apply with no-op diff issued %d HSETs, want 0 (free-rider skip)", got)
	}
}

// hsetCounterHook is a tiny redis.Hook that increments `count` every time
// an HSET command is issued against a key containing `filter`. Used by the
// BatchSetAttrer dispatcher tests.
type hsetCounterHook struct {
	filter string
	count  *atomic.Int64
}

func (h *hsetCounterHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h *hsetCounterHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		h.maybeCount(cmd)
		return next(ctx, cmd)
	}
}
func (h *hsetCounterHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		for _, cmd := range cmds {
			h.maybeCount(cmd)
		}
		return next(ctx, cmds)
	}
}
func (h *hsetCounterHook) maybeCount(cmd redis.Cmder) {
	args := cmd.Args()
	if len(args) < 2 {
		return
	}
	name, _ := args[0].(string)
	if name != "hset" && name != "HSET" {
		return
	}
	key, _ := args[1].(string)
	if h.filter != "" && !containsString(key, h.filter) {
		return
	}
	h.count.Add(1)
}

func containsString(s, needle string) bool {
	return len(s) >= len(needle) && indexString(s, needle) >= 0
}

func indexString(s, needle string) int {
	// small and explicit to avoid pulling strings into the test file just
	// for a filter helper.
	n := len(needle)
	if n == 0 {
		return 0
	}
	for i := 0; i+n <= len(s); i++ {
		if s[i:i+n] == needle {
			return i
		}
	}
	return -1
}
