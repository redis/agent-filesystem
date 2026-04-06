package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/rowantrollope/agent-filesystem/cli/internal/controlplane"
)

func main() {
	listenAddr := flag.String("listen", "127.0.0.1:8091", "bind address")
	allowOrigin := flag.String("allow-origin", "*", "Access-Control-Allow-Origin value")
	configPath := flag.String("config", "", "path to afs.config.json")
	flag.Parse()

	cfg, err := controlplane.LoadConfig(*configPath)
	if err != nil {
		fatal(err)
	}
	store, closeStore, err := controlplane.OpenStore(context.Background(), cfg)
	if err != nil {
		fatal(err)
	}
	defer closeStore()

	service := controlplane.NewService(cfg, store)
	server := &http.Server{
		Addr:              *listenAddr,
		Handler:           controlplane.NewHandler(service, *allowOrigin),
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
