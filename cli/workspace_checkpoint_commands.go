package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	rafPrimaryStateName      = "main"
	rafInitialCheckpointName = "initial"
)

func primaryStateName(cfg config) string {
	if strings.TrimSpace(cfg.DefaultSession) != "" {
		return cfg.DefaultSession
	}
	return rafPrimaryStateName
}

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
	if err := validateRAFName("workspace", workspace); err != nil {
		return err
	}

	cfg, store, closeStore, err := openRAFStore(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	ctx := context.Background()
	exists, err := store.workspaceExists(ctx, workspace)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("workspace %q already exists", workspace)
	}

	if err := createEmptyWorkspace(ctx, cfg, store, workspace); err != nil {
		return err
	}

	printBox(clr(ansiBGreen, "●")+" "+clr(ansiBold, "workspace created"), []boxRow{
		{Label: "workspace", Value: workspace},
		{Label: "checkpoint", Value: rafInitialCheckpointName},
		{Label: "run", Value: filepath.Base(os.Args[0]) + " workspace run " + workspace + " -- /bin/sh"},
	})
	return nil
}

func createEmptyWorkspace(ctx context.Context, cfg config, store *rafStore, workspace string) error {
	now := time.Now().UTC()
	stateName := primaryStateName(cfg)

	rootManifest := manifest{
		Version:   rafFormatVersion,
		Workspace: workspace,
		Savepoint: rafInitialCheckpointName,
		Entries: map[string]manifestEntry{
			"/": {
				Type:    "dir",
				Mode:    0o755,
				MtimeMs: now.UnixMilli(),
				Size:    0,
			},
		},
	}
	manifestHash, err := hashManifest(rootManifest)
	if err != nil {
		return err
	}

	workspaceMeta := workspaceMeta{
		Version:          rafFormatVersion,
		Name:             workspace,
		CreatedAt:        now,
		DefaultSession:   stateName,
		DefaultSavepoint: rafInitialCheckpointName,
	}
	stateMeta := sessionMeta{
		Version:       rafFormatVersion,
		Workspace:     workspace,
		Name:          stateName,
		HeadSavepoint: rafInitialCheckpointName,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	checkpointMeta := savepointMeta{
		Version:      rafFormatVersion,
		ID:           rafInitialCheckpointName,
		Name:         rafInitialCheckpointName,
		Workspace:    workspace,
		Session:      stateName,
		ManifestHash: manifestHash,
		CreatedAt:    now,
		FileCount:    0,
		DirCount:     0,
		TotalBytes:   0,
	}

	if err := store.putWorkspaceMeta(ctx, workspaceMeta); err != nil {
		return err
	}
	if err := store.putSavepoint(ctx, checkpointMeta, rootManifest); err != nil {
		return err
	}
	if err := store.putSessionMeta(ctx, stateMeta); err != nil {
		return err
	}
	return store.audit(ctx, workspace, stateName, "workspace_create", map[string]any{
		"checkpoint": rafInitialCheckpointName,
	})
}

func cmdWorkspaceList(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, workspaceListUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	if len(args) != 2 {
		return fmt.Errorf("%s", workspaceListUsageText(filepath.Base(os.Args[0])))
	}
	cfg, store, closeStore, err := openRAFStore(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	ctx := context.Background()
	workspaces, err := store.listWorkspaces(ctx)
	if err != nil {
		return err
	}

	rows := make([]boxRow, 0, len(workspaces))
	for _, meta := range workspaces {
		checkpoints, err := store.listSavepoints(ctx, meta.Name, primaryStateName(cfg), 0)
		if err != nil {
			return err
		}
		rows = append(rows, boxRow{
			Label: meta.Name,
			Value: fmt.Sprintf("%d checkpoints · created %s", len(checkpoints), meta.CreatedAt.Local().Format(time.RFC3339)),
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
	if err := validateRAFName("workspace", workspace); err != nil {
		return err
	}

	cfg, store, closeStore, err := openRAFStore(context.Background())
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
		{Label: "config", Value: clr(ansiDim, configPath())},
	})
	return nil
}

func cmdWorkspaceRun(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, workspaceRunUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	parsed, childArgs, err := parseRAFCommandInvocation(args[2:])
	if err != nil {
		return err
	}
	if parsed.session != "" {
		return fmt.Errorf("workspace run does not accept --session")
	}
	if len(parsed.positionals) > 1 {
		return fmt.Errorf("%s", workspaceRunUsageText(filepath.Base(os.Args[0])))
	}

	cfg, store, closeStore, err := openRAFStore(context.Background())
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
	if err := validateRAFName("workspace", workspace); err != nil {
		return err
	}

	stateName := primaryStateName(cfg)
	if err := validateRAFName("session", stateName); err != nil {
		return err
	}
	if err := runRAFCommand(context.Background(), cfg, store, workspace, stateName, childArgs, parsed.readonly); err != nil {
		if errors.Is(err, errRAFSessionConflict) {
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

	cfg, store, closeStore, err := openRAFStore(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	ctx := context.Background()
	deleted := make([]string, 0, len(names))
	for _, name := range names {
		if err := validateRAFName("workspace", name); err != nil {
			return err
		}
		exists, err := store.workspaceExists(ctx, name)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("workspace %q does not exist", name)
		}

		step := startStep("Deleting workspace " + name)
		if err := store.deleteWorkspace(ctx, name); err != nil {
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
	if err := validateRAFName("workspace", workspace); err != nil {
		return err
	}

	cfg, _, _, err := openRAFStore(context.Background())
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
	if err := validateRAFName("workspace", sourceWorkspace); err != nil {
		return err
	}
	if err := validateRAFName("workspace", newWorkspace); err != nil {
		return err
	}

	cfg, store, closeStore, err := openRAFStore(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	ctx := context.Background()
	exists, err := store.workspaceExists(ctx, newWorkspace)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("workspace %q already exists", newWorkspace)
	}
	if err := forkWorkspace(ctx, cfg, store, sourceWorkspace, newWorkspace); err != nil {
		return err
	}

	printBox(clr(ansiBGreen, "●")+" "+clr(ansiBold, "workspace forked"), []boxRow{
		{Label: "from", Value: sourceWorkspace},
		{Label: "new", Value: newWorkspace},
		{Label: "run", Value: filepath.Base(os.Args[0]) + " workspace run " + newWorkspace + " -- /bin/sh"},
	})
	return nil
}

func forkWorkspace(ctx context.Context, cfg config, store *rafStore, sourceWorkspace, newWorkspace string) error {
	sourceMeta, err := store.getWorkspaceMeta(ctx, sourceWorkspace)
	if err != nil {
		return err
	}
	stateName := primaryStateName(cfg)
	sourceState, err := store.getSessionMeta(ctx, sourceWorkspace, sourceMeta.DefaultSession)
	if err != nil {
		return err
	}
	sourceManifest, err := store.getManifest(ctx, sourceWorkspace, sourceState.HeadSavepoint)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	newManifest := cloneManifest(sourceManifest)
	newManifest.Workspace = newWorkspace
	newManifest.Savepoint = rafInitialCheckpointName
	manifestHash, err := hashManifest(newManifest)
	if err != nil {
		return err
	}

	blobs := map[string][]byte{}
	for blobID := range manifestBlobRefs(sourceManifest) {
		data, err := store.getBlob(ctx, sourceWorkspace, blobID)
		if err != nil {
			return err
		}
		blobs[blobID] = data
	}
	if err := store.saveBlobs(ctx, newWorkspace, blobs); err != nil {
		return err
	}
	if err := store.addBlobRefs(ctx, newWorkspace, newManifest, now); err != nil {
		return err
	}

	workspaceMeta := workspaceMeta{
		Version:          rafFormatVersion,
		Name:             newWorkspace,
		CreatedAt:        now,
		DefaultSession:   stateName,
		DefaultSavepoint: rafInitialCheckpointName,
	}
	stateMeta := sessionMeta{
		Version:       rafFormatVersion,
		Workspace:     newWorkspace,
		Name:          stateName,
		HeadSavepoint: rafInitialCheckpointName,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	checkpointMeta := savepointMeta{
		Version:      rafFormatVersion,
		ID:           rafInitialCheckpointName,
		Name:         rafInitialCheckpointName,
		Workspace:    newWorkspace,
		Session:      stateName,
		ManifestHash: manifestHash,
		CreatedAt:    now,
		FileCount:    manifestImportStats(newManifest).Files,
		DirCount:     manifestImportStats(newManifest).Dirs,
		TotalBytes:   manifestImportStats(newManifest).Bytes,
	}

	if err := store.putWorkspaceMeta(ctx, workspaceMeta); err != nil {
		return err
	}
	if err := store.putSavepoint(ctx, checkpointMeta, newManifest); err != nil {
		return err
	}
	if err := store.putSessionMeta(ctx, stateMeta); err != nil {
		return err
	}
	return store.audit(ctx, newWorkspace, stateName, "workspace_fork", map[string]any{
		"source_workspace":  sourceWorkspace,
		"source_checkpoint": sourceState.HeadSavepoint,
	})
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
	cfg, _, _, err := openRAFStore(context.Background())
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

	backupDir := targetDir + ".pre-raf"
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
	_, store, closeStore, err := openRAFStore(ctx)
	if err != nil {
		return err
	}
	defer closeStore()

	stateName := primaryStateName(cfg)
	stateMeta, err := store.getSessionMeta(ctx, workspace, stateName)
	if err != nil {
		return err
	}
	m, err := store.getManifest(ctx, workspace, stateMeta.HeadSavepoint)
	if err != nil {
		return err
	}
	return materializeManifestToPath(ctx, store, workspace, m, targetDir)
}

func materializeManifestToPath(ctx context.Context, store *rafStore, workspace string, m manifest, targetDir string) error {
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
	if err := validateRAFName("workspace", workspace); err != nil {
		return err
	}

	cfg, store, closeStore, err := openRAFStore(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	checkpoints, err := store.listSavepoints(context.Background(), workspace, primaryStateName(cfg), 100)
	if err != nil {
		return err
	}
	rows := make([]boxRow, 0, len(checkpoints))
	for _, meta := range checkpoints {
		rows = append(rows, boxRow{
			Label: meta.Name,
			Value: fmt.Sprintf("%s · %s", meta.CreatedAt.Local().Format(time.RFC3339), formatBytes(meta.TotalBytes)),
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
	if err := validateRAFName("workspace", workspace); err != nil {
		return err
	}
	if err := validateRAFName("checkpoint", checkpointID); err != nil {
		return err
	}

	cfg, store, closeStore, err := openRAFStore(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	saved, err := saveRAFSession(context.Background(), cfg, store, workspace, primaryStateName(cfg), checkpointID, false)
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
	if err := validateRAFName("workspace", workspace); err != nil {
		return err
	}
	if err := validateRAFName("checkpoint", checkpointID); err != nil {
		return err
	}

	return restoreCheckpoint(context.Background(), workspace, checkpointID)
}

func restoreCheckpoint(ctx context.Context, workspace, checkpointID string) error {
	cfg, store, closeStore, err := openRAFStore(ctx)
	if err != nil {
		return err
	}
	defer closeStore()

	stateName := primaryStateName(cfg)
	exists, err := store.savepointExists(ctx, workspace, checkpointID)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("checkpoint %q does not exist", checkpointID)
	}

	resetResult, err := resetRAFSessionHead(ctx, cfg, store, workspace, stateName, checkpointID)
	if err != nil {
		if err == redis.TxFailedErr {
			return fmt.Errorf("checkpoint restore conflict while restoring %q", checkpointID)
		}
		return err
	}

	if err := store.audit(ctx, workspace, stateName, "checkpoint_restore", map[string]any{
		"checkpoint": checkpointID,
		"archive":    resetResult.archivePath,
	}); err != nil {
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
and save changes back to Redis unless --readonly is set.

Notes:
  If <workspace> is omitted, RAF uses the current workspace selected with
  '%s workspace use <workspace>'.
  workspace run does not accept --session.

Examples:
  %s workspace run demo -- /bin/sh
  %s workspace run --readonly demo -- make test
`, bin, bin, bin, bin)
}

func workspaceCurrentUsageText(bin string) string {
	return fmt.Sprintf(`Usage:
  %s workspace current

Show the current workspace selection RAF will use when a workspace argument is omitted.
`, bin)
}

func workspaceUseUsageText(bin string) string {
	return fmt.Sprintf(`Usage:
  %s workspace use <workspace>

Set the current workspace RAF will use when a workspace argument is omitted.
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
                     and keep the original directory as <directory>.pre-raf
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
If [checkpoint] is omitted, RAF generates a timestamped name.
`, bin)
}

func checkpointRestoreUsageText(bin string) string {
	return fmt.Sprintf(`Usage:
  %s checkpoint restore <workspace> <checkpoint>

Restore a workspace to a checkpoint and rematerialize the local working copy.
The previous local tree is archived if one exists.
`, bin)
}
