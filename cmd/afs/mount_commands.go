package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/redis/agent-filesystem/internal/version"
	"github.com/redis/agent-filesystem/mount/client"
	"github.com/redis/go-redis/v9"
)

const mountShellCDFileEnv = "AFS_MOUNT_CD_FILE"

type mountOptions struct {
	workspace   string
	directory   string
	sessionName string
	verbose     bool
	dryRun      bool
	yes         bool
	readonly    bool
}

type unmountOptions struct {
	target      string
	deleteLocal bool
}

var (
	startMountServicesForWorkspaceMount = startMountServices
	startSyncMountForWorkspaceMount     = startSyncMount
)

func cmdMountArgs(args []string) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		fmt.Fprint(os.Stderr, mountUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	opts, err := parseMountOptions(args)
	if err != nil {
		return err
	}
	if strings.TrimSpace(opts.workspace) == "" {
		return promptMountSelection(opts)
	}
	if strings.TrimSpace(opts.directory) == "" {
		directory, ok, err := promptMountPathForWorkspace(opts.workspace)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		opts.directory = directory
	}
	return mountWorkspace(opts)
}

func cmdUnmountArgs(args []string) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		fmt.Fprint(os.Stderr, unmountUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	opts, err := parseUnmountOptions(args, false)
	if err != nil {
		return err
	}
	if strings.TrimSpace(opts.target) == "" {
		return promptUnmountSelection(opts.deleteLocal)
	}
	return unmountWorkspaceTarget(opts.target, opts.deleteLocal)
}

func parseMountOptions(args []string) (mountOptions, error) {
	var opts mountOptions
	var positionals []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--verbose", "-v":
			opts.verbose = true
		case "--dry-run":
			opts.dryRun = true
		case "--yes", "-y":
			opts.yes = true
		case "--readonly":
			opts.readonly = true
		case "--session":
			i++
			if i >= len(args) || strings.TrimSpace(args[i]) == "" {
				return opts, fmt.Errorf("--session requires a name")
			}
			opts.sessionName = strings.TrimSpace(args[i])
		default:
			if strings.HasPrefix(arg, "--session=") {
				value := strings.TrimSpace(strings.TrimPrefix(arg, "--session="))
				if value == "" {
					return opts, fmt.Errorf("--session requires a name")
				}
				opts.sessionName = value
				continue
			}
			if strings.HasPrefix(arg, "-") {
				return opts, fmt.Errorf("unknown flag %q\n\n%s", arg, mountUsageText(filepath.Base(os.Args[0])))
			}
			positionals = append(positionals, arg)
		}
	}
	if len(positionals) > 2 {
		return opts, fmt.Errorf("%s", mountUsageText(filepath.Base(os.Args[0])))
	}
	if len(positionals) >= 1 {
		opts.workspace = positionals[0]
	}
	if len(positionals) == 2 {
		opts.directory = positionals[1]
	}
	return opts, nil
}

func parseUnmountOptions(args []string, requirePath bool) (unmountOptions, error) {
	var opts unmountOptions
	var positionals []string
	for _, arg := range args {
		switch arg {
		case "--delete":
			opts.deleteLocal = true
		default:
			if strings.HasPrefix(arg, "-") {
				return opts, fmt.Errorf("unknown flag %q\n\n%s", arg, unmountUsageText(filepath.Base(os.Args[0])))
			}
			positionals = append(positionals, arg)
		}
	}
	if len(positionals) > 1 || (requirePath && len(positionals) != 1) {
		return opts, fmt.Errorf("%s", unmountUsageText(filepath.Base(os.Args[0])))
	}
	if len(positionals) == 1 {
		opts.target = positionals[0]
	}
	return opts, nil
}

