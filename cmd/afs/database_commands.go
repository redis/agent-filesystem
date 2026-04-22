package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/agent-filesystem/internal/controlplane"
)

func cmdDatabase(args []string) error {
	if len(args) < 2 || isHelpArg(args[1]) {
		fmt.Fprint(os.Stderr, databaseUsageText(filepath.Base(os.Args[0])))
		return nil
	}

	switch args[1] {
	case "list":
		return cmdDatabaseList(args)
	case "use":
		return cmdDatabaseUse(args)
	default:
		return fmt.Errorf("unknown database subcommand %q\n\n%s", args[1], databaseUsageText(filepath.Base(os.Args[0])))
	}
}

func cmdDatabaseList(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, databaseListUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	if len(args) != 2 {
		return fmt.Errorf("%s", databaseListUsageText(filepath.Base(os.Args[0])))
	}

	cfg, client, err := openManagedDatabaseClient(context.Background())
	if err != nil {
		return err
	}

	response, err := client.listDatabases(context.Background())
	if err != nil {
		return err
	}

	rows := make([]boxRow, 0, len(response.Items)+1)
	if len(response.Items) == 0 {
		rows = append(rows, boxRow{Value: clr(ansiDim, "No databases found")})
	} else {
		layout := newDatabaseListLayout(response.Items)
		rows = append(rows, boxRow{Value: layout.header()})
		for _, item := range response.Items {
			rows = append(rows, boxRow{Value: layout.row(item, strings.TrimSpace(cfg.DatabaseID) == item.ID)})
		}
	}
	printBox(clr(ansiBold, "databases on "+configRemoteLabel(cfg)), rows)
	return nil
}

func cmdDatabaseUse(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, databaseUseUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	if len(args) != 3 {
		return fmt.Errorf("%s", databaseUseUsageText(filepath.Base(os.Args[0])))
	}

	cfg, client, err := openManagedDatabaseClient(context.Background())
	if err != nil {
		return err
	}

	ref := strings.TrimSpace(args[2])
	if ref == "" {
		return fmt.Errorf("%s", databaseUseUsageText(filepath.Base(os.Args[0])))
	}

	if strings.EqualFold(ref, "auto") || strings.EqualFold(ref, "default") {
		cfg.DatabaseID = ""
		cfg.CurrentWorkspaceID = ""
		if err := prepareConfigForSave(&cfg); err != nil {
			return err
		}
		if err := saveConfig(cfg); err != nil {
			return err
		}
		printBox(markerSuccess+" "+clr(ansiBold, "database selection cleared"), []boxRow{
			{Label: "database", Value: "auto"},
			{Label: "config", Value: configPathLabel()},
		})
		return nil
	}

	response, err := client.listDatabases(context.Background())
	if err != nil {
		return err
	}
	database, err := resolveDatabaseReference(ref, response.Items)
	if err != nil {
		return err
	}

	cfg.DatabaseID = database.ID
	cfg.CurrentWorkspaceID = ""
	if err := prepareConfigForSave(&cfg); err != nil {
		return err
	}
	if err := saveConfig(cfg); err != nil {
		return err
	}

	printBox(markerSuccess+" "+clr(ansiBold, "database selected"), []boxRow{
		{Label: "database", Value: database.Name},
		{Label: "id", Value: database.ID},
		{Label: "config", Value: configPathLabel()},
	})
	return nil
}

func openManagedDatabaseClient(ctx context.Context) (config, *httpControlPlaneClient, error) {
	cfg, err := loadAFSConfig()
	if err != nil {
		return config{}, nil, err
	}

	productMode, err := effectiveProductMode(cfg)
	if err != nil {
		return config{}, nil, err
	}
	if productMode == productModeLocal {
		return config{}, nil, fmt.Errorf("database commands require a self-managed control plane\nRun '%s setup' or '%s config set controlPlane.url <url>' first", filepath.Base(os.Args[0]), filepath.Base(os.Args[0]))
	}

	client, _, err := newHTTPControlPlaneClient(ctx, cfg)
	if err != nil {
		return config{}, nil, err
	}
	return cfg, client, nil
}

