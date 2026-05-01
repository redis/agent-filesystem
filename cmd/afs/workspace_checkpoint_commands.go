package main

import (
	"bufio"
	"context"
	"encoding/json"
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
		fmt.Fprint(os.Stderr, workspaceUsageTextFor(filepath.Base(os.Args[0]), args[0]))
		return nil
	}

	switch args[1] {
	case "create":
		return cmdWorkspaceCreate(args)
	case "list":
		return cmdWorkspaceList(args)
	case "info":
		return cmdWorkspaceInfo(args)
	case "mount":
		return cmdMountArgs(args[2:])
	case "unmount":
		return cmdUnmountArgs(args[2:])
	case "fork":
		return cmdWorkspaceFork(args)
	case "delete":
		return cmdWorkspaceDelete(args)
	case "import":
		return cmdWorkspaceImport(args)
	default:
		return fmt.Errorf("unknown workspace subcommand %q\n\n%s", args[1], workspaceUsageTextFor(filepath.Base(os.Args[0]), args[0]))
	}
}

func cmdCheckpoint(args []string) error {
	if len(args) < 2 || isHelpArg(args[1]) {
		group := "cp"
		if len(args) > 0 {
			group = args[0]
		}
		fmt.Fprint(os.Stderr, checkpointUsageTextFor(filepath.Base(os.Args[0]), group))
		return nil
	}

	switch args[1] {
	case "create":
		return cmdCheckpointCreate(args)
	case "list":
		return cmdCheckpointList(args)
	case "show":
		return cmdCheckpointShow(args)
	case "diff":
		return cmdCheckpointDiff(args)
	case "restore":
		return cmdCheckpointRestore(args)
	default:
		return fmt.Errorf("unknown checkpoint subcommand %q\n\n%s", args[1], checkpointUsageTextFor(filepath.Base(os.Args[0]), args[0]))
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

	next := filepath.Base(os.Args[0]) + " ws mount " + workspace + " <directory>"
	if productMode, _ := effectiveProductMode(cfg); productMode != productModeLocal {
		next = filepath.Base(os.Args[0]) + " ws mount " + workspace + " <directory>"
	}

	printSection(markerSuccess+" "+clr(ansiBold, "workspace created"), []outputRow{
		{Label: "workspace", Value: workspace},
		{Label: "checkpoint", Value: afsInitialCheckpointName},
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

	fmt.Println()
	fmt.Println("Workspaces")
	fmt.Println()
	if len(workspaces.Items) == 0 {
		fmt.Println("No workspaces found")
	} else {
		printPlainTable(
			[]string{"Workspace", "Database", "ID", "Updated", "Mounted"},
			workspaceSummaryTableRows(cfg, workspaces.Items, workspaceListMounts()),
		)
	}
	fmt.Println()
	return nil
}

func cmdWorkspaceInfo(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, workspaceInfoUsageText(filepath.Base(os.Args[0]), args[0]))
		return nil
	}
	if len(args) != 3 {
		fmt.Fprint(os.Stderr, workspaceInfoUsageText(filepath.Base(os.Args[0]), args[0]))
		return nil
	}

	cfg, service, closeStore, err := openAFSControlPlane(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	selection, err := resolveWorkspaceSelectionFromControlPlane(context.Background(), cfg, service, args[2])
	if err != nil {
		return err
	}
	detail, err := service.GetWorkspace(context.Background(), selection.ID)
	if err != nil {
		return err
	}
	rows := []outputRow{
		{Label: "workspace", Value: detail.Name},
		{Label: "id", Value: detail.ID},
		{Label: "files", Value: strconv.Itoa(detail.FileCount)},
		{Label: "folders", Value: strconv.Itoa(detail.FolderCount)},
		{Label: "checkpoints", Value: strconv.Itoa(detail.CheckpointCount)},
	}
	if strings.TrimSpace(detail.HeadCheckpointID) != "" {
		rows = append(rows, outputRow{Label: "head", Value: detail.HeadCheckpointID})
	}
	if strings.TrimSpace(detail.DraftState) != "" {
		rows = append(rows, outputRow{Label: "state", Value: detail.DraftState})
	}
	printSection("Workspace", rows)
	return nil
}

func workspaceSummaryTableRows(cfg config, items []workspaceSummary, mounts map[string]string) [][]string {
	rows := make([][]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, []string{
			workspaceListName(item.Name),
			workspaceListDatabase(item),
			workspaceListID(item),
			workspaceListUpdated(item),
			workspaceListMounted(item, mounts),
		})
	}
	return rows
}

func workspaceListName(name string) string {
	return clr(ansiBold+ansiWhite, name)
}

func workspaceListMarker(selected bool) string {
	if selected {
		return clr(ansiBGreen, "✓")
	}
	return " "
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

func workspaceListMounts() map[string]string {
	reg, err := loadMountRegistry()
	if err != nil || len(reg.Mounts) == 0 {
		return nil
	}
	paths := make(map[string][]string)
	for _, rec := range sortedMountRecords(reg.Mounts) {
		path := strings.TrimSpace(rec.LocalPath)
		if path == "" {
			continue
		}
		display := workspaceListMountedPath(path)
		if id := strings.TrimSpace(rec.WorkspaceID); id != "" {
			paths["id:"+id] = append(paths["id:"+id], display)
		}
		if name := strings.TrimSpace(rec.Workspace); name != "" {
			paths["name:"+name] = append(paths["name:"+name], display)
		}
	}
	out := make(map[string]string, len(paths))
	for key, values := range paths {
		out[key] = strings.Join(values, ", ")
	}
	return out
}

func workspaceListMounted(summary workspaceSummary, mounts map[string]string) string {
	if len(mounts) == 0 {
		return "-"
	}
	if id := strings.TrimSpace(summary.ID); id != "" {
		if path := strings.TrimSpace(mounts["id:"+id]); path != "" {
			return path
		}
	}
	if name := strings.TrimSpace(summary.Name); name != "" {
		if path := strings.TrimSpace(mounts["name:"+name]); path != "" {
			return path
		}
	}
	return "-"
}

func workspaceListMountedPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "-"
	}
	home, err := os.UserHomeDir()
	if err == nil {
		home = filepath.Clean(home)
		clean := filepath.Clean(path)
		if clean == home {
			return "~"
		}
		if rel, err := filepath.Rel(home, clean); err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." {
			return filepath.Join("~", rel)
		}
	}
	return path
}

