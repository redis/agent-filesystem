package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/redis/agent-filesystem/internal/version"
)

const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiRed    = "\033[31m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiCyan   = "\033[36m"
	ansiWhite  = "\033[37m"
	ansiBRed   = "\033[91m"
	ansiBGreen = "\033[92m"
	ansiGray   = "\033[90m"
	// ansiOrange is the 256-color amber Claude Code uses for actionable
	// text (version numbers, command references, "run this next" hints).
	// We use it for any `afs ...` command a setup flow is telling the user
	// to run next.
	ansiOrange  = "\033[38;5;208m"
	ansiHideCur = "\033[?25l"
	ansiShowCur = "\033[?25h"
	ansiClearLn = "\033[2K"
	// ansiLabel uses bold cyan for row labels — clear on any background.
	ansiLabel = "\033[36m"
)

// markerSuccess is the emoji marker older call sites still pass at the start
// of success titles. Plain output strips it before rendering.
const markerSuccess = "✅"

var (
	spinFrames = [...]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	colorTerm  bool
)

const (
	bannerIndent = "  "
	maxCLIWidth  = 80
)

func init() {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return
	}
	colorTerm = fi.Mode()&os.ModeCharDevice != 0
}

func hideCursor() {
	if colorTerm {
		fmt.Print(ansiHideCur)
	}
}

func showCursor() {
	if colorTerm {
		fmt.Print(ansiShowCur)
	}
}

func clr(code, text string) string {
	if !colorTerm {
		return text
	}
	return code + text + ansiReset
}

func stripAnsi(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\033' && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && !((s[j] >= 'A' && s[j] <= 'Z') || (s[j] >= 'a' && s[j] <= 'z')) {
				j++
			}
			if j < len(s) {
				j++
			}
			i = j
		} else {
			b.WriteByte(s[i])
			i++
		}
	}
	return b.String()
}

func runeWidth(s string) int {
	stripped := stripAnsi(s)
	n := 0
	for _, r := range stripped {
		n += cellWidth(r)
	}
	return n
}

// cellWidth returns the terminal cell width of a single rune. We only need
// to distinguish single-width (1) from double-width (2) characters because
// combining marks and zero-width glyphs don't appear in our UI strings.
// Emoji fall into specific Unicode ranges — we check the handful we actually
// use rather than pulling in a full runewidth table dependency.
func cellWidth(r rune) int {
	switch {
	case r == '✅', r == '❌', r == '⏸':
		return 2
	// Miscellaneous Symbols and Pictographs + Supplemental Symbols — the
	// blocks that hold most common emoji. Catches future emoji we add to
	// status output without touching this function.
	case r >= 0x1F300 && r <= 0x1FAFF:
		return 2
	case r >= 0x2600 && r <= 0x27BF && r != '✓' && r != '✗':
		// Dingbats / misc symbols. ✓ (U+2713) and ✗ (U+2717) render single-
		// width in most monospaced fonts and we rely on that in step output.
		return 2
	}
	return 1
}

func fitDisplayText(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if runeWidth(text) <= maxWidth {
		return text
	}
	if maxWidth == 1 {
		return "…"
	}

	var b strings.Builder
	visible := 0
	hasAnsi := false

	for i := 0; i < len(text); {
		if text[i] == '\033' && i+1 < len(text) && text[i+1] == '[' {
			hasAnsi = true
			j := i + 2
			for j < len(text) && !((text[j] >= 'A' && text[j] <= 'Z') || (text[j] >= 'a' && text[j] <= 'z')) {
				j++
			}
			if j < len(text) {
				j++
			}
			b.WriteString(text[i:j])
			i = j
			continue
		}

		r, size := utf8.DecodeRuneInString(text[i:])
		w := cellWidth(r)
		if visible+w > maxWidth-1 {
			break
		}
		b.WriteRune(r)
		visible += w
		i += size
	}

	b.WriteRune('…')
	if hasAnsi {
		b.WriteString(ansiReset)
	}
	return b.String()
}

func padVisibleText(text string, width int) string {
	padding := width - runeWidth(text)
	if padding <= 0 {
		return text
	}
	return text + strings.Repeat(" ", padding)
}

// ---------------------------------------------------------------------------
// Banner
// ---------------------------------------------------------------------------

func printBanner() {
	printBrandHeader(os.Stdout)
}

func printBannerCompact() {
	printBrandHeader(os.Stderr)
}

// ---------------------------------------------------------------------------
// Step spinner
// ---------------------------------------------------------------------------

type uiStep struct {
	mu    sync.Mutex
	label string
	start time.Time
	stop  chan struct{}
	done  chan struct{}
}

func startStep(label string) *uiStep {
	s := &uiStep{
		label: label,
		start: time.Now(),
		stop:  make(chan struct{}),
		done:  make(chan struct{}),
	}
	close(s.done)
	return s
}

func (s *uiStep) update(label string) {
	s.mu.Lock()
	s.label = label
	s.mu.Unlock()
}

func (s *uiStep) elapsed() time.Duration {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.start.IsZero() {
		return 0
	}
	return time.Since(s.start)
}

func (s *uiStep) succeed(detail string) {
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
	<-s.done

	s.mu.Lock()
	lbl := s.label
	s.mu.Unlock()

	lbl = stripAnsi(strings.TrimSpace(lbl))
	detail = stripAnsi(strings.TrimSpace(detail))
	if detail != "" {
		fmt.Printf("%s: %s\n", lbl, detail)
	} else {
		fmt.Printf("%s: ok\n", lbl)
	}
}

