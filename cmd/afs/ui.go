package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	ansiReset   = "\033[0m"
	ansiBold    = "\033[1m"
	ansiDim     = "\033[2m"
	ansiRed     = "\033[31m"
	ansiGreen   = "\033[32m"
	ansiYellow  = "\033[33m"
	ansiCyan    = "\033[36m"
	ansiWhite   = "\033[37m"
	ansiBRed    = "\033[91m"
	ansiBGreen  = "\033[92m"
	ansiGray    = "\033[90m"
	// ansiOrange is the 256-color amber Claude Code uses for actionable
	// text (version numbers, command references, "run this next" hints).
	// We use it for any `afs …` command a box or setup flow is telling the
	// user to run next.
	ansiOrange  = "\033[38;5;208m"
	ansiHideCur = "\033[?25l"
	ansiShowCur = "\033[?25h"
	ansiClearLn = "\033[2K"
)

// markerSuccess is the emoji marker we use at the start of success box
// titles and top-level completion messages. Matches Claude Code's install
// banner.
const markerSuccess = "✅"

var (
	spinFrames = [...]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	colorTerm  bool
)

const (
	bannerIndent = "  "
	bannerWidth  = 40
	maxCLIWidth  = 80
	maxBoxWidth  = maxCLIWidth - len(bannerIndent) - 2
	maxBoxText   = maxBoxWidth - 4
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
	if !colorTerm {
		fmt.Println()
		fmt.Println("  AFS")
		fmt.Println("  Agent Filesystem")
		fmt.Println()
		return
	}

	bar := bannerIndent +
		ansiGray + "░░░░" +
		ansiRed + "▒▒▒▒" +
		ansiBRed + "▓▓▓▓" +
		ansiBold + ansiWhite + "████████████████" + ansiReset +
		ansiBRed + "▓▓▓▓" +
		ansiRed + "▒▒▒▒" +
		ansiGray + "░░░░" + ansiReset

	lines := []string{
		"",
		bar,
		centerBannerText(clr(ansiBold+ansiWhite, "AFS")),
		centerBannerText(clr(ansiDim, "Agent Filesystem")),
		bar,
		"",
	}

	for _, line := range lines {
		fmt.Println(line)
		if line != "" {
			time.Sleep(40 * time.Millisecond)
		}
	}
}

func printBannerCompact() {
	if !colorTerm {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "  AFS")
		fmt.Fprintln(os.Stderr, "  Agent Filesystem")
		fmt.Fprintln(os.Stderr)
		return
	}
	bar := ansiGray + "░░░░" +
		ansiRed + "▒▒▒▒" +
		ansiBRed + "▓▓▓▓" +
		ansiBold + ansiWhite + "████████████████" + ansiReset +
		ansiBRed + "▓▓▓▓" +
		ansiRed + "▒▒▒▒" +
		ansiGray + "░░░░" + ansiReset
	fmt.Fprintf(os.Stderr, "\n%s%s\n%s\n%s\n%s%s\n\n",
		bannerIndent, bar,
		centerBannerTextForOutput(os.Stderr, clr(ansiBold+ansiWhite, "AFS")),
		centerBannerTextForOutput(os.Stderr, clr(ansiDim, "Agent Filesystem")),
		bannerIndent, bar)
}

func centerBannerText(text string) string {
	return centerBannerTextForOutput(os.Stdout, text)
}

func centerBannerTextForOutput(out io.Writer, text string) string {
	padding := (bannerWidth - runeWidth(text)) / 2
	if padding < 0 {
		padding = 0
	}
	return bannerIndent + strings.Repeat(" ", padding) + text
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

	if !colorTerm {
		fmt.Printf("  %s...", label)
		close(s.done)
		return s
	}

	hideCursor()
	go func() {
		defer close(s.done)
		i := 0
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()

		s.mu.Lock()
		lbl := s.label
		s.mu.Unlock()
		fmt.Printf("\r%s  %s%s%s %s", ansiClearLn, ansiYellow, spinFrames[0], ansiReset, lbl)

		for {
			select {
			case <-s.stop:
				return
			case <-ticker.C:
				i++
				s.mu.Lock()
				lbl = s.label
				s.mu.Unlock()
				fmt.Printf("\r%s  %s%s%s %s",
					ansiClearLn, ansiYellow, spinFrames[i%len(spinFrames)], ansiReset, lbl)
			}
		}
	}()
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

	if !colorTerm {
		if detail != "" {
			fmt.Printf(" %s\n", detail)
		} else {
			fmt.Println(" ok")
		}
		return
	}

	s.mu.Lock()
	lbl := s.label
	s.mu.Unlock()

	suffix := ""
	if detail != "" {
		suffix = ansiDim + " · " + ansiReset + detail
	}
	fmt.Printf("\r%s  %s✓%s %s%s\n", ansiClearLn, ansiGreen, ansiReset, lbl, suffix)
	showCursor()
}

