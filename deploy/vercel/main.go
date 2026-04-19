package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/redis/agent-filesystem/internal/controlplane"
)

const (
	configPathEnvVar  = "AFS_CONFIG_PATH"
	allowOriginEnvVar = "AFS_ALLOW_ORIGIN"
)

func main() {
	listenAddr := defaultListenAddr()
	configPath := strings.TrimSpace(os.Getenv(configPathEnvVar))
	allowOrigin := strings.TrimSpace(os.Getenv(allowOriginEnvVar))
	if allowOrigin == "" {
		allowOrigin = "*"
	}

	manager, err := controlplane.OpenDatabaseManager(configPath)
	if err != nil {
		fatal(err)
	}
	defer manager.Close()

	auth, err := controlplane.LoadAuthHandlerFromEnv()
	if err != nil {
		fatal(err)
	}

	server := &http.Server{
		Addr:              listenAddr,
		Handler:           controlplane.NewHandlerWithOptions(manager, controlplane.HandlerOptions{AllowOrigin: allowOrigin, Auth: auth}),
		ReadHeaderTimeout: 5 * time.Second,
	}

	fmt.Fprintf(os.Stderr, "AFS control plane listening on http://%s\n", listenAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fatal(err)
	}
}

func defaultListenAddr() string {
	if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
		return ":" + port
	}
	return "127.0.0.1:8091"
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
