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

	// The Vercel build is the cloud control plane. Let install.sh and the UI
	// runtime config report this unless an operator explicitly overrides it
	// (e.g. for a preview deployment impersonating self-hosted).
	if strings.TrimSpace(os.Getenv(controlplane.ProductModeEnvVar)) == "" {
		_ = os.Setenv(controlplane.ProductModeEnvVar, controlplane.ProductModeCloud)
	}

	// Unpack the embedded CLI bundle (populated at deploy time by prod.sh) and
	// point the control plane at the extracted directory so /v1/cli can serve
	// the matching binary on Vercel, where the project filesystem doesn't
	// include non-Go artifacts by default. The embed is the source of truth
	// when it exists, so it overrides any pre-set AFS_CLI_ARTIFACT_DIR.
	if dir, err := extractCLIBundle(); err != nil {
		fmt.Fprintln(os.Stderr, "warn: extract CLI bundle:", err)
	} else if dir != "" {
		_ = os.Setenv("AFS_CLI_ARTIFACT_DIR", dir)
		fmt.Fprintln(os.Stderr, "extracted CLI bundle to", dir)
	} else {
		fmt.Fprintln(os.Stderr, "no embedded CLI bundle; /v1/cli will use normal resolver")
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
