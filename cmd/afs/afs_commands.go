package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/agent-filesystem/internal/controlplane"
	"github.com/redis/agent-filesystem/internal/worktree"
	"github.com/redis/go-redis/v9"
)

var errImportCancelled = errors.New("import cancelled")

// importBlobSink is a worktree.BlobSink that pipelines blobs to Redis via a
// BlobWriter and retains each blob body in an in-memory map so the immediately
// following SyncWorkspaceRoot can fetch them without re-reading Redis. Entries
// are dropped after the sync step, keeping peak RAM bounded to
// source_size + (workers × read_buffer) during the build, plus one more pass
// during sync.
type importBlobSink struct {
	mu     sync.Mutex
	ctx    context.Context
	writer *controlplane.BlobWriter
	cache  map[string][]byte
}

func newImportBlobSink(ctx context.Context, writer *controlplane.BlobWriter) *importBlobSink {
	return &importBlobSink{
		ctx:    ctx,
		writer: writer,
		cache:  make(map[string][]byte),
	}
}

func (s *importBlobSink) Submit(blobID string, data []byte, size int64) error {
	s.mu.Lock()
	if _, ok := s.cache[blobID]; ok {
		s.mu.Unlock()
		return nil
	}
	// Share the byte slice with the writer and the cache; BlobWriter does not
	// mutate the buffer.
	s.cache[blobID] = data
	s.mu.Unlock()
	return s.writer.Submit(s.ctx, blobID, data, size)
}

func (s *importBlobSink) Get(blobID string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.cache[blobID]
	return data, ok
}

func (s *importBlobSink) Drop() {
	s.mu.Lock()
	s.cache = nil
	s.mu.Unlock()
}

func cmdImport(args []string) error {
	parsed, err := parseAFSArgs(args[1:], true, false)
	if err != nil {
		return err
	}
	if len(parsed.positionals) != 2 {
		return fmt.Errorf("usage: %s import [--force] [--database <database-id|database-name>] <workspace> <directory>", filepath.Base(os.Args[0]))
	}

	workspace := parsed.positionals[0]
	if err := validateAFSName("workspace", workspace); err != nil {
		return err
	}

	sourceDir, err := expandPath(parsed.positionals[1])
	if err != nil {
		return err
	}
	info, err := os.Stat(sourceDir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", sourceDir)
	}

	ctx := context.Background()
	cfg, err := loadAFSConfig()
	if err != nil {
		return err
	}
	productMode, err := effectiveProductMode(cfg)
	if err != nil {
		return err
	}

	switch productMode {
	case productModeLocal:
		if strings.TrimSpace(parsed.database) != "" {
			return fmt.Errorf("--database is only supported in control plane mode")
		}
		return cmdImportDirect(ctx, cfg, workspace, sourceDir, parsed.force)
	case productModeSelfHosted:
		return cmdImportSelfHosted(ctx, cfg, workspace, sourceDir, parsed.force, parsed.database)
	default:
		_, _, _, err := openAFSControlPlaneForConfig(ctx, cfg)
		return err
	}
}

