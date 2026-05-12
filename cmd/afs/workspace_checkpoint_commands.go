package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
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
	case "create", "create-manifest":
		return cmdWorkspaceManifestCreate(args)
	case "list", "list-manifests":
		return cmdWorkspaceManifestList(args)
	case "show", "info", "show-manifest":
		return cmdWorkspaceManifestShow(args)
	case "add":
		return cmdWorkspaceAddVolume(args)
	case "attach":
		return cmdWorkspaceAttachVolume(args)
	case "detach":
		return cmdWorkspaceDetachVolume(args)
	case "bookmark":
		return cmdWorkspaceBookmarkCommand(args)
	case "restore-bookmark":
		return cmdWorkspaceRestoreBookmark(args)
	case "clone":
		return workspaceLegacyVolumeCommandError(args[1])
	case "config":
		return workspaceLegacyVolumeCommandError(args[1])
	case "default":
		return workspaceLegacyVolumeCommandError(args[1])
	case "set-default":
		return workspaceLegacyVolumeCommandError(args[1])
	case "unset-default":
		return workspaceLegacyVolumeCommandError(args[1])
	case "mount":
		return cmdWorkspaceCompositionMount(args)
	case "unmount":
		return cmdWorkspaceCompositionUnmount(args)
	case "fork":
		return workspaceLegacyVolumeCommandError(args[1])
	case "delete":
		return workspaceLegacyVolumeCommandError(args[1])
	case "import":
		return workspaceLegacyVolumeCommandError(args[1])
	default:
		return fmt.Errorf("unknown workspace subcommand %q\n\n%s", args[1], workspaceUsageTextFor(filepath.Base(os.Args[0]), args[0]))
	}
}

func workspaceLegacyVolumeCommandError(command string) error {
	return fmt.Errorf(
		"%q now manages Agent Workspaces, which are manifests of attached volumes\nUse %q for the volume file-tree command instead",
		"afs ws "+command,
		"afs vol "+command,
	)
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

func extractVolumeFlag(args []string) (string, []string, error) {
	volume := ""
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--volume":
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("missing value for --volume")
			}
			if strings.TrimSpace(volume) != "" {
				return "", nil, fmt.Errorf("only one --volume flag may be provided")
			}
			i++
			volume = strings.TrimSpace(args[i])
		case strings.HasPrefix(arg, "--volume="):
			if strings.TrimSpace(volume) != "" {
				return "", nil, fmt.Errorf("only one --volume flag may be provided")
			}
			volume = strings.TrimSpace(strings.TrimPrefix(arg, "--volume="))
		default:
			rest = append(rest, arg)
		}
	}
	if strings.TrimSpace(volume) == "" {
		return "", rest, nil
	}
	return volume, append([]string{volume}, rest...), nil
}

func checkpointPositionalsWithVolume(args []string) ([]string, error) {
	_, rest, err := extractVolumeFlag(args)
	if err != nil {
		return nil, err
	}
	for _, arg := range rest {
		if strings.HasPrefix(arg, "-") {
			return nil, fmt.Errorf("unknown flag %q", arg)
		}
	}
	return rest, nil
}

