package nfsfs

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rowantrollope/agent-filesystem/mount/internal/client"
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
