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
)

func cmdFile(args []string) error {
	if len(args) < 2 || isHelpArg(args[1]) {
		fmt.Fprint(os.Stderr, fileUsageText(filepath.Base(os.Args[0])))
		return nil
	}

	switch args[1] {
	case "history":
		return cmdFileHistory(args)
	case "show":
		return cmdFileShow(args)
	case "diff":
		return cmdFileDiff(args)
	case "restore":
		return cmdFileRestore(args)
	case "undelete":
		return cmdFileUndelete(args)
	default:
		return fmt.Errorf("unknown file subcommand %q\n\n%s", args[1], fileUsageText(filepath.Base(os.Args[0])))
	}
}

type fileHistoryArgs struct {
	workspace   string
	path        string
	newestFirst bool
	limit       int
	cursor      string
}

func cmdFileHistory(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, fileHistoryUsageText(filepath.Base(os.Args[0])))
		return nil
	}

	parsed, err := parseFileHistoryArgs(args[2:])
	if err != nil {
		return fmt.Errorf("%s\n\n%s", err, fileHistoryUsageText(filepath.Base(os.Args[0])))
	}

	ctx := context.Background()
	remote, err := openFSRemoteWorkspace(ctx, parsed.workspace)
	if err != nil {
		return err
	}
	defer remote.close()

	history, err := remote.controlPlane.GetFileHistoryPage(ctx, remote.selection.ID, controlplane.FileHistoryRequest{
		Path:        parsed.path,
		NewestFirst: parsed.newestFirst,
		Limit:       parsed.limit,
		Cursor:      parsed.cursor,
	})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return printNoFileHistoryIfLiveFileExists(ctx, remote, parsed.path, err)
		}
		return err
	}

	rows := make([]outputRow, 0, 2+len(history.Lineages)*4)
	rows = append(rows, outputRow{Label: "workspace", Value: remote.selection.Name})
	rows = append(rows, outputRow{Label: "order", Value: history.Order})
	for _, lineage := range history.Lineages {
		currentPath := lineage.CurrentPath
		if strings.TrimSpace(currentPath) == "" {
			currentPath = "<deleted>"
		}
		rows = append(rows, outputRow{
			Label: lineage.FileID,
			Value: fmt.Sprintf("%s · current %s", lineage.State, currentPath),
		})
		for _, version := range lineage.Versions {
			value := fmt.Sprintf("ordinal %d · %s · %s", version.Ordinal, version.Op, version.Path)
			if version.PrevPath != "" {
				value += " <- " + version.PrevPath
			}
			value += " · " + formatDisplayTimestamp(version.CreatedAt.Format(time.RFC3339))
			rows = append(rows, outputRow{
				Label: "  " + version.VersionID,
				Value: value,
			})
		}
	}
	printSection(clr(ansiBold, "file history: "+history.Path), rows)
	if history.NextCursor != "" {
		fmt.Fprintf(os.Stdout, "\nnext cursor: %s\n", history.NextCursor)
	}
	return nil
}

func printNoFileHistoryIfLiveFileExists(ctx context.Context, remote *fsRemoteWorkspace, rawPath string, historyErr error) error {
	file, fileErr := remote.controlPlane.GetFileContent(ctx, remote.selection.ID, "working-copy", normalizeFSRemotePath(rawPath))
	if fileErr != nil {
		return historyErr
	}
	printSection(clr(ansiBold, "file history: "+file.Path), []outputRow{
		{Label: "workspace", Value: remote.selection.Name},
		{Label: "path", Value: file.Path},
		{Label: "status", Value: "no history recorded yet"},
	})
	return nil
}