func cmdWorkspaceCreate(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, workspaceCreateUsageTextFor(filepath.Base(os.Args[0]), args[0]))
		return nil
	}
	parsed, err := parseWorkspaceCreateArgs(args[2:])
	if err != nil {
		return err
	}
	if len(parsed.positionals) != 1 {
		return fmt.Errorf("%s", workspaceCreateUsageTextFor(filepath.Base(os.Args[0]), args[0]))
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
		database, err := resolveManagedDatabaseForWrite(context.Background(), cfg, client, parsed.database, "volume create")
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

	next := filepath.Base(os.Args[0]) + " vol mount " + workspace + " <directory>"

	printSection(markerSuccess+" "+clr(ansiBold, "volume created"), []outputRow{
		{Label: "volume", Value: workspace},
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
		fmt.Fprint(os.Stderr, workspaceListUsageTextFor(filepath.Base(os.Args[0]), args[0]))
		return nil
	}
	fs := flag.NewFlagSet(args[0]+" list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var jsonOut bool
	fs.BoolVar(&jsonOut, "json", false, "write JSON output")
	if err := fs.Parse(args[2:]); err != nil {
		return fmt.Errorf("%s", workspaceListUsageTextFor(filepath.Base(os.Args[0]), args[0]))
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("%s", workspaceListUsageTextFor(filepath.Base(os.Args[0]), args[0]))
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
	mounts := workspaceListMounts(workspaces.Items)

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(workspaceListJSONResponse(cfg, workspaces.Items, mounts))
	}

	fmt.Println()
	fmt.Println(workspaceListTitle(cfg))
	fmt.Println()
	if len(workspaces.Items) == 0 {
		fmt.Println("No volumes found")
	} else {
		headers := []string{"", "Volume", "Mounted", "Updated", "ID", "Database"}
		printPlainTable(headers, workspaceSummaryTableRows(cfg, workspaces.Items, mounts))
	}
	fmt.Println()
	return nil
}

type workspaceListJSONItem struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Selected        bool   `json:"selected"`
	Mounted         string `json:"mounted,omitempty"`
	UpdatedAt       string `json:"updated_at,omitempty"`
	DatabaseID      string `json:"database_id,omitempty"`
	DatabaseName    string `json:"database_name,omitempty"`
	Status          string `json:"status,omitempty"`
	FileCount       int    `json:"file_count"`
	FolderCount     int    `json:"folder_count"`
	TotalBytes      int64  `json:"total_bytes"`
	CheckpointCount int    `json:"checkpoint_count"`
	DraftState      string `json:"draft_state,omitempty"`
}

func workspaceListJSONResponse(cfg config, items []workspaceSummary, mounts map[string]string) struct {
	Items []workspaceListJSONItem `json:"items"`
} {
	out := struct {
		Items []workspaceListJSONItem `json:"items"`
	}{Items: make([]workspaceListJSONItem, 0, len(items))}
	for _, item := range items {
		mounted := workspaceListMounted(item, mounts)
		if mounted == "-" {
			mounted = ""
		}
		out.Items = append(out.Items, workspaceListJSONItem{
			ID:              item.ID,
			Name:            item.Name,
			Selected:        workspaceListSelected(cfg, item),
			Mounted:         mounted,
			UpdatedAt:       item.UpdatedAt,
			DatabaseID:      item.DatabaseID,
			DatabaseName:    item.DatabaseName,
			Status:          item.Status,
			FileCount:       item.FileCount,
			FolderCount:     item.FolderCount,
			TotalBytes:      item.TotalBytes,
			CheckpointCount: item.CheckpointCount,
			DraftState:      item.DraftState,
		})
	}
	return out
}

func cmdWorkspaceDefault(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, workspaceDefaultUsageText(filepath.Base(os.Args[0]), args[0]))
		return nil
	}
	if len(args) != 2 {
		return fmt.Errorf("%s", workspaceDefaultUsageText(filepath.Base(os.Args[0]), args[0]))
	}

	cfg, service, closeStore, err := openAFSControlPlane(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	rows := []outputRow{
		{Label: "saved default", Value: workspaceDefaultLabel(cfg)},
	}
	selection, err := resolveWorkspaceSelectionFromControlPlane(context.Background(), cfg, service, "")
	if err != nil {
		rows = append(rows,
			outputRow{Label: "effective", Value: clr(ansiDim, "none")},
			outputRow{Label: "note", Value: err.Error()},
		)
	} else {
		rows = append(rows,
			outputRow{Label: "effective", Value: selection.Name},
			outputRow{Label: "source", Value: workspaceSelectionSourceLabel(selection)},
		)
		if selection.MountPath != "" {
			rows = append(rows, outputRow{Label: "mounted at", Value: workspaceListMountedPath(selection.MountPath)})
		}
		if selection.ID != "" {
			rows = append(rows, outputRow{Label: "id", Value: selection.ID})
		}
	}
	rows = append(rows, outputRow{Label: "config", Value: clr(ansiDim, compactDisplayPath(configPath()))})
	printSection(clr(ansiBold, "default volume"), rows)
	return nil
}

func cmdWorkspaceSetDefault(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, workspaceSetDefaultUsageText(filepath.Base(os.Args[0]), args[0]))
		return nil
	}
	if len(args) != 3 {
		return fmt.Errorf("%s", workspaceSetDefaultUsageText(filepath.Base(os.Args[0]), args[0]))
	}

	cfg, service, closeStore, err := openAFSControlPlane(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	selection, err := resolveWorkspaceSetDefaultSelection(context.Background(), cfg, service, args[2])
	if err != nil {
		return err
	}
	if err := applyWorkspaceSelection(&cfg, selection); err != nil {
		return err
	}
	if err := saveConfig(cfg); err != nil {
		return err
	}

	rows := []outputRow{
		{Label: "volume", Value: selection.Name},
	}
	if selection.ID != "" {
		rows = append(rows, outputRow{Label: "id", Value: selection.ID})
	}
	if mounted := selectedMountedWorkspace(cfg); mounted.Name != "" && !sameWorkspaceSelection(mounted, selection) {
		rows = append(rows,
			outputRow{},
			outputRow{Label: "active mount", Value: mounted.Name},
			outputRow{Label: "note", Value: "mounted volume takes precedence until it is unmounted or you leave its folder"},
		)
		if mounted.MountPath != "" {
			rows = append(rows, outputRow{Label: "mounted at", Value: workspaceListMountedPath(mounted.MountPath)})
		}
	}
	rows = append(rows, outputRow{Label: "config", Value: clr(ansiDim, compactDisplayPath(configPath()))})
	printSection(markerSuccess+" "+clr(ansiBold, "default volume set"), rows)
	return nil
}

