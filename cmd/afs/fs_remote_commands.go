package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/redis/agent-filesystem/internal/controlplane"
)

type fsRemoteWorkspace struct {
	cfg          config
	close        func()
	selection    workspaceSelection
	controlPlane afsControlPlane
}

func openFSRemoteWorkspace(ctx context.Context, workspace string) (*fsRemoteWorkspace, error) {
	cfg, service, closeFn, err := openAFSControlPlane(ctx)
	if err != nil {
		return nil, err
	}
	selection, err := resolveFSWorkspaceSelection(ctx, cfg, service, workspace)
	if err != nil {
		closeFn()
		return nil, err
	}
	return &fsRemoteWorkspace{
		cfg:          cfg,
		close:        closeFn,
		selection:    selection,
		controlPlane: service,
	}, nil
}

func resolveFSWorkspaceSelection(ctx context.Context, cfg config, service afsControlPlane, workspace string) (workspaceSelection, error) {
	return resolveCommandWorkspaceSelectionFromControlPlane(ctx, cfg, service, workspace)
}

func cmdFSList(workspace string, args []string) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		fmt.Fprint(os.Stderr, fsListUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	fs := flag.NewFlagSet("fs ls", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%s", fsListUsageText(filepath.Base(os.Args[0])))
	}
	if fs.NArg() > 1 {
		return fmt.Errorf("%s", fsListUsageText(filepath.Base(os.Args[0])))
	}
	targetPath := "/"
	if fs.NArg() == 1 {
		targetPath = normalizeFSRemotePath(fs.Arg(0))
	}

	ctx := context.Background()
	remote, err := openFSRemoteWorkspace(ctx, workspace)
	if err != nil {
		return err
	}
	defer remote.close()

	fmt.Println()
	fmt.Println("workspace: " + remote.selection.Name)
	fmt.Println("path: " + targetPath)
	fmt.Println()
	var entries []controlplane.TreeItem
	tree, err := remote.controlPlane.GetTree(ctx, remote.selection.ID, "working-copy", targetPath, 1)
	if err != nil {
		file, fileErr := remote.controlPlane.GetFileContent(ctx, remote.selection.ID, "working-copy", targetPath)
		if fileErr != nil {
			return err
		}
		entries = []controlplane.TreeItem{fsTreeItemFromFile(file)}
	} else {
		entries = tree.Items
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Kind == "dir" && entries[j].Kind != "dir" {
			return true
		}
		if entries[i].Kind != "dir" && entries[j].Kind == "dir" {
			return false
		}
		return entries[i].Name < entries[j].Name
	})

	if len(entries) == 0 {
		fmt.Println("(empty)")
	} else {
		printPlainTable([]string{"Name", "Type", "Size", "Modified"}, fsListRows(entries))
	}
	fmt.Println()
	return nil
}

func fsTreeItemFromFile(file controlplane.FileContentResponse) controlplane.TreeItem {
	return controlplane.TreeItem{
		Path:       file.Path,
		Name:       path.Base(file.Path),
		Kind:       file.Kind,
		Size:       file.Size,
		ModifiedAt: file.ModifiedAt,
		Target:     file.Target,
	}
}

func fsListRows(entries []controlplane.TreeItem) [][]string {
	rows := make([][]string, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, []string{
			entry.Name,
			fsDisplayType(entry.Kind),
			fsDisplaySize(entry),
			formatFSMtime(entry.ModifiedAt),
		})
	}
	return rows
}

func cmdFSCat(workspace string, args []string) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		fmt.Fprint(os.Stderr, fsCatUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	if len(args) != 1 {
		return fmt.Errorf("%s", fsCatUsageText(filepath.Base(os.Args[0])))
	}
	ctx := context.Background()
	remote, err := openFSRemoteWorkspace(ctx, workspace)
	if err != nil {
		return err
	}
	defer remote.close()

	file, err := remote.controlPlane.GetFileContent(ctx, remote.selection.ID, "working-copy", normalizeFSRemotePath(args[0]))
	if err != nil {
		return err
	}
	if file.Binary {
		return fmt.Errorf("path %q is binary and cannot be printed as text", file.Path)
	}
	_, err = io.WriteString(os.Stdout, file.Content)
	return err
}

func cmdFSFind(workspace string, args []string) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		fmt.Fprint(os.Stderr, fsFindUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	targetPath, pattern, typeFilter, err := parseFSFindArgs(args)
	if err != nil {
		return err
	}
	ctx := context.Background()
	remote, err := openFSRemoteWorkspace(ctx, workspace)
	if err != nil {
		return err
	}
	defer remote.close()

	tree, err := remote.controlPlane.GetTree(ctx, remote.selection.ID, "working-copy", targetPath, 1000)
	if err != nil {
		return err
	}
	matches := fsFindMatches(tree.Items, pattern, typeFilter)
	sort.Strings(matches)
	for _, match := range matches {
		fmt.Println(match)
	}
	return nil
}

