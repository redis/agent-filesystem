package main

import (
	"context"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/redis/agent-filesystem/mount/internal/client"
	"github.com/redis/agent-filesystem/mount/internal/nfsfs"
	"github.com/redis/agent-filesystem/mount/internal/redisconn"
	"github.com/redis/go-redis/v9"
	"github.com/willscott/go-nfs"
	"github.com/willscott/go-nfs/helpers"
)

// opCounterHook counts Redis commands. Enabled via AFS_NFS_OPSTATS=1. Used
// for performance instrumentation; logs delta every second while non-zero.
type opCounterHook struct {
	cmds     atomic.Int64
	pipeCmds atomic.Int64
}

func (h *opCounterHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h *opCounterHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		h.cmds.Add(1)
		return next(ctx, cmd)
	}
}
func (h *opCounterHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		h.cmds.Add(1) // one pipeline = one round trip
		h.pipeCmds.Add(int64(len(cmds)))
		return next(ctx, cmds)
	}
}

type authCompatHandler struct {
	nfs.Handler
}

const nfsHandleCacheLimit = 16384

// macOS directory enumeration will often follow a READDIR/READDIRPLUS with a
// burst of LOOKUP/GETATTR calls for the same entries. Keep path metadata warm
// long enough to collapse those repeated round-trips to Redis.
const nfsClientCacheTTL = time.Hour

func (h authCompatHandler) Mount(ctx context.Context, conn net.Conn, req nfs.MountRequest) (nfs.MountStatus, billy.Filesystem, []nfs.AuthFlavor) {
	status, fs, flavors := h.Handler.Mount(ctx, conn, req)
	if status != nfs.MountStatusOk {
		return status, fs, flavors
	}

	hasNull := false
	hasUnix := false
	for _, fl := range flavors {
		if fl == nfs.AuthFlavorNull {
			hasNull = true
		}
		if fl == nfs.AuthFlavorUnix {
			hasUnix = true
		}
	}
	if !hasUnix {
		flavors = append(flavors, nfs.AuthFlavorUnix)
	}
	if !hasNull {
		flavors = append(flavors, nfs.AuthFlavorNull)
	}
	return status, fs, flavors
}

func (h authCompatHandler) RenameHandle(fs billy.Filesystem, oldPath, newPath []string) error {
	if renamer, ok := h.Handler.(nfs.HandleRenamer); ok {
		return renamer.RenameHandle(fs, oldPath, newPath)
	}
	return h.Handler.InvalidateHandle(fs, h.Handler.ToHandle(fs, oldPath))
}

func (h authCompatHandler) InvalidateVerifier(path string) {
	if invalidator, ok := h.Handler.(nfs.VerifierInvalidator); ok {
		invalidator.InvalidateVerifier(path)
	}
}

func newNFSHandler(fs billy.Filesystem) authCompatHandler {
	baseHandler := helpers.NewNullAuthHandler(fs)
	// macOS creates AppleDouble sidecar files (._*) for files and directories on NFS mounts.
	// Keep a larger handle cache so recursive reads/searches do not evict live handles mid-walk.
	return authCompatHandler{Handler: helpers.NewCachingHandler(baseHandler, nfsHandleCacheLimit)}
}