func resolveManagedDatabaseForWrite(ctx context.Context, cfg config, client *httpControlPlaneClient, explicitRef, action string) (controlplane.DatabaseRecord, error) {
	response, err := client.listDatabases(ctx)
	if err != nil {
		return controlplane.DatabaseRecord{}, err
	}
	if explicit := strings.TrimSpace(explicitRef); explicit != "" {
		return resolveDatabaseReference(explicit, response.Items)
	}
	if configuredID := strings.TrimSpace(cfg.DatabaseID); configuredID != "" {
		return resolveDatabaseReference(configuredID, response.Items)
	}
	switch len(response.Items) {
	case 0:
		return controlplane.DatabaseRecord{}, fmt.Errorf("control plane at %s returned no databases", client.baseURL)
	case 1:
		return response.Items[0], nil
	}
	for _, item := range response.Items {
		if item.IsDefault {
			return item, nil
		}
	}
	if !isInteractiveTerminal() {
		return controlplane.DatabaseRecord{}, fmt.Errorf("control plane at %s has %d databases and no default database is set; choose one with --database or run '%s database use <id>'", client.baseURL, len(response.Items), filepath.Base(os.Args[0]))
	}
	return promptDatabaseSelection(action, response.Items)
}

func promptDatabaseSelection(action string, items []controlplane.DatabaseRecord) (controlplane.DatabaseRecord, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println()
	fmt.Printf("  %s\n\n", clr(ansiBold, "Choose a database for "+action))
	printPromptDatabaseTable(os.Stdout, items)
	fmt.Println()

	for {
		choice, err := promptString(reader, os.Stdout, "  Choose database\n  "+clr(ansiDim, "Enter a number, database id, or name"), "1")
		if err != nil {
			return controlplane.DatabaseRecord{}, err
		}
		selected, err := resolvePromptDatabaseChoice(choice, items)
		if err == nil {
			return selected, nil
		}
		fmt.Printf("  %s\n\n", clr(ansiYellow, err.Error()))
	}
}

func printPromptDatabaseTable(out *os.File, items []controlplane.DatabaseRecord) {
	nameHeader := clr(ansiDim, "Name")
	idHeader := clr(ansiDim, "ID")
	endpointHeader := clr(ansiDim, "Endpoint")

	nameWidth := runeWidth(nameHeader)
	idWidth := runeWidth(idHeader)
	endpointWidth := runeWidth(endpointHeader)
	for index, item := range items {
		nameWidth = maxInt(nameWidth, runeWidth(fmt.Sprintf("%d. %s", index+1, item.Name)))
		idWidth = maxInt(idWidth, runeWidth(item.ID))
		endpointWidth = maxInt(endpointWidth, runeWidth(item.RedisAddr))
	}

	fmt.Fprintf(out, "    %s   %s   %s\n",
		padVisibleText(nameHeader, nameWidth),
		padVisibleText(idHeader, idWidth),
		endpointHeader,
	)
	for index, item := range items {
		fmt.Fprintf(out, "    %s   %s   %s\n",
			padVisibleText(fmt.Sprintf("%d. %s", index+1, clr(ansiBold, item.Name)), nameWidth),
			padVisibleText(item.ID, idWidth),
			item.RedisAddr,
		)
	}
}

func resolvePromptDatabaseChoice(choice string, items []controlplane.DatabaseRecord) (controlplane.DatabaseRecord, error) {
	choice = strings.TrimSpace(choice)
	if choice == "" {
		return controlplane.DatabaseRecord{}, fmt.Errorf("database choice is required")
	}
	if index, err := strconv.Atoi(choice); err == nil {
		if index >= 1 && index <= len(items) {
			return items[index-1], nil
		}
		return controlplane.DatabaseRecord{}, fmt.Errorf("choose a listed database")
	}
	return resolveDatabaseReference(choice, items)
}

func resolveDatabaseReference(ref string, items []controlplane.DatabaseRecord) (controlplane.DatabaseRecord, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return controlplane.DatabaseRecord{}, fmt.Errorf("database is required")
	}
	for _, item := range items {
		if item.ID == ref {
			return item, nil
		}
	}

	matches := make([]controlplane.DatabaseRecord, 0)
	for _, item := range items {
		if item.Name == ref {
			matches = append(matches, item)
		}
	}
	switch len(matches) {
	case 0:
		return controlplane.DatabaseRecord{}, fmt.Errorf("database %q does not exist", ref)
	case 1:
		return matches[0], nil
	default:
		ids := make([]string, 0, len(matches))
		for _, item := range matches {
			ids = append(ids, item.ID)
		}
		sort.Strings(ids)
		return controlplane.DatabaseRecord{}, fmt.Errorf("database %q exists multiple times; use a database id instead: %s", ref, strings.Join(ids, ", "))
	}
}