func parseFileHistoryArgs(args []string) (fileHistoryArgs, error) {
	parsed := fileHistoryArgs{newestFirst: true}
	positionals := make([]string, 0, 2)
	for index := 0; index < len(args); index++ {
		if ok, next, err := parseFileWorkspaceFlag(&parsed.workspace, args, index); ok || err != nil {
			if err != nil {
				return parsed, err
			}
			index = next
			continue
		}
		arg := args[index]
		switch {
		case arg == "--order":
			if index+1 >= len(args) {
				return parsed, fmt.Errorf("--order requires a value")
			}
			index++
			switch strings.ToLower(strings.TrimSpace(args[index])) {
			case "asc":
				parsed.newestFirst = false
			case "desc":
				parsed.newestFirst = true
			default:
				return parsed, fmt.Errorf("--order must be asc or desc")
			}
		case strings.HasPrefix(arg, "--order="):
			switch strings.ToLower(strings.TrimSpace(strings.TrimPrefix(arg, "--order="))) {
			case "asc":
				parsed.newestFirst = false
			case "desc":
				parsed.newestFirst = true
			default:
				return parsed, fmt.Errorf("--order must be asc or desc")
			}
		case arg == "--limit":
			if index+1 >= len(args) {
				return parsed, fmt.Errorf("--limit requires a value")
			}
			index++
			value, err := strconv.Atoi(strings.TrimSpace(args[index]))
			if err != nil || value < 0 {
				return parsed, fmt.Errorf("--limit must be a non-negative integer")
			}
			parsed.limit = value
		case strings.HasPrefix(arg, "--limit="):
			value, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(arg, "--limit=")))
			if err != nil || value < 0 {
				return parsed, fmt.Errorf("--limit must be a non-negative integer")
			}
			parsed.limit = value
		case arg == "--cursor":
			if index+1 >= len(args) {
				return parsed, fmt.Errorf("--cursor requires a value")
			}
			index++
			parsed.cursor = strings.TrimSpace(args[index])
		case strings.HasPrefix(arg, "--cursor="):
			parsed.cursor = strings.TrimSpace(strings.TrimPrefix(arg, "--cursor="))
		case strings.HasPrefix(arg, "--"):
			return parsed, fmt.Errorf("unknown flag %q", arg)
		default:
			positionals = append(positionals, arg)
		}
	}
	if len(positionals) == 0 || len(positionals) > 2 {
		return parsed, fmt.Errorf("usage requires <path>")
	}
	if len(positionals) == 1 {
		parsed.path = positionals[0]
		return parsed, nil
	}
	parsed.workspace = positionals[0]
	parsed.path = positionals[1]
	return parsed, nil
}

func parseFileWorkspaceFlag(workspace *string, args []string, index int) (bool, int, error) {
	arg := args[index]
	switch {
	case arg == "--workspace" || arg == "-w":
		if index+1 >= len(args) {
			return true, index, fmt.Errorf("%s requires a value", arg)
		}
		*workspace = strings.TrimSpace(args[index+1])
		return true, index + 1, nil
	case strings.HasPrefix(arg, "--workspace="):
		*workspace = strings.TrimSpace(strings.TrimPrefix(arg, "--workspace="))
		return true, index, nil
	case strings.HasPrefix(arg, "-w="):
		*workspace = strings.TrimSpace(strings.TrimPrefix(arg, "-w="))
		return true, index, nil
	default:
		return false, index, nil
	}
}

type fileShowArgs struct {
	workspace string
	path      string
	versionID string
	fileID    string
	ordinal   *int64
}

type fileRestoreArgs struct {
	workspace string
	path      string
	versionID string
	fileID    string
	ordinal   *int64
}

type fileUndeleteArgs struct {
	workspace string
	path      string
	versionID string
	fileID    string
	ordinal   *int64
}

type fileDiffOperandArgs struct {
	ref       string
	versionID string
	fileID    string
	ordinal   *int64
}

type fileDiffArgs struct {
	workspace string
	path      string
	from      fileDiffOperandArgs
	to        fileDiffOperandArgs
}

func cmdFileShow(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, fileShowUsageText(filepath.Base(os.Args[0])))
		return nil
	}

	parsed, err := parseFileShowArgs(args[2:])
	if err != nil {
		return fmt.Errorf("%s\n\n%s", err, fileShowUsageText(filepath.Base(os.Args[0])))
	}

	ctx := context.Background()
	remote, err := openFSRemoteWorkspace(ctx, parsed.workspace)
	if err != nil {
		return err
	}
	defer remote.close()

	var version controlplane.FileVersionContentResponse
	switch {
	case parsed.versionID != "":
		version, err = remote.controlPlane.GetFileVersionContent(ctx, remote.selection.ID, parsed.versionID)
	case parsed.fileID != "" && parsed.ordinal != nil:
		version, err = remote.controlPlane.GetFileVersionContentAtOrdinal(ctx, remote.selection.ID, parsed.fileID, *parsed.ordinal)
	default:
		err = fmt.Errorf("--version or --file-id with --ordinal is required")
	}
	if err != nil {
		return err
	}
	parsedPath := normalizeFSRemotePath(parsed.path)
	if parsed.path != "" && parsedPath != strings.TrimSpace(version.Path) {
		ref := parsed.versionID
		if ref == "" && parsed.fileID != "" && parsed.ordinal != nil {
			ref = fmt.Sprintf("%s@%d", parsed.fileID, *parsed.ordinal)
		}
		return fmt.Errorf("version %q belongs to %q, not %q", ref, version.Path, parsedPath)
	}

	switch {
	case version.Binary:
		printSection(clr(ansiBold, "binary history entry"), []outputRow{
			{Label: "workspace", Value: remote.selection.Name},
			{Label: "version", Value: version.VersionID},
			{Label: "path", Value: version.Path},
			{Label: "size", Value: formatBytes(version.Size)},
		})
	case version.Kind == controlplane.FileVersionKindSymlink:
		fmt.Fprintln(os.Stdout, version.Target)
	default:
		fmt.Fprint(os.Stdout, version.Content)
	}
	return nil
}

