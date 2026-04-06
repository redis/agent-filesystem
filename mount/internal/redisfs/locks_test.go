package redisfs

import (
	"context"
	"net"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
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

func TestFileHandleReleaseUnlocksLocks(t *testing.T) {
	t.Parallel()
	rdb, ctx := setupTestRedis(t)
	c := client.New(rdb, "redisfs-locks")

	if err := c.Echo(ctx, "/file.txt", []byte("content")); err != nil {
		t.Fatalf("echo: %v", err)
	}
	st, err := c.Stat(ctx, "/file.txt")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	h1 := newFileHandle("/file.txt", st.Inode, c, &FSNode{})
	h2 := newFileHandle("/file.txt", st.Inode, c, &FSNode{})
	lock := &fuse.FileLock{Start: 0, End: 99, Typ: syscall.F_WRLCK, Pid: 111}

	if errno := h1.Setlk(ctx, 0, lock, 0); errno != 0 {
		t.Fatalf("setlk h1: %v", errno)
	}
	if errno := h2.Setlk(ctx, 0, lock, 0); errno != syscall.EAGAIN {
		t.Fatalf("expected EAGAIN for conflicting lock, got %v", errno)
	}
	if errno := h1.Release(ctx); errno != 0 {
		t.Fatalf("release h1: %v", errno)
	}
	if errno := h2.Setlk(ctx, 0, lock, 0); errno != 0 {
		t.Fatalf("setlk h2 after release: %v", errno)
	}
}
