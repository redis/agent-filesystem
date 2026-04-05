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
	"time"

	"github.com/redis/go-redis/v9"
)

var errImportCancelled = errors.New("import cancelled")

func cmdImport(args []string) error {
	parsed, err := parseRAFArgs(args[1:], true, false)
	if err != nil {
		return err
	}
	if parsed.session != "" {
		return fmt.Errorf("import does not accept --session")
	}
	if len(parsed.positionals) != 2 {
		return fmt.Errorf("usage: %s import [--force] <workspace> <directory>", filepath.Base(os.Args[0]))
	}

	workspace := parsed.positionals[0]
	if err := validateRAFName("workspace", workspace); err != nil {
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

	cfg, store, closeStore, err := openRAFStore(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	exists, err := store.workspaceExists(context.Background(), workspace)
	if err != nil {
		return err
	}
	if exists && !parsed.force {
		return fmt.Errorf("workspace %q already exists; rerun with --force to replace it", workspace)
	}

	defaultSession := cfg.DefaultSession
	if defaultSession == "" {
		defaultSession = "main"
	}
	if err := validateRAFName("session", defaultSession); err != nil {
		return err
	}

	const initialSavepoint = "initial"
	total, ignorer, scanDuration, err := prepareRAFImport(sourceDir, workspace, defaultSession, cfg, parsed.force)
	if err != nil {
		if errors.Is(err, errImportCancelled) {
			fmt.Println()
			fmt.Println("  Import cancelled.")
			fmt.Println()
			return nil
		}
		return err
	}

	if parsed.force {
		step := startStep("Replacing existing workspace")
		if err := store.deleteWorkspace(context.Background(), workspace); err != nil {
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
	step := startStep("Building manifest")
	manifest, blobs, stats, err := buildManifestFromDirectoryWithOptions(sourceDir, workspace, initialSavepoint, ignorer, func(progress importStats) {
		step.update(formatRAFImportProgressLabel("Building manifest", progress, total, step.elapsed()))
	})
	if err != nil {
		step.fail(err.Error())
		return err
	}
	manifestDuration := step.elapsed()
	step.succeed(formatRAFImportSummary(total))
	manifestHash, err := hashManifest(manifest)
	if err != nil {
		return err
	}

	workspaceMeta := workspaceMeta{
		Version:          rafFormatVersion,
		Name:             workspace,
		CreatedAt:        now,
		DefaultSession:   defaultSession,
		DefaultSavepoint: initialSavepoint,
	}
	sessionMeta := sessionMeta{
		Version:       rafFormatVersion,
		Workspace:     workspace,
		Name:          defaultSession,
		HeadSavepoint: initialSavepoint,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	savepointMeta := savepointMeta{
		Version:      rafFormatVersion,
		ID:           initialSavepoint,
		Name:         initialSavepoint,
		Workspace:    workspace,
		Session:      defaultSession,
		ManifestHash: manifestHash,
		CreatedAt:    now,
		FileCount:    stats.FileCount,
		DirCount:     stats.DirCount,
		TotalBytes:   stats.TotalBytes,
	}

	ctx := context.Background()
	step = startStep("Saving blobs")
	if err := store.saveBlobs(ctx, workspace, blobs); err != nil {
		step.fail(err.Error())
		return err
	}
	blobCount, blobBytes := rafBlobTotals(blobs)
	blobDuration := step.elapsed()
	if blobCount == 0 {
		step.succeed("all files inlined")
	} else {
		step.succeed(fmt.Sprintf("%d blobs, %s", blobCount, formatBytes(blobBytes)))
	}

	step = startStep("Writing workspace metadata")
	if err := store.addBlobRefs(ctx, workspace, manifest, now); err != nil {
		step.fail(err.Error())
		return err
	}
	if err := store.putWorkspaceMeta(ctx, workspaceMeta); err != nil {
		step.fail(err.Error())
		return err
	}
	if err := store.putSavepoint(ctx, savepointMeta, manifest); err != nil {
		step.fail(err.Error())
		return err
	}
	if err := store.putSessionMeta(ctx, sessionMeta); err != nil {
		step.fail(err.Error())
		return err
	}
	metadataDuration := step.elapsed()
	step.succeed(initialSavepoint)

	if err := store.audit(ctx, workspace, defaultSession, "import", map[string]any{
		"savepoint":   initialSavepoint,
		"files":       stats.FileCount,
		"dirs":        stats.DirCount,
		"total_bytes": stats.TotalBytes,
		"source":      sourceDir,
	}); err != nil {
		return err
	}

	printBox(clr(ansiBGreen, "●")+" "+clr(ansiBold, "workspace imported"), []boxRow{
		{Label: "workspace", Value: workspace},
		{Label: "checkpoint", Value: initialSavepoint},
		{Label: "files", Value: strconv.Itoa(stats.FileCount)},
		{Label: "dirs", Value: strconv.Itoa(stats.DirCount)},
		{Label: "symlinks", Value: strconv.Itoa(total.Symlinks)},
		{Label: "ignored", Value: strconv.Itoa(total.Ignored)},
		{Label: "bytes", Value: formatBytes(stats.TotalBytes)},
		{Label: "scan", Value: formatStepDuration(scanDuration)},
		{Label: "manifest", Value: formatStepDuration(manifestDuration)},
		{Label: "blobs", Value: formatStepDuration(blobDuration)},
		{Label: "metadata", Value: formatStepDuration(metadataDuration)},
		{Label: "next", Value: filepath.Base(os.Args[0]) + " workspace run " + workspace + " -- /bin/sh"},
	})
	return nil
}

func prepareRAFImport(sourceDir, workspace, session string, cfg config, replaceExisting bool) (importStats, *migrationIgnore, time.Duration, error) {
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
		step.succeed(formatRAFImportSummary(total))

		if !interactive {
			return total, ignorer, scanDuration, nil
		}

		estimate := estimateRAFImportDuration(total)
		rows := []boxRow{
			{Label: "source", Value: sourceDir},
			{Label: "workspace", Value: workspace},
			{Label: "scan", Value: formatRAFImportSummary(total)},
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
			boxRow{Value: clr(ansiDim, "Tip: use .rafignore to skip caches, dependencies, logs, or build output before import.")},
		)
		printBox(clr(ansiBold, "Import plan"), rows)

		editLabel := "  Create or edit .rafignore before importing?"
		if ignorer != nil {
			editLabel = fmt.Sprintf("  Edit %s before importing?", filepath.Base(ignorer.path))
		}
		editIgnore, err := promptYesNo(reader, os.Stdout, editLabel, false)
		if err != nil {
			return importStats{}, nil, 0, err
		}
		if editIgnore {
			ignorePath := filepath.Join(sourceDir, rafIgnoreFilename)
			if ignorer != nil && !ignorer.legacy {
				ignorePath = ignorer.path
			}
			if err := ensureRAFIgnoreTemplate(ignorePath); err != nil {
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

func estimateRAFImportDuration(total importStats) time.Duration {
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

func formatRAFImportSummary(total importStats) string {
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

func formatRAFImportProgressLabel(phase string, progress, total importStats, elapsed time.Duration) string {
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

func ensureRAFIgnoreTemplate(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	template := strings.Join([]string{
		"# Ignore paths during raf import and migrate.",
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

func rafBlobTotals(blobs map[string][]byte) (int, int64) {
	var total int64
	for _, blob := range blobs {
		total += int64(len(blob))
	}
	return len(blobs), total
}

func currentWorkspaceName(ctx context.Context, cfg config, store *rafStore) (string, error) {
	if strings.TrimSpace(cfg.CurrentWorkspace) != "" {
		exists, err := store.workspaceExists(ctx, cfg.CurrentWorkspace)
		if err != nil {
			return "", err
		}
		if exists {
			return cfg.CurrentWorkspace, nil
		}
		return "", fmt.Errorf("current workspace %q does not exist; run '%s workspace use <workspace>' or pass a workspace explicitly", cfg.CurrentWorkspace, filepath.Base(os.Args[0]))
	}
	return "", fmt.Errorf("workspace is required; no current workspace is selected\nRun '%s workspace use <workspace>' or pass a workspace explicitly", filepath.Base(os.Args[0]))
}

func resolveWorkspaceName(ctx context.Context, cfg config, store *rafStore, requested string) (string, error) {
	if requested != "" {
		return requested, nil
	}
	return currentWorkspaceName(ctx, cfg, store)
}

func cmdClone(args []string) error {
	parsed, err := parseRAFArgs(args[1:], false, false)
	if err != nil {
		return err
	}
	if len(parsed.positionals) > 1 {
		return fmt.Errorf("usage: %s session clone [workspace] [--session <name>]", filepath.Base(os.Args[0]))
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

	session := defaultRAFSession(cfg, parsed.session)
	if err := validateRAFName("session", session); err != nil {
		return err
	}

	treePath := rafSessionTreePath(cfg, workspace, session)
	if _, _, err := requireMaterializedSession(context.Background(), store, cfg, workspace, session); err == nil {
		printBox(clr(ansiBGreen, "●")+" "+clr(ansiBold, "working copy ready"), []boxRow{
			{Label: "workspace", Value: workspace},
			{Label: "session", Value: session},
			{Label: "path", Value: treePath},
		})
		return nil
	} else if err != nil && !errors.Is(err, errRAFSessionNotMaterialized) && !errors.Is(err, errRAFSessionConflict) {
		return err
	}

	ctx := context.Background()
	meta, err := store.getSessionMeta(ctx, workspace, session)
	if err != nil {
		return err
	}
	headManifest, err := store.getManifest(ctx, workspace, meta.HeadSavepoint)
	if err != nil {
		return err
	}
	total := manifestImportStats(headManifest)

	step := startStep("Creating working copy")
	if err := materializeSessionWithProgress(ctx, store, cfg, workspace, session, func(progress importStats) {
		step.update(formatRAFImportProgressLabel("Creating working copy", progress, total, step.elapsed()))
	}); err != nil {
		step.fail(err.Error())
		return err
	}
	step.succeed(treePath)

	_ = store.audit(ctx, workspace, session, "clone", map[string]any{
		"head": meta.HeadSavepoint,
		"path": treePath,
	})
	printBox(clr(ansiBGreen, "●")+" "+clr(ansiBold, "working copy created"), []boxRow{
		{Label: "workspace", Value: workspace},
		{Label: "session", Value: session},
		{Label: "head", Value: meta.HeadSavepoint},
		{Label: "path", Value: treePath},
	})
	return nil
}

func cmdRun(args []string) error {
	parsed, childArgs, err := parseRAFCommandInvocation(args[1:])
	if err != nil {
		return err
	}
	if len(parsed.positionals) > 1 {
		return fmt.Errorf("usage: %s session run [workspace] [--session <name>] [--readonly] -- <command...>", filepath.Base(os.Args[0]))
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

	session := defaultRAFSession(cfg, parsed.session)
	if err := validateRAFName("session", session); err != nil {
		return err
	}

	if err := runRAFCommand(context.Background(), cfg, store, workspace, session, childArgs, parsed.readonly); err != nil {
		if errors.Is(err, errRAFSessionConflict) {
			return fmt.Errorf("session %q moved since this tree was materialized; inspect the workspace or fork before running again", session)
		}
		return err
	}
	return nil
}

func runRAFCommand(ctx context.Context, cfg config, store *rafStore, workspace, session string, childArgs []string, readonly bool) error {
	sessionMeta, localState, err := ensureMaterializedSession(ctx, store, cfg, workspace, session)
	if err != nil {
		return err
	}

	treePath := rafSessionTreePath(cfg, workspace, session)
	cmd := exec.Command(childArgs[0], childArgs[1:]...)
	cmd.Dir = treePath
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	if err := cmd.Start(); err != nil {
		return err
	}
	startedAt := time.Now().UTC()
	_ = store.audit(ctx, workspace, session, "run_start", map[string]any{
		"pid":      cmd.Process.Pid,
		"cwd":      treePath,
		"argv":     strings.Join(childArgs, " "),
		"readonly": readonly,
	})

	waitErr := cmd.Wait()
	exitCode := 0
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			exitCode = exitErr.ExitCode()
			if exitCode < 0 {
				exitCode = 1
			}
		} else {
			return waitErr
		}
	}

	_ = store.audit(ctx, workspace, session, "run_exit", map[string]any{
		"pid":         cmd.Process.Pid,
		"exit_code":   exitCode,
		"duration_ms": time.Since(startedAt).Milliseconds(),
		"readonly":    readonly,
	})

	if readonly {
		localState.Dirty = true
		localState.LastScanAt = time.Now().UTC()
		if err := saveRAFLocalState(cfg, localState); err != nil {
			return err
		}

		sessionMeta.DirtyHint = true
		if err := store.putSessionMeta(ctx, sessionMeta); err != nil {
			return err
		}
	} else {
		if _, err := saveRAFSession(ctx, cfg, store, workspace, session, generatedSavepointName(), true); err != nil {
			if exitCode != 0 {
				return fmt.Errorf("command exited with code %d, and auto-save failed: %w", exitCode, err)
			}
			return err
		}
	}

	if exitCode != 0 {
		return rafProcessExitError{Code: exitCode}
	}
	return nil
}

func cmdDiff(args []string) error {
	parsed, err := parseRAFArgs(args[1:], false, false)
	if err != nil {
		return err
	}
	if len(parsed.positionals) > 1 {
		return fmt.Errorf("usage: %s session diff [workspace] [--session <name>]", filepath.Base(os.Args[0]))
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

	session := defaultRAFSession(cfg, parsed.session)
	if err := validateRAFName("session", session); err != nil {
		return err
	}

	ctx := context.Background()
	sessionMeta, localState, err := requireMaterializedSession(ctx, store, cfg, workspace, session)
	if err != nil {
		if errors.Is(err, errRAFSessionConflict) {
			return fmt.Errorf("session %q has unsaved local changes against an old head; save or fork before diffing", session)
		}
		if errors.Is(err, errRAFSessionNotMaterialized) {
			return fmt.Errorf("session %q has no working copy yet; run '%s session clone %s --session %s' or '%s session run %s --session %s -- <command>' first", session, filepath.Base(os.Args[0]), workspace, session, filepath.Base(os.Args[0]), workspace, session)
		}
		return err
	}

	headManifest, err := store.getManifest(ctx, workspace, sessionMeta.HeadSavepoint)
	if err != nil {
		return err
	}
	localManifest, _, _, err := buildManifestFromDirectory(rafSessionTreePath(cfg, workspace, session), workspace, sessionMeta.HeadSavepoint)
	if err != nil {
		return err
	}
	diff := summarizeManifestDiff(headManifest, localManifest)

	localState.Dirty = len(diff) > 0
	localState.LastScanAt = time.Now().UTC()
	if err := saveRAFLocalState(cfg, localState); err != nil {
		return err
	}
	sessionMeta.DirtyHint = localState.Dirty
	if err := store.putSessionMeta(ctx, sessionMeta); err != nil {
		return err
	}

	if len(diff) == 0 {
		fmt.Println("No changes")
		return nil
	}
	for _, entry := range diff {
		fmt.Printf("%s %s\n", entry.Kind, strings.TrimPrefix(entry.Path, "/"))
	}
	return nil
}

func cmdSave(args []string) error {
	parsed, err := parseRAFArgs(args[1:], false, false)
	if err != nil {
		return err
	}
	if len(parsed.positionals) > 2 {
		return fmt.Errorf("usage: %s session save [workspace] [--session <name>] [savepoint]", filepath.Base(os.Args[0]))
	}

	cfg, store, closeStore, err := openRAFStore(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	ctx := context.Background()
	workspace := ""
	savepointID := generatedSavepointName()
	switch len(parsed.positionals) {
	case 0:
		workspace, err = currentWorkspaceName(ctx, cfg, store)
		if err != nil {
			return err
		}
	case 1:
		explicitWorkspace, explicitErr := store.workspaceExists(ctx, parsed.positionals[0])
		if explicitErr != nil {
			return explicitErr
		}
		if explicitWorkspace {
			workspace = parsed.positionals[0]
		} else {
			workspace, err = currentWorkspaceName(ctx, cfg, store)
			if err != nil {
				return err
			}
			savepointID = parsed.positionals[0]
		}
	case 2:
		workspace = parsed.positionals[0]
		savepointID = parsed.positionals[1]
	}
	if err := validateRAFName("workspace", workspace); err != nil {
		return err
	}

	session := defaultRAFSession(cfg, parsed.session)
	if err := validateRAFName("session", session); err != nil {
		return err
	}

	if err := validateRAFName("savepoint", savepointID); err != nil {
		return err
	}

	_, err = saveRAFSession(ctx, cfg, store, workspace, session, savepointID, true)
	return err
}

func saveRAFSession(ctx context.Context, cfg config, store *rafStore, workspace, session, savepointID string, printResult bool) (bool, error) {
	sessionInfo, localState, err := requireMaterializedSession(ctx, store, cfg, workspace, session)
	if err != nil {
		if errors.Is(err, errRAFSessionConflict) {
			return false, fmt.Errorf("workspace %q moved since this tree was materialized; reopen it before creating a checkpoint", workspace)
		}
		if errors.Is(err, errRAFSessionNotMaterialized) {
			return false, fmt.Errorf("workspace %q has no working copy yet; run '%s workspace run %s -- /bin/sh' first", workspace, filepath.Base(os.Args[0]), workspace)
		}
		return false, err
	}

	headManifest, err := store.getManifest(ctx, workspace, sessionInfo.HeadSavepoint)
	if err != nil {
		return false, err
	}

	now := time.Now().UTC()
	treePath := rafSessionTreePath(cfg, workspace, session)
	localManifest, blobs, stats, err := buildManifestFromDirectory(treePath, workspace, savepointID)
	if err != nil {
		return false, err
	}
	if manifestEquivalent(headManifest, localManifest) {
		localState.Dirty = false
		localState.LastScanAt = now
		if err := saveRAFLocalState(cfg, localState); err != nil {
			return false, err
		}
		sessionInfo.DirtyHint = false
		if err := store.putSessionMeta(ctx, sessionInfo); err != nil {
			return false, err
		}
		if printResult {
			fmt.Println("No changes to save")
		}
		return false, nil
	}

	saved, err := saveRAFManifest(ctx, store, workspace, session, localState.HeadSavepoint, savepointID, localManifest, blobs, stats)
	if err != nil {
		if errors.Is(err, errRAFSessionConflict) {
			return false, fmt.Errorf("checkpoint conflict: workspace %q moved while saving; reopen it before retrying", workspace)
		}
		return false, err
	}
	if !saved {
		return false, nil
	}

	localState.HeadSavepoint = savepointID
	localState.Dirty = false
	localState.LastScanAt = now
	if err := saveRAFLocalState(cfg, localState); err != nil {
		return false, err
	}

	if printResult {
		printBox(clr(ansiBGreen, "●")+" "+clr(ansiBold, "save complete"), []boxRow{
			{Label: "workspace", Value: workspace},
			{Label: "session", Value: session},
			{Label: "savepoint", Value: savepointID},
			{Label: "files", Value: strconv.Itoa(stats.FileCount)},
			{Label: "dirs", Value: strconv.Itoa(stats.DirCount)},
			{Label: "bytes", Value: formatBytes(stats.TotalBytes)},
		})
	}
	return true, nil
}

func saveRAFManifest(ctx context.Context, store *rafStore, workspace, session, expectedHead, savepointID string, localManifest manifest, blobs map[string][]byte, stats manifestStats) (bool, error) {
	headManifest, err := store.getManifest(ctx, workspace, expectedHead)
	if err != nil {
		return false, err
	}
	if manifestEquivalent(headManifest, localManifest) {
		return false, nil
	}

	now := time.Now().UTC()
	manifestHash, err := hashManifest(localManifest)
	if err != nil {
		return false, err
	}
	if err := store.saveBlobs(ctx, workspace, blobs); err != nil {
		return false, err
	}

	savepointMeta := savepointMeta{
		Version:         rafFormatVersion,
		ID:              savepointID,
		Name:            savepointID,
		Workspace:       workspace,
		Session:         session,
		ParentSavepoint: expectedHead,
		ManifestHash:    manifestHash,
		CreatedAt:       now,
		FileCount:       stats.FileCount,
		DirCount:        stats.DirCount,
		TotalBytes:      stats.TotalBytes,
	}

	err = store.rdb.Watch(ctx, func(tx *redis.Tx) error {
		current, err := rafGetJSON[sessionMeta](ctx, tx, rafSessionMetaKey(workspace, session))
		if err != nil {
			return err
		}
		if current.HeadSavepoint != expectedHead {
			return errRAFSessionConflict
		}
		exists, err := tx.Exists(ctx, rafSavepointMetaKey(workspace, savepointID)).Result()
		if err != nil {
			return err
		}
		if exists > 0 {
			return fmt.Errorf("savepoint %q already exists", savepointID)
		}

		updatedRefs := map[string]blobRef{}
		for blobID, size := range manifestBlobRefs(localManifest) {
			ref, err := rafGetJSON[blobRef](ctx, tx, rafBlobRefKey(workspace, blobID))
			if err != nil {
				if !errors.Is(err, os.ErrNotExist) {
					return err
				}
				ref = blobRef{
					BlobID:    blobID,
					Size:      size,
					CreatedAt: now,
				}
			}
			ref.RefCount++
			if ref.Size == 0 {
				ref.Size = size
			}
			updatedRefs[blobID] = ref
		}

		current.HeadSavepoint = savepointID
		current.UpdatedAt = now
		current.DirtyHint = false

		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			if err := rafSetJSON(ctx, pipe, rafSavepointMetaKey(workspace, savepointID), savepointMeta); err != nil {
				return err
			}
			if err := rafSetJSON(ctx, pipe, rafSavepointManifestKey(workspace, savepointID), localManifest); err != nil {
				return err
			}
			if err := rafSetJSON(ctx, pipe, rafSessionMetaKey(workspace, session), current); err != nil {
				return err
			}
			pipe.ZAdd(ctx, rafWorkspaceSavepointsKey(workspace), redis.Z{
				Score:  float64(now.UnixMilli()),
				Member: savepointID,
			})
			pipe.ZAdd(ctx, rafWorkspaceSessionsKey(workspace), redis.Z{
				Score:  float64(now.UnixMilli()),
				Member: session,
			})
			for blobID, ref := range updatedRefs {
				if err := rafSetJSON(ctx, pipe, rafBlobRefKey(workspace, blobID), ref); err != nil {
					return err
				}
			}
			return nil
		})
		return err
	}, rafSessionMetaKey(workspace, session))
	if err != nil {
		if errors.Is(err, errRAFSessionConflict) || err == redis.TxFailedErr {
			return false, errRAFSessionConflict
		}
		return false, err
	}

	if err := store.audit(ctx, workspace, session, "save", map[string]any{
		"savepoint": savepointID,
		"parent":    savepointMeta.ParentSavepoint,
		"files":     stats.FileCount,
		"dirs":      stats.DirCount,
		"bytes":     stats.TotalBytes,
	}); err != nil {
		return false, err
	}
	return true, nil
}

func cmdFork(args []string) error {
	if len(args) != 3 && len(args) != 4 {
		return fmt.Errorf("usage: %s session fork [workspace] <session> <new-session>", filepath.Base(os.Args[0]))
	}

	cfg, store, closeStore, err := openRAFStore(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	ctx := context.Background()
	workspace := ""
	sourceSession := ""
	newSession := ""
	switch len(args) {
	case 3:
		workspace, err = currentWorkspaceName(ctx, cfg, store)
		if err != nil {
			return err
		}
		sourceSession = args[1]
		newSession = args[2]
	case 4:
		workspace = args[1]
		sourceSession = args[2]
		newSession = args[3]
	}
	if err := validateRAFName("workspace", workspace); err != nil {
		return err
	}
	if err := validateRAFName("session", sourceSession); err != nil {
		return err
	}
	if err := validateRAFName("session", newSession); err != nil {
		return err
	}

	sourceMeta, err := store.getSessionMeta(ctx, workspace, sourceSession)
	if err != nil {
		return err
	}
	if _, err := store.getSessionMeta(ctx, workspace, newSession); err == nil {
		return fmt.Errorf("session %q already exists", newSession)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := os.Stat(rafSessionDir(cfg, workspace, newSession)); err == nil {
		return fmt.Errorf("local session directory already exists for %q", newSession)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	now := time.Now().UTC()
	forked := sessionMeta{
		Version:       rafFormatVersion,
		Workspace:     workspace,
		Name:          newSession,
		HeadSavepoint: sourceMeta.HeadSavepoint,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.putSessionMeta(ctx, forked); err != nil {
		return err
	}
	if err := store.audit(ctx, workspace, newSession, "fork", map[string]any{
		"from_session": sourceSession,
		"head":         sourceMeta.HeadSavepoint,
	}); err != nil {
		return err
	}

	printBox(clr(ansiBGreen, "●")+" "+clr(ansiBold, "session forked"), []boxRow{
		{Label: "workspace", Value: workspace},
		{Label: "from", Value: sourceSession},
		{Label: "new", Value: newSession},
		{Label: "head", Value: sourceMeta.HeadSavepoint},
	})
	return nil
}

func cmdRollback(args []string) error {
	parsed, err := parseRAFArgs(args[1:], false, false)
	if err != nil {
		return err
	}
	if len(parsed.positionals) < 1 || len(parsed.positionals) > 2 {
		return fmt.Errorf("usage: %s session rollback [workspace] [--session <name>] <savepoint>", filepath.Base(os.Args[0]))
	}

	cfg, store, closeStore, err := openRAFStore(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	ctx := context.Background()
	workspace := ""
	targetSavepoint := ""
	switch len(parsed.positionals) {
	case 1:
		workspace, err = currentWorkspaceName(ctx, cfg, store)
		if err != nil {
			return err
		}
		targetSavepoint = parsed.positionals[0]
	case 2:
		workspace = parsed.positionals[0]
		targetSavepoint = parsed.positionals[1]
	}
	if err := validateRAFName("workspace", workspace); err != nil {
		return err
	}
	if err := validateRAFName("savepoint", targetSavepoint); err != nil {
		return err
	}

	session := defaultRAFSession(cfg, parsed.session)
	if err := validateRAFName("session", session); err != nil {
		return err
	}

	exists, err := store.savepointExists(ctx, workspace, targetSavepoint)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("savepoint %q does not exist", targetSavepoint)
	}

	resetResult, err := resetRAFSessionHead(ctx, cfg, store, workspace, session, targetSavepoint)
	if err != nil {
		if err == redis.TxFailedErr {
			return fmt.Errorf("rollback conflict: session %q moved while rolling back", session)
		}
		return err
	}

	if err := store.audit(ctx, workspace, session, "rollback", map[string]any{
		"savepoint": targetSavepoint,
		"archive":   resetResult.archivePath,
	}); err != nil {
		return err
	}

	rows := []boxRow{
		{Label: "workspace", Value: workspace},
		{Label: "session", Value: session},
		{Label: "savepoint", Value: targetSavepoint},
		{Label: "local tree", Value: resetResult.treePath},
	}
	if resetResult.archivePath != "" {
		rows = append(rows, boxRow{Label: "archive", Value: resetResult.archivePath})
	}
	printBox(clr(ansiBGreen, "●")+" "+clr(ansiBold, "rollback complete"), rows)
	return nil
}

type rafProcessExitError struct {
	Code int
}

func (e rafProcessExitError) Error() string {
	return fmt.Sprintf("process exited with code %d", e.Code)
}

type rafParsedArgs struct {
	positionals []string
	session     string
	force       bool
	readonly    bool
}

func parseRAFCommandInvocation(args []string) (rafParsedArgs, []string, error) {
	for i, arg := range args {
		if arg == "--" {
			parsed, err := parseRAFArgs(args[:i], false, true)
			if err != nil {
				return rafParsedArgs{}, nil, err
			}
			if i+1 >= len(args) {
				return rafParsedArgs{}, nil, errors.New("missing command after --")
			}
			return parsed, args[i+1:], nil
		}
	}
	return rafParsedArgs{}, nil, errors.New("run requires '--' before the command to execute")
}

func parseRAFArgs(args []string, allowForce, allowReadonly bool) (rafParsedArgs, error) {
	var parsed rafParsedArgs
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--session":
			if i+1 >= len(args) {
				return parsed, errors.New("missing value for --session")
			}
			parsed.session = args[i+1]
			i++
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
			if strings.HasPrefix(args[i], "--") {
				return parsed, fmt.Errorf("unknown flag %q", args[i])
			}
			parsed.positionals = append(parsed.positionals, args[i])
		}
	}
	return parsed, nil
}

func defaultRAFSession(cfg config, requested string) string {
	if requested != "" {
		return requested
	}
	if strings.TrimSpace(cfg.DefaultSession) != "" {
		return cfg.DefaultSession
	}
	return "main"
}

func generatedSavepointName() string {
	return "save-" + time.Now().UTC().Format("20060102-150405")
}

func inspectWorkspace(ctx context.Context, cfg config, store *rafStore, workspace string) error {
	meta, err := store.getWorkspaceMeta(ctx, workspace)
	if err != nil {
		return err
	}
	sessions, err := store.listSessions(ctx, workspace)
	if err != nil {
		return err
	}
	savepoints, err := store.listSavepoints(ctx, workspace, "", 5)
	if err != nil {
		return err
	}
	blobStats, err := store.blobStats(ctx, workspace)
	if err != nil {
		return err
	}

	latest := "none"
	if len(savepoints) > 0 {
		names := make([]string, 0, len(savepoints))
		for _, savepoint := range savepoints {
			names = append(names, savepoint.Name)
		}
		latest = strings.Join(names, ", ")
	}

	printBox(clr(ansiBold, "workspace")+" "+workspace, []boxRow{
		{Label: "created", Value: meta.CreatedAt.Local().Format(time.RFC3339)},
		{Label: "default session", Value: meta.DefaultSession},
		{Label: "default savepoint", Value: meta.DefaultSavepoint},
		{Label: "sessions", Value: strconv.Itoa(len(sessions))},
		{Label: "recent saves", Value: latest},
		{Label: "blobs", Value: fmt.Sprintf("%d (%s)", blobStats.Count, formatBytes(blobStats.Bytes))},
	})
	return nil
}

func inspectSession(ctx context.Context, cfg config, store *rafStore, workspace, session string) error {
	meta, err := store.getSessionMeta(ctx, workspace, session)
	if err != nil {
		return err
	}

	stateValue := clr(ansiDim, "not materialized")
	materializedAt := clr(ansiDim, "n/a")
	if st, err := loadRAFLocalState(cfg, workspace, session); err == nil {
		if st.Dirty {
			stateValue = clr(ansiYellow, "dirty")
		} else {
			stateValue = clr(ansiGreen, "clean")
		}
		materializedAt = st.MaterializedAt.Local().Format(time.RFC3339)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	printBox(clr(ansiBold, "session")+" "+workspace+"/"+session, []boxRow{
		{Label: "head", Value: meta.HeadSavepoint},
		{Label: "state", Value: stateValue},
		{Label: "local tree", Value: rafSessionTreePath(cfg, workspace, session)},
		{Label: "materialized", Value: materializedAt},
		{Label: "updated", Value: meta.UpdatedAt.Local().Format(time.RFC3339)},
	})
	return nil
}

func loadRAFConfig() (config, error) {
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

func openRAFStore(ctx context.Context) (config, *rafStore, func(), error) {
	cfg, err := loadRAFConfig()
	if err != nil {
		return cfg, nil, func() {}, err
	}

	rdb := redis.NewClient(buildRedisOptions(cfg, 8))
	closeFn := func() {
		_ = rdb.Close()
	}
	if err := rdb.Ping(ctx).Err(); err != nil {
		closeFn()
		return cfg, nil, func() {}, fmt.Errorf("cannot connect to Redis at %s: %w\nRun '%s up' first or point RAF at an existing Redis server",
			cfg.RedisAddr, err, filepath.Base(os.Args[0]))
	}
	return cfg, newRAFStore(rdb), closeFn, nil
}