func parseFileShowArgs(args []string) (fileShowArgs, error) {
	var parsed fileShowArgs
	positionals := make([]string, 0, 2)
	for index := 0; index < len(args); index++ {
		if ok, next, err := parseFileWorkspaceFlag(&parsed.workspace, args, index); ok || err != nil {
			if err != nil {
				return parsed, err
			}
			index = next
			continue
		}
		arg := args[index]
		switch {
		case arg == "--version":
			if index+1 >= len(args) {
				return parsed, fmt.Errorf("--version requires a value")
			}
			index++
			parsed.versionID = strings.TrimSpace(args[index])
		case strings.HasPrefix(arg, "--version="):
			parsed.versionID = strings.TrimSpace(strings.TrimPrefix(arg, "--version="))
		case arg == "--file-id":
			if index+1 >= len(args) {
				return parsed, fmt.Errorf("--file-id requires a value")
			}
			index++
			parsed.fileID = strings.TrimSpace(args[index])
		case strings.HasPrefix(arg, "--file-id="):
			parsed.fileID = strings.TrimSpace(strings.TrimPrefix(arg, "--file-id="))
		case arg == "--ordinal":
			if index+1 >= len(args) {
				return parsed, fmt.Errorf("--ordinal requires a value")
			}
			index++
			value, err := parseFileOrdinal(args[index])
			if err != nil {
				return parsed, err
			}
			parsed.ordinal = &value
		case strings.HasPrefix(arg, "--ordinal="):
			value, err := parseFileOrdinal(strings.TrimPrefix(arg, "--ordinal="))
			if err != nil {
				return parsed, err
			}
			parsed.ordinal = &value
		case strings.HasPrefix(arg, "--"):
			return parsed, fmt.Errorf("unknown flag %q", arg)
		default:
			positionals = append(positionals, arg)
		}
	}
	switch {
	case parsed.versionID != "":
	case parsed.fileID != "" && parsed.ordinal != nil:
	default:
		return parsed, fmt.Errorf("--version or --file-id with --ordinal is required")
	}
	if len(positionals) == 0 || len(positionals) > 2 {
		return parsed, fmt.Errorf("usage requires <path>")
	}
	if len(positionals) == 1 {
		parsed.path = positionals[0]
		return parsed, nil
	}
	parsed.workspace = positionals[0]
	parsed.path = positionals[1]
	return parsed, nil
}

func parseFileOrdinal(raw string) (int64, error) {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("--ordinal must be an integer")
	}
	return value, nil
}

func cmdFileDiff(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, fileDiffUsageText(filepath.Base(os.Args[0])))
		return nil
	}

	parsed, err := parseFileDiffArgs(args[2:])
	if err != nil {
		return fmt.Errorf("%s\n\n%s", err, fileDiffUsageText(filepath.Base(os.Args[0])))
	}

	ctx := context.Background()
	remote, err := openFSRemoteWorkspace(ctx, parsed.workspace)
	if err != nil {
		return err
	}
	defer remote.close()

	diff, err := remote.controlPlane.DiffFileVersions(
		ctx,
		fileDiffWorkspaceRef(remote.cfg, remote.selection, parsed),
		parsed.path,
		fileDiffOperandFromArgs(parsed.from),
		fileDiffOperandFromArgs(parsed.to),
	)
	if err != nil {
		return err
	}
	if diff.Binary {
		printSection(clr(ansiBold, "binary file diff"), []outputRow{
			{Label: "workspace", Value: remote.selection.Name},
			{Label: "path", Value: diff.Path},
			{Label: "from", Value: diff.From},
			{Label: "to", Value: diff.To},
		})
		return nil
	}
	fmt.Fprint(os.Stdout, diff.Diff)
	return nil
}