func mountWorkspace(opts mountOptions) error {
	if err := validateAFSName("workspace", opts.workspace); err != nil {
		return err
	}
	localPath, err := normalizeMountPath(opts.directory)
	if err != nil {
		return err
	}

	reg, err := loadMountRegistry()
	if err != nil {
		return err
	}
	if conflict, ok := mountPathConflict(reg, localPath); ok {
		return fmt.Errorf("path %s overlaps existing mount %s at %s", localPath, conflict.Workspace, conflict.LocalPath)
	}

	cfg, err := loadAFSConfig()
	if err != nil {
		return err
	}
	mode, err := effectiveMode(cfg)
	if err != nil {
		return err
	}
	if mode == modeNone {
		return fmt.Errorf("mode is set to none; update config first")
	}
	cfg.CurrentWorkspace = opts.workspace
	cfg.CurrentWorkspaceID = ""
	cfg.LocalPath = localPath
	if mode == modeSync {
		cfg.Mode = modeSync
		cfg.MountBackend = mountBackendNone
	} else {
		cfg.Mode = modeMount
	}

	ctx := context.Background()
	resolvedCfg, service, closeStore, err := openAFSControlPlaneForConfig(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeStore()

	selection, err := resolveWorkspaceSelectionFromControlPlane(ctx, resolvedCfg, service, opts.workspace)
	if err != nil {
		return err
	}
	if conflict, ok := mountWorkspaceConflict(reg, selection.ID, selection.Name); ok {
		return fmt.Errorf("workspace %s is already mounted at %s", conflict.Workspace, conflict.LocalPath)
	}

	resolvedCfg.CurrentWorkspace = selection.Name
	resolvedCfg.CurrentWorkspaceID = selection.ID
	resolvedCfg.LocalPath = localPath
	if mode == modeMount {
		if err := applyWorkspaceSelection(&resolvedCfg, selection); err != nil {
			return err
		}
		resolvedCfg.LocalPath = localPath
		resolvedCfg.Mode = modeMount
		resolvedCfg.ReadOnly = opts.readonly
		return startLiveMount(resolvedCfg, selection, opts)
	}
	resolvedCfg.Mode = modeSync
	resolvedCfg.MountBackend = mountBackendNone
	return startSyncMountForWorkspaceMount(ctx, resolvedCfg, selection, opts)
}

func startLiveMount(cfg config, selection workspaceSelection, opts mountOptions) error {
	if opts.dryRun {
		return fmt.Errorf("--dry-run is only available for sync-mode mounts")
	}
	if strings.TrimSpace(cfg.LocalPath) == "" {
		return errors.New("mount requires a local directory")
	}
	if err := validateUpModeSelection(cfg); err != nil {
		return err
	}
	st, err := startMountServicesForWorkspaceMount(cfg, opts.sessionName)
	if err != nil {
		return err
	}
	if strings.TrimSpace(st.CurrentWorkspace) == "" {
		st.CurrentWorkspace = selection.Name
	}
	if strings.TrimSpace(st.CurrentWorkspaceID) == "" {
		st.CurrentWorkspaceID = selection.ID
	}
	if err := registerLiveMount(st); err != nil {
		_ = stopMount(mountRecordFromLiveState(st, ""), false)
		return err
	}
	recordMountShellDirectory(st.LocalPath)
	return nil
}

func registerLiveMount(st state) error {
	reg, err := loadMountRegistry()
	if err != nil {
		return err
	}
	id, err := randomSuffix()
	if err != nil {
		return err
	}
	upsertMount(&reg, mountRecordFromLiveState(st, "mnt_"+id))
	return saveMountRegistry(reg)
}

func mountRecordFromLiveState(st state, id string) mountRecord {
	return mountRecord{
		ID:                   id,
		Workspace:            st.CurrentWorkspace,
		WorkspaceID:          st.CurrentWorkspaceID,
		LocalPath:            st.LocalPath,
		Mode:                 modeMount,
		MountBackend:         st.MountBackend,
		ProductMode:          st.ProductMode,
		ControlPlaneURL:      st.ControlPlaneURL,
		ControlPlaneDatabase: st.ControlPlaneDatabase,
		SessionID:            st.SessionID,
		RedisAddr:            st.RedisAddr,
		RedisDB:              st.RedisDB,
		RedisKey:             st.RedisKey,
		PID:                  st.MountPID,
		ReadOnly:             st.ReadOnly,
		StartedAt:            st.StartedAt,
	}
}

func startSyncMount(ctx context.Context, cfg config, selection workspaceSelection, opts mountOptions) error {
	if strings.TrimSpace(cfg.LocalPath) == "" {
		return errors.New("mount requires a local directory")
	}
	localRoot, err := normalizeMountPath(cfg.LocalPath)
	if err != nil {
		return err
	}
	cfg.LocalPath = localRoot
	if err := validateSyncLocalPath(cfg, localRoot); err != nil {
		return err
	}
	localSnapshot, err := inspectMountLocalRoot(localRoot)
	if err != nil {
		return err
	}

	if opts.verbose {
		fmt.Printf("opening workspace session: %s\n", selection.Name)
	}
	requested := selection.ID
	if strings.TrimSpace(requested) == "" {
		requested = selection.Name
	}
	bootstrap, closeSession, err := prepareSyncBootstrapForWorkspace(ctx, cfg, requested, opts.sessionName)
	if err != nil {
		return err
	}
	defer closeSession()
	runtimeCfg := bootstrap.cfg
	runtimeCfg.CurrentWorkspace = selection.Name
	runtimeCfg.CurrentWorkspaceID = selection.ID
	runtimeCfg.LocalPath = localRoot
	runtimeCfg.ReadOnly = opts.readonly
	ctx = withSessionID(ctx, bootstrap.sessionID)

	rdb := redis.NewClient(buildRedisOptions(runtimeCfg, 4))
	defer rdb.Close()
	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	defer pingCancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		closeManagedWorkspaceSession(runtimeCfg, bootstrap.workspace, bootstrap.sessionID)
		return fmt.Errorf("cannot connect to Redis at %s: %w", runtimeCfg.RedisAddr, err)
	}

	store := newAFSStore(rdb)
	fsClient := client.New(rdb, bootstrap.redisKey)
	daemon, err := newSyncDaemon(syncDaemonConfig{
		Workspace:    bootstrap.workspace,
		LocalRoot:    localRoot,
		FS:           fsClient,
		Store:        store,
		MaxFileBytes: syncSizeCapBytes(runtimeCfg),
		Readonly:     runtimeCfg.ReadOnly,
		Rdb:          rdb,
		StorageID:    bootstrap.redisKey,
		SessionID:    bootstrap.sessionID,
		AgentID:      runtimeCfg.ID,
		Label:        runtimeCfg.Name,
		AgentVersion: version.String(),
	})
	if err != nil {
		closeManagedWorkspaceSession(runtimeCfg, bootstrap.workspace, bootstrap.sessionID)
		return err
	}
	plan, err := buildMountReconcilePlan(ctx, daemon)
	if err != nil {
		closeManagedWorkspaceSession(runtimeCfg, bootstrap.workspace, bootstrap.sessionID)
		return err
	}
	if mountPlanShouldResetEmptyLocalState(plan, localSnapshot) {
		resetMountSyncState(daemon)
		plan, err = buildMountReconcilePlan(ctx, daemon)
		if err != nil {
			closeManagedWorkspaceSession(runtimeCfg, bootstrap.workspace, bootstrap.sessionID)
			return err
		}
	}
	emptyLocalRemoteDelete := mountPlanDeletesRemoteFromEmptyLocal(plan, localSnapshot)
	if opts.dryRun {
		printMountReconcilePlan("Would mount workspace", selection.Name, localRoot, plan, true)
		if emptyLocalRemoteDelete {
			printEmptyLocalDeleteWarning(selection.Name, localRoot, plan)
		}
		return nil
	}
	if plan.hasReportableChanges() {
		printMountReconcilePlan("Mount changes", selection.Name, localRoot, plan, opts.verbose || plan.ConflictCount > 0)
	}
	if plan.ConflictCount > 0 {
		closeManagedWorkspaceSession(runtimeCfg, bootstrap.workspace, bootstrap.sessionID)
		return fmt.Errorf("mount has %d conflict(s); resolve them or move conflicting files aside before mounting", plan.ConflictCount)
	}
	emptyLocalDeleteConfirmed := false
	if emptyLocalRemoteDelete {
		printEmptyLocalDeleteWarning(selection.Name, localRoot, plan)
		if !isInteractiveTerminal() {
			closeManagedWorkspaceSession(runtimeCfg, bootstrap.workspace, bootstrap.sessionID)
			return errors.New("mount would delete remote workspace entries because the local folder is empty; rerun in an interactive terminal to confirm that destructive action")
		}
		ok, err := promptYesNo(
			bufio.NewReader(os.Stdin),
			os.Stdout,
			"Did you intentionally delete every local file and want to delete the remote workspace entries?",
			false,
		)
		if err != nil {
			closeManagedWorkspaceSession(runtimeCfg, bootstrap.workspace, bootstrap.sessionID)
			return err
		}
		if !ok {
			fmt.Println("Mount cancelled.")
			closeManagedWorkspaceSession(runtimeCfg, bootstrap.workspace, bootstrap.sessionID)
			return nil
		}
		emptyLocalDeleteConfirmed = true
	}
	if plan.requiresConfirmation() && !opts.yes && !emptyLocalDeleteConfirmed {
		if !isInteractiveTerminal() {
			closeManagedWorkspaceSession(runtimeCfg, bootstrap.workspace, bootstrap.sessionID)
			return errors.New("mount would change an existing local folder; rerun in an interactive terminal, pass --dry-run to inspect the plan, or pass --yes to accept the safe mount plan")
		}
		ok, err := promptYesNo(bufio.NewReader(os.Stdin), os.Stdout, "Continue with mount sync plan?", false)
		if err != nil {
			closeManagedWorkspaceSession(runtimeCfg, bootstrap.workspace, bootstrap.sessionID)
			return err
		}
		if !ok {
			fmt.Println("Mount cancelled.")
			closeManagedWorkspaceSession(runtimeCfg, bootstrap.workspace, bootstrap.sessionID)
			return nil
		}
	}
	approveMountReconcilePlan(daemon, plan)
	progress := func(done, total int64) {
		if !opts.verbose {
			return
		}
		if total < 0 {
			fmt.Printf("scanning: %d entries\n", done)
			return
		}
		fmt.Printf("syncing: %d/%d files\n", done, total)
	}
	if err := daemon.StartWithProgress(ctx, progress); err != nil {
		closeManagedWorkspaceSession(runtimeCfg, bootstrap.workspace, bootstrap.sessionID)
		return err
	}
	daemon.Stop()

	daemonBootstrap := &syncDaemonBootstrap{
		Config:                   runtimeCfg,
		Workspace:                bootstrap.workspace,
		RedisKey:                 bootstrap.redisKey,
		SessionID:                bootstrap.sessionID,
		HeartbeatIntervalSeconds: int(bootstrap.heartbeatEvery / time.Second),
	}
	daemonPID, err := startSyncDaemonProcess(runtimeCfg, daemonBootstrap)
	if err != nil {
		closeManagedWorkspaceSession(runtimeCfg, bootstrap.workspace, bootstrap.sessionID)
		return err
	}

	reg, err := loadMountRegistry()
	if err != nil {
		_ = terminatePID(daemonPID, 5*time.Second)
		closeManagedWorkspaceSession(runtimeCfg, bootstrap.workspace, bootstrap.sessionID)
		return err
	}
	id, err := randomSuffix()
	if err != nil {
		_ = terminatePID(daemonPID, 5*time.Second)
		closeManagedWorkspaceSession(runtimeCfg, bootstrap.workspace, bootstrap.sessionID)
		return err
	}
	upsertMount(&reg, mountRecord{
		ID:                   "mnt_" + id,
		Workspace:            bootstrap.workspace,
		WorkspaceID:          runtimeCfg.CurrentWorkspaceID,
		LocalPath:            localRoot,
		Mode:                 modeSync,
		ProductMode:          runtimeCfg.ProductMode,
		ControlPlaneURL:      runtimeCfg.URL,
		ControlPlaneDatabase: runtimeCfg.DatabaseID,
		SessionID:            bootstrap.sessionID,
		RedisAddr:            runtimeCfg.RedisAddr,
		RedisDB:              runtimeCfg.RedisDB,
		RedisKey:             bootstrap.redisKey,
		PID:                  daemonPID,
		ReadOnly:             runtimeCfg.ReadOnly,
		SyncLog:              runtimeCfg.SyncLog,
		StartedAt:            time.Now().UTC(),
	})
	if err := saveMountRegistry(reg); err != nil {
		_ = terminatePID(daemonPID, 5*time.Second)
		closeManagedWorkspaceSession(runtimeCfg, bootstrap.workspace, bootstrap.sessionID)
		return err
	}
	recordMountShellDirectory(localRoot)

	entryCount := "synced"
	if snap, err := loadSyncState(bootstrap.workspace); err == nil && snap != nil {
		live, _ := syncStateEntryCounts(snap)
		entryCount = strconv.Itoa(live) + " entries"
	}
	rows := []outputRow{
		{Label: "workspace", Value: bootstrap.workspace},
		{Label: "path", Value: compactDisplayPath(localRoot)},
		{Label: "mode", Value: "sync"},
		{Label: "files", Value: entryCount},
	}
	if runtimeCfg.ReadOnly {
		rows = append(rows, outputRow{Label: "readonly", Value: "yes"})
	}
	rows = append(rows, outputRow{Label: "unmount", Value: filepath.Base(os.Args[0]) + " ws unmount " + shellQuote(bootstrap.workspace)})
	if opts.verbose && strings.TrimSpace(bootstrap.sessionID) != "" {
		rows = append(rows, outputRow{Label: "session", Value: strings.TrimSpace(bootstrap.sessionID)})
	}
	printSection("Workspace mounted", rows)
	return nil
}

