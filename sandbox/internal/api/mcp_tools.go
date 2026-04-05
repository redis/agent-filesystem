package api

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis-fs/sandbox/internal/executor"
)

func (s *MCPServer) callTool(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	switch name {
	case "sandbox_launch":
		return s.toolLaunch(ctx, args)
	case "sandbox_read":
		return s.toolRead(args)
	case "sandbox_write":
		return s.toolWrite(args)
	case "sandbox_kill":
		return s.toolKill(args)
	case "sandbox_list":
		return s.toolList()
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

func (s *MCPServer) toolLaunch(ctx context.Context, args map[string]interface{}) (string, error) {
	command, _ := args["command"].(string)
	if command == "" {
		return "", fmt.Errorf("command is required")
	}

	opts := executor.LaunchOptions{Command: command}

	if cwd, ok := args["cwd"].(string); ok {
		opts.Cwd = cwd
	}
	if timeout, ok := args["timeout_secs"].(float64); ok {
		opts.Timeout = time.Duration(timeout) * time.Second
	}
	if wait, ok := args["wait"].(bool); ok {
		opts.Wait = wait
	}
	if keepStdin, ok := args["keep_stdin_open"].(bool); ok {
		opts.KeepStdinOpen = keepStdin
	}

	result, err := s.manager.Launch(ctx, opts)
	if err != nil {
		return "", err
	}

	out, _ := json.MarshalIndent(result, "", "  ")
	return string(out), nil
}

func (s *MCPServer) toolRead(args map[string]interface{}) (string, error) {
	id, _ := args["id"].(string)
	if id == "" {
		return "", fmt.Errorf("id is required")
	}

	result, err := s.manager.Read(id)
	if err != nil {
		return "", err
	}

	out, _ := json.MarshalIndent(result, "", "  ")
	return string(out), nil
}

func (s *MCPServer) toolWrite(args map[string]interface{}) (string, error) {
	id, _ := args["id"].(string)
	input, _ := args["input"].(string)
	if id == "" {
		return "", fmt.Errorf("id is required")
	}

	if err := s.manager.Write(id, input); err != nil {
		return "", err
	}
	return "OK", nil
}

func (s *MCPServer) toolKill(args map[string]interface{}) (string, error) {
	id, _ := args["id"].(string)
	if id == "" {
		return "", fmt.Errorf("id is required")
	}

	if err := s.manager.Kill(id); err != nil {
		return "", err
	}
	return "OK", nil
}

func (s *MCPServer) toolList() (string, error) {
	procs := s.manager.List()
	out, _ := json.MarshalIndent(procs, "", "  ")
	return string(out), nil
}