func cmdFileRestore(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, fileRestoreUsageText(filepath.Base(os.Args[0])))
		return nil
	}

	parsed, err := parseFileRestoreArgs(args[2:])
	if err != nil {
		return fmt.Errorf("%s\n\n%s", err, fileRestoreUsageText(filepath.Base(os.Args[0])))
	}

	ctx := context.Background()
	remote, err := openFSRemoteWorkspace(ctx, parsed.workspace)
	if err != nil {
		return err
	}
	defer remote.close()
	selector := controlplane.FileVersionSelector{
		VersionID: parsed.versionID,
		FileID:    parsed.fileID,
	}
	if parsed.ordinal != nil {
		selector.Ordinal = *parsed.ordinal
	}
	response, err := remote.controlPlane.RestoreFileVersion(ctx, remote.selection.ID, parsed.path, selector)
	if err != nil {
		return err
	}
	printSection(markerSuccess+" "+clr(ansiBold, "file restored from history"), []outputRow{
		{Label: "workspace", Value: remote.selection.Name},
		{Label: "path", Value: response.Path},
		{Label: "restored from", Value: response.RestoredFromVersionID},
		{Label: "new version", Value: response.VersionID},
	})
	return nil
}

func cmdFileUndelete(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, fileUndeleteUsageText(filepath.Base(os.Args[0])))
		return nil
	}

	parsed, err := parseFileUndeleteArgs(args[2:])
	if err != nil {
		return fmt.Errorf("%s\n\n%s", err, fileUndeleteUsageText(filepath.Base(os.Args[0])))
	}

	ctx := context.Background()
	remote, err := openFSRemoteWorkspace(ctx, parsed.workspace)
	if err != nil {
		return err
	}
	defer remote.close()

	selector := controlplane.FileVersionSelector{
		VersionID: parsed.versionID,
		FileID:    parsed.fileID,
	}
	if parsed.ordinal != nil {
		selector.Ordinal = *parsed.ordinal
	}
	response := controlplane.FileVersionUndeleteResponse{}
	if localRoot, ok, rootErr := activeSyncControlRootForWorkspace(remote.cfg, remote.selection); rootErr != nil {
		return rootErr
	} else if ok {
		result, controlErr := runSyncControlRequest(localRoot, syncControlRequest{
			Version:   syncControlVersion,
			Operation: syncControlOpUndelete,
			Path:      parsed.path,
			VersionID: parsed.versionID,
			FileID:    parsed.fileID,
			Ordinal:   selector.Ordinal,
		}, defaultSyncControlTimeout)
		if controlErr != nil {
			return controlErr
		}
		response = controlplane.FileVersionUndeleteResponse{
			WorkspaceID:            remote.selection.ID,
			Path:                   result.Path,
			VersionID:              result.VersionID,
			UndeletedFromVersionID: result.SourceID,
		}
	} else {
		response, err = remote.controlPlane.UndeleteFileVersion(ctx, remote.selection.ID, parsed.path, selector)
		if err != nil {
			return err
		}
	}
	printSection(markerSuccess+" "+clr(ansiBold, "file undeleted from history"), []outputRow{
		{Label: "workspace", Value: remote.selection.Name},
		{Label: "path", Value: response.Path},
		{Label: "undeleted from", Value: response.UndeletedFromVersionID},
		{Label: "new version", Value: response.VersionID},
	})
	return nil
}