func cmdWorkspaceDelete(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, workspaceDeleteUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	opts, err := parseWorkspaceDeleteArgs(args[2:])
	if err != nil {
		return err
	}
	if len(opts.names) == 0 {
		return fmt.Errorf("%s", workspaceDeleteUsageText(filepath.Base(os.Args[0])))
	}
	names := opts.names
	if !opts.noConfirmation {
		ok, err := confirmWorkspaceDelete(names)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Println()
			fmt.Println("Delete cancelled.")
			fmt.Println()
			return nil
		}
	}

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

	rows := make([]outputRow, 0, len(deleted)+1)
	rows = append(rows, outputRow{Label: "deleted", Value: strconv.Itoa(len(deleted))})
	rows = append(rows, outputRow{})
	for _, name := range deleted {
		rows = append(rows, outputRow{Value: name})
	}
	printSection(markerSuccess+" "+clr(ansiBold, "workspaces deleted"), rows)
	return nil
}

type workspaceDeleteOptions struct {
	names          []string
	noConfirmation bool
}

func parseWorkspaceDeleteArgs(args []string) (workspaceDeleteOptions, error) {
	var opts workspaceDeleteOptions
	for _, arg := range args {
		switch arg {
		case "--no-confirmation", "--yes", "-y":
			opts.noConfirmation = true
		default:
			if strings.HasPrefix(arg, "-") {
				return opts, fmt.Errorf("unknown flag %q\n\n%s", arg, workspaceDeleteUsageText(filepath.Base(os.Args[0])))
			}
			opts.names = append(opts.names, arg)
		}
	}
	return opts, nil
}

