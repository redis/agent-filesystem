package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/redis/agent-filesystem/internal/controlplane"
)

type workspaceCompositionControlPlane interface {
	CreateWorkspaceComposition(context.Context, controlplane.CreateWorkspaceCompositionRequest) (controlplane.WorkspaceCompositionDetail, error)
	ListWorkspaceSummaries(context.Context) (controlplane.WorkspaceListResponse, error)
	ListWorkspaceCompositions(context.Context) (controlplane.WorkspaceCompositionListResponse, error)
	GetWorkspaceComposition(context.Context, string) (controlplane.WorkspaceCompositionDetail, error)
	AddWorkspaceCompositionMount(context.Context, string, controlplane.WorkspaceCompositionMount) (controlplane.WorkspaceCompositionDetail, error)
	RemoveWorkspaceCompositionMount(context.Context, string, string) (controlplane.WorkspaceCompositionDetail, error)
	CreateWorkspaceBookmark(context.Context, string, controlplane.CreateWorkspaceBookmarkRequest) (controlplane.WorkspaceBookmark, error)
	RestoreWorkspaceBookmark(context.Context, string, string) (controlplane.WorkspaceBookmark, error)
}

func openWorkspaceCompositionControlPlane(ctx context.Context) (workspaceCompositionControlPlane, func(), error) {
	cfg, err := loadAFSConfig()
	if err != nil {
		return nil, nil, err
	}
	_, service, closeStore, err := openAFSControlPlaneForConfig(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}
	composer, ok := service.(workspaceCompositionControlPlane)
	if !ok {
		closeStore()
		return nil, nil, fmt.Errorf("workspace composition API is unavailable")
	}
	return composer, closeStore, nil
}

func cmdRootMountArgs(args []string) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		fmt.Fprint(os.Stderr, workspaceCompositionMountUsageFor(filepath.Base(os.Args[0])+" mount"))
		return nil
	}
	opts, err := parseMountOptions(args)
	if err != nil {
		return err
	}
	return mountWorkspaceComposition(opts)
}

func cmdRootUnmountArgs(args []string) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		fmt.Fprint(os.Stderr, workspaceCompositionUnmountUsageFor(filepath.Base(os.Args[0])+" unmount"))
		return nil
	}
	opts, err := parseUnmountOptions(args, false)
	if err != nil {
		return err
	}
	return unmountWorkspaceComposition(opts)
}

func cmdWorkspaceManifestCreate(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, workspaceManifestCreateUsage(filepath.Base(os.Args[0])))
		return nil
	}
	opts, err := parseWorkspaceManifestCreateArgs(args[2:])
	if err != nil {
		return err
	}
	if opts.name == "" {
		return fmt.Errorf("%s", workspaceManifestCreateUsage(filepath.Base(os.Args[0])))
	}
	ctx := context.Background()
	service, closeStore, err := openWorkspaceCompositionControlPlane(ctx)
	if err != nil {
		return err
	}
	defer closeStore()
	detail, err := service.CreateWorkspaceComposition(ctx, controlplane.CreateWorkspaceCompositionRequest{
		Name:        opts.name,
		Description: opts.description,
	})
	if err != nil {
		return err
	}
	if opts.jsonOut {
		return writeJSON(detail)
	}
	printSection(markerSuccess+" "+clr(ansiBold, "workspace manifest created"), []outputRow{
		{Label: "workspace", Value: detail.Name},
		{Label: "id", Value: detail.ID},
		{Label: "volumes", Value: fmt.Sprintf("%d", len(detail.Mounts))},
	})
	return nil
}

func cmdWorkspaceManifestList(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, workspaceManifestListUsage(filepath.Base(os.Args[0])))
		return nil
	}
	jsonOut, err := parseOnlyJSONFlag(args[2:])
	if err != nil {
		return err
	}
	ctx := context.Background()
	service, closeStore, err := openWorkspaceCompositionControlPlane(ctx)
	if err != nil {
		return err
	}
	defer closeStore()
	response, err := service.ListWorkspaceCompositions(ctx)
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(response)
	}
	if len(response.Items) == 0 {
		fmt.Fprintln(os.Stdout, "No workspace manifests found.")
		return nil
	}
	fmt.Fprintf(os.Stdout, "%-28s  %-7s  %s\n", "Workspace", "Volumes", "Updated")
	for _, item := range response.Items {
		fmt.Fprintf(os.Stdout, "%-28s  %-7d  %s\n", item.Name, item.MountCount, item.UpdatedAt)
	}
	return nil
}

func cmdWorkspaceManifestShow(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, workspaceManifestShowUsage(filepath.Base(os.Args[0])))
		return nil
	}
	rest, jsonOut, err := parseJSONFlagWithPositionals(args[2:])
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return fmt.Errorf("%s", workspaceManifestShowUsage(filepath.Base(os.Args[0])))
	}
	ctx := context.Background()
	service, closeStore, err := openWorkspaceCompositionControlPlane(ctx)
	if err != nil {
		return err
	}
	defer closeStore()
	detail, err := service.GetWorkspaceComposition(ctx, rest[0])
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(detail)
	}
	printWorkspaceCompositionDetail(detail)
	return nil
}