func parseFileRestoreArgs(args []string) (fileRestoreArgs, error) {
	var parsed fileRestoreArgs
	positionals := make([]string, 0, 2)
	for index := 0; index < len(args); index++ {
		if ok, next, err := parseFileWorkspaceFlag(&parsed.workspace, args, index); ok || err != nil {
			if err != nil {
				return parsed, err
			}
			index = next
			continue
		}
		arg := args[index]
		switch {
		case arg == "--version":
			if index+1 >= len(args) {
				return parsed, fmt.Errorf("--version requires a value")
			}
			index++
			parsed.versionID = strings.TrimSpace(args[index])
		case strings.HasPrefix(arg, "--version="):
			parsed.versionID = strings.TrimSpace(strings.TrimPrefix(arg, "--version="))
		case arg == "--file-id":
			if index+1 >= len(args) {
				return parsed, fmt.Errorf("--file-id requires a value")
			}
			index++
			parsed.fileID = strings.TrimSpace(args[index])
		case strings.HasPrefix(arg, "--file-id="):
			parsed.fileID = strings.TrimSpace(strings.TrimPrefix(arg, "--file-id="))
		case arg == "--ordinal":
			if index+1 >= len(args) {
				return parsed, fmt.Errorf("--ordinal requires a value")
			}
			index++
			value, err := parseFileOrdinal(args[index])
			if err != nil {
				return parsed, err
			}
			parsed.ordinal = &value
		case strings.HasPrefix(arg, "--ordinal="):
			value, err := parseFileOrdinal(strings.TrimPrefix(arg, "--ordinal="))
			if err != nil {
				return parsed, err
			}
			parsed.ordinal = &value
		case strings.HasPrefix(arg, "--"):
			return parsed, fmt.Errorf("unknown flag %q", arg)
		default:
			positionals = append(positionals, arg)
		}
	}
	switch {
	case parsed.versionID != "":
	case parsed.fileID != "" && parsed.ordinal != nil:
	default:
		return parsed, fmt.Errorf("--version or --file-id with --ordinal is required")
	}
	if len(positionals) == 0 || len(positionals) > 2 {
		return parsed, fmt.Errorf("usage requires <path>")
	}
	if len(positionals) == 1 {
		parsed.path = positionals[0]
		return parsed, nil
	}
	parsed.workspace = positionals[0]
	parsed.path = positionals[1]
	return parsed, nil
}

func parseFileUndeleteArgs(args []string) (fileUndeleteArgs, error) {
	var parsed fileUndeleteArgs
	positionals := make([]string, 0, 2)
	for index := 0; index < len(args); index++ {
		if ok, next, err := parseFileWorkspaceFlag(&parsed.workspace, args, index); ok || err != nil {
			if err != nil {
				return parsed, err
			}
			index = next
			continue
		}
		arg := args[index]
		switch {
		case arg == "--version":
			if index+1 >= len(args) {
				return parsed, fmt.Errorf("--version requires a value")
			}
			index++
			parsed.versionID = strings.TrimSpace(args[index])
		case strings.HasPrefix(arg, "--version="):
			parsed.versionID = strings.TrimSpace(strings.TrimPrefix(arg, "--version="))
		case arg == "--file-id":
			if index+1 >= len(args) {
				return parsed, fmt.Errorf("--file-id requires a value")
			}
			index++
			parsed.fileID = strings.TrimSpace(args[index])
		case strings.HasPrefix(arg, "--file-id="):
			parsed.fileID = strings.TrimSpace(strings.TrimPrefix(arg, "--file-id="))
		case arg == "--ordinal":
			if index+1 >= len(args) {
				return parsed, fmt.Errorf("--ordinal requires a value")
			}
			index++
			value, err := parseFileOrdinal(args[index])
			if err != nil {
				return parsed, err
			}
			parsed.ordinal = &value
		case strings.HasPrefix(arg, "--ordinal="):
			value, err := parseFileOrdinal(strings.TrimPrefix(arg, "--ordinal="))
			if err != nil {
				return parsed, err
			}
			parsed.ordinal = &value
		case strings.HasPrefix(arg, "--"):
			return parsed, fmt.Errorf("unknown flag %q", arg)
		default:
			positionals = append(positionals, arg)
		}
	}
	if parsed.versionID == "" && ((parsed.fileID == "") != (parsed.ordinal == nil)) {
		return parsed, fmt.Errorf("--file-id and --ordinal must be used together")
	}
	if parsed.versionID != "" && (parsed.fileID != "" || parsed.ordinal != nil) {
		return parsed, fmt.Errorf("choose either --version or --file-id with --ordinal")
	}
	if len(positionals) == 0 || len(positionals) > 2 {
		return parsed, fmt.Errorf("usage requires <path>")
	}
	if len(positionals) == 1 {
		parsed.path = positionals[0]
		return parsed, nil
	}
	parsed.workspace = positionals[0]
	parsed.path = positionals[1]
	return parsed, nil
}