func confirmWorkspaceDelete(names []string) (bool, error) {
	if len(names) == 0 {
		return false, nil
	}
	target := names[0]
	if len(names) > 1 {
		target = strings.Join(names, ", ")
	}
	fmt.Println()
	fmt.Printf("Are you sure you want to delete %s? [y/N] ", target)
	reader := bufio.NewReader(os.Stdin)
	raw, err := reader.ReadString('\n')
	if err != nil && strings.TrimSpace(raw) == "" {
		fmt.Println()
		return false, nil
	}
	fmt.Println()
	answer := strings.ToLower(strings.TrimSpace(raw))
	return answer == "y" || answer == "yes", nil
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

	printSection(markerSuccess+" "+clr(ansiBold, "workspace forked"), []outputRow{
		{Label: "workspace", Value: newWorkspace},
		{Label: "source", Value: sourceSelection.Name},
		{Label: "next", Value: filepath.Base(os.Args[0]) + " ws mount " + newWorkspace + " <directory>"},
	})
	return nil
}

func cmdWorkspaceImport(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, workspaceImportUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	importArgs := []string{"import"}
	for _, arg := range args[2:] {
		importArgs = append(importArgs, arg)
	}
	if len(importArgs) < 3 {
		return fmt.Errorf("%s", workspaceImportUsageText(filepath.Base(os.Args[0])))
	}

	if err := cmdImport(importArgs); err != nil {
		return err
	}
	return nil
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
	selection, err := resolveCheckpointWorkspaceSelection(context.Background(), cfg, service, workspace)
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
	fmt.Println()
	fmt.Println("checkpoints in workspace: " + selection.Name)
	fmt.Println()
	if len(checkpoints) == 0 {
		fmt.Println("No checkpoints found")
	} else {
		printPlainTable(
			[]string{"Checkpoint", "Active", "Created", "Size"},
			checkpointTableRows(checkpoints, activeCheckpointID),
		)
	}
	fmt.Println()
	return nil
}

func resolveCheckpointWorkspaceSelection(ctx context.Context, cfg config, service afsControlPlane, workspace string) (workspaceSelection, error) {
	if strings.TrimSpace(workspace) != "" {
		return resolveWorkspaceSelectionFromControlPlane(ctx, cfg, service, workspace)
	}
	return promptCheckpointWorkspaceSelection(ctx, service)
}

func promptCheckpointWorkspaceSelection(ctx context.Context, service afsControlPlane) (workspaceSelection, error) {
	workspaces, err := service.ListWorkspaceSummaries(ctx)
	if err != nil {
		return workspaceSelection{}, err
	}
	if len(workspaces.Items) == 0 {
		return workspaceSelection{}, fmt.Errorf("no workspaces found\nCreate one with: %s ws create <workspace>", filepath.Base(os.Args[0]))
	}

	fmt.Println()
	fmt.Println("Select workspace")
	fmt.Println()
	printPlainTable([]string{"#", "Workspace", "Updated", "Mounted"}, checkpointWorkspacePromptRows(workspaces.Items, workspaceListMounts()))
	fmt.Println()
	fmt.Print("Workspace: ")

	reader := bufio.NewReader(os.Stdin)
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
	if err != nil || idx < 1 || idx > len(workspaces.Items) {
		return workspaceSelection{}, fmt.Errorf("invalid selection %q", choiceText)
	}
	fmt.Println()
	selected := workspaces.Items[idx-1]
	return workspaceSelection{ID: selected.ID, Name: selected.Name}, nil
}

func checkpointWorkspacePromptRows(workspaces []workspaceSummary, mounts map[string]string) [][]string {
	rows := make([][]string, 0, len(workspaces))
	for i, workspace := range workspaces {
		rows = append(rows, []string{
			strconv.Itoa(i + 1),
			workspace.Name,
			workspaceListUpdated(workspace),
			workspaceListMounted(workspace, mounts),
		})
	}
	return rows
}

