// Command sandbox runs the Redis-FS sandbox server.
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

	"github.com/redis-fs/sandbox/internal/api"
	"github.com/redis-fs/sandbox/internal/executor"
)

func main() {
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
	addr := fmt.Sprintf(":%d", *port)

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

