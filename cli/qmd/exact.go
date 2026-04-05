package qmd

import (
	"context"
	"strings"
)

const exactPhraseChunkSize = 200

// SearchParsed runs a parsed DSL query. For a pure quoted phrase, it keeps the
// RediSearch filters but verifies the exact substring match in Go to avoid
// false negatives on long markdown-style documents.
func (c *Client) SearchParsed(ctx context.Context, parsed ParsedQuery, opts QueryOptions) (int64, []SearchHit, error) {
	if phrase, ok := exactPhraseText(parsed.TextQuery); ok {
		return c.searchExactPhrase(ctx, parsed, phrase, opts)
	}
	return c.Search(ctx, BuildFTQuery(parsed), opts)
}

func exactPhraseText(text string) (string, bool) {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) < 2 || trimmed[0] != '"' || trimmed[len(trimmed)-1] != '"' {
		return "", false
	}
	inner := trimmed[1 : len(trimmed)-1]
	if inner == "" || strings.ContainsRune(inner, '"') {
		return "", false
	}
	return inner, true
}

func exactPhraseCandidateQuery(parsed ParsedQuery, phrase string) string {
	candidate := parsed
	if candidate.PathPrefix != "" {
		candidate.TextQuery = "*"
	} else {
		candidate.TextQuery = phrase
	}
	return BuildFTQuery(candidate)
}

func filterExactPhraseHits(hits []SearchHit, phrase string) []SearchHit {
	if phrase == "" {
		return append([]SearchHit(nil), hits...)
	}
	filtered := make([]SearchHit, 0, len(hits))
	for _, hit := range hits {
		if strings.Contains(hit.Content, phrase) {
			filtered = append(filtered, hit)
		}
	}
	return filtered
}

func sliceHits(hits []SearchHit, opts QueryOptions) []SearchHit {
	if opts.Offset < 0 {
		opts.Offset = 0
	}
	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	if opts.Offset >= len(hits) {
		return nil
	}
	end := opts.Offset + opts.Limit
	if end > len(hits) {
		end = len(hits)
	}
	return hits[opts.Offset:end]
}

func (c *Client) searchExactPhrase(
	ctx context.Context,
	parsed ParsedQuery,
	phrase string,
	opts QueryOptions,
) (int64, []SearchHit, error) {
	if opts.Limit <= 0 {
		opts.Limit = 20
	}

	query := exactPhraseCandidateQuery(parsed, phrase)
	chunkSize := opts.Limit
	if chunkSize < exactPhraseChunkSize {
		chunkSize = exactPhraseChunkSize
	}

	filtered := make([]SearchHit, 0, opts.Limit)
	offset := 0
	for {
		total, hits, err := c.searchExactPhraseCandidates(ctx, query, QueryOptions{
			Limit:  chunkSize,
			Offset: offset,
		})
		if err != nil {
			return 0, nil, err
		}
		if len(hits) == 0 {
			break
		}
		filtered = append(filtered, filterExactPhraseHits(hits, phrase)...)
		offset += len(hits)
		if offset >= int(total) {
			break
		}
	}

	return int64(len(filtered)), sliceHits(filtered, opts), nil
}

func (c *Client) searchExactPhraseCandidates(
	ctx context.Context,
	query string,
	opts QueryOptions,
) (int64, []SearchHit, error) {
	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	if opts.Offset < 0 {
		opts.Offset = 0
	}
	args := []interface{}{
		"FT.SEARCH", c.indexName, query,
		"WITHSCORES",
		"SORTBY", "path", "ASC",
		"RETURN", "6", "path", "type", "content", "size", "mtime_ms", "ctime_ms",
		"LIMIT", opts.Offset, opts.Limit,
	}
	res, err := c.rdb.Do(ctx, args...).Result()
	if err != nil {
		return 0, nil, err
	}
	return parseSearchReply(res)
}