func cmdWorkspaceAddVolume(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, workspaceAddVolumeUsage(filepath.Base(os.Args[0])))
		return nil
	}
	opts, err := parseWorkspaceAddVolumeArgs(args[2:])
	if err != nil {
		return err
	}
	if opts.workspace == "" || opts.sourceDir == "" {
		return fmt.Errorf("%s", workspaceAddVolumeUsage(filepath.Base(os.Args[0])))
	}
	sourceDir, err := expandPath(opts.sourceDir)
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
	volumeName := strings.TrimSpace(opts.volumeName)
	if volumeName == "" {
		volumeName = filepath.Base(filepath.Clean(sourceDir))
	}
	if err := validateAFSName("volume", volumeName); err != nil {
		return fmt.Errorf("%w\nPass --name <volume> to choose an AFS-safe volume name.", err)
	}
	mountPath := strings.TrimSpace(opts.mountPath)
	if mountPath == "" {
		mountPath = defaultWorkspaceVolumeMountPath(volumeName)
	}
	mountPath, err = normalizeWorkspaceVolumeMountPathForCLI(mountPath)
	if err != nil {
		return err
	}
	ctx := context.Background()
	cfg, err := loadAFSConfig()
	if err != nil {
		return err
	}
	service, closeStore, err := openWorkspaceCompositionControlPlane(ctx)
	if err != nil {
		return err
	}
	defer closeStore()

	detail, err := service.GetWorkspaceComposition(ctx, opts.workspace)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return workspaceCompositionNotFoundError(opts.workspace)
		}
		return err
	}
	if existing, ok, err := resolveWorkspaceVolumeSelection(ctx, service, volumeName); err != nil {
		return err
	} else if ok {
		label := existing.Name
		if label == "" {
			label = existing.ID
		}
		return fmt.Errorf("volume %q already exists\nUse '%s ws attach %s %s' to reuse it, or pass --name <new-volume>.", label, filepath.Base(os.Args[0]), shellQuote(detail.Name), shellQuote(label))
	}
	if err := validateWorkspaceAddPathAvailable(detail.Mounts, mountPath); err != nil {
		return err
	}

	explicitDatabase := ""
	if mode, err := effectiveProductMode(cfg); err == nil && mode != productModeLocal {
		explicitDatabase = detail.DatabaseID
	}
	importResult, err := importVolume(ctx, cfg, volumeName, sourceDir, volumeImportOptions{
		ExplicitDatabase: explicitDatabase,
		PrintReport:      false,
	})
	if err != nil {
		return err
	}
	if importResult.Cancelled {
		return nil
	}

	detail, err = service.AddWorkspaceCompositionMount(ctx, detail.ID, controlplane.WorkspaceCompositionMount{
		VolumeID:  volumeName,
		MountPath: mountPath,
		Readonly:  opts.readonly,
	})
	if err != nil {
		return err
	}
	printSection(markerSuccess+" "+clr(ansiBold, "folder added to workspace"), []outputRow{
		{Label: "workspace", Value: detail.Name},
		{Label: "source", Value: homeRelativeDisplayPath(sourceDir)},
		{Label: "volume", Value: volumeName},
		{Label: "path", Value: mountPath},
		{Label: "mode", Value: workspaceCompositionSummaryPermission(opts.readonly)},
		{Label: "files", Value: workspaceCompositionFileCountLabel(importResult.FileCount)},
	})
	return nil
}

func cmdWorkspaceAttachVolume(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, workspaceAttachVolumeUsage(filepath.Base(os.Args[0])))
		return nil
	}
	opts, err := parseWorkspaceAttachVolumeArgs(args[2:])
	if err != nil {
		return err
	}
	ctx := context.Background()
	service, closeStore, err := openWorkspaceCompositionControlPlane(ctx)
	if err != nil {
		return err
	}
	defer closeStore()
	workspaceDetail, err := resolveWorkspaceCompositionForCommand(ctx, service, opts.workspace)
	if err != nil {
		return err
	}
	volume, err := resolveWorkspaceAttachVolume(ctx, service, opts.volume)
	if err != nil {
		return err
	}
	mountPath := strings.TrimSpace(opts.mountPath)
	if mountPath == "" {
		mountPath = defaultWorkspaceVolumeMountPath(firstNonEmptyString(volume.Name, opts.volume, volume.ID))
	}
	mountPath, err = normalizeWorkspaceVolumeMountPathForCLI(mountPath)
	if err != nil {
		return err
	}
	detail, err := service.AddWorkspaceCompositionMount(ctx, workspaceDetail.ID, controlplane.WorkspaceCompositionMount{
		VolumeID:      firstNonEmptyString(volume.ID, opts.volume),
		MountPath:     mountPath,
		Readonly:      opts.readonly,
		VolumeTokenID: opts.volumeTokenID,
	})
	if err != nil {
		return err
	}
	if opts.jsonOut {
		return writeJSON(detail)
	}
	volumeLabel := firstNonEmptyString(volume.Name, opts.volume, volume.ID)
	printSection(markerSuccess+" "+clr(ansiBold, "volume attached"), []outputRow{
		{Label: "workspace", Value: detail.Name},
		{Label: "volume", Value: volumeLabel},
		{Label: "path", Value: mountPath},
		{Label: "mode", Value: workspaceCompositionSummaryPermission(opts.readonly)},
	})
	return nil
}

func cmdWorkspaceDetachVolume(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, workspaceDetachVolumeUsage(filepath.Base(os.Args[0])))
		return nil
	}
	rest, jsonOut, err := parseJSONFlagWithPositionals(args[2:])
	if err != nil {
		return err
	}
	if len(rest) != 2 {
		return fmt.Errorf("%s", workspaceDetachVolumeUsage(filepath.Base(os.Args[0])))
	}
	ctx := context.Background()
	service, closeStore, err := openWorkspaceCompositionControlPlane(ctx)
	if err != nil {
		return err
	}
	defer closeStore()
	detail, err := service.RemoveWorkspaceCompositionMount(ctx, rest[0], rest[1])
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(detail)
	}
	printSection(markerSuccess+" "+clr(ansiBold, "volume detached"), []outputRow{
		{Label: "workspace", Value: detail.Name},
		{Label: "volume", Value: rest[1]},
	})
	return nil
}

func cmdWorkspaceBookmark(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, workspaceBookmarkUsage(filepath.Base(os.Args[0])))
		return nil
	}
	opts, err := parseWorkspaceBookmarkArgs(args[2:])
	if err != nil {
		return err
	}
	if opts.workspace == "" || opts.name == "" {
		return fmt.Errorf("%s", workspaceBookmarkUsage(filepath.Base(os.Args[0])))
	}
	ctx := context.Background()
	service, closeStore, err := openWorkspaceCompositionControlPlane(ctx)
	if err != nil {
		return err
	}
	defer closeStore()
	bookmark, err := service.CreateWorkspaceBookmark(ctx, opts.workspace, controlplane.CreateWorkspaceBookmarkRequest{
		Name:        opts.name,
		Description: opts.description,
	})
	if err != nil {
		return err
	}
	if opts.jsonOut {
		return writeJSON(bookmark)
	}
	printSection(markerSuccess+" "+clr(ansiBold, "workspace bookmark created"), []outputRow{
		{Label: "workspace", Value: bookmark.WorkspaceID},
		{Label: "bookmark", Value: bookmark.Name},
		{Label: "volumes", Value: fmt.Sprintf("%d", len(bookmark.Volumes))},
	})
	return nil
}

