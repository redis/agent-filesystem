package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/redis/agent-filesystem/internal/controlplane"
)

func managedWorkspaceSessionRequest(cfg config) controlplane.CreateWorkspaceSessionRequest {
	productMode, err := effectiveProductMode(cfg)
	if err != nil || productMode == productModeDirect {
		return controlplane.CreateWorkspaceSessionRequest{}
	}

	hostname, err := os.Hostname()
	if err != nil {
		hostname = ""
	}
	clientKind := "sync"
	if mode, err := effectiveMode(cfg); err == nil && mode == modeMount {
		clientKind = "mount"
	}
	return controlplane.CreateWorkspaceSessionRequest{
		ClientKind:      clientKind,
		AFSVersion:      "dev",
		Hostname:        strings.TrimSpace(hostname),
		OperatingSystem: runtime.GOOS,
		LocalPath:       strings.TrimSpace(cfg.LocalPath),
		Readonly:        cfg.ReadOnly,
	}
}

func startManagedWorkspaceSessionLifecycle(cfg config, workspace, sessionID string, heartbeatEvery time.Duration) (func(), error) {
	sessionID = strings.TrimSpace(sessionID)
	workspace = strings.TrimSpace(workspace)
	if sessionID == "" || workspace == "" || heartbeatEvery <= 0 {
		return func() {}, nil
	}

	_, service, closeFn, err := openAFSControlPlaneForConfig(context.Background(), cfg)
	if err != nil {
		return nil, err
	}

	stopCh := make(chan struct{})
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		ticker := time.NewTicker(heartbeatEvery)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				_, err := service.HeartbeatWorkspaceSession(ctx, workspace, sessionID)
				cancel()
				if err != nil && !errors.Is(err, os.ErrNotExist) {
					fmt.Fprintf(os.Stderr, "afs sync: session heartbeat failed: %v\n", err)
				}
			}
		}
	}()

	return func() {
		close(stopCh)
		<-doneCh
		closeManagedWorkspaceSessionWithService(service, closeFn, workspace, sessionID)
	}, nil
}

func closeManagedWorkspaceSession(cfg config, workspace, sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	workspace = strings.TrimSpace(workspace)
	if sessionID == "" || workspace == "" {
		return
	}

	_, service, closeFn, err := openAFSControlPlaneForConfig(context.Background(), cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "afs sync: session cleanup failed to connect to control plane: %v\n", err)
		return
	}
	closeManagedWorkspaceSessionWithService(service, closeFn, workspace, sessionID)
}

func closeManagedWorkspaceSessionWithService(service afsControlPlane, closeFn func(), workspace, sessionID string) {
	defer closeFn()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := service.CloseWorkspaceSession(ctx, workspace, sessionID); err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "afs sync: session cleanup failed: %v\n", err)
	}
}

func configFromState(st state) config {
	cfg := loadConfigOrDefault()
	if strings.TrimSpace(st.ProductMode) != "" {
		cfg.ProductMode = strings.TrimSpace(st.ProductMode)
	}
	if strings.TrimSpace(st.ControlPlaneURL) != "" {
		cfg.URL = strings.TrimSpace(st.ControlPlaneURL)
	}
	if strings.TrimSpace(st.ControlPlaneDatabase) != "" {
		cfg.DatabaseID = strings.TrimSpace(st.ControlPlaneDatabase)
	}
	if strings.TrimSpace(st.CurrentWorkspace) != "" {
		cfg.CurrentWorkspace = strings.TrimSpace(st.CurrentWorkspace)
	}
	if strings.TrimSpace(st.CurrentWorkspaceID) != "" {
		cfg.CurrentWorkspaceID = strings.TrimSpace(st.CurrentWorkspaceID)
	}
	return cfg
}
