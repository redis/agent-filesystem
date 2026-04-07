package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const afsImportReviewLargestLimit = 5

type afsImportExclusions struct {
	files map[string]struct{}
	dirs  map[string]struct{}
}

type afsImportReviewChoice struct {
	ID   string
	Item importScanEntry
}

func newAFSImportExclusions() afsImportExclusions {
	return afsImportExclusions{
		files: make(map[string]struct{}),
		dirs:  make(map[string]struct{}),
	}
}

func (e afsImportExclusions) empty() bool {
	return len(e.files) == 0 && len(e.dirs) == 0
}

func (e afsImportExclusions) count() int {
	return len(e.files) + len(e.dirs)
}

func (e afsImportExclusions) preview(limit int) string {
	if e.empty() {
		return clr(ansiDim, "none")
	}

	items := make([]string, 0, e.count())
	for dir := range e.dirs {
		items = append(items, dir+"/")
	}
	for file := range e.files {
		items = append(items, file)
	}
	sort.Strings(items)

	if limit > 0 && len(items) > limit {
		hidden := len(items) - limit
		return strings.Join(items[:limit], ", ") + fmt.Sprintf(" +%d more", hidden)
	}
	return strings.Join(items, ", ")
}

func (e *afsImportExclusions) clear() bool {
	if e == nil || e.empty() {
		return false
	}
	e.files = make(map[string]struct{})
	e.dirs = make(map[string]struct{})
	return true
}

func (e *afsImportExclusions) add(item importScanEntry) bool {
	if e == nil {
		return false
	}
	if e.files == nil {
		e.files = make(map[string]struct{})
	}
	if e.dirs == nil {
		e.dirs = make(map[string]struct{})
	}

	switch item.Kind {
	case "dir":
		if e.containsPath(item.Path) {
			return false
		}
		for dir := range e.dirs {
			if dir == item.Path || strings.HasPrefix(dir, item.Path+"/") {
				delete(e.dirs, dir)
			}
		}
		for file := range e.files {
			if file == item.Path || strings.HasPrefix(file, item.Path+"/") {
				delete(e.files, file)
			}
		}
		e.dirs[item.Path] = struct{}{}
		return true
	default:
		if e.containsPath(item.Path) {
			return false
		}
		e.files[item.Path] = struct{}{}
		return true
	}
}

func (e afsImportExclusions) containsPath(path string) bool {
	if _, ok := e.files[path]; ok {
		return true
	}
	for dir := range e.dirs {
		if path == dir || strings.HasPrefix(path, dir+"/") {
			return true
		}
	}
	return false
}

func applyAFSImportExclusions(base *migrationIgnore, exclusions afsImportExclusions) *migrationIgnore {
	if base == nil && exclusions.empty() {
		return nil
	}

	overlay := &migrationIgnore{}
	if base != nil {
		*overlay = *base
	}
	overlay.tempFiles = copyImportSelectionSet(exclusions.files)
	overlay.tempDirs = copyImportSelectionSet(exclusions.dirs)
	return overlay
}

func copyImportSelectionSet(src map[string]struct{}) map[string]struct{} {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]struct{}, len(src))
	for key := range src {
		dst[key] = struct{}{}
	}
	return dst
}

func hasAFSImportReviewCandidates(scan importScanReport) bool {
	return len(scan.LargestFiles) > 0 || len(scan.LargestDirs) > 0
}

func reviewAFSImportExclusions(r *bufio.Reader, out io.Writer, sourceDir string, scan importScanReport, exclusions *afsImportExclusions) (bool, error) {
	choices, rows := buildAFSImportReviewChoices(scan)
	if len(choices) == 0 {
		return false, nil
	}

	for {
		printBox(clr(ansiBold, "Largest items"), rows)
		fmt.Fprintln(out, "  Enter numbers or relative paths to exclude for this import.")
		fmt.Fprintln(out, "  Use commas for multiple entries, `reset` to clear temporary exclusions, or press Enter to go back.")

		input, err := promptString(r, out, "  Exclude", "")
		if err != nil {
			return false, err
		}
		input = strings.TrimSpace(input)
		if input == "" {
			return false, nil
		}
		if strings.EqualFold(input, "reset") {
			if exclusions.clear() {
				fmt.Fprintln(out)
				fmt.Fprintln(out, "  Cleared temporary exclusions.")
				fmt.Fprintln(out)
				return true, nil
			}
			fmt.Fprintln(out)
			fmt.Fprintln(out, "  No temporary exclusions to clear.")
			fmt.Fprintln(out)
			continue
		}

		added, err := applyAFSImportReviewInput(sourceDir, input, choices, exclusions)
		if err != nil {
			fmt.Fprintln(out)
			fmt.Fprintf(out, "  %s\n\n", err)
			continue
		}

		fmt.Fprintln(out)
		if added == 1 {
			fmt.Fprintln(out, "  Added 1 temporary exclusion. Re-scanning import plan...")
		} else {
			fmt.Fprintf(out, "  Added %d temporary exclusions. Re-scanning import plan...\n", added)
		}
		fmt.Fprintln(out)
		return true, nil
	}
}