func cmdWorkspaceRestoreBookmark(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, workspaceRestoreBookmarkUsage(filepath.Base(os.Args[0])))
		return nil
	}
	rest, jsonOut, err := parseJSONFlagWithPositionals(args[2:])
	if err != nil {
		return err
	}
	if len(rest) != 2 {
		return fmt.Errorf("%s", workspaceRestoreBookmarkUsage(filepath.Base(os.Args[0])))
	}
	ctx := context.Background()
	service, closeStore, err := openWorkspaceCompositionControlPlane(ctx)
	if err != nil {
		return err
	}
	defer closeStore()
	bookmark, err := service.RestoreWorkspaceBookmark(ctx, rest[0], rest[1])
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(bookmark)
	}
	printSection(markerSuccess+" "+clr(ansiBold, "workspace bookmark restored"), []outputRow{
		{Label: "workspace", Value: bookmark.WorkspaceID},
		{Label: "bookmark", Value: bookmark.Name},
		{Label: "volumes", Value: fmt.Sprintf("%d", len(bookmark.Volumes))},
	})
	return nil
}

func cmdWorkspaceBookmarkCommand(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, workspaceBookmarkUsage(filepath.Base(os.Args[0])))
		return nil
	}
	if len(args) > 2 {
		switch args[2] {
		case "create":
			return cmdWorkspaceBookmark(append([]string{args[0], args[1]}, args[3:]...))
		case "list":
			return cmdWorkspaceBookmarkList(append([]string{args[0], args[1]}, args[3:]...))
		case "restore":
			return cmdWorkspaceRestoreBookmark(append([]string{args[0], args[1]}, args[3:]...))
		}
	}
	return cmdWorkspaceBookmark(args)
}

func cmdWorkspaceBookmarkList(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, workspaceBookmarkListUsage(filepath.Base(os.Args[0])))
		return nil
	}
	rest, jsonOut, err := parseJSONFlagWithPositionals(args[2:])
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return fmt.Errorf("%s", workspaceBookmarkListUsage(filepath.Base(os.Args[0])))
	}
	ctx := context.Background()
	service, closeStore, err := openWorkspaceCompositionControlPlane(ctx)
	if err != nil {
		return err
	}
	defer closeStore()
	detail, err := service.GetWorkspaceComposition(ctx, rest[0])
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(detail.Bookmarks)
	}
	if len(detail.Bookmarks) == 0 {
		fmt.Fprintln(os.Stdout, "No workspace bookmarks found.")
		return nil
	}
	fmt.Fprintf(os.Stdout, "%-28s  %-7s  %s\n", "Bookmark", "Volumes", "Created")
	for _, bookmark := range detail.Bookmarks {
		fmt.Fprintf(os.Stdout, "%-28s  %-7d  %s\n", bookmark.Name, len(bookmark.Volumes), bookmark.CreatedAt)
	}
	return nil
}

func cmdWorkspaceCompositionMount(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, workspaceCompositionMountUsage(filepath.Base(os.Args[0])))
		return nil
	}
	opts, err := parseMountOptions(args[2:])
	if err != nil {
		return err
	}
	return mountWorkspaceComposition(opts)
}

func mountWorkspaceComposition(opts mountOptions) error {
	if strings.TrimSpace(opts.workspace) == "" {
		return promptWorkspaceCompositionMountSelection(opts)
	}

	ctx := context.Background()
	service, closeStore, err := openWorkspaceCompositionControlPlane(ctx)
	if err != nil {
		return err
	}
	defer closeStore()
	detail, err := service.GetWorkspaceComposition(ctx, opts.workspace)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return workspaceCompositionNotFoundError(opts.workspace)
		}
		return err
	}
	if len(detail.Mounts) == 0 {
		return fmt.Errorf("workspace %s has no attached volumes", detail.Name)
	}
	if strings.TrimSpace(opts.directory) == "" {
		directory, ok, err := promptMountPathForWorkspace(detail.Name)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		opts.directory = directory
	}
	rootPath, err := normalizeMountPath(opts.directory)
	if err != nil {
		return err
	}

	resultRows := make([]workspaceCompositionMountResultRow, 0, len(detail.Mounts))
	for _, mount := range detail.Mounts {
		localPath := workspaceCompositionLocalMountPath(rootPath, mount.MountPath)
		sessionName := workspaceCompositionSessionName(opts.sessionName, detail.Name, mount.MountPath)
		volumeName := mount.VolumeName
		if strings.TrimSpace(volumeName) == "" {
			volumeName = mount.VolumeID
		}
		if err := mountWorkspace(mountOptions{
			workspace:   mount.VolumeID,
			directory:   localPath,
			sessionName: sessionName,
			verbose:     opts.verbose,
			dryRun:      opts.dryRun,
			yes:         opts.yes,
			readonly:    opts.readonly || mount.Readonly,
			quiet:       true,
		}); err != nil {
			return fmt.Errorf("mount volume %s at %s: %w", volumeName, mount.MountPath, err)
		}
		if !opts.dryRun {
			if err := tagWorkspaceCompositionMount(localPath, rootPath, detail, mount); err != nil {
				return err
			}
		}
		resultRows = append(resultRows, workspaceCompositionMountResultRow{
			Path:      homeRelativeDisplayPath(localPath),
			Mode:      workspaceCompositionSummaryPermission(opts.readonly || mount.Readonly),
			FileCount: workspaceCompositionMountedFileCount(volumeName),
			HasCount:  !opts.dryRun,
		})
	}
	printWorkspaceCompositionMountResult(detail.Name, rootPath, workspaceCompositionMountResultMode(opts), resultRows)
	return nil
}

type workspaceCompositionMountResultRow struct {
	Path      string
	Mode      string
	FileCount int
	HasCount  bool
}

func workspaceCompositionSummaryPermission(readonly bool) string {
	if readonly {
		return "read-only"
	}
	return "read-write"
}

func workspaceCompositionMountResultMode(opts mountOptions) string {
	if opts.dryRun {
		return "dry-run"
	}
	cfg, err := loadAFSConfig()
	if err == nil {
		mode, err := effectiveMode(cfg)
		if err == nil && strings.TrimSpace(mode) != "" {
			return mode
		}
	}
	return modeSync
}

