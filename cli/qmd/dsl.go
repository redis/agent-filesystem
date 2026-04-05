package qmd

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// ParseDSL parses a compact query language into filterable parts.
func ParseDSL(input string) (ParsedQuery, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return ParsedQuery{}, fmt.Errorf("query cannot be empty")
	}

	tokens, err := splitTokens(input)
	if err != nil {
		return ParsedQuery{}, err
	}

	out := ParsedQuery{}
	textTokens := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		switch {
		case strings.HasPrefix(tok, "path:"):
			v := strings.TrimPrefix(tok, "path:")
			if v == "" {
				return ParsedQuery{}, fmt.Errorf("path filter cannot be empty")
			}
			out.PathPrefix = strings.TrimSuffix(v, "*")
		case strings.HasPrefix(tok, "type:"):
			v := strings.TrimPrefix(tok, "type:")
			if v != "file" && v != "dir" && v != "symlink" {
				return ParsedQuery{}, fmt.Errorf("unsupported type filter %q", v)
			}
			out.TypeFilter = v
		case strings.HasPrefix(tok, "size>"):
			v, err := parseInt64Filter(strings.TrimPrefix(tok, "size>"), "size>")
			if err != nil {
				return ParsedQuery{}, err
			}
			out.MinSize = &v
		case strings.HasPrefix(tok, "size<"):
			v, err := parseInt64Filter(strings.TrimPrefix(tok, "size<"), "size<")
			if err != nil {
				return ParsedQuery{}, err
			}
			out.MaxSize = &v
		case strings.HasPrefix(tok, "mtime>"):
			v, err := parseInt64Filter(strings.TrimPrefix(tok, "mtime>"), "mtime>")
			if err != nil {
				return ParsedQuery{}, err
			}
			out.MinMtimeMS = &v
		case strings.HasPrefix(tok, "mtime<"):
			v, err := parseInt64Filter(strings.TrimPrefix(tok, "mtime<"), "mtime<")
			if err != nil {
				return ParsedQuery{}, err
			}
			out.MaxMtimeMS = &v
		case strings.HasPrefix(tok, "ctime>"):
			v, err := parseInt64Filter(strings.TrimPrefix(tok, "ctime>"), "ctime>")
			if err != nil {
				return ParsedQuery{}, err
			}
			out.MinCtimeMS = &v
		case strings.HasPrefix(tok, "ctime<"):
			v, err := parseInt64Filter(strings.TrimPrefix(tok, "ctime<"), "ctime<")
			if err != nil {
				return ParsedQuery{}, err
			}
			out.MaxCtimeMS = &v
		default:
			textTokens = append(textTokens, tok)
		}
	}

	if out.TypeFilter == "" {
		out.TypeFilter = "file"
	}
	if len(textTokens) == 0 {
		out.TextQuery = "*"
	} else {
		out.TextQuery = strings.Join(textTokens, " ")
	}
	return out, nil
}

// BuildFTQuery compiles parsed DSL into a RediSearch query string.
func BuildFTQuery(p ParsedQuery) string {
	parts := []string{fmt.Sprintf("@type:{%s}", escapeTagValue(p.TypeFilter))}

	if p.PathPrefix != "" {
		prefix := strings.TrimSpace(p.PathPrefix)
		if prefix == "" {
			prefix = "/"
		} else if prefix != "/" {
			prefix = strings.TrimSuffix(prefix, "/")
			if prefix == "" {
				prefix = "/"
			}
		}
		parts = append(parts, fmt.Sprintf("@path_ancestors:{%s}", escapeTagValue(prefix)))
	}
	if p.MinSize != nil || p.MaxSize != nil {
		parts = append(parts, numericRange("@size", p.MinSize, p.MaxSize))
	}
	if p.MinMtimeMS != nil || p.MaxMtimeMS != nil {
		parts = append(parts, numericRange("@mtime_ms", p.MinMtimeMS, p.MaxMtimeMS))
	}
	if p.MinCtimeMS != nil || p.MaxCtimeMS != nil {
		parts = append(parts, numericRange("@ctime_ms", p.MinCtimeMS, p.MaxCtimeMS))
	}
	if strings.TrimSpace(p.TextQuery) != "" && p.TextQuery != "*" {
		parts = append(parts, p.TextQuery)
	}

	return strings.Join(parts, " ")
}

func BuildSimpleSearchQuery(text string) string {
	t := strings.TrimSpace(text)
	if t == "" {
		return "@type:{file} *"
	}
	return "@type:{file} " + t
}

func splitTokens(input string) ([]string, error) {
	var out []string
	var b strings.Builder
	inQuote := false
	escaped := false
	for _, r := range input {
		switch {
		case escaped:
			b.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case r == '"':
			inQuote = !inQuote
			b.WriteRune(r)
		case unicode.IsSpace(r) && !inQuote:
			if b.Len() > 0 {
				out = append(out, b.String())
				b.Reset()
			}
		default:
			b.WriteRune(r)
		}
	}
	if escaped || inQuote {
		return nil, fmt.Errorf("unterminated escape or quote in query")
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out, nil
}

func parseInt64Filter(raw, label string) (int64, error) {
	if raw == "" {
		return 0, fmt.Errorf("%s filter cannot be empty", label)
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s value %q", label, raw)
	}
	return v, nil
}

func numericRange(field string, min, max *int64) string {
	minS := "-inf"
	maxS := "+inf"
	if min != nil {
		minS = strconv.FormatInt(*min, 10)
	}
	if max != nil {
		maxS = strconv.FormatInt(*max, 10)
	}
	return fmt.Sprintf("%s:[%s %s]", field, minS, maxS)
}

func escapeTagValue(v string) string {
	repl := strings.NewReplacer(" ", "\\ ", "-", "\\-", ".", "\\.", "/", "\\/", ":", "\\:")
	return repl.Replace(v)
}