func cmdImportDirect(ctx context.Context, cfg config, workspace, sourceDir string, replaceExisting bool) error {
	cfg, store, closeStore, err := openAFSStore(ctx)
	if err != nil {
		return err
	}
	defer closeStore()

	exists, err := store.workspaceExists(ctx, workspace)
	if err != nil {
		return err
	}
	if exists && !replaceExisting {
		return fmt.Errorf("workspace %q already exists; rerun with --force to replace it", workspace)
	}

	lock, err := store.acquireImportLock(ctx, workspace)
	if err != nil {
		if errors.Is(err, controlplane.ErrImportInProgress) {
			return fmt.Errorf("another import is already running for workspace %q; wait for it to finish or clear the stale lock", workspace)
		}
		return fmt.Errorf("acquire import lock: %w", err)
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = lock.Release(releaseCtx)
	}()

	const initialSavepoint = "initial"
	total, ignorer, scanDuration, err := prepareAFSImport(sourceDir, workspace, cfg, replaceExisting)
	if err != nil {
		if errors.Is(err, errImportCancelled) {
			fmt.Println()
			fmt.Println("  Import cancelled.")
			fmt.Println()
			return nil
		}
		return err
	}

	if replaceExisting {
		step := startStep("Replacing existing workspace")
		if err := store.deleteWorkspace(ctx, workspace); err != nil {
			step.fail(err.Error())
			return err
		}
		if err := removeLocalWorkspace(cfg, workspace); err != nil {
			step.fail(err.Error())
			return err
		}
		step.succeed(workspace)
	}

	now := time.Now().UTC()
	defaultMeta := controlplane.ApplyWorkspaceMetaDefaults(controlPlaneConfigFromCLI(cfg), workspaceMeta{Name: workspace})

	writer := store.newBlobWriter(workspace, now)
	sink := newImportBlobSink(ctx, writer)

	step := startStep("Building manifest")
	manifest, stats, err := buildManifestStreaming(sourceDir, workspace, initialSavepoint, ignorer, sink, func(progress importStats) {
		step.update(formatAFSImportProgressLabel("Building manifest", progress, total, step.elapsed()))
	})
	if err != nil {
		step.fail(err.Error())
		return err
	}
	if flushErr := writer.Flush(ctx); flushErr != nil {
		step.fail(flushErr.Error())
		return flushErr
	}
	if lockErr := lock.Lost(); lockErr != nil {
		step.fail(lockErr.Error())
		return lockErr
	}
	buildDuration := step.elapsed()
	blobCount, blobBytes := writer.Totals()
	if blobCount == 0 {
		step.succeed(formatAFSImportSummary(total) + " · all files inlined")
	} else {
		step.succeed(fmt.Sprintf("%s · %d blobs, %s pipelined", formatAFSImportSummary(total), blobCount, formatBytes(blobBytes)))
	}

	manifestHash, err := hashManifest(manifest)
	if err != nil {
		return err
	}

	workspaceMeta := workspaceMeta{
		Version:          afsFormatVersion,
		Name:             workspace,
		Description:      fmt.Sprintf("Imported from %s.", sourceDir),
		DatabaseID:       defaultMeta.DatabaseID,
		DatabaseName:     defaultMeta.DatabaseName,
		CloudAccount:     defaultMeta.CloudAccount,
		Region:           defaultMeta.Region,
		Source:           controlplane.SourceGitImport,
		Tags:             controlplane.WorkspaceTags("", controlplane.SourceGitImport),
		CreatedAt:        now,
		UpdatedAt:        now,
		HeadSavepoint:    initialSavepoint,
		DefaultSavepoint: initialSavepoint,
	}
	savepointMeta := savepointMeta{
		Version:      afsFormatVersion,
		ID:           initialSavepoint,
		Name:         initialSavepoint,
		Description:  "Initial import snapshot.",
		Author:       "afs",
		Workspace:    workspace,
		ManifestHash: manifestHash,
		CreatedAt:    now,
		FileCount:    stats.FileCount,
		DirCount:     stats.DirCount,
		TotalBytes:   stats.TotalBytes,
	}

	step = startStep("Writing workspace metadata")
	if err := store.putWorkspaceMeta(ctx, workspaceMeta); err != nil {
		step.fail(err.Error())
		return err
	}
	if err := store.putSavepoint(ctx, savepointMeta, manifest); err != nil {
		step.fail(err.Error())
		return err
	}
	metadataDuration := step.elapsed()
	step.succeed(initialSavepoint)

	step = startStep("Initializing live workspace")
	syncOpts := controlplane.SyncOptions{
		BlobProvider:       sink.Get,
		SkipNamespaceReset: true,
	}
	if err := store.syncWorkspaceRootWithOptions(ctx, workspace, manifest, syncOpts); err != nil {
		step.fail(err.Error())
		return err
	}
	rootDuration := step.elapsed()
	step.succeed("ready for afs up")

	// Drop the blob cache now that sync has consumed it.
	sink.Drop()

	if err := store.audit(ctx, workspace, "import", map[string]any{
		"savepoint":   initialSavepoint,
		"files":       stats.FileCount,
		"dirs":        stats.DirCount,
		"total_bytes": stats.TotalBytes,
		"source":      sourceDir,
	}); err != nil {
		return err
	}

	printBox(markerSuccess+" "+clr(ansiBold, "workspace imported"), []boxRow{
		{Label: "workspace", Value: workspace},
		{Label: "checkpoint", Value: initialSavepoint},
		{Label: "database", Value: configRemoteLabel(cfg)},
		{Label: "files", Value: strconv.Itoa(stats.FileCount)},
		{Label: "dirs", Value: strconv.Itoa(stats.DirCount)},
		{Label: "symlinks", Value: strconv.Itoa(total.Symlinks)},
		{Label: "ignored", Value: strconv.Itoa(total.Ignored)},
		{Label: "bytes", Value: formatBytes(stats.TotalBytes)},
		{Label: "workers", Value: strconv.Itoa(resolveImportWorkers())},
		{Label: "import time", Value: formatStepDuration(scanDuration + buildDuration + metadataDuration + rootDuration)},
		{Label: "next", Value: filepath.Base(os.Args[0]) + " up " + workspace + " " + sourceDir},
	})
	return nil
}