func workspaceCompositionMountedFileCount(volumeName string) int {
	st, err := loadSyncState(volumeName)
	if err != nil || st == nil {
		return 0
	}
	count := 0
	for _, entry := range st.Entries {
		if entry.Deleted || entry.Type != "file" {
			continue
		}
		count++
	}
	return count
}

func printWorkspaceCompositionMountResult(workspace, rootPath, mode string, rows []workspaceCompositionMountResultRow) {
	printSection("Workspace Mounted", []outputRow{
		{Label: "workspace", Value: workspace},
		{Label: "path", Value: homeRelativeDisplayPath(rootPath)},
		{Label: "mode", Value: mode},
	})
	fmt.Println("Volumes:")
	fmt.Println()
	printWorkspaceCompositionMountRows(rows)
	fmt.Println()
	fmt.Printf("unmount  %s ws unmount %s\n", filepath.Base(os.Args[0]), workspace)
	fmt.Println()
}

func printWorkspaceCompositionMountRows(rows []workspaceCompositionMountResultRow) {
	if len(rows) == 0 {
		fmt.Println("none")
		return
	}
	tableRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		files := "files unavailable"
		if row.HasCount {
			files = workspaceCompositionFileCountLabel(row.FileCount)
		}
		tableRows = append(tableRows, []string{row.Path, row.Mode, files})
	}
	printPlainRows(tableRows)
}

func printPlainRows(rows [][]string) {
	maxCols := 0
	for _, row := range rows {
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}
	if maxCols == 0 {
		return
	}
	widths := make([]int, maxCols)
	for _, row := range rows {
		for i, value := range row {
			if width := runeWidth(value); width > widths[i] {
				widths[i] = width
			}
		}
	}
	for _, row := range rows {
		printPlainTableRow(row, widths)
	}
}

func workspaceCompositionFileCountLabel(count int) string {
	if count == 1 {
		return "1 file"
	}
	return fmt.Sprintf("%d files", count)
}

type workspaceCompositionPromptChoice struct {
	Name       string
	ID         string
	Mounts     int
	Root       string
	Mounted    bool
	VolumeRows int
}

func promptWorkspaceCompositionMountSelection(opts mountOptions) error {
	ctx := context.Background()
	service, closeStore, err := openWorkspaceCompositionControlPlane(ctx)
	if err != nil {
		return err
	}
	defer closeStore()

	response, err := service.ListWorkspaceCompositions(ctx)
	if err != nil {
		return err
	}
	reg, err := loadMountRegistry()
	if err != nil {
		return err
	}
	choices := workspaceCompositionMountChoices(reg, response.Items)
	if len(choices) == 0 {
		fmt.Println()
		fmt.Println("Mount Agent Workspace")
		fmt.Println()
		fmt.Println("No Agent Workspaces found.")
		fmt.Println("Create one with: " + filepath.Base(os.Args[0]) + " ws create <workspace>")
		fmt.Println()
		return nil
	}

	fmt.Println()
	fmt.Println("Mount Agent Workspace")
	fmt.Println()
	headers := []string{"#", "Workspace", "Volumes", "Status", "Path"}
	printPlainTable(headers, workspaceCompositionPromptRows(choices))
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
		printSection("Agent Workspace already mounted", []outputRow{
			{Label: "workspace", Value: selected.Name},
			{Label: "path", Value: homeRelativeDisplayPath(selected.Root)},
		})
		return nil
	}
	opts.workspace = selected.Name
	if strings.TrimSpace(selected.ID) != "" {
		opts.workspace = selected.ID
	}
	defaultPath := "~/" + selected.Name
	fmt.Println()
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
	opts.directory = localPath
	return mountWorkspaceComposition(opts)
}

func workspaceCompositionMountChoices(reg mountRegistry, items []controlplane.WorkspaceCompositionSummary) []workspaceCompositionPromptChoice {
	roots := workspaceCompositionMountedRoots(reg)
	rootByID := make(map[string]workspaceCompositionMountedRoot, len(roots))
	rootByName := make(map[string]workspaceCompositionMountedRoot, len(roots))
	for _, root := range roots {
		if id := strings.TrimSpace(root.WorkspaceID); id != "" {
			rootByID[id] = root
		}
		if name := strings.TrimSpace(root.WorkspaceName); name != "" {
			rootByName[name] = root
		}
	}
	choices := make([]workspaceCompositionPromptChoice, 0, len(items))
	for _, item := range items {
		choice := workspaceCompositionPromptChoice{
			Name:   item.Name,
			ID:     item.ID,
			Mounts: item.MountCount,
		}
		if root, ok := rootByID[item.ID]; ok {
			choice.Root = root.Root
			choice.Mounted = true
			choice.VolumeRows = root.VolumeRows
		} else if root, ok := rootByName[item.Name]; ok {
			choice.Root = root.Root
			choice.Mounted = true
			choice.VolumeRows = root.VolumeRows
		}
		choices = append(choices, choice)
	}
	sort.Slice(choices, func(i, j int) bool {
		return strings.ToLower(choices[i].Name) < strings.ToLower(choices[j].Name)
	})
	return choices
}

func workspaceCompositionPromptRows(choices []workspaceCompositionPromptChoice) [][]string {
	rows := make([][]string, 0, len(choices))
	for i, choice := range choices {
		status := "available"
		path := ""
		if choice.Mounted {
			status = "mounted"
			path = homeRelativeDisplayPath(choice.Root)
		}
		rows = append(rows, []string{
			strconv.Itoa(i + 1),
			choice.Name,
			fmt.Sprintf("%d", choice.Mounts),
			status,
			path,
		})
	}
	return rows
}

func tagWorkspaceCompositionMount(localPath, rootPath string, detail controlplane.WorkspaceCompositionDetail, mount controlplane.WorkspaceCompositionMount) error {
	normalizedLocal, err := normalizeMountPath(localPath)
	if err != nil {
		return err
	}
	normalizedRoot, err := normalizeMountPath(rootPath)
	if err != nil {
		return err
	}
	reg, err := loadMountRegistry()
	if err != nil {
		return err
	}
	for i := range reg.Mounts {
		if filepath.Clean(reg.Mounts[i].LocalPath) != normalizedLocal {
			continue
		}
		reg.Mounts[i].AgentWorkspace = detail.Name
		reg.Mounts[i].AgentWorkspaceID = detail.ID
		reg.Mounts[i].AgentWorkspaceRoot = normalizedRoot
		reg.Mounts[i].AgentWorkspacePath = mount.MountPath
		return saveMountRegistry(reg)
	}
	return fmt.Errorf("mounted volume record for %s was not registered", normalizedLocal)
}