func (s *uiStep) fail(detail string) {
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
	<-s.done

	if !colorTerm {
		fmt.Printf(" FAILED: %s\n", detail)
		return
	}

	s.mu.Lock()
	lbl := s.label
	s.mu.Unlock()

	suffix := ""
	if detail != "" {
		suffix = " " + ansiRed + detail + ansiReset
	}
	fmt.Printf("\r%s  %s✗%s %s%s\n", ansiClearLn, ansiRed, ansiReset, lbl, suffix)
	showCursor()
}

// ---------------------------------------------------------------------------
// Box rendering
// ---------------------------------------------------------------------------

type boxRow struct {
	Label string
	Value string
}

func printBox(title string, rows []boxRow) {
	maxLabel := 0
	for _, r := range rows {
		if w := runeWidth(r.Label); w > maxLabel {
			maxLabel = w
		}
	}
	if maxLabel > maxBoxText-3 {
		maxLabel = maxBoxText - 3
	}

	type fmtLine struct {
		content string
		empty   bool
	}
	var lines []fmtLine

	if title != "" {
		lines = append(lines, fmtLine{content: fitDisplayText(title, maxBoxText)})
		lines = append(lines, fmtLine{empty: true})
	}

	for _, r := range rows {
		if r.Label == "" && r.Value == "" {
			lines = append(lines, fmtLine{empty: true})
			continue
		}
		var content string
		if r.Label != "" {
			label := padVisibleText(r.Label, maxLabel)
			valueWidth := maxBoxText - maxLabel - 3
			if valueWidth < 0 {
				valueWidth = 0
			}
			content = fmt.Sprintf("%s   %s",
				clr(ansiDim, label),
				fitDisplayText(r.Value, valueWidth))
		} else {
			content = fitDisplayText(r.Value, maxBoxText)
		}
		lines = append(lines, fmtLine{content: content})
	}

	maxWidth := 0
	for _, l := range lines {
		if w := runeWidth(l.content); w > maxWidth {
			maxWidth = w
		}
	}
	if maxWidth < 36 {
		maxWidth = 36
	}
	innerWidth := maxWidth + 4
	if innerWidth > maxBoxWidth {
		innerWidth = maxBoxWidth
	}

	if !colorTerm {
		fmt.Println()
		fmt.Println("  Redis Agent Filesystem (AFS)")
		fmt.Println()
		for _, l := range lines {
			if l.empty {
				fmt.Println()
			} else {
				fmt.Printf("  %s\n", stripAnsi(l.content))
			}
		}
		fmt.Println()
		return
	}

	d := ansiDim
	r := ansiReset

	// Branded top border, centered: ╭──── ░▒▓█ Redis Agent Filesystem (AFS) █▓▒░ ────╮
	brandText := "Redis Agent Filesystem (AFS)"
	gradient := ansiGray + "░" + ansiRed + "▒" + ansiBRed + "▓" + ansiBold + ansiWhite + "█" + ansiReset
	gradientR := ansiBold + ansiWhite + "█" + ansiReset + ansiBRed + "▓" + ansiRed + "▒" + ansiGray + "░" + ansiReset
	brandLabel := gradient + " " + ansiBold + ansiWhite + brandText + ansiReset + " " + gradientR
	brandVisible := 4 + 1 + len(brandText) + 1 + 4 // ░▒▓█ + space + text + space + █▓▒░
	totalFill := innerWidth - brandVisible - 2       // 2 for the spaces around brand
	if totalFill < 2 {
		totalFill = 2
	}
	leftFill := totalFill / 2
	rightFill := totalFill - leftFill
	fmt.Printf("  %s╭%s %s%s %s%s╮%s\n", d, strings.Repeat("─", leftFill), ansiReset, brandLabel, ansiDim, strings.Repeat("─", rightFill), r)
	fmt.Printf("  %s│%s%s%s│%s\n", d, r, strings.Repeat(" ", innerWidth), d, r)

	for _, l := range lines {
		if l.empty {
			fmt.Printf("  %s│%s%s%s│%s\n", d, r, strings.Repeat(" ", innerWidth), d, r)
		} else {
			rightPad := innerWidth - 2 - runeWidth(l.content)
			if rightPad < 2 {
				rightPad = 2
			}
			fmt.Printf("  %s│%s  %s%s%s│%s\n",
				d, r, l.content, strings.Repeat(" ", rightPad), d, r)
		}
	}

	fmt.Printf("  %s│%s%s%s│%s\n", d, r, strings.Repeat(" ", innerWidth), d, r)
	fmt.Printf("  %s╰%s╯%s\n", d, strings.Repeat("─", innerWidth), r)
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
