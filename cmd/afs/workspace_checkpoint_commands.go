package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
	"github.com/rowantrollope/agent-filesystem/internal/controlplane"
)

const (
	afsInitialCheckpointName = "initial"
)

func cloneManifest(source manifest) manifest {
	cloned := manifest{
		Version:   source.Version,
		Workspace: source.Workspace,
		Savepoint: source.Savepoint,
		Entries:   make(map[string]manifestEntry, len(source.Entries)),
	}
	for path, entry := range source.Entries {
		cloned.Entries[path] = entry
	}
	return cloned
}

func cmdWorkspace(args []string) error {
	if len(args) < 2 || isHelpArg(args[1]) {
		fmt.Fprint(os.Stderr, workspaceUsageText(filepath.Base(os.Args[0])))
		return nil
	}

	switch args[1] {
	case "create":
		return cmdWorkspaceCreate(args)
	case "list":
		return cmdWorkspaceList(args)
	case "current":
		return cmdWorkspaceCurrent(args)
	case "use":
		return cmdWorkspaceUse(args)
	case "run":
		return cmdWorkspaceRun(args)
	case "clone":
		return cmdWorkspaceClone(args)
	case "fork":
		return cmdWorkspaceFork(args)
	case "delete":
		return cmdWorkspaceDelete(args)
	case "import":
		return cmdWorkspaceImport(args)
	default:
		return fmt.Errorf("unknown workspace subcommand %q\n\n%s", args[1], workspaceUsageText(filepath.Base(os.Args[0])))
	}
}

func cmdCheckpoint(args []string) error {
	if len(args) < 2 || isHelpArg(args[1]) {
		fmt.Fprint(os.Stderr, checkpointUsageText(filepath.Base(os.Args[0])))
		return nil
	}

	switch args[1] {
	case "create":
		return cmdCheckpointCreate(args)
	case "list":
		return cmdCheckpointList(args)
	case "restore":
		return cmdCheckpointRestore(args)
	default:
		return fmt.Errorf("unknown checkpoint subcommand %q\n\n%s", args[1], checkpointUsageText(filepath.Base(os.Args[0])))
	}
}

