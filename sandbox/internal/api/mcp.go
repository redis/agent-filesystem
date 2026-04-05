package api

import (
	"bufio"
	"context"
	"encoding/json"
	"io"

	"github.com/redis-fs/sandbox/internal/executor"
)

// MCP JSON-RPC types
type MCPRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type MCPResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *MCPError   `json:"error,omitempty"`
}

type MCPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MCPServer handles MCP protocol over stdio.
type MCPServer struct {
	manager *executor.Manager
}

// NewMCPServer creates a new MCP server.
func NewMCPServer(manager *executor.Manager) *MCPServer {
	return &MCPServer{manager: manager}
}

// Run starts the MCP server reading from r and writing to w.
func (s *MCPServer) Run(ctx context.Context, r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	encoder := json.NewEncoder(w)

	for scanner.Scan() {
		line := scanner.Bytes()
		var req MCPRequest
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}

		resp := s.handleRequest(ctx, &req)
		encoder.Encode(resp)
	}
	return scanner.Err()
}

func (s *MCPServer) handleRequest(ctx context.Context, req *MCPRequest) *MCPResponse {
	resp := &MCPResponse{JSONRPC: "2.0", ID: req.ID}

	switch req.Method {
	case "initialize":
		resp.Result = map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{"tools": map[string]bool{}},
			"serverInfo":      map[string]string{"name": "redis-fs-sandbox", "version": "1.0.0"},
		}

	case "tools/list":
		resp.Result = map[string]interface{}{"tools": s.getTools()}

	case "tools/call":
		var params struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		}
		json.Unmarshal(req.Params, &params)
		result, err := s.callTool(ctx, params.Name, params.Arguments)
		if err != nil {
			resp.Error = &MCPError{Code: -32000, Message: err.Error()}
		} else {
			resp.Result = map[string]interface{}{
				"content": []map[string]string{{"type": "text", "text": result}},
			}
		}

	default:
		resp.Error = &MCPError{Code: -32601, Message: "method not found"}
	}

	return resp
}

func (s *MCPServer) getTools() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"name":        "sandbox_launch",
			"description": "Launch a process in the sandbox",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"command":         map[string]string{"type": "string", "description": "Shell command"},
					"cwd":             map[string]string{"type": "string", "description": "Working directory"},
					"timeout_secs":    map[string]string{"type": "integer", "description": "Timeout"},
					"wait":            map[string]string{"type": "boolean", "description": "Wait for completion"},
					"keep_stdin_open": map[string]string{"type": "boolean", "description": "Keep stdin open"},
				},
				"required": []string{"command"},
			},
		},
		{
			"name":        "sandbox_read",
			"description": "Read output from a sandbox process",
			"inputSchema": map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{"id": map[string]string{"type": "string"}},
				"required":   []string{"id"},
			},
		},
		{
			"name":        "sandbox_write",
			"description": "Write to a sandbox process stdin",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id":    map[string]string{"type": "string"},
					"input": map[string]string{"type": "string"},
				},
				"required": []string{"id", "input"},
			},
		},
		{
			"name":        "sandbox_kill",
			"description": "Kill a sandbox process",
			"inputSchema": map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{"id": map[string]string{"type": "string"}},
				"required":   []string{"id"},
			},
		},
		{
			"name":        "sandbox_list",
			"description": "List all sandbox processes",
			"inputSchema": map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
	}
}