func main() {
	redisAddr := flag.String("redis", "localhost:6379", "Redis server address")
	redisUser := flag.String("user", "", "Redis username")
	redisPassword := flag.String("password", "", "Redis password")
	redisDB := flag.Int("db", 0, "Redis database number")
	redisTLS := flag.Bool("tls", false, "Use TLS for the Redis connection")
	listenAddr := flag.String("listen", "127.0.0.1:20490", "Listen address for NFS server")
	exportPath := flag.String("export", "/myfs", "Exported NFS path")
	readOnly := flag.Bool("readonly", false, "Export read-only")
	foreground := flag.Bool("foreground", true, "Run in foreground")
	disableInvalidation := flag.Bool("disable-cross-client-invalidation", false, "Disable Redis pub/sub cache invalidation between clients. Falls back to TTL-based staleness.")
	flag.Parse()

	if !*foreground {
		log.Printf("--foreground=false is not supported; running foreground")
	}

	exp := strings.TrimSpace(*exportPath)
	if exp == "" || !strings.HasPrefix(exp, "/") {
		log.Fatalf("invalid --export %q: expected absolute path", *exportPath)
	}

	rdb := redis.NewClient(redisconn.Options(redisconn.Config{
		Addr:       *redisAddr,
		Username:   *redisUser,
		Password:   *redisPassword,
		DB:         *redisDB,
		PoolSize:   16,
		TLSEnabled: *redisTLS,
	}))
	defer rdb.Close()

	var opHook *opCounterHook
	if os.Getenv("AFS_NFS_OPSTATS") == "1" {
		opHook = &opCounterHook{}
		rdb.AddHook(opHook)
		go func() {
			var last, lastPipe int64
			for range time.Tick(1 * time.Second) {
				c := opHook.cmds.Load()
				p := opHook.pipeCmds.Load()
				if c != last || p != lastPipe {
					log.Printf("opstats: cmds+pipes=%d (delta %d), pipelined_cmds=%d (delta %d)", c, c-last, p, p-lastPipe)
					last = c
					lastPipe = p
				}
			}
		}()
	}

	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("cannot connect to Redis at %s: %v", *redisAddr, err)
	}

	redisKey := strings.TrimPrefix(exp, "/")
	if redisKey == "" {
		redisKey = "myfs"
	}
	c := client.NewWithCache(rdb, redisKey, nfsClientCacheTTL)
	if err := c.Mkdir(ctx, "/"); err != nil {
		log.Fatalf("failed to initialize key %q: %v", redisKey, err)
	}
	if warmer, ok := c.(client.PathCacheWarmer); ok {
		start := time.Now()
		if err := warmer.WarmPathCache(ctx); err != nil {
			log.Printf("warning: path cache warmup failed for %q: %v", redisKey, err)
		} else {
			log.Printf("Prewarmed path cache for %q in %s", redisKey, time.Since(start).Round(time.Millisecond))
		}
	}

	// Cross-client cache invalidation. The NFS mount is the primary
	// beneficiary here: nfsClientCacheTTL is one hour, so without this a
	// write on host A is hidden from host B for up to an hour. The
	// SubscribeInvalidations handler can be a no-op because the client
	// layer cache is the only cache to flush and SubscribeInvalidations
	// handles that internally.
	subscriberCtx, cancelSubscriber := context.WithCancel(context.Background())
	defer cancelSubscriber()
	if *disableInvalidation {
		c.DisableInvalidationPublishing()
		log.Printf("Cross-client cache invalidation: DISABLED (flag)")
	} else {
		if err := c.SubscribeInvalidations(subscriberCtx, nil); err != nil {
			log.Printf("warning: failed to start invalidation subscriber: %v", err)
		} else {
			log.Printf("Cross-client cache invalidation: enabled (channel afs:{%s}:invalidate)", redisKey)
		}
	}

	listener, err := net.Listen("tcp", *listenAddr)
	if err != nil {
		log.Fatalf("listen failed on %s: %v", *listenAddr, err)
	}
	defer listener.Close()

	fs := nfsfs.New(c, *readOnly)
	handler := newNFSHandler(fs)

	log.Printf("Serving Redis key %q via NFS at %s", redisKey, *listenAddr)
	log.Printf("Export path: %s", exp)
	log.Printf("Mount target example: %s:%s", hostOnly(*listenAddr), exp)
	log.Printf("NFS advisory locking is disabled for this export. Mount clients should use nolock/nolocks.")
	log.Printf("FUSE record locks are Redis-backed and inode-keyed, but they are not propagated over NFS yet.")

	errCh := make(chan error, 1)
	go func() {
		errCh <- nfs.Serve(listener, handler)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Printf("Received signal %v, shutting down", sig)
		cancelSubscriber()
		_ = listener.Close()
	case err := <-errCh:
		cancelSubscriber()
		if err != nil {
			log.Fatalf("nfs server failed: %v", err)
		}
	}
}

func hostOnly(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return "127.0.0.1"
	}
	if host == "" || host == "0.0.0.0" {
		return "127.0.0.1"
	}
	if host == "::" {
		return "::1"
	}
	return host
}