func resolveWorkspaceSetDefaultSelection(ctx context.Context, cfg config, service afsControlPlane, ref string) (workspaceSelection, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return workspaceSelection{}, fmt.Errorf("volume is required")
	}

	workspaces, listErr := service.ListWorkspaceSummaries(ctx)
	if listErr == nil {
		match, ok, err := matchWorkspaceSelection(ref, "", workspaces.Items)
		if err != nil {
			return workspaceSelection{}, fmt.Errorf("%w\nRun '%s vol list' and pass the volume id explicitly", err, filepath.Base(os.Args[0]))
		}
		if ok {
			match.Source = workspaceSelectionExplicit
			return match, nil
		}
	}

	if selection, ok, err := workspaceSelectionFromMountedDefaultRef(cfg, ref, workspaces.Items); err != nil {
		return workspaceSelection{}, err
	} else if ok {
		return selection, nil
	}

	if recs, err := mountedWorkspaceRecordsForRef(ref); err != nil {
		return workspaceSelection{}, err
	} else if len(recs) > 0 {
		return workspaceSelection{}, mountedWorkspaceConfigMismatchError(cfg, ref, recs)
	}

	if listErr != nil {
		return workspaceSelection{}, listErr
	}
	return workspaceSelection{}, fmt.Errorf("volume %q does not exist", ref)
}

func workspaceSelectionFromMountedDefaultRef(cfg config, ref string, workspaces []workspaceSummary) (workspaceSelection, bool, error) {
	recs, err := mountedWorkspaceRecordsForRef(ref)
	if err != nil {
		return workspaceSelection{}, false, err
	}
	matches := make([]mountRecord, 0, len(recs))
	for _, rec := range recs {
		if mountRecordMatchesDefaultConfig(cfg, rec) {
			matches = append(matches, rec)
		}
	}
	if len(matches) == 0 {
		return workspaceSelection{}, false, nil
	}
	if len(matches) > 1 {
		paths := make([]string, 0, len(matches))
		for _, rec := range matches {
			paths = append(paths, homeRelativeDisplayPath(rec.LocalPath))
		}
		sort.Strings(paths)
		return workspaceSelection{}, false, fmt.Errorf("volume %q matches multiple mounted volumes: %s\nPass a volume id explicitly", ref, strings.Join(paths, ", "))
	}
	return workspaceSelectionFromMountedDefaultRecord(matches[0], workspaces)
}

func workspaceSelectionFromMountedDefaultRecord(rec mountRecord, workspaces []workspaceSummary) (workspaceSelection, bool, error) {
	selection := workspaceSelection{
		ID:        strings.TrimSpace(rec.WorkspaceID),
		Name:      strings.TrimSpace(rec.Workspace),
		Source:    workspaceSelectionExplicit,
		MountPath: strings.TrimSpace(rec.LocalPath),
	}
	if selection.ID == "" && selection.Name == "" {
		return workspaceSelection{}, false, nil
	}
	if len(workspaces) == 0 {
		return selection, true, nil
	}
	ref := selection.ID
	if ref == "" {
		ref = selection.Name
	}
	match, ok, err := matchWorkspaceSelection(ref, selection.Name, workspaces)
	if err != nil {
		return workspaceSelection{}, false, fmt.Errorf("mounted volume %q is ambiguous: %w\nRun '%s vol list' and pass a volume id explicitly", selection.Name, err, filepath.Base(os.Args[0]))
	}
	if ok {
		match.Source = workspaceSelectionExplicit
		match.MountPath = selection.MountPath
		return match, true, nil
	}
	return selection, true, nil
}

func mountedWorkspaceRecordsForRef(ref string) ([]mountRecord, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, nil
	}
	reg, err := loadMountRegistry()
	if err != nil {
		return nil, err
	}
	matches := make([]mountRecord, 0)
	for _, rec := range sortedMountRecords(reg.Mounts) {
		if mountStatus(rec) != "running" {
			continue
		}
		if strings.TrimSpace(rec.WorkspaceID) == ref || strings.TrimSpace(rec.Workspace) == ref {
			matches = append(matches, rec)
		}
	}
	return matches, nil
}

