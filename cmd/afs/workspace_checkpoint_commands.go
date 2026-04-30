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
	case "create":
		return cmdWorkspaceCreate(args)
	case "list":
		return cmdWorkspaceList(args)
	case "current":
		return cmdWorkspaceCurrent(args)
	case "use":
		return cmdWorkspaceUse(args)
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
	case "diff":
		return cmdCheckpointDiff(args)
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
	parsed, err := parseWorkspaceCreateArgs(args[2:])
	if err != nil {
		return err
	}
	if len(parsed.positionals) != 1 {
		return fmt.Errorf("%s", workspaceCreateUsageText(filepath.Base(os.Args[0])))
	}

	workspace := parsed.positionals[0]
	if err := validateAFSName("workspace", workspace); err != nil {
		return err
	}

	cfg, err := loadAFSConfig()
	if err != nil {
		return err
	}
	productMode, err := effectiveProductMode(cfg)
	if err != nil {
		return err
	}
	if productMode == productModeSelfHosted {
		client, _, err := newHTTPControlPlaneClient(context.Background(), cfg)
		if err != nil {
			return err
		}
		database, err := resolveManagedDatabaseForWrite(context.Background(), cfg, client, parsed.database, "workspace create")
		if err != nil {
			return err
		}
		cfg.DatabaseID = database.ID
	} else if strings.TrimSpace(parsed.database) != "" {
		return fmt.Errorf("--database is only supported in control plane mode")
	}

	cfg, service, closeStore, err := openAFSControlPlaneForConfig(context.Background(), cfg)
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
	if productMode, _ := effectiveProductMode(cfg); productMode != productModeLocal {
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

type workspaceCreateArgs struct {
	positionals []string
	database    string
}

func parseWorkspaceCreateArgs(args []string) (workspaceCreateArgs, error) {
	var parsed workspaceCreateArgs
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "--database":
			if index+1 >= len(args) {
				return parsed, fmt.Errorf("missing value for %q", arg)
			}
			index++
			parsed.database = strings.TrimSpace(args[index])
		case strings.HasPrefix(arg, "--database="):
			parsed.database = strings.TrimSpace(strings.TrimPrefix(arg, "--database="))
		case strings.HasPrefix(arg, "--"):
			return parsed, fmt.Errorf("unknown flag %q", arg)
		default:
			parsed.positionals = append(parsed.positionals, arg)
		}
	}
	return parsed, nil
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

	rows := make([]boxRow, 0, len(workspaces.Items)+1)
	if len(workspaces.Items) == 0 {
		rows = append(rows, boxRow{Value: clr(ansiDim, "No workspaces found")})
	} else {
		layout := newWorkspaceListLayout(cfg, workspaces.Items)
		rows = append(rows, boxRow{Value: layout.header()})
		for _, meta := range workspaces.Items {
			rows = append(rows, boxRow{Value: layout.row(meta, workspaceListSelected(cfg, meta))})
		}
	}
	printBox(clr(ansiBold, "workspaces on "+configRemoteLabel(cfg)), rows)
	return nil
}

const workspaceListColumnSep = "  "

func workspaceListName(name string) string {
	return clr(ansiBold+ansiWhite, name)
}

func workspaceListMarker(selected bool) string {
	if selected {
		return clr(ansiBGreen, "✓")
	}
	return " "
}

type workspaceListLayout struct {
	markerWidth   int
	nameWidth     int
	databaseWidth int
	idWidth       int
	updatedWidth  int
}

