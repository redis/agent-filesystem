package main

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/redis/agent-filesystem/internal/controlplane"
	"github.com/redis/go-redis/v9"
)

const (
	productModeLocal        = "local"
	legacyProductModeDirect = "direct"
	productModeDirect       = productModeLocal
	productModeSelfHosted   = "self-hosted"
	productModeCloud        = "cloud"
)

func productModeDisplayLabel(mode string) string {
	switch mode {
	case productModeLocal:
		return "Local"
	case productModeSelfHosted:
		return "Self Managed"
	case productModeCloud:
		return "Cloud Managed"
	default:
		return mode
	}
}

func productModeStatusLabel(mode string) string {
	switch mode {
	case productModeLocal:
		return "local"
	case productModeSelfHosted:
		return "self managed"
	case productModeCloud:
		return "cloud managed"
	default:
		return mode
	}
}

func effectiveProductMode(cfg config) (string, error) {
	mode := strings.TrimSpace(cfg.ProductMode)
	switch mode {
	case "":
		return productModeLocal, nil
	case productModeLocal, legacyProductModeDirect:
		return productModeLocal, nil
	case productModeSelfHosted, productModeCloud:
		return mode, nil
	default:
		return "", fmt.Errorf("unknown connection %q in config (expected one of: local, self-hosted, cloud)", cfg.ProductMode)
	}
}

func normalizeControlPlaneURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("controlPlane.url is required when connection is not local")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid control plane url %q: %w", raw, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported control plane url scheme %q (expected http or https)", parsed.Scheme)
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", fmt.Errorf("control plane url must include host[:port]")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

// rewriteManagedRedisAddrForLocalhost maps Docker-internal Redis hostnames like
// "redis:6379" to "localhost:6379" when the control plane itself is being
// reached through localhost from the host machine. This keeps the Docker
// Compose quickstart usable from the host CLI without changing the control
// plane's own internal Redis connection.
func rewriteManagedRedisAddrForLocalhost(controlPlaneURL, redisAddr string) string {
	parsed, err := url.Parse(strings.TrimSpace(controlPlaneURL))
	if err != nil {
		return redisAddr
	}

	switch parsed.Hostname() {
	case "localhost", "127.0.0.1", "::1":
	default:
		return redisAddr
	}

	host, port, err := splitAddr(strings.TrimSpace(redisAddr))
	if err != nil {
		return redisAddr
	}
	host = strings.TrimSpace(host)
	switch host {
	case "", "localhost", "127.0.0.1", "::1":
		return redisAddr
	}
	if net.ParseIP(host) != nil {
		return redisAddr
	}
	if strings.Contains(host, ".") {
		return redisAddr
	}
	return fmt.Sprintf("localhost:%d", port)
}

type afsControlPlane interface {
	ListWorkspaceSummaries(ctx context.Context) (controlplane.WorkspaceListResponse, error)
	GetWorkspace(ctx context.Context, workspace string) (controlplane.WorkspaceDetail, error)
	CreateWorkspace(ctx context.Context, input controlplane.CreateWorkspaceRequest) (controlplane.WorkspaceDetail, error)
	ImportWorkspace(ctx context.Context, input controlplane.ImportWorkspaceRequest) (controlplane.ImportWorkspaceResponse, error)
	DeleteWorkspace(ctx context.Context, workspace string) error
	CreateWorkspaceSession(ctx context.Context, workspace string, input controlplane.CreateWorkspaceSessionRequest) (controlplane.WorkspaceSession, error)
	HeartbeatWorkspaceSession(ctx context.Context, workspace, sessionID string) (controlplane.WorkspaceSessionInfo, error)
	CloseWorkspaceSession(ctx context.Context, workspace, sessionID string) error
	ListCheckpoints(ctx context.Context, workspace string, limit int) ([]controlplane.CheckpointSummary, error)
	RestoreCheckpoint(ctx context.Context, workspace, checkpointID string) error
	SaveCheckpoint(ctx context.Context, input controlplane.SaveCheckpointRequest) (bool, error)
	SaveCheckpointFromLive(ctx context.Context, workspace, checkpointID string) (bool, error)
	ForkWorkspace(ctx context.Context, sourceWorkspace, newWorkspace string) error
}

type afsBackendSession struct {
	cfg          config
	store        *afsStore
	controlPlane afsControlPlane
	close        func()
}

type afsBackend interface {
	ProductMode() string
	OpenSession(ctx context.Context, cfg config) (*afsBackendSession, error)
}

type directBackend struct{}
type selfHostedBackend struct{}

func (directBackend) ProductMode() string {
	return productModeLocal
}

func (directBackend) OpenSession(ctx context.Context, cfg config) (*afsBackendSession, error) {
	rdb := redis.NewClient(buildRedisOptions(cfg, 8))
	closeFn := func() {
		_ = rdb.Close()
	}
	if err := rdb.Ping(ctx).Err(); err != nil {
		closeFn()
		return nil, fmt.Errorf("cannot connect to Redis at %s: %w\nRun '%s up' first or point AFS at an existing Redis server",
			cfg.RedisAddr, err, filepath.Base(os.Args[0]))
	}

	store := newAFSStore(rdb)
	return &afsBackendSession{
		cfg:          cfg,
		store:        store,
		controlPlane: controlPlaneServiceFromStore(cfg, store),
		close:        closeFn,
	}, nil
}

func (selfHostedBackend) ProductMode() string {
	return productModeSelfHosted
}

func (selfHostedBackend) OpenSession(ctx context.Context, cfg config) (*afsBackendSession, error) {
	client, resolvedDatabaseID, err := newHTTPControlPlaneClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	cfg.DatabaseID = resolvedDatabaseID
	return &afsBackendSession{
		cfg:          cfg,
		controlPlane: client,
		close:        func() {},
	}, nil
}

func productBackendForConfig(cfg config) (afsBackend, error) {
	productMode, err := effectiveProductMode(cfg)
	if err != nil {
		return nil, err
	}

	switch productMode {
	case productModeLocal:
		return directBackend{}, nil
	case productModeSelfHosted, productModeCloud:
		return selfHostedBackend{}, nil
	default:
		return nil, fmt.Errorf("unknown connection %q", productMode)
	}
}

func openAFSBackendSession(ctx context.Context) (*afsBackendSession, error) {
	cfg, err := loadAFSConfig()
	if err != nil {
		return nil, err
	}

	return openAFSBackendSessionForConfig(ctx, cfg)
}

func openAFSBackendSessionForConfig(ctx context.Context, cfg config) (*afsBackendSession, error) {
	if err := resolveConfigPaths(&cfg); err != nil {
		return nil, err
	}

	backend, err := productBackendForConfig(cfg)
	if err != nil {
		return nil, err
	}

	session, err := backend.OpenSession(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if session.close == nil {
		session.close = func() {}
	}
	return session, nil
}

func openAFSControlPlane(ctx context.Context) (config, afsControlPlane, func(), error) {
	session, err := openAFSBackendSession(ctx)
	if err != nil {
		return config{}, nil, func() {}, err
	}
	return session.cfg, session.controlPlane, session.close, nil
}

func openAFSControlPlaneForConfig(ctx context.Context, cfg config) (config, afsControlPlane, func(), error) {
	session, err := openAFSBackendSessionForConfig(ctx, cfg)
	if err != nil {
		return config{}, nil, func() {}, err
	}
	return session.cfg, session.controlPlane, session.close, nil
}