func workspaceCompositionNotFoundError(workspace string) error {
	ref := strings.TrimSpace(workspace)
	if ref == "" {
		ref = "<workspace>"
	}
	bin := filepath.Base(os.Args[0])
	return fmt.Errorf("Agent Workspace %q does not exist\n\nRun '%s ws list' to see Agent Workspaces, or create one with '%s ws create <workspace>'.", ref, bin, bin)
}

func cmdWorkspaceCompositionUnmount(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, workspaceCompositionUnmountUsage(filepath.Base(os.Args[0])))
		return nil
	}
	opts, err := parseUnmountOptions(args[2:], false)
	if err != nil {
		return err
	}
	return unmountWorkspaceComposition(opts)
}

func unmountWorkspaceComposition(opts unmountOptions) error {
	if strings.TrimSpace(opts.target) == "" {
		return promptWorkspaceCompositionUnmountSelection(opts.deleteLocal)
	}
	if !unmountTargetLooksLikePath(opts.target) {
		return unmountWorkspaceCompositionByRef(opts.target, opts.deleteLocal)
	}
	return unmountWorkspaceCompositionRoot(opts.target, opts.deleteLocal)
}

func unmountWorkspaceCompositionRoot(target string, deleteLocal bool) error {
	root, err := normalizeMountPath(target)
	if err != nil {
		return err
	}
	reg, err := loadMountRegistry()
	if err != nil {
		return err
	}
	matches := make([]mountRecord, 0)
	for _, rec := range sortedMountRecords(reg.Mounts) {
		if mountRecordUnderRoot(rec, root) {
			matches = append(matches, rec)
		}
	}
	if len(matches) == 0 {
		return fmt.Errorf("no volume mounts found under %s", root)
	}
	workspaceName := workspaceCompositionUnmountName(matches, root)
	for _, match := range matches {
		rec, ok := removeMountByPath(&reg, match.LocalPath)
		if !ok {
			continue
		}
		if err := stopMount(rec, deleteLocal); err != nil {
			return err
		}
	}
	if deleteLocal {
		if err := os.RemoveAll(root); err != nil {
			return err
		}
	}
	if err := saveMountRegistry(reg); err != nil {
		return err
	}
	printWorkspaceCompositionUnmountResult(workspaceName, root, len(matches), deleteLocal)
	return nil
}

func workspaceCompositionUnmountName(records []mountRecord, root string) string {
	for _, rec := range records {
		if name := strings.TrimSpace(rec.AgentWorkspace); name != "" {
			return name
		}
	}
	base := filepath.Base(filepath.Clean(root))
	if base == "." || base == string(filepath.Separator) || base == "" {
		return filepath.Clean(root)
	}
	return base
}

func printWorkspaceCompositionUnmountResult(workspace, root string, volumeCount int, deleteLocal bool) {
	local := "preserved"
	if deleteLocal {
		local = "deleted"
	}
	printSection("Agent Workspace unmounted", []outputRow{
		{Label: "workspace", Value: workspace},
		{Label: "path", Value: homeRelativeDisplayPath(root)},
		{Label: "volumes", Value: fmt.Sprintf("%d", volumeCount)},
		{Label: "local", Value: local},
	})
}

func unmountWorkspaceCompositionByRef(ref string, deleteLocal bool) error {
	reg, err := loadMountRegistry()
	if err != nil {
		return err
	}
	matches := workspaceCompositionMountedRootsForRef(reg, ref)
	switch len(matches) {
	case 0:
		return fmt.Errorf("no mounted Agent Workspace found for %s", ref)
	case 1:
		return unmountWorkspaceCompositionRoot(matches[0].Root, deleteLocal)
	default:
		paths := make([]string, 0, len(matches))
		for _, match := range matches {
			paths = append(paths, homeRelativeDisplayPath(match.Root))
		}
		return fmt.Errorf("Agent Workspace %s matches multiple mounted roots: %s\nRun '%s unmount <directory>' to choose one.", ref, strings.Join(paths, ", "), filepath.Base(os.Args[0]))
	}
}

func promptWorkspaceCompositionUnmountSelection(deleteLocal bool) error {
	reg, err := loadMountRegistry()
	if err != nil {
		return err
	}
	roots := workspaceCompositionMountedRoots(reg)
	if len(roots) == 0 {
		fmt.Println()
		fmt.Println("No mounted Agent Workspaces.")
		fmt.Println()
		return nil
	}

	fmt.Println()
	fmt.Println("Unmount Agent Workspace")
	fmt.Println()
	headers := []string{"#", "Workspace", "Volumes", "Path"}
	printPlainTable(headers, workspaceCompositionUnmountPromptRows(roots))
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
	idx, err := strconv.Atoi(choice)
	if err != nil || idx < 1 || idx > len(roots) {
		return fmt.Errorf("invalid selection %q", choice)
	}
	return unmountWorkspaceCompositionRoot(roots[idx-1].Root, deleteLocal)
}

type workspaceCompositionMountedRoot struct {
	WorkspaceName string
	WorkspaceID   string
	Root          string
	VolumeRows    int
}

func workspaceCompositionMountedRoots(reg mountRegistry) []workspaceCompositionMountedRoot {
	byKey := make(map[string]*workspaceCompositionMountedRoot)
	for _, rec := range sortedMountRecords(reg.Mounts) {
		root := strings.TrimSpace(rec.AgentWorkspaceRoot)
		name := strings.TrimSpace(rec.AgentWorkspace)
		id := strings.TrimSpace(rec.AgentWorkspaceID)
		if root == "" || (name == "" && id == "") {
			continue
		}
		key := id + "\x00" + name + "\x00" + filepath.Clean(root)
		item, ok := byKey[key]
		if !ok {
			item = &workspaceCompositionMountedRoot{
				WorkspaceName: name,
				WorkspaceID:   id,
				Root:          filepath.Clean(root),
			}
			byKey[key] = item
		}
		item.VolumeRows++
	}
	roots := make([]workspaceCompositionMountedRoot, 0, len(byKey))
	for _, item := range byKey {
		roots = append(roots, *item)
	}
	sort.Slice(roots, func(i, j int) bool {
		left := strings.ToLower(roots[i].WorkspaceName)
		right := strings.ToLower(roots[j].WorkspaceName)
		if left == right {
			return roots[i].Root < roots[j].Root
		}
		return left < right
	})
	return roots
}