func newWorkspaceListLayout(cfg config, items []workspaceSummary) workspaceListLayout {
	markerHeader := " "
	nameHeader := "Workspace"
	databaseHeader := "Database"
	idHeader := "ID"
	updatedHeader := "Updated"

	layout := workspaceListLayout{
		markerWidth:   runeWidth(markerHeader),
		nameWidth:     runeWidth(nameHeader),
		databaseWidth: runeWidth(databaseHeader),
		idWidth:       runeWidth(idHeader),
		updatedWidth:  runeWidth(updatedHeader),
	}
	for _, item := range items {
		layout.markerWidth = maxInt(layout.markerWidth, runeWidth(workspaceListMarker(workspaceListSelected(cfg, item))))
		layout.nameWidth = maxInt(layout.nameWidth, runeWidth(workspaceListName(item.Name)))
		layout.databaseWidth = maxInt(layout.databaseWidth, runeWidth(workspaceListDatabase(item)))
		layout.idWidth = maxInt(layout.idWidth, runeWidth(workspaceListID(item)))
		layout.updatedWidth = maxInt(layout.updatedWidth, runeWidth(workspaceListUpdated(item)))
	}

	maxContentWidth := maxBoxText - 4*runeWidth(workspaceListColumnSep) - layout.markerWidth
	layout.nameWidth, layout.databaseWidth, layout.idWidth, layout.updatedWidth =
		shrinkWorkspaceListColumns(
			maxContentWidth,
			layout.nameWidth,
			layout.databaseWidth,
			layout.idWidth,
			layout.updatedWidth,
			runeWidth(nameHeader),
			runeWidth(databaseHeader),
			runeWidth(idHeader),
			runeWidth(updatedHeader),
		)
	return layout
}

func shrinkWorkspaceListColumns(maxTotal, nameWidth, databaseWidth, idWidth, updatedWidth, minName, minDatabase, minID, minUpdated int) (int, int, int, int) {
	for nameWidth+databaseWidth+idWidth+updatedWidth > maxTotal {
		switch {
		case nameWidth > minName && nameWidth >= databaseWidth:
			nameWidth--
		case databaseWidth > minDatabase:
			databaseWidth--
		case nameWidth > minName:
			nameWidth--
		case updatedWidth > minUpdated:
			updatedWidth--
		case idWidth > minID:
			idWidth--
		default:
			return nameWidth, databaseWidth, idWidth, updatedWidth
		}
	}
	return nameWidth, databaseWidth, idWidth, updatedWidth
}

func (l workspaceListLayout) header() string {
	return strings.Join([]string{
		clr(ansiDim, padVisibleText("", l.markerWidth)),
		clr(ansiDim, padVisibleText("Workspace", l.nameWidth)),
		clr(ansiDim, padVisibleText("Database", l.databaseWidth)),
		clr(ansiDim, padVisibleText("ID", l.idWidth)),
		clr(ansiDim, padVisibleText("Updated", l.updatedWidth)),
	}, workspaceListColumnSep)
}

func (l workspaceListLayout) row(summary workspaceSummary, selected bool) string {
	return strings.Join([]string{
		padVisibleText(workspaceListMarker(selected), l.markerWidth),
		padVisibleText(fitDisplayText(workspaceListName(summary.Name), l.nameWidth), l.nameWidth),
		padVisibleText(fitDisplayText(workspaceListDatabase(summary), l.databaseWidth), l.databaseWidth),
		padVisibleText(fitDisplayText(workspaceListID(summary), l.idWidth), l.idWidth),
		padVisibleText(fitDisplayText(workspaceListUpdated(summary), l.updatedWidth), l.updatedWidth),
	}, workspaceListColumnSep)
}

func workspaceListDatabase(summary workspaceSummary) string {
	if databaseName := strings.TrimSpace(summary.DatabaseName); databaseName != "" {
		return databaseName
	}
	return "Direct Redis"
}

func workspaceListID(summary workspaceSummary) string {
	id := strings.TrimSpace(summary.ID)
	if id == "" {
		return "-"
	}
	return id
}

