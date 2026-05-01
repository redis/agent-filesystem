package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/redis/agent-filesystem/internal/controlplane"
	"github.com/redis/agent-filesystem/internal/version"
)

func main() {
	listenAddr := flag.String("listen", defaultListenAddr(), "bind address")
	allowOrigin := flag.String("allow-origin", "*", "Access-Control-Allow-Origin value")
	configPath := flag.String("config", defaultConfigPath(), "path to afs.config.json")
	flag.Parse()

	manager, err := controlplane.OpenDatabaseManager(*configPath)
	if err != nil {
		fatal(err)
	}
	defer manager.Close()

	// On self-hosted boot, ensure a `getting-started` workspace exists so a
	// fresh `afs auth login --self-hosted && afs setup` lands the user on a usable
	// workspace without any manual create step. Idempotent; cloud deploys opt
	// out via AFS_SEED_GETTING_STARTED=0 (Vercel entrypoint sets this).
	if controlplane.ShouldSeedGettingStarted() {
		seedCtx, seedCancel := context.WithTimeout(context.Background(), 15*time.Second)
		if err := manager.SeedGettingStarted(seedCtx); err != nil {
			fmt.Fprintln(os.Stderr, "warn: seed getting-started workspace:", err)
		}
		seedCancel()
	}

	auth, err := controlplane.LoadAuthHandlerFromEnv()
	if err != nil {
		fatal(err)
	}
	server := &http.Server{
		Addr:              *listenAddr,
		Handler:           controlplane.NewHandlerWithOptions(manager, controlplane.HandlerOptions{AllowOrigin: *allowOrigin, Auth: auth}),
		ReadHeaderTimeout: 5 * time.Second,
	}

	fmt.Fprintf(os.Stderr, "AFS control plane %s — listening on http://%s\n", version.String(), *listenAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