func cmdImportSelfHosted(ctx context.Context, cfg config, workspace, sourceDir string, replaceExisting bool, explicitDatabase string) error {
	client, _, err := newHTTPControlPlaneClient(ctx, cfg)
	if err != nil {
		return err
	}
	database, err := resolveManagedDatabaseForWrite(ctx, cfg, client, explicitDatabase, "workspace import")
	if err != nil {
		return err
	}
	cfg.DatabaseID = database.ID

	cfg, service, closeControlPlane, err := openAFSControlPlaneForConfig(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeControlPlane()

	exists := false
	_, err = service.GetWorkspace(ctx, workspace)
	switch {
	case err == nil:
		exists = true
		if !replaceExisting {
			return fmt.Errorf("workspace %q already exists; rerun with --force to replace it", workspace)
		}
	case errors.Is(err, os.ErrNotExist):
		// Importing a new workspace is fine.
	default:
		return err
	}

	const initialSavepoint = "initial"
	total, ignorer, scanDuration, err := prepareAFSImport(sourceDir, workspace, cfg, replaceExisting)
	if err != nil {
		if errors.Is(err, errImportCancelled) {
			fmt.Println()
			fmt.Println("  Import cancelled.")
			fmt.Println()
			return nil
		}
		return err
	}

	if replaceExisting && exists {
		step := startStep("Replacing existing workspace")
		if err := service.DeleteWorkspace(ctx, workspace); err != nil && !errors.Is(err, os.ErrNotExist) {
			step.fail(err.Error())
			return err
		}
		if err := removeLocalWorkspace(cfg, workspace); err != nil {
			step.fail(err.Error())
			return err
		}
		step.succeed(workspace)
	}

	step := startStep("Building manifest")
	manifest, blobs, stats, err := buildManifestFromDirectoryWithOptions(sourceDir, workspace, initialSavepoint, ignorer, func(progress importStats) {
		step.update(formatAFSImportProgressLabel("Building manifest", progress, total, step.elapsed()))
	})
	if err != nil {
		step.fail(err.Error())
		return err
	}
	buildDuration := step.elapsed()
	if blobCount := len(blobs); blobCount == 0 {
		step.succeed(formatAFSImportSummary(total) + " · all files inlined")
	} else {
		step.succeed(fmt.Sprintf("%s · %d blobs prepared", formatAFSImportSummary(total), blobCount))
	}

	step = startStep("Uploading workspace")
	response, err := service.ImportWorkspace(ctx, controlplane.ImportWorkspaceRequest{
		Name:        workspace,
		Description: fmt.Sprintf("Imported from %s.", sourceDir),
		Manifest:    controlPlaneManifestFromAFS(manifest),
		Blobs:       blobs,
		FileCount:   stats.FileCount,
		DirCount:    stats.DirCount,
		TotalBytes:  stats.TotalBytes,
	})
	if err != nil {
		step.fail(err.Error())
		return err
	}
	uploadDuration := step.elapsed()
	step.succeed(response.Workspace.HeadCheckpointID)

	printBox(markerSuccess+" "+clr(ansiBold, "workspace imported"), []boxRow{
		{Label: "workspace", Value: workspace},
		{Label: "checkpoint", Value: response.Workspace.HeadCheckpointID},
		{Label: "database", Value: configRemoteLabel(cfg)},
		{Label: "files", Value: strconv.Itoa(stats.FileCount)},
		{Label: "dirs", Value: strconv.Itoa(stats.DirCount)},
		{Label: "symlinks", Value: strconv.Itoa(total.Symlinks)},
		{Label: "ignored", Value: strconv.Itoa(total.Ignored)},
		{Label: "bytes", Value: formatBytes(stats.TotalBytes)},
		{Label: "workers", Value: strconv.Itoa(resolveImportWorkers())},
		{Label: "import time", Value: formatStepDuration(scanDuration + buildDuration + uploadDuration)},
		{Label: "next", Value: filepath.Base(os.Args[0]) + " up " + workspace + " " + sourceDir},
	})
	return nil
}

// resolveImportWorkers mirrors the logic used inside worktree.BuildManifest so
// the summary box reports the actual worker count.
func resolveImportWorkers() int {
	if raw := strings.TrimSpace(os.Getenv("AFS_IMPORT_WORKERS")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			return n
		}
	}
	// Keep a conservative default display; the real worker count is sourced
	// the same way inside worktree.resolveWorkerCount.
	return worktreeDefaultWorkers()
}

