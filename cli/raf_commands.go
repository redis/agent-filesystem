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

	const initialSavepoint = "initial"
	total, ignorer, scanDuration, err := prepareRAFImport(sourceDir, workspace, cfg, parsed.force)
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
		UpdatedAt:        now,
		HeadSavepoint:    initialSavepoint,
		DefaultSavepoint: initialSavepoint,
	}
	savepointMeta := savepointMeta{
		Version:      rafFormatVersion,
		ID:           initialSavepoint,
		Name:         initialSavepoint,
		Workspace:    workspace,
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
	metadataDuration := step.elapsed()
	step.succeed(initialSavepoint)

	if err := store.audit(ctx, workspace, "import", map[string]any{
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

func prepareRAFImport(sourceDir, workspace string, cfg config, replaceExisting bool) (importStats, *migrationIgnore, time.Duration, error) {
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

func runRAFCommand(ctx context.Context, cfg config, store *rafStore, workspace string, childArgs []string, readonly bool) error {
	workspaceMeta, localState, err := ensureMaterializedWorkspace(ctx, store, cfg, workspace)
	if err != nil {
		return err
	}

	treePath := rafWorkspaceTreePath(cfg, workspace)
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
	_ = store.audit(ctx, workspace, "run_start", map[string]any{
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

	_ = store.audit(ctx, workspace, "run_exit", map[string]any{
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

		workspaceMeta.DirtyHint = true
		if err := store.putWorkspaceMeta(ctx, workspaceMeta); err != nil {
			return err
		}
	} else {
		if _, err := saveRAFWorkspace(ctx, cfg, store, workspace, generatedSavepointName(), true); err != nil {
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

func saveRAFWorkspace(ctx context.Context, cfg config, store *rafStore, workspace, savepointID string, printResult bool) (bool, error) {
	workspaceInfo, localState, err := requireMaterializedWorkspace(ctx, store, cfg, workspace)
	if err != nil {
		if errors.Is(err, errRAFWorkspaceConflict) {
			return false, fmt.Errorf("workspace %q moved since this tree was materialized; reopen it before creating a checkpoint", workspace)
		}
		if errors.Is(err, errRAFWorkspaceNotMaterialized) {
			return false, fmt.Errorf("workspace %q has no working copy yet; run '%s workspace run %s -- /bin/sh' first", workspace, filepath.Base(os.Args[0]), workspace)
		}
		return false, err
	}

	headManifest, err := store.getManifest(ctx, workspace, workspaceInfo.HeadSavepoint)
	if err != nil {
		return false, err
	}

	now := time.Now().UTC()
	treePath := rafWorkspaceTreePath(cfg, workspace)
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
		workspaceInfo.DirtyHint = false
		if err := store.putWorkspaceMeta(ctx, workspaceInfo); err != nil {
			return false, err
		}
		if printResult {
			fmt.Println("No changes to save")
		}
		return false, nil
	}

	saved, err := saveRAFManifest(ctx, store, workspace, localState.HeadSavepoint, savepointID, localManifest, blobs, stats)
	if err != nil {
		if errors.Is(err, errRAFWorkspaceConflict) {
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
			{Label: "savepoint", Value: savepointID},
			{Label: "files", Value: strconv.Itoa(stats.FileCount)},
			{Label: "dirs", Value: strconv.Itoa(stats.DirCount)},
			{Label: "bytes", Value: formatBytes(stats.TotalBytes)},
		})
	}
	return true, nil
}

func saveRAFManifest(ctx context.Context, store *rafStore, workspace, expectedHead, savepointID string, localManifest manifest, blobs map[string][]byte, stats manifestStats) (bool, error) {
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
		ParentSavepoint: expectedHead,
		ManifestHash:    manifestHash,
		CreatedAt:       now,
		FileCount:       stats.FileCount,
		DirCount:        stats.DirCount,
		TotalBytes:      stats.TotalBytes,
	}

	err = store.rdb.Watch(ctx, func(tx *redis.Tx) error {
		current, err := rafGetJSON[workspaceMeta](ctx, tx, rafWorkspaceMetaKey(workspace))
		if err != nil {
			return err
		}
		if current.HeadSavepoint != expectedHead {
			return errRAFWorkspaceConflict
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
			if err := rafSetJSON(ctx, pipe, rafWorkspaceMetaKey(workspace), current); err != nil {
				return err
			}
			pipe.ZAdd(ctx, rafWorkspaceSavepointsKey(workspace), redis.Z{
				Score:  float64(now.UnixMilli()),
				Member: savepointID,
			})
			for blobID, ref := range updatedRefs {
				if err := rafSetJSON(ctx, pipe, rafBlobRefKey(workspace, blobID), ref); err != nil {
					return err
				}
			}
			return nil
		})
		return err
	}, rafWorkspaceMetaKey(workspace))
	if err != nil {
		if errors.Is(err, errRAFWorkspaceConflict) || err == redis.TxFailedErr {
			return false, errRAFWorkspaceConflict
		}
		return false, err
	}

	if err := store.audit(ctx, workspace, "save", map[string]any{
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

type rafProcessExitError struct {
	Code int
}

func (e rafProcessExitError) Error() string {
	return fmt.Sprintf("process exited with code %d", e.Code)
}

type rafParsedArgs struct {
	positionals []string
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

func generatedSavepointName() string {
	return "save-" + time.Now().UTC().Format("20060102-150405")
}

func inspectWorkspace(ctx context.Context, cfg config, store *rafStore, workspace string) error {
	meta, err := store.getWorkspaceMeta(ctx, workspace)
	if err != nil {
		return err
	}
	savepoints, err := store.listSavepoints(ctx, workspace, 5)
	if err != nil {
		return err
	}
	blobStats, err := store.blobStats(ctx, workspace)
	if err != nil {
		return err
	}

	stateValue := clr(ansiDim, "not materialized")
	localTree := clr(ansiDim, rafWorkspaceTreePath(cfg, workspace))
	if st, err := loadRAFLocalState(cfg, workspace); err == nil {
		if st.Dirty {
			stateValue = clr(ansiYellow, "dirty")
		} else {
			stateValue = clr(ansiGreen, "clean")
		}
		localTree = rafWorkspaceTreePath(cfg, workspace)
	} else if !errors.Is(err, os.ErrNotExist) {
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
		{Label: "head", Value: meta.HeadSavepoint},
		{Label: "default savepoint", Value: meta.DefaultSavepoint},
		{Label: "state", Value: stateValue},
		{Label: "local tree", Value: localTree},
		{Label: "recent saves", Value: latest},
		{Label: "blobs", Value: fmt.Sprintf("%d (%s)", blobStats.Count, formatBytes(blobStats.Bytes))},
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
		return cfg, nil, func() {}, fmt.Errorf("cannot connect to Redis at %s: %w\nRun '%s up' first or point AFS at an existing Redis server",
			cfg.RedisAddr, err, filepath.Base(os.Args[0]))
	}
	return cfg, newRAFStore(rdb), closeFn, nil
}
