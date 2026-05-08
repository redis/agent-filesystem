package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	case "create":
		return runWorkspaceQueryIndexCreate(workspace, args[1:])
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

func runWorkspaceQueryIndexCreate(workspace string, args []string) error {
	return runWorkspaceQueryIndexRebuildWithOptions(workspace, args, true)
}

func runWorkspaceQueryIndexRebuild(workspace string, args []string) error {
	return runWorkspaceQueryIndexRebuildWithOptions(workspace, args, false)
}

func runWorkspaceQueryIndexRebuildWithOptions(workspace string, args []string, create bool) error {
	fs := flag.NewFlagSet("query index rebuild", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var path string
	var wait bool
	var force bool
	var jsonOut bool
	var embeddings bool
	fs.StringVar(&path, "path", "/", "workspace path scope")
	fs.BoolVar(&wait, "wait", false, "wait for rebuild completion")
	fs.BoolVar(&force, "force", false, "rebuild existing chunks")
	fs.BoolVar(&jsonOut, "json", false, "write JSON output")
	fs.BoolVar(&embeddings, "embeddings", create, "build semantic embeddings")
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
	request := controlplane.WorkspaceQueryIndexRebuildRequest{
		Workspace:  remote.selection.Name,
		Path:       normalizeFSRemotePath(path),
		Force:      force,
		Wait:       wait || create,
		Embeddings: embeddings,
	}
	var response controlplane.WorkspaceQueryIndexRebuildResponse
	if !jsonOut && (request.Wait || request.Embeddings) {
		response, err = rebuildQueryIndexWithProgress(ctx, remote, request)
	} else {
		response, err = remote.controlPlane.RebuildQueryIndex(ctx, remote.selection.ID, request)
	}
	if err != nil {
		return err
	}
	return writeWorkspaceQueryIndexRebuild(response, jsonOut)
}

func rebuildQueryIndexWithProgress(ctx context.Context, remote *fsRemoteWorkspace, request controlplane.WorkspaceQueryIndexRebuildRequest) (controlplane.WorkspaceQueryIndexRebuildResponse, error) {
	type rebuildResult struct {
		response controlplane.WorkspaceQueryIndexRebuildResponse
		err      error
	}
	done := make(chan rebuildResult, 1)
	go func() {
		response, err := remote.controlPlane.RebuildQueryIndex(ctx, remote.selection.ID, request)
		done <- rebuildResult{response: response, err: err}
	}()

	started := time.Now()
	fmt.Fprintf(os.Stderr, "Building query index for %s (%s)", remote.selection.Name, normalizeFSRemotePath(request.Path))
	if request.Embeddings {
		fmt.Fprint(os.Stderr, ": keyword chunks and semantic embeddings")
	} else {
		fmt.Fprint(os.Stderr, ": keyword chunks")
	}
	fmt.Fprintln(os.Stderr)

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case result := <-done:
			return result.response, result.err
		case <-ticker.C:
			fmt.Fprintf(os.Stderr, "Still building query index for %s after %s...\n", remote.selection.Name, formatStepDuration(time.Since(started)))
		}
	}
}

func runWorkspaceQueryIndexClean(workspace string, args []string) error {
	fs := flag.NewFlagSet("query index clean", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var jsonOut bool
	var yes bool
	fs.BoolVar(&jsonOut, "json", false, "write JSON output")
	fs.BoolVar(&yes, "yes", false, "confirm removal of generated query index data")
	fs.BoolVar(&yes, "y", false, "confirm removal of generated query index data")
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
	if !yes {
		ok, err := confirmWorkspaceQueryIndexClean(remote.selection.Name)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Println()
			fmt.Println("Query index clean cancelled.")
			fmt.Println()
			return nil
		}
	}
	response, err := remote.controlPlane.CleanQueryIndex(ctx, remote.selection.ID, controlplane.WorkspaceQueryIndexCleanRequest{
		Workspace: remote.selection.Name,
		Confirm:   true,
	})
	if err != nil {
		return err
	}
	return writeWorkspaceQueryIndexClean(response, jsonOut)
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
	writeWorkspaceQueryIndexField("workspace", status.Workspace)
	if status.Path != "" {
		writeWorkspaceQueryIndexField("path", status.Path)
	}
	writeWorkspaceQueryIndexField("state", status.State)
	writeWorkspaceQueryIndexField("backend", "keyword bm25")
	writeWorkspaceQueryIndexField("redissearch", fmt.Sprintf("%t", status.Keyword.SearchAvailable))
	writeWorkspaceQueryIndexField("files", fmt.Sprintf("%d", status.Keyword.Files))
	writeWorkspaceQueryIndexField("ready", fmt.Sprintf("%d", status.Keyword.Ready))
	writeWorkspaceQueryIndexField("pending", fmt.Sprintf("%d", status.Keyword.Pending))
	writeWorkspaceQueryIndexField("stale", fmt.Sprintf("%d", status.Keyword.Stale))
	writeWorkspaceQueryIndexField("unindexed", fmt.Sprintf("%d", status.Keyword.Unindexed))
	writeWorkspaceQueryIndexField("skipped", fmt.Sprintf("%d", status.Keyword.Skipped))
	writeWorkspaceQueryIndexField("errors", fmt.Sprintf("%d", status.Keyword.Errors))
	writeWorkspaceQueryIndexField("chunks", fmt.Sprintf("%d", status.Keyword.Chunks))
	writeWorkspaceQueryIndexField("embeddings", "global "+embeddingStatusLabel(status.Embeddings))
	if status.Embeddings.Provider != "" {
		writeWorkspaceQueryIndexField("embedding_provider", status.Embeddings.Provider)
	}
	if status.Embeddings.Model != "" {
		writeWorkspaceQueryIndexField("embedding_model", status.Embeddings.Model)
	}
	if status.Embeddings.Message != "" && !status.Embeddings.Available {
		writeWorkspaceQueryIndexField("embedding_message", status.Embeddings.Message)
	}
	if status.Message != "" {
		fmt.Fprintln(os.Stdout)
		fmt.Fprintln(os.Stdout, status.Message)
	}
	return nil
}

