// Command sandbox runs the Agent Filesystem sandbox server.
//
// SECURITY / THREAT MODEL
//
// The sandbox HTTP API runs arbitrary shell commands sent in the request
// body (POST /processes -> sh -c <command>). It has no authentication, no
// TLS, and no rate limiting. Any client that can reach the listen address
// can execute code as the sandbox process user, in the workspace directory.
//
// This is intentional: the sandbox is meant to be reached only by tightly
// trusted callers on the same host (e.g. an MCP host running as the same
// user, or a wrapper that gates access). It is NOT a public-facing
// service.
//
// To preserve that assumption the default bind address is 127.0.0.1.
// Binding to 0.0.0.0 or any externally reachable interface requires the
// caller to pass --bind explicitly and accept the consequences.
//
// If you need the sandbox to be reachable across hosts:
//   - terminate auth at a proxy that fronts it, AND
//   - bind the sandbox itself to 127.0.0.1 (or a unix-only interface),
//     so the proxy is the only path in.
//
// MCP transport (--transport stdio) does not open a network socket and is
// not affected by these binding rules.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/agent-filesystem/sandbox/internal/api"
	"github.com/redis/agent-filesystem/sandbox/internal/executor"
)

// gcInterval is how often the process-map sweeper runs.
// gcMaxAge is how long terminal-state processes (exited / killed / timed-out)
// are retained before they are pruned. Long enough that a CLI client can
// reasonably read the result back, short enough that a long-lived sandbox
// doesn't accumulate forever.
const (
	gcInterval = 5 * time.Minute
	gcMaxAge   = 1 * time.Hour
)

func main() {
	bind := flag.String("bind", "127.0.0.1", "HTTP server bind address. Default 127.0.0.1; set to 0.0.0.0 to expose externally (see security note in source).")
	port := flag.Int("port", 8090, "HTTP server port")
	workspace := flag.String("workspace", "/workspace", "Workspace directory")
	transport := flag.String("transport", "http", "Transport: http or stdio (MCP)")

	flag.Parse()

	manager := executor.NewManager(*workspace)

	if *transport == "stdio" {
		// Run MCP server over stdio
		mcp := api.NewMCPServer(manager)
		if err := mcp.Run(context.Background(), os.Stdin, os.Stdout); err != nil {
			log.Fatalf("MCP server error: %v", err)
		}
		return
	}

	// HTTP server
	server := api.NewServer(manager)
	addr := fmt.Sprintf("%s:%d", *bind, *port)

	httpServer := &http.Server{
		Addr:    addr,
		Handler: server.Handler(),
		// Bound the request side; the wait endpoints stream
		// long-running process completion, so WriteTimeout is
		// intentionally not set.
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Periodic process-map GC so terminal-state entries don't accumulate.
	gcCtx, cancelGC := context.WithCancel(context.Background())
	defer cancelGC()
	go runProcessGC(gcCtx, manager)

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down...")
		cancelGC()
		httpServer.Shutdown(context.Background())
	}()

	log.Printf("Sandbox server listening on %s", addr)
	if *bind != "127.0.0.1" && *bind != "localhost" {
		log.Printf("WARNING: bind=%s exposes shell-execution endpoints beyond localhost. There is no auth on this API.", *bind)
	}
	log.Printf("Workspace: %s", *workspace)
	log.Printf("Endpoints:")
	log.Printf("  POST   /processes       - Launch process")
	log.Printf("  GET    /processes       - List processes")
	log.Printf("  GET    /processes/{id}  - Read process output")
	log.Printf("  POST   /processes/{id}/write - Write to stdin")
	log.Printf("  POST   /processes/{id}/wait  - Wait for completion")
	log.Printf("  DELETE /processes/{id}  - Kill process")

	if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}

func runProcessGC(ctx context.Context, m *executor.Manager) {
	tick := time.NewTicker(gcInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if removed := m.GC(gcMaxAge); removed > 0 {
				log.Printf("sandbox: pruned %d terminal-state process(es)", removed)
			}
		}
	}
}
