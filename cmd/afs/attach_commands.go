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

const attachShellCDFileEnv = "AFS_ATTACH_CD_FILE"

type attachOptions struct {
	workspace string
	directory string
	verbose   bool
	dryRun    bool
	yes       bool
}

type detachOptions struct {
	target      string
	deleteLocal bool
}

func cmdAttachArgs(args []string) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		fmt.Fprint(os.Stderr, attachUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	opts, err := parseAttachOptions(args)
	if err != nil {
		return err
	}
	if strings.TrimSpace(opts.workspace) == "" {
		return promptAttachSelection(opts)
	}
	if strings.TrimSpace(opts.directory) == "" {
		directory, ok, err := promptAttachPathForWorkspace(opts.workspace)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		opts.directory = directory
	}
	return attachWorkspace(opts)
}

func cmdDetachArgs(args []string) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		fmt.Fprint(os.Stderr, detachUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	opts, err := parseDetachOptions(args, false)
	if err != nil {
		return err
	}
	if strings.TrimSpace(opts.target) == "" {
		return promptDetachSelection(opts.deleteLocal)
	}
	return detachWorkspaceTarget(opts.target, opts.deleteLocal)
}

func parseAttachOptions(args []string) (attachOptions, error) {
	var opts attachOptions
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
		default:
			if strings.HasPrefix(arg, "-") {
				return opts, fmt.Errorf("unknown flag %q\n\n%s", arg, attachUsageText(filepath.Base(os.Args[0])))
			}
			positionals = append(positionals, arg)
		}
	}
	if len(positionals) > 2 {
		return opts, fmt.Errorf("%s", attachUsageText(filepath.Base(os.Args[0])))
	}
	if len(positionals) >= 1 {
		opts.workspace = positionals[0]
	}
	if len(positionals) == 2 {
		opts.directory = positionals[1]
	}
	return opts, nil
}

func parseDetachOptions(args []string, requirePath bool) (detachOptions, error) {
	var opts detachOptions
	var positionals []string
	for _, arg := range args {
		switch arg {
		case "--delete":
			opts.deleteLocal = true
		default:
			if strings.HasPrefix(arg, "-") {
				return opts, fmt.Errorf("unknown flag %q\n\n%s", arg, detachUsageText(filepath.Base(os.Args[0])))
			}
			positionals = append(positionals, arg)
		}
	}
	if len(positionals) > 1 || (requirePath && len(positionals) != 1) {
		return opts, fmt.Errorf("%s", detachUsageText(filepath.Base(os.Args[0])))
	}
	if len(positionals) == 1 {
		opts.target = positionals[0]
	}
	return opts, nil
}

func attachWorkspace(opts attachOptions) error {
	if err := validateAFSName("workspace", opts.workspace); err != nil {
		return err
	}
	localPath, err := normalizeAttachmentPath(opts.directory)
	if err != nil {
		return err
	}

	reg, err := loadAttachmentRegistry()
	if err != nil {
		return err
	}
	if conflict, ok := attachmentPathConflict(reg, localPath); ok {
		return fmt.Errorf("path %s overlaps existing attachment %s at %s", localPath, conflict.Workspace, conflict.LocalPath)
	}

	cfg, err := loadAFSConfig()
	if err != nil {
		return err
	}
	cfg.Mode = modeSync
	cfg.MountBackend = mountBackendNone
	cfg.CurrentWorkspace = opts.workspace
	cfg.CurrentWorkspaceID = ""
	cfg.LocalPath = localPath

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
	if conflict, ok := attachmentWorkspaceConflict(reg, selection.ID, selection.Name); ok {
		return fmt.Errorf("workspace %s is already attached at %s", conflict.Workspace, conflict.LocalPath)
	}

	resolvedCfg.CurrentWorkspace = selection.Name
	resolvedCfg.CurrentWorkspaceID = selection.ID
	resolvedCfg.LocalPath = localPath
	resolvedCfg.Mode = modeSync
	resolvedCfg.MountBackend = mountBackendNone
	return startSyncAttachment(ctx, resolvedCfg, selection, opts)
}

