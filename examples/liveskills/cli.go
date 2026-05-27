package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

func runCLI(argv []string, cwd string, env map[string]string, stdout io.Writer, stderr io.Writer) int {
	return runCLIWithInput(argv, cwd, env, os.Stdin, stdout, stderr)
}

func runCLIWithInput(argv []string, cwd string, env map[string]string, stdin *os.File, stdout io.Writer, stderr io.Writer) int {
	parsed := parseArgs(argv)
	command := parsed.Pos(0)
	app := NewApp(cwd, env)
	style := styleForOutput(stdout, env)

	if command == "" || command == "help" || command == "--help" || parsed.Bool("help") {
		fmt.Fprint(stdout, helpText())
		return 0
	}

	if command == "auth" && parsed.Pos(1) == "login" {
		result, err := app.AuthLogin(parsed.Flag("endpoint"), parsed.Flag("token"))
		return writeResult(err, parsed.Bool("json"), result, stdout, stderr, func() {
			printRows(stdout, "Logged in", [][2]string{{"Endpoint", result["endpoint"]}})
		})
	}

	if command == "publish" {
		result, err := app.Publish(parsed.Pos(1), map[string]string{
			"skill":      parsed.Flag("skill"),
			"slug":       parsed.Flag("name"),
			"owner":      parsed.Flag("owner"),
			"version":    parsed.Flag("version"),
			"visibility": parsed.Flag("visibility"),
		})
		if err != nil {
			return writeError(err, stderr)
		}
		payload := map[string]any{
			"name":       result.Skill.Owner + "/" + result.Skill.Slug,
			"version":    result.Version.Version,
			"volume":     result.Skill.CanonicalVolumeID,
			"checkpoint": result.Version.CheckpointID,
			"scripts":    result.Scripts,
		}
		if parsed.Bool("json") {
			if err := printJSON(stdout, payload); err != nil {
				return writeError(err, stderr)
			}
		} else {
			printRows(stdout, "Skill published", [][2]string{
				{"Name", payload["name"].(string)},
				{"Version", payload["version"].(string)},
				{"Volume", payload["volume"].(string)},
				{"Checkpoint", payload["checkpoint"].(string)},
				{"Scripts", joinOrNone(result.Scripts)},
				{"Install", "liveskills add " + payload["name"].(string)},
			})
		}
		return 0
	}

	if command == "find" {
		query := parsed.Flag("query")
		if query == "" {
			query = parsed.Pos(1)
		}
		searchQuery := query
		if parsed.Bool("interactive") {
			searchQuery = ""
		}
		result, err := app.List(searchQuery, false)
		if err != nil {
			return writeError(err, stderr)
		}
		if parsed.Bool("json") {
			if err := printJSON(stdout, result); err != nil {
				return writeError(err, stderr)
			}
			return 0
		}
		if parsed.Bool("interactive") {
			tty, cleanup, ok := interactiveFindTTY(stdin, stdout)
			if !ok {
				return writeError(fail("Interactive find requires a terminal. Run `liveskills find [query]` to print results."), stderr)
			}
			defer cleanup()
			selected, ok, err := runInteractiveFind(tty, stdout, result, query, style)
			if err != nil {
				if errors.Is(err, errInteractiveInputUnavailable) {
					printAvailableSkillList(stdout, result, query, style)
					return 0
				}
				return writeError(err, stderr)
			}
			if !ok {
				fmt.Fprintln(stdout, style.dim("Search cancelled"))
				return 0
			}
			fmt.Fprintln(stdout)
			fmt.Fprintf(stdout, "Installing %s from %s...\n\n", style.bold(skillSearchName(selected)), style.dim(selected.Name))
			installResult, err := app.Add(selected.Name, installOptions(parsed))
			return writeResult(err, false, installResult, stdout, stderr, func() {
				title := "Skill added"
				if installResult.Status == "unchanged" {
					title = "Skill already added"
				}
				printRows(stdout, title, installationRows(installResult))
			})
		}
		printAvailableSkillList(stdout, result, query, style)
		return 0
	}

	if command == "list" || command == "ls" {
		if parsed.Bool("all") {
			return writeError(fail("Use `liveskills find` to show available skills. `liveskills list` shows installed skills."), stderr)
		}
		result, err := app.ListInstalled(parsed.Bool("global"))
		return writeResult(err, parsed.Bool("json"), result, stdout, stderr, func() {
			printInstalledSkillList(stdout, result, parsed.Bool("global"), style)
		})
	}

	if command == "scan" {
		result, err := app.Scan(scanOptions(parsed))
		return writeResult(err, parsed.Bool("json"), result, stdout, stderr, func() {
			printScannedSkillList(stdout, result)
		})
	}

	if command == "show" {
		result, err := app.Show(parsed.Pos(1))
		return writeResult(err, parsed.Bool("json"), result, stdout, stderr, func() {
			printRows(stdout, "Skill", [][2]string{
				{"Name", result.Name},
				{"Version", result.Version},
				{"Volume", result.Volume},
				{"Description", result.Description},
			})
		})
	}

	if command == "download" {
		result, err := app.Download(parsed.Pos(1), parsed.Flag("version"), parsed.Flag("output"))
		return writeResult(err, parsed.Bool("json"), result, stdout, stderr, func() {
			printRows(stdout, "Skill downloaded", [][2]string{
				{"Name", result.Name},
				{"Version", result.Version},
				{"Output", result.Output},
			})
		})
	}

	if command == "add" {
		if parsed.Bool("list") {
			result, err := app.ListSource(parsed.Pos(1))
			return writeResult(err, parsed.Bool("json"), result, stdout, stderr, func() {
				printSourceSkillList(stdout, result)
			})
		}
		if err := validateAddSourceBeforePrompt(app.CWD, parsed.Pos(1)); err != nil {
			return writeError(err, stderr)
		}
		if !parsed.Bool("json") {
			printSecurityAssessment(stdout, parsed.Pos(1))
		}
		if !parsed.Bool("yes") && shouldPromptForConfirmation(stdin, stdout) {
			ok, err := confirmInstall(stdin, stdout)
			if err != nil {
				return writeError(err, stderr)
			}
			if !ok {
				fmt.Fprintln(stdout, "Installation cancelled")
				return 0
			}
		}
		results, err := app.AddMany(parsed.Pos(1), installOptions(parsed))
		if err != nil {
			return writeError(err, stderr)
		}
		if parsed.Bool("json") {
			if len(results) == 1 {
				if err := printJSON(stdout, results[0]); err != nil {
					return writeError(err, stderr)
				}
				return 0
			}
			if err := printJSON(stdout, results); err != nil {
				return writeError(err, stderr)
			}
			return 0
		}
		printInstallResults(stdout, results)
		return 0
	}

	if command == "update" {
		result, err := app.Update(parsed.Pos(1), installOptions(parsed))
		return writeResult(err, parsed.Bool("json"), result, stdout, stderr, func() {
			printRows(stdout, "Skill updated", installationRows(result))
		})
	}

	if command == "remove" || command == "rm" {
		result, err := app.Remove(parsed.Pos(1), installOptions(parsed))
		return writeResult(err, parsed.Bool("json"), result, stdout, stderr, func() {
			printRows(stdout, "Skill removed", [][2]string{
				{"Skill", result.Name},
				{"Workspace", result.Workspace},
				{"Removed", result.Path},
			})
		})
	}

	return writeError(fail("Unknown command: %s", command), stderr)
}