func (s *uiStep) fail(detail string) {
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
	<-s.done

	s.mu.Lock()
	lbl := s.label
	s.mu.Unlock()

	lbl = stripAnsi(strings.TrimSpace(lbl))
	detail = stripAnsi(strings.TrimSpace(detail))
	if detail != "" {
		fmt.Printf("%s: failed: %s\n", lbl, detail)
	} else {
		fmt.Printf("%s: failed\n", lbl)
	}
}

// ---------------------------------------------------------------------------
// Plain output rendering
// ---------------------------------------------------------------------------

type outputRow struct {
	Label      string
	Value      string
	NoTruncate bool
}

func printSection(title string, rows []outputRow) {
	fmt.Printf("\n")

	title = plainOutputTitle(title)
	if title != "" {
		fmt.Println(title)
	}

	fmt.Printf("\n")

	maxLabel := 0
	for _, r := range rows {
		if w := runeWidth(stripAnsi(r.Label)); w > maxLabel {
			maxLabel = w
		}
	}

	for _, r := range rows {
		if r.Label == "" && r.Value == "" {
			fmt.Println()
			continue
		}
		label := stripAnsi(strings.TrimSpace(r.Label))
		value := stripAnsi(strings.TrimSpace(r.Value))
		if label == "" {
			fmt.Println(fitDisplayText(value, maxCLIWidth))
		} else if value == "" {
			fmt.Println(label)
		} else {
			valueWidth := maxCLIWidth - maxLabel - 2
			if valueWidth < 1 {
				valueWidth = 1
			}
			if !r.NoTruncate {
				value = fitDisplayText(value, valueWidth)
			}
			fmt.Printf("%s  %s\n", padVisibleText(label, maxLabel), value)
		}
	}
	fmt.Printf("\n")
}

func printPlainTable(headers []string, rows [][]string) {
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = runeWidth(header)
	}
	for _, row := range rows {
		for i := range headers {
			if i >= len(row) {
				continue
			}
			if width := runeWidth(row[i]); width > widths[i] {
				widths[i] = width
			}
		}
	}
	printPlainTableRow(headers, widths)
	for _, row := range rows {
		printPlainTableRow(row, widths)
	}
}

func printPlainTableRow(cols []string, widths []int) {
	used := 0
	for i, width := range widths {
		value := ""
		if i < len(cols) {
			value = stripAnsi(strings.TrimSpace(cols[i]))
		}
		if i > 0 {
			fmt.Print("  ")
			used += 2
		}
		if i == len(widths)-1 {
			remaining := maxCLIWidth - used
			if remaining < 1 {
				remaining = 1
			}
			value = fitDisplayText(value, remaining)
			fmt.Print(value)
			used += runeWidth(value)
			continue
		}
		value = padVisibleText(value, width)
		fmt.Print(value)
		used += runeWidth(value)
	}
	fmt.Println()
}

func plainOutputTitle(title string) string {
	title = stripAnsi(strings.TrimSpace(title))
	title = strings.TrimPrefix(title, markerSuccess)
	title = strings.TrimSpace(title)
	for _, prefix := range []string{"✓", "○", "■", "●"} {
		title = strings.TrimPrefix(title, prefix)
		title = strings.TrimSpace(title)
	}
	return title
}

// brandHeaderLabel returns the colorized brand string used by help/setup
// surfaces, plus its visible width (runes, no ANSI).
func brandHeaderLabel() (string, int) {
	brandText := "Redis Agent Filesystem (AFS)"
	versionSuffix := strings.TrimSpace(version.Short())
	versionPart := ""
	if versionSuffix != "" {
		versionPart = " " + ansiDim + versionSuffix + ansiReset
	}
	label := ansiBold + brandText + ansiReset + versionPart
	textWidth := utf8.RuneCountInString(brandText)
	if versionSuffix != "" {
		textWidth += 1 + utf8.RuneCountInString(versionSuffix)
	}
	visible := textWidth
	return label, visible
}

// printBrandHeader emits the brand line for guided setup and help surfaces.
func printBrandHeader(w io.Writer) {
	fmt.Fprint(w, brandHeaderString())
}

// brandHeaderString returns the same brand line as printBrandHeader, suitable
// for prepending to multi-line strings (e.g. usage text) so callers can
// embed it in fmt.Sprintf templates.
func brandHeaderString() string {
	if !colorTerm {
		return fmt.Sprintf("\nRedis Agent Filesystem (AFS) %s\n\n", version.Short())
	}
	label, _ := brandHeaderLabel()
	return fmt.Sprintf("\n%s\n\n", label)
}

// ---------------------------------------------------------------------------
// Status helpers
// ---------------------------------------------------------------------------

func formatDuration(d time.Duration) string {
	d = d.Truncate(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}

func formatDisplayTimestamp(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return raw
	}
	return parsed.Local().Format("Jan 2, 2006 3:04 PM")
}

func pidStatusColored(pid int) string {
	if pid <= 0 {
		return "unknown"
	}
	if processAlive(pid) {
		return fmt.Sprintf("%d %s", pid, clr(ansiGreen, "(running)"))
	}
	return fmt.Sprintf("%d %s", pid, clr(ansiRed, "(stopped)"))
}