func mountRecordMatchesDefaultConfig(cfg config, rec mountRecord) bool {
	if !mountRecordMatchesConfig(cfg, rec) {
		return false
	}
	mode, err := effectiveProductMode(cfg)
	if err != nil {
		return false
	}
	if mode == productModeLocal {
		return true
	}
	cfgDatabase := strings.TrimSpace(cfg.DatabaseID)
	recDatabase := strings.TrimSpace(rec.ControlPlaneDatabase)
	return cfgDatabase == "" || recDatabase == "" || cfgDatabase == recDatabase
}

func mountedWorkspaceConfigMismatchError(cfg config, ref string, recs []mountRecord) error {
	current := currentConfigScopeLabel(cfg)
	if len(recs) == 1 {
		return fmt.Errorf("volume %q is mounted from %s, but the current config uses %s\nRun '%s status --verbose' to inspect mounts or '%s vol list' to choose a volume in the current config", ref, mountRecordScopeLabel(recs[0]), current, filepath.Base(os.Args[0]), filepath.Base(os.Args[0]))
	}
	return fmt.Errorf("volume %q is mounted under another config, but the current config uses %s\nRun '%s status --verbose' to inspect mounts or '%s vol list' to choose a volume in the current config", ref, current, filepath.Base(os.Args[0]), filepath.Base(os.Args[0]))
}

func cmdWorkspaceUnsetDefault(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, workspaceUnsetDefaultUsageText(filepath.Base(os.Args[0]), args[0]))
		return nil
	}
	if len(args) != 2 {
		return fmt.Errorf("%s", workspaceUnsetDefaultUsageText(filepath.Base(os.Args[0]), args[0]))
	}

	cfg, err := loadAFSConfig()
	if err != nil {
		return err
	}
	cfg.CurrentWorkspace = ""
	cfg.CurrentWorkspaceID = ""
	if err := saveConfig(cfg); err != nil {
		return err
	}
	printSection(markerSuccess+" "+clr(ansiBold, "default volume cleared"), []outputRow{
		{Label: "config", Value: clr(ansiDim, compactDisplayPath(configPath()))},
	})
	return nil
}

func workspaceDefaultLabel(cfg config) string {
	if strings.TrimSpace(cfg.CurrentWorkspace) == "" && strings.TrimSpace(cfg.CurrentWorkspaceID) == "" {
		return clr(ansiDim, "unset")
	}
	if strings.TrimSpace(cfg.CurrentWorkspaceID) != "" && strings.TrimSpace(cfg.CurrentWorkspace) != "" {
		return fmt.Sprintf("%s (%s)", strings.TrimSpace(cfg.CurrentWorkspace), strings.TrimSpace(cfg.CurrentWorkspaceID))
	}
	if strings.TrimSpace(cfg.CurrentWorkspaceID) != "" {
		return strings.TrimSpace(cfg.CurrentWorkspaceID)
	}
	return strings.TrimSpace(cfg.CurrentWorkspace)
}

func workspaceSelectionSourceLabel(selection workspaceSelection) string {
	switch selection.Source {
	case workspaceSelectionExplicit:
		return "explicit"
	case workspaceSelectionCWD:
		return "mounted volume from current directory"
	case workspaceSelectionSingleMount:
		return "only mounted volume"
	case workspaceSelectionActiveState:
		return "active runtime state"
	case workspaceSelectionSavedDefault:
		return "saved default"
	case workspaceSelectionPrompt:
		return "selected"
	default:
		return "resolved"
	}
}

func sameWorkspaceSelection(left, right workspaceSelection) bool {
	if left.ID != "" && right.ID != "" {
		return left.ID == right.ID
	}
	return strings.TrimSpace(left.Name) != "" && strings.TrimSpace(left.Name) == strings.TrimSpace(right.Name)
}

