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

	"github.com/redis/agent-filesystem/sandbox/internal/api"
	"github.com/redis/agent-filesystem/sandbox/internal/executor"
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
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down...")
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