func parseFileDiffArgs(args []string) (fileDiffArgs, error) {
	var parsed fileDiffArgs
	positionals := make([]string, 0, 2)
	for index := 0; index < len(args); index++ {
		if ok, next, err := parseFileWorkspaceFlag(&parsed.workspace, args, index); ok || err != nil {
			if err != nil {
				return parsed, err
			}
			index = next
			continue
		}
		arg := args[index]
		switch {
		case arg == "--from-ref":
			if index+1 >= len(args) {
				return parsed, fmt.Errorf("--from-ref requires a value")
			}
			index++
			parsed.from.ref = normalizeFileDiffRef(args[index])
		case strings.HasPrefix(arg, "--from-ref="):
			parsed.from.ref = normalizeFileDiffRef(strings.TrimPrefix(arg, "--from-ref="))
		case arg == "--to-ref":
			if index+1 >= len(args) {
				return parsed, fmt.Errorf("--to-ref requires a value")
			}
			index++
			parsed.to.ref = normalizeFileDiffRef(args[index])
		case strings.HasPrefix(arg, "--to-ref="):
			parsed.to.ref = normalizeFileDiffRef(strings.TrimPrefix(arg, "--to-ref="))
		case arg == "--from-version":
			if index+1 >= len(args) {
				return parsed, fmt.Errorf("--from-version requires a value")
			}
			index++
			parsed.from.versionID = strings.TrimSpace(args[index])
		case strings.HasPrefix(arg, "--from-version="):
			parsed.from.versionID = strings.TrimSpace(strings.TrimPrefix(arg, "--from-version="))
		case arg == "--to-version":
			if index+1 >= len(args) {
				return parsed, fmt.Errorf("--to-version requires a value")
			}
			index++
			parsed.to.versionID = strings.TrimSpace(args[index])
		case strings.HasPrefix(arg, "--to-version="):
			parsed.to.versionID = strings.TrimSpace(strings.TrimPrefix(arg, "--to-version="))
		case arg == "--from-file-id":
			if index+1 >= len(args) {
				return parsed, fmt.Errorf("--from-file-id requires a value")
			}
			index++
			parsed.from.fileID = strings.TrimSpace(args[index])
		case strings.HasPrefix(arg, "--from-file-id="):
			parsed.from.fileID = strings.TrimSpace(strings.TrimPrefix(arg, "--from-file-id="))
		case arg == "--to-file-id":
			if index+1 >= len(args) {
				return parsed, fmt.Errorf("--to-file-id requires a value")
			}
			index++
			parsed.to.fileID = strings.TrimSpace(args[index])
		case strings.HasPrefix(arg, "--to-file-id="):
			parsed.to.fileID = strings.TrimSpace(strings.TrimPrefix(arg, "--to-file-id="))
		case arg == "--from-ordinal":
			if index+1 >= len(args) {
				return parsed, fmt.Errorf("--from-ordinal requires a value")
			}
			index++
			value, err := parseFileOrdinal(args[index])
			if err != nil {
				return parsed, err
			}
			parsed.from.ordinal = &value
		case strings.HasPrefix(arg, "--from-ordinal="):
			value, err := parseFileOrdinal(strings.TrimPrefix(arg, "--from-ordinal="))
			if err != nil {
				return parsed, err
			}
			parsed.from.ordinal = &value
		case arg == "--to-ordinal":
			if index+1 >= len(args) {
				return parsed, fmt.Errorf("--to-ordinal requires a value")
			}
			index++
			value, err := parseFileOrdinal(args[index])
			if err != nil {
				return parsed, err
			}
			parsed.to.ordinal = &value
		case strings.HasPrefix(arg, "--to-ordinal="):
			value, err := parseFileOrdinal(strings.TrimPrefix(arg, "--to-ordinal="))
			if err != nil {
				return parsed, err
			}
			parsed.to.ordinal = &value
		case strings.HasPrefix(arg, "--"):
			return parsed, fmt.Errorf("unknown flag %q", arg)
		default:
			positionals = append(positionals, arg)
		}
	}
	if len(positionals) == 0 || len(positionals) > 2 {
		return parsed, fmt.Errorf("usage requires <path>")
	}
	if len(positionals) == 1 {
		parsed.path = positionals[0]
	} else {
		parsed.workspace = positionals[0]
		parsed.path = positionals[1]
	}
	if err := validateFileDiffOperand(parsed.from, "from"); err != nil {
		return parsed, err
	}
	if !hasFileDiffOperand(parsed.to) {
		parsed.to.ref = "head"
	}
	if err := validateFileDiffOperand(parsed.to, "to"); err != nil {
		return parsed, err
	}
	return parsed, nil
}