func workspaceListUpdated(summary workspaceSummary) string {
	updated := strings.TrimSpace(summary.UpdatedAt)
	if updated == "" {
		return "unknown"
	}
	parsed, err := time.Parse(time.RFC3339, updated)
	if err != nil {
		return updated
	}
	return parsed.Local().Format("2006-01-02 15:04")
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
	detail, err := service.GetWorkspace(context.Background(), selection.ID)
	if err != nil {
		return err
	}
	activeCheckpointID := ""
	if detail.DraftState != "dirty" {
		activeCheckpointID = detail.HeadCheckpointID
	}
	rows := make([]boxRow, 0, len(checkpoints))
	if len(checkpoints) == 0 {
		rows = append(rows, boxRow{Value: clr(ansiDim, "No checkpoints found")})
	} else {
		layout := newCheckpointListLayout(checkpoints, activeCheckpointID)
		rows = append(rows, boxRow{Value: layout.header()})
		for _, meta := range checkpoints {
			rows = append(rows, boxRow{Value: layout.row(meta, activeCheckpointID)})
		}
	}
	printBox(clr(ansiBold, "checkpoints in workspace: "+selection.Name), rows)
	return nil
}

const checkpointListColumnSep = "  "

type checkpointListLayout struct {
	nameWidth    int
	activeWidth  int
	createdWidth int
	sizeWidth    int
}

func newCheckpointListLayout(items []controlplane.CheckpointSummary, activeCheckpointID string) checkpointListLayout {
	nameHeader := "Checkpoint"
	activeHeader := "Active"
	createdHeader := "Created"
	sizeHeader := "Size"

	layout := checkpointListLayout{
		nameWidth:    runeWidth(nameHeader),
		activeWidth:  runeWidth(activeHeader),
		createdWidth: runeWidth(createdHeader),
		sizeWidth:    runeWidth(sizeHeader),
	}
	for _, item := range items {
		layout.nameWidth = maxInt(layout.nameWidth, runeWidth(checkpointListName(item)))
		layout.activeWidth = maxInt(layout.activeWidth, runeWidth(checkpointListActive(item, activeCheckpointID)))
		layout.createdWidth = maxInt(layout.createdWidth, runeWidth(checkpointListCreated(item)))
		layout.sizeWidth = maxInt(layout.sizeWidth, runeWidth(checkpointListSize(item)))
	}

	maxContentWidth := maxBoxText - 3*runeWidth(checkpointListColumnSep)
	for layout.nameWidth+layout.activeWidth+layout.createdWidth+layout.sizeWidth > maxContentWidth {
		switch {
		case layout.nameWidth > runeWidth(nameHeader):
			layout.nameWidth--
		case layout.createdWidth > runeWidth(createdHeader):
			layout.createdWidth--
		default:
			return layout
		}
	}
	return layout
}

func (l checkpointListLayout) header() string {
	return strings.Join([]string{
		clr(ansiDim, padVisibleText("Checkpoint", l.nameWidth)),
		clr(ansiDim, padVisibleText("Active", l.activeWidth)),
		clr(ansiDim, padVisibleText("Created", l.createdWidth)),
		clr(ansiDim, padVisibleText("Size", l.sizeWidth)),
	}, checkpointListColumnSep)
}

func (l checkpointListLayout) row(item controlplane.CheckpointSummary, activeCheckpointID string) string {
	return strings.Join([]string{
		padVisibleText(fitDisplayText(checkpointListName(item), l.nameWidth), l.nameWidth),
		padVisibleText(checkpointListActive(item, activeCheckpointID), l.activeWidth),
		padVisibleText(fitDisplayText(checkpointListCreated(item), l.createdWidth), l.createdWidth),
		padVisibleText(checkpointListSize(item), l.sizeWidth),
	}, checkpointListColumnSep)
}

func checkpointListName(item controlplane.CheckpointSummary) string {
	return clr(ansiBold+ansiWhite, item.Name)
}

func checkpointListActive(item controlplane.CheckpointSummary, activeCheckpointID string) string {
	if activeCheckpointID != "" && item.ID == activeCheckpointID {
		return "active"
	}
	return ""
}

func checkpointListCreated(item controlplane.CheckpointSummary) string {
	if created := strings.TrimSpace(formatDisplayTimestamp(item.CreatedAt)); created != "" {
		return created
	}
	return "unknown"
}

func checkpointListSize(item controlplane.CheckpointSummary) string {
	return formatBytes(item.TotalBytes)
}

type checkpointDiffArgs struct {
	workspace     string
	base          string
	head          string
	compareActive bool
}