func checkpointTableRows(items []controlplane.CheckpointSummary, activeCheckpointID string) [][]string {
	rows := make([][]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, []string{
			checkpointListName(item),
			checkpointListActive(item, activeCheckpointID),
			checkpointListCreated(item),
			checkpointListSize(item),
		})
	}
	return rows
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

type checkpointShowArgs struct {
	workspace    string
	checkpointID string
	json         bool
}

func cmdCheckpointShow(args []string) error {
	for _, arg := range args[2:] {
		if isHelpArg(arg) {
			fmt.Fprint(os.Stderr, checkpointShowUsageText(filepath.Base(os.Args[0])))
			return nil
		}
	}
	parsed, err := parseCheckpointShowArgs(args[2:])
	if err != nil {
		return fmt.Errorf("%s\n\n%s", err, checkpointShowUsageText(filepath.Base(os.Args[0])))
	}

	cfg, service, closeStore, err := openAFSControlPlane(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	selection, err := resolveCheckpointWorkspaceSelection(context.Background(), cfg, service, parsed.workspace)
	if err != nil {
		return err
	}
	detail, err := service.GetCheckpoint(context.Background(), selection.ID, parsed.checkpointID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("checkpoint %q does not exist", parsed.checkpointID)
		}
		return err
	}
	if parsed.json {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(detail)
	}

	activeCheckpointID := ""
	workspaceDetail, err := service.GetWorkspace(context.Background(), selection.ID)
	if err == nil && workspaceDetail.DraftState != "dirty" {
		activeCheckpointID = workspaceDetail.HeadCheckpointID
	}
	printCheckpointShow(selection.Name, detail, activeCheckpointID)
	return nil
}

func parseCheckpointShowArgs(args []string) (checkpointShowArgs, error) {
	var parsed checkpointShowArgs
	positionals := make([]string, 0, len(args))
	for _, arg := range args {
		switch arg {
		case "--json":
			parsed.json = true
		default:
			if strings.HasPrefix(arg, "-") {
				return parsed, fmt.Errorf("unknown flag %q", arg)
			}
			positionals = append(positionals, arg)
		}
	}
	switch len(positionals) {
	case 1:
		parsed.checkpointID = positionals[0]
	case 2:
		parsed.workspace = positionals[0]
		parsed.checkpointID = positionals[1]
	default:
		return parsed, fmt.Errorf("expected [workspace] <checkpoint>")
	}
	if err := validateAFSName("checkpoint", parsed.checkpointID); err != nil {
		return parsed, err
	}
	return parsed, nil
}

func printCheckpointShow(workspace string, detail controlplane.CheckpointDetail, activeCheckpointID string) {
	rows := []outputRow{
		{Label: "workspace", Value: workspace},
		{Label: "checkpoint", Value: detail.ID},
		{Label: "active", Value: yesNo(detail.ID != "" && detail.ID == activeCheckpointID)},
		{Label: "created", Value: formatDisplayTimestamp(detail.CreatedAt)},
		{Label: "author", Value: checkpointDisplayDefault(detail.Author, "afs")},
	}
	if strings.TrimSpace(detail.Description) != "" {
		rows = append(rows, outputRow{Label: "description", Value: detail.Description})
	}
	if strings.TrimSpace(detail.Kind) != "" {
		rows = append(rows, outputRow{Label: "kind", Value: detail.Kind})
	}
	if strings.TrimSpace(detail.Source) != "" {
		rows = append(rows, outputRow{Label: "source", Value: detail.Source})
	}
	if actor := checkpointDetailActor(detail); actor != "" {
		rows = append(rows, outputRow{Label: "actor", Value: actor})
	}
	if strings.TrimSpace(detail.SessionID) != "" {
		rows = append(rows, outputRow{Label: "session", Value: detail.SessionID})
	}
	if strings.TrimSpace(detail.ParentCheckpointID) != "" {
		rows = append(rows, outputRow{Label: "parent", Value: detail.ParentCheckpointID})
	}
	rows = append(rows,
		outputRow{Label: "files", Value: strconv.Itoa(detail.FileCount)},
		outputRow{Label: "folders", Value: strconv.Itoa(detail.FolderCount)},
		outputRow{Label: "size", Value: formatBytes(detail.TotalBytes)},
		outputRow{Label: "changes", Value: checkpointDiffSummary(detail.ChangeSummary)},
	)
	if strings.TrimSpace(detail.ManifestHash) != "" {
		rows = append(rows, outputRow{Label: "manifest", Value: detail.ManifestHash})
	}
	printSection(clr(ansiBold, "checkpoint"), rows)
}