func startSyncAttachment(ctx context.Context, cfg config, selection workspaceSelection, opts attachOptions) error {
	if strings.TrimSpace(cfg.LocalPath) == "" {
		return errors.New("attach requires a local directory")
	}
	localRoot, err := normalizeAttachmentPath(cfg.LocalPath)
	if err != nil {
		return err
	}
	cfg.LocalPath = localRoot
	if err := validateSyncLocalPath(cfg, localRoot); err != nil {
		return err
	}

	if opts.verbose {
		fmt.Printf("opening workspace session: %s\n", selection.Name)
	}
	requested := selection.ID
	if strings.TrimSpace(requested) == "" {
		requested = selection.Name
	}
	bootstrap, closeSession, err := prepareSyncBootstrapForWorkspace(ctx, cfg, requested)
	if err != nil {
		return err
	}
	defer closeSession()
	runtimeCfg := bootstrap.cfg
	runtimeCfg.CurrentWorkspace = selection.Name
	runtimeCfg.CurrentWorkspaceID = selection.ID
	runtimeCfg.LocalPath = localRoot
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
	plan, err := buildAttachReconcilePlan(ctx, daemon)
	if err != nil {
		closeManagedWorkspaceSession(runtimeCfg, bootstrap.workspace, bootstrap.sessionID)
		return err
	}
	if opts.dryRun {
		printAttachReconcilePlan("Would attach workspace", selection.Name, localRoot, plan, true)
		return nil
	}
	if plan.hasReportableChanges() {
		printAttachReconcilePlan("Attach changes", selection.Name, localRoot, plan, opts.verbose || plan.ConflictCount > 0)
	}
	if plan.ConflictCount > 0 {
		closeManagedWorkspaceSession(runtimeCfg, bootstrap.workspace, bootstrap.sessionID)
		return fmt.Errorf("attach has %d conflict(s); resolve them or move conflicting files aside before attaching", plan.ConflictCount)
	}
	if plan.requiresConfirmation() && !opts.yes {
		if !isInteractiveTerminal() {
			closeManagedWorkspaceSession(runtimeCfg, bootstrap.workspace, bootstrap.sessionID)
			return errors.New("attach would change an existing local folder; rerun in an interactive terminal, pass --dry-run to inspect the plan, or pass --yes to accept the safe attach plan")
		}
		ok, err := promptYesNo(bufio.NewReader(os.Stdin), os.Stdout, "Continue with attach sync plan?", false)
		if err != nil {
			closeManagedWorkspaceSession(runtimeCfg, bootstrap.workspace, bootstrap.sessionID)
			return err
		}
		if !ok {
			fmt.Println("Attach cancelled.")
			closeManagedWorkspaceSession(runtimeCfg, bootstrap.workspace, bootstrap.sessionID)
			return nil
		}
	}
	approveAttachReconcilePlan(daemon, plan)
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

	reg, err := loadAttachmentRegistry()
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
	upsertAttachment(&reg, attachmentRecord{
		ID:                   "att_" + id,
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
	if err := saveAttachmentRegistry(reg); err != nil {
		_ = terminatePID(daemonPID, 5*time.Second)
		closeManagedWorkspaceSession(runtimeCfg, bootstrap.workspace, bootstrap.sessionID)
		return err
	}
	recordAttachShellDirectory(localRoot)

	entryCount := "synced"
	if snap, err := loadSyncState(bootstrap.workspace); err == nil && snap != nil {
		entryCount = strconv.Itoa(len(snap.Entries)) + " entries"
	}
	rows := []outputRow{
		{Label: "workspace", Value: bootstrap.workspace},
		{Label: "path", Value: compactDisplayPath(localRoot)},
		{Label: "mode", Value: "sync"},
		{Label: "files", Value: entryCount},
		{Label: "detach", Value: filepath.Base(os.Args[0]) + " ws detach " + shellQuote(bootstrap.workspace)},
	}
	if opts.verbose && strings.TrimSpace(bootstrap.sessionID) != "" {
		rows = append(rows, outputRow{Label: "session", Value: strings.TrimSpace(bootstrap.sessionID)})
	}
	printSection("Workspace attached", rows)
	return nil
}

func recordAttachShellDirectory(localRoot string) {
	target := strings.TrimSpace(os.Getenv(attachShellCDFileEnv))
	if target == "" {
		return
	}
	_ = os.WriteFile(target, []byte(localRoot+"\n"), 0o600)
}

type attachPromptChoice struct {
	Workspace   string
	WorkspaceID string
	Path        string
	Attached    bool
}

func promptAttachSelection(opts attachOptions) error {
	_, service, closeStore, err := openAFSControlPlane(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	workspaces, err := service.ListWorkspaceSummaries(context.Background())
	if err != nil {
		return err
	}
	reg, err := loadAttachmentRegistry()
	if err != nil {
		return err
	}

	choices := attachPromptChoices(reg, workspaces.Items)
	if len(choices) == 0 {
		fmt.Println()
		fmt.Println("Attach workspace")
		fmt.Println()
		fmt.Println("No workspaces found.")
		fmt.Println("Create one with: " + filepath.Base(os.Args[0]) + " ws create <workspace>")
		fmt.Println()
		return nil
	}

	fmt.Println()
	fmt.Println("Attach workspace")
	fmt.Println()
	printPlainTable([]string{"#", "Workspace", "Status", "Path"}, attachPromptRows(choices))
	fmt.Println()
	fmt.Print("Workspace to attach: ")

	reader := bufio.NewReader(os.Stdin)
	raw, err := reader.ReadString('\n')
	if err != nil && strings.TrimSpace(raw) == "" {
		fmt.Println()
		fmt.Println("Attach cancelled.")
		fmt.Println()
		return nil
	}
	choiceText := strings.TrimSpace(raw)
	if choiceText == "" {
		fmt.Println("Attach cancelled.")
		fmt.Println()
		return nil
	}
	idx, err := strconv.Atoi(choiceText)
	if err != nil || idx < 1 || idx > len(choices) {
		return fmt.Errorf("invalid selection %q", choiceText)
	}
	selected := choices[idx-1]
	if selected.Attached {
		printSection("Workspace already attached", []outputRow{
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
		fmt.Println("Attach cancelled.")
		fmt.Println()
		return nil
	}
	localPath := strings.TrimSpace(rawPath)
	if localPath == "" {
		localPath = defaultPath
	}
	opts.workspace = selected.Workspace
	opts.directory = localPath
	return attachWorkspace(opts)
}

func attachPromptChoices(reg attachmentRegistry, workspaces []workspaceSummary) []attachPromptChoice {
	choices := make([]attachPromptChoice, 0, len(reg.Attachments)+len(workspaces))
	attachedByID := make(map[string]bool, len(reg.Attachments))
	attachedByName := make(map[string]bool, len(reg.Attachments))
	for _, rec := range sortedAttachmentRecords(reg.Attachments) {
		choices = append(choices, attachPromptChoice{
			Workspace:   rec.Workspace,
			WorkspaceID: rec.WorkspaceID,
			Path:        rec.LocalPath,
			Attached:    true,
		})
		if id := strings.TrimSpace(rec.WorkspaceID); id != "" {
			attachedByID[id] = true
		}
		if name := strings.TrimSpace(rec.Workspace); name != "" {
			attachedByName[name] = true
		}
	}
	for _, ws := range workspaces {
		if strings.TrimSpace(ws.ID) != "" && attachedByID[ws.ID] {
			continue
		}
		if strings.TrimSpace(ws.Name) != "" && attachedByName[ws.Name] {
			continue
		}
		choices = append(choices, attachPromptChoice{
			Workspace:   ws.Name,
			WorkspaceID: ws.ID,
		})
	}
	return choices
}

func attachPromptRows(choices []attachPromptChoice) [][]string {
	rows := make([][]string, 0, len(choices))
	for i, choice := range choices {
		status := "available"
		path := ""
		if choice.Attached {
			status = "attached"
			path = compactDisplayPath(choice.Path)
		}
		rows = append(rows, []string{strconv.Itoa(i + 1), choice.Workspace, status, path})
	}
	return rows
}

func promptAttachPathForWorkspace(workspace string) (string, bool, error) {
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
		fmt.Println("Attach cancelled.")
		fmt.Println()
		return "", false, nil
	}
	localPath := strings.TrimSpace(raw)
	if localPath == "" {
		localPath = defaultPath
	}
	return localPath, true, nil
}

func detachWorkspacePath(rawPath string, deleteLocal bool) error {
	localPath, err := normalizeAttachmentPath(rawPath)
	if err != nil {
		return err
	}
	reg, err := loadAttachmentRegistry()
	if err != nil {
		return err
	}
	rec, ok := removeAttachmentByPath(&reg, localPath)
	if !ok {
		return fmt.Errorf("no attachment found at %s", localPath)
	}
	if err := stopAttachment(rec, deleteLocal); err != nil {
		return err
	}
	if err := saveAttachmentRegistry(reg); err != nil {
		return err
	}
	printDetachResult(rec, deleteLocal)
	return nil
}

func detachWorkspaceTarget(rawTarget string, deleteLocal bool) error {
	target := strings.TrimSpace(rawTarget)
	if target == "" {
		return errors.New("detach requires a workspace or directory")
	}

	reg, err := loadAttachmentRegistry()
	if err != nil {
		return err
	}

	if detachTargetLooksLikePath(target) {
		localPath, err := normalizeAttachmentPath(target)
		if err != nil {
			return err
		}
		if rec, ok := removeAttachmentByPath(&reg, localPath); ok {
			return detachAttachmentRecord(reg, rec, deleteLocal)
		}
		rec, ok, err := removeAttachmentByWorkspaceRef(&reg, target)
		if err != nil {
			return err
		}
		if ok {
			return detachAttachmentRecord(reg, rec, deleteLocal)
		}
		return fmt.Errorf("no attachment found at %s", localPath)
	}

	rec, ok, err := removeAttachmentByWorkspaceRef(&reg, target)
	if err != nil {
		return err
	}
	if ok {
		return detachAttachmentRecord(reg, rec, deleteLocal)
	}
	localPath, err := normalizeAttachmentPath(target)
	if err != nil {
		return err
	}
	if rec, ok := removeAttachmentByPath(&reg, localPath); ok {
		return detachAttachmentRecord(reg, rec, deleteLocal)
	}
	return fmt.Errorf("no attachment found for workspace %s", target)
}

func detachAttachmentRecord(reg attachmentRegistry, rec attachmentRecord, deleteLocal bool) error {
	if err := stopAttachment(rec, deleteLocal); err != nil {
		return err
	}
	if err := saveAttachmentRegistry(reg); err != nil {
		return err
	}
	printDetachResult(rec, deleteLocal)
	return nil
}

func detachTargetLooksLikePath(target string) bool {
	if filepath.IsAbs(target) {
		return true
	}
	if strings.HasPrefix(target, "~") || strings.HasPrefix(target, ".") {
		return true
	}
	return strings.ContainsRune(target, os.PathSeparator)
}

func promptDetachSelection(deleteLocal bool) error {
	reg, err := loadAttachmentRegistry()
	if err != nil {
		return err
	}
	if len(reg.Attachments) == 0 {
		fmt.Println()
		fmt.Println("No attached workspaces.")
		fmt.Println()
		return nil
	}

	records := sortedAttachmentRecords(reg.Attachments)
	fmt.Println()
	fmt.Println("Detach workspace")
	fmt.Println()
	printPlainTable([]string{"#", "Workspace", "Path"}, detachPromptRows(records))
	fmt.Println()
	fmt.Print("Workspace to detach: ")

	reader := bufio.NewReader(os.Stdin)
	raw, err := reader.ReadString('\n')
	if err != nil && strings.TrimSpace(raw) == "" {
		fmt.Println()
		fmt.Println("Detach cancelled.")
		fmt.Println()
		return nil
	}
	choice := strings.TrimSpace(raw)
	if choice == "" {
		fmt.Println("Detach cancelled.")
		fmt.Println()
		return nil
	}
	fmt.Println()
	idx, err := strconv.Atoi(choice)
	if err != nil || idx < 1 || idx > len(records) {
		return fmt.Errorf("invalid selection %q", choice)
	}

	selected := records[idx-1]
	reg, err = loadAttachmentRegistry()
	if err != nil {
		return err
	}
	rec, ok := removeAttachmentByPath(&reg, selected.LocalPath)
	if !ok {
		return fmt.Errorf("attachment for %s is no longer attached", selected.Workspace)
	}
	return detachAttachmentRecord(reg, rec, deleteLocal)
}

func detachPromptRows(records []attachmentRecord) [][]string {
	rows := make([][]string, 0, len(records))
	for i, rec := range records {
		rows = append(rows, []string{strconv.Itoa(i + 1), rec.Workspace, compactDisplayPath(rec.LocalPath)})
	}
	return rows
}

func stopAttachment(rec attachmentRecord, deleteLocal bool) error {
	if rec.PID > 0 && processAlive(rec.PID) {
		if err := terminatePID(rec.PID, 5*time.Second); err != nil {
			return err
		}
	}
	closeManagedWorkspaceSession(configFromAttachment(rec), strings.TrimSpace(rec.Workspace), strings.TrimSpace(rec.SessionID))
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
	return removeLegacyStateForAttachment(rec)
}

func removeLegacyStateForAttachment(rec attachmentRecord) error {
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

func configFromAttachment(rec attachmentRecord) config {
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
	cfg.SyncLog = rec.SyncLog
	return cfg
}

func printDetachResult(rec attachmentRecord, deleteLocal bool) {
	local := "preserved"
	if deleteLocal {
		local = "deleted"
	}
	printSection("Workspace detached", []outputRow{
		{Label: "workspace", Value: rec.Workspace},
		{Label: "path", Value: compactDisplayPath(rec.LocalPath)},
		{Label: "local", Value: local},
	})
}

func countAttachableLocalEntries(root string) (int, error) {
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

func attachUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s ws attach [--dry-run] [--yes] [--verbose] [<workspace> [directory]]

Attach a workspace to a local directory using sync mode.
With no workspace, lists workspaces and prompts for a selection.
With no directory, prompts for a local folder.
When attaching to a populated local folder, AFS shows the safe reconciliation
plan and asks before uploading or downloading files. Use --yes to accept a
safe plan non-interactively; conflicts still block attach.
The directory is preserved on detach unless --delete is used.
`, bin)
}

func detachUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s ws detach [--delete] [<workspace|directory>]

Detach an AFS workspace by workspace name, workspace ID, or local directory.
With no target, lists attached workspaces and prompts for a selection.
By default, local files are preserved. Use --delete only when you want to
remove the attached local directory after the daemon stops.
`, bin)
}