func cmdCheckpointDiff(args []string) error {
	for _, arg := range args[2:] {
		if isHelpArg(arg) {
			fmt.Fprint(os.Stderr, checkpointDiffUsageText(filepath.Base(os.Args[0])))
			return nil
		}
	}
	parsed, err := parseCheckpointDiffArgs(args[2:])
	if err != nil {
		return fmt.Errorf("%s\n\n%s", err, checkpointDiffUsageText(filepath.Base(os.Args[0])))
	}

	cfg, service, closeStore, err := openAFSControlPlane(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	selection, err := resolveWorkspaceSelectionFromControlPlane(context.Background(), cfg, service, parsed.workspace)
	if err != nil {
		return err
	}
	baseView, err := checkpointDiffView(parsed.base)
	if err != nil {
		return err
	}
	headView, err := checkpointDiffView(parsed.head)
	if err != nil {
		return err
	}
	diff, err := service.DiffWorkspace(context.Background(), selection.ID, baseView, headView)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("checkpoint does not exist")
		}
		return err
	}
	printCheckpointDiff(selection.Name, diff)
	return nil
}

func parseCheckpointDiffArgs(args []string) (checkpointDiffArgs, error) {
	var parsed checkpointDiffArgs
	positionals := make([]string, 0, len(args))
	for _, arg := range args {
		switch arg {
		case "--active":
			parsed.compareActive = true
		default:
			if strings.HasPrefix(arg, "-") {
				return parsed, fmt.Errorf("unknown flag %q", arg)
			}
			positionals = append(positionals, arg)
		}
	}
	if parsed.compareActive {
		switch len(positionals) {
		case 1:
			parsed.base = positionals[0]
		case 2:
			parsed.workspace = positionals[0]
			parsed.base = positionals[1]
		default:
			return parsed, fmt.Errorf("expected [workspace] <checkpoint> --active")
		}
		parsed.head = "working-copy"
		return parsed, nil
	}
	switch len(positionals) {
	case 2:
		parsed.base = positionals[0]
		parsed.head = positionals[1]
	case 3:
		parsed.workspace = positionals[0]
		parsed.base = positionals[1]
		parsed.head = positionals[2]
	default:
		return parsed, fmt.Errorf("expected [workspace] <base-checkpoint> <target-checkpoint>")
	}
	return parsed, nil
}

func checkpointDiffView(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	switch {
	case raw == "":
		return "", fmt.Errorf("checkpoint is required")
	case raw == "active", raw == "working-copy":
		return "working-copy", nil
	case raw == "head":
		return raw, nil
	case strings.HasPrefix(raw, "checkpoint:"):
		checkpointID := strings.TrimPrefix(raw, "checkpoint:")
		if err := validateAFSName("checkpoint", checkpointID); err != nil {
			return "", err
		}
		return raw, nil
	default:
		if err := validateAFSName("checkpoint", raw); err != nil {
			return "", err
		}
		return "checkpoint:" + raw, nil
	}
}

func printCheckpointDiff(workspace string, diff controlplane.WorkspaceDiffResponse) {
	summary := diff.Summary
	rows := []boxRow{
		{Label: "workspace", Value: workspace},
		{Label: "base", Value: checkpointDiffDisplayView(diff.Base)},
		{Label: "target", Value: checkpointDiffDisplayView(diff.Head)},
		{Label: "changes", Value: checkpointDiffSummary(summary)},
	}
	if len(diff.Entries) == 0 {
		rows = append(rows, boxRow{})
		rows = append(rows, boxRow{Value: clr(ansiDim, "No changes")})
		printBox(clr(ansiBold, "checkpoint diff"), rows)
		return
	}
	rows = append(rows, boxRow{})
	limit := len(diff.Entries)
	if limit > 100 {
		limit = 100
	}
	for _, entry := range diff.Entries[:limit] {
		rows = append(rows, boxRow{
			Label: checkpointDiffOpLabel(entry.Op),
			Value: checkpointDiffEntryValue(entry),
		})
	}
	if extra := len(diff.Entries) - limit; extra > 0 {
		rows = append(rows, boxRow{Value: fmt.Sprintf("%d more changes not shown", extra)})
	}
	printBox(clr(ansiBold, "checkpoint diff"), rows)
}