func printEmptyLocalDeleteWarning(workspace, localRoot string, plan mountReconcilePlan) {
	printSection("Empty local folder", []outputRow{
		{Label: "workspace", Value: workspace},
		{Label: "path", Value: compactDisplayPath(localRoot)},
		{Label: "remote entries", Value: fmt.Sprintf("%d", plan.RemoteCount)},
		{Label: "would delete", Value: fmt.Sprintf("%d remote entries", plan.DeleteRemoteCount)},
		{Label: "note", Value: "This only makes sense if you deleted everything while unmounted."},
	})
}

func recordMountShellDirectory(localRoot string) {
	target := strings.TrimSpace(os.Getenv(mountShellCDFileEnv))
	if target == "" {
		return
	}
	_ = os.WriteFile(target, []byte(localRoot+"\n"), 0o600)
}

type mountPromptChoice struct {
	Workspace   string
	WorkspaceID string
	DatabaseID  string
	Database    string
	Path        string
	Mounted     bool
}

func promptMountSelection(opts mountOptions) error {
	_, service, closeStore, err := openAFSControlPlane(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	workspaces, err := service.ListWorkspaceSummaries(context.Background())
	if err != nil {
		return err
	}
	reg, err := loadMountRegistry()
	if err != nil {
		return err
	}

	choices := mountPromptChoices(reg, workspaces.Items)
	if len(choices) == 0 {
		fmt.Println()
		fmt.Println("Mount workspace")
		fmt.Println()
		fmt.Println("No workspaces found.")
		fmt.Println("Create one with: " + filepath.Base(os.Args[0]) + " ws create <workspace>")
		fmt.Println()
		return nil
	}

	fmt.Println()
	fmt.Println("Mount workspace")
	fmt.Println()
	printPlainTable([]string{"#", "Workspace", "Workspace ID", "Database", "Status", "Path"}, mountPromptRows(choices))
	fmt.Println()
	fmt.Print("Workspace to mount: ")

	reader := bufio.NewReader(os.Stdin)
	raw, err := reader.ReadString('\n')
	if err != nil && strings.TrimSpace(raw) == "" {
		fmt.Println()
		fmt.Println("Mount cancelled.")
		fmt.Println()
		return nil
	}
	choiceText := strings.TrimSpace(raw)
	if choiceText == "" {
		fmt.Println("Mount cancelled.")
		fmt.Println()
		return nil
	}
	idx, err := strconv.Atoi(choiceText)
	if err != nil || idx < 1 || idx > len(choices) {
		return fmt.Errorf("invalid selection %q", choiceText)
	}
	selected := choices[idx-1]
	if selected.Mounted {
		printSection("Workspace already mounted", []outputRow{
			{Label: "workspace", Value: selected.Workspace},
			{Label: "path", Value: compactDisplayPath(selected.Path)},
		})
		return nil
	}

	defaultPath := "~/" + selected.Workspace
	fmt.Printf("Local folder [%s]: ", defaultPath)
	rawPath, err := reader.ReadString('\n')
	if err != nil && strings.TrimSpace(rawPath) == "" {
		fmt.Println()
		fmt.Println("Mount cancelled.")
		fmt.Println()
		return nil
	}
	localPath := strings.TrimSpace(rawPath)
	if localPath == "" {
		localPath = defaultPath
	}
	opts.workspace = selected.Workspace
	if strings.TrimSpace(selected.WorkspaceID) != "" {
		opts.workspace = selected.WorkspaceID
	}
	opts.directory = localPath
	return mountWorkspace(opts)
}

func mountPromptChoices(reg mountRegistry, workspaces []workspaceSummary) []mountPromptChoice {
	choices := make([]mountPromptChoice, 0, len(reg.Mounts)+len(workspaces))
	summariesByID := make(map[string]workspaceSummary, len(workspaces))
	summariesByName := make(map[string][]workspaceSummary)
	for _, ws := range workspaces {
		if id := strings.TrimSpace(ws.ID); id != "" {
			summariesByID[id] = ws
		}
		if name := strings.TrimSpace(ws.Name); name != "" {
			summariesByName[name] = append(summariesByName[name], ws)
		}
	}

	mountedByID := make(map[string]bool, len(reg.Mounts))
	mountedByDatabaseName := make(map[string]bool, len(reg.Mounts))
	mountedByLegacyName := make(map[string]bool, len(reg.Mounts))
	for _, rec := range sortedMountRecords(reg.Mounts) {
		choice := mountPromptChoice{
			Workspace:   rec.Workspace,
			WorkspaceID: rec.WorkspaceID,
			DatabaseID:  rec.ControlPlaneDatabase,
			Path:        rec.LocalPath,
			Mounted:     true,
		}
		if summary, ok := workspaceSummaryForMountRecord(rec, summariesByID, summariesByName); ok {
			choice.Workspace = summary.Name
			choice.WorkspaceID = summary.ID
			choice.DatabaseID = summary.DatabaseID
			choice.Database = summary.DatabaseName
		}
		choices = append(choices, choice)
		if id := strings.TrimSpace(rec.WorkspaceID); id != "" {
			mountedByID[id] = true
			continue
		}
		name := strings.TrimSpace(rec.Workspace)
		databaseID := strings.TrimSpace(rec.ControlPlaneDatabase)
		if name != "" && databaseID != "" {
			mountedByDatabaseName[mountPromptDatabaseNameKey(databaseID, name)] = true
			continue
		}
		if name != "" {
			mountedByLegacyName[name] = true
		}
	}
	for _, ws := range workspaces {
		if strings.TrimSpace(ws.ID) != "" && mountedByID[ws.ID] {
			continue
		}
		if strings.TrimSpace(ws.DatabaseID) != "" && strings.TrimSpace(ws.Name) != "" && mountedByDatabaseName[mountPromptDatabaseNameKey(ws.DatabaseID, ws.Name)] {
			continue
		}
		if strings.TrimSpace(ws.ID) == "" && strings.TrimSpace(ws.DatabaseID) == "" && strings.TrimSpace(ws.Name) != "" && mountedByLegacyName[ws.Name] {
			continue
		}
		choices = append(choices, mountPromptChoice{
			Workspace:   ws.Name,
			WorkspaceID: ws.ID,
			DatabaseID:  ws.DatabaseID,
			Database:    ws.DatabaseName,
		})
	}
	return choices
}

func workspaceSummaryForMountRecord(rec mountRecord, summariesByID map[string]workspaceSummary, summariesByName map[string][]workspaceSummary) (workspaceSummary, bool) {
	if id := strings.TrimSpace(rec.WorkspaceID); id != "" {
		if summary, ok := summariesByID[id]; ok {
			return summary, true
		}
	}
	name := strings.TrimSpace(rec.Workspace)
	if name == "" {
		return workspaceSummary{}, false
	}
	databaseID := strings.TrimSpace(rec.ControlPlaneDatabase)
	if databaseID != "" {
		for _, summary := range summariesByName[name] {
			if strings.TrimSpace(summary.DatabaseID) == databaseID {
				return summary, true
			}
		}
	}
	matches := summariesByName[name]
	if len(matches) == 1 {
		return matches[0], true
	}
	return workspaceSummary{}, false
}

func mountPromptDatabaseNameKey(databaseID, workspaceName string) string {
	return strings.TrimSpace(databaseID) + "\x00" + strings.TrimSpace(workspaceName)
}

func mountPromptRows(choices []mountPromptChoice) [][]string {
	rows := make([][]string, 0, len(choices))
	for i, choice := range choices {
		status := "available"
		path := ""
		if choice.Mounted {
			status = "mounted"
			path = compactDisplayPath(choice.Path)
		}
		rows = append(rows, []string{
			strconv.Itoa(i + 1),
			choice.Workspace,
			mountPromptWorkspaceID(choice),
			mountPromptDatabase(choice),
			status,
			path,
		})
	}
	return rows
}

func mountPromptWorkspaceID(choice mountPromptChoice) string {
	id := strings.TrimSpace(choice.WorkspaceID)
	if id == "" {
		return "-"
	}
	return id
}

func mountPromptDatabase(choice mountPromptChoice) string {
	return workspaceListDatabase(workspaceSummary{
		DatabaseID:   choice.DatabaseID,
		DatabaseName: choice.Database,
	})
}

func promptMountPathForWorkspace(workspace string) (string, bool, error) {
	if err := validateAFSName("workspace", workspace); err != nil {
		return "", false, err
	}
	defaultPath := "~/" + workspace
	fmt.Println()
	fmt.Printf("Local folder [%s]: ", defaultPath)

	reader := bufio.NewReader(os.Stdin)
	raw, err := reader.ReadString('\n')
	if err != nil && strings.TrimSpace(raw) == "" {
		fmt.Println()
		fmt.Println("Mount cancelled.")
		fmt.Println()
		return "", false, nil
	}
	localPath := strings.TrimSpace(raw)
	if localPath == "" {
		localPath = defaultPath
	}
	return localPath, true, nil
}

func unmountWorkspacePath(rawPath string, deleteLocal bool) error {
	localPath, err := normalizeMountPath(rawPath)
	if err != nil {
		return err
	}
	reg, err := loadMountRegistry()
	if err != nil {
		return err
	}
	rec, ok := removeMountByPath(&reg, localPath)
	if !ok {
		return fmt.Errorf("no mount found at %s", localPath)
	}
	if err := stopMount(rec, deleteLocal); err != nil {
		return err
	}
	if err := saveMountRegistry(reg); err != nil {
		return err
	}
	printUnmountResult(rec, deleteLocal)
	return nil
}

func unmountWorkspaceTarget(rawTarget string, deleteLocal bool) error {
	target := strings.TrimSpace(rawTarget)
	if target == "" {
		return errors.New("unmount requires a workspace or directory")
	}

	reg, err := loadMountRegistry()
	if err != nil {
		return err
	}

	if unmountTargetLooksLikePath(target) {
		localPath, err := normalizeMountPath(target)
		if err != nil {
			return err
		}
		if rec, ok := removeMountByPath(&reg, localPath); ok {
			return unmountMountRecord(reg, rec, deleteLocal)
		}
		rec, ok, err := removeMountByWorkspaceRef(&reg, target)
		if err != nil {
			return err
		}
		if ok {
			return unmountMountRecord(reg, rec, deleteLocal)
		}
		return fmt.Errorf("no mount found at %s", localPath)
	}

	rec, ok, err := removeMountByWorkspaceRef(&reg, target)
	if err != nil {
		return err
	}
	if ok {
		return unmountMountRecord(reg, rec, deleteLocal)
	}
	localPath, err := normalizeMountPath(target)
	if err != nil {
		return err
	}
	if rec, ok := removeMountByPath(&reg, localPath); ok {
		return unmountMountRecord(reg, rec, deleteLocal)
	}
	return fmt.Errorf("no mount found for workspace %s", target)
}

func unmountMountRecord(reg mountRegistry, rec mountRecord, deleteLocal bool) error {
	if err := stopMount(rec, deleteLocal); err != nil {
		return err
	}
	if err := saveMountRegistry(reg); err != nil {
		return err
	}
	printUnmountResult(rec, deleteLocal)
	return nil
}

func unmountTargetLooksLikePath(target string) bool {
	if filepath.IsAbs(target) {
		return true
	}
	if strings.HasPrefix(target, "~") || strings.HasPrefix(target, ".") {
		return true
	}
	return strings.ContainsRune(target, os.PathSeparator)
}

func promptUnmountSelection(deleteLocal bool) error {
	reg, err := loadMountRegistry()
	if err != nil {
		return err
	}
	if len(reg.Mounts) == 0 {
		fmt.Println()
		fmt.Println("No mounted workspaces.")
		fmt.Println()
		return nil
	}

	records := sortedMountRecords(reg.Mounts)
	fmt.Println()
	fmt.Println("Unmount workspace")
	fmt.Println()
	printPlainTable([]string{"#", "Workspace", "Path"}, unmountPromptRows(records))
	fmt.Println()
	fmt.Print("Workspace to unmount: ")

	reader := bufio.NewReader(os.Stdin)
	raw, err := reader.ReadString('\n')
	if err != nil && strings.TrimSpace(raw) == "" {
		fmt.Println()
		fmt.Println("Unmount cancelled.")
		fmt.Println()
		return nil
	}
	choice := strings.TrimSpace(raw)
	if choice == "" {
		fmt.Println("Unmount cancelled.")
		fmt.Println()
		return nil
	}
	fmt.Println()
	idx, err := strconv.Atoi(choice)
	if err != nil || idx < 1 || idx > len(records) {
		return fmt.Errorf("invalid selection %q", choice)
	}

	selected := records[idx-1]
	reg, err = loadMountRegistry()
	if err != nil {
		return err
	}
	rec, ok := removeMountByPath(&reg, selected.LocalPath)
	if !ok {
		return fmt.Errorf("mount for %s is no longer mounted", selected.Workspace)
	}
	return unmountMountRecord(reg, rec, deleteLocal)
}

func unmountPromptRows(records []mountRecord) [][]string {
	rows := make([][]string, 0, len(records))
	for i, rec := range records {
		rows = append(rows, []string{strconv.Itoa(i + 1), rec.Workspace, compactDisplayPath(rec.LocalPath)})
	}
	return rows
}

func stopMount(rec mountRecord, deleteLocal bool) error {
	if strings.TrimSpace(rec.Mode) == modeMount {
		cfg := configFromMount(rec)
		backend, _, err := backendForConfig(cfg)
		if err != nil {
			return err
		}
		if localPath := strings.TrimSpace(rec.LocalPath); localPath != "" && backend.IsMounted(localPath) {
			if err := backend.Unmount(localPath); err != nil {
				return err
			}
		}
	}
	if rec.PID > 0 && processAlive(rec.PID) {
		if err := terminatePID(rec.PID, 5*time.Second); err != nil {
			return err
		}
	}
	closeManagedWorkspaceSession(configFromMount(rec), strings.TrimSpace(rec.Workspace), strings.TrimSpace(rec.SessionID))
	if deleteLocal {
		if localPath := strings.TrimSpace(rec.LocalPath); localPath != "" {
			if err := os.RemoveAll(localPath); err != nil {
				return err
			}
		}
		if workspace := strings.TrimSpace(rec.Workspace); workspace != "" {
			_ = removeSyncState(workspace)
		}
	}
	return removeLegacyStateForMount(rec)
}

func removeLegacyStateForMount(rec mountRecord) error {
	st, err := loadState()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if filepath.Clean(st.LocalPath) != filepath.Clean(rec.LocalPath) {
		return nil
	}
	if strings.TrimSpace(st.CurrentWorkspace) != "" && strings.TrimSpace(st.CurrentWorkspace) != strings.TrimSpace(rec.Workspace) {
		return nil
	}
	if err := os.Remove(statePath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func configFromMount(rec mountRecord) config {
	cfg := loadConfigOrDefault()
	cfg.ProductMode = rec.ProductMode
	cfg.URL = rec.ControlPlaneURL
	cfg.DatabaseID = rec.ControlPlaneDatabase
	cfg.RedisAddr = rec.RedisAddr
	cfg.RedisDB = rec.RedisDB
	cfg.CurrentWorkspace = rec.Workspace
	cfg.CurrentWorkspaceID = rec.WorkspaceID
	cfg.LocalPath = rec.LocalPath
	cfg.Mode = rec.Mode
	if strings.TrimSpace(rec.MountBackend) != "" {
		cfg.MountBackend = rec.MountBackend
	}
	cfg.SyncLog = rec.SyncLog
	return cfg
}

func printUnmountResult(rec mountRecord, deleteLocal bool) {
	local := "preserved"
	if deleteLocal {
		local = "deleted"
	}
	printSection("Workspace unmounted", []outputRow{
		{Label: "workspace", Value: rec.Workspace},
		{Label: "path", Value: compactDisplayPath(rec.LocalPath)},
		{Label: "local", Value: local},
	})
}

func countMountableLocalEntries(root string) (int, error) {
	info, err := os.Stat(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	if !info.IsDir() {
		return 0, fmt.Errorf("%s is not a directory", root)
	}
	count := 0
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		name := d.Name()
		if name == syncControlDirName || name == ".DS_Store" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		count++
		return nil
	})
	return count, err
}

func mountUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s ws mount [--dry-run] [--yes] [--readonly] [--verbose] [--session <name>] [<workspace> [directory]]

Mount a workspace to a local directory using sync mode.
With no workspace, lists workspaces and prompts for a selection.
With no directory, prompts for a local folder.
Use --readonly to make this mount refuse local writes.
Use --session to name this mount session separately from agent.name.
When mounting to a populated local folder, AFS shows the safe reconciliation
plan and asks before uploading or downloading files. Use --yes to accept a
safe plan non-interactively; conflicts still block mount.
The directory is preserved on unmount unless --delete is used.
`, bin)
}

func unmountUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s ws unmount [--delete] [<workspace|directory>]

Unmount an AFS workspace by workspace name, workspace ID, or local directory.
With no target, lists mounted workspaces and prompts for a selection.
By default, local files are preserved. Use --delete only when you want to
remove the mounted local directory after the daemon stops.
`, bin)
}