func normalizeFileDiffRef(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func usesFileDiffLiveRef(operand fileDiffOperandArgs) bool {
	switch strings.TrimSpace(strings.ToLower(operand.ref)) {
	case "head", "working-copy":
		return true
	default:
		return false
	}
}

func fileDiffWorkspaceRef(cfg config, selection workspaceSelection, parsed fileDiffArgs) string {
	workspaceRef := selection.ID
	if usesFileDiffLiveRef(parsed.from) || usesFileDiffLiveRef(parsed.to) {
		if mode, modeErr := effectiveProductMode(cfg); modeErr == nil && mode == productModeLocal {
			return selection.Name
		}
	}
	return workspaceRef
}

func hasFileDiffOperand(operand fileDiffOperandArgs) bool {
	return operand.ref != "" || operand.versionID != "" || operand.fileID != "" || operand.ordinal != nil
}

func validateFileDiffOperand(operand fileDiffOperandArgs, side string) error {
	selectorCount := 0
	if operand.ref != "" {
		selectorCount++
		if operand.ref != "head" && operand.ref != "working-copy" {
			return fmt.Errorf("--%s-ref must be head or working-copy", side)
		}
	}
	if operand.versionID != "" {
		selectorCount++
	}
	if operand.fileID != "" || operand.ordinal != nil {
		if operand.fileID == "" || operand.ordinal == nil {
			return fmt.Errorf("--%s-file-id and --%s-ordinal must be used together", side, side)
		}
		selectorCount++
	}
	if selectorCount == 0 {
		return fmt.Errorf("a --%s-* selector is required", side)
	}
	if selectorCount > 1 {
		return fmt.Errorf("choose exactly one --%s-* selector", side)
	}
	return nil
}

func fileDiffOperandFromArgs(operand fileDiffOperandArgs) controlplane.FileVersionDiffOperand {
	response := controlplane.FileVersionDiffOperand{
		Ref:       operand.ref,
		VersionID: operand.versionID,
		FileID:    operand.fileID,
	}
	if operand.ordinal != nil {
		response.Ordinal = *operand.ordinal
	}
	return response
}

func fileUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s fs <subcommand>

Subcommands:
  history <path>                              Show ordered file history for a path
  cat <path> --version <id>                   Show the content for an exact file version
  diff <path> --from-version <id>             Diff a historical version against head or another selector
  restore <path> --version <id>               Restore historical content into the live workspace
  undelete <path>                             Revive the latest deleted lineage or a selected version

Run '%s fs <subcommand> --help' for details.
`, bin, bin)
}

func fileHistoryUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s fs [-w <workspace>] history <path> [--order asc|desc] [--limit <n>] [--cursor <opaque>]

List file history in deterministic lineage order. The default order is desc.
Use --limit and --cursor to page through long histories without losing history order.
If -w is omitted, AFS prompts for a workspace when needed.
`, bin)
}

func fileShowUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s fs [-w <workspace>] cat <path> (--version <version-id> | --file-id <file-id> --ordinal <n>)

Show the exact historical content for a file version.
If -w is omitted, AFS prompts for a workspace when needed.
`, bin)
}

func fileDiffUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s fs [-w <workspace>] diff <path> (--from-ref <head|working-copy> | --from-version <version-id> | --from-file-id <file-id> --from-ordinal <n>) [--to-ref <head|working-copy> | --to-version <version-id> | --to-file-id <file-id> --to-ordinal <n>]

Diff one file version selector against another. If the --to selector is omitted, AFS diffs against head.
If -w is omitted, AFS prompts for a workspace when needed.
`, bin)
}

func fileRestoreUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s fs [-w <workspace>] restore <path> (--version <version-id> | --file-id <file-id> --ordinal <n>)

Restore historical content into the live workspace and create a new latest version.
If -w is omitted, AFS prompts for a workspace when needed.
`, bin)
}

func fileUndeleteUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s fs [-w <workspace>] undelete <path> [--version <version-id> | --file-id <file-id> --ordinal <n>]

Revive the latest deleted lineage at a path by default, or restore a selected historical version from a deleted lineage.
If -w is omitted, AFS prompts for a workspace when needed.
`, bin)
}