func validateAddSourceBeforePrompt(cwd string, source string) error {
	if source == "" {
		return fail("Usage: liveskills add <source-or-ref>")
	}
	sourcePath, _ := splitInlineSkillRef(source)
	if !looksLikeLocalPathLiteral(sourcePath) || looksLikeRemoteSource(sourcePath) {
		return nil
	}
	if _, err := os.Stat(resolvePath(cwd, sourcePath)); err != nil {
		return fail("Skill source not found: %s", sourcePath)
	}
	return nil
}

func looksLikeLocalPathLiteral(input string) bool {
	return strings.HasPrefix(input, ".") || strings.HasPrefix(input, "/") || strings.HasPrefix(input, "~")
}

func installOptions(parsed ParsedArgs) map[string]string {
	return map[string]string{
		"workspace":  parsed.Flag("workspace"),
		"agent":      parsed.Flag("agent"),
		"agents":     strings.Join(parsed.Values("agent"), "\n"),
		"mount":      parsed.Flag("mount"),
		"version":    parsed.Flag("version"),
		"skill":      parsed.Flag("skill"),
		"skills":     strings.Join(parsed.Values("skill"), "\n"),
		"name":       parsed.Flag("name"),
		"owner":      parsed.Flag("owner"),
		"visibility": parsed.Flag("visibility"),
		"copy":       boolString(parsed.Bool("copy")),
		"all":        boolString(parsed.Bool("all")),
		"yes":        boolString(parsed.Bool("yes")),
		"global":     boolString(parsed.Bool("global")),
	}
}

