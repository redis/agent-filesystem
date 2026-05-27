package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
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
			query = strings.Join(parsed.Positionals[1:], " ")
		}
		result, err := app.List(query, false)
		if err != nil {
			return writeError(err, stderr)
		}
		if parsed.Bool("json") {
			if err := printJSON(stdout, result); err != nil {
				return writeError(err, stderr)
			}
			return 0
		}
		runInteractive := parsed.Bool("interactive")
		if !runInteractive && query == "" {
			runInteractive = canRunInteractiveFind(stdin, stdout)
		}
		if runInteractive {
			promptRows := result
			if query != "" {
				promptRows, err = app.List("", false)
				if err != nil {
					return writeError(err, stderr)
				}
			}
			tty, cleanup, ok := interactiveFindTTY(stdin, stdout)
			if !ok {
				return writeError(fail("Interactive find requires a terminal. Run `liveskills find [query]` to print results."), stderr)
			}
			defer cleanup()
			selected, ok, err := runInteractiveFind(tty, stdout, promptRows, query, style)
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
			options := installOptions(parsed)
			if !parsed.Bool("yes") {
				ok, err := confirmInteractiveFindInstall(tty, stdout, selected, options, hasExplicitAgents(parsed), parsed.Bool("global") || parsed.Bool("project"), homeForScan(app.Env), app.Env, style)
				if err != nil {
					return writeError(err, stderr)
				}
				if !ok {
					fmt.Fprintln(stdout, "Installation cancelled")
					return 0
				}
			}
			fmt.Fprintf(stdout, "Installing %s from %s...\n\n", style.bold(skillSearchName(selected)), style.dim(selected.Name))
			installResult, err := app.Add(selected.Name, options)
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
		options := installOptions(parsed)
		interactiveConfirm := !parsed.Bool("yes") && shouldPromptForConfirmation(stdin, stdout)
		if !parsed.Bool("json") && !interactiveConfirm {
			printSecurityAssessment(stdout, parsed.Pos(1))
		}
		if interactiveConfirm {
			ok, err := confirmInteractiveInstall(stdin, stdout, parsed.Pos(1), options, hasExplicitAgents(parsed), parsed.Bool("global") || parsed.Bool("project"), homeForScan(app.Env), app.Env, style)
			if err != nil {
				return writeError(err, stderr)
			}
			if !ok {
				fmt.Fprintln(stdout, "Installation cancelled")
				return 0
			}
		}
		results, err := app.AddMany(parsed.Pos(1), options)
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

func hasExplicitAgents(parsed ParsedArgs) bool {
	return parsed.Flag("agent") != "" || len(parsed.Values("agent")) > 0
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
	line, err := readPromptLine(stdin)
	if err != nil {
		return false, err
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "" || answer == "y" || answer == "yes", nil
}

func confirmInteractiveFindInstall(stdin *os.File, stdout io.Writer, row SkillListItem, options map[string]string, agentsAlreadyChosen bool, scopeAlreadyChosen bool, home string, env map[string]string, style outputStyle) (bool, error) {
	fmt.Fprintln(stdout, "Install Skill")
	fmt.Fprintf(stdout, "%-12s %s\n", "Skill", row.Name)
	if row.Version != "" && row.Version != "-" {
		fmt.Fprintf(stdout, "%-12s %s\n", "Version", row.Version)
	}
	if strings.TrimSpace(row.Description) != "" {
		fmt.Fprintf(stdout, "%-12s %s\n", "Does", row.Description)
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, style.dim("Project installs are written under this project and can be shared with the repo."))
	fmt.Fprintln(stdout, style.dim("Global installs are written under your home directory and are available across projects."))
	fmt.Fprintln(stdout)

	return confirmInteractiveInstall(stdin, stdout, row.Name, options, agentsAlreadyChosen, scopeAlreadyChosen, home, env, style)
}

func confirmInteractiveInstall(stdin *os.File, stdout io.Writer, assessmentSource string, options map[string]string, agentsAlreadyChosen bool, scopeAlreadyChosen bool, home string, env map[string]string, style outputStyle) (bool, error) {
	if !agentsAlreadyChosen {
		agents, ok, err := promptInstallAgents(stdin, stdout, home, env, style)
		if err != nil || !ok {
			return ok, err
		}
		options["agent"] = ""
		options["agents"] = strings.Join(agents, "\n")
	}
	if !scopeAlreadyChosen {
		scope, ok, err := promptInstallScope(stdin, stdout)
		if err != nil || !ok {
			return ok, err
		}
		if scope == scopeGlobal {
			options["global"] = "true"
		} else {
			options["global"] = ""
		}
	}
	if options["copy"] != "true" && installTargetsUseMultipleRoots(options, home, env) {
		mode, ok, err := promptInstallMode(stdin, stdout)
		if err != nil || !ok {
			return ok, err
		}
		if mode == installModeCopy {
			options["copy"] = "true"
		}
	}
	printInteractiveInstallSummary(stdout, options, home, env)
	printSecurityAssessment(stdout, assessmentSource)
	return confirmInstall(stdin, stdout)
}

func promptInstallAgents(stdin *os.File, stdout io.Writer, home string, env map[string]string, style outputStyle) ([]string, bool, error) {
	choices := commonAgentChoices(home, env)
	for {
		fmt.Fprintln(stdout, "Target agents")
		for index, agent := range choices {
			fmt.Fprintf(stdout, "  %d. %-12s %s\n", index+1, agent.DisplayName, style.dim(agentInstallHint(agent)))
		}
		fmt.Fprintln(stdout, "  all. All supported agents")
		fmt.Fprint(stdout, "Which agents do you want to install to? [1] ")
		line, err := readPromptLine(stdin)
		if err != nil {
			return nil, false, err
		}
		selectors, ok, parseErr := parseAgentPromptSelection(line, choices, home, env)
		if parseErr == nil {
			return selectors, ok, nil
		}
		fmt.Fprintln(stdout, parseErr.Error())
	}
}

func commonAgentChoices(home string, env map[string]string) []AgentDefinition {
	preferred := []string{"codex", "claude-code", "cursor", "gemini-cli", "opencode"}
	choices := make([]AgentDefinition, 0, len(preferred))
	for _, name := range preferred {
		if agent, ok := AgentDefinitionByName(home, env, name); ok {
			choices = append(choices, agent)
		}
	}
	return choices
}

func agentInstallHint(agent AgentDefinition) string {
	project := defaultString(agent.ProjectDir, "no project target")
	global := "no global target"
	if agent.GlobalDir != "" {
		global = homeRelative(agent.GlobalDir)
	}
	return project + " / " + global
}

func parseAgentPromptSelection(input string, choices []AgentDefinition, home string, env map[string]string) ([]string, bool, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return []string{"codex"}, true, nil
	}
	lower := strings.ToLower(input)
	switch lower {
	case "q", "quit", "cancel", "n", "no":
		return nil, false, nil
	case "*", "all":
		return []string{"*"}, true, nil
	}
	parts := strings.FieldsFunc(input, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	})
	if len(parts) == 0 {
		return []string{"codex"}, true, nil
	}
	selectors := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if index, err := strconv.Atoi(part); err == nil {
			if index < 1 || index > len(choices) {
				return nil, false, fail("Agent choice %d is not listed.", index)
			}
			name := choices[index-1].Name
			if !seen[name] {
				seen[name] = true
				selectors = append(selectors, name)
			}
			continue
		}
		agent, ok := AgentDefinitionByName(home, env, part)
		if !ok {
			return nil, false, fail("Unsupported agent %q. Choose numbers, names, all, or quit.", part)
		}
		if !seen[agent.Name] {
			seen[agent.Name] = true
			selectors = append(selectors, agent.Name)
		}
	}
	if len(selectors) == 0 {
		return nil, false, fail("Choose at least one agent, all, or quit.")
	}
	return selectors, true, nil
}

