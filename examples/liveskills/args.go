package main

import "strings"

type ParsedArgs struct {
	Positionals []string
	Flags       map[string]string
	FlagValues  map[string][]string
	Booleans    map[string]bool
}

func parseArgs(argv []string) ParsedArgs {
	parsed := ParsedArgs{
		Positionals: []string{},
		Flags:       map[string]string{},
		FlagValues:  map[string][]string{},
		Booleans:    map[string]bool{},
	}

	for index := 0; index < len(argv); index++ {
		arg := argv[index]
		if arg == "--" {
			parsed.Positionals = append(parsed.Positionals, argv[index+1:]...)
			break
		}
		if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") && len(arg) > 1 {
			name := shortFlagName(strings.TrimPrefix(arg, "-"))
			if shortFlagNeedsValue(name) {
				if index+1 < len(argv) {
					parsed.addFlag(name, argv[index+1])
					index++
				} else {
					parsed.Booleans[name] = true
				}
				continue
			}
			parsed.Booleans[name] = true
			continue
		}
		if len(arg) < 3 || arg[:2] != "--" {
			parsed.Positionals = append(parsed.Positionals, arg)
			continue
		}
		body := arg[2:]
		if name, value, ok := cut(body, "="); ok {
			parsed.addFlag(name, value)
			continue
		}
		if !longFlagNeedsValue(body) || index+1 >= len(argv) || len(argv[index+1]) >= 2 && argv[index+1][:2] == "--" {
			parsed.Booleans[body] = true
			continue
		}
		parsed.addFlag(body, argv[index+1])
		index++
	}

	return parsed
}

func (p *ParsedArgs) addFlag(name string, value string) {
	p.Flags[name] = value
	p.FlagValues[name] = append(p.FlagValues[name], value)
}

func (p ParsedArgs) Bool(name string) bool {
	return p.Booleans[name] || p.Flags[name] == "true"
}

func (p ParsedArgs) Flag(name string) string {
	return p.Flags[name]
}

func (p ParsedArgs) Values(name string) []string {
	values := p.FlagValues[name]
	return append([]string(nil), values...)
}

func (p ParsedArgs) Pos(index int) string {
	if index >= len(p.Positionals) {
		return ""
	}
	return p.Positionals[index]
}

func cut(value, separator string) (string, string, bool) {
	for index := 0; index+len(separator) <= len(value); index++ {
		if value[index:index+len(separator)] == separator {
			return value[:index], value[index+len(separator):], true
		}
	}
	return value, "", false
}

func shortFlagName(flag string) string {
	switch flag {
	case "g":
		return "global"
	case "a":
		return "agent"
	case "s":
		return "skill"
	case "l":
		return "list"
	case "y":
		return "yes"
	case "h":
		return "help"
	case "p":
		return "project"
	case "i":
		return "interactive"
	default:
		return flag
	}
}

func shortFlagNeedsValue(name string) bool {
	switch name {
	case "agent", "skill":
		return true
	default:
		return false
	}
}

func longFlagNeedsValue(name string) bool {
	switch name {
	case "agent", "endpoint", "mount", "name", "output", "owner", "query", "skill", "token", "version", "visibility", "workspace":
		return true
	default:
		return false
	}
}