func worktreeDefaultWorkers() int {
	// Exported helper lives in worktree; lazily mirror it here so we don't
	// add another exported surface.
	return worktree.DefaultImportWorkers()
}

func prepareAFSImport(sourceDir, workspace string, cfg config, replaceExisting bool) (importStats, *migrationIgnore, time.Duration, error) {
	reader := bufio.NewReader(os.Stdin)
	interactive := isInteractiveTerminal()

	for {
		ignorer, err := loadMigrationIgnore(sourceDir)
		if err != nil {
			return importStats{}, nil, 0, err
		}

		step := startStep("Scanning source directory")
		total, err := scanDirectory(sourceDir, ignorer)
		if err != nil {
			step.fail(err.Error())
			return importStats{}, nil, 0, err
		}
		scanDuration := step.elapsed()
		step.succeed(formatAFSImportSummary(total))

		if !interactive {
			return total, ignorer, scanDuration, nil
		}

		estimate := estimateAFSImportDuration(total)
		rows := []boxRow{
			{Label: "source", Value: sourceDir},
			{Label: "workspace", Value: workspace},
			{Label: "database", Value: configRemoteLabel(cfg)},
			{Label: "scan", Value: formatAFSImportSummary(total)},
			{Label: "estimate", Value: "~" + formatStepDuration(estimate)},
		}
		if ignorer != nil {
			value := ignorer.path
			if ignorer.legacy {
				value += " (legacy filename)"
			}
			rows = append(rows, boxRow{Label: "ignore", Value: value})
		} else {
			rows = append(rows, boxRow{Label: "ignore", Value: clr(ansiDim, "none")})
		}
		if replaceExisting {
			rows = append(rows, boxRow{Label: "replace", Value: "existing workspace state will be removed"})
		}
		rows = append(rows,
			boxRow{},
			boxRow{Value: clr(ansiDim, "Tip: use .afsignore to skip caches, dependencies, logs, or build output before import.")},
		)
		printBox(clr(ansiBold, "Import plan"), rows)

		editLabel := "  Create or edit .afsignore before importing?"
		if ignorer != nil {
			editLabel = fmt.Sprintf("  Edit %s before importing?", filepath.Base(ignorer.path))
		}
		editIgnore, err := promptYesNo(reader, os.Stdout, editLabel, false)
		if err != nil {
			return importStats{}, nil, 0, err
		}
		if editIgnore {
			ignorePath := filepath.Join(sourceDir, afsIgnoreFilename)
			if ignorer != nil && !ignorer.legacy {
				ignorePath = ignorer.path
			}
			if err := ensureAFSIgnoreTemplate(ignorePath); err != nil {
				return importStats{}, nil, 0, err
			}
			if err := openPathInEditor(ignorePath); err != nil {
				return importStats{}, nil, 0, err
			}
			fmt.Println()
			continue
		}

		ok, err := promptYesNo(reader, os.Stdout, "  Proceed?", false)
		if err != nil {
			return importStats{}, nil, 0, err
		}
		if !ok {
			return importStats{}, nil, 0, errImportCancelled
		}
		fmt.Println()
		return total, ignorer, scanDuration, nil
	}
}