func cmdWorkspaceInfo(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, workspaceInfoUsageText(filepath.Base(os.Args[0]), "vol"))
		return nil
	}
	if len(args) != 2 && len(args) != 3 {
		fmt.Fprint(os.Stderr, workspaceInfoUsageText(filepath.Base(os.Args[0]), "vol"))
		return nil
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
	selection, err := resolveCommandWorkspaceSelectionFromControlPlane(context.Background(), cfg, service, workspace)
	if err != nil {
		return err
	}
	detail, err := service.GetWorkspace(context.Background(), selection.ID)
	if err != nil {
		return err
	}
	rows := []outputRow{
		{Label: "volume", Value: detail.Name},
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
	printSection("Volume", rows)
	return nil
}

func workspaceSummaryTableRows(cfg config, items []workspaceSummary, mounts map[string]string) [][]string {
	rows := make([][]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, []string{
			workspaceListMarker(workspaceListSelected(cfg, item)),
			workspaceListName(item.Name),
			workspaceListMounted(item, mounts),
			workspaceListUpdated(item),
			workspaceListID(item),
			workspaceListDatabase(item),
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
	if databaseID := strings.TrimSpace(summary.DatabaseID); databaseID != "" {
		return databaseID
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

func workspaceListMounts(items []workspaceSummary) map[string]string {
	reg, err := loadMountRegistry()
	if err != nil || len(reg.Mounts) == 0 {
		return nil
	}
	nameCounts := make(map[string]int)
	for _, item := range items {
		if name := strings.TrimSpace(item.Name); name != "" {
			nameCounts[name]++
		}
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
			continue
		}
		name := strings.TrimSpace(rec.Workspace)
		if name == "" {
			continue
		}
		if databaseID := strings.TrimSpace(rec.ControlPlaneDatabase); databaseID != "" {
			key := workspaceListDatabaseNameMountKey(databaseID, name)
			paths[key] = append(paths[key], display)
			continue
		}
		if nameCounts[name] <= 1 {
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
	name := strings.TrimSpace(summary.Name)
	if databaseID := strings.TrimSpace(summary.DatabaseID); databaseID != "" && name != "" {
		if path := strings.TrimSpace(mounts[workspaceListDatabaseNameMountKey(databaseID, name)]); path != "" {
			return path
		}
	}
	if name != "" {
		if path := strings.TrimSpace(mounts["name:"+name]); path != "" {
			return path
		}
	}
	return "-"
}

func workspaceListDatabaseNameMountKey(databaseID, workspaceName string) string {
	return "database-name:" + strings.TrimSpace(databaseID) + "\x00" + strings.TrimSpace(workspaceName)
}

func workspaceListMountedPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "-"
	}
	return homeRelativeDisplayPath(path)
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
	ctx := context.Background()
	targets := workspaceDeleteTargetsFromNames(opts.names)
	var cfg config
	var service afsControlPlane
	var closeStore func()
	var stdin *bufio.Reader
	if len(targets) == 0 {
		var err error
		cfg, service, closeStore, err = openAFSControlPlane(ctx)
		if err != nil {
			return err
		}
		defer closeStore()
		stdin = bufio.NewReader(os.Stdin)
		target, err := promptWorkspaceDeleteTarget(ctx, service, stdin)
		if err != nil {
			return err
		}
		targets = []workspaceDeleteTarget{target}
	}

	names := workspaceDeleteTargetNames(targets)
	if !opts.noConfirmation {
		if stdin == nil {
			stdin = bufio.NewReader(os.Stdin)
		}
		ok, err := confirmWorkspaceDeleteWithReader(names, stdin)
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

	if service == nil {
		var err error
		cfg, service, closeStore, err = openAFSControlPlane(ctx)
		if err != nil {
			return err
		}
		defer closeStore()
	}

	deleted := make([]string, 0, len(targets))
	for _, target := range targets {
		if err := validateAFSName("workspace", target.ref); err != nil {
			return err
		}

		step := startStep("Deleting volume " + target.name)
		if err := service.DeleteWorkspace(ctx, target.ref); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				step.fail("does not exist")
				return fmt.Errorf("volume %q does not exist", target.name)
			}
			step.fail(err.Error())
			return err
		}
		if err := removeLocalWorkspace(cfg, target.name); err != nil {
			step.fail(err.Error())
			return err
		}
		step.succeed(target.name)
		deleted = append(deleted, target.name)
	}

	rows := make([]outputRow, 0, len(deleted)+1)
	rows = append(rows, outputRow{Label: "deleted", Value: strconv.Itoa(len(deleted))})
	rows = append(rows, outputRow{})
	for _, name := range deleted {
		rows = append(rows, outputRow{Value: name})
	}
	printSection(markerSuccess+" "+clr(ansiBold, "volumes deleted"), rows)
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
		if workspace == "" {
			workspaces, listErr := store.listWorkspaces(context.Background())
			if listErr == nil && len(workspaces) == 1 {
				workspace = workspaces[0].Name
			} else {
				return err
			}
		} else {
			return err
		}
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

	printSection(markerSuccess+" "+clr(ansiBold, "volume cloned"), []outputRow{
		{Label: "volume", Value: workspace},
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

type workspaceDeleteOptions struct {
	names          []string
	noConfirmation bool
}

type workspaceDeleteTarget struct {
	ref  string
	name string
}

func workspaceDeleteTargetsFromNames(names []string) []workspaceDeleteTarget {
	targets := make([]workspaceDeleteTarget, 0, len(names))
	for _, name := range names {
		targets = append(targets, workspaceDeleteTarget{ref: name, name: name})
	}
	return targets
}

func promptWorkspaceDeleteTarget(ctx context.Context, service afsControlPlane, reader *bufio.Reader) (workspaceDeleteTarget, error) {
	workspaces, err := service.ListWorkspaceSummaries(ctx)
	if err != nil {
		return workspaceDeleteTarget{}, err
	}
	selection, err := promptWorkspaceSelectionFromSummariesWithReader(workspaces.Items, reader)
	if err != nil {
		return workspaceDeleteTarget{}, err
	}
	name := strings.TrimSpace(selection.Name)
	return workspaceDeleteTarget{ref: name, name: name}, nil
}

func workspaceDeleteTargetNames(targets []workspaceDeleteTarget) []string {
	names := make([]string, 0, len(targets))
	for _, target := range targets {
		names = append(names, target.name)
	}
	return names
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
	return confirmWorkspaceDeleteWithReader(names, bufio.NewReader(os.Stdin))
}

func confirmWorkspaceDeleteWithReader(names []string, reader *bufio.Reader) (bool, error) {
	if len(names) == 0 {
		return false, nil
	}
	target := names[0]
	if len(names) > 1 {
		target = strings.Join(names, ", ")
	}
	fmt.Println()
	fmt.Printf("Are you sure you want to delete %s? [y/N] ", target)
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
	sourceSelection, err := resolveCommandWorkspaceSelectionFromControlPlane(context.Background(), cfg, service, sourceWorkspace)
	if err != nil {
		if sourceWorkspace == "" {
			workspaces, listErr := service.ListWorkspaceSummaries(context.Background())
			if listErr == nil && len(workspaces.Items) == 1 {
				only := workspaces.Items[0]
				sourceSelection = workspaceSelection{ID: only.ID, Name: only.Name}
			} else {
				return err
			}
		} else {
			return err
		}
	}
	if err := validateAFSName("workspace", newWorkspace); err != nil {
		return err
	}

	ctx := context.Background()
	if err := service.ForkWorkspace(ctx, sourceSelection.ID, newWorkspace); err != nil {
		return err
	}

	printSection(markerSuccess+" "+clr(ansiBold, "volume forked"), []outputRow{
		{Label: "volume", Value: newWorkspace},
		{Label: "source", Value: sourceSelection.Name},
		{Label: "next", Value: filepath.Base(os.Args[0]) + " vol mount " + newWorkspace + " <directory>"},
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

func workspaceListTitle(cfg config) string {
	return "volumes on " + configRemoteLabel(cfg)
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
		return state{}, fmt.Errorf("AFS already has an active mounted filesystem state; run '%s vol unmount <volume-or-directory>' first", filepath.Base(os.Args[0]))
	}
	return st, nil
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
	m, blobs, err := liveWorkspaceManifest(ctx, store, workspace, workspaceMeta.HeadSavepoint)
	if err != nil {
		return err
	}
	targetDir, err = expandPath(targetDir)
	if err != nil {
		return err
	}
	_, err = materializeManifestToDirectory(targetDir, m, func(blobID string) ([]byte, error) {
		data, ok := blobs[blobID]
		if !ok {
			return nil, fmt.Errorf("live workspace blob %q is missing during clone", blobID)
		}
		return data, nil
	}, manifestMaterializeOptions{})
	return err
}

func cmdCheckpointList(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, checkpointListUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	positionals, err := checkpointPositionalsWithVolume(args[2:])
	if err != nil {
		return fmt.Errorf("%s\n\n%s", err, checkpointListUsageText(filepath.Base(os.Args[0])))
	}
	if len(positionals) > 1 {
		return fmt.Errorf("%s", checkpointListUsageText(filepath.Base(os.Args[0])))
	}

	cfg, service, closeStore, err := openAFSControlPlane(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	workspace := ""
	if len(positionals) == 1 {
		workspace = positionals[0]
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
	return resolveCommandWorkspaceSelectionFromControlPlane(ctx, cfg, service, workspace)
}

func promptCheckpointWorkspaceSelection(ctx context.Context, service afsControlPlane) (workspaceSelection, error) {
	workspaces, err := service.ListWorkspaceSummaries(ctx)
	if err != nil {
		return workspaceSelection{}, err
	}
	return promptWorkspaceSelectionFromSummaries(workspaces.Items)
}

func checkpointWorkspacePromptRows(workspaces []workspaceSummary, mounts map[string]string) [][]string {
	rows := make([][]string, 0, len(workspaces))
	for i, workspace := range workspaces {
		rows = append(rows, []string{
			strconv.Itoa(i + 1),
			workspace.Name,
			workspaceListID(workspace),
			workspaceListDatabase(workspace),
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
	_, args, err := extractVolumeFlag(args)
	if err != nil {
		return parsed, err
	}
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
		return parsed, fmt.Errorf("expected [volume] <checkpoint>")
	}
	if err := validateAFSName("checkpoint", parsed.checkpointID); err != nil {
		return parsed, err
	}
	return parsed, nil
}

func printCheckpointShow(workspace string, detail controlplane.CheckpointDetail, activeCheckpointID string) {
	rows := []outputRow{
		{Label: "volume", Value: workspace},
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
	_, args, err := extractVolumeFlag(args)
	if err != nil {
		return parsed, err
	}
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
		{Label: "volume", Value: workspace},
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
	for _, entry := range diff.Entries {
		rows = append(rows, outputRow{
			Label: checkpointDiffOpLabel(entry.Op),
			Value: checkpointDiffEntryValue(entry),
		})
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
	for _, entry := range diff.Entries {
		if entry.TextDiff == nil {
			continue
		}
		fmt.Fprintln(os.Stdout)
		fmt.Fprintln(os.Stdout, clr(ansiBold, entry.Path))
		if !entry.TextDiff.Available {
			reason := checkpointTextDiffSkippedReason(entry.TextDiff.SkippedReason)
			fmt.Fprintln(os.Stdout, clr(ansiDim, "text diff skipped: "+reason))
			continue
		}
		for _, hunk := range entry.TextDiff.Hunks {
			fmt.Fprintln(os.Stdout, clr(ansiDim, checkpointTextDiffHunkHeader(hunk)))
			for _, line := range hunk.Lines {
				fmt.Fprintln(os.Stdout, checkpointTextDiffLine(line))
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
		{Label: "volume", Value: selection.Name},
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
	_, args, err := extractVolumeFlag(args)
	if err != nil {
		return parsed, err
	}
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
	positionals, err := checkpointPositionalsWithVolume(args[2:])
	if err != nil {
		return fmt.Errorf("%s\n\n%s", err, checkpointRestoreUsageText(filepath.Base(os.Args[0])))
	}
	if len(positionals) != 1 && len(positionals) != 2 {
		return fmt.Errorf("%s", checkpointRestoreUsageText(filepath.Base(os.Args[0])))
	}
	cfg, service, closeFn, err := openAFSControlPlane(context.Background())
	if err != nil {
		return err
	}
	defer closeFn()

	workspace := ""
	checkpointID := ""
	if len(positionals) == 2 {
		workspace = positionals[0]
		checkpointID = positionals[1]
	} else {
		checkpointID = positionals[0]
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
		{Label: "volume", Value: workspace},
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
  create <workspace>                          Create an Agent Workspace manifest
  list                                        List Agent Workspaces
  show <workspace>                            Show attached volumes and bookmarks
  add <workspace> <directory>                 Import a folder into a new volume, and attach it to the workspace
  attach <workspace> [volume] [--at <path>]   Attach an existing volume
  detach <workspace> <volume>                 Detach a volume from a workspace
  mount <workspace> <directory>               Mount the workspace under a local root
  unmount [--delete] <directory>              Unmount a local workspace root
  bookmark create <workspace> <name>          Capture all mounted volume checkpoints
  bookmark list <workspace>                   List workspace bookmarks
  bookmark restore <workspace> <name>         Restore mounted volumes to a bookmark

Examples:
  %s %s create coding-agent
  %s %s add coding-agent ./coding-skills --readonly
  %s %s attach coding-agent common-skills --at /common-skills
  %s %s list
  %s %s show coding-agent
  %s %s mount coding-agent ~/coding-agent

Run '%s %s <subcommand> --help' for details.
`, bin, group, bin, group, bin, group, bin, group, bin, group, bin, group, bin, group, bin, group)
}

func workspaceCreateUsageText(bin string) string {
	return workspaceCreateUsageTextFor(bin, "vol")
}

func workspaceCreateUsageTextFor(bin, group string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s %s create [--database <database-id|database-name>] <volume>

Create an empty volume with an initial checkpoint named "initial".
`, bin, group)
}

func workspaceListUsageText(bin string) string {
	return workspaceListUsageTextFor(bin, "vol")
}

func workspaceListUsageTextFor(bin, group string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s %s list [--json]

List volumes stored in Redis, along with checkpoint counts and creation time.
`, bin, group)
}

func workspaceCloneUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s vol clone [volume] <directory>

Clone the current live volume state into a local directory.
The destination must not already contain files.

If volume is omitted, AFS uses the default, the CWD (if a volume is mounted
there), the only mounted volume, or prompts for one.
`, bin)
}

func workspaceDefaultUsageText(bin, group string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s %s default

Show the saved default volume and the effective volume AFS will use when
a command omits [volume]. Mounted volumes take precedence when the current
directory is inside a mounted volume, or when exactly one volume is mounted.
`, bin, group)
}

func workspaceSetDefaultUsageText(bin, group string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s %s set-default <volume>

Save a default volume for commands that allow [volume] to be omitted.
In Cloud or Self-managed mode, pass a volume id when duplicate volume
names exist.
`, bin, group)
}

func workspaceUnsetDefaultUsageText(bin, group string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s %s unset-default

Clear the saved default volume. Mounted volumes can still be used as an
implicit volume when they are unambiguous.
`, bin, group)
}

func workspaceInfoUsageText(bin, group string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s %s info [volume]

Show volume metadata without mounting it locally.
If volume is omitted, AFS uses the default, the CWD (if a volume is mounted
there), the only mounted volume, or prompts for one.
`, bin, group)
}

func workspaceForkUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s vol fork [source-volume] <new-volume>

Create a new volume from the source volume's current checkpoint.

If source-volume is omitted, AFS uses the default, the CWD (if a volume is
mounted there), the only mounted volume, or prompts for one.
`, bin)
}

func workspaceDeleteUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s vol delete [--no-confirmation] [volume]...

Delete one or more volumes from Redis and remove their local materialized state.
By default, asks for confirmation before deleting.
If volume is omitted, AFS lists volumes and prompts for one.
`, bin)
}

func workspaceImportUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s vol import [--force] [--mount-at-source] [--database <database-id|database-name>] <volume> <directory>

Import a local directory into a volume.

Options:
  --force             Replace an existing volume
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
  list [volume]                        List checkpoints for a volume
  create [volume] [checkpoint]         Create a checkpoint
  show [volume] <checkpoint>           Show checkpoint metadata
  diff [volume] <base> <target>        Compare two checkpoints
  restore [volume] <checkpoint>        Restore a volume to a checkpoint

If volume is omitted, AFS uses the default, the CWD (if a volume is mounted
there), the only mounted volume, or prompts for one.

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
  %s cp list [volume]
  %s cp list --volume <volume>

List checkpoints for a volume, newest first.
If volume is omitted, AFS uses the default, the CWD (if a volume is mounted
there), the only mounted volume, or prompts for one.
`, bin, bin)
}

func checkpointCreateUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s cp create [volume] [checkpoint] [--description <text>]
  %s cp create --volume <volume> [checkpoint] [--description <text>]

Create a checkpoint from the volume's active state.
If [checkpoint] is omitted, AFS generates a timestamped name.
If volume is omitted, AFS uses the default, the CWD (if a volume is mounted
there), the only mounted volume, or prompts for one.
With one positional argument, AFS treats it as the checkpoint name.

Options:
  --description <text>  Human-readable checkpoint description
`, bin, bin)
}

func checkpointShowUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s cp show [volume] <checkpoint> [--json]
  %s cp show --volume <volume> <checkpoint> [--json]

Show checkpoint metadata and the change summary from its parent checkpoint.
If volume is omitted, AFS uses the default, the CWD (if a volume is mounted
there), the only mounted volume, or prompts for one.

Options:
  --json  Emit structured JSON
`, bin, bin)
}

func checkpointDiffUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s cp diff [volume] <base-checkpoint> <target-checkpoint> [--json]
  %s cp diff [volume] <checkpoint> --active [--json]
  %s cp diff --volume <volume> <base-checkpoint> <target-checkpoint> [--json]

Compare saved filesystem states. Use --active to compare a checkpoint to
volume state.
If volume is omitted, AFS uses the default, the CWD (if a volume is mounted
there), the only mounted volume, or prompts for one.

Options:
  --json  Emit structured JSON, including text diff hunks when available
`, bin, bin, bin)
}

func checkpointRestoreUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s cp restore [volume] <checkpoint>
  %s cp restore --volume <volume> <checkpoint>

Restore volume state to the selected checkpoint.
If volume is omitted, AFS uses the default, the CWD (if a volume is mounted
there), the only mounted volume, or prompts for one.
`, bin, bin)
}