func cmdWorkspaceCreate(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, workspaceCreateUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	if len(args) != 3 {
		return fmt.Errorf("%s", workspaceCreateUsageText(filepath.Base(os.Args[0])))
	}

	workspace := args[2]
	if err := validateAFSName("workspace", workspace); err != nil {
		return err
	}

	cfg, store, closeStore, err := openAFSStore(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	ctx := context.Background()
	if err := createEmptyWorkspace(ctx, cfg, store, workspace); err != nil {
		return err
	}

	printBox(clr(ansiBGreen, "●")+" "+clr(ansiBold, "workspace created"), []boxRow{
		{Label: "workspace", Value: workspace},
		{Label: "checkpoint", Value: afsInitialCheckpointName},
		{Label: "run", Value: filepath.Base(os.Args[0]) + " workspace run " + workspace + " -- /bin/sh"},
	})
	return nil
}

func createEmptyWorkspace(ctx context.Context, cfg config, store *afsStore, workspace string) error {
	service := controlPlaneServiceFromStore(cfg, store)
	_, err := service.CreateWorkspace(ctx, controlplane.CreateWorkspaceRequest{
		Name: workspace,
		Source: controlplane.SourceRef{
			Kind: controlplane.SourceBlank,
		},
	})
	return err
}

func cmdWorkspaceList(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, workspaceListUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	if len(args) != 2 {
		return fmt.Errorf("%s", workspaceListUsageText(filepath.Base(os.Args[0])))
	}

	cfg, store, closeStore, err := openAFSStore(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	service := controlPlaneServiceFromStore(cfg, store)
	ctx := context.Background()
	workspaces, err := service.ListWorkspaceSummaries(ctx)
	if err != nil {
		return err
	}

	rows := make([]boxRow, 0, len(workspaces.Items))
	for _, meta := range workspaces.Items {
		rows = append(rows, boxRow{
			Label: meta.Name,
			Value: fmt.Sprintf("%d checkpoints · updated %s", meta.CheckpointCount, meta.UpdatedAt),
		})
	}
	if len(rows) == 0 {
		rows = append(rows, boxRow{Value: clr(ansiDim, "No workspaces found")})
	}
	printBox(clr(ansiBold, "workspaces"), rows)
	return nil
}

func cmdWorkspaceCurrent(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, workspaceCurrentUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	if len(args) != 2 {
		return fmt.Errorf("%s", workspaceCurrentUsageText(filepath.Base(os.Args[0])))
	}

	cfg, _, err := loadConfigWithPresence()
	if err != nil {
		return err
	}
	if err := prepareConfigForSave(&cfg); err != nil {
		return err
	}

	printBox(clr(ansiBold, "current workspace"), []boxRow{
		{Label: "workspace", Value: currentWorkspaceLabel(cfg.CurrentWorkspace)},
	})
	return nil
}

func cmdWorkspaceUse(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, workspaceUseUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	if len(args) != 3 {
		return fmt.Errorf("%s", workspaceUseUsageText(filepath.Base(os.Args[0])))
	}

	workspace := strings.TrimSpace(args[2])
	if err := validateAFSName("workspace", workspace); err != nil {
		return err
	}

	cfg, store, closeStore, err := openAFSStore(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	exists, err := store.workspaceExists(context.Background(), workspace)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("workspace %q does not exist", workspace)
	}

	cfg.CurrentWorkspace = workspace
	if err := prepareConfigForSave(&cfg); err != nil {
		return err
	}
	if err := saveConfig(cfg); err != nil {
		return err
	}

	printBox(clr(ansiBGreen, "●")+" "+clr(ansiBold, "current workspace updated"), []boxRow{
		{Label: "workspace", Value: workspace},
		{Label: "config", Value: clr(ansiDim, compactDisplayPath(configPath()))},
	})
	return nil
}

func cmdWorkspaceRun(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, workspaceRunUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	parsed, childArgs, err := parseAFSCommandInvocation(args[2:])
	if err != nil {
		return err
	}
	if len(parsed.positionals) > 1 {
		return fmt.Errorf("%s", workspaceRunUsageText(filepath.Base(os.Args[0])))
	}

	cfg, store, closeStore, err := openAFSStore(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	workspace := ""
	if len(parsed.positionals) == 1 {
		workspace = parsed.positionals[0]
	}
	workspace, err = resolveWorkspaceName(context.Background(), cfg, store, workspace)
	if err != nil {
		return err
	}
	if err := validateAFSName("workspace", workspace); err != nil {
		return err
	}

	if err := runAFSCommand(context.Background(), cfg, store, workspace, childArgs, parsed.readonly); err != nil {
		if errors.Is(err, errAFSWorkspaceConflict) {
			return fmt.Errorf("workspace %q moved since this working copy was materialized; inspect it before running again", workspace)
		}
		return err
	}
	return nil
}

func cmdWorkspaceDelete(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, workspaceDeleteUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	if len(args) < 3 {
		return fmt.Errorf("%s", workspaceDeleteUsageText(filepath.Base(os.Args[0])))
	}
	names := args[2:]

	cfg, store, closeStore, err := openAFSStore(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	ctx := context.Background()
	service := controlPlaneServiceFromStore(cfg, store)
	deleted := make([]string, 0, len(names))
	for _, name := range names {
		if err := validateAFSName("workspace", name); err != nil {
			return err
		}

		step := startStep("Deleting workspace " + name)
		if err := service.DeleteWorkspace(ctx, name); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				step.fail("does not exist")
				return fmt.Errorf("workspace %q does not exist", name)
			}
			step.fail(err.Error())
			return err
		}
		if err := removeLocalWorkspace(cfg, name); err != nil {
			step.fail(err.Error())
			return err
		}
		step.succeed(name)
		deleted = append(deleted, name)
	}

	rows := make([]boxRow, 0, len(deleted)+1)
	rows = append(rows, boxRow{Label: "deleted", Value: strconv.Itoa(len(deleted))})
	for _, name := range deleted {
		rows = append(rows, boxRow{Value: name})
	}
	printBox(clr(ansiBGreen, "●")+" "+clr(ansiBold, "workspaces deleted"), rows)
	return nil
}

func cmdWorkspaceClone(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, workspaceCloneUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	if len(args) != 4 {
		return fmt.Errorf("%s", workspaceCloneUsageText(filepath.Base(os.Args[0])))
	}
	workspace := args[2]
	targetDir := args[3]
	if err := validateAFSName("workspace", workspace); err != nil {
		return err
	}

	cfg, _, _, err := openAFSStore(context.Background())
	if err != nil {
		return err
	}

	clonedPath, err := prepareWorkspaceCloneTarget(targetDir)
	if err != nil {
		return err
	}

	if err := materializeWorkspaceToPath(context.Background(), cfg, workspace, clonedPath); err != nil {
		return err
	}

	printBox(clr(ansiBGreen, "●")+" "+clr(ansiBold, "workspace cloned"), []boxRow{
		{Label: "workspace", Value: workspace},
		{Label: "path", Value: clonedPath},
		{Label: "next", Value: "cd " + clonedPath},
	})
	return nil
}

func prepareWorkspaceCloneTarget(targetDir string) (string, error) {
	clonedPath, err := expandPath(targetDir)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(clonedPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return clonedPath, nil
		}
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("destination path %s already exists and is not a directory", clonedPath)
	}
	entries, err := os.ReadDir(clonedPath)
	if err != nil {
		return "", err
	}
	if len(entries) > 0 {
		return "", fmt.Errorf("destination path %s already exists and is not an empty directory", clonedPath)
	}
	return clonedPath, nil
}

func cmdWorkspaceFork(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, workspaceForkUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	if len(args) != 4 {
		return fmt.Errorf("%s", workspaceForkUsageText(filepath.Base(os.Args[0])))
	}
	sourceWorkspace := args[2]
	newWorkspace := args[3]
	if err := validateAFSName("workspace", sourceWorkspace); err != nil {
		return err
	}
	if err := validateAFSName("workspace", newWorkspace); err != nil {
		return err
	}

	cfg, store, closeStore, err := openAFSStore(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	ctx := context.Background()
	service := controlPlaneServiceFromStore(cfg, store)
	if err := service.ForkWorkspace(ctx, sourceWorkspace, newWorkspace); err != nil {
		return err
	}

	printBox(clr(ansiBGreen, "●")+" "+clr(ansiBold, "workspace forked"), []boxRow{
		{Label: "from", Value: sourceWorkspace},
		{Label: "new", Value: newWorkspace},
		{Label: "run", Value: filepath.Base(os.Args[0]) + " workspace run " + newWorkspace + " -- /bin/sh"},
	})
	return nil
}

func cmdWorkspaceImport(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, workspaceImportUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	cloneAtSource := false
	importArgs := []string{"import"}
	for _, arg := range args[2:] {
		if arg == "--clone-at-source" || arg == "--mount-at-source" {
			cloneAtSource = true
			continue
		}
		importArgs = append(importArgs, arg)
	}
	if len(importArgs) < 3 {
		return fmt.Errorf("%s", workspaceImportUsageText(filepath.Base(os.Args[0])))
	}

	sourceDir := importArgs[len(importArgs)-1]
	if err := cmdImport(importArgs); err != nil {
		return err
	}
	if !cloneAtSource {
		return nil
	}

	workspace := importArgs[len(importArgs)-2]
	return cloneWorkspaceAtSource(workspace, sourceDir)
}

func cloneWorkspaceAtSource(workspace, sourceDir string) error {
	cfg, _, _, err := openAFSStore(context.Background())
	if err != nil {
		return err
	}

	targetDir, err := expandPath(sourceDir)
	if err != nil {
		return err
	}
	info, err := os.Stat(targetDir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", targetDir)
	}

	backupDir := targetDir + ".pre-afs"
	if _, err := os.Stat(backupDir); err == nil {
		return fmt.Errorf("backup path %s already exists; move it aside before mounting at source", backupDir)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if err := os.Rename(targetDir, backupDir); err != nil {
		return err
	}
	if err := materializeWorkspaceToPath(context.Background(), cfg, workspace, targetDir); err != nil {
		_ = os.Rename(backupDir, targetDir)
		return err
	}

	printBox(clr(ansiBGreen, "●")+" "+clr(ansiBold, "workspace cloned at source"), []boxRow{
		{Label: "workspace", Value: workspace},
		{Label: "path", Value: targetDir},
		{Label: "backup", Value: backupDir},
	})
	return nil
}

func materializeWorkspaceToPath(ctx context.Context, cfg config, workspace, targetDir string) error {
	_, store, closeStore, err := openAFSStore(ctx)
	if err != nil {
		return err
	}
	defer closeStore()

	workspaceMeta, err := store.getWorkspaceMeta(ctx, workspace)
	if err != nil {
		return err
	}
	m, err := store.getManifest(ctx, workspace, workspaceMeta.HeadSavepoint)
	if err != nil {
		return err
	}
	return materializeManifestToPath(ctx, store, workspace, m, targetDir)
}

func materializeManifestToPath(ctx context.Context, store *afsStore, workspace string, m manifest, targetDir string) error {
	targetDir, err := expandPath(targetDir)
	if err != nil {
		return err
	}
	_, err = materializeManifestToDirectory(targetDir, m, func(blobID string) ([]byte, error) {
		return store.getBlob(ctx, workspace, blobID)
	}, manifestMaterializeOptions{})
	return err
}

func cmdCheckpointList(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, checkpointListUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	if len(args) != 3 {
		return fmt.Errorf("%s", checkpointListUsageText(filepath.Base(os.Args[0])))
	}
	workspace := args[2]
	if err := validateAFSName("workspace", workspace); err != nil {
		return err
	}

	cfg, store, closeStore, err := openAFSStore(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	service := controlPlaneServiceFromStore(cfg, store)
	checkpoints, err := service.ListCheckpoints(context.Background(), workspace, 100)
	if err != nil {
		return err
	}
	rows := make([]boxRow, 0, len(checkpoints))
	for _, meta := range checkpoints {
		rows = append(rows, boxRow{
			Label: meta.Name,
			Value: fmt.Sprintf("%s · %s", meta.CreatedAt, formatBytes(meta.TotalBytes)),
		})
	}
	if len(rows) == 0 {
		rows = append(rows, boxRow{Value: clr(ansiDim, "No checkpoints found")})
	}
	printBox(clr(ansiBold, "checkpoints")+" "+workspace, rows)
	return nil
}

func cmdCheckpointCreate(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, checkpointCreateUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	if len(args) != 3 && len(args) != 4 {
		return fmt.Errorf("%s", checkpointCreateUsageText(filepath.Base(os.Args[0])))
	}
	workspace := args[2]
	checkpointID := generatedSavepointName()
	if len(args) == 4 {
		checkpointID = args[3]
	}
	if err := validateAFSName("workspace", workspace); err != nil {
		return err
	}
	if err := validateAFSName("checkpoint", checkpointID); err != nil {
		return err
	}

	cfg, store, closeStore, err := openAFSStore(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	saved, err := saveAFSWorkspaceOrLiveRoot(context.Background(), cfg, store, workspace, checkpointID, false)
	if err != nil {
		return err
	}
	if !saved {
		fmt.Println("No changes to checkpoint")
		return nil
	}

	printBox(clr(ansiBGreen, "●")+" "+clr(ansiBold, "checkpoint created"), []boxRow{
		{Label: "workspace", Value: workspace},
		{Label: "checkpoint", Value: checkpointID},
	})
	return nil
}

func cmdCheckpointRestore(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, checkpointRestoreUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	if len(args) != 4 {
		return fmt.Errorf("%s", checkpointRestoreUsageText(filepath.Base(os.Args[0])))
	}
	workspace := args[2]
	checkpointID := args[3]
	if err := validateAFSName("workspace", workspace); err != nil {
		return err
	}
	if err := validateAFSName("checkpoint", checkpointID); err != nil {
		return err
	}

	return restoreCheckpoint(context.Background(), workspace, checkpointID)
}

func restoreCheckpoint(ctx context.Context, workspace, checkpointID string) error {
	cfg, store, closeStore, err := openAFSStore(ctx)
	if err != nil {
		return err
	}
	defer closeStore()

	service := controlPlaneServiceFromStore(cfg, store)
	resetResult, err := resetAFSWorkspaceHead(ctx, cfg, store, service, workspace, checkpointID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("checkpoint %q does not exist", checkpointID)
		}
		if err == redis.TxFailedErr || errors.Is(err, errAFSWorkspaceConflict) {
			return fmt.Errorf("checkpoint restore conflict while restoring %q", checkpointID)
		}
		return err
	}

	rows := []boxRow{
		{Label: "workspace", Value: workspace},
		{Label: "checkpoint", Value: checkpointID},
		{Label: "path", Value: resetResult.treePath},
	}
	if resetResult.archivePath != "" {
		rows = append(rows, boxRow{Label: "archive", Value: resetResult.archivePath})
	}
	printBox(clr(ansiBGreen, "●")+" "+clr(ansiBold, "checkpoint restored"), rows)
	return nil
}

func workspaceUsageText(bin string) string {
	return fmt.Sprintf(`Usage:
  %s workspace <subcommand>

Subcommands:
  create <workspace>                           Create an empty workspace
  list                                         List workspaces in Redis
  current                                      Show the current workspace
  use <workspace>                              Set the current workspace
  run [workspace] [--readonly] -- <command...> Materialize and run in a workspace cwd
  clone <workspace> <directory>               Clone a workspace into a local directory
  fork <source-workspace> <new-workspace>     Fork a workspace at its current head
  delete <workspace>...                       Delete workspaces and local materialized state
  import [--force] [--clone-at-source] <workspace> <directory>
                                               Import a local directory into a workspace

Examples:
  %s workspace create demo
  %s workspace use demo
  %s workspace run demo -- /bin/sh
  %s workspace clone demo ~/src/demo
  %s workspace fork demo demo-copy

Run '%s workspace <subcommand> --help' for details.
`, bin, bin, bin, bin, bin, bin, bin)
}

func workspaceCreateUsageText(bin string) string {
	return fmt.Sprintf(`Usage:
  %s workspace create <workspace>

Create an empty workspace with an initial checkpoint named "initial".
`, bin)
}

func workspaceListUsageText(bin string) string {
	return fmt.Sprintf(`Usage:
  %s workspace list

List workspaces stored in Redis, along with checkpoint counts and creation time.
`, bin)
}

func workspaceRunUsageText(bin string) string {
	return fmt.Sprintf(`Usage:
  %s workspace run [workspace] [--readonly] -- <command...>

Materialize the workspace locally, run a command with that workspace as the cwd,
and refresh the local dirty state when the command exits.

Notes:
  If <workspace> is omitted, AFS uses the current workspace selected with
  '%s workspace use <workspace>'.
  Create a checkpoint explicitly with '%s checkpoint create <workspace> [checkpoint]'
  when you want to persist the current working copy.

Examples:
  %s workspace run demo -- /bin/sh
  %s workspace run --readonly demo -- make test
`, bin, bin, bin, bin, bin)
}

func workspaceCurrentUsageText(bin string) string {
	return fmt.Sprintf(`Usage:
  %s workspace current

Show the current workspace selection AFS will use when a workspace argument is omitted.
`, bin)
}

func workspaceUseUsageText(bin string) string {
	return fmt.Sprintf(`Usage:
  %s workspace use <workspace>

Set the current workspace AFS will use when a workspace argument is omitted.
`, bin)
}

func workspaceCloneUsageText(bin string) string {
	return fmt.Sprintf(`Usage:
  %s workspace clone <workspace> <directory>

Clone a workspace into a local directory at its current saved head.
The destination must not already contain files.
`, bin)
}

func workspaceForkUsageText(bin string) string {
	return fmt.Sprintf(`Usage:
  %s workspace fork <source-workspace> <new-workspace>

Create a new workspace from the source workspace's current saved head.
`, bin)
}

func workspaceDeleteUsageText(bin string) string {
	return fmt.Sprintf(`Usage:
  %s workspace delete <workspace>...

Delete one or more workspaces from Redis and remove their local materialized state.
`, bin)
}

func workspaceImportUsageText(bin string) string {
	return fmt.Sprintf(`Usage:
  %s workspace import [--force] [--clone-at-source] <workspace> <directory>

Import a local directory into a workspace.

Options:
  --force            Replace an existing workspace
  --clone-at-source  Replace the source directory with a materialized workspace copy
                     and keep the original directory as <directory>.pre-afs
`, bin)
}

func checkpointUsageText(bin string) string {
	return fmt.Sprintf(`Usage:
  %s checkpoint <subcommand>

Subcommands:
  list <workspace>                     List checkpoints for a workspace
  create <workspace> [checkpoint]     Create a checkpoint from the local working copy
  restore <workspace> <checkpoint>    Restore a workspace to a checkpoint

Examples:
  %s checkpoint list demo
  %s checkpoint create demo before-refactor
  %s checkpoint restore demo initial

Run '%s checkpoint <subcommand> --help' for details.
`, bin, bin, bin, bin, bin)
}

func checkpointListUsageText(bin string) string {
	return fmt.Sprintf(`Usage:
  %s checkpoint list <workspace>

List checkpoints for a workspace, newest first.
`, bin)
}

func checkpointCreateUsageText(bin string) string {
	return fmt.Sprintf(`Usage:
  %s checkpoint create <workspace> [checkpoint]

Create a checkpoint from the workspace's current local working copy.
If [checkpoint] is omitted, AFS generates a timestamped name.
`, bin)
}

func checkpointRestoreUsageText(bin string) string {
	return fmt.Sprintf(`Usage:
  %s checkpoint restore <workspace> <checkpoint>

Restore the workspace working copy to the selected checkpoint.
`, bin)
}