func cmdFSGrep(workspace string, args []string) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		fmt.Fprint(os.Stderr, grepUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	opts, err := parseGrepArgs(args)
	if err != nil {
		return err
	}
	if strings.TrimSpace(workspace) != "" {
		opts.workspace = strings.TrimSpace(workspace)
	}

	matcher, err := compileGrepMatcher(opts)
	if err != nil {
		return err
	}

	ctx := context.Background()
	remote, err := openFSRemoteWorkspace(ctx, opts.workspace)
	if err != nil {
		return err
	}
	defer remote.close()

	targets, err := collectControlPlaneGrepTargets(ctx, remote, normalizeAFSGrepPath(opts.path))
	if err != nil {
		return err
	}
	sort.Slice(targets, func(i, j int) bool {
		return targets[i].path < targets[j].path
	})
	for _, target := range targets {
		if err := grepProcessFile(target.path, target.content, opts, matcher); err != nil {
			return err
		}
	}
	return nil
}

func collectControlPlaneGrepTargets(ctx context.Context, remote *fsRemoteWorkspace, searchPath string) ([]grepFileTarget, error) {
	tree, err := remote.controlPlane.GetTree(ctx, remote.selection.ID, "working-copy", searchPath, grepTreeMaxDepth)
	if err == nil {
		targets := make([]grepFileTarget, 0, len(tree.Items))
		for _, item := range tree.Items {
			if item.Kind != "file" {
				continue
			}
			file, err := remote.controlPlane.GetFileContent(ctx, remote.selection.ID, "working-copy", item.Path)
			if err != nil {
				return nil, err
			}
			if file.Binary {
				continue
			}
			targets = append(targets, grepFileTarget{
				path:    file.Path,
				content: []byte(file.Content),
				loaded:  true,
			})
		}
		return targets, nil
	}

	file, fileErr := remote.controlPlane.GetFileContent(ctx, remote.selection.ID, "working-copy", searchPath)
	if fileErr != nil {
		return nil, err
	}
	if file.Binary {
		return []grepFileTarget{}, nil
	}
	return []grepFileTarget{{
		path:    file.Path,
		content: []byte(file.Content),
		loaded:  true,
	}}, nil
}

func fsFindMatches(items []controlplane.TreeItem, pattern, typeFilter string) []string {
	matches := make([]string, 0)
	for _, item := range items {
		if typeFilter != "" && item.Kind != typeFilter {
			continue
		}
		name := item.Name
		if name == "" {
			name = path.Base(item.Path)
		}
		ok, err := path.Match(pattern, name)
		if err != nil {
			ok = name == pattern
		}
		if ok {
			matches = append(matches, item.Path)
		}
	}
	return matches
}

func parseFSFindArgs(args []string) (string, string, string, error) {
	targetPath := "/"
	pattern := "*"
	typeFilter := ""
	positionals := make([]string, 0, 1)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-name":
			if i+1 >= len(args) {
				return "", "", "", errors.New("-name requires a pattern")
			}
			i++
			pattern = args[i]
		case "-type":
			if i+1 >= len(args) {
				return "", "", "", errors.New("-type requires f, d, l, file, dir, or symlink")
			}
			i++
			parsed, err := parseFSFindType(args[i])
			if err != nil {
				return "", "", "", err
			}
			typeFilter = parsed
		case "-print":
			continue
		default:
			if strings.HasPrefix(arg, "-") {
				return "", "", "", fmt.Errorf("unknown find flag %q", arg)
			}
			positionals = append(positionals, arg)
		}
	}
	if len(positionals) > 1 {
		return "", "", "", fmt.Errorf("%s", fsFindUsageText(filepath.Base(os.Args[0])))
	}
	if len(positionals) == 1 {
		targetPath = normalizeFSRemotePath(positionals[0])
	}
	return targetPath, pattern, typeFilter, nil
}

func parseFSFindType(raw string) (string, error) {
	switch strings.TrimSpace(raw) {
	case "f", "file":
		return "file", nil
	case "d", "dir":
		return "dir", nil
	case "l", "symlink":
		return "symlink", nil
	case "":
		return "", nil
	default:
		return "", fmt.Errorf("unknown find type %q", raw)
	}
}

func normalizeFSRemotePath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "." {
		return "/"
	}
	return normalizeAFSGrepPath(raw)
}

func fsDisplayType(raw string) string {
	switch raw {
	case "dir":
		return "dir"
	case "symlink":
		return "link"
	case "file":
		return "file"
	default:
		return raw
	}
}

func fsDisplaySize(entry controlplane.TreeItem) string {
	if entry.Kind == "dir" {
		return "-"
	}
	return formatBytes(entry.Size)
}

func formatFSMtime(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "unknown"
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return raw
	}
	return parsed.Local().Format("2006-01-02 15:04")
}

func fsListUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s fs [workspace] ls [path]

List files in a workspace directory.
`, bin)
}

func fsCatUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s fs [workspace] cat <path>
  %s fs [workspace] cat <path> --version <version-id>

Print a workspace file to stdout. With --version or --file-id/--ordinal, print
an exact historical file version.
`, bin, bin)
}

func fsFindUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s fs [workspace] find [path] [-name <pattern>] [-type f|d|l] [-print]

Find workspace paths by basename pattern.
`, bin)
}