const databaseListColumnSep = "  "

type databaseListLayout struct {
	markerWidth     int
	nameWidth       int
	idWidth         int
	endpointWidth   int
	workspacesWidth int
	roleWidth       int
}

func newDatabaseListLayout(items []controlplane.DatabaseRecord) databaseListLayout {
	layout := databaseListLayout{
		markerWidth:     1,
		nameWidth:       runeWidth("Name"),
		idWidth:         runeWidth("ID"),
		endpointWidth:   runeWidth("Endpoint"),
		workspacesWidth: runeWidth("Workspaces"),
		roleWidth:       runeWidth("Role"),
	}
	for _, item := range items {
		layout.nameWidth = maxInt(layout.nameWidth, runeWidth(item.Name))
		layout.idWidth = maxInt(layout.idWidth, runeWidth(item.ID))
		layout.endpointWidth = maxInt(layout.endpointWidth, runeWidth(item.RedisAddr))
		layout.workspacesWidth = maxInt(layout.workspacesWidth, runeWidth(strconv.Itoa(item.WorkspaceCount)))
		layout.roleWidth = maxInt(layout.roleWidth, runeWidth(databaseListRole(item)))
	}

	maxContentWidth := maxBoxText - 5*runeWidth(databaseListColumnSep) - layout.markerWidth
	for layout.nameWidth+layout.idWidth+layout.endpointWidth+layout.workspacesWidth+layout.roleWidth > maxContentWidth {
		switch {
		case layout.endpointWidth > runeWidth("Endpoint"):
			layout.endpointWidth--
		case layout.nameWidth > runeWidth("Name"):
			layout.nameWidth--
		case layout.idWidth > runeWidth("ID"):
			layout.idWidth--
		default:
			return layout
		}
	}
	return layout
}

func (l databaseListLayout) header() string {
	return strings.Join([]string{
		clr(ansiDim, padVisibleText("", l.markerWidth)),
		clr(ansiDim, padVisibleText("Name", l.nameWidth)),
		clr(ansiDim, padVisibleText("ID", l.idWidth)),
		clr(ansiDim, padVisibleText("Endpoint", l.endpointWidth)),
		clr(ansiDim, padVisibleText("Workspaces", l.workspacesWidth)),
		clr(ansiDim, padVisibleText("Role", l.roleWidth)),
	}, databaseListColumnSep)
}

func (l databaseListLayout) row(item controlplane.DatabaseRecord, selected bool) string {
	return strings.Join([]string{
		padVisibleText(workspaceListMarker(selected), l.markerWidth),
		padVisibleText(fitDisplayText(item.Name, l.nameWidth), l.nameWidth),
		padVisibleText(fitDisplayText(item.ID, l.idWidth), l.idWidth),
		padVisibleText(fitDisplayText(item.RedisAddr, l.endpointWidth), l.endpointWidth),
		padVisibleText(strconv.Itoa(item.WorkspaceCount), l.workspacesWidth),
		padVisibleText(databaseListRole(item), l.roleWidth),
	}, databaseListColumnSep)
}

func databaseListRole(item controlplane.DatabaseRecord) string {
	if item.IsDefault {
		return "default"
	}
	return ""
}

func databaseUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s database <subcommand>

Subcommands:
  list                List configured databases
  use <database|auto> Persist the database used for new workspaces/imports
`, bin)
}

func databaseListUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s database list

List the databases configured in the control plane.
`, bin)
}

func databaseUseUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s database use <database-id|database-name|auto>

Choose which control-plane database new workspaces and imports should use.
Use "auto" to clear the local override and fall back to the control-plane default.
`, bin)
}

func databaseTimestamp(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "unknown"
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return raw
	}
	return parsed.Local().Format("2006-01-02 15:04")
}