func checkpointDiffDisplayView(state controlplane.DiffState) string {
	if state.CheckpointID != "" {
		return state.CheckpointID
	}
	if state.View == "working-copy" || state.View == "head" {
		return "active workspace"
	}
	return state.View
}

func checkpointDiffSummary(summary controlplane.DiffSummary) string {
	parts := []string{fmt.Sprintf("%d total", summary.Total)}
	if summary.Created > 0 {
		parts = append(parts, fmt.Sprintf("%d created", summary.Created))
	}
	if summary.Updated > 0 {
		parts = append(parts, fmt.Sprintf("%d updated", summary.Updated))
	}
	if summary.Deleted > 0 {
		parts = append(parts, fmt.Sprintf("%d deleted", summary.Deleted))
	}
	if summary.Renamed > 0 {
		parts = append(parts, fmt.Sprintf("%d renamed", summary.Renamed))
	}
	if summary.MetadataChanged > 0 {
		parts = append(parts, fmt.Sprintf("%d metadata", summary.MetadataChanged))
	}
	if summary.BytesAdded > 0 || summary.BytesRemoved > 0 {
		parts = append(parts, fmt.Sprintf("+%s / -%s", formatBytes(summary.BytesAdded), formatBytes(summary.BytesRemoved)))
	}
	return strings.Join(parts, " · ")
}

func checkpointDiffOpLabel(op string) string {
	switch op {
	case controlplane.DiffOpCreate:
		return "Create"
	case controlplane.DiffOpUpdate:
		return "Update"
	case controlplane.DiffOpDelete:
		return "Delete"
	case controlplane.DiffOpRename:
		return "Rename"
	case controlplane.DiffOpMetadata:
		return "Metadata"
	default:
		return op
	}
}

func checkpointDiffEntryValue(entry controlplane.DiffEntry) string {
	switch entry.Op {
	case controlplane.DiffOpRename:
		return fmt.Sprintf("%s -> %s", entry.PreviousPath, entry.Path)
	case controlplane.DiffOpDelete:
		kind := strings.TrimSpace(entry.PreviousKind)
		if kind == "" {
			kind = "path"
		}
		return fmt.Sprintf("%s (%s)", entry.Path, kind)
	default:
		value := entry.Path
		if entry.Kind != "" {
			value += " (" + entry.Kind + ")"
		}
		if entry.DeltaBytes > 0 {
			value += " +" + formatBytes(entry.DeltaBytes)
		} else if entry.DeltaBytes < 0 {
			value += " -" + formatBytes(-entry.DeltaBytes)
		}
		return value
	}
}

func cmdCheckpointCreate(args []string) error {
	for _, arg := range args[2:] {
		if isHelpArg(arg) {
			fmt.Fprint(os.Stderr, checkpointCreateUsageText(filepath.Base(os.Args[0])))
			return nil
		}
	}
	parsed, err := parseCheckpointCreateArgs(args[2:])
	if err != nil {
		return fmt.Errorf("%s\n\n%s", err, checkpointCreateUsageText(filepath.Base(os.Args[0])))
	}

	cfg, service, closeFn, err := openAFSControlPlane(context.Background())
	if err != nil {
		return err
	}
	defer closeFn()

	workspace := ""
	checkpointID := generatedSavepointName()
	switch len(parsed.positionals) {
	case 2:
		workspace = parsed.positionals[0]
		checkpointID = parsed.positionals[1]
	case 1:
		if hasCurrentWorkspaceSelection(cfg) {
			checkpointID = parsed.positionals[0]
		} else {
			workspace = parsed.positionals[0]
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
	saved, err := saveCheckpointFromLiveWithOptions(context.Background(), service, selection.Name, checkpointID, controlplane.SaveCheckpointFromLiveOptions{
		Description: parsed.description,
	})
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

	rows := []boxRow{
		{Label: "workspace", Value: selection.Name},
		{Label: "checkpoint", Value: checkpointID},
	}
	if parsed.description != "" {
		rows = append(rows, boxRow{Label: "description", Value: parsed.description})
	}
	rows = append(rows, boxRow{Label: "database", Value: configRemoteLabel(cfg)})
	printBox(markerSuccess+" "+clr(ansiBold, "checkpoint created"), rows)
	return nil
}

type checkpointCreateArgs struct {
	positionals []string
	description string
}

func parseCheckpointCreateArgs(args []string) (checkpointCreateArgs, error) {
	var parsed checkpointCreateArgs
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--description":
			if i+1 >= len(args) {
				return parsed, fmt.Errorf("--description requires a value")
			}
			i++
			parsed.description = strings.TrimSpace(args[i])
		case strings.HasPrefix(arg, "--description="):
			parsed.description = strings.TrimSpace(strings.TrimPrefix(arg, "--description="))
		case strings.HasPrefix(arg, "-"):
			return parsed, fmt.Errorf("unknown flag %q", arg)
		default:
			parsed.positionals = append(parsed.positionals, arg)
		}
	}
	if len(parsed.positionals) > 2 {
		return parsed, fmt.Errorf("expected at most workspace and checkpoint, got %d positional arguments", len(parsed.positionals))
	}
	return parsed, nil
}

