package mcpproto

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string     `json:"jsonrpc"`
	ID      any        `json:"id,omitempty"`
	Result  any        `json:"result,omitempty"`
	Error   *ErrorBody `json:"error,omitempty"`
}

type ErrorBody struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type TextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type ToolResult struct {
	Content           []TextContent `json:"content"`
	StructuredContent any           `json:"structuredContent,omitempty"`
	IsError           bool          `json:"isError,omitempty"`
}

type ToolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type Provider interface {
	Tools(ctx context.Context) []Tool
	CallTool(ctx context.Context, name string, args map[string]any) ToolResult
}

type ProviderFunc struct {
	ToolsFn    func(ctx context.Context) []Tool
	CallToolFn func(ctx context.Context, name string, args map[string]any) ToolResult
}

func (p ProviderFunc) Tools(ctx context.Context) []Tool {
	if p.ToolsFn == nil {
		return nil
	}
	return p.ToolsFn(ctx)
}

func (p ProviderFunc) CallTool(ctx context.Context, name string, args map[string]any) ToolResult {
	if p.CallToolFn == nil {
		return ToolResult{
			Content: []TextContent{{Type: "text", Text: "mcp provider is unavailable"}},
			IsError: true,
		}
	}
	return p.CallToolFn(ctx, name, args)
}

type Server struct {
	ProtocolVersion string
	Name            string
	Version         string
	Instructions    string
	Provider        Provider
}

func (s *Server) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	reader := bufio.NewReader(r)
	writer := bufio.NewWriter(w)

	for {
		payload, err := ReadFrame(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if len(bytes.TrimSpace(payload)) == 0 {
			continue
		}

		var req Request
		if err := json.Unmarshal(payload, &req); err != nil {
			if err := WriteFrame(writer, Response{
				JSONRPC: "2.0",
				Error:   &ErrorBody{Code: -32700, Message: "parse error"},
			}); err != nil {
				return err
			}
			if err := writer.Flush(); err != nil {
				return err
			}
			continue
		}

		resp := s.HandleRequest(ctx, req)
		if resp == nil {
			continue
		}
		if err := WriteFrame(writer, *resp); err != nil {
			return err
		}
		if err := writer.Flush(); err != nil {
			return err
		}
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	defer r.Body.Close()
	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeHTTPJSON(w, http.StatusBadRequest, Response{
			JSONRPC: "2.0",
			Error:   &ErrorBody{Code: -32700, Message: "parse error"},
		})
		return
	}

	resp := s.HandleRequest(r.Context(), req)
	if resp == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeHTTPJSON(w, http.StatusOK, resp)
}

func (s *Server) HandleRequest(ctx context.Context, req Request) *Response {
	if req.Method == "" {
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &ErrorBody{Code: -32600, Message: "invalid request"},
		}
	}

	if strings.HasPrefix(req.Method, "notifications/") && req.ID == nil {
		return nil
	}

	resp := &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
	}

	switch req.Method {
	case "initialize":
		resp.Result = map[string]any{
			"protocolVersion": s.ProtocolVersion,
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"serverInfo": map[string]string{
				"name":    s.Name,
				"version": s.Version,
			},
			"instructions": s.Instructions,
		}
	case "ping":
		resp.Result = map[string]any{}
	case "tools/list":
		resp.Result = map[string]any{
			"tools": s.tools(ctx),
		}
	case "tools/call":
		var params ToolCallParams
		if len(req.Params) > 0 {
			if err := json.Unmarshal(req.Params, &params); err != nil {
				resp.Error = &ErrorBody{Code: -32602, Message: "invalid tools/call params"}
				return resp
			}
		}
		if params.Arguments == nil {
			params.Arguments = map[string]any{}
		}
		resp.Result = s.callTool(ctx, params.Name, params.Arguments)
	default:
		resp.Error = &ErrorBody{Code: -32601, Message: "method not found"}
	}

	return resp
}

func (s *Server) tools(ctx context.Context) []Tool {
	if s == nil || s.Provider == nil {
		return nil
	}
	return s.Provider.Tools(ctx)
}

func (s *Server) callTool(ctx context.Context, name string, args map[string]any) ToolResult {
	if s == nil || s.Provider == nil {
		return ToolResult{
			Content: []TextContent{{
				Type: "text",
				Text: "mcp provider is unavailable",
			}},
			IsError: true,
		}
	}
	return s.Provider.CallTool(ctx, name, args)
}

func ReadFrame(r *bufio.Reader) ([]byte, error) {
	contentLength := -1

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) && line == "" {
				return nil, io.EOF
			}
			return nil, err
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if contentLength >= 0 {
				break
			}
			continue
		}

		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(parts[0]), "Content-Length") {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil || n < 0 {
			return nil, fmt.Errorf("invalid Content-Length header %q", trimmed)
		}
		contentLength = n
	}

	if contentLength < 0 {
		return nil, errors.New("missing Content-Length header")
	}

	payload := make([]byte, contentLength)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func WriteFrame(w io.Writer, resp Response) error {
	body, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	_, err = w.Write(body)
	return err
}

func StructuredResult(value any) ToolResult {
	text := ""
	switch v := value.(type) {
	case string:
		text = v
	default:
		body, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			text = fmt.Sprintf("%v", value)
		} else {
			text = string(body)
		}
	}
	return ToolResult{
		Content: []TextContent{{
			Type: "text",
			Text: text,
		}},
		StructuredContent: value,
	}
}

func writeHTTPJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
