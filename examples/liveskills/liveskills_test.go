package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublishListsAndShowsSkill(t *testing.T) {
	fixture := newFixture(t)
	source := fixture.skill("Review Skill")

	result := fixture.run("publish", source, "--owner", "acme", "--json")
	requireStatus(t, result, 0)
	var payload map[string]any
	decodeJSON(t, result.stdout, &payload)
	if payload["name"] != "acme/review-skill" {
		t.Fatalf("unexpected name: %v", payload["name"])
	}
	if payload["version"] != "0.1.0" {
		t.Fatalf("unexpected version: %v", payload["version"])
	}
	scripts := payload["scripts"].([]any)
	if len(scripts) != 1 || scripts[0] != "scripts/check.js" {
		t.Fatalf("unexpected scripts: %#v", scripts)
	}

	listed := fixture.run("find", "--json")
	requireStatus(t, listed, 0)
	var rows []SkillListItem
	decodeJSON(t, listed.stdout, &rows)
	if len(rows) != 1 || rows[0].Name != "acme/review-skill" {
		t.Fatalf("unexpected list rows: %#v", rows)
	}

	shown := fixture.run("show", "acme/review-skill", "--json")
	requireStatus(t, shown, 0)
	var detail SkillDetail
	decodeJSON(t, shown.stdout, &detail)
	if len(detail.Versions) != 1 || detail.Volume != "skill_acme_review-skill" {
		t.Fatalf("unexpected detail: %#v", detail)
	}
}

func TestCommandVocabularySeparatesAvailableInstalledPublishAndAdd(t *testing.T) {
	fixture := newFixture(t)
	source := fixture.skill("Clear Skill")

	published := fixture.run("publish", source, "--owner", "acme", "--json")
	requireStatus(t, published, 0)
	var payload map[string]any
	decodeJSON(t, published.stdout, &payload)
	if payload["name"] != "acme/clear-skill" {
		t.Fatalf("unexpected published payload: %#v", payload)
	}

	found := fixture.run("find")
	requireStatus(t, found, 0)
	if !strings.Contains(found.stdout, "Available Skills") || !strings.Contains(found.stdout, "Ref: acme/clear-skill") || !strings.Contains(found.stdout, "Install: liveskills add acme/clear-skill") {
		t.Fatalf("unexpected find output: %s", found.stdout)
	}

	installed := fixture.run("add", "acme/clear-skill", "--json")
	requireStatus(t, installed, 0)
	var installPayload InstallResult
	decodeJSON(t, installed.stdout, &installPayload)
	if installPayload.Path != "skills/clear-skill" || installPayload.Scope != scopeProject || installPayload.ListCommand != "liveskills list" || installPayload.CanonicalPath == "" || len(installPayload.Targets) == 0 {
		t.Fatalf("unexpected install payload: %#v", installPayload)
	}
	if installPayload.Targets[0].Mode != installModeSymlink || !strings.Contains(installPayload.Targets[0].Path, ".agents/skills/clear-skill") {
		t.Fatalf("unexpected install target: %#v", installPayload.Targets)
	}

	listed := fixture.run("list")
	requireStatus(t, listed, 0)
	if !strings.Contains(listed.stdout, "Project Skills") || !strings.Contains(listed.stdout, "Clear Skill") || !strings.Contains(listed.stdout, ".agents/skills/clear-skill") {
		t.Fatalf("unexpected list output: %s", listed.stdout)
	}
	if !strings.Contains(listed.stdout, "Local Skills (not managed by LiveSkills)") || !strings.Contains(listed.stdout, "None installed.") {
		t.Fatalf("expected empty local section to be explicit: %s", listed.stdout)
	}

	manifest := readManifest(t, fixture.cwd)
	if len(manifest.Skills) != 1 || manifest.Skills[0].Name != "acme/clear-skill" || manifest.Skills[0].Tracking != "pinned" {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
	installedSkill := readFile(t, filepath.Join(fixture.cwd, ".agents", "skills", "clear-skill", "SKILL.md"))
	sourceSkill := readFile(t, filepath.Join(source, "SKILL.md"))
	if installedSkill != sourceSkill {
		t.Fatalf("installed skill differs from source")
	}
}

func TestListAllPointsToFind(t *testing.T) {
	fixture := newFixture(t)

	listed := fixture.run("list", "--all")
	requireNotStatus(t, listed, 0)
	if !strings.Contains(listed.stderr, "Use `liveskills find`") {
		t.Fatalf("unexpected stderr: %s", listed.stderr)
	}
}

func TestFindFiltersAvailableSkillsByQuery(t *testing.T) {
	fixture := newFixture(t)
	requireStatus(t, fixture.run("publish", fixture.skill("React Skill"), "--owner", "acme"), 0)
	requireStatus(t, fixture.run("publish", fixture.skill("Database Skill"), "--owner", "acme"), 0)

	found := fixture.run("find", "react", "--json")
	requireStatus(t, found, 0)
	var rows []SkillListItem
	decodeJSON(t, found.stdout, &rows)
	if len(rows) != 1 || rows[0].Name != "acme/react-skill" {
		t.Fatalf("unexpected find rows: %#v", rows)
	}
}

func TestFindTextOutputShowsCopyableInstallRefs(t *testing.T) {
	fixture := newFixture(t)
	source := filepath.Join(fixture.root, "long-skill")
	must(t, os.MkdirAll(source, 0o755))
	must(t, os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("---\nname: Long Skill\ndescription: Review React components for accessibility, hook correctness, and maintainable state boundaries without creating hard-to-read terminal output.\nversion: 0.1.0\n---\n\n# Long Skill\n"), 0o644))
	requireStatus(t, fixture.run("publish", source, "--owner", "acme"), 0)

	found := fixture.run("find")
	requireStatus(t, found, 0)
	for _, expected := range []string{
		"Available Skills",
		"long-skill",
		"  Ref: acme/long-skill",
		"  Version: 0.1.0",
		"  Install: liveskills add acme/long-skill",
	} {
		if !strings.Contains(found.stdout, expected) {
			t.Fatalf("expected find output to contain %q\noutput:\n%s", expected, found.stdout)
		}
	}
	for _, unexpected := range []string{">", "up/down navigate"} {
		if strings.Contains(found.stdout, unexpected) {
			t.Fatalf("non-interactive find should not show %q:\n%s", unexpected, found.stdout)
		}
	}
	for _, line := range strings.Split(strings.TrimRight(found.stdout, "\n"), "\n") {
		if len(line) > textOutputWidth {
			t.Fatalf("find output line is too wide (%d): %q\noutput:\n%s", len(line), line, found.stdout)
		}
	}
}

func TestInteractiveFindPromptFiltersAndMarksSelection(t *testing.T) {
	rows := []SkillListItem{
		{Name: "acme/react-skill", DisplayName: "React Skill", Version: "0.1.0"},
		{Name: "acme/database-skill", DisplayName: "Database Skill", Version: "0.1.0"},
	}

	filtered := filterSkillRows(rows, "react")
	lines := findPromptLines("react", filtered, 0, outputStyle{})
	output := strings.Join(lines, "\n")
	for _, expected := range []string{
		"Search skills: react_",
		"  > react-skill acme/react-skill 0.1.0",
		"up/down navigate | enter select | esc cancel",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected interactive prompt to contain %q\noutput:\n%s", expected, output)
		}
	}
	if strings.Contains(output, "database-skill") {
		t.Fatalf("interactive prompt included unfiltered row:\n%s", output)
	}
}

func TestInteractiveFindWaitsForSearchAndShowsTopTen(t *testing.T) {
	rows := make([]SkillListItem, 12)
	for index := range rows {
		name := "Skill " + string(rune('a'+index))
		rows[index] = SkillListItem{Name: "acme/" + slugify(name), DisplayName: name, Version: "0.1.0"}
	}

	if filtered := filterSkillRows(rows, "s"); len(filtered) != 0 {
		t.Fatalf("expected short query to withhold results, got %#v", filtered)
	}
	filtered := filterSkillRows(rows, "skill")
	lines := findPromptLines("skill", filtered, 0, outputStyle{})
	output := strings.Join(lines, "\n")
	if !strings.Contains(output, "skill-j") {
		t.Fatalf("expected tenth match in prompt:\n%s", output)
	}
	if strings.Contains(output, "skill-k") || strings.Contains(output, "skill-l") {
		t.Fatalf("expected prompt to cap visible results at ten:\n%s", output)
	}
}

