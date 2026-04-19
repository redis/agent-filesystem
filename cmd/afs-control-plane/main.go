package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/redis/agent-filesystem/internal/controlplane"
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

	auth, err := controlplane.LoadAuthHandlerFromEnv()
	if err != nil {
		fatal(err)
	}
	server := &http.Server{
		Addr:              *listenAddr,
		Handler:           controlplane.NewHandlerWithOptions(manager, controlplane.HandlerOptions{AllowOrigin: *allowOrigin, Auth: auth}),
		ReadHeaderTimeout: 5 * time.Second,
	}

	fmt.Fprintf(os.Stderr, "AFS control plane listening on http://%s\n", *listenAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
