package searchindex

import (
	"bytes"
	"context"
	"encoding/hex"
	"sort"
	"strings"
	"unicode"

	"github.com/redis/go-redis/v9"
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

func EnsureIndex(ctx context.Context, rdb *redis.Client, fsKey string) (bool, error) {
	if rdb == nil {
		return false, nil
	}
	indexName := IndexName(fsKey)
	searchRDB, closeSearch := newSearchRedisClient(rdb)
	defer closeSearch()
	if _, err := searchRDB.FTInfo(ctx, indexName).Result(); err != nil {
		switch {
		case isSearchUnavailable(err):
			return false, nil
		case !isUnknownSearchIndex(err):
			return false, err
		}
		_, err = searchRDB.FTCreate(ctx, indexName, &redis.FTCreateOptions{
			OnHash:    true,
			Prefix:    []interface{}{"afs:{" + fsKey + "}:inode:"},
			NoOffsets: true,
			NoHL:      true,
			NoFreqs:   true,
		},
			&redis.FieldSchema{FieldName: "type", FieldType: redis.SearchFieldTypeTag},
			&redis.FieldSchema{FieldName: "path", FieldType: redis.SearchFieldTypeTag},
			&redis.FieldSchema{FieldName: "path_ancestors", FieldType: redis.SearchFieldTypeTag, Separator: ","},
			&redis.FieldSchema{FieldName: "search_state", FieldType: redis.SearchFieldTypeTag},
			&redis.FieldSchema{FieldName: "grep_grams_ci", FieldType: redis.SearchFieldTypeText, NoStem: true},
			&redis.FieldSchema{FieldName: "size", FieldType: redis.SearchFieldTypeNumeric},
			&redis.FieldSchema{FieldName: "mtime_ms", FieldType: redis.SearchFieldTypeNumeric},
		).Result()
		if err != nil {
			switch {
			case isSearchUnavailable(err):
				return false, nil
			case isIndexAlreadyExists(err):
			default:
				return false, err
			}
		}
	}
	return true, nil
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

func isSearchUnavailable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unknown command") ||
		strings.Contains(msg, "module") ||
		strings.Contains(msg, "not supported") ||
		strings.Contains(msg, "resp3 responses for this command are disabled")
}

func isUnknownSearchIndex(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unknown index") ||
		strings.Contains(msg, "no such index") ||
		strings.Contains(msg, "index does not exist")
}

func isIndexAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "index already exists") ||
		strings.Contains(msg, "already exists")
}

func newSearchRedisClient(base *redis.Client) (*redis.Client, func()) {
	if base == nil {
		return nil, func() {}
	}
	opts := *base.Options()
	opts.Protocol = 2
	opts.UnstableResp3 = false
	client := redis.NewClient(&opts)
	return client, func() { _ = client.Close() }
}
