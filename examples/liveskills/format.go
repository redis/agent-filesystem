package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const textOutputWidth = 80

func printJSON(w io.Writer, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s\n", data)
	return err
}

func printRows(w io.Writer, title string, rows [][2]string) {
	fmt.Fprintln(w, title)
	for _, row := range rows {
		fmt.Fprintf(w, "%-12s %s\n", row[0], row[1])
	}
}

type outputStyle struct {
	color bool
}

func styleForOutput(w io.Writer, env map[string]string) outputStyle {
	if envValue(env, "NO_COLOR") != "" || envValue(env, "TERM") == "dumb" {
		return outputStyle{}
	}
	file, ok := w.(*os.File)
	if !ok || !isTerminalFile(file) {
		return outputStyle{}
	}
	return outputStyle{color: true}
}

func isTerminalFile(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func (s outputStyle) paint(code string, value string) string {
	if !s.color || value == "" {
		return value
	}
	return "\x1b[" + code + "m" + value + "\x1b[0m"
}

func (s outputStyle) bold(value string) string {
	return s.paint("1", value)
}

func (s outputStyle) dim(value string) string {
	return s.paint("38;5;102", value)
}

func (s outputStyle) text(value string) string {
	return s.paint("38;5;145", value)
}

func (s outputStyle) cyan(value string) string {
	return s.paint("36", value)
}

func printSkillList(w io.Writer, rows []SkillListItem, style outputStyle) {
	if len(rows) == 0 {
		fmt.Fprintln(w, "No skills found")
		return
	}
	for index, row := range visibleSkillRows(rows) {
		pointer := " "
		name := style.text(skillSearchName(row))
		if index == 0 {
			pointer = ">"
			name = style.bold(skillSearchName(row))
		}
		fmt.Fprintf(w, "  %s %s %s", pointer, name, style.dim(row.Name))
		if row.Version != "" && row.Version != "-" {
			fmt.Fprintf(w, " %s", style.cyan(row.Version))
		}
		fmt.Fprintln(w)
	}
}

func printAvailableSkillList(w io.Writer, rows []SkillListItem, query string, style outputStyle) {
	query = strings.TrimSpace(query)
	if query == "" {
		fmt.Fprintln(w, style.text("Available Skills"))
	} else {
		fmt.Fprintf(w, "%s %s\n", style.text("Search Results:"), query)
	}
	fmt.Fprintln(w)
	if len(rows) == 0 {
		fmt.Fprintln(w, style.dim("No skills found"))
		return
	}
	printAvailableSkillRows(w, rows, style)
}

func printAvailableSkillRows(w io.Writer, rows []SkillListItem, style outputStyle) {
	for index, row := range visibleSkillRows(rows) {
		if index > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w, style.cyan(skillSearchName(row)))
		fmt.Fprintf(w, "  %s %s\n", style.dim("Ref:"), row.Name)
		if row.Version != "" && row.Version != "-" {
			fmt.Fprintf(w, "  %s %s\n", style.dim("Version:"), row.Version)
		}
		fmt.Fprintf(w, "  %s liveskills add %s\n", style.dim("Install:"), row.Name)
	}
}

func printInstalledSkillList(w io.Writer, rows []InstalledSkillItem, global bool, style outputStyle) {
	if len(rows) == 0 {
		if global {
			fmt.Fprintln(w, "No global skills found.")
		} else {
			fmt.Fprintln(w, "No project skills found.")
			fmt.Fprintln(w, "Try listing global skills with -g")
		}
		return
	}
	if global {
		fmt.Fprintln(w, "Global Skills")
	} else {
		fmt.Fprintln(w, "Project Skills")
	}
	fmt.Fprintln(w)
	liveRows, localRows := partitionInstalledSkills(rows)
	if global {
		projectLiveRows, globalLiveRows := splitInstalledSkillsByScope(liveRows)
		if len(projectLiveRows) > 0 {
			printInstalledSkillSection(w, "Current Project Live Skills (managed by LiveSkills)", projectLiveRows, style)
			fmt.Fprintln(w)
		}
		printInstalledSkillSection(w, "Global Live Skills (managed by LiveSkills)", globalLiveRows, style)
		fmt.Fprintln(w)
		printInstalledSkillSection(w, "Local Skills (not managed by LiveSkills)", localRows, style)
		return
	}
	printInstalledSkillSection(w, "Live Skills (managed by LiveSkills)", liveRows, style)
	fmt.Fprintln(w)
	printInstalledSkillSection(w, "Local Skills (not managed by LiveSkills)", localRows, style)
}

func partitionInstalledSkills(rows []InstalledSkillItem) ([]InstalledSkillItem, []InstalledSkillItem) {
	var liveRows []InstalledSkillItem
	var localRows []InstalledSkillItem
	for _, row := range rows {
		if row.Live || row.Managed {
			liveRows = append(liveRows, row)
			continue
		}
		localRows = append(localRows, row)
	}
	return liveRows, localRows
}

func splitInstalledSkillsByScope(rows []InstalledSkillItem) ([]InstalledSkillItem, []InstalledSkillItem) {
	var projectRows []InstalledSkillItem
	var globalRows []InstalledSkillItem
	for _, row := range rows {
		if row.Scope == scopeProject {
			projectRows = append(projectRows, row)
			continue
		}
		globalRows = append(globalRows, row)
	}
	return projectRows, globalRows
}

func printInstalledSkillSection(w io.Writer, title string, rows []InstalledSkillItem, style outputStyle) {
	fmt.Fprintln(w, title)
	if len(rows) == 0 {
		fmt.Fprintln(w, "  None installed.")
		return
	}
	printInstalledSkillRows(w, rows, style)
}