func workspaceCompositionMountedRootsForRef(reg mountRegistry, ref string) []workspaceCompositionMountedRoot {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}
	matches := make([]workspaceCompositionMountedRoot, 0)
	for _, root := range workspaceCompositionMountedRoots(reg) {
		if root.WorkspaceName == ref || root.WorkspaceID == ref {
			matches = append(matches, root)
		}
	}
	return matches
}

func workspaceCompositionUnmountPromptRows(roots []workspaceCompositionMountedRoot) [][]string {
	rows := make([][]string, 0, len(roots))
	for i, root := range roots {
		rows = append(rows, []string{
			strconv.Itoa(i + 1),
			root.WorkspaceName,
			fmt.Sprintf("%d", root.VolumeRows),
			homeRelativeDisplayPath(root.Root),
		})
	}
	return rows
}

func workspaceCompositionLocalMountPath(root, mountPath string) string {
	clean := strings.TrimSpace(mountPath)
	if clean == "" || clean == "/" {
		return root
	}
	return filepath.Join(root, strings.TrimPrefix(clean, "/"))
}

func workspaceCompositionSessionName(prefix, workspaceName, mountPath string) string {
	base := strings.TrimSpace(prefix)
	if base == "" {
		base = strings.TrimSpace(workspaceName)
	}
	path := strings.TrimSpace(mountPath)
	if path == "" {
		path = "/"
	}
	if base == "" {
		return path
	}
	return base + " " + path
}

func mountRecordUnderRoot(rec mountRecord, root string) bool {
	path := strings.TrimSpace(rec.LocalPath)
	if path == "" {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	if abs == root {
		return true
	}
	rel, err := filepath.Rel(root, abs)
	return err == nil && rel != "." && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)
}

func resolveWorkspaceCompositionForCommand(ctx context.Context, service workspaceCompositionControlPlane, workspace string) (controlplane.WorkspaceCompositionDetail, error) {
	ref := strings.TrimSpace(workspace)
	if ref != "" {
		detail, err := service.GetWorkspaceComposition(ctx, ref)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return controlplane.WorkspaceCompositionDetail{}, workspaceCompositionNotFoundError(ref)
			}
			return controlplane.WorkspaceCompositionDetail{}, err
		}
		return detail, nil
	}
	return promptWorkspaceCompositionSelection(ctx, service)
}

func promptWorkspaceCompositionSelection(ctx context.Context, service workspaceCompositionControlPlane) (controlplane.WorkspaceCompositionDetail, error) {
	response, err := service.ListWorkspaceCompositions(ctx)
	if err != nil {
		return controlplane.WorkspaceCompositionDetail{}, err
	}
	if len(response.Items) == 0 {
		return controlplane.WorkspaceCompositionDetail{}, fmt.Errorf("no Agent Workspaces found\nCreate one with: %s ws create <workspace>", filepath.Base(os.Args[0]))
	}

	fmt.Println()
	fmt.Println("Select Agent Workspace")
	fmt.Println()
	headers := []string{"#", "Workspace", "Volumes", "Updated"}
	rows := make([][]string, 0, len(response.Items))
	for i, item := range response.Items {
		rows = append(rows, []string{
			strconv.Itoa(i + 1),
			item.Name,
			strconv.Itoa(item.MountCount),
			item.UpdatedAt,
		})
	}
	printPlainTable(headers, rows)
	fmt.Println()
	fmt.Print("Workspace: ")

	reader := bufio.NewReader(os.Stdin)
	raw, err := reader.ReadString('\n')
	if err != nil && strings.TrimSpace(raw) == "" {
		fmt.Println()
		return controlplane.WorkspaceCompositionDetail{}, errors.New("workspace selection cancelled")
	}
	choiceText := strings.TrimSpace(raw)
	if choiceText == "" {
		fmt.Println()
		return controlplane.WorkspaceCompositionDetail{}, errors.New("workspace selection cancelled")
	}
	idx, err := strconv.Atoi(choiceText)
	if err != nil || idx < 1 || idx > len(response.Items) {
		return controlplane.WorkspaceCompositionDetail{}, fmt.Errorf("invalid selection %q", choiceText)
	}
	fmt.Println()
	selected := response.Items[idx-1]
	ref := firstNonEmptyString(selected.ID, selected.Name)
	return service.GetWorkspaceComposition(ctx, ref)
}

func resolveWorkspaceAttachVolume(ctx context.Context, service workspaceCompositionControlPlane, volume string) (workspaceSelection, error) {
	ref := strings.TrimSpace(volume)
	if ref == "" {
		response, err := service.ListWorkspaceSummaries(ctx)
		if err != nil {
			return workspaceSelection{}, err
		}
		return promptWorkspaceSelectionFromSummaries(response.Items)
	}
	selection, ok, err := resolveWorkspaceVolumeSelection(ctx, service, ref)
	if err != nil {
		return workspaceSelection{}, err
	}
	if !ok {
		return workspaceSelection{}, fmt.Errorf("volume %q does not exist\nRun '%s vol list' to see volumes, or create one with '%s vol import <volume> <directory>'.", ref, filepath.Base(os.Args[0]), filepath.Base(os.Args[0]))
	}
	return selection, nil
}

func resolveWorkspaceVolumeSelection(ctx context.Context, service workspaceCompositionControlPlane, volume string) (workspaceSelection, bool, error) {
	response, err := service.ListWorkspaceSummaries(ctx)
	if err != nil {
		return workspaceSelection{}, false, err
	}
	return matchVolumeSelection(volume, response.Items)
}

