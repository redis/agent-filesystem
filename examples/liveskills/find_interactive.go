package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

var errInteractiveInputUnavailable = errors.New("interactive input unavailable")

func canRunInteractiveFind(stdin *os.File, stdout io.Writer) bool {
	tty, cleanup, ok := interactiveFindTTY(stdin, stdout)
	if cleanup != nil {
		cleanup()
	}
	if tty == nil {
		return false
	}
	return ok
}

func interactiveFindTTY(stdin *os.File, stdout io.Writer) (*os.File, func(), bool) {
	out, ok := stdout.(*os.File)
	if !ok || !isTerminalFile(out) {
		return nil, nil, false
	}
	if stdin != nil && isTerminalFile(stdin) {
		return stdin, func() {}, true
	}
	if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil && isTerminalFile(tty) {
		return tty, func() { _ = tty.Close() }, true
	}
	return nil, nil, false
}

func runInteractiveFind(stdin *os.File, stdout io.Writer, rows []SkillListItem, initialQuery string, style outputStyle) (SkillListItem, bool, error) {
	restore, err := enableRawInput(stdin)
	if err != nil {
		return SkillListItem{}, false, err
	}
	defer restore()

	query := initialQuery
	selected := 0
	lastLines := 0
	render := func() []SkillListItem {
		matches := filterSkillRows(rows, query)
		visible := visibleSkillRows(matches)
		if selected >= len(visible) {
			selected = len(visible) - 1
		}
		if selected < 0 {
			selected = 0
		}
		lastLines = renderFindPrompt(stdout, query, matches, selected, lastLines, style)
		return matches
	}

	buffer := make([]byte, 1)
	fmt.Fprint(stdout, "\x1b[?25l")
	defer fmt.Fprint(stdout, "\x1b[?25h")

	matches := render()
	for {
		var input byte
		n, readErr := stdin.Read(buffer)
		if readErr != nil {
			if readErr == io.EOF {
				continue
			}
			return SkillListItem{}, false, readErr
		}
		if n == 0 {
			continue
		}
		input = buffer[0]
		switch input {
		case 3, 4, 27:
			if input == 27 {
				if handled := readEscapeSequence(stdin, &selected, len(visibleSkillRows(matches))); handled {
					matches = render()
					continue
				}
			}
			fmt.Fprint(stdout, "\r\n")
			return SkillListItem{}, false, nil
		case '\r', '\n':
			fmt.Fprint(stdout, "\r\n")
			selectable := visibleSkillRows(matches)
			if len(selectable) == 0 {
				return SkillListItem{}, false, nil
			}
			return selectable[selected], true, nil
		case 127, 8:
			if query != "" {
				query = query[:len(query)-1]
				selected = 0
				matches = render()
			}
		default:
			if input >= 32 && input <= 126 {
				query += string(input)
				selected = 0
				matches = render()
			}
		}
	}
}

func enableRawInput(stdin *os.File) (func(), error) {
	oldState, err := stty(stdin, "-g")
	if err != nil {
		return nil, err
	}
	if _, err := stty(stdin, "raw", "-echo", "min", "0", "time", "1"); err != nil {
		return nil, err
	}
	return func() {
		_, _ = stty(stdin, strings.TrimSpace(oldState))
	}, nil
}

func stty(stdin *os.File, args ...string) (string, error) {
	cmd := exec.Command("stty", args...)
	cmd.Stdin = stdin
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fail("stty %s failed: %s", strings.Join(args, " "), strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func readEscapeSequence(stdin *os.File, selected *int, count int) bool {
	sequence := make([]byte, 2)
	read := 0
	for read < len(sequence) {
		n, err := stdin.Read(sequence[read:])
		if err != nil || n == 0 {
			break
		}
		read += n
	}
	if read != 2 || sequence[0] != '[' {
		return false
	}
	switch sequence[1] {
	case 'A':
		if *selected > 0 {
			*selected--
		}
	case 'B':
		if *selected+1 < count {
			*selected++
		}
	default:
		return false
	}
	return true
}

func renderFindPrompt(w io.Writer, query string, rows []SkillListItem, selected int, lastLines int, style outputStyle) int {
	if lastLines > 0 {
		fmt.Fprintf(w, "\x1b[%dA\x1b[1G\x1b[J", lastLines)
	}
	lines := findPromptLines(query, rows, selected, style)
	for _, line := range lines {
		fmt.Fprint(w, line+"\r\n")
	}
	return len(lines)
}

func findPromptLines(query string, rows []SkillListItem, selected int, style outputStyle) []string {
	cursor := style.bold("_")
	lines := []string{fmt.Sprintf("%s %s%s", style.text("Search skills:"), query, cursor), ""}
	if len(query) < 2 {
		lines = append(lines, style.dim("Start typing to search (min 2 chars)"))
	} else if len(rows) == 0 {
		lines = append(lines, style.dim("No skills found"))
	} else {
		for index, row := range visibleSkillRows(rows) {
			pointer := " "
			name := style.text(skillSearchName(row))
			if index == selected {
				pointer = ">"
				name = style.bold(skillSearchName(row))
			}
			line := fmt.Sprintf("  %s %s %s", pointer, name, style.dim(row.Name))
			if row.Version != "" && row.Version != "-" {
				line += " " + style.cyan(row.Version)
			}
			lines = append(lines, line)
		}
	}
	lines = append(lines, "", style.dim("up/down navigate | enter select | esc cancel"))
	return lines
}

func filterSkillRows(rows []SkillListItem, query string) []SkillListItem {
	query = strings.ToLower(strings.TrimSpace(query))
	if len(query) < 2 {
		return nil
	}
	filtered := []SkillListItem{}
	for _, row := range rows {
		if strings.Contains(strings.ToLower(row.Name), query) ||
			strings.Contains(strings.ToLower(row.DisplayName), query) ||
			strings.Contains(strings.ToLower(row.Description), query) {
			filtered = append(filtered, row)
		}
	}
	return filtered
}
