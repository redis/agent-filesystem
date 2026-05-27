package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type ScanOptions struct {
	Project bool
	Global  bool
	Agent   string
}

type ScannedSkillItem struct {
	Name        string   `json:"name"`
	Slug        string   `json:"slug"`
	Description string   `json:"description"`
	Scope       string   `json:"scope"`
	Path        string   `json:"path"`
	DisplayPath string   `json:"displayPath"`
	Agents      []string `json:"agents"`
}

type scanLocation struct {
	Scope  string
	Path   string
	Direct bool
	Agents []string
}

type scannedSkillAccumulator struct {
	item      ScannedSkillItem
	agentSet  map[string]bool
	sortAgent []string
}

func (a *App) Scan(options ScanOptions) ([]ScannedSkillItem, error) {
	home := homeForScan(a.Env)
	includeProject, includeGlobal := scanScopes(options)
	locations := scanLocations(a.CWD, home, a.Env, includeProject, includeGlobal, options.Agent)
	accumulators := map[string]*scannedSkillAccumulator{}

	for _, location := range locations {
		skillDirs, err := scanSkillDirs(location)
		if err != nil {
			return nil, err
		}
		for _, skillDir := range skillDirs {
			item, err := scanSkillDirectory(skillDir, location.Scope, a.CWD, home)
			if err != nil {
				return nil, err
			}
			key := item.Scope + "\x00" + filepath.Clean(item.Path)
			accumulator := accumulators[key]
			if accumulator == nil {
				accumulator = &scannedSkillAccumulator{
					item:     item,
					agentSet: map[string]bool{},
				}
				accumulators[key] = accumulator
			}
			for _, agent := range location.Agents {
				if !accumulator.agentSet[agent] {
					accumulator.agentSet[agent] = true
					accumulator.sortAgent = append(accumulator.sortAgent, agent)
				}
			}
		}
	}

	items := make([]ScannedSkillItem, 0, len(accumulators))
	for _, accumulator := range accumulators {
		sort.Strings(accumulator.sortAgent)
		accumulator.item.Agents = accumulator.sortAgent
		items = append(items, accumulator.item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Scope != items[j].Scope {
			return items[i].Scope > items[j].Scope
		}
		if strings.EqualFold(items[i].Name, items[j].Name) {
			return items[i].Path < items[j].Path
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	return items, nil
}

func scanScopes(options ScanOptions) (includeProject bool, includeGlobal bool) {
	includeProject = true
	includeGlobal = true
	if options.Project && !options.Global {
		includeGlobal = false
	}
	if options.Global && !options.Project {
		includeProject = false
	}
	return includeProject, includeGlobal
}

func scanLocations(cwd string, home string, env map[string]string, includeProject bool, includeGlobal bool, agentFilter string) []scanLocation {
	var locations []scanLocation
	addLocation := func(scope, path string, direct bool, agent string, aliases ...string) {
		if path == "" || !scanAgentMatchesAny(agentFilter, append([]string{agent}, aliases...)...) {
			return
		}
		clean := filepath.Clean(path)
		for index := range locations {
			existing := &locations[index]
			if existing.Scope == scope && existing.Path == clean && existing.Direct == direct {
				if !containsString(existing.Agents, agent) {
					existing.Agents = append(existing.Agents, agent)
				}
				return
			}
		}
		locations = append(locations, scanLocation{Scope: scope, Path: clean, Direct: direct, Agents: []string{agent}})
	}

	if includeProject {
		for _, location := range standardProjectSkillLocations(cwd) {
			addLocation(scopeProject, location.Path, location.Direct, "Standard", "standard")
		}
	}

	for _, agent := range scanAgents(home, env) {
		if includeProject {
			addLocation(scopeProject, filepath.Join(cwd, filepath.FromSlash(agent.ProjectDir)), false, agent.DisplayName, agent.Name)
		}
		if includeGlobal && agent.GlobalDir != "" {
			addLocation(scopeGlobal, agent.GlobalDir, false, agent.DisplayName, agent.Name)
			if agent.Name == "codex" {
				for _, path := range codexPackagedGlobalSkillLocations(filepath.Dir(agent.GlobalDir)) {
					addLocation(scopeGlobal, path, false, agent.DisplayName, agent.Name)
				}
			}
		}
	}

	sort.Slice(locations, func(i, j int) bool {
		if locations[i].Scope != locations[j].Scope {
			return locations[i].Scope > locations[j].Scope
		}
		return locations[i].Path < locations[j].Path
	})
	return locations
}

func standardProjectSkillLocations(cwd string) []scanLocation {
	locations := []scanLocation{
		{Path: cwd, Direct: true},
		{Path: filepath.Join(cwd, "skills")},
	}
	for _, dir := range packagedSkillCollectionDirs() {
		locations = append(locations, scanLocation{Path: filepath.Join(cwd, "skills", dir)})
	}
	return locations
}

func codexPackagedGlobalSkillLocations(codexHome string) []string {
	var paths []string
	for _, dir := range packagedSkillCollectionDirs() {
		paths = append(paths, filepath.Join(codexHome, "skills", dir))
	}
	return paths
}

func packagedSkillCollectionDirs() []string {
	return []string{".curated", ".experimental", ".system"}
}

func scanSkillDirs(location scanLocation) ([]string, error) {
	if location.Direct {
		if fileExists(filepath.Join(location.Path, "SKILL.md")) {
			return []string{location.Path}, nil
		}
		return []string{}, nil
	}

	entries, err := os.ReadDir(location.Path)
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}

	var skillDirs []string
	for _, entry := range entries {
		candidate := filepath.Join(location.Path, entry.Name())
		if !isDirectoryOrDirectorySymlink(entry, candidate) {
			continue
		}
		if fileExists(filepath.Join(candidate, "SKILL.md")) {
			skillDirs = append(skillDirs, candidate)
		}
	}
	sort.Strings(skillDirs)
	return skillDirs, nil
}

func scanSkillDirectory(path string, scope string, cwd string, home string) (ScannedSkillItem, error) {
	data, err := os.ReadFile(filepath.Join(path, "SKILL.md"))
	if err != nil {
		return ScannedSkillItem{}, err
	}
	metadata := parseSkillMarkdown(string(data))
	name := metadata["name"]
	if name == "" {
		name = filepath.Base(path)
	}
	return ScannedSkillItem{
		Name:        name,
		Slug:        slugify(name),
		Description: metadata["description"],
		Scope:       scope,
		Path:        filepath.Clean(path),
		DisplayPath: displayScanPath(path, cwd, home, scope),
		Agents:      []string{},
	}, nil
}

func isDirectoryOrDirectorySymlink(entry os.DirEntry, path string) bool {
	if entry.IsDir() {
		return true
	}
	if entry.Type()&os.ModeSymlink == 0 {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func openClawHome(home string) string {
	for _, candidate := range []string{".openclaw", ".clawdbot", ".moltbot"} {
		path := filepath.Join(home, candidate)
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path
		}
	}
	return filepath.Join(home, ".openclaw")
}

func directoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func homeForScan(env map[string]string) string {
	if home := strings.TrimSpace(envValue(env, "HOME")); home != "" {
		return filepath.Clean(home)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return home
}

func displayScanPath(path string, cwd string, home string, scope string) string {
	clean := filepath.Clean(path)
	if scope == scopeProject {
		rel, err := filepath.Rel(cwd, clean)
		if err == nil && !strings.HasPrefix(rel, "..") && rel != "." {
			return filepath.ToSlash(rel)
		}
		if err == nil && rel == "." {
			return "."
		}
	}
	if home != "" {
		home = filepath.Clean(home)
		if clean == home {
			return "~"
		}
		if strings.HasPrefix(clean, home+string(os.PathSeparator)) {
			rel, err := filepath.Rel(home, clean)
			if err == nil {
				return "~/" + filepath.ToSlash(rel)
			}
		}
	}
	return clean
}

func scanAgentMatchesAny(filter string, agents ...string) bool {
	filter = strings.ToLower(strings.TrimSpace(filter))
	if filter == "" {
		return true
	}
	for _, agent := range agents {
		agent = strings.ToLower(agent)
		if agent == filter || slugify(agent) == filter {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