type checkpointLiveSaverWithOptions interface {
	SaveCheckpointFromLiveWithOptions(ctx context.Context, workspace, checkpointID string, options controlplane.SaveCheckpointFromLiveOptions) (bool, error)
}

var (
	checkpointRestoreStartSyncServices = startSyncServices
	checkpointRestoreTerminatePID      = terminatePID
)

func saveCheckpointFromLiveWithOptions(ctx context.Context, service afsControlPlane, workspace, checkpointID string, options controlplane.SaveCheckpointFromLiveOptions) (bool, error) {
	options.Description = strings.TrimSpace(options.Description)
	if saver, ok := service.(checkpointLiveSaverWithOptions); ok {
		return saver.SaveCheckpointFromLiveWithOptions(ctx, workspace, checkpointID, options)
	}
	if options.Description != "" {
		return false, fmt.Errorf("checkpoint descriptions are not supported by this control plane")
	}
	return service.SaveCheckpointFromLive(ctx, workspace, checkpointID)
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

	if err := guardCheckpointRestoreLocalHandles(cfg, workspace); err != nil {
		return err
	}
	syncRuntime, err := prepareCheckpointRestoreSyncRuntime(ctx, cfg, workspace)
	if err != nil {
		return err
	}

	result, err := resetAFSWorkspaceHead(ctx, service, workspace, checkpointID)
	if err != nil {
		if syncRuntime != nil {
			_ = syncRuntime.restart(ctx)
		}
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("checkpoint %q does not exist", checkpointID)
		}
		if err == redis.TxFailedErr || errors.Is(err, errAFSWorkspaceConflict) {
			return fmt.Errorf("checkpoint restore conflict while restoring %q", checkpointID)
		}
		return err
	}
	if syncRuntime != nil {
		if err := syncRuntime.replaceLocalAndRestart(ctx); err != nil {
			return err
		}
	}

	rows := []boxRow{
		{Label: "workspace", Value: workspace},
		{Label: "checkpoint", Value: checkpointID},
	}
	if result.SafetyCheckpointCreated {
		rows = append(rows, boxRow{Label: "safety checkpoint", Value: result.SafetyCheckpointID})
	}
	rows = append(rows, boxRow{Label: "database", Value: configRemoteLabel(cfg)})
	printBox(markerSuccess+" "+clr(ansiBold, "checkpoint restored"), rows)
	return nil
}

func guardCheckpointRestoreLocalHandles(cfg config, workspace string) error {
	st, err := loadState()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if strings.TrimSpace(st.Mode) != modeSync || !stateMatchesRestoreWorkspace(cfg, st, workspace) || !stateClientAlive(st) {
		return nil
	}
	return ensureNoOpenHandlesUnderPath(st.LocalPath, st.SyncPID)
}

type checkpointRestoreSyncRuntime struct {
	st         state
	restartCfg config
	workspace  string
}

