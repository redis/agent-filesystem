package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/redis/agent-filesystem/internal/controlplane"
)

func runWorkspaceQueryIndex(workspace string, args []string) error {
	if len(args) == 0 || isHelpArg(args[0]) {
		fmt.Fprint(os.Stderr, workspaceQueryIndexUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	subcommand := strings.TrimSpace(args[0])
	switch subcommand {
	case "status":
		return runWorkspaceQueryIndexStatus(workspace, args[1:])
	case "rebuild":
		return runWorkspaceQueryIndexRebuild(workspace, args[1:])
	case "clean":
		return runWorkspaceQueryIndexClean(workspace, args[1:])
	default:
		return fmt.Errorf("unknown query index subcommand %q\n\n%s", subcommand, workspaceQueryIndexUsageText(filepath.Base(os.Args[0])))
	}
}

func runWorkspaceQueryIndexStatus(workspace string, args []string) error {
	fs := flag.NewFlagSet("query index status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var jsonOut bool
	var path string
	fs.BoolVar(&jsonOut, "json", false, "write JSON output")
	fs.StringVar(&path, "path", "/", "workspace path scope")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%s", workspaceQueryIndexUsageText(filepath.Base(os.Args[0])))
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("%s", workspaceQueryIndexUsageText(filepath.Base(os.Args[0])))
	}
	status, err := workspaceQueryIndexStatusForWorkspace(workspace, path)
	if err != nil {
		return err
	}
	return writeWorkspaceQueryIndexStatus(status, jsonOut)
}

func runWorkspaceQueryIndexRebuild(workspace string, args []string) error {
	fs := flag.NewFlagSet("query index rebuild", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var path string
	var wait bool
	var force bool
	var jsonOut bool
	fs.StringVar(&path, "path", "/", "workspace path scope")
	fs.BoolVar(&wait, "wait", false, "wait for rebuild completion")
	fs.BoolVar(&force, "force", false, "rebuild existing chunks")
	fs.BoolVar(&jsonOut, "json", false, "write JSON output")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%s", workspaceQueryIndexUsageText(filepath.Base(os.Args[0])))
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("%s", workspaceQueryIndexUsageText(filepath.Base(os.Args[0])))
	}
	ctx := context.Background()
	remote, err := openFSRemoteWorkspace(ctx, workspace)
	if err != nil {
		return err
	}
	defer remote.close()
	response, err := remote.controlPlane.RebuildQueryIndex(ctx, remote.selection.ID, controlplane.WorkspaceQueryIndexRebuildRequest{
		Workspace: remote.selection.Name,
		Path:      normalizeFSRemotePath(path),
		Force:     force,
		Wait:      wait,
	})
	if err != nil {
		return err
	}
	return writeWorkspaceQueryIndexRebuild(response, jsonOut)
}

func runWorkspaceQueryIndexClean(workspace string, args []string) error {
	fs := flag.NewFlagSet("query index clean", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var jsonOut bool
	fs.BoolVar(&jsonOut, "json", false, "write JSON output")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%s", workspaceQueryIndexUsageText(filepath.Base(os.Args[0])))
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("%s", workspaceQueryIndexUsageText(filepath.Base(os.Args[0])))
	}
	status, err := workspaceQueryIndexStatusForWorkspace(workspace, "/")
	if err != nil {
		return err
	}
	status.State = "clean"
	status.Message = "No query index data was removed."
	return writeWorkspaceQueryIndexStatus(status, jsonOut)
}

func workspaceQueryIndexStatusForWorkspace(workspace, path string) (controlplane.WorkspaceQueryIndexStatus, error) {
	ctx := context.Background()
	remote, err := openFSRemoteWorkspace(ctx, workspace)
	if err != nil {
		return controlplane.WorkspaceQueryIndexStatus{}, err
	}
	defer remote.close()
	return remote.controlPlane.QueryIndexStatus(ctx, remote.selection.ID, controlplane.WorkspaceQueryIndexStatusRequest{
		Workspace: remote.selection.Name,
		Path:      normalizeFSRemotePath(path),
	})
}

func writeWorkspaceQueryIndexStatus(status controlplane.WorkspaceQueryIndexStatus, jsonOut bool) error {
	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	}
	fmt.Fprintln(os.Stdout, "Query index")
	fmt.Fprintln(os.Stdout)
	fmt.Fprintf(os.Stdout, "workspace   %s\n", status.Workspace)
	if status.Path != "" {
		fmt.Fprintf(os.Stdout, "path        %s\n", status.Path)
	}
	fmt.Fprintf(os.Stdout, "state       %s\n", status.State)
	fmt.Fprintf(os.Stdout, "backend     keyword bm25\n")
	fmt.Fprintf(os.Stdout, "redissearch %t\n", status.Keyword.SearchAvailable)
	fmt.Fprintf(os.Stdout, "files       %d\n", status.Keyword.Files)
	fmt.Fprintf(os.Stdout, "ready       %d\n", status.Keyword.Ready)
	fmt.Fprintf(os.Stdout, "pending     %d\n", status.Keyword.Pending)
	fmt.Fprintf(os.Stdout, "stale       %d\n", status.Keyword.Stale)
	fmt.Fprintf(os.Stdout, "unindexed   %d\n", status.Keyword.Unindexed)
	fmt.Fprintf(os.Stdout, "skipped     %d\n", status.Keyword.Skipped)
	fmt.Fprintf(os.Stdout, "errors      %d\n", status.Keyword.Errors)
	fmt.Fprintf(os.Stdout, "chunks      %d\n", status.Keyword.Chunks)
	fmt.Fprintf(os.Stdout, "embeddings  %t\n", status.EmbeddingsEnabled)
	if status.Model != "" {
		fmt.Fprintf(os.Stdout, "model       %s\n", status.Model)
	}
	if status.ChunkStrategy != "" {
		fmt.Fprintf(os.Stdout, "strategy    %s\n", status.ChunkStrategy)
	}
	if status.Message != "" {
		fmt.Fprintln(os.Stdout)
		fmt.Fprintln(os.Stdout, status.Message)
	}
	return nil
}

func writeWorkspaceQueryIndexRebuild(response controlplane.WorkspaceQueryIndexRebuildResponse, jsonOut bool) error {
	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(response)
	}
	fmt.Fprintln(os.Stdout, "Query index rebuild")
	fmt.Fprintln(os.Stdout)
	fmt.Fprintf(os.Stdout, "workspace  %s\n", response.Workspace)
	if response.Path != "" {
		fmt.Fprintf(os.Stdout, "path       %s\n", response.Path)
	}
	fmt.Fprintf(os.Stdout, "enqueued   %d\n", response.Keyword.Enqueued)
	fmt.Fprintf(os.Stdout, "waited     %t\n", response.Keyword.Waited)
	if response.Keyword.Waited {
		fmt.Fprintf(os.Stdout, "processed  %d\n", response.Keyword.Process.Processed)
		fmt.Fprintf(os.Stdout, "indexed    %d\n", response.Keyword.Process.Indexed)
		fmt.Fprintf(os.Stdout, "skipped    %d\n", response.Keyword.Process.Skipped)
		fmt.Fprintf(os.Stdout, "pending    %d\n", response.Keyword.Process.Pending)
	}
	fmt.Fprintf(os.Stdout, "state      %s\n", response.Status.State)
	if response.Message != "" {
		fmt.Fprintln(os.Stdout)
		fmt.Fprintln(os.Stdout, response.Message)
	}
	return nil
}

func workspaceQueryIndexUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %[1]s query index <status|rebuild|clean> [flags]
  %[1]s fs [workspace] query index <status|rebuild|clean> [flags]

Manage the query index for a workspace.

Subcommands:
  status             Show keyword query projection and embedding state
  rebuild            Enqueue existing files for keyword query indexing
  clean              Remove stale query index data

Flags:
  --json             Write JSON output
  --path <path>      Scope status or rebuild to a workspace path
  --wait             Wait for rebuild completion
  --force            Rebuild existing chunks

Examples:
  %[1]s query index status
  %[1]s fs repo query index rebuild --path /cmd/afs --wait
`, bin)
}