func isInteractiveTerminal() bool {
	stdin, err := os.Stdin.Stat()
	if err != nil || stdin.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	stdout, err := os.Stdout.Stat()
	if err != nil || stdout.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	return true
}

func estimateAFSImportDuration(total importStats) time.Duration {
	const (
		bytesPerSecond = 12 * 1024 * 1024
		fileCost       = 12 * time.Millisecond
		dirCost        = 2 * time.Millisecond
		symlinkCost    = 3 * time.Millisecond
	)

	estimate := (time.Duration(total.Bytes) * time.Second / bytesPerSecond) +
		(time.Duration(total.Files) * fileCost) +
		(time.Duration(total.Dirs) * dirCost) +
		(time.Duration(total.Symlinks) * symlinkCost)
	if estimate < time.Second {
		return time.Second
	}
	return estimate
}

func formatAFSImportSummary(total importStats) string {
	detail := fmt.Sprintf("%d files, %d dirs", total.Files, total.Dirs)
	if total.Symlinks > 0 {
		detail += fmt.Sprintf(", %d symlinks", total.Symlinks)
	}
	if total.Ignored > 0 {
		detail += fmt.Sprintf(", %d ignored", total.Ignored)
	}
	detail += fmt.Sprintf(", %s", formatBytes(total.Bytes))
	return detail
}

func formatAFSImportProgressLabel(phase string, progress, total importStats, elapsed time.Duration) string {
	label := fmt.Sprintf("%s · %d/%d files", phase, progress.Files, total.Files)
	if total.Dirs > 0 {
		label += fmt.Sprintf(", %d/%d dirs", progress.Dirs, total.Dirs)
	}
	if total.Symlinks > 0 {
		label += fmt.Sprintf(", %d/%d symlinks", progress.Symlinks, total.Symlinks)
	}
	if total.Bytes > 0 {
		pct := int((progress.Bytes * 100) / total.Bytes)
		label += fmt.Sprintf(" · %s / %s (%d%%)", formatBytes(progress.Bytes), formatBytes(total.Bytes), pct)
	}
	if elapsed > 0 {
		label += fmt.Sprintf(" · %s elapsed", formatStepDuration(elapsed))
		if progress.Bytes > 0 {
			label += fmt.Sprintf(" · %s", formatMigrationThroughput(progress.Bytes, elapsed))
		}
	}
	if total.Bytes > 0 && progress.Bytes > 0 {
		label += fmt.Sprintf(" · ETA %s", formatMigrationETA(progress.Bytes, total.Bytes, elapsed))
	}
	return label
}

func ensureAFSIgnoreTemplate(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	template := strings.Join([]string{
		"# Ignore paths during afs import and migrate.",
		"# Syntax matches .gitignore.",
		"",
		"# Common examples:",
		"# .git/",
		"# node_modules/",
		"# dist/",
		"# tmp/",
		"# *.log",
		"",
	}, "\n")
	return os.WriteFile(path, []byte(template), 0o644)
}

