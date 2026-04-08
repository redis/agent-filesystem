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
	ansiHideCur = "\033[?25l"
	ansiShowCur = "\033[?25h"
	ansiClearLn = "\033[2K"
)

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
	return utf8.RuneCountInString(stripAnsi(s))
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
		if visible >= maxWidth-1 {
			break
		}
		b.WriteRune(r)
		visible++
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

	fmt.Printf("  %s╭%s╮%s\n", d, strings.Repeat("─", innerWidth), r)
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