func printInteractiveInstallSummary(stdout io.Writer, options map[string]string, home string, env map[string]string) {
	scope := installScope(options)
	selectors := installAgentSelectors(options)
	agents, err := AgentDefinitionsForSelectors(home, env, selectors)
	names := selectors
	if err == nil {
		names = make([]string, 0, len(agents))
		for _, agent := range agents {
			names = append(names, agent.DisplayName)
		}
	}
	mode := "Symlink"
	if options["copy"] == "true" {
		mode = "Copy"
	}
	fmt.Fprintln(stdout, "Installation Summary")
	fmt.Fprintf(stdout, "%-12s %s\n", "Scope", scope)
	fmt.Fprintf(stdout, "%-12s %s\n", "Agents", formatPromptList(names, 5))
	fmt.Fprintf(stdout, "%-12s %s\n", "Mode", mode)
	fmt.Fprintln(stdout)
}

func installTargetsUseMultipleRoots(options map[string]string, home string, env map[string]string) bool {
	selectors := installAgentSelectors(options)
	agents, err := AgentDefinitionsForSelectors(home, env, selectors)
	if err != nil {
		return false
	}
	scope := installScope(options)
	roots := map[string]bool{}
	for _, agent := range agents {
		root := agent.ProjectDir
		if scope == scopeGlobal {
			root = agent.GlobalDir
		}
		if root == "" {
			continue
		}
		roots[root] = true
		if len(roots) > 1 {
			return true
		}
	}
	return false
}