func TestInteractiveFindInstallConfirmationSelectsScope(t *testing.T) {
	input := promptInput(t, "\ng\ny\n")
	var output bytes.Buffer
	options := map[string]string{}
	home := t.TempDir()
	row := SkillListItem{
		Name:        "acme/review-skill",
		DisplayName: "Review Skill",
		Description: "Reviews code for maintainability before handoff.",
		Version:     "0.1.0",
	}

	ok, err := confirmInteractiveFindInstall(input, &output, row, options, false, false, home, map[string]string{"HOME": home}, outputStyle{})
	must(t, err)
	if !ok {
		t.Fatalf("expected confirmation")
	}
	if options["global"] != "true" {
		t.Fatalf("expected global scope option, got %#v", options)
	}
	if options["agents"] != "codex" {
		t.Fatalf("expected default codex agent, got %#v", options)
	}
	for _, expected := range []string{
		"Install Skill",
		"Skill        acme/review-skill",
		"Does         Reviews code for maintainability before handoff.",
		"Which agents do you want to install to?",
		"Installation scope?",
		"Agents       Codex",
		"Security Risk Assessments",
		"Proceed with installation?",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("expected confirmation output to contain %q\noutput:\n%s", expected, output.String())
		}
	}
}

func TestInteractiveFindInstallConfirmationCanCancelAtScope(t *testing.T) {
	input := promptInput(t, "\nq\n")
	var output bytes.Buffer
	options := map[string]string{}
	home := t.TempDir()

	ok, err := confirmInteractiveFindInstall(input, &output, SkillListItem{Name: "acme/nope"}, options, false, false, home, map[string]string{"HOME": home}, outputStyle{})
	must(t, err)
	if ok {
		t.Fatalf("expected cancelled install")
	}
	if _, exists := options["global"]; exists {
		t.Fatalf("cancelled scope should not set options: %#v", options)
	}
	if strings.Contains(output.String(), "Security Risk Assessments") {
		t.Fatalf("cancelled scope should not continue to security confirmation:\n%s", output.String())
	}
}

func TestPromptInstallAgentsAcceptsMultipleChoices(t *testing.T) {
	home := t.TempDir()
	input := promptInput(t, "1,2\n")
	var output bytes.Buffer

	agents, ok, err := promptInstallAgents(input, &output, home, map[string]string{"HOME": home}, outputStyle{})
	must(t, err)
	if !ok {
		t.Fatalf("expected agent selection")
	}
	if strings.Join(agents, ",") != "codex,claude-code" {
		t.Fatalf("unexpected agents: %#v", agents)
	}
	for _, expected := range []string{
		"Target agents",
		"Codex",
		"Claude Code",
		"Which agents do you want to install to?",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("expected agent prompt to contain %q\noutput:\n%s", expected, output.String())
		}
	}
}

func TestInteractiveInstallPromptsForModeWithMultipleAgentRoots(t *testing.T) {
	home := t.TempDir()
	input := promptInput(t, "p\nc\ny\n")
	var output bytes.Buffer
	options := map[string]string{"agents": "codex\nclaude-code"}

	ok, err := confirmInteractiveInstall(input, &output, "acme/review-skill", options, true, false, home, map[string]string{"HOME": home}, outputStyle{})
	must(t, err)
	if !ok {
		t.Fatalf("expected confirmation")
	}
	if options["copy"] != "true" {
		t.Fatalf("expected copy mode option, got %#v", options)
	}
	for _, expected := range []string{
		"Installation method",
		"Installation method? [S]ymlink/[c]opy/[q]uit:",
		"Mode         Copy",
		"Agents       Codex, Claude Code",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("expected mode prompt to contain %q\noutput:\n%s", expected, output.String())
		}
	}
}

func TestInteractiveFindRequiresTerminal(t *testing.T) {
	fixture := newFixture(t)
	requireStatus(t, fixture.run("publish", fixture.skill("React Skill"), "--owner", "acme"), 0)

	found := fixture.run("find", "--interactive")
	requireNotStatus(t, found, 0)
	if !strings.Contains(found.stderr, "Interactive find requires a terminal") {
		t.Fatalf("unexpected stderr: %s", found.stderr)
	}
}