func matchVolumeSelection(ref string, volumes []workspaceSummary) (workspaceSelection, bool, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return workspaceSelection{}, false, nil
	}

	idMatches := make([]workspaceSummary, 0, 1)
	nameMatches := make([]workspaceSummary, 0)
	for _, volume := range volumes {
		if volume.ID == ref {
			idMatches = append(idMatches, volume)
		}
		if volume.Name == ref {
			nameMatches = append(nameMatches, volume)
		}
	}

	switch {
	case len(nameMatches) > 1:
		return workspaceSelection{}, false, fmt.Errorf("volume %q exists multiple times; use a volume id instead: %s", ref, strings.Join(workspaceSelectionLabels(nameMatches), ", "))
	case len(idMatches) == 1:
		return workspaceSelection{ID: idMatches[0].ID, Name: idMatches[0].Name}, true, nil
	case len(idMatches) > 1:
		return workspaceSelection{}, false, fmt.Errorf("volume id %q is ambiguous; choose one of: %s", ref, strings.Join(workspaceSelectionLabels(idMatches), ", "))
	}

	switch len(nameMatches) {
	case 0:
		return workspaceSelection{}, false, nil
	case 1:
		return workspaceSelection{ID: nameMatches[0].ID, Name: nameMatches[0].Name}, true, nil
	default:
		return workspaceSelection{}, false, nil
	}
}

func defaultWorkspaceVolumeMountPath(volumeName string) string {
	value := strings.Trim(strings.TrimSpace(volumeName), "/")
	if value == "" {
		return "/"
	}
	return "/" + strings.ReplaceAll(value, "\\", "/")
}

func normalizeWorkspaceVolumeMountPathForCLI(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		value = "/"
	}
	if strings.Contains(value, "\x00") {
		return "", fmt.Errorf("mount path contains an invalid character")
	}
	value = strings.ReplaceAll(value, "\\", "/")
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	clean := path.Clean(value)
	if clean == "." {
		clean = "/"
	}
	for _, part := range strings.Split(strings.Trim(clean, "/"), "/") {
		if part == ".." {
			return "", fmt.Errorf("mount path %q escapes the workspace root", raw)
		}
	}
	return clean, nil
}

func validateWorkspaceAddPathAvailable(mounts []controlplane.WorkspaceCompositionMount, mountPath string) error {
	for _, mount := range mounts {
		existing, err := normalizeWorkspaceVolumeMountPathForCLI(mount.MountPath)
		if err != nil {
			return err
		}
		if existing == mountPath {
			return fmt.Errorf("workspace path %q is already used by volume %q\nUse --at <path> to choose a different path.", mountPath, workspaceCompositionMountVolumeLabel(mount))
		}
		if workspaceVolumeMountPathAncestor(existing, mountPath) || workspaceVolumeMountPathAncestor(mountPath, existing) {
			return fmt.Errorf("workspace path %q overlaps existing path %q\nUse --at <path> to choose a non-overlapping path.", mountPath, existing)
		}
	}
	return nil
}

func workspaceVolumeMountPathAncestor(parent, child string) bool {
	parent = strings.TrimSuffix(parent, "/")
	child = strings.TrimSuffix(child, "/")
	if parent == "" {
		parent = "/"
	}
	if parent == child {
		return true
	}
	if parent == "/" {
		return child != "/"
	}
	return strings.HasPrefix(child, parent+"/")
}

func workspaceCompositionMountVolumeLabel(mount controlplane.WorkspaceCompositionMount) string {
	if name := strings.TrimSpace(mount.VolumeName); name != "" {
		return name
	}
	return strings.TrimSpace(mount.VolumeID)
}

type workspaceManifestCreateArgs struct {
	name        string
	description string
	jsonOut     bool
}

func parseWorkspaceManifestCreateArgs(args []string) (workspaceManifestCreateArgs, error) {
	var opts workspaceManifestCreateArgs
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--json":
			opts.jsonOut = true
		case arg == "--description":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("missing value for --description")
			}
			i++
			opts.description = strings.TrimSpace(args[i])
		case strings.HasPrefix(arg, "--description="):
			opts.description = strings.TrimSpace(strings.TrimPrefix(arg, "--description="))
		case strings.HasPrefix(arg, "-"):
			return opts, fmt.Errorf("unknown flag %q", arg)
		default:
			if opts.name != "" {
				return opts, fmt.Errorf("unexpected argument %q", arg)
			}
			opts.name = arg
		}
	}
	return opts, nil
}

type workspaceAddVolumeArgs struct {
	workspace  string
	sourceDir  string
	volumeName string
	mountPath  string
	readonly   bool
}

func parseWorkspaceAddVolumeArgs(args []string) (workspaceAddVolumeArgs, error) {
	var opts workspaceAddVolumeArgs
	positionals := make([]string, 0, 2)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--readonly" || arg == "--read-only":
			opts.readonly = true
		case arg == "--name":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("missing value for --name")
			}
			i++
			opts.volumeName = strings.TrimSpace(args[i])
		case strings.HasPrefix(arg, "--name="):
			opts.volumeName = strings.TrimSpace(strings.TrimPrefix(arg, "--name="))
		case arg == "--at":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("missing value for --at")
			}
			i++
			opts.mountPath = strings.TrimSpace(args[i])
		case strings.HasPrefix(arg, "--at="):
			opts.mountPath = strings.TrimSpace(strings.TrimPrefix(arg, "--at="))
		case strings.HasPrefix(arg, "-"):
			return opts, fmt.Errorf("unknown flag %q", arg)
		default:
			positionals = append(positionals, arg)
		}
	}
	if len(positionals) > 2 {
		return opts, fmt.Errorf("too many arguments")
	}
	if len(positionals) > 0 {
		opts.workspace = positionals[0]
	}
	if len(positionals) > 1 {
		opts.sourceDir = positionals[1]
	}
	return opts, nil
}

type workspaceAttachVolumeArgs struct {
	workspace     string
	volume        string
	mountPath     string
	readonly      bool
	volumeTokenID string
	jsonOut       bool
}