func checkpointDetailActor(detail controlplane.CheckpointDetail) string {
	switch {
	case strings.TrimSpace(detail.AgentName) != "":
		return strings.TrimSpace(detail.AgentName)
	case strings.TrimSpace(detail.AgentID) != "":
		return strings.TrimSpace(detail.AgentID)
	case strings.TrimSpace(detail.CreatedBy) != "":
		return strings.TrimSpace(detail.CreatedBy)
	default:
		return ""
	}
}

func checkpointDisplayDefault(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

type checkpointDiffArgs struct {
	workspace     string
	base          string
	head          string
	compareActive bool
	json          bool
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

	selection, err := resolveCheckpointWorkspaceSelection(context.Background(), cfg, service, parsed.workspace)
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
	if parsed.json {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(diff)
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
		case "--json":
			parsed.json = true
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
	rows := []outputRow{
		{Label: "workspace", Value: workspace},
		{Label: "base", Value: checkpointDiffDisplayView(diff.Base)},
		{Label: "target", Value: checkpointDiffDisplayView(diff.Head)},
		{Label: "changes", Value: checkpointDiffSummary(summary)},
	}
	if len(diff.Entries) == 0 {
		rows = append(rows, outputRow{})
		rows = append(rows, outputRow{Value: clr(ansiDim, "No changes")})
		printSection(clr(ansiBold, "checkpoint diff"), rows)
		return
	}
	rows = append(rows, outputRow{})
	limit := len(diff.Entries)
	if limit > 100 {
		limit = 100
	}
	for _, entry := range diff.Entries[:limit] {
		rows = append(rows, outputRow{
			Label: checkpointDiffOpLabel(entry.Op),
			Value: checkpointDiffEntryValue(entry),
		})
	}
	if extra := len(diff.Entries) - limit; extra > 0 {
		rows = append(rows, outputRow{Value: fmt.Sprintf("%d more changes not shown", extra)})
	}
	printSection(clr(ansiBold, "checkpoint diff"), rows)
	printCheckpointDiffText(diff)
}

func checkpointDiffDisplayView(state controlplane.DiffState) string {
	if state.CheckpointID != "" {
		return state.CheckpointID
	}
	if state.View == "working-copy" || state.View == "head" {
		return "workspace"
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

func printCheckpointDiffText(diff controlplane.WorkspaceDiffResponse) {
	const (
		maxTextFiles = 12
		maxTextLines = 600
	)
	filesShown := 0
	linesShown := 0
	for _, entry := range diff.Entries {
		if entry.TextDiff == nil {
			continue
		}
		if filesShown >= maxTextFiles || linesShown >= maxTextLines {
			fmt.Fprintln(os.Stdout)
			fmt.Fprintln(os.Stdout, clr(ansiDim, "Additional text diffs not shown. Use --json for complete structured diff data."))
			return
		}
		filesShown++
		fmt.Fprintln(os.Stdout)
		fmt.Fprintln(os.Stdout, clr(ansiBold, entry.Path))
		if !entry.TextDiff.Available {
			reason := checkpointTextDiffSkippedReason(entry.TextDiff.SkippedReason)
			fmt.Fprintln(os.Stdout, clr(ansiDim, "text diff skipped: "+reason))
			continue
		}
		for _, hunk := range entry.TextDiff.Hunks {
			if linesShown >= maxTextLines {
				fmt.Fprintln(os.Stdout, clr(ansiDim, "Additional text diff lines not shown. Use --json for complete structured diff data."))
				return
			}
			fmt.Fprintln(os.Stdout, clr(ansiDim, checkpointTextDiffHunkHeader(hunk)))
			linesShown++
			for _, line := range hunk.Lines {
				if linesShown >= maxTextLines {
					fmt.Fprintln(os.Stdout, clr(ansiDim, "Additional text diff lines not shown. Use --json for complete structured diff data."))
					return
				}
				fmt.Fprintln(os.Stdout, checkpointTextDiffLine(line))
				linesShown++
			}
		}
	}
}

func checkpointTextDiffSkippedReason(reason string) string {
	switch strings.TrimSpace(reason) {
	case "binary":
		return "binary file"
	case "too_large":
		return "file exceeds text diff size limit"
	case "too_many_lines":
		return "file exceeds text diff line limit"
	case "content_unavailable":
		return "content is unavailable"
	case "unsupported_kind":
		return "unsupported file kind"
	case "":
		return "unavailable"
	default:
		return reason
	}
}

func checkpointTextDiffHunkHeader(hunk controlplane.TextDiffHunk) string {
	return fmt.Sprintf("@@ -%d,%d +%d,%d @@", hunk.OldStart, hunk.OldLines, hunk.NewStart, hunk.NewLines)
}

func checkpointTextDiffLine(line controlplane.TextDiffLine) string {
	switch line.Kind {
	case "delete":
		return clr(ansiRed, "-"+line.Text)
	case "insert":
		return clr(ansiGreen, "+"+line.Text)
	default:
		return " " + line.Text
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
		checkpointID = parsed.positionals[0]
	}
	selection, err := resolveCheckpointWorkspaceSelection(context.Background(), cfg, service, workspace)
	if err != nil {
		return err
	}
	if err := validateAFSName("checkpoint", checkpointID); err != nil {
		return err
	}

	step := startStep("Saving checkpoint")
	_, err = saveCheckpointFromLiveWithOptions(context.Background(), service, selection.Name, checkpointID, controlplane.SaveCheckpointFromLiveOptions{
		Description:    parsed.description,
		AllowUnchanged: true,
	})
	if err != nil {
		step.fail(err.Error())
		return err
	}
	step.succeed(checkpointID)

	rows := []outputRow{
		{Label: "workspace", Value: selection.Name},
		{Label: "checkpoint", Value: checkpointID},
	}
	if parsed.description != "" {
		rows = append(rows, outputRow{Label: "description", Value: parsed.description})
	}
	printSection(markerSuccess+" "+clr(ansiBold, "checkpoint created"), rows)
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
	selection, err := resolveCheckpointWorkspaceSelection(context.Background(), cfg, service, workspace)
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

	rows := []outputRow{
		{Label: "workspace", Value: workspace},
		{Label: "checkpoint", Value: checkpointID},
	}
	if result.SafetyCheckpointCreated {
		rows = append(rows, outputRow{Label: "safety checkpoint", Value: result.SafetyCheckpointID})
	}
	printSection(markerSuccess+" "+clr(ansiBold, "checkpoint restored"), rows)
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
	return workspaceUsageTextFor(bin, "ws")
}

func workspaceUsageTextFor(bin, group string) string {
	if strings.TrimSpace(group) == "" {
		group = "workspace"
	}
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s %s <subcommand>

Subcommands:
  mount [<workspace> [directory]]             Mount a workspace to a local folder
  unmount [--delete] [<workspace|directory>]    Unmount a workspace
  create <workspace>                           Create an empty workspace
  list                                         List workspaces
  info <workspace>                             Show workspace details
  import [--force] [--mount-at-source] <workspace> <directory>
                                                Import a local directory into a workspace
  fork [source-workspace] <new-workspace>      Fork a workspace from its current checkpoint
  delete [--no-confirmation] <workspace>...    Delete workspaces and local materialized state

Examples:
  %s %s mount demo ~/demo
  %s %s unmount demo
  %s %s create demo
  %s %s list
  %s %s import demo ~/src/demo

Run '%s %s <subcommand> --help' for details.
`, bin, group, bin, group, bin, group, bin, group, bin, group, bin, group, bin, group)
}

func workspaceCreateUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s ws create [--database <database-id|database-name>] <workspace>

Create an empty workspace with an initial checkpoint named "initial".
`, bin)
}

func workspaceListUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s ws list

List workspaces stored in Redis, along with checkpoint counts and creation time.
`, bin)
}

func workspaceInfoUsageText(bin, group string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s %s info <workspace>

Show workspace metadata without mounting it locally.
`, bin, group)
}

func workspaceForkUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s ws fork [source-workspace] <new-workspace>

Create a new workspace from the source workspace's current checkpoint.

If [source-workspace] is omitted, AFS uses the mounted workspace when one is
unambiguous.
`, bin)
}

func workspaceDeleteUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s ws delete [--no-confirmation] <workspace>...

Delete one or more workspaces from Redis and remove their local materialized state.
By default, asks for confirmation before deleting.
`, bin)
}

func workspaceImportUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s ws import [--force] [--mount-at-source] [--database <database-id|database-name>] <workspace> <directory>

Import a local directory into a workspace.

Options:
  --force             Replace an existing workspace
  --mount-at-source  Mount the source directory after import
  --database          Override the control-plane database for this import
`, bin)
}

func checkpointUsageText(bin string) string {
	return checkpointUsageTextFor(bin, "cp")
}

func checkpointUsageTextFor(bin, group string) string {
	if strings.TrimSpace(group) == "" {
		group = "cp"
	}
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s %s <subcommand>

Subcommands:
  list [workspace]                     List checkpoints for a workspace
  create [workspace] [checkpoint]      Create a checkpoint
  show [workspace] <checkpoint>        Show checkpoint metadata
  diff [workspace] <base> <target>     Compare two checkpoints
  restore [workspace] <checkpoint>     Restore a workspace to a checkpoint

If workspace is omitted, AFS prompts for one.

Examples:
  %s %s list demo
  %s %s create demo before-refactor
  %s %s show demo before-refactor
  %s %s diff demo initial before-refactor
  %s %s restore demo initial

Run '%s %s <subcommand> --help' for details.
`, bin, group, bin, group, bin, group, bin, group, bin, group, bin, group, bin, group)
}

func checkpointListUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s cp list [workspace]

