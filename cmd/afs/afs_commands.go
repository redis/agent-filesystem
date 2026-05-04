package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
		return fmt.Errorf("usage: %s import [--force] [--mount-at-source] [--database <database-id|database-name>] <workspace> <directory>", filepath.Base(os.Args[0]))
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
		return cmdImportDirect(ctx, cfg, workspace, sourceDir, parsed.force, parsed.mountAtSource)
	case productModeSelfHosted:
		return cmdImportSelfHosted(ctx, cfg, workspace, sourceDir, parsed.force, parsed.database, parsed.mountAtSource)
	default:
		_, _, _, err := openAFSControlPlaneForConfig(ctx, cfg)
		return err
	}
}

func cmdImportDirect(ctx context.Context, cfg config, workspace, sourceDir string, replaceExisting, mountAtSource bool) error {
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
	var preservedVersioningPolicy *controlplane.WorkspaceVersioningPolicy
	if replaceExisting && exists {
		policy, err := store.cp.GetWorkspaceVersioningPolicy(ctx, workspace)
		if err != nil {
			return err
		}
		preservedVersioningPolicy = &policy
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
	workspaceID, err := newCLIWorkspaceID()
	if err != nil {
		return err
	}

	writer := store.newBlobWriter(workspaceID, now)
	sink := newImportBlobSink(ctx, writer)

	step := startStep("Building manifest")
	manifest, stats, err := buildManifestStreaming(sourceDir, workspaceID, initialSavepoint, ignorer, sink, func(progress importStats) {
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
		ID:               workspaceID,
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
		Kind:         controlplane.CheckpointKindImport,
		Source:       controlplane.CheckpointSourceImport,
		Author:       "afs",
		Workspace:    workspaceID,
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
	if preservedVersioningPolicy != nil {
		if err := store.cp.PutWorkspaceVersioningPolicy(ctx, workspaceID, *preservedVersioningPolicy); err != nil {
			step.fail(err.Error())
			return err
		}
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
	if err := store.syncWorkspaceRootWithOptions(ctx, workspaceID, manifest, syncOpts); err != nil {
		step.fail(err.Error())
		return err
	}
	rootDuration := step.elapsed()
	step.succeed("ready to mount")

	// Drop the blob cache now that sync has consumed it.
	sink.Drop()

	if preservedVersioningPolicy != nil {
		if _, err := store.cp.RecordManifestVersionChangesWithResults(
			ctx,
			workspaceID,
			controlplane.Manifest{},
			controlPlaneManifestFromAFS(manifest),
			controlplane.FileVersionMutationMetadata{
				Source:       controlplane.ChangeSourceImport,
				CheckpointID: initialSavepoint,
			},
		); err != nil {
			return err
		}
	}

	if err := store.audit(ctx, workspaceID, "import", map[string]any{
		"savepoint":   initialSavepoint,
		"files":       stats.FileCount,
		"dirs":        stats.DirCount,
		"total_bytes": stats.TotalBytes,
		"source":      sourceDir,
	}); err != nil {
		return err
	}

	rows := []outputRow{
		{Label: "workspace", Value: workspace},
		{Label: "checkpoint", Value: initialSavepoint},
		{Label: "files", Value: strconv.Itoa(stats.FileCount)},
		{Label: "dirs", Value: strconv.Itoa(stats.DirCount)},
		{Label: "symlinks", Value: strconv.Itoa(total.Symlinks)},
		{Label: "ignored", Value: strconv.Itoa(total.Ignored)},
		{Label: "bytes", Value: formatBytes(stats.TotalBytes)},
		{Label: "workers", Value: strconv.Itoa(resolveImportWorkers())},
		{Label: "import time", Value: formatStepDuration(scanDuration + buildDuration + metadataDuration + rootDuration)},
	}
	if !mountAtSource {
		rows = append(rows, outputRow{Label: "next", Value: filepath.Base(os.Args[0]) + " ws mount " + workspace + " " + shellQuote(sourceDir)})
	}
	printSection(markerSuccess+" "+clr(ansiBold, "workspace imported"), rows)
	if mountAtSource {
		return mountWorkspace(mountOptions{workspace: workspace, directory: sourceDir})
	}
	return nil
}

func newCLIWorkspaceID() (string, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return "ws_" + hex.EncodeToString(raw[:]), nil
}

func cmdImportSelfHosted(ctx context.Context, cfg config, workspace, sourceDir string, replaceExisting bool, explicitDatabase string, mountAtSource bool) error {
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
	var preservedVersioningPolicy *controlplane.WorkspaceVersioningPolicy
	_, err = service.GetWorkspace(ctx, workspace)
	switch {
	case err == nil:
		exists = true
		if !replaceExisting {
			return fmt.Errorf("workspace %q already exists; rerun with --force to replace it", workspace)
		}
		policy, err := service.GetWorkspaceVersioningPolicy(ctx, workspace)
		if err != nil {
			return err
		}
		preservedVersioningPolicy = &policy
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
		Name:             workspace,
		Description:      fmt.Sprintf("Imported from %s.", sourceDir),
		Manifest:         controlPlaneManifestFromAFS(manifest),
		Blobs:            blobs,
		FileCount:        stats.FileCount,
		DirCount:         stats.DirCount,
		TotalBytes:       stats.TotalBytes,
		VersioningPolicy: preservedVersioningPolicy,
	})
	if err != nil {
		step.fail(err.Error())
		return err
	}
	uploadDuration := step.elapsed()
	step.succeed(response.Workspace.HeadCheckpointID)

	rows := []outputRow{
		{Label: "workspace", Value: workspace},
		{Label: "checkpoint", Value: response.Workspace.HeadCheckpointID},
		{Label: "files", Value: strconv.Itoa(stats.FileCount)},
		{Label: "dirs", Value: strconv.Itoa(stats.DirCount)},
		{Label: "symlinks", Value: strconv.Itoa(total.Symlinks)},
		{Label: "ignored", Value: strconv.Itoa(total.Ignored)},
		{Label: "bytes", Value: formatBytes(stats.TotalBytes)},
		{Label: "workers", Value: strconv.Itoa(resolveImportWorkers())},
		{Label: "import time", Value: formatStepDuration(scanDuration + buildDuration + uploadDuration)},
	}
	if !mountAtSource {
		rows = append(rows, outputRow{Label: "next", Value: filepath.Base(os.Args[0]) + " ws mount " + workspace + " " + shellQuote(sourceDir)})
	}
	printSection(markerSuccess+" "+clr(ansiBold, "workspace imported"), rows)
	if mountAtSource {
		return mountWorkspace(mountOptions{workspace: workspace, directory: sourceDir})
	}
	return nil
}

// resolveImportWorkers mirrors the logic used inside worktree.BuildManifest so
// the summary reports the actual worker count.
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
		rows := []outputRow{
			{Label: "source", Value: sourceDir},
			{Label: "workspace", Value: workspace},
			{Label: "scan", Value: formatAFSImportSummary(total)},
			{Label: "estimate", Value: "~" + formatStepDuration(estimate)},
		}
		if ignorer != nil {
			rows = append(rows, outputRow{Label: "ignore", Value: ignorer.path})
		} else {
			rows = append(rows, outputRow{Label: "ignore", Value: clr(ansiDim, "none")})
		}
		if replaceExisting {
			rows = append(rows, outputRow{Label: "replace", Value: "existing workspace state will be removed"})
		}
		rows = append(rows,
			outputRow{},
			outputRow{Value: clr(ansiDim, "Tip: use .afsignore to skip caches, dependencies, logs, or build output before import.")},
		)
		printSection(clr(ansiBold, "Import plan"), rows)

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
			if ignorer != nil {
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

type workspaceSelection struct {
	ID        string
	Name      string
	Source    workspaceSelectionSource
	MountPath string
}

type workspaceSelectionSource string

const (
	workspaceSelectionExplicit     workspaceSelectionSource = "explicit"
	workspaceSelectionCWD          workspaceSelectionSource = "cwd-mounted"
	workspaceSelectionSingleMount  workspaceSelectionSource = "mounted"
	workspaceSelectionActiveState  workspaceSelectionSource = "active-runtime"
	workspaceSelectionSavedDefault workspaceSelectionSource = "default"
	workspaceSelectionPrompt       workspaceSelectionSource = "prompt"
)

func currentWorkspaceName(ctx context.Context, cfg config, store *afsStore) (string, error) {
	workspaces, err := store.listWorkspaces(ctx)
	if err != nil {
		return "", err
	}
	selection, err := resolveWorkspaceSelectionFromSummaries(cfg, "", workspaceSummariesFromMetas(workspaces), false)
	if err != nil {
		return "", err
	}
	return selection.Name, nil
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
	return resolveWorkspaceSelectionFromControlPlaneWithPrompt(ctx, cfg, service, requested, false)
}

func resolveCommandWorkspaceSelectionFromControlPlane(ctx context.Context, cfg config, service afsControlPlane, requested string) (workspaceSelection, error) {
	return resolveWorkspaceSelectionFromControlPlaneWithPrompt(ctx, cfg, service, requested, true)
}

func resolveWorkspaceSelectionFromControlPlaneWithPrompt(ctx context.Context, cfg config, service afsControlPlane, requested string, allowPrompt bool) (workspaceSelection, error) {
	workspaces, err := service.ListWorkspaceSummaries(ctx)
	if err != nil {
		return workspaceSelection{}, err
	}
	return resolveWorkspaceSelectionFromSummaries(cfg, requested, workspaces.Items, allowPrompt)
}

func resolveWorkspaceSelectionFromSummaries(cfg config, requested string, workspaces []workspaceSummary, allowPrompt bool) (workspaceSelection, error) {
	ref := strings.TrimSpace(requested)
	if ref != "" {
		selection, err := resolveExplicitWorkspaceSelection(ref, workspaces)
		if err != nil {
			return workspaceSelection{}, err
		}
		return selection, nil
	}

	var fallbackErr error
	trySelection := func(selection workspaceSelection, ok bool, err error) (workspaceSelection, bool, error) {
		if err != nil {
			if allowPrompt {
				if fallbackErr == nil {
					fallbackErr = err
				}
				return workspaceSelection{}, false, nil
			}
			return workspaceSelection{}, false, err
		}
		if ok {
			return selection, true, nil
		}
		return workspaceSelection{}, false, nil
	}

	if selection, ok, err := trySelection(workspaceSelectionFromCWDMount(cfg, workspaces)); err != nil {
		return workspaceSelection{}, err
	} else if ok {
		return selection, nil
	}

	if selection, ok, err := trySelection(workspaceSelectionFromSingleMount(cfg, workspaces)); err != nil {
		return workspaceSelection{}, err
	} else if ok {
		return selection, nil
	}

	if strings.TrimSpace(cfg.CurrentWorkspaceID) != "" {
		if selection, ok, err := trySelection(workspaceSelectionFromSavedDefault(cfg, workspaces)); err != nil {
			return workspaceSelection{}, err
		} else if ok {
			return selection, nil
		}
	}

	if selection, ok, err := trySelection(workspaceSelectionFromActiveState(cfg, workspaces)); err != nil {
		return workspaceSelection{}, err
	} else if ok {
		return selection, nil
	}

	if selection, ok, err := trySelection(workspaceSelectionFromSavedDefault(cfg, workspaces)); err != nil {
		return workspaceSelection{}, err
	} else if ok {
		return selection, nil
	}

	if allowPrompt {
		selection, err := promptWorkspaceSelectionFromSummaries(workspaces)
		if err == nil {
			return selection, nil
		}
		if fallbackErr != nil {
			return workspaceSelection{}, fallbackErr
		}
		return workspaceSelection{}, err
	}

	return workspaceSelection{}, workspaceRequiredError(cfg)
}

func resolveExplicitWorkspaceSelection(ref string, workspaces []workspaceSummary) (workspaceSelection, error) {
	match, ok, err := matchWorkspaceSelection(ref, "", workspaces)
	if err != nil {
		return workspaceSelection{}, fmt.Errorf("%w\nRun '%s ws list' and pass the workspace id explicitly", err, filepath.Base(os.Args[0]))
	}
	if ok {
		match.Source = workspaceSelectionExplicit
		return match, nil
	}
	return workspaceSelection{}, fmt.Errorf("workspace %q does not exist", strings.TrimSpace(ref))
}

func selectedWorkspaceName(cfg config) string {
	if active := selectedMountedWorkspace(cfg); active.Name != "" {
		return active.Name
	}
	if active := activeWorkspaceFromState(cfg); active.Name != "" {
		return active.Name
	}
	return strings.TrimSpace(cfg.CurrentWorkspace)
}

func selectedWorkspaceReference(cfg config) string {
	if active := selectedMountedWorkspace(cfg); active.ID != "" || active.Name != "" {
		if active.ID != "" {
			return active.ID
		}
		return active.Name
	}
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

func configuredWorkspaceReference(cfg config) string {
	if strings.TrimSpace(cfg.CurrentWorkspaceID) != "" {
		return strings.TrimSpace(cfg.CurrentWorkspaceID)
	}
	return strings.TrimSpace(cfg.CurrentWorkspace)
}

func selectedMountedWorkspace(cfg config) workspaceSelection {
	if selection, ok, _ := workspaceSelectionFromCWDMount(cfg, nil); ok {
		return selection
	}
	if selection, ok, _ := workspaceSelectionFromSingleMount(cfg, nil); ok {
		return selection
	}
	return workspaceSelection{}
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
			ID:     strings.TrimSpace(st.CurrentWorkspaceID),
			Name:   strings.TrimSpace(st.CurrentWorkspace),
			Source: workspaceSelectionActiveState,
		}
	}

	syncActive := strings.TrimSpace(st.Mode) == modeSync || st.SyncPID > 0
	if !syncActive || !runtimeStateMatchesConfig(cfg, st) {
		return workspaceSelection{}
	}

	return workspaceSelection{
		ID:     strings.TrimSpace(st.CurrentWorkspaceID),
		Name:   strings.TrimSpace(st.CurrentWorkspace),
		Source: workspaceSelectionActiveState,
	}
}

func workspaceSelectionFromCWDMount(cfg config, workspaces []workspaceSummary) (workspaceSelection, bool, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return workspaceSelection{}, false, nil
	}
	reg, err := loadMountRegistry()
	if err != nil {
		return workspaceSelection{}, false, err
	}
	var matches []mountRecord
	for _, rec := range runningMountRecordsForConfig(cfg, reg.Mounts) {
		localPath := physicalCleanPath(rec.LocalPath)
		cleanCWD := physicalCleanPath(cwd)
		if cleanCWD == localPath || pathContains(localPath, cleanCWD) {
			matches = append(matches, rec)
		}
	}
	if len(matches) == 0 {
		return workspaceSelection{}, false, nil
	}
	sort.SliceStable(matches, func(i, j int) bool {
		return len(filepath.Clean(matches[i].LocalPath)) > len(filepath.Clean(matches[j].LocalPath))
	})
	return workspaceSelectionFromMountRecord(matches[0], workspaces, workspaceSelectionCWD)
}

func workspaceSelectionFromSingleMount(cfg config, workspaces []workspaceSummary) (workspaceSelection, bool, error) {
	reg, err := loadMountRegistry()
	if err != nil {
		return workspaceSelection{}, false, err
	}
	running := runningMountRecordsForConfig(cfg, reg.Mounts)
	if len(running) != 1 {
		return workspaceSelection{}, false, nil
	}
	return workspaceSelectionFromMountRecord(running[0], workspaces, workspaceSelectionSingleMount)
}

func physicalCleanPath(path string) string {
	clean := filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(clean); err == nil {
		return filepath.Clean(resolved)
	}
	return clean
}

func runningMountRecordsForConfig(cfg config, records []mountRecord) []mountRecord {
	matches := make([]mountRecord, 0, len(records))
	for _, rec := range sortedMountRecords(records) {
		if mountStatus(rec) != "running" {
			continue
		}
		if !mountRecordMatchesConfig(cfg, rec) {
			continue
		}
		matches = append(matches, rec)
	}
	return matches
}

func mountRecordMatchesConfig(cfg config, rec mountRecord) bool {
	cfgMode, err := effectiveProductMode(cfg)
	if err != nil {
		return false
	}
	recMode := strings.TrimSpace(rec.ProductMode)
	if recMode != "" && recMode != cfgMode {
		return false
	}
	switch cfgMode {
	case productModeLocal:
		if strings.TrimSpace(rec.RedisAddr) != "" && strings.TrimSpace(cfg.RedisAddr) != "" && strings.TrimSpace(rec.RedisAddr) != strings.TrimSpace(cfg.RedisAddr) {
			return false
		}
		return rec.RedisDB == cfg.RedisDB
	case productModeSelfHosted, productModeCloud:
		recURL := strings.TrimSpace(rec.ControlPlaneURL)
		cfgURL := strings.TrimSpace(cfg.URL)
		if recURL == "" || cfgURL == "" {
			return false
		}
		return recURL == cfgURL
	default:
		return false
	}
}

func workspaceSelectionFromMountRecord(rec mountRecord, workspaces []workspaceSummary, source workspaceSelectionSource) (workspaceSelection, bool, error) {
	selection := workspaceSelection{
		ID:        strings.TrimSpace(rec.WorkspaceID),
		Name:      strings.TrimSpace(rec.Workspace),
		Source:    source,
		MountPath: strings.TrimSpace(rec.LocalPath),
	}
	if len(workspaces) == 0 {
		return selection, selection.Name != "" || selection.ID != "", nil
	}
	ref := selection.ID
	if ref == "" {
		ref = selection.Name
	}
	match, ok, err := matchWorkspaceSelection(ref, selection.Name, workspaces)
	if err != nil {
		return workspaceSelection{}, false, fmt.Errorf("mounted workspace %q is ambiguous: %w\nRun '%s ws list' and pass a workspace id explicitly", selection.Name, err, filepath.Base(os.Args[0]))
	}
	if !ok {
		label := selection.Name
		if label == "" {
			label = selection.ID
		}
		return workspaceSelection{}, false, fmt.Errorf("mounted workspace %q does not exist; pass a workspace explicitly", label)
	}
	match.Source = source
	match.MountPath = selection.MountPath
	return match, true, nil
}

func workspaceSelectionFromActiveState(cfg config, workspaces []workspaceSummary) (workspaceSelection, bool, error) {
	active := activeWorkspaceFromState(cfg)
	if active.ID == "" && active.Name == "" {
		return workspaceSelection{}, false, nil
	}
	if len(workspaces) == 0 {
		return active, true, nil
	}
	ref := active.ID
	if ref == "" {
		ref = active.Name
	}
	match, ok, err := matchWorkspaceSelection(ref, active.Name, workspaces)
	if err != nil {
		return workspaceSelection{}, false, fmt.Errorf("active workspace %q is ambiguous: %w\nRun '%s ws list' and pass a workspace id explicitly", active.Name, err, filepath.Base(os.Args[0]))
	}
	if !ok {
		label := active.Name
		if label == "" {
			label = active.ID
		}
		return workspaceSelection{}, false, fmt.Errorf("active workspace %q does not exist; pass a workspace explicitly", label)
	}
	match.Source = workspaceSelectionActiveState
	return match, true, nil
}

func workspaceSelectionFromSavedDefault(cfg config, workspaces []workspaceSummary) (workspaceSelection, bool, error) {
	ref := configuredWorkspaceReference(cfg)
	if ref == "" {
		return workspaceSelection{}, false, nil
	}
	displayName := strings.TrimSpace(cfg.CurrentWorkspace)
	match, ok, err := matchWorkspaceSelection(ref, displayName, workspaces)
	if err != nil {
		label := displayName
		if label == "" {
			label = ref
		}
		return workspaceSelection{}, false, fmt.Errorf("default workspace %q is ambiguous: %w\nRun '%s ws set-default <workspace-id>'", label, err, filepath.Base(os.Args[0]))
	}
	if !ok {
		label := displayName
		if label == "" {
			label = ref
		}
		return workspaceSelection{}, false, fmt.Errorf("default workspace %q does not exist; pass a workspace explicitly or run '%s ws unset-default'", label, filepath.Base(os.Args[0]))
	}
	match.Source = workspaceSelectionSavedDefault
	return match, true, nil
}

func workspaceRequiredError(cfg config) error {
	reg, err := loadMountRegistry()
	if err == nil {
		running := runningMountRecordsForConfig(cfg, reg.Mounts)
		if len(running) > 1 {
			return fmt.Errorf("workspace is required; multiple workspaces are mounted\nRun this command inside a mounted workspace, pass a workspace explicitly, or run '%s ws set-default <workspace>'", filepath.Base(os.Args[0]))
		}
	}
	return fmt.Errorf("workspace is required; pass a workspace explicitly or run '%s ws set-default <workspace>'", filepath.Base(os.Args[0]))
}

func workspaceSummariesFromMetas(metas []workspaceMeta) []workspaceSummary {
	items := make([]workspaceSummary, 0, len(metas))
	for _, meta := range metas {
		items = append(items, workspaceSummary{
			ID:           strings.TrimSpace(meta.ID),
			Name:         strings.TrimSpace(meta.Name),
			DatabaseID:   strings.TrimSpace(meta.DatabaseID),
			DatabaseName: strings.TrimSpace(meta.DatabaseName),
			RedisKey:     strings.TrimSpace(meta.ID),
			UpdatedAt:    meta.UpdatedAt.Format(time.RFC3339),
		})
	}
	return items
}

func promptWorkspaceSelectionFromSummaries(workspaces []workspaceSummary) (workspaceSelection, error) {
	return promptWorkspaceSelectionFromSummariesWithReader(workspaces, bufio.NewReader(os.Stdin))
}

func promptWorkspaceSelectionFromSummariesWithReader(workspaces []workspaceSummary, reader *bufio.Reader) (workspaceSelection, error) {
	if len(workspaces) == 0 {
		return workspaceSelection{}, fmt.Errorf("no workspaces found\nCreate one with: %s ws create <workspace>", filepath.Base(os.Args[0]))
	}

	fmt.Println()
	fmt.Println("Select workspace")
	fmt.Println()
	printPlainTable([]string{"#", "Workspace", "Updated", "Mounted"}, checkpointWorkspacePromptRows(workspaces, workspaceListMounts()))
	fmt.Println()
	fmt.Print("Workspace: ")

	raw, err := reader.ReadString('\n')
	if err != nil && strings.TrimSpace(raw) == "" {
		fmt.Println()
		return workspaceSelection{}, errors.New("workspace selection cancelled")
	}
	choiceText := strings.TrimSpace(raw)
	if choiceText == "" {
		fmt.Println()
		return workspaceSelection{}, errors.New("workspace selection cancelled")
	}
	idx, err := strconv.Atoi(choiceText)
	if err != nil || idx < 1 || idx > len(workspaces) {
		return workspaceSelection{}, fmt.Errorf("invalid selection %q", choiceText)
	}
	fmt.Println()
	selected := workspaces[idx-1]
	return workspaceSelection{
		ID:     selected.ID,
		Name:   selected.Name,
		Source: workspaceSelectionPrompt,
	}, nil
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

	idMatches := make([]workspaceSummary, 0, 1)
	nameMatches := make([]workspaceSummary, 0)
	for _, workspace := range workspaces {
		if workspace.ID == ref {
			idMatches = append(idMatches, workspace)
		}
		if workspace.Name == ref {
			nameMatches = append(nameMatches, workspace)
		}
	}

	switch {
	case len(nameMatches) > 1:
		labels := workspaceSelectionLabels(nameMatches)
		return workspaceSelection{}, false, fmt.Errorf(
			"workspace %q exists multiple times; use a workspace id instead: %s",
			ref,
			strings.Join(labels, ", "),
		)
	case len(idMatches) == 1:
		return workspaceSelection{ID: idMatches[0].ID, Name: idMatches[0].Name}, true, nil
	case len(idMatches) > 1:
		labels := workspaceSelectionLabels(idMatches)
		return workspaceSelection{}, false, fmt.Errorf(
			"workspace id %q is ambiguous; choose one of: %s",
			ref,
			strings.Join(labels, ", "),
		)
	}

	switch len(nameMatches) {
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
		return workspaceSelection{ID: nameMatches[0].ID, Name: nameMatches[0].Name}, true, nil
	default:
		return workspaceSelection{}, false, nil
	}
}

func workspaceSelectionLabels(matches []workspaceSummary) []string {
	seen := make(map[string]struct{}, len(matches))
	labels := make([]string, 0, len(matches))
	for _, workspace := range matches {
		label := workspace.ID
		if databaseName := strings.TrimSpace(workspace.DatabaseName); databaseName != "" {
			label = fmt.Sprintf("%s (%s)", workspace.ID, databaseName)
		}
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		labels = append(labels, label)
	}
	sort.Strings(labels)
	return labels
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
	if productMode != productModeLocal {
		cfg.CurrentWorkspaceID = strings.TrimSpace(selection.ID)
	}
	return nil
}

func saveAFSManifest(ctx context.Context, store *afsStore, workspace, expectedHead, savepointID string, localManifest manifest, blobs map[string][]byte, stats manifestStats, syncWorkspaceRoot bool, options ...controlplane.SaveCheckpointFromLiveOptions) (bool, error) {
	cfg, err := loadAFSConfig()
	if err != nil {
		return false, err
	}
	var metadata controlplane.SaveCheckpointFromLiveOptions
	if len(options) > 0 {
		metadata = options[0]
	}
	if strings.TrimSpace(metadata.Kind) == "" {
		metadata.Kind = controlplane.CheckpointKindManual
	}
	if strings.TrimSpace(metadata.Source) == "" {
		metadata.Source = controlplane.CheckpointSourceCLI
	}
	service := controlPlaneServiceFromStore(cfg, store)
	saved, err := service.SaveCheckpoint(ctx, controlplane.SaveCheckpointRequest{
		Workspace:             workspace,
		ExpectedHead:          expectedHead,
		CheckpointID:          savepointID,
		Description:           metadata.Description,
		Kind:                  metadata.Kind,
		Source:                metadata.Source,
		Author:                metadata.Author,
		CreatedBy:             metadata.CreatedBy,
		Manifest:              controlPlaneManifestFromAFS(localManifest),
		Blobs:                 blobs,
		FileCount:             stats.FileCount,
		DirCount:              stats.DirCount,
		TotalBytes:            stats.TotalBytes,
		SkipWorkspaceRootSync: !syncWorkspaceRoot,
		AllowUnchanged:        metadata.AllowUnchanged,
	})
	if errors.Is(err, controlplane.ErrWorkspaceConflict) || err == redis.TxFailedErr {
		return false, errAFSWorkspaceConflict
	}
	return saved, err
}

type afsParsedArgs struct {
	positionals   []string
	force         bool
	readonly      bool
	database      string
	mountAtSource bool
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
		case "--mount-at-source":
			parsed.mountAtSource = true
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