func openPathInEditor(path string) error {
	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if editor != "" {
		cmd := exec.Command("/bin/sh", "-lc", editor+" "+shellQuote(path))
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	for _, candidate := range []string{"nano", "vi"} {
		lp, err := exec.LookPath(candidate)
		if err != nil {
			continue
		}
		cmd := exec.Command(lp, path)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	return fmt.Errorf("no editor found to edit %s; set $EDITOR or create the file manually", path)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func afsBlobTotals(blobs map[string][]byte) (int, int64) {
	var total int64
	for _, blob := range blobs {
		total += int64(len(blob))
	}
	return len(blobs), total
}

func currentWorkspaceName(ctx context.Context, cfg config, store *afsStore) (string, error) {
	workspace := selectedWorkspaceName(cfg)
	if workspace != "" {
		exists, err := store.workspaceExists(ctx, workspace)
		if err != nil {
			return "", err
		}
		if exists {
			return workspace, nil
		}
		return "", fmt.Errorf("current workspace %q does not exist; run '%s workspace use <workspace>' or pass a workspace explicitly", workspace, filepath.Base(os.Args[0]))
	}
	return "", fmt.Errorf("workspace is required; no current workspace is selected\nRun '%s workspace use <workspace>' or pass a workspace explicitly", filepath.Base(os.Args[0]))
}

type workspaceSelection struct {
	ID   string
	Name string
}

func currentWorkspaceSelectionFromControlPlane(ctx context.Context, cfg config, service afsControlPlane) (workspaceSelection, error) {
	return resolveWorkspaceSelectionFromControlPlane(ctx, cfg, service, "")
}

func resolveWorkspaceName(ctx context.Context, cfg config, store *afsStore, requested string) (string, error) {
	if requested != "" {
		return requested, nil
	}
	return currentWorkspaceName(ctx, cfg, store)
}

func resolveWorkspaceSelectionFromControlPlane(ctx context.Context, cfg config, service afsControlPlane, requested string) (workspaceSelection, error) {
	workspaces, err := service.ListWorkspaceSummaries(ctx)
	if err != nil {
		return workspaceSelection{}, err
	}

	ref := strings.TrimSpace(requested)
	displayName := selectedWorkspaceName(cfg)
	if ref == "" {
		// In managed modes, an explicitly configured workspace ID should win
		// over any inherited runtime state so duplicate-name routing stays
		// stable across bootstrap and reconnect flows.
		if configuredID := strings.TrimSpace(cfg.CurrentWorkspaceID); configuredID != "" {
			ref = configuredID
			if configuredName := strings.TrimSpace(cfg.CurrentWorkspace); configuredName != "" {
				displayName = configuredName
			}
		} else {
			ref = selectedWorkspaceReference(cfg)
		}
	}
	if ref == "" {
		return workspaceSelection{}, fmt.Errorf("workspace is required; no current workspace is selected\nRun '%s workspace use <workspace>' or pass a workspace explicitly", filepath.Base(os.Args[0]))
	}

	if match, ok, err := matchWorkspaceSelection(ref, displayName, workspaces.Items); err != nil {
		return workspaceSelection{}, err
	} else if ok {
		return match, nil
	}

	if requested == "" {
		label := ref
		if displayName != "" {
			label = displayName
		}
		return workspaceSelection{}, fmt.Errorf("current workspace %q does not exist; run '%s workspace use <workspace>' or pass a workspace explicitly", label, filepath.Base(os.Args[0]))
	}
	return workspaceSelection{}, fmt.Errorf("workspace %q does not exist", ref)
}

func selectedWorkspaceName(cfg config) string {
	if active := activeWorkspaceFromState(cfg); active.Name != "" {
		return active.Name
	}
	return strings.TrimSpace(cfg.CurrentWorkspace)
}

func selectedWorkspaceReference(cfg config) string {
	if active := activeWorkspaceFromState(cfg); active.ID != "" || active.Name != "" {
		if active.ID != "" {
			return active.ID
		}
		return active.Name
	}
	if strings.TrimSpace(cfg.CurrentWorkspaceID) != "" {
		return strings.TrimSpace(cfg.CurrentWorkspaceID)
	}
	return strings.TrimSpace(cfg.CurrentWorkspace)
}

func activeWorkspaceFromState(cfg config) workspaceSelection {
	st, err := loadState()
	if err != nil {
		return workspaceSelection{}
	}

	backendName := strings.TrimSpace(st.MountBackend)
	if backendName == "" {
		backendName = mountBackendNone
	}

	mountActive := backendName != mountBackendNone || st.MountPID > 0
	if mountActive {
		if !runtimeStateMatchesConfig(cfg, st) {
			return workspaceSelection{}
		}
		return workspaceSelection{
			ID:   strings.TrimSpace(st.CurrentWorkspaceID),
			Name: strings.TrimSpace(st.CurrentWorkspace),
		}
	}

	syncActive := strings.TrimSpace(st.Mode) == modeSync || st.SyncPID > 0
	if !syncActive || !runtimeStateMatchesConfig(cfg, st) {
		return workspaceSelection{}
	}

	return workspaceSelection{
		ID:   strings.TrimSpace(st.CurrentWorkspaceID),
		Name: strings.TrimSpace(st.CurrentWorkspace),
	}
}

func runtimeStateMatchesConfig(cfg config, st state) bool {
	cfgMode, err := effectiveProductMode(cfg)
	if err != nil {
		return false
	}

	stateMode := strings.TrimSpace(st.ProductMode)
	switch stateMode {
	case "":
		if strings.TrimSpace(st.ControlPlaneURL) != "" {
			stateMode = productModeSelfHosted
		} else {
			stateMode = productModeLocal
		}
	case legacyProductModeDirect:
		stateMode = productModeLocal
	}
	if stateMode != cfgMode {
		return false
	}

	switch cfgMode {
	case productModeLocal:
		stateAddr := strings.TrimSpace(st.RedisAddr)
		cfgAddr := strings.TrimSpace(cfg.RedisAddr)
		if stateAddr == "" || cfgAddr == "" {
			return true
		}
		return stateAddr == cfgAddr && st.RedisDB == cfg.RedisDB
	case productModeSelfHosted, productModeCloud:
		stateURL := strings.TrimSpace(st.ControlPlaneURL)
		cfgURL := strings.TrimSpace(cfg.URL)
		if stateURL == "" || cfgURL == "" {
			return stateURL == cfgURL
		}
		return stateURL == cfgURL
	default:
		return false
	}
}

func matchWorkspaceSelection(ref, displayName string, workspaces []workspaceSummary) (workspaceSelection, bool, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return workspaceSelection{}, false, nil
	}

	for _, workspace := range workspaces {
		if workspace.ID == ref {
			return workspaceSelection{ID: workspace.ID, Name: workspace.Name}, true, nil
		}
	}

	matches := make([]workspaceSummary, 0)
	for _, workspace := range workspaces {
		if workspace.Name == ref {
			matches = append(matches, workspace)
		}
	}
	switch len(matches) {
	case 0:
		if displayName != "" && displayName != ref {
			for _, workspace := range workspaces {
				if workspace.Name == displayName || workspace.ID == displayName {
					return workspaceSelection{ID: workspace.ID, Name: workspace.Name}, true, nil
				}
			}
		}
		return workspaceSelection{}, false, nil
	case 1:
		return workspaceSelection{ID: matches[0].ID, Name: matches[0].Name}, true, nil
	default:
		ids := make([]string, 0, len(matches))
		for _, workspace := range matches {
			label := workspace.ID
			if databaseName := strings.TrimSpace(workspace.DatabaseName); databaseName != "" {
				label = fmt.Sprintf("%s (%s)", workspace.ID, databaseName)
			}
			ids = append(ids, label)
		}
		return workspaceSelection{}, false, fmt.Errorf(
			"workspace %q exists multiple times; use a workspace id instead: %s",
			ref,
			strings.Join(ids, ", "),
		)
	}
}

func applyWorkspaceSelection(cfg *config, selection workspaceSelection) error {
	if cfg == nil {
		return fmt.Errorf("missing config")
	}
	cfg.CurrentWorkspace = strings.TrimSpace(selection.Name)
	cfg.CurrentWorkspaceID = ""

	productMode, err := effectiveProductMode(*cfg)
	if err != nil {
		return err
	}
	if productMode != productModeDirect {
		cfg.CurrentWorkspaceID = strings.TrimSpace(selection.ID)
	}
	return nil
}

func saveAFSManifest(ctx context.Context, store *afsStore, workspace, expectedHead, savepointID string, localManifest manifest, blobs map[string][]byte, stats manifestStats, syncWorkspaceRoot bool) (bool, error) {
	cfg, err := loadAFSConfig()
	if err != nil {
		return false, err
	}
	service := controlPlaneServiceFromStore(cfg, store)
	saved, err := service.SaveCheckpoint(ctx, controlplane.SaveCheckpointRequest{
		Workspace:             workspace,
		ExpectedHead:          expectedHead,
		CheckpointID:          savepointID,
		Manifest:              controlPlaneManifestFromAFS(localManifest),
		Blobs:                 blobs,
		FileCount:             stats.FileCount,
		DirCount:              stats.DirCount,
		TotalBytes:            stats.TotalBytes,
		SkipWorkspaceRootSync: !syncWorkspaceRoot,
	})
	if errors.Is(err, controlplane.ErrWorkspaceConflict) || err == redis.TxFailedErr {
		return false, errAFSWorkspaceConflict
	}
	return saved, err
}

type afsParsedArgs struct {
	positionals []string
	force       bool
	readonly    bool
	database    string
}

func parseAFSArgs(args []string, allowForce, allowReadonly bool) (afsParsedArgs, error) {
	var parsed afsParsedArgs
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--database":
			if i+1 >= len(args) {
				return parsed, fmt.Errorf("missing value for %q", args[i])
			}
			i++
			parsed.database = strings.TrimSpace(args[i])
		case "--force":
			if !allowForce {
				return parsed, fmt.Errorf("unknown flag %q", args[i])
			}
			parsed.force = true
		case "--readonly":
			if !allowReadonly {
				return parsed, fmt.Errorf("unknown flag %q", args[i])
			}
			parsed.readonly = true
		default:
			if strings.HasPrefix(args[i], "--database=") {
				parsed.database = strings.TrimSpace(strings.TrimPrefix(args[i], "--database="))
				continue
			}
			if strings.HasPrefix(args[i], "--") {
				return parsed, fmt.Errorf("unknown flag %q", args[i])
			}
			parsed.positionals = append(parsed.positionals, args[i])
		}
	}
	return parsed, nil
}

func generatedSavepointName() string {
	return "save-" + time.Now().UTC().Format("20060102-150405.000")
}

func loadAFSConfig() (config, error) {
	cfg, err := loadConfig()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, fmt.Errorf("no configuration found\nRun '%s setup' first", filepath.Base(os.Args[0]))
		}
		return cfg, err
	}
	if err := resolveConfigPaths(&cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func openAFSStore(ctx context.Context) (config, *afsStore, func(), error) {
	session, err := openAFSBackendSession(ctx)
	if err != nil {
		return config{}, nil, func() {}, err
	}
	if session.store == nil {
		productMode, _ := effectiveProductMode(session.cfg)
		session.close()
		return config{}, nil, func() {}, fmt.Errorf("%s mode does not expose a local Redis store yet\nThis command still requires local mode for now", productMode)
	}
	return session.cfg, session.store, session.close, nil
}
