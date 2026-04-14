package searchindex

import (
	"bytes"
	"encoding/hex"
	"sort"
	"strings"
	"unicode"
)

const (
	GramSize        = 3
	MaxIndexedBytes = 256 << 10
	MaxUniqueGrams  = 16384

	StateReady  = "ready"
	StateBinary = "binary"
	StateLarge  = "large"
)

type FileFields struct {
	SearchState string
	GrepGramsCI string
}

func IndexName(fsKey string) string {
	return "afs:idx:{" + fsKey + "}:v1"
}

func ReadyKey(fsKey string) string {
	return "afs:{" + fsKey + "}:search_index_v1"
}

func BuildFileFields(data []byte) FileFields {
	switch {
	case IsBinaryPrefix(data):
		return FileFields{SearchState: StateBinary}
	case len(data) > MaxIndexedBytes:
		return FileFields{SearchState: StateLarge}
	default:
		return FileFields{
			SearchState: StateReady,
			GrepGramsCI: strings.Join(gramTerms(bytes.ToLower(data)), " "),
		}
	}
}

func QueryTermsForLiteral(literal string) []string {
	if len(literal) < GramSize {
		return nil
	}
	return gramTerms(bytes.ToLower([]byte(literal)))
}

func IsBinaryPrefix(data []byte) bool {
	checkLen := len(data)
	if checkLen > 8192 {
		checkLen = 8192
	}
	return bytes.IndexByte(data[:checkLen], '\x00') >= 0
}

func EscapeTagValue(value string) string {
	if value == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(value) * 2)
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('\\')
		b.WriteRune(r)
	}
	return b.String()
}

func gramTerms(data []byte) []string {
	if len(data) < GramSize {
		return nil
	}

	seen := make(map[string]struct{}, 256)
	terms := make([]string, 0, 256)
	for i := 0; i+GramSize <= len(data) && len(terms) < MaxUniqueGrams; i++ {
		term := "g" + hex.EncodeToString(data[i:i+GramSize])
		if _, ok := seen[term]; ok {
			continue
		}
		seen[term] = struct{}{}
		terms = append(terms, term)
	}
	sort.Strings(terms)
	return terms
}