List checkpoints for a workspace, newest first.
If workspace is omitted, AFS prompts for one.
`, bin)
}

func checkpointCreateUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s cp create [workspace] [checkpoint] [--description <text>]

Create a checkpoint from the workspace's active state.
If [checkpoint] is omitted, AFS generates a timestamped name.
If workspace is omitted, AFS prompts for one. With one positional argument,
AFS treats it as the checkpoint name.

Options:
  --description <text>  Human-readable checkpoint description
`, bin)
}

func checkpointShowUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s cp show [workspace] <checkpoint> [--json]

Show checkpoint metadata and the change summary from its parent checkpoint.
If workspace is omitted, AFS prompts for one.

Options:
  --json  Emit structured JSON
`, bin)
}

func checkpointDiffUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s cp diff [workspace] <base-checkpoint> <target-checkpoint> [--json]
  %s cp diff [workspace] <checkpoint> --active [--json]

Compare saved filesystem states. Use --active to compare a checkpoint to
workspace state.
If workspace is omitted, AFS prompts for one.

Options:
  --json  Emit structured JSON, including text diff hunks when available
`, bin, bin)
}

func checkpointRestoreUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s cp restore [workspace] <checkpoint>

Restore workspace state to the selected checkpoint.
If workspace is omitted, AFS prompts for one.
`, bin)
}