func parseWorkspaceAttachVolumeArgs(args []string) (workspaceAttachVolumeArgs, error) {
	var opts workspaceAttachVolumeArgs
	positionals := make([]string, 0, 2)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--json":
			opts.jsonOut = true
		case arg == "--readonly" || arg == "--read-only":
			opts.readonly = true
		case arg == "--at":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("missing value for --at")
			}
			i++
			opts.mountPath = strings.TrimSpace(args[i])
		case strings.HasPrefix(arg, "--at="):
			opts.mountPath = strings.TrimSpace(strings.TrimPrefix(arg, "--at="))
		case arg == "--token":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("missing value for --token")
			}
			i++
			opts.volumeTokenID = strings.TrimSpace(args[i])
		case strings.HasPrefix(arg, "--token="):
			opts.volumeTokenID = strings.TrimSpace(strings.TrimPrefix(arg, "--token="))
		case strings.HasPrefix(arg, "-"):
			return opts, fmt.Errorf("unknown flag %q", arg)
		default:
			positionals = append(positionals, arg)
		}
	}
	if len(positionals) > 2 {
		return opts, fmt.Errorf("too many arguments")
	}
	if len(positionals) > 0 {
		opts.workspace = positionals[0]
	}
	if len(positionals) > 1 {
		opts.volume = positionals[1]
	}
	return opts, nil
}

type workspaceBookmarkArgs struct {
	workspace   string
	name        string
	description string
	jsonOut     bool
}

func parseWorkspaceBookmarkArgs(args []string) (workspaceBookmarkArgs, error) {
	var opts workspaceBookmarkArgs
	positionals := make([]string, 0, 2)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--json":
			opts.jsonOut = true
		case arg == "--description":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("missing value for --description")
			}
			i++
			opts.description = strings.TrimSpace(args[i])
		case strings.HasPrefix(arg, "--description="):
			opts.description = strings.TrimSpace(strings.TrimPrefix(arg, "--description="))
		case strings.HasPrefix(arg, "-"):
			return opts, fmt.Errorf("unknown flag %q", arg)
		default:
			positionals = append(positionals, arg)
		}
	}
	if len(positionals) > 2 {
		return opts, fmt.Errorf("too many arguments")
	}
	if len(positionals) > 0 {
		opts.workspace = positionals[0]
	}
	if len(positionals) > 1 {
		opts.name = positionals[1]
	}
	return opts, nil
}

func parseOnlyJSONFlag(args []string) (bool, error) {
	jsonOut := false
	for _, arg := range args {
		if arg == "--json" {
			jsonOut = true
			continue
		}
		return false, fmt.Errorf("unknown flag %q", arg)
	}
	return jsonOut, nil
}

func parseJSONFlagWithPositionals(args []string) ([]string, bool, error) {
	jsonOut := false
	positionals := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--json" {
			jsonOut = true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return nil, false, fmt.Errorf("unknown flag %q", arg)
		}
		positionals = append(positionals, arg)
	}
	return positionals, jsonOut, nil
}

func writeJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func printWorkspaceCompositionDetail(detail controlplane.WorkspaceCompositionDetail) {
	printSection(clr(ansiBold, "workspace manifest"), []outputRow{
		{Label: "workspace", Value: detail.Name},
		{Label: "id", Value: detail.ID},
		{Label: "volumes", Value: fmt.Sprintf("%d", len(detail.Mounts))},
		{Label: "bookmarks", Value: fmt.Sprintf("%d", len(detail.Bookmarks))},
	})
	if len(detail.Mounts) > 0 {
		fmt.Fprintln(os.Stdout, "")
		fmt.Fprintf(os.Stdout, "%-28s  %-18s  %s\n", "Volume", "Mode", "Path")
		for _, mount := range detail.Mounts {
			mode := "rw"
			if mount.Readonly {
				mode = "ro"
			}
			name := mount.VolumeName
			if strings.TrimSpace(name) == "" {
				name = mount.VolumeID
			}
			fmt.Fprintf(os.Stdout, "%-28s  %-18s  %s\n", name, mode, mount.MountPath)
		}
	}
}

func workspaceManifestCreateUsage(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s ws create [--description <text>] [--json] <workspace>

Create an Agent Workspace manifest that can mount one or more volumes.
`, bin)
}

func workspaceManifestListUsage(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s ws list [--json]

List Agent Workspace manifests.
`, bin)
}

func workspaceManifestShowUsage(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s ws show [--json] <workspace>

Show attached volumes and bookmarks for an Agent Workspace.
`, bin)
}

func workspaceAddVolumeUsage(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s ws add [--name <volume>] [--at <mount-path>] [--readonly] <workspace> <directory>

Import a folder into a new volume, and attach it to the workspace.
By default, the volume name comes from the folder name and the workspace path is /<volume>.
`, bin)
}

func workspaceAttachVolumeUsage(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s ws attach [--at <mount-path>] [--readonly] [--token <volume-token>] [--json] [<workspace> [volume]]

Attach an existing volume to an Agent Workspace. If volume is omitted, AFS prompts for one.
By default, the workspace path is /<volume>.
`, bin)
}

func workspaceDetachVolumeUsage(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s ws detach [--json] <workspace> <volume>

Detach a volume from an Agent Workspace manifest.
`, bin)
}

func workspaceBookmarkUsage(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s ws bookmark create [--description <text>] [--json] <workspace> <name>
  %s ws bookmark list [--json] <workspace>
  %s ws bookmark restore [--json] <workspace> <name>

Capture each attached volume's current checkpoint.
`, bin, bin, bin)
}

func workspaceBookmarkListUsage(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s ws bookmark list [--json] <workspace>

List Agent Workspace bookmarks.
`, bin)
}

func workspaceRestoreBookmarkUsage(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s ws bookmark restore [--json] <workspace> <name>

Restore attached volumes to the checkpoints captured by a bookmark.
`, bin)
}

func workspaceCompositionMountUsage(bin string) string {
	return workspaceCompositionMountUsageFor(bin + " ws mount")
}

func workspaceCompositionMountUsageFor(command string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s [--dry-run] [--yes] [--readonly] [--verbose] [--session <name>] [<workspace> [directory]]

Mount the workspace under a local root.
Each manifest mount path becomes a mounted volume directory under that root.
With no workspace, lists Agent Workspaces and prompts for a selection.
With no directory, prompts for a local folder.
`, command)
}

func workspaceCompositionUnmountUsage(bin string) string {
	return workspaceCompositionUnmountUsageFor(bin + " ws unmount")
}

func workspaceCompositionUnmountUsageFor(command string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s [--delete] [<workspace|workspace-root-directory>]

Unmount volume mounts under a local Agent Workspace root directory.
With no target, lists mounted Agent Workspaces and prompts for a selection.
`, command)
}