func TestHelpUsesAFSStyleFormatting(t *testing.T) {
	fixture := newFixture(t)

	help := fixture.run("help")
	requireStatus(t, help, 0)
	expected := `LiveSkills

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
	if help.stdout != expected {
		t.Fatalf("unexpected help output:\n%s", help.stdout)
	}
	for _, unexpected := range []string{"Advanced:", "Environment:", "list --all", "$ liveskills"} {
		if strings.Contains(help.stdout, unexpected) {
			t.Fatalf("help should not surface %q:\n%s", unexpected, help.stdout)
		}
	}
}

func TestAddIsIdempotentForSamePinnedVersion(t *testing.T) {
	fixture := newFixture(t)
	source := fixture.skill("Idempotent Skill")
	requireStatus(t, fixture.run("publish", source, "--owner", "acme"), 0)
	workspace := filepath.Join(fixture.root, "workspace")

	first := fixture.run("add", "acme/idempotent-skill", "--workspace", "demo", "--mount", workspace)
	requireStatus(t, first, 0)
	if !strings.Contains(first.stdout, "Skill added") {
		t.Fatalf("unexpected first output: %s", first.stdout)
	}
	second := fixture.run("add", "acme/idempotent-skill", "--workspace", "demo", "--mount", workspace)
	requireStatus(t, second, 0)
	if !strings.Contains(second.stdout, "Skill already added") {
		t.Fatalf("unexpected second output: %s", second.stdout)
	}
	manifest := readManifest(t, workspace)
	if len(manifest.Skills) != 1 {
		t.Fatalf("expected one manifest skill, got %d", len(manifest.Skills))
	}
}

func TestMountSkillVolumeIgnoresExistingSameMount(t *testing.T) {
	mountPoint := filepath.Join("tmp", "skill")
	runner := &scriptedAFSRunner{
		failures: map[int]error{
			1: fail("afs vol mount --yes --session liveskills-demo skill_acme_demo %s failed: Error\n\nPath %s overlaps existing mount skill_acme_demo at %s.", mountPoint, mountPoint, mountPoint),
		},
	}
	adapter := &LocalAFSAdapter{Mode: afsModeCLI, Runner: runner}

	must(t, adapter.MountSkillVolume("skill_acme_demo", "chk_demo", mountPoint, scopeProject, "codex"))
	if len(runner.calls) != 2 {
		t.Fatalf("unexpected calls: %#v", runner.calls)
	}
	requireAFSCall(t, runner.calls[1], "vol", "mount", "--yes", "--session")
}

func TestPublishThenAddIsTheHappyPath(t *testing.T) {
	fixture := newFixture(t)
	source := fixture.skill("Simple Skill")
	requireStatus(t, fixture.run("publish", source), 0)

	added := fixture.run("add", "local/simple-skill", "--json")
	requireStatus(t, added, 0)
	var payload InstallResult
	decodeJSON(t, added.stdout, &payload)
	if payload.Name != "local/simple-skill" {
		t.Fatalf("unexpected name: %#v", payload)
	}
	if payload.Workspace != filepath.Base(fixture.cwd) {
		t.Fatalf("expected cwd workspace, got %s", payload.Workspace)
	}
	if readFile(t, filepath.Join(fixture.cwd, ".agents", "skills", "simple-skill", "SKILL.md")) == "" {
		t.Fatalf("expected skill installed in cwd")
	}

	listed := fixture.run("list", "--json")
	requireStatus(t, listed, 0)
	var rows []InstalledSkillItem
	decodeJSON(t, listed.stdout, &rows)
	if len(rows) != 1 || rows[0].Name != "local/simple-skill" {
		t.Fatalf("expected installed skill, got %#v", rows)
	}
}

func TestAddLocalSourcePublishesAndInstalls(t *testing.T) {
	fixture := newFixture(t)
	source := fixture.skill("Source Skill")

	added := fixture.run("add", source)
	requireStatus(t, added, 0)
	if !strings.Contains(added.stdout, "Skill added") || !strings.Contains(added.stdout, "local/source-skill") {
		t.Fatalf("unexpected stdout: %s", added.stdout)
	}
	if readFile(t, filepath.Join(fixture.cwd, ".agents", "skills", "source-skill", "SKILL.md")) == "" {
		t.Fatalf("expected source skill installed")
	}
}

func TestListDefaultsToProjectAndHintsGlobal(t *testing.T) {
	fixture := newFixture(t)

	listed := fixture.run("list")
	requireStatus(t, listed, 0)
	if !strings.Contains(listed.stdout, "No project skills found.") || !strings.Contains(listed.stdout, "Try listing global skills with -g") {
		t.Fatalf("unexpected list output: %s", listed.stdout)
	}
}

func TestGlobalAddAndList(t *testing.T) {
	fixture := newFixture(t)
	source := fixture.skill("Global Skill")
	requireStatus(t, fixture.run("publish", source), 0)
	home := filepath.Join(fixture.root, "user-home")
	must(t, os.MkdirAll(home, 0o755))
	t.Setenv("HOME", home)
	fixture.env["HOME"] = home

	added := fixture.run("add", "-g", "local/global-skill", "--json")
	requireStatus(t, added, 0)
	var payload InstallResult
	decodeJSON(t, added.stdout, &payload)
	if payload.Workspace != "global" || payload.Path != "skills/global-skill" || payload.Scope != scopeGlobal || payload.ListCommand != "liveskills list -g" || len(payload.Targets) != 1 {
		t.Fatalf("unexpected global payload: %#v", payload)
	}
	if readFile(t, filepath.Join(home, ".codex", "skills", "global-skill", "SKILL.md")) == "" {
		t.Fatalf("expected global codex skill")
	}
	if _, err := os.Stat(filepath.Join(fixture.cwd, ".agents")); !os.IsNotExist(err) {
		t.Fatalf("global install should not create project .agents, err=%v", err)
	}

	projectList := fixture.run("list")
	requireStatus(t, projectList, 0)
	if !strings.Contains(projectList.stdout, "No project skills found.") {
		t.Fatalf("global skill leaked into project list: %s", projectList.stdout)
	}
	globalList := fixture.run("list", "-g")
	requireStatus(t, globalList, 0)
	if !strings.Contains(globalList.stdout, "Global Skills") || !strings.Contains(globalList.stdout, "Global Live Skills (managed by LiveSkills)") || !strings.Contains(globalList.stdout, "~/.codex/skills/global-skill") || !strings.Contains(globalList.stdout, "Status: live, managed by LiveSkills") || !strings.Contains(globalList.stdout, "Version: 0.1.0") {
		t.Fatalf("unexpected global list: %s", globalList.stdout)
	}
	if !strings.Contains(globalList.stdout, "Local Skills (not managed by LiveSkills)") || !strings.Contains(globalList.stdout, "None installed.") {
		t.Fatalf("expected empty global local section to be explicit: %s", globalList.stdout)
	}
	if !strings.Contains(globalList.stdout, "Scope: global") {
		t.Fatalf("expected global live skill scope in list: %s", globalList.stdout)
	}
	globalJSON := fixture.run("list", "-g", "--json")
	requireStatus(t, globalJSON, 0)
	var globalRows []InstalledSkillItem
	decodeJSON(t, globalJSON.stdout, &globalRows)
	if len(globalRows) != 1 || !globalRows[0].Live || !globalRows[0].Managed || globalRows[0].ManagedBy != "LiveSkills" || globalRows[0].Version != "0.1.0" || globalRows[0].Volume == "" || globalRows[0].Checkpoint == "" {
		t.Fatalf("expected live managed global row, got %#v", globalRows)
	}
}

func TestGlobalListShowsCurrentProjectLiveSkills(t *testing.T) {
	fixture := newFixture(t)
	home := filepath.Join(fixture.root, "user-home")
	for _, path := range []string{
		filepath.Join(home, ".codex"),
		filepath.Join(home, ".cursor"),
		filepath.Join(home, ".gemini", "antigravity"),
	} {
		must(t, os.MkdirAll(path, 0o755))
	}
	t.Setenv("HOME", home)
	fixture.env["HOME"] = home
	source := fixture.skill("Project Live Skill")
	requireStatus(t, fixture.run("publish", source), 0)
	requireStatus(t, fixture.run("add", "local/project-live-skill"), 0)

	globalList := fixture.run("list", "-g")
	requireStatus(t, globalList, 0)
	for _, expected := range []string{
		"Global Skills",
		"Current Project Live Skills (managed by LiveSkills)",
		"Global Live Skills (managed by LiveSkills)",
		"Project Live Skill",
		".agents/skills/project-live-skill",
		"Agents: Antigravity, Codex, Cursor, Gemini CLI",
		"Scope: project",
		"Status: live, managed by LiveSkills",
		"Local Skills (not managed by LiveSkills)",
	} {
		if !strings.Contains(globalList.stdout, expected) {
			t.Fatalf("expected global list to contain %q\noutput:\n%s", expected, globalList.stdout)
		}
	}
}

func TestGlobalListDiscoversExistingAgentSkills(t *testing.T) {
	fixture := newFixture(t)
	home := filepath.Join(fixture.root, "user-home")
	must(t, os.MkdirAll(home, 0o755))
	t.Setenv("HOME", home)
	fixture.env["HOME"] = home

	writeLocalSkill(t, filepath.Join(home, ".agents", "skills", "afs-shared-memory"), "afs-shared-memory")
	writeLocalSkill(t, filepath.Join(home, ".agents", "skills", "afs-team-board"), "afs-team-board")
	writeLocalSkill(t, filepath.Join(home, ".claude", "skills", "afs-shared-memory"), "afs-shared-memory")
	writeLocalSkill(t, filepath.Join(home, ".claude", "skills", "shared-memory"), "shared-memory")
	writeLocalSkill(t, filepath.Join(home, ".codex", "skills", "imagegen"), "imagegen")
	writeLocalSkill(t, filepath.Join(home, ".codex", "skills", "screenshot"), "screenshot")
	writeLocalSkill(t, filepath.Join(home, ".codex", "skills", "vercel-deploy"), "vercel-deploy")

	listed := fixture.run("list", "-g")
	requireStatus(t, listed, 0)
	for _, expected := range []string{
		"Global Skills",
		"Global Live Skills (managed by LiveSkills)",
		"Local Skills (not managed by LiveSkills)",
		"afs-shared-memory ~/.agents/skills/afs-shared-memory",
		"afs-team-board ~/.agents/skills/afs-team-board",
		"shared-memory ~/.claude/skills/shared-memory",
		"imagegen ~/.codex/skills/imagegen",
		"screenshot ~/.codex/skills/screenshot",
		"vercel-deploy ~/.codex/skills/vercel-deploy",
		"Agents: Claude Code",
		"Agents: Codex",
		"Agents: not linked",
	} {
		if !strings.Contains(listed.stdout, expected) {
			t.Fatalf("expected global list to contain %q\noutput:\n%s", expected, listed.stdout)
		}
	}
	if strings.Contains(listed.stdout, "~/.claude/skills/afs-shared-memory") {
		t.Fatalf("expected duplicate linked skill to collapse to canonical path:\n%s", listed.stdout)
	}

	jsonResult := fixture.run("list", "-g", "--json")
	requireStatus(t, jsonResult, 0)
	var rows []InstalledSkillItem
	decodeJSON(t, jsonResult.stdout, &rows)
	if len(rows) != 6 || rows[0].Name != "afs-shared-memory" || rows[0].Agents[0] != "Claude Code" || rows[0].Live || rows[0].Managed {
		t.Fatalf("unexpected global rows: %#v", rows)
	}
}

func TestGlobalListDiscoversCodexPackagedSkillsWithoutCache(t *testing.T) {
	fixture := newFixture(t)
	home := filepath.Join(fixture.root, "user-home")
	must(t, os.MkdirAll(home, 0o755))
	t.Setenv("HOME", home)
	fixture.env["HOME"] = home

	writeLocalSkill(t, filepath.Join(home, ".codex", "skills", ".system", "imagegen"), "imagegen")
	writeLocalSkill(t, filepath.Join(home, ".codex", "plugins", "cache", "openai-curated", "github", "abc123", "skills", "github"), "github")

	listed := fixture.run("list", "-g")
	requireStatus(t, listed, 0)
	for _, expected := range []string{
		"imagegen ~/.codex/skills/.system/imagegen",
		"Agents: Codex",
	} {
		if !strings.Contains(listed.stdout, expected) {
			t.Fatalf("expected global list to contain %q\noutput:\n%s", expected, listed.stdout)
		}
	}
	if strings.Contains(listed.stdout, ".codex/plugins/cache") || strings.Contains(listed.stdout, "github") {
		t.Fatalf("global list should not include volatile plugin cache skills:\n%s", listed.stdout)
	}

	jsonResult := fixture.run("list", "-g", "--json")
	requireStatus(t, jsonResult, 0)
	var rows []InstalledSkillItem
	decodeJSON(t, jsonResult.stdout, &rows)
	if len(rows) != 1 {
		t.Fatalf("expected packaged global rows, got %#v", rows)
	}
	row := findInstalledSkill(rows, "imagegen")
	if row == nil || row.Scope != scopeGlobal || row.Live || row.Managed || len(row.Agents) != 1 || row.Agents[0] != "Codex" {
		t.Fatalf("unexpected packaged row: %#v", row)
	}

	scanned := fixture.run("scan", "-g", "--json")
	requireStatus(t, scanned, 0)
	var scannedRows []ScannedSkillItem
	decodeJSON(t, scanned.stdout, &scannedRows)
	if len(scannedRows) != 1 || findScannedSkill(scannedRows, "imagegen") == nil || findScannedSkill(scannedRows, "github") != nil {
		t.Fatalf("expected scan to include packaged skills, got %#v", scannedRows)
	}
}

func TestAddValidatesMissingLocalSourceBeforeSecurityPrompt(t *testing.T) {
	fixture := newFixture(t)
	missing := filepath.Join(fixture.root, "missing-skill")

	added := fixture.run("add", missing)
	requireNotStatus(t, added, 0)
	if !strings.Contains(added.stderr, "Skill source not found: "+missing) {
		t.Fatalf("unexpected stderr: %s", added.stderr)
	}
	if strings.Contains(added.stdout, "Security Risk Assessments") {
		t.Fatalf("should not print security prompt for missing local path:\n%s", added.stdout)
	}
}

func TestBooleanLongFlagsMayPrecedePositionals(t *testing.T) {
	fixture := newFixture(t)
	source := fixture.skill("Leading Flag Skill")

	added := fixture.run("add", "--yes", source, "--json")
	requireStatus(t, added, 0)
	var payload InstallResult
	decodeJSON(t, added.stdout, &payload)
	if payload.Name != "local/leading-flag-skill" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestProjectInstallPreservesNeighboringSkills(t *testing.T) {
	fixture := newFixture(t)
	source := fixture.skill("Neighbor Skill")
	requireStatus(t, fixture.run("publish", source), 0)
	manualSkill := filepath.Join(fixture.cwd, ".agents", "skills", "manual-skill")
	must(t, os.MkdirAll(manualSkill, 0o755))
	must(t, os.WriteFile(filepath.Join(manualSkill, "SKILL.md"), []byte("manual\n"), 0o644))

	added := fixture.run("add", "local/neighbor-skill")
	requireStatus(t, added, 0)
	if readFile(t, filepath.Join(manualSkill, "SKILL.md")) != "manual\n" {
		t.Fatalf("manual neighboring skill was touched")
	}
	if readFile(t, filepath.Join(fixture.cwd, ".agents", "skills", "neighbor-skill", "SKILL.md")) == "" {
		t.Fatalf("expected LiveSkills skill mounted at individual folder")
	}
}

func TestAddListsSourceWithoutInstalling(t *testing.T) {
	fixture := newFixture(t)
	source := fixture.skill("Listed Skill")

	listed := fixture.run("add", source, "--list", "--json")
	requireStatus(t, listed, 0)
	var rows []SkillSourceListItem
	decodeJSON(t, listed.stdout, &rows)
	if len(rows) != 1 || rows[0].Slug != "listed-skill" {
		t.Fatalf("unexpected source rows: %#v", rows)
	}
	if _, err := os.Stat(filepath.Join(fixture.cwd, ".agents")); !os.IsNotExist(err) {
		t.Fatalf("--list should not install anything, err=%v", err)
	}
}

func TestAddAndPublishValidateRequiredMetadata(t *testing.T) {
	fixture := newFixture(t)
	invalid := filepath.Join(fixture.root, "invalid")
	must(t, os.MkdirAll(invalid, 0o755))
	must(t, os.WriteFile(filepath.Join(invalid, "SKILL.md"), []byte("# Missing description\n"), 0o644))

	addPath := fixture.run("add", invalid)
	requireNotStatus(t, addPath, 0)
	if !strings.Contains(addPath.stderr, "description") {
		t.Fatalf("unexpected stderr: %s", addPath.stderr)
	}
	published := fixture.run("publish", invalid, "--owner", "acme")
	requireNotStatus(t, published, 0)
	if !strings.Contains(published.stderr, "description") {
		t.Fatalf("unexpected stderr: %s", published.stderr)
	}
}

func TestDownloadExportsPinnedAFSSnapshot(t *testing.T) {
	fixture := newFixture(t)
	source := fixture.skill("Download Skill")
	requireStatus(t, fixture.run("publish", source, "--owner", "acme"), 0)
	output := filepath.Join(fixture.root, "downloaded")

	downloaded := fixture.run("download", "acme/download-skill", "--output", output, "--json")
	requireStatus(t, downloaded, 0)
	var payload DownloadResult
	decodeJSON(t, downloaded.stdout, &payload)
	if payload.Output != output {
		t.Fatalf("unexpected output: %s", payload.Output)
	}
	if readFile(t, filepath.Join(output, "references", "notes.md")) != "Reference notes\n" {
		t.Fatalf("missing downloaded reference")
	}
}

func TestRemoveCleansWorkspaceFilesAndManifestEntry(t *testing.T) {
	fixture := newFixture(t)
	source := fixture.skill("Remove Skill")
	requireStatus(t, fixture.run("publish", source, "--owner", "acme"), 0)
	workspace := filepath.Join(fixture.root, "workspace")
	requireStatus(t, fixture.run("add", "acme/remove-skill", "--workspace", "demo", "--mount", workspace), 0)

	removed := fixture.run("remove", "acme/remove-skill", "--workspace", "demo")
	requireStatus(t, removed, 0)
	if _, err := os.Stat(filepath.Join(workspace, ".agents", "skills", "remove-skill")); !os.IsNotExist(err) {
		t.Fatalf("expected skill directory removed, err=%v", err)
	}
	manifest := readManifest(t, workspace)
	if len(manifest.Skills) != 0 {
		t.Fatalf("expected empty manifest, got %#v", manifest.Skills)
	}
}

func TestAmbiguousMultiSkillSourceRequiresSkill(t *testing.T) {
	fixture := newFixture(t)
	multi := filepath.Join(fixture.root, "multi")
	fixture.skillAt(multi, "One Skill")
	fixture.skillAt(multi, "Two Skill")

	ambiguous := fixture.run("publish", multi, "--owner", "acme")
	requireNotStatus(t, ambiguous, 0)
	if !strings.Contains(ambiguous.stderr, "Multiple skills") {
		t.Fatalf("unexpected stderr: %s", ambiguous.stderr)
	}
}

func TestAddRefusesToReplaceInstalledVersionWithoutUpdate(t *testing.T) {
	fixture := newFixture(t)
	source := fixture.skill("Versioned Skill")
	requireStatus(t, fixture.run("publish", source, "--owner", "acme", "--version", "0.1.0"), 0)
	requireStatus(t, fixture.run("publish", source, "--owner", "acme", "--version", "0.1.1"), 0)
	workspace := filepath.Join(fixture.root, "workspace")
	requireStatus(t, fixture.run("add", "acme/versioned-skill", "--version", "0.1.0", "--workspace", "demo", "--mount", workspace), 0)

	rejected := fixture.run("add", "acme/versioned-skill", "--workspace", "demo", "--mount", workspace)
	requireNotStatus(t, rejected, 0)
	if !strings.Contains(rejected.stderr, "Use liveskills update") {
		t.Fatalf("unexpected stderr: %s", rejected.stderr)
	}
}

func TestUnsupportedAgentsFailBeforePartialInstall(t *testing.T) {
	fixture := newFixture(t)
	source := fixture.skill("Agent Skill")
	requireStatus(t, fixture.run("publish", source, "--owner", "acme"), 0)
	workspace := filepath.Join(fixture.root, "workspace")

	rejected := fixture.run("add", "acme/agent-skill", "--workspace", "demo", "--mount", workspace, "--agent", "unknown-agent")
	requireNotStatus(t, rejected, 0)
	if !strings.Contains(rejected.stderr, "Unsupported agent") {
		t.Fatalf("unexpected stderr: %s", rejected.stderr)
	}
	if _, err := os.Stat(workspace); !os.IsNotExist(err) {
		t.Fatalf("workspace should not exist after unsupported agent, err=%v", err)
	}
}

func TestRepeatedAgentFlagsInstallSymlinksFromOneWorkspace(t *testing.T) {
	fixture := newFixture(t)
	source := fixture.skill("Linked Skill")
	requireStatus(t, fixture.run("publish", source, "--owner", "acme"), 0)

	added := fixture.run("add", "acme/linked-skill", "--agent", "codex", "--agent", "claude-code", "--json")
	requireStatus(t, added, 0)
	var payload InstallResult
	decodeJSON(t, added.stdout, &payload)
	if payload.CanonicalPath == "" || len(payload.Targets) != 2 {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	for _, target := range payload.Targets {
		if target.Mode != installModeSymlink || !SkillSymlinkPointsTo(target.Path, payload.CanonicalPath) {
			t.Fatalf("expected symlink target pointing at canonical path: %#v canonical=%s", target, payload.CanonicalPath)
		}
	}
	if readFile(t, filepath.Join(fixture.cwd, ".agents", "skills", "linked-skill", "SKILL.md")) == "" {
		t.Fatalf("expected codex/universal skill link")
	}
	if readFile(t, filepath.Join(fixture.cwd, ".claude", "skills", "linked-skill", "SKILL.md")) == "" {
		t.Fatalf("expected claude skill link")
	}
	manifest := readManifest(t, fixture.cwd)
	if len(manifest.Skills) != 1 || len(manifest.Skills[0].Targets) != 2 {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
}

func TestCopyInstallDoesNotCreateSymlink(t *testing.T) {
	fixture := newFixture(t)
	source := fixture.skill("Copied Skill")
	requireStatus(t, fixture.run("publish", source, "--owner", "acme"), 0)

	added := fixture.run("add", "acme/copied-skill", "--agent", "claude-code", "--copy", "--json")
	requireStatus(t, added, 0)
	var payload InstallResult
	decodeJSON(t, added.stdout, &payload)
	if payload.Mode != installModeCopy || len(payload.Targets) != 1 || payload.Targets[0].Mode != installModeCopy {
		t.Fatalf("unexpected copy payload: %#v", payload)
	}
	targetPath := filepath.Join(fixture.cwd, ".claude", "skills", "copied-skill")
	info, err := os.Lstat(targetPath)
	must(t, err)
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("copy install should not create a symlink")
	}
	if readFile(t, filepath.Join(targetPath, "SKILL.md")) == "" {
		t.Fatalf("expected copied skill content")
	}
}

func TestAddMultiSkillSourceWithRepeatedSkillFlags(t *testing.T) {
	fixture := newFixture(t)
	sourceRoot := filepath.Join(fixture.root, "multi")
	fixture.skillAt(sourceRoot, "One Skill")
	fixture.skillAt(sourceRoot, "Two Skill")

	added := fixture.run("add", sourceRoot, "--skill", "one-skill", "--skill", "two-skill", "--json")
	requireStatus(t, added, 0)
	var payload []InstallResult
	decodeJSON(t, added.stdout, &payload)
	if len(payload) != 2 || payload[0].Name == payload[1].Name {
		t.Fatalf("unexpected multi-skill payload: %#v", payload)
	}
	if readFile(t, filepath.Join(fixture.cwd, ".agents", "skills", "one-skill", "SKILL.md")) == "" {
		t.Fatalf("expected first skill installed")
	}
	if readFile(t, filepath.Join(fixture.cwd, ".agents", "skills", "two-skill", "SKILL.md")) == "" {
		t.Fatalf("expected second skill installed")
	}
}

func TestRemoveSingleAgentTargetKeepsRemainingWorkspaceSkill(t *testing.T) {
	fixture := newFixture(t)
	source := fixture.skill("Shared Skill")
	requireStatus(t, fixture.run("publish", source, "--owner", "acme"), 0)
	requireStatus(t, fixture.run("add", "acme/shared-skill", "--agent", "codex", "--agent", "claude-code"), 0)

	removed := fixture.run("remove", "acme/shared-skill", "--agent", "codex")
	requireStatus(t, removed, 0)
	if _, err := os.Lstat(filepath.Join(fixture.cwd, ".agents", "skills", "shared-skill")); !os.IsNotExist(err) {
		t.Fatalf("expected codex target removed, got %v", err)
	}
	if readFile(t, filepath.Join(fixture.cwd, ".claude", "skills", "shared-skill", "SKILL.md")) == "" {
		t.Fatalf("expected claude target to remain")
	}
	canonical := filepath.Join(fixture.cwd, ".liveskills", "mount", "skills", "shared-skill")
	if readFile(t, filepath.Join(canonical, "SKILL.md")) == "" {
		t.Fatalf("expected canonical skill to remain while one target exists")
	}
}

func TestAFSCLIAdapterImportsCheckpointsAndMountsVolumes(t *testing.T) {
	root := t.TempDir()
	runner := &recordingAFSRunner{}
	adapter := &LocalAFSAdapter{
		Home:   filepath.Join(root, "home"),
		Root:   filepath.Join(root, "home", "afs"),
		Mode:   afsModeCLI,
		Runner: runner,
	}
	files := []SkillFile{{
		Path:    "SKILL.md",
		Content: []byte("# Demo\n"),
		Hash:    hashText("# Demo\n"),
	}}

	checkpoint, _, err := adapter.PublishVersion("skill_acme_demo", "0.1.0", files)
	must(t, err)
	mountPoint, err := adapter.MaterializeVersion("skill_acme_demo", checkpoint, filepath.Join(root, "project"), ".agents/skills/demo")
	must(t, err)
	must(t, adapter.MountSkillVolume("skill_acme_demo", checkpoint, mountPoint, scopeProject, "codex"))

	if len(runner.calls) != 4 {
		t.Fatalf("unexpected calls: %#v", runner.calls)
	}
	requireAFSCall(t, runner.calls[0], "vol", "import", "skill_acme_demo")
	requireAFSCall(t, runner.calls[1], "cp", "create", "skill_acme_demo", checkpoint, "--description", "LiveSkills 0.1.0")
	requireAFSCall(t, runner.calls[2], "cp", "restore", "skill_acme_demo", checkpoint)
	requireAFSCall(t, runner.calls[3], "vol", "mount", "--yes", "--session")
	if runner.calls[3][len(runner.calls[3])-2] != "skill_acme_demo" || runner.calls[3][len(runner.calls[3])-1] != mountPoint {
		t.Fatalf("unexpected mount call: %#v", runner.calls[3])
	}
	if readFile(t, filepath.Join(adapter.versionPath("skill_acme_demo", checkpoint), "SKILL.md")) != "# Demo\n" {
		t.Fatalf("expected local checkpoint staging")
	}
}

func TestAFSCLIAdapterImportsStagedCheckpointWhenVolumeIsMissing(t *testing.T) {
	root := t.TempDir()
	runner := &scriptedAFSRunner{
		failures: map[int]error{
			0: fail("afs cp restore skill_acme_demo chk_demo failed: Volume \"skill_acme_demo\" does not exist."),
		},
	}
	adapter := &LocalAFSAdapter{
		Home:   filepath.Join(root, "home"),
		Root:   filepath.Join(root, "home", "afs"),
		Mode:   afsModeCLI,
		Runner: runner,
	}
	staged := adapter.versionPath("skill_acme_demo", "chk_demo")
	must(t, os.MkdirAll(staged, 0o755))
	must(t, os.WriteFile(filepath.Join(staged, "SKILL.md"), []byte("# Demo\n"), 0o644))

	mountPoint := filepath.Join(root, "project", ".agents", "skills", "demo")
	must(t, adapter.MountSkillVolume("skill_acme_demo", "chk_demo", mountPoint, scopeProject, "codex"))

	if len(runner.calls) != 5 {
		t.Fatalf("unexpected calls: %#v", runner.calls)
	}
	requireAFSCall(t, runner.calls[0], "cp", "restore", "skill_acme_demo", "chk_demo")
	requireAFSCall(t, runner.calls[1], "vol", "import", "skill_acme_demo", staged)
	requireAFSCall(t, runner.calls[2], "cp", "create", "skill_acme_demo", "chk_demo", "--description", "LiveSkills chk_demo")
	requireAFSCall(t, runner.calls[3], "cp", "restore", "skill_acme_demo", "chk_demo")
	requireAFSCall(t, runner.calls[4], "vol", "mount", "--yes", "--session")
}

func TestAFSCLIAdapterRetriesImportWithForceWhenVolumeExists(t *testing.T) {
	root := t.TempDir()
	runner := &scriptedAFSRunner{
		failures: map[int]error{
			0: fail("afs vol import skill_acme_demo /tmp/staged failed: volume \"skill_acme_demo\" already exists; rerun with --force to replace it"),
		},
	}
	adapter := &LocalAFSAdapter{
		Home:   filepath.Join(root, "home"),
		Root:   filepath.Join(root, "home", "afs"),
		Mode:   afsModeCLI,
		Runner: runner,
	}
	files := []SkillFile{{
		Path:    "SKILL.md",
		Content: []byte("# Demo\n"),
		Hash:    hashText("# Demo\n"),
	}}

	checkpoint, _, err := adapter.PublishVersion("skill_acme_demo", "0.2.0", files)
	must(t, err)

	if len(runner.calls) != 3 {
		t.Fatalf("unexpected calls: %#v", runner.calls)
	}
	staged := adapter.versionPath("skill_acme_demo", checkpoint)
	requireAFSCall(t, runner.calls[0], "vol", "import", "skill_acme_demo", staged)
	requireAFSCall(t, runner.calls[1], "vol", "import", "--force", "skill_acme_demo", staged)
	requireAFSCall(t, runner.calls[2], "cp", "create", "skill_acme_demo", checkpoint, "--description", "LiveSkills 0.2.0")
}

func TestEnsureRelativeSkillSymlinkCreatesRelativeLink(t *testing.T) {
	root := t.TempDir()
	canonical := filepath.Join(root, "workspace", "skills", "demo")
	link := filepath.Join(root, "project", ".agents", "skills", "demo")
	writeLocalSkill(t, canonical, "Demo")

	must(t, EnsureRelativeSkillSymlink(link, canonical))

	target, err := os.Readlink(link)
	must(t, err)
	if filepath.IsAbs(target) {
		t.Fatalf("expected relative symlink target, got %q", target)
	}
	if !SkillSymlinkPointsTo(link, canonical) {
		t.Fatalf("expected %s to point at %s", link, canonical)
	}
}

func TestEnsureRelativeSkillSymlinkRefusesUnmanagedPath(t *testing.T) {
	root := t.TempDir()
	canonical := filepath.Join(root, "workspace", "skills", "demo")
	link := filepath.Join(root, "project", ".agents", "skills", "demo")
	writeLocalSkill(t, canonical, "Demo")
	must(t, os.MkdirAll(link, 0o755))

	err := EnsureRelativeSkillSymlink(link, canonical)
	if err == nil || !strings.Contains(err.Error(), "Refusing to overwrite unmanaged skill path") {
		t.Fatalf("expected unmanaged path refusal, got %v", err)
	}
}

func TestEnsureRelativeSkillSymlinkRepairsSafeBrokenLink(t *testing.T) {
	root := t.TempDir()
	canonical := filepath.Join(root, "new", "skills", "demo")
	link := filepath.Join(root, "project", ".agents", "skills", "demo")
	oldTarget := filepath.Join(root, "old", "skills", "demo")
	writeLocalSkill(t, canonical, "Demo")
	must(t, os.MkdirAll(filepath.Dir(link), 0o755))
	relativeOld, err := filepath.Rel(filepath.Dir(link), oldTarget)
	must(t, err)
	must(t, os.Symlink(relativeOld, link))

	must(t, EnsureRelativeSkillSymlink(link, canonical))

	if !SkillSymlinkPointsTo(link, canonical) {
		t.Fatalf("expected repaired symlink to point at %s", canonical)
	}
}

func TestCopySkillFromCanonicalPreservesNeighboringSkills(t *testing.T) {
	root := t.TempDir()
	canonical := filepath.Join(root, "workspace", "skills", "demo")
	target := filepath.Join(root, "project", ".agents", "skills", "demo")
	neighbor := filepath.Join(root, "project", ".agents", "skills", "neighbor")
	writeLocalSkill(t, canonical, "Demo")
	must(t, os.MkdirAll(target, 0o755))
	must(t, os.WriteFile(filepath.Join(target, "stale.txt"), []byte("stale\n"), 0o644))
	writeLocalSkill(t, neighbor, "Neighbor")

	must(t, CopySkillFromCanonical(canonical, target))

	if _, err := os.Stat(filepath.Join(target, "stale.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected stale target file to be removed, got %v", err)
	}
	if readFile(t, filepath.Join(target, "SKILL.md")) != readFile(t, filepath.Join(canonical, "SKILL.md")) {
		t.Fatalf("expected copied skill content")
	}
	if _, err := os.Stat(filepath.Join(neighbor, "SKILL.md")); err != nil {
		t.Fatalf("expected neighboring skill to be preserved: %v", err)
	}
}

func TestLocalSkillsWorkspaceAttachesAndDetachesUnusedVolume(t *testing.T) {
	root := t.TempDir()
	adapter := &LocalAFSAdapter{
		Home: filepath.Join(root, "home"),
		Root: filepath.Join(root, "home", "afs"),
		Mode: afsModeLocal,
	}
	files := []SkillFile{{
		Path:    "SKILL.md",
		Content: []byte("# Demo\n"),
		Hash:    hashText("# Demo\n"),
	}}
	checkpoint, _, err := adapter.PublishVersion("skill_acme_demo", "0.1.0", files)
	must(t, err)
	workspace, err := adapter.EnsureSkillsWorkspace(scopeProject, "demo")
	must(t, err)
	canonical, err := adapter.AttachSkillVolume(workspace, "skill_acme_demo", checkpoint, "demo")
	must(t, err)
	link := filepath.Join(root, "project", ".agents", "skills", "demo")
	must(t, EnsureRelativeSkillSymlink(link, canonical))

	must(t, adapter.DetachSkillVolumeIfUnused(workspace, "skill_acme_demo", "demo", []string{link}))
	if _, err := os.Stat(filepath.Join(canonical, "SKILL.md")); err != nil {
		t.Fatalf("expected linked canonical skill to stay attached: %v", err)
	}
	must(t, os.Remove(link))
	must(t, adapter.DetachSkillVolumeIfUnused(workspace, "skill_acme_demo", "demo", []string{link}))
	if _, err := os.Stat(canonical); !os.IsNotExist(err) {
		t.Fatalf("expected unused canonical skill to be detached, got %v", err)
	}
}

func TestCLIWorkspaceHelpersUseAFSWorkspaceAndVolumeCalls(t *testing.T) {
	root := t.TempDir()
	runner := &recordingAFSRunner{}
	adapter := &LocalAFSAdapter{
		Home:   filepath.Join(root, "home"),
		Root:   filepath.Join(root, "home", "afs"),
		Mode:   afsModeCLI,
		Runner: runner,
	}

	workspace, err := adapter.EnsureSkillsWorkspace(scopeProject, "demo")
	must(t, err)
	mountPoint := filepath.Join(root, "skills-workspace")
	workspace, err = adapter.MountSkillsWorkspace(workspace, mountPoint)
	must(t, err)
	canonical, err := adapter.AttachSkillVolume(workspace, "skill_acme_demo", "", "demo")
	must(t, err)
	must(t, adapter.DetachSkillVolumeIfUnused(workspace, "skill_acme_demo", "demo", nil))

	if canonical != filepath.Join(mountPoint, "skills", "demo") {
		t.Fatalf("unexpected canonical path: %s", canonical)
	}
	if len(runner.calls) != 4 {
		t.Fatalf("unexpected calls: %#v", runner.calls)
	}
	requireAFSCall(t, runner.calls[0], "ws", "create", "liveskills_project_demo")
	requireAFSCall(t, runner.calls[1], "ws", "mount", "liveskills_project_demo", mountPoint)
	requireAFSCall(t, runner.calls[2], "vol", "mount", "--yes", "--session")
	if runner.calls[2][len(runner.calls[2])-2] != "skill_acme_demo" || runner.calls[2][len(runner.calls[2])-1] != canonical {
		t.Fatalf("unexpected attach call: %#v", runner.calls[2])
	}
	requireAFSCall(t, runner.calls[3], "vol", "unmount", canonical)
}

func TestCLIWorkspaceMountAllowsEmptyHelperWorkspace(t *testing.T) {
	root := t.TempDir()
	mountPoint := filepath.Join(root, "skills-workspace")
	runner := &scriptedAFSRunner{
		failures: map[int]error{
			1: fail("afs ws mount liveskills_project_demo %s failed: Error\n\nWorkspace liveskills_project_demo has no attached volumes.", mountPoint),
		},
	}
	adapter := &LocalAFSAdapter{
		Home:   filepath.Join(root, "home"),
		Root:   filepath.Join(root, "home", "afs"),
		Mode:   afsModeCLI,
		Runner: runner,
	}

	workspace, err := adapter.EnsureSkillsWorkspace(scopeProject, "demo")
	must(t, err)
	workspace, err = adapter.MountSkillsWorkspace(workspace, mountPoint)
	must(t, err)

	if workspace.Root != mountPoint {
		t.Fatalf("unexpected workspace root: %s", workspace.Root)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("unexpected calls: %#v", runner.calls)
	}
	requireAFSCall(t, runner.calls[0], "ws", "create", "liveskills_project_demo")
	requireAFSCall(t, runner.calls[1], "ws", "mount", "liveskills_project_demo", mountPoint)
}

func TestCLIWorkspaceAttachAliasesVolumeMountedAtOtherCanonicalPath(t *testing.T) {
	root := t.TempDir()
	existingMount := filepath.Join(root, "project", ".liveskills", "mount", "skills", "demo")
	writeLocalSkill(t, existingMount, "Demo")
	workspace := SkillsWorkspace{
		ID:    "liveskills_global_global",
		Name:  "global",
		Scope: scopeGlobal,
		Root:  filepath.Join(root, "home", ".liveskills", "mount"),
	}
	canonical := canonicalSkillPath(workspace.Root, "demo")
	runner := &scriptedAFSRunner{
		failures: map[int]error{
			0: fail("afs vol mount --yes --session liveskills-ws-demo skill_acme_demo %s failed: Error\n\nWorkspace skill_acme_demo is already mounted at %s.", canonical, existingMount),
		},
	}
	adapter := &LocalAFSAdapter{
		Home:   filepath.Join(root, "home"),
		Root:   filepath.Join(root, "home", "afs"),
		Mode:   afsModeCLI,
		Runner: runner,
	}

	resolved, err := adapter.AttachSkillVolume(workspace, "skill_acme_demo", "", "demo")
	must(t, err)

	if resolved != canonical {
		t.Fatalf("unexpected canonical path: %s", resolved)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("unexpected calls: %#v", runner.calls)
	}
	requireAFSCall(t, runner.calls[0], "vol", "mount", "--yes", "--session")
	if !SkillSymlinkPointsTo(canonical, existingMount) {
		t.Fatalf("expected canonical alias %s to point at %s", canonical, existingMount)
	}
	if readFile(t, filepath.Join(canonical, "SKILL.md")) == "" {
		t.Fatalf("expected canonical alias to expose skill files")
	}
}

func TestScanFindsSkillsAcrossAgentLocations(t *testing.T) {
	fixture := newFixture(t)
	home := filepath.Join(fixture.root, "user-home")
	must(t, os.MkdirAll(home, 0o755))
	t.Setenv("HOME", home)
	fixture.env["HOME"] = home

	writeLocalSkill(t, filepath.Join(fixture.cwd, ".agents", "skills", "project-skill"), "Project Skill")
	writeLocalSkill(t, filepath.Join(fixture.cwd, ".claude", "skills", "claude-project"), "Claude Project")
	writeLocalSkill(t, filepath.Join(home, ".codex", "skills", "global-codex"), "Global Codex")
	writeLocalSkill(t, filepath.Join(home, ".config", "opencode", "skills", "open-global"), "Open Global")

	scanned := fixture.run("scan", "--json")
	requireStatus(t, scanned, 0)
	var rows []ScannedSkillItem
	decodeJSON(t, scanned.stdout, &rows)
	if len(rows) != 4 {
		t.Fatalf("expected 4 scanned skills, got %#v", rows)
	}

	projectSkill := requireScannedSkill(t, rows, "Project Skill")
	if projectSkill.Scope != scopeProject || projectSkill.DisplayPath != ".agents/skills/project-skill" {
		t.Fatalf("unexpected project skill row: %#v", projectSkill)
	}
	requireAgent(t, projectSkill, "Codex")

	claudeProject := requireScannedSkill(t, rows, "Claude Project")
	if claudeProject.Scope != scopeProject {
		t.Fatalf("unexpected claude scope: %#v", claudeProject)
	}
	requireAgent(t, claudeProject, "Claude Code")

	globalCodex := requireScannedSkill(t, rows, "Global Codex")
	if globalCodex.Scope != scopeGlobal || globalCodex.DisplayPath != "~/.codex/skills/global-codex" {
		t.Fatalf("unexpected global codex row: %#v", globalCodex)
	}
	requireAgent(t, globalCodex, "Codex")

	openGlobal := requireScannedSkill(t, rows, "Open Global")
	if openGlobal.Scope != scopeGlobal {
		t.Fatalf("unexpected opencode scope: %#v", openGlobal)
	}
	requireAgent(t, openGlobal, "OpenCode")

	globalOnly := fixture.run("scan", "-g", "--json")
	requireStatus(t, globalOnly, 0)
	var globalRows []ScannedSkillItem
	decodeJSON(t, globalOnly.stdout, &globalRows)
	if len(globalRows) != 2 || findScannedSkill(globalRows, "Project Skill") != nil {
		t.Fatalf("expected only global rows, got %#v", globalRows)
	}

	projectOnly := fixture.run("scan", "-p", "--json")
	requireStatus(t, projectOnly, 0)
	var projectRows []ScannedSkillItem
	decodeJSON(t, projectOnly.stdout, &projectRows)
	if len(projectRows) != 2 || findScannedSkill(projectRows, "Global Codex") != nil {
		t.Fatalf("expected only project rows, got %#v", projectRows)
	}
}

func TestScanIncludesStandardSkillFolders(t *testing.T) {
	fixture := newFixture(t)
	writeLocalSkill(t, filepath.Join(fixture.cwd, "skills", ".experimental", "experimental-skill"), "Experimental Skill")

	scanned := fixture.run("scan", "-p", "--json")
	requireStatus(t, scanned, 0)
	var rows []ScannedSkillItem
	decodeJSON(t, scanned.stdout, &rows)

	row := requireScannedSkill(t, rows, "Experimental Skill")
	if row.DisplayPath != "skills/.experimental/experimental-skill" {
		t.Fatalf("unexpected standard skill path: %#v", row)
	}
	requireAgent(t, row, "Standard")
}

func TestScanTextOutputIncludesSummaryAndSpacing(t *testing.T) {
	fixture := newFixture(t)
	home := filepath.Join(fixture.root, "user-home")
	must(t, os.MkdirAll(home, 0o755))
	t.Setenv("HOME", home)
	fixture.env["HOME"] = home

	writeLocalSkill(t, filepath.Join(fixture.cwd, ".agents", "skills", "project-skill"), "Project Skill")
	writeLocalSkill(t, filepath.Join(home, ".codex", "skills", "global-codex"), "Global Codex")
	writeLocalSkill(t, filepath.Join(home, ".claude", "skills", "global-claude"), "Global Claude")

	scanned := fixture.run("scan")
	requireStatus(t, scanned, 0)

	for _, expected := range []string{
		"Summary",
		"3 skills found",
		"1 project skill",
		"2 global skills",
		"1 project skill in .agents/skills (shared by",
		"1 global skill in ~/.codex/skills (Codex)",
		"1 global skill in ~/.claude/skills (Claude Code)",
		"\n\nGlobal Claude\n",
		"\n\nGlobal Codex\n",
		"\n\nProject Skill\n",
		"  Path: ~/.codex/skills/global-codex",
		"  Agents: Codex",
	} {
		if !strings.Contains(scanned.stdout, expected) {
			t.Fatalf("expected scan output to contain %q\noutput:\n%s", expected, scanned.stdout)
		}
	}
}

type fixture struct {
	root string
	cwd  string
	env  map[string]string
}

type cliResult struct {
	status int
	stdout string
	stderr string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	cwd := filepath.Join(root, "project")
	must(t, os.MkdirAll(cwd, 0o755))
	return &fixture{
		root: root,
		cwd:  cwd,
		env: map[string]string{
			"LIVESKILLS_AFS_MODE": "local",
			"LIVESKILLS_HOME":     filepath.Join(root, "home"),
			"USER":                "tester",
		},
	}
}

func (f *fixture) run(args ...string) cliResult {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	status := runCLI(args, f.cwd, f.env, &stdout, &stderr)
	return cliResult{status: status, stdout: stdout.String(), stderr: stderr.String()}
}

func promptInput(t *testing.T, text string) *os.File {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "prompt-input-*")
	must(t, err)
	_, err = file.WriteString(text)
	must(t, err)
	_, err = file.Seek(0, 0)
	must(t, err)
	t.Cleanup(func() { _ = file.Close() })
	return file
}

func (f *fixture) skill(name string) string {
	return f.skillAt(f.root, name)
}

func (f *fixture) skillAt(root string, name string) string {
	slug := strings.ReplaceAll(strings.ToLower(name), " ", "-")
	directory := filepath.Join(root, slug)
	mustPanic(os.MkdirAll(filepath.Join(directory, "scripts"), 0o755))
	mustPanic(os.MkdirAll(filepath.Join(directory, "references"), 0o755))
	mustPanic(os.WriteFile(filepath.Join(directory, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: "+name+" description.\nversion: 0.1.0\n---\n\n# "+name+"\n"), 0o644))
	mustPanic(os.WriteFile(filepath.Join(directory, "scripts", "check.js"), []byte("console.log('check')\n"), 0o644))
	mustPanic(os.WriteFile(filepath.Join(directory, "references", "notes.md"), []byte("Reference notes\n"), 0o644))
	return directory
}

func writeLocalSkill(t *testing.T, directory string, name string) {
	t.Helper()
	must(t, os.MkdirAll(directory, 0o755))
	must(t, os.WriteFile(filepath.Join(directory, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: "+name+" description.\n---\n\n# "+name+"\n"), 0o644))
}

func requireScannedSkill(t *testing.T, rows []ScannedSkillItem, name string) ScannedSkillItem {
	t.Helper()
	row := findScannedSkill(rows, name)
	if row == nil {
		t.Fatalf("missing scanned skill %q in %#v", name, rows)
	}
	return *row
}

func findScannedSkill(rows []ScannedSkillItem, name string) *ScannedSkillItem {
	for index := range rows {
		if rows[index].Name == name {
			return &rows[index]
		}
	}
	return nil
}

func findInstalledSkill(rows []InstalledSkillItem, name string) *InstalledSkillItem {
	for index := range rows {
		if rows[index].Name == name {
			return &rows[index]
		}
	}
	return nil
}

func requireAgent(t *testing.T, row ScannedSkillItem, agent string) {
	t.Helper()
	for _, candidate := range row.Agents {
		if candidate == agent {
			return
		}
	}
	t.Fatalf("expected %s in agents for %#v", agent, row)
}

type recordingAFSRunner struct {
	calls [][]string
}

func (r *recordingAFSRunner) Run(args ...string) error {
	r.calls = append(r.calls, append([]string(nil), args...))
	return nil
}

type scriptedAFSRunner struct {
	calls    [][]string
	failures map[int]error
}

func (r *scriptedAFSRunner) Run(args ...string) error {
	callIndex := len(r.calls)
	r.calls = append(r.calls, append([]string(nil), args...))
	if err := r.failures[callIndex]; err != nil {
		return err
	}
	return nil
}

func requireAFSCall(t *testing.T, actual []string, prefix ...string) {
	t.Helper()
	if len(actual) < len(prefix) {
		t.Fatalf("expected call prefix %#v, got %#v", prefix, actual)
	}
	for index, expected := range prefix {
		if actual[index] != expected {
			t.Fatalf("expected call prefix %#v, got %#v", prefix, actual)
		}
	}
}

func requireStatus(t *testing.T, result cliResult, expected int) {
	t.Helper()
	if result.status != expected {
		t.Fatalf("expected status %d, got %d\nstdout: %s\nstderr: %s", expected, result.status, result.stdout, result.stderr)
	}
}

func requireNotStatus(t *testing.T, result cliResult, unexpected int) {
	t.Helper()
	if result.status == unexpected {
		t.Fatalf("did not expect status %d\nstdout: %s\nstderr: %s", unexpected, result.stdout, result.stderr)
	}
}

func decodeJSON(t *testing.T, input string, target any) {
	t.Helper()
	if err := json.Unmarshal([]byte(input), target); err != nil {
		t.Fatalf("json decode failed: %v\ninput: %s", err, input)
	}
}

func readManifest(t *testing.T, workspace string) WorkspaceManifest {
	t.Helper()
	var manifest WorkspaceManifest
	decodeJSON(t, readFile(t, filepath.Join(workspace, ".liveskills", "manifest.json")), &manifest)
	return manifest
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	must(t, err)
	return string(data)
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func mustPanic(err error) {
	if err != nil {
		panic(err)
	}
}