func installationRows(result *InstallResult) [][2]string {
	return [][2]string{
		{"Skill", result.Name},
		{"Version", result.Version},
		{"Scope", result.Scope},
		{"Workspace", result.Workspace},
		{"Path", result.Path},
		{"Canonical", homeRelative(defaultString(result.CanonicalPath, result.MountPoint))},
		{"Targets", joinInstallTargets(result.Targets)},
		{"List with", result.ListCommand},
	}
}

func printInstallResults(stdout io.Writer, results []InstallResult) {
	if len(results) == 0 {
		fmt.Fprintln(stdout, "No skills installed")
		return
	}
	if len(results) == 1 {
		result := results[0]
		title := "Skill added"
		if result.Status == "unchanged" {
			title = "Skill already added"
		}
		printRows(stdout, title, installationRows(&result))
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Review skills before use; they run with full agent permissions.")
		return
	}
	fmt.Fprintf(stdout, "Installed %d skills\n", len(results))
	for _, result := range results {
		fmt.Fprintf(stdout, "- %s %s\n", result.Name, result.Version)
		fmt.Fprintf(stdout, "  Canonical: %s\n", homeRelative(defaultString(result.CanonicalPath, result.MountPoint)))
		fmt.Fprintf(stdout, "  Targets: %s\n", joinInstallTargets(result.Targets))
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Review skills before use; they run with full agent permissions.")
}

func printSecurityAssessment(stdout io.Writer, source string) {
	fmt.Fprintln(stdout, "Security Risk Assessments")
	fmt.Fprintf(stdout, "  %s Not assessed locally\n", defaultString(source, "source"))
	fmt.Fprintln(stdout, "  Review skill contents before use; skills run with full agent permissions.")
	fmt.Fprintln(stdout)
}

func shouldPromptForConfirmation(stdin *os.File, stdout io.Writer) bool {
	if stdin == nil || !isTerminalFile(stdin) {
		return false
	}
	file, ok := stdout.(*os.File)
	return ok && isTerminalFile(file)
}

func confirmInstall(stdin *os.File, stdout io.Writer) (bool, error) {
	fmt.Fprint(stdout, "Proceed with installation? [Y/n] ")
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "" || answer == "y" || answer == "yes", nil
}

func writeResult(err error, asJSON bool, result any, stdout io.Writer, stderr io.Writer, render func()) int {
	if err != nil {
		return writeError(err, stderr)
	}
	if asJSON {
		if err := printJSON(stdout, result); err != nil {
			return writeError(err, stderr)
		}
	} else {
		render()
	}
	return 0
}

func writeError(err error, stderr io.Writer) int {
	if err == nil {
		return 0
	}
	fmt.Fprintln(stderr, err.Error())
	if liveErr, ok := err.(*LiveSkillsError); ok && liveErr.Code != 0 {
		return liveErr.Code
	}
	return 1
}

func helpText() string {
	return `LiveSkills

Usage: liveskills [options] [command]

Options:
  -h, --help           Display help for command

Commands:
  add <source-or-ref>  Add a skill
  remove               Remove installed skills
  list                 List installed skills
  find [query]         Search for skills
  publish <source>     Publish a skill
`
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return ""
}

func joinInstallTargets(targets []InstallTargetResult) string {
	if len(targets) == 0 {
		return "none"
	}
	rows := make([]string, 0, len(targets))
	for _, target := range targets {
		label := target.Agent
		if target.Mode != "" {
			label += " " + target.Mode
		}
		if target.Path != "" {
			label += " -> " + homeRelative(target.Path)
		}
		rows = append(rows, label)
	}
	return strings.Join(rows, ", ")
}

func scanOptions(parsed ParsedArgs) ScanOptions {
	return ScanOptions{
		Project: parsed.Bool("project"),
		Global:  parsed.Bool("global"),
		Agent:   parsed.Flag("agent"),
	}
}

func realEnv() map[string]string {
	env := map[string]string{}
	for _, pair := range os.Environ() {
		key, value, ok := cut(pair, "=")
		if ok {
			env[key] = value
		}
	}
	return env
}