func writeWorkspaceQueryIndexField(label, value string) {
	fmt.Fprintf(os.Stdout, "%-18s %s\n", label, value)
}

func embeddingStatusLabel(status controlplane.QueryEmbeddingStatus) string {
	if status.Available {
		return "ready"
	}
	if status.Enabled {
		return "unavailable"
	}
	return "off"
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
	if response.Embeddings != nil {
		fmt.Fprintf(os.Stdout, "embeddings %s\n", embeddingBackfillLabel(*response.Embeddings))
		fmt.Fprintf(os.Stdout, "embedded   %d\n", response.Embeddings.Embedded)
		fmt.Fprintf(os.Stdout, "scanned    %d\n", response.Embeddings.Scanned)
		if response.Embeddings.Message != "" {
			fmt.Fprintf(os.Stdout, "embedding_message %s\n", response.Embeddings.Message)
		}
	}
	fmt.Fprintf(os.Stdout, "state      %s\n", response.Status.State)
	if response.Message != "" {
		fmt.Fprintln(os.Stdout)
		fmt.Fprintln(os.Stdout, response.Message)
	}
	return nil
}

func embeddingBackfillLabel(result controlplane.QueryEmbeddingBackfillResult) string {
	if result.Available {
		return "ready"
	}
	if result.Enabled {
		return "unavailable"
	}
	return "off"
}

func writeWorkspaceQueryIndexClean(response controlplane.WorkspaceQueryIndexCleanResponse, jsonOut bool) error {
	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(response)
	}
	fmt.Fprintln(os.Stdout, "Query index clean")
	fmt.Fprintln(os.Stdout)
	fmt.Fprintf(os.Stdout, "workspace  %s\n", response.Workspace)
	fmt.Fprintf(os.Stdout, "cleared    %t\n", response.Cleared)
	fmt.Fprintf(os.Stdout, "files      %d\n", response.RemovedFiles)
	fmt.Fprintf(os.Stdout, "chunks     %d\n", response.RemovedChunks)
	fmt.Fprintf(os.Stdout, "state      %s\n", response.Status.State)
	if response.Message != "" {
		fmt.Fprintln(os.Stdout)
		fmt.Fprintln(os.Stdout, response.Message)
	}
	return nil
}

func confirmWorkspaceQueryIndexClean(workspace string) (bool, error) {
	return confirmWorkspaceQueryIndexCleanWithReader(workspace, bufio.NewReader(os.Stdin))
}

func confirmWorkspaceQueryIndexCleanWithReader(workspace string, reader *bufio.Reader) (bool, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return false, nil
	}
	fmt.Println()
	fmt.Printf("Are you sure you want to clear generated query index data for %s? Workspace files will not change. [y/N] ", workspace)
	raw, err := reader.ReadString('\n')
	if err != nil && strings.TrimSpace(raw) == "" {
		fmt.Println()
		return false, nil
	}
	fmt.Println()
	answer := strings.ToLower(strings.TrimSpace(raw))
	return answer == "y" || answer == "yes", nil
}

func workspaceQueryIndexUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %[1]s query index <status|create|rebuild|clean> [flags]
  %[1]s fs [workspace] query index <status|create|rebuild|clean> [flags]

Manage the query index for a workspace.

Subcommands:
  status             Show keyword query projection and embedding state
  create             Build keyword chunks and semantic embeddings
  rebuild            Enqueue existing files for keyword query indexing
  clean              Clear generated query index data for the workspace

Flags:
  --json             Write JSON output
  --path <path>      Scope status or rebuild to a workspace path
  --wait             Wait for rebuild completion
  --force            Rebuild existing chunks
  --embeddings       Build semantic embeddings
  --yes, -y          Confirm full-workspace query index cleanup

Examples:
  %[1]s query index status
  %[1]s fs repo query index create --embeddings --wait
  %[1]s fs repo query index rebuild --path /cmd/afs --wait
  %[1]s query index clean --yes
`, bin)
}