func installAgentSelectors(options map[string]string) []string {
	selectors := optionList(options, "agents")
	if len(selectors) == 0 && options["agent"] != "" {
		selectors = append(selectors, options["agent"])
	}
	if len(selectors) == 0 {
		selectors = []string{"codex"}
	}
	return selectors
}

func formatPromptList(values []string, maxShow int) string {
	if len(values) == 0 {
		return "none"
	}
	if len(values) <= maxShow {
		return strings.Join(values, ", ")
	}
	return strings.Join(values[:maxShow], ", ") + fmt.Sprintf(" +%d more", len(values)-maxShow)
}

func promptInstallMode(stdin *os.File, stdout io.Writer) (string, bool, error) {
	for {
		fmt.Fprintln(stdout, "Installation method")
		fmt.Fprintln(stdout, "  s. Symlink (recommended) - one canonical copy, easy updates")
		fmt.Fprintln(stdout, "  c. Copy - independent copies in each agent folder")
		fmt.Fprint(stdout, "Installation method? [S]ymlink/[c]opy/[q]uit: ")
		line, err := readPromptLine(stdin)
		if err != nil {
			return "", false, err
		}
		answer := strings.ToLower(strings.TrimSpace(line))
		switch answer {
		case "", "s", "symlink":
			return installModeSymlink, true, nil
		case "c", "copy":
			return installModeCopy, true, nil
		case "q", "quit", "cancel", "n", "no":
			return "", false, nil
		default:
			fmt.Fprintln(stdout, "Choose symlink, copy, or quit.")
		}
	}
}

func promptInstallScope(stdin *os.File, stdout io.Writer) (string, bool, error) {
	for {
		fmt.Fprint(stdout, "Installation scope? [P]roject/[g]lobal/[q]uit: ")
		line, err := readPromptLine(stdin)
		if err != nil {
			return "", false, err
		}
		answer := strings.ToLower(strings.TrimSpace(line))
		switch answer {
		case "", "p", "project":
			return scopeProject, true, nil
		case "g", "global":
			return scopeGlobal, true, nil
		case "q", "quit", "cancel", "n", "no":
			return "", false, nil
		default:
			fmt.Fprintln(stdout, "Choose project, global, or quit.")
		}
	}
}

func readPromptLine(stdin *os.File) (string, error) {
	var builder strings.Builder
	buffer := make([]byte, 1)
	for {
		n, err := stdin.Read(buffer)
		if n > 0 {
			switch buffer[0] {
			case '\n':
				return builder.String(), nil
			case '\r':
				continue
			default:
				builder.WriteByte(buffer[0])
			}
		}
		if err != nil {
			if err == io.EOF {
				return builder.String(), nil
			}
			return "", err
		}
	}
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