func prepareCheckpointRestoreSyncRuntime(ctx context.Context, cfg config, workspace string) (*checkpointRestoreSyncRuntime, error) {
	st, err := loadState()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if strings.TrimSpace(st.Mode) != modeSync || !stateMatchesRestoreWorkspace(cfg, st, workspace) || !stateClientAlive(st) {
		return nil, nil
	}
	runtime := &checkpointRestoreSyncRuntime{
		st:         st,
		restartCfg: checkpointRestoreSyncConfig(cfg, st, workspace),
		workspace:  workspace,
	}
	if st.SyncPID > 0 && processAlive(st.SyncPID) {
		step := startStep("Stopping sync daemon")
		if err := checkpointRestoreTerminatePID(st.SyncPID, 5*time.Second); err != nil {
			step.fail(err.Error())
			return nil, err
		}
		step.succeed(fmt.Sprintf("pid %d", st.SyncPID))
	}
	closeManagedWorkspaceSession(configFromState(st), strings.TrimSpace(st.CurrentWorkspace), strings.TrimSpace(st.SessionID))
	return runtime, nil
}

func checkpointRestoreSyncConfig(cfg config, st state, workspace string) config {
	restart := cfg
	restart.Mode = modeSync
	restart.MountBackend = mountBackendNone
	restart.CurrentWorkspace = strings.TrimSpace(workspace)
	if restart.CurrentWorkspace == "" {
		restart.CurrentWorkspace = strings.TrimSpace(st.CurrentWorkspace)
	}
	restart.CurrentWorkspaceID = strings.TrimSpace(st.CurrentWorkspaceID)
	if strings.TrimSpace(st.ProductMode) != "" {
		restart.ProductMode = strings.TrimSpace(st.ProductMode)
	}
	if strings.TrimSpace(st.ControlPlaneURL) != "" {
		restart.URL = strings.TrimSpace(st.ControlPlaneURL)
	}
	if strings.TrimSpace(st.ControlPlaneDatabase) != "" {
		restart.DatabaseID = strings.TrimSpace(st.ControlPlaneDatabase)
	}
	if strings.TrimSpace(st.RedisAddr) != "" {
		restart.RedisAddr = strings.TrimSpace(st.RedisAddr)
	}
	if st.RedisDB >= 0 {
		restart.RedisDB = st.RedisDB
	}
	if strings.TrimSpace(st.LocalPath) != "" {
		restart.LocalPath = strings.TrimSpace(st.LocalPath)
	}
	if strings.TrimSpace(st.SyncLog) != "" {
		restart.SyncLog = strings.TrimSpace(st.SyncLog)
	}
	restart.ReadOnly = st.ReadOnly
	return restart
}

func (r *checkpointRestoreSyncRuntime) restart(ctx context.Context) error {
	if r == nil {
		return nil
	}
	return checkpointRestoreStartSyncServices(r.restartCfg, false)
}

func (r *checkpointRestoreSyncRuntime) replaceLocalAndRestart(ctx context.Context) error {
	if r == nil {
		return nil
	}
	localPath := strings.TrimSpace(r.st.LocalPath)
	if localPath != "" {
		step := startStep("Replacing local sync folder")
		if err := os.RemoveAll(localPath); err != nil {
			step.fail(err.Error())
			return err
		}
		step.succeed(localPath)
	}
	if workspace := strings.TrimSpace(r.st.CurrentWorkspace); workspace != "" {
		_ = removeSyncState(workspace)
	}
	if err := os.Remove(statePath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return r.restart(ctx)
}

func stateMatchesRestoreWorkspace(cfg config, st state, workspace string) bool {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return false
	}
	if strings.TrimSpace(st.CurrentWorkspace) != workspace && strings.TrimSpace(st.CurrentWorkspaceID) != workspace {
		return false
	}
	if mode := strings.TrimSpace(st.ProductMode); mode != "" && strings.TrimSpace(cfg.ProductMode) != "" && mode != strings.TrimSpace(cfg.ProductMode) {
		return false
	}
	if url := strings.TrimSpace(st.ControlPlaneURL); url != "" && strings.TrimSpace(cfg.URL) != "" && url != strings.TrimSpace(cfg.URL) {
		return false
	}
	if databaseID := strings.TrimSpace(st.ControlPlaneDatabase); databaseID != "" && strings.TrimSpace(cfg.DatabaseID) != "" && databaseID != strings.TrimSpace(cfg.DatabaseID) {
		return false
	}
	return true
}