func buildAFSImportReviewChoices(scan importScanReport) ([]afsImportReviewChoice, []boxRow) {
	choices := make([]afsImportReviewChoice, 0, len(scan.LargestFiles)+len(scan.LargestDirs))
	rows := make([]boxRow, 0, len(scan.LargestFiles)+len(scan.LargestDirs)+4)
	nextID := 1

	if len(scan.LargestFiles) > 0 {
		rows = append(rows, boxRow{Value: clr(ansiBold, "Largest files")})
		for _, item := range scan.LargestFiles {
			choice := afsImportReviewChoice{
				ID:   strconv.Itoa(nextID),
				Item: item,
			}
			choices = append(choices, choice)
			rows = append(rows, boxRow{
				Label: choice.ID,
				Value: formatAFSImportReviewChoice(choice),
			})
			nextID++
		}
	}

	if len(scan.LargestDirs) > 0 {
		if len(rows) > 0 {
			rows = append(rows, boxRow{})
		}
		rows = append(rows, boxRow{Value: clr(ansiBold, "Largest directories")})
		for _, item := range scan.LargestDirs {
			choice := afsImportReviewChoice{
				ID:   strconv.Itoa(nextID),
				Item: item,
			}
			choices = append(choices, choice)
			rows = append(rows, boxRow{
				Label: choice.ID,
				Value: formatAFSImportReviewChoice(choice),
			})
			nextID++
		}
	}

	return choices, rows
}

func formatAFSImportReviewChoice(choice afsImportReviewChoice) string {
	if choice.Item.Kind == "dir" {
		return fmt.Sprintf("%s · dir · %d files · %s/", formatBytes(choice.Item.Bytes), choice.Item.Files, choice.Item.Path)
	}
	return fmt.Sprintf("%s · file · %s", formatBytes(choice.Item.Bytes), choice.Item.Path)
}

func applyAFSImportReviewInput(sourceDir, input string, choices []afsImportReviewChoice, exclusions *afsImportExclusions) (int, error) {
	choiceMap := make(map[string]importScanEntry, len(choices))
	for _, choice := range choices {
		choiceMap[choice.ID] = choice.Item
	}

	var added int
	for _, token := range splitAFSImportReviewInput(input) {
		item, err := resolveAFSImportReviewToken(sourceDir, token, choiceMap)
		if err != nil {
			return 0, err
		}
		if exclusions.add(item) {
			added++
		}
	}
	if added == 0 {
		return 0, fmt.Errorf("those selections are already excluded for this import")
	}
	return added, nil
}

func splitAFSImportReviewInput(input string) []string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil
	}
	if strings.Contains(trimmed, ",") {
		raw := strings.Split(trimmed, ",")
		out := make([]string, 0, len(raw))
		for _, token := range raw {
			token = strings.TrimSpace(token)
			if token != "" {
				out = append(out, token)
			}
		}
		return out
	}

	fields := strings.Fields(trimmed)
	if len(fields) > 1 {
		numeric := true
		for _, field := range fields {
			if _, err := strconv.Atoi(field); err != nil {
				numeric = false
				break
			}
		}
		if numeric {
			return fields
		}
	}

	return []string{trimmed}
}

func resolveAFSImportReviewToken(sourceDir, token string, choiceMap map[string]importScanEntry) (importScanEntry, error) {
	token = strings.Trim(strings.TrimSpace(token), `"'`)
	if token == "" {
		return importScanEntry{}, fmt.Errorf("choose a numbered item or a path inside %s", sourceDir)
	}
	if item, ok := choiceMap[token]; ok {
		return item, nil
	}

	trimmed := strings.TrimRight(token, "/")
	if trimmed == "" || trimmed == "." {
		return importScanEntry{}, fmt.Errorf("choose a file or directory inside %s", sourceDir)
	}

	fullPath := trimmed
	if !filepath.IsAbs(fullPath) {
		fullPath = filepath.Join(sourceDir, filepath.FromSlash(trimmed))
	}
	fullPath = filepath.Clean(fullPath)

	rel, err := filepath.Rel(sourceDir, fullPath)
	if err != nil {
		return importScanEntry{}, err
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return importScanEntry{}, fmt.Errorf("%s is outside the import root", token)
	}

	info, err := os.Lstat(fullPath)
	if err != nil {
		return importScanEntry{}, err
	}

	item := importScanEntry{
		Path: filepath.ToSlash(rel),
	}
	if info.IsDir() {
		item.Kind = "dir"
	} else {
		item.Kind = "file"
		item.Bytes = info.Size()
		item.Files = 1
	}
	return item, nil
}