func printInstalledSkillRows(w io.Writer, rows []InstalledSkillItem, style outputStyle) {
	for _, row := range rows {
		name := defaultString(row.DisplayName, row.Name)
		fmt.Fprintf(w, "%s %s\n", style.cyan(name), style.dim(row.Path))
		if len(row.Agents) == 0 {
			fmt.Fprintf(w, "  %s %s\n", style.dim("Agents:"), "not linked")
		} else {
			fmt.Fprintf(w, "  %s %s\n", style.dim("Agents:"), strings.Join(row.Agents, ", "))
		}
		if row.Live || row.Managed {
			if row.Scope != "" {
				fmt.Fprintf(w, "  %s %s\n", style.dim("Scope:"), row.Scope)
			}
			status := "live"
			if row.Status != "" {
				status = row.Status
			}
			if row.Mode != "" && row.Mode != installModeSymlink {
				status += ", " + row.Mode
			}
			fmt.Fprintf(w, "  %s %s\n", style.dim("Status:"), status+", managed by LiveSkills")
			if row.Version != "" {
				fmt.Fprintf(w, "  %s %s\n", style.dim("Version:"), row.Version)
			}
		}
	}
}

func printSourceSkillList(w io.Writer, rows []SkillSourceListItem) {
	if len(rows) == 0 {
		fmt.Fprintln(w, "No skills found")
		return
	}
	for _, row := range rows {
		fmt.Fprintf(w, "%-32s %s\n", row.Slug, row.Description)
	}
}

func printScannedSkillList(w io.Writer, rows []ScannedSkillItem) {
	if len(rows) == 0 {
		fmt.Fprintln(w, "No local skills found.")
		return
	}
	fmt.Fprintln(w, "Local Skills")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Summary")
	fmt.Fprintf(w, "  %s found\n", pluralCount(len(rows), "skill"))
	for _, line := range scanSummaryLines(rows) {
		fmt.Fprintf(w, "  %s\n", line)
	}
	fmt.Fprintln(w)

	for _, row := range rows {
		fmt.Fprintln(w, row.Name)
		fmt.Fprintf(w, "  Path: %s\n", row.DisplayPath)
		fmt.Fprintf(w, "  Scope: %s\n", row.Scope)
		if len(row.Agents) == 0 {
			fmt.Fprintln(w, "  Agents: none")
		} else {
			fmt.Fprintf(w, "  Agents: %s\n", strings.Join(row.Agents, ", "))
		}
		if row.Description != "" {
			fmt.Fprintf(w, "  Description: %s\n", row.Description)
		}
		fmt.Fprintln(w)
	}
}

type scanSummaryBucket struct {
	scope    string
	path     string
	agents   []string
	agentKey string
	count    int
}

func scanSummaryLines(rows []ScannedSkillItem) []string {
	scopeCounts := map[string]int{}
	buckets := map[string]*scanSummaryBucket{}
	for _, row := range rows {
		scopeCounts[row.Scope]++
		path := scanSummaryPath(row.DisplayPath)
		agents := []string{}
		if len(row.Agents) > 0 {
			agents = append(agents, row.Agents...)
		}
		agentKey := strings.Join(agents, "\x00")
		key := row.Scope + "\x00" + path + "\x00" + agentKey
		bucket := buckets[key]
		if bucket == nil {
			bucket = &scanSummaryBucket{
				scope:    row.Scope,
				path:     path,
				agents:   agents,
				agentKey: agentKey,
			}
			buckets[key] = bucket
		}
		bucket.count++
	}

	lines := []string{}
	for _, scope := range []string{scopeProject, scopeGlobal} {
		if count := scopeCounts[scope]; count > 0 {
			lines = append(lines, pluralCount(count, scope+" skill"))
		}
	}

	sortedBuckets := make([]scanSummaryBucket, 0, len(buckets))
	for _, bucket := range buckets {
		sortedBuckets = append(sortedBuckets, *bucket)
	}
	sort.Slice(sortedBuckets, func(i, j int) bool {
		if sortedBuckets[i].scope != sortedBuckets[j].scope {
			return sortedBuckets[i].scope > sortedBuckets[j].scope
		}
		if sortedBuckets[i].path != sortedBuckets[j].path {
			return sortedBuckets[i].path < sortedBuckets[j].path
		}
		return sortedBuckets[i].agentKey < sortedBuckets[j].agentKey
	})
	for _, bucket := range sortedBuckets {
		lines = append(lines, fmt.Sprintf("%s in %s (%s)", pluralCount(bucket.count, bucket.scope+" skill"), bucket.path, scanSummaryAgents(bucket.agents)))
	}
	return lines
}

func scanSummaryAgents(agents []string) string {
	if len(agents) == 0 {
		return "unattributed"
	}
	if len(agents) > 3 {
		return "shared by " + pluralCount(len(agents), "agent")
	}
	return strings.Join(agents, ", ")
}

func scanSummaryPath(displayPath string) string {
	if displayPath == "." || displayPath == "" {
		return displayPath
	}
	dir := filepath.ToSlash(filepath.Dir(displayPath))
	if dir == "." && strings.Contains(displayPath, "/") {
		lastSlash := strings.LastIndex(displayPath, "/")
		if lastSlash > 0 {
			return displayPath[:lastSlash]
		}
	}
	return dir
}

func pluralCount(count int, singular string) string {
	if count == 1 {
		return fmt.Sprintf("1 %s", singular)
	}
	return fmt.Sprintf("%d %ss", count, singular)
}

func joinOrNone(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}

func visibleSkillRows(rows []SkillListItem) []SkillListItem {
	if len(rows) <= 10 {
		return rows
	}
	return rows[:10]
}

func skillSearchName(row SkillListItem) string {
	_, slug, err := parseSkillRef(row.Name)
	if err == nil {
		return slug
	}
	return row.Name
}