func stateClientAlive(st state) bool {
	return (st.MountPID > 0 && processAlive(st.MountPID)) || (st.SyncPID > 0 && processAlive(st.SyncPID))
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
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s workspace <subcommand>

Subcommands:
  create <workspace>                           Create an empty workspace
  list                                         List workspaces in Redis
  current                                      Show the current workspace
  use <workspace>                              Set the current workspace
  clone [workspace] <directory>                Clone a workspace into a local directory
  fork [source-workspace] <new-workspace>      Fork a workspace from its current checkpoint
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
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s workspace create [--database <database-id|database-name>] <workspace>

Create an empty workspace with an initial checkpoint named "initial".
`, bin)
}

func workspaceListUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s workspace list

List workspaces stored in Redis, along with checkpoint counts and creation time.
`, bin)
}

func workspaceCurrentUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s workspace current

Show the current workspace selection AFS will use when a workspace argument is omitted.
`, bin)
}

func workspaceUseUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s workspace use <workspace-name-or-id>

Set the current workspace AFS will use when a workspace argument is omitted.
When names collide across databases, use the workspace ID from '%s workspace list'.
`, bin, bin)
}

func workspaceCloneUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s workspace clone [workspace] <directory>

Clone a workspace into a local directory at its current checkpoint.
The destination must not already contain files.

If [workspace] is omitted, AFS uses the current workspace.
`, bin)
}

func workspaceForkUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s workspace fork [source-workspace] <new-workspace>

Create a new workspace from the source workspace's current checkpoint.

If [source-workspace] is omitted, AFS uses the current workspace.
`, bin)
}

func workspaceDeleteUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s workspace delete <workspace>...

Delete one or more workspaces from Redis and remove their local materialized state.
`, bin)
}

func workspaceImportUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s workspace import [--force] [--database <database-id|database-name>] [--mount-at-source] <workspace> <directory>

Import a local directory into a workspace.

Options:
  --force            Replace an existing workspace
  --database         Override the control-plane database for this import
  --mount-at-source  Archive the source directory to <directory>.pre-afs and
                     mount the imported workspace at the original path
`, bin)
}

func checkpointUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s checkpoint <subcommand>

	Subcommands:
	  list [workspace]                     List checkpoints for a workspace
	  create [workspace] [checkpoint]     Create a checkpoint from the current workspace state
	  diff [workspace] <base> <target>    Compare two checkpoints
	  restore [workspace] <checkpoint>    Restore a workspace to a checkpoint

	Examples:
	  %s checkpoint list demo
	  %s checkpoint create demo before-refactor
	  %s checkpoint diff demo initial before-refactor
	  %s checkpoint restore demo initial

	Run '%s checkpoint <subcommand> --help' for details.
	`, bin, bin, bin, bin, bin, bin)
}

func checkpointListUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s checkpoint list [workspace]

List checkpoints for a workspace, newest first.
`, bin)
}

func checkpointCreateUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s checkpoint create [workspace] [checkpoint] [--description <text>]

Create a checkpoint from the workspace's active state.
If [checkpoint] is omitted, AFS generates a timestamped name.

Options:
  --description <text>  Human-readable checkpoint description
`, bin)
}

func checkpointDiffUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s checkpoint diff [workspace] <base-checkpoint> <target-checkpoint>
  %s checkpoint diff [workspace] <checkpoint> --active

Compare saved filesystem states. Use --active to compare a checkpoint to the
active workspace state.
`, bin, bin)
}

func checkpointRestoreUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s checkpoint restore [workspace] <checkpoint>

Restore the active workspace state to the selected checkpoint.
`, bin)
}
