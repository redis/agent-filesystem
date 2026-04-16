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

	"github.com/redis/agent-filesystem/internal/controlplane"
	"github.com/redis/go-redis/v9"
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
	case "create", "cr":
		return cmdWorkspaceCreate(args)
	case "list", "l", "ls":
		return cmdWorkspaceList(args)
	case "current", "cu":
		return cmdWorkspaceCurrent(args)
	case "use", "u":
		return cmdWorkspaceUse(args)
	case "clone", "cl":
		return cmdWorkspaceClone(args)
	case "fork", "f":
		return cmdWorkspaceFork(args)
	case "delete", "d", "rm":
		return cmdWorkspaceDelete(args)
	case "import", "i":
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
	case "create", "cr", "save":
		return cmdCheckpointCreate(args)
	case "list", "l", "ls":
		return cmdCheckpointList(args)
	case "restore", "r":
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

	cfg, service, closeStore, err := openAFSControlPlane(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	ctx := context.Background()
	_, err = service.CreateWorkspace(ctx, controlplane.CreateWorkspaceRequest{
		Name: workspace,
		Source: controlplane.SourceRef{
			Kind: controlplane.SourceBlank,
		},
	})
	if err != nil {
		return err
	}

	next := filepath.Base(os.Args[0]) + " up " + workspace + " <folder>"
	if productMode, _ := effectiveProductMode(cfg); productMode != productModeDirect {
		next = filepath.Base(os.Args[0]) + " workspace use " + workspace
	}

	printBox(markerSuccess+" "+clr(ansiBold, "workspace created"), []boxRow{
		{Label: "workspace", Value: workspace},
		{Label: "checkpoint", Value: afsInitialCheckpointName},
		{Label: "database", Value: configRemoteLabel(cfg)},
		{Label: "next", Value: next},
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

	cfg, service, closeStore, err := openAFSControlPlane(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	ctx := context.Background()
	workspaces, err := service.ListWorkspaceSummaries(ctx)
	if err != nil {
		return err
	}

	nameWidth := 0
	for _, meta := range workspaces.Items {
		if w := runeWidth(workspaceListName(meta.Name, workspaceListSelected(cfg, meta))); w > nameWidth {
			nameWidth = w
		}
	}

	rows := make([]boxRow, 0, len(workspaces.Items))
	for _, meta := range workspaces.Items {
		value := workspaceListLine(meta.Name, checkpointCountLabel(meta.CheckpointCount), formatDisplayTimestamp(meta.UpdatedAt), workspaceListSelected(cfg, meta), nameWidth)
		rows = append(rows, boxRow{
			Value: value,
		})
	}
	if len(rows) == 0 {
		rows = append(rows, boxRow{Value: clr(ansiDim, "No workspaces found")})
	}
	printBox(clr(ansiBold, "workspaces on "+configRemoteLabel(cfg)), rows)
	return nil
}

func workspaceListName(name string, selected bool) string {
	prefix := "  "
	if selected {
		prefix = clr(ansiBGreen, "✓") + " "
	}
	return prefix + clr(ansiBold+ansiWhite, name)
}

func workspaceListLine(name, checkpointLabel, updated string, selected bool, nameWidth int) string {
	namePart := padVisibleText(workspaceListName(name, selected), nameWidth)
	details := clr(ansiDim, checkpointLabel+" · updated "+updated)
	line := namePart + "   " + details
	if selected {
		line += " " + clr(ansiBGreen, "<active>")
	}
	return line
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

	printBox(clr(ansiBold, "current workspace on "+configRemoteLabel(cfg)), []boxRow{
		{Label: "workspace", Value: currentWorkspaceLabel(selectedWorkspaceName(cfg))},
		{Label: "config", Value: configPathLabel()},
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

	cfg, service, closeStore, err := openAFSControlPlane(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	selection, err := resolveWorkspaceSelectionFromControlPlane(context.Background(), cfg, service, workspace)
	if err != nil {
		return err
	}
	if active, err := activeMountedWorkspaceState(cfg); err != nil {
		return err
	} else if active.workspace != "" && active.workspace != selection.Name {
		if active.mountpoint != "" {
			return fmt.Errorf("active workspace %q mounted at %s\nRun '%s down' before selecting %q", active.workspace, active.mountpoint, filepath.Base(os.Args[0]), selection.Name)
		}
		return fmt.Errorf("active workspace %q mounted\nRun '%s down' before selecting %q", active.workspace, filepath.Base(os.Args[0]), selection.Name)
	}

	if err := applyWorkspaceSelection(&cfg, selection); err != nil {
		return err
	}
	if err := prepareConfigForSave(&cfg); err != nil {
		return err
	}
	if err := saveConfig(cfg); err != nil {
		return err
	}

	printBox(markerSuccess+" "+clr(ansiBold, "workspace selected"), []boxRow{
		{Label: "workspace", Value: selection.Name},
		{Label: "database", Value: configRemoteLabel(cfg)},
		{Label: "config", Value: configPathLabel()},
	})
	return nil
}

type mountedWorkspaceState struct {
	workspace  string
	mountpoint string
}

func activeMountedWorkspaceState(cfg config) (mountedWorkspaceState, error) {
	st, err := loadState()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return mountedWorkspaceState{}, nil
		}
		return mountedWorkspaceState{}, err
	}

	backendName := strings.TrimSpace(st.MountBackend)
	if backendName == "" {
		backendName = mountBackendNone
	}
	workspace := strings.TrimSpace(st.CurrentWorkspace)
	if backendName == mountBackendNone || workspace == "" || !runtimeStateMatchesConfig(cfg, st) {
		return mountedWorkspaceState{}, nil
	}
	if st.MountPID > 0 && processAlive(st.MountPID) {
		return mountedWorkspaceState{workspace: workspace, mountpoint: strings.TrimSpace(st.LocalPath)}, nil
	}

	backend, _, err := backendForState(st)
	if err != nil {
		return mountedWorkspaceState{}, err
	}
	if strings.TrimSpace(st.LocalPath) != "" && backend.IsMounted(st.LocalPath) {
		return mountedWorkspaceState{workspace: workspace, mountpoint: strings.TrimSpace(st.LocalPath)}, nil
	}
	return mountedWorkspaceState{}, nil
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

	cfg, service, closeStore, err := openAFSControlPlane(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	ctx := context.Background()
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
	rows = append(rows, boxRow{Label: "database", Value: configRemoteLabel(cfg)})
	rows = append(rows, boxRow{})
	for _, name := range deleted {
		rows = append(rows, boxRow{Value: name})
	}
	printBox(markerSuccess+" "+clr(ansiBold, "workspaces deleted"), rows)
	return nil
}

func cmdWorkspaceClone(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, workspaceCloneUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	if len(args) != 3 && len(args) != 4 {
		return fmt.Errorf("%s", workspaceCloneUsageText(filepath.Base(os.Args[0])))
	}

	cfg, store, closeStore, err := openAFSStore(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	workspace := ""
	targetDir := ""
	if len(args) == 4 {
		workspace = args[2]
		targetDir = args[3]
	} else {
		targetDir = args[2]
	}
	workspace, err = resolveWorkspaceName(context.Background(), cfg, store, workspace)
	if err != nil {
		return err
	}
	if err := validateAFSName("workspace", workspace); err != nil {
		return err
	}

	clonedPath, err := prepareWorkspaceCloneTarget(targetDir)
	if err != nil {
		return err
	}

	if err := materializeWorkspaceToPath(context.Background(), cfg, workspace, clonedPath); err != nil {
		return err
	}

	printBox(markerSuccess+" "+clr(ansiBold, "workspace cloned"), []boxRow{
		{Label: "workspace", Value: workspace},
		{Label: "database", Value: configRemoteLabel(cfg)},
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
	if len(args) != 3 && len(args) != 4 {
		return fmt.Errorf("%s", workspaceForkUsageText(filepath.Base(os.Args[0])))
	}

	cfg, service, closeStore, err := openAFSControlPlane(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	sourceWorkspace := ""
	newWorkspace := ""
	if len(args) == 4 {
		sourceWorkspace = args[2]
		newWorkspace = args[3]
	} else {
		newWorkspace = args[2]
	}
	sourceSelection, err := resolveWorkspaceSelectionFromControlPlane(context.Background(), cfg, service, sourceWorkspace)
	if err != nil {
		return err
	}
	if err := validateAFSName("workspace", newWorkspace); err != nil {
		return err
	}

	ctx := context.Background()
	if err := service.ForkWorkspace(ctx, sourceSelection.ID, newWorkspace); err != nil {
		return err
	}

	printBox(markerSuccess+" "+clr(ansiBold, "workspace forked"), []boxRow{
		{Label: "workspace", Value: newWorkspace},
		{Label: "source", Value: sourceSelection.Name},
		{Label: "database", Value: configRemoteLabel(cfg)},
		{Label: "next", Value: filepath.Base(os.Args[0]) + " workspace use " + newWorkspace},
	})
	return nil
}

func cmdWorkspaceImport(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, workspaceImportUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	mountAtSource := false
	importArgs := []string{"import"}
	for _, arg := range args[2:] {
		if arg == "--clone-at-source" {
			return fmt.Errorf(`unknown flag "--clone-at-source"; use "--mount-at-source" instead`)
		}
		if arg == "--mount-at-source" {
			mountAtSource = true
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
	if !mountAtSource {
		return nil
	}

	workspace := importArgs[len(importArgs)-2]
	return mountWorkspaceAtSource(workspace, sourceDir)
}

func mountWorkspaceAtSource(workspace, sourceDir string) (err error) {
	ctx := context.Background()

	cfg, store, closeStore, err := openAFSStore(ctx)
	if err != nil {
		return err
	}
	defer closeStore()

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
	if entry, mounted := mountTableEntry(targetDir); mounted {
		return fmt.Errorf("mountpoint %s is already mounted by another filesystem\n  mount entry: %s", targetDir, entry)
	}

	if _, err := loadStateForMountAtSource(); err != nil {
		return err
	}

	mountCfg, err := prepareMountedConfig(cfg, workspace, targetDir)
	if err != nil {
		return err
	}
	backend, backendName, err := backendForConfig(mountCfg)
	if err != nil {
		return err
	}

	backupDir := targetDir + ".pre-afs"
	if _, err := os.Stat(backupDir); err == nil {
		return fmt.Errorf("backup path %s already exists; move it aside before mounting at source", backupDir)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if err := store.checkImportLock(ctx, workspace); err != nil {
		return fmt.Errorf("cannot mount workspace %q at source: %w", workspace, err)
	}

	prepareStep := startStep("Opening live workspace")
	mountKey, mountedHeadSavepoint, initialized, err := seedWorkspaceMountKey(ctx, store, workspace)
	if err != nil {
		prepareStep.fail(err.Error())
		return err
	}
	if initialized {
		prepareStep.succeed(workspace + " (initialized)")
	} else {
		prepareStep.succeed(workspace)
	}

	rollbackArchive := false
	cleanupMountpoint := false
	var started mountStartResult
	defer func() {
		if err == nil {
			return
		}
		if started.PID > 0 && processAlive(started.PID) {
			_ = terminatePID(started.PID, 2*time.Second)
		}
		if backend != nil {
			_ = backend.Unmount(targetDir)
		}
		if cleanupMountpoint {
			_ = os.RemoveAll(targetDir)
			_ = os.Remove(targetDir)
		}
		if rollbackArchive {
			_ = os.Rename(backupDir, targetDir)
		}
	}()

	archiveStep := startStep("Archiving source directory")
	if err := os.Rename(targetDir, backupDir); err != nil {
		archiveStep.fail(err.Error())
		return err
	}
	rollbackArchive = true
	cleanupMountpoint = true
	archiveStep.succeed(backupDir)

	mountStep := startStep("Mounting workspace at source")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		mountStep.fail(err.Error())
		return err
	}
	mountCfg, err = prepareRuntimeMountConfig(mountCfg, backendName)
	if err != nil {
		mountStep.fail(err.Error())
		return err
	}
	started, err = backend.Start(mountCfg)
	if err != nil {
		mountStep.fail(err.Error())
		return err
	}
	if err := backend.WaitForMount(mountCfg, started, 8*time.Second); err != nil {
		mountStep.fail("timeout")
		return fmt.Errorf("mount did not become ready: %w", err)
	}
	mountStep.succeed(targetDir)

	st := state{
		StartedAt:            time.Now().UTC(),
		RedisAddr:            mountCfg.RedisAddr,
		RedisDB:              mountCfg.RedisDB,
		CurrentWorkspace:     workspace,
		MountedHeadSavepoint: mountedHeadSavepoint,
		MountPID:             started.PID,
		MountBackend:         backendName,
		ReadOnly:             mountCfg.ReadOnly,
		MountEndpoint:        started.Endpoint,
		LocalPath:            mountCfg.LocalPath,
		RedisKey:             mountKey,
		MountLog:             mountCfg.MountLog,
		MountBin:             mountCfg.MountBin,
		ArchivePath:          backupDir,
	}
	if err := saveState(st); err != nil {
		return err
	}

	printBox(markerSuccess+" "+clr(ansiBold, "workspace mounted at source"), []boxRow{
		{Label: "workspace", Value: workspace},
		{Label: "database", Value: configRemoteLabel(cfg)},
		{Label: "path", Value: targetDir},
		{Label: "backup", Value: backupDir},
		{Label: "stop", Value: filepath.Base(os.Args[0]) + " down"},
	})
	return nil
}

func loadStateForMountAtSource() (state, error) {
	st, err := loadState()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return state{}, nil
		}
		return state{}, err
	}

	backendName := strings.TrimSpace(st.MountBackend)
	if backendName == "" {
		backendName = mountBackendNone
	}
	if backendName != mountBackendNone || strings.TrimSpace(st.ArchivePath) != "" {
		return state{}, fmt.Errorf("AFS already has an active mounted filesystem state; run '%s down' first", filepath.Base(os.Args[0]))
	}
	return st, nil
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
	if len(args) != 2 && len(args) != 3 {
		return fmt.Errorf("%s", checkpointListUsageText(filepath.Base(os.Args[0])))
	}

	cfg, service, closeStore, err := openAFSControlPlane(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	workspace := ""
	if len(args) == 3 {
		workspace = args[2]
	}
	selection, err := resolveWorkspaceSelectionFromControlPlane(context.Background(), cfg, service, workspace)
	if err != nil {
		return err
	}

	checkpoints, err := service.ListCheckpoints(context.Background(), selection.ID, 100)
	if err != nil {
		return err
	}
	rows := make([]boxRow, 0, len(checkpoints))
	for _, meta := range checkpoints {
		value := fmt.Sprintf("%s · %s", formatDisplayTimestamp(meta.CreatedAt), formatBytes(meta.TotalBytes))
		if meta.IsHead {
			value = "head · " + value
		}
		rows = append(rows, boxRow{
			Label: meta.Name,
			Value: value,
		})
	}
	if len(rows) == 0 {
		rows = append(rows, boxRow{Value: clr(ansiDim, "No checkpoints found")})
	}
	printBox(clr(ansiBold, "checkpoints in workspace: "+selection.Name), rows)
	return nil
}

func cmdCheckpointCreate(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, checkpointCreateUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	if len(args) != 2 && len(args) != 3 && len(args) != 4 {
		return fmt.Errorf("%s", checkpointCreateUsageText(filepath.Base(os.Args[0])))
	}

	cfg, service, closeFn, err := openAFSControlPlane(context.Background())
	if err != nil {
		return err
	}
	defer closeFn()

	workspace := ""
	checkpointID := generatedSavepointName()
	switch len(args) {
	case 4:
		workspace = args[2]
		checkpointID = args[3]
	case 3:
		if hasCurrentWorkspaceSelection(cfg) {
			checkpointID = args[2]
		} else {
			workspace = args[2]
		}
	}
	selection, err := resolveWorkspaceSelectionFromControlPlane(context.Background(), cfg, service, workspace)
	if err != nil {
		return err
	}
	if err := validateAFSName("checkpoint", checkpointID); err != nil {
		return err
	}

	step := startStep("Saving checkpoint")
	saved, err := service.SaveCheckpointFromLive(context.Background(), selection.Name, checkpointID)
	if err != nil {
		step.fail(err.Error())
		return err
	}
	if !saved {
		step.succeed("no changes")
		printBox(clr(ansiBold, "checkpoint unchanged"), []boxRow{
			{Label: "workspace", Value: selection.Name},
			{Label: "database", Value: configRemoteLabel(cfg)},
			{Label: "result", Value: "no changes"},
		})
		return nil
	}
	step.succeed(checkpointID)

	printBox(markerSuccess+" "+clr(ansiBold, "checkpoint created"), []boxRow{
		{Label: "workspace", Value: selection.Name},
		{Label: "checkpoint", Value: checkpointID},
		{Label: "database", Value: configRemoteLabel(cfg)},
	})
	return nil
}

func cmdCheckpointRestore(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, checkpointRestoreUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	if len(args) != 3 && len(args) != 4 {
		return fmt.Errorf("%s", checkpointRestoreUsageText(filepath.Base(os.Args[0])))
	}
	cfg, service, closeFn, err := openAFSControlPlane(context.Background())
	if err != nil {
		return err
	}
	defer closeFn()

	workspace := ""
	checkpointID := ""
	if len(args) == 4 {
		workspace = args[2]
		checkpointID = args[3]
	} else {
		checkpointID = args[2]
	}
	selection, err := resolveWorkspaceSelectionFromControlPlane(context.Background(), cfg, service, workspace)
	if err != nil {
		return err
	}
	if err := validateAFSName("checkpoint", checkpointID); err != nil {
		return err
	}

	return restoreCheckpoint(context.Background(), selection.Name, checkpointID)
}

func restoreCheckpoint(ctx context.Context, workspace, checkpointID string) error {
	cfg, service, closeStore, err := openAFSControlPlane(ctx)
	if err != nil {
		return err
	}
	defer closeStore()

	_, err = resetAFSWorkspaceHead(ctx, service, workspace, checkpointID)
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
		{Label: "database", Value: configRemoteLabel(cfg)},
	}
	printBox(markerSuccess+" "+clr(ansiBold, "checkpoint restored"), rows)
	return nil
}

func checkpointCountLabel(count int) string {
	if count == 1 {
		return "1 checkpoint"
	}
	return fmt.Sprintf("%d checkpoints", count)
}

func hasCurrentWorkspaceSelection(cfg config) bool {
	return selectedWorkspaceReference(cfg) != ""
}

func workspaceListSelected(cfg config, summary workspaceSummary) bool {
	selected := selectedWorkspaceReference(cfg)
	if selected == "" {
		return false
	}
	return selected == summary.ID || selected == summary.Name
}

func workspaceUsageText(bin string) string {
	return fmt.Sprintf(`Usage:
  %s workspace <subcommand>

Subcommands:
  create <workspace>                           Create an empty workspace
  list                                         List workspaces in Redis
  current                                      Show the current workspace
  use <workspace>                              Set the current workspace
  clone [workspace] <directory>                Clone a workspace into a local directory
  fork [source-workspace] <new-workspace>      Fork a workspace at its current head
  delete <workspace>...                       Delete workspaces and local materialized state
  import [--force] [--mount-at-source] <workspace> <directory>
                                               Import a local directory into a workspace

Examples:
  %s workspace create demo
  %s workspace use demo
  %s workspace clone demo ~/src/demo
  %s workspace fork demo demo-copy

Run '%s workspace <subcommand> --help' for details.
`, bin, bin, bin, bin, bin, bin)
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
  %s workspace clone [workspace] <directory>

Clone a workspace into a local directory at its current saved head.
The destination must not already contain files.

If [workspace] is omitted, AFS uses the current workspace.
`, bin)
}

func workspaceForkUsageText(bin string) string {
	return fmt.Sprintf(`Usage:
  %s workspace fork [source-workspace] <new-workspace>

Create a new workspace from the source workspace's current saved head.

If [source-workspace] is omitted, AFS uses the current workspace.
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
  %s workspace import [--force] [--mount-at-source] <workspace> <directory>

Import a local directory into a workspace.

Options:
  --force            Replace an existing workspace
  --mount-at-source  Archive the source directory to <directory>.pre-afs and
                     mount the imported workspace at the original path
`, bin)
}

func checkpointUsageText(bin string) string {
	return fmt.Sprintf(`Usage:
  %s checkpoint <subcommand>

Subcommands:
  list [workspace]                     List checkpoints for a workspace
  create [workspace] [checkpoint]     Create a checkpoint from the current workspace state
  restore [workspace] <checkpoint>    Restore a workspace to a checkpoint

Examples:
  %s checkpoint list demo
  %s checkpoint create demo before-refactor
  %s checkpoint restore demo initial

Run '%s checkpoint <subcommand> --help' for details.
`, bin, bin, bin, bin, bin)
}

func checkpointListUsageText(bin string) string {
	return fmt.Sprintf(`Usage:
  %s checkpoint list [workspace]

List checkpoints for a workspace, newest first.
`, bin)
}

func checkpointCreateUsageText(bin string) string {
	return fmt.Sprintf(`Usage:
  %s checkpoint create [workspace] [checkpoint]

Create a checkpoint from the workspace's current live state.
If [checkpoint] is omitted, AFS generates a timestamped name.
`, bin)
}

func checkpointRestoreUsageText(bin string) string {
	return fmt.Sprintf(`Usage:
  %s checkpoint restore [workspace] <checkpoint>

Restore the workspace live state to the selected checkpoint.
`, bin)
}
