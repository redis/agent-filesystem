// agent-filesystem-mount mounts an Agent Filesystem volume via FUSE.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/redis/agent-filesystem/internal/controlplane"
	"github.com/redis/agent-filesystem/mount/internal/afsfs"
	"github.com/redis/agent-filesystem/mount/internal/client"
	"github.com/redis/agent-filesystem/mount/internal/redisconn"
	"github.com/redis/go-redis/v9"
)

const (
	afsDaemonEnv = "AFS_DAEMON"
)

func main() {
	redisAddr := flag.String("redis", "localhost:6379", "Redis server address")
	redisUser := flag.String("user", "", "Redis username")
	redisPassword := flag.String("password", "", "Redis password")
	redisDB := flag.Int("db", 0, "Redis database number")
	redisTLS := flag.Bool("tls", false, "Use TLS for the Redis connection")
	attrTimeout := flag.Float64("attr-timeout", 1.0, "Attribute cache TTL in seconds")
	readOnly := flag.Bool("readonly", false, "Mount read-only")
	allowOther := flag.Bool("allow-other", false, "Allow other users to access mount")
	foreground := flag.Bool("foreground", true, "Run in foreground")
	debug := flag.Bool("debug", false, "Enable FUSE debug logging")
	disableInvalidation := flag.Bool("disable-cross-client-invalidation", false, "Disable Redis pub/sub cache invalidation between clients. Falls back to TTL-based staleness.")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags] <redis-key> <mountpoint>\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Mount an Agent Filesystem volume via FUSE.\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if flag.NArg() != 2 {
		flag.Usage()
		os.Exit(1)
	}

	// Optional daemon mode: re-exec detached and return in parent.
	if !*foreground && os.Getenv(afsDaemonEnv) != "1" {
		args := make([]string, 0, len(os.Args))
		for i := 1; i < len(os.Args); i++ {
			a := os.Args[i]
			if a == "--foreground" {
				i++ // skip value as well
				continue
			}
			if strings.HasPrefix(a, "--foreground=") {
				continue
			}
			args = append(args, a)
		}
		args = append(args, "--foreground=true")

		cmd := exec.Command(os.Args[0], args...)
		cmd.Env = append(os.Environ(), afsDaemonEnv+"=1")
		devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
		if err != nil {
			log.Fatalf("daemon mode failed opening %s: %v", os.DevNull, err)
		}
		defer devNull.Close()
		cmd.Stdin = devNull
		cmd.Stdout = devNull
		cmd.Stderr = devNull
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := cmd.Start(); err != nil {
			log.Fatalf("daemon mode failed: %v", err)
		}
		fmt.Printf("agent-filesystem-mount started in background (pid %d)\n", cmd.Process.Pid)
		return
	}

	redisKey := flag.Arg(0)
	mountpoint := flag.Arg(1)

	// Verify mountpoint exists.
	fi, err := os.Stat(mountpoint)
	if err != nil {
		log.Fatalf("mountpoint error: %v", err)
	}
	if !fi.IsDir() {
		log.Fatalf("mountpoint %s is not a directory", mountpoint)
	}

	// Connect to Redis.
	rdb := redis.NewClient(redisconn.Options(redisconn.Config{
		Addr:       *redisAddr,
		Username:   *redisUser,
		Password:   *redisPassword,
		DB:         *redisDB,
		PoolSize:   16,
		TLSEnabled: *redisTLS,
	}))

	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("cannot connect to Redis at %s: %v", *redisAddr, err)
	}

	c := client.NewWithObserver(rdb, redisKey, controlplane.NewMountVersionObserver(rdb))

	uid, gid := afsfs.GetOwnership()

	opts := &afsfs.Options{
		AttrTimeout:                    time.Duration(*attrTimeout * float64(time.Second)),
		ReadOnly:                       *readOnly,
		AllowOther:                     *allowOther,
		Debug:                          *debug,
		UID:                            uid,
		GID:                            gid,
		DisableCrossClientInvalidation: *disableInvalidation,
	}

	log.Printf("Mounting AFS key %q at %s", redisKey, mountpoint)
	log.Printf("Redis: %s (db %d)", *redisAddr, *redisDB)
	if *disableInvalidation {
		log.Printf("Cross-client cache invalidation: DISABLED (flag)")
	}

	// Subscriber lifetime is tied to shutdown; cancelling mountCtx tears
	// the pub/sub goroutine down.
	mountCtx, cancelMount := context.WithCancel(context.Background())
	defer cancelMount()

	server, err := afsfs.Mount(mountCtx, mountpoint, c, opts)
	if err != nil {
		log.Fatalf("mount failed: %v", err)
	}

	// Handle shutdown signals.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		log.Printf("Received signal %v, unmounting...", sig)
		cancelMount()
		err := server.Unmount()
		if err != nil {
			log.Printf("Unmount error: %v", err)
		}
	}()

	log.Printf("Filesystem mounted. Press Ctrl+C to unmount.")
	server.Wait()
	log.Printf("Unmounted.")

	rdb.Close()
}
