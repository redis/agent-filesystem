package queryvector

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/redis/agent-filesystem/internal/mcptools"
	"github.com/redis/agent-filesystem/internal/queryembedding"
	"github.com/redis/agent-filesystem/internal/queryindex"
	"github.com/redis/agent-filesystem/internal/searchindex"
	"github.com/redis/go-redis/v9"
)

const (
	ProjectionVersion = "2"
	distanceField     = "distance"

	maxEmbeddingBatchInputs          = 96
	maxEmbeddingBatchEstimatedTokens = 200000
	semanticSnippetMaxLines          = 8
)

var ErrSearchUnavailable = errors.New("redis vector search is unavailable")

type SearchOptions struct {
	Path           string
	Limit          int
	All            bool
	MinScore       float64
	CandidateLimit int
	Full           bool
}

type SearchResult struct {
	Results  []mcptools.FileQueryResult
	Stats    SearchStats
	Warnings []string
}

type SearchStats struct {
	Backend         string
	Model           string
	Dimension       int
	SearchAvailable bool
	ChunksScanned   int
	ChunksEmbedded  int
}

type BackfillStats struct {
	Scanned  int
	Embedded int
}

type chunkEmbedding struct {
	Key    string
	Fields map[string]string
	Text   string
	Hash   string
}

func IndexName(fsKey string, provider queryembedding.Provider) string {
	return "afs:qvec:{" + fsKey + "}:" + providerDigest(provider) + ":v" + ProjectionVersion
}

func embeddingField(provider queryembedding.Provider) string {
	return "embedding_" + providerDigest(provider)
}

func Search(ctx context.Context, rdb *redis.Client, fsKey string, provider queryembedding.Provider, query string, opts SearchOptions) (SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return SearchResult{}, fmt.Errorf("semantic query must include text")
	}
	if provider == nil {
		return SearchResult{}, queryembedding.ErrUnavailable
	}
	if rdb == nil {
		return SearchResult{}, ErrSearchUnavailable
	}
	baseStats := SearchStats{
		Model:     provider.Model(),
		Dimension: provider.Dimension(),
	}
	indexedEmbeddings, err := CountIndexedEmbeddings(ctx, rdb, fsKey, provider, opts.Path)
	if err != nil {
		return SearchResult{}, err
	}
	baseStats.ChunksScanned = indexedEmbeddings
	ok, err := EnsureIndex(ctx, rdb, fsKey, provider)
	if err != nil {
		if !isVectorUnavailable(err) {
			return SearchResult{}, err
		}
		ok = false
	}
	if ok {
		results, err := searchRedis(ctx, rdb, fsKey, provider, query, opts)
		if err == nil {
			baseStats.Backend = "redis_vector"
			baseStats.SearchAvailable = true
			return SearchResult{Results: results, Stats: baseStats}, nil
		}
		if !errors.Is(err, ErrSearchUnavailable) && !isVectorUnavailable(err) {
			return SearchResult{}, err
		}
	}
	results, err := searchLocal(ctx, rdb, fsKey, provider, query, opts)
	if err != nil {
		return SearchResult{}, err
	}
	baseStats.Backend = "local_vector"
	return SearchResult{
		Results: results,
		Stats:   baseStats,
		Warnings: []string{
			"Redis vector search is unavailable; using direct semantic ranking over existing embeddings.",
		},
	}, nil
}

func CountIndexedEmbeddings(ctx context.Context, rdb *redis.Client, fsKey string, provider queryembedding.Provider, scopePath string) (int, error) {
	if provider == nil {
		return 0, queryembedding.ErrUnavailable
	}
	fieldName := embeddingField(provider)
	count := 0
	err := scanChunks(ctx, rdb, fsKey, scopePath, func(key string, fields map[string]string) error {
		raw := []byte(fields[fieldName])
		if len(queryembedding.DecodeFloat32(raw)) == provider.Dimension() &&
			fields["embedding_model"] == provider.Model() &&
			fields["embedding_provider"] == provider.Name() &&
			fields["embedding_dim"] == strconv.Itoa(provider.Dimension()) &&
			fields["embedding_projection_ver"] == ProjectionVersion {
			count++
		}
		return nil
	})
	return count, err
}

func EnsureIndex(ctx context.Context, rdb *redis.Client, fsKey string, provider queryembedding.Provider) (bool, error) {
	if rdb == nil {
		return false, nil
	}
	searchRDB, closeSearch := newSearchRedisClient(rdb)
	defer closeSearch()
	indexName := IndexName(fsKey, provider)
	fieldName := embeddingField(provider)
	if _, err := searchRDB.FTInfo(ctx, indexName).Result(); err != nil {
		switch {
		case isVectorUnavailable(err):
			return false, nil
		case !isUnknownSearchIndex(err):
			return false, err
		}
		_, err = searchRDB.FTCreate(ctx, indexName, &redis.FTCreateOptions{
			OnHash:    true,
			Prefix:    []interface{}{queryindex.ChunkPrefix(fsKey)},
			NoOffsets: true,
			NoHL:      true,
			NoFreqs:   true,
		},
			&redis.FieldSchema{FieldName: "type", FieldType: redis.SearchFieldTypeTag},
			&redis.FieldSchema{FieldName: "path", FieldType: redis.SearchFieldTypeTag},
			&redis.FieldSchema{FieldName: "path_ancestors", FieldType: redis.SearchFieldTypeTag, Separator: ","},
			&redis.FieldSchema{FieldName: "inode_id", FieldType: redis.SearchFieldTypeTag},
			&redis.FieldSchema{FieldName: "content_hash", FieldType: redis.SearchFieldTypeTag},
			&redis.FieldSchema{FieldName: "seq", FieldType: redis.SearchFieldTypeNumeric},
			&redis.FieldSchema{FieldName: "start_line", FieldType: redis.SearchFieldTypeNumeric},
			&redis.FieldSchema{FieldName: "end_line", FieldType: redis.SearchFieldTypeNumeric},
			&redis.FieldSchema{FieldName: "embedding_provider", FieldType: redis.SearchFieldTypeTag},
			&redis.FieldSchema{FieldName: "embedding_model", FieldType: redis.SearchFieldTypeTag},
			&redis.FieldSchema{FieldName: "embedding_dim", FieldType: redis.SearchFieldTypeNumeric},
			&redis.FieldSchema{FieldName: "embedding_projection_ver", FieldType: redis.SearchFieldTypeTag},
			&redis.FieldSchema{
				FieldName: fieldName,
				FieldType: redis.SearchFieldTypeVector,
				VectorArgs: &redis.FTVectorArgs{FlatOptions: &redis.FTFlatOptions{
					Type:           "FLOAT32",
					Dim:            provider.Dimension(),
					DistanceMetric: "COSINE",
				}},
			},
		).Result()
		if err != nil {
			switch {
			case isVectorUnavailable(err):
				return false, nil
			case isIndexAlreadyExists(err):
			default:
				return false, err
			}
		}
	}
	return true, nil
}

func Backfill(ctx context.Context, rdb *redis.Client, fsKey string, provider queryembedding.Provider, opts SearchOptions) (BackfillStats, error) {
	stats := BackfillStats{}
	if provider == nil {
		return stats, queryembedding.ErrUnavailable
	}
	if err := queryindex.EnsureReady(ctx, rdb, fsKey, opts.Path, opts.CandidateLimit); err != nil {
		return stats, err
	}
	if _, err := EnsureIndex(ctx, rdb, fsKey, provider); err != nil && !isVectorUnavailable(err) {
		return stats, err
	}
	fieldName := embeddingField(provider)
	pending := make([]chunkEmbedding, 0)
	if err := scanChunks(ctx, rdb, fsKey, opts.Path, func(key string, fields map[string]string) error {
		stats.Scanned++
		text := fields["text"]
		formatted := queryembedding.FormatDocument(text, queryembedding.TitleFromPath(fields["path"]), provider.Model())
		hash := embeddingHash(provider, formatted)
		if fields["embedding_model"] == provider.Model() &&
			fields["embedding_provider"] == provider.Name() &&
			fields["embedding_dim"] == strconv.Itoa(provider.Dimension()) &&
			fields["embedding_hash"] == hash &&
			fields["embedding_projection_ver"] == ProjectionVersion &&
			fields[fieldName] != "" {
			return nil
		}
		pending = append(pending, chunkEmbedding{Key: key, Fields: fields, Text: formatted, Hash: hash})
		return nil
	}); err != nil {
		return stats, err
	}
	if len(pending) == 0 {
		return stats, nil
	}
	embedded, err := embedAndStorePendingChunks(ctx, rdb, fieldName, provider, pending)
	if err != nil {
		stats.Embedded += embedded
		return stats, err
	}
	stats.Embedded += embedded
	return stats, nil
}

func embedAndStorePendingChunks(ctx context.Context, rdb *redis.Client, fieldName string, provider queryembedding.Provider, pending []chunkEmbedding) (int, error) {
	embedded := 0
	for start := 0; start < len(pending); {
		end := nextEmbeddingBatchEnd(pending, start)
		texts := make([]string, 0, end-start)
		for _, item := range pending[start:end] {
			texts = append(texts, item.Text)
		}
		vectors, err := provider.EmbedBatch(ctx, texts)
		if err != nil {
			return embedded, err
		}
		if len(vectors) != len(texts) {
			return embedded, fmt.Errorf("embedding provider returned %d vectors, want %d", len(vectors), len(texts))
		}
		if err := storeChunkEmbeddingBatch(ctx, rdb, fieldName, provider, pending[start:end], vectors); err != nil {
			return embedded, err
		}
		embedded += len(vectors)
		start = end
	}
	return embedded, nil
}

func storeChunkEmbeddingBatch(ctx context.Context, rdb *redis.Client, fieldName string, provider queryembedding.Provider, pending []chunkEmbedding, vectors [][]float32) error {
	now := time.Now().UTC().UnixMilli()
	for i, item := range pending {
		vec := vectors[i]
		if len(vec) != provider.Dimension() {
			return fmt.Errorf("embedding dimension = %d, want %d", len(vec), provider.Dimension())
		}
		if err := rdb.HSet(ctx, item.Key, map[string]interface{}{
			fieldName:                  queryembedding.EncodeFloat32(vec),
			"embedding_provider":       provider.Name(),
			"embedding_model":          provider.Model(),
			"embedding_dim":            provider.Dimension(),
			"embedding_hash":           item.Hash,
			"embedding_indexed_at_ms":  now,
			"embedding_projection_ver": ProjectionVersion,
		}).Err(); err != nil {
			return err
		}
	}
	return nil
}

func nextEmbeddingBatchEnd(pending []chunkEmbedding, start int) int {
	end := start
	estimatedTokens := 0
	for end < len(pending) {
		itemTokens := estimateEmbeddingTokens(pending[end].Text)
		if end > start && (end-start >= maxEmbeddingBatchInputs || estimatedTokens+itemTokens > maxEmbeddingBatchEstimatedTokens) {
			break
		}
		estimatedTokens += itemTokens
		end++
	}
	if end == start {
		return start + 1
	}
	return end
}

func estimateEmbeddingTokens(text string) int {
	if text == "" {
		return 1
	}
	tokens := (len(text) + 3) / 4
	if tokens < 1 {
		return 1
	}
	return tokens
}

func searchRedis(ctx context.Context, rdb *redis.Client, fsKey string, provider queryembedding.Provider, query string, opts SearchOptions) ([]mcptools.FileQueryResult, error) {
	limit := semanticLimit(opts)
	queryVec, err := provider.Embed(ctx, queryembedding.FormatQuery(query, provider.Model()))
	if err != nil {
		return nil, err
	}
	searchRDB, closeSearch := newSearchRedisClient(rdb)
	defer closeSearch()
	fieldName := embeddingField(provider)
	result, err := searchRDB.FTSearchWithArgs(ctx, IndexName(fsKey, provider), buildVectorQuery(opts.Path, limit, provider, fieldName), &redis.FTSearchOptions{
		Return: []redis.FTSearchReturn{
			{FieldName: "path"},
			{FieldName: "inode_id"},
			{FieldName: "content_hash"},
			{FieldName: "seq"},
			{FieldName: "start_line"},
			{FieldName: "end_line"},
			{FieldName: "text"},
			{FieldName: "preview"},
			{FieldName: distanceField},
		},
		Params:         map[string]interface{}{"vec": queryembedding.EncodeFloat32(queryVec)},
		SortBy:         []redis.FTSearchSortBy{{FieldName: distanceField, Asc: true}},
		LimitOffset:    0,
		Limit:          limit,
		DialectVersion: 2,
	}).Result()
	if err != nil {
		if isVectorUnavailable(err) {
			return nil, ErrSearchUnavailable
		}
		return nil, err
	}
	out := make([]mcptools.FileQueryResult, 0, len(result.Docs))
	for _, doc := range result.Docs {
		distance, _ := strconv.ParseFloat(doc.Fields[distanceField], 64)
		score := semanticScoreFromDistance(distance)
		if score < opts.MinScore {
			continue
		}
		chunkStartLine := atoi(doc.Fields["start_line"])
		chunkEndLine := atoi(doc.Fields["end_line"])
		snippet, startLine, endLine := semanticSnippet(doc.Fields["text"], chunkStartLine, chunkEndLine, opts.Full)
		out = append(out, mcptools.FileQueryResult{
			Path:        doc.Fields["path"],
			ChunkID:     doc.ID,
			StartLine:   startLine,
			EndLine:     endLine,
			Score:       score,
			Snippet:     snippet,
			SearchTypes: []string{mcptools.FileQuerySearchVec},
			Metadata: map[string]any{
				"backend":          "redis_vector",
				"distance":         distance,
				"model":            provider.Model(),
				"provider":         provider.Name(),
				"inode_id":         doc.Fields["inode_id"],
				"content_hash":     doc.Fields["content_hash"],
				"seq":              atoi(doc.Fields["seq"]),
				"chunk_start_line": chunkStartLine,
				"chunk_end_line":   chunkEndLine,
			},
		})
	}
	sortResults(out)
	if !opts.All && opts.Limit >= 0 && len(out) > opts.Limit {
		out = out[:opts.Limit]
	}
	return out, nil
}

func searchLocal(ctx context.Context, rdb *redis.Client, fsKey string, provider queryembedding.Provider, query string, opts SearchOptions) ([]mcptools.FileQueryResult, error) {
	queryVec, err := provider.Embed(ctx, queryembedding.FormatQuery(query, provider.Model()))
	if err != nil {
		return nil, err
	}
	out := make([]mcptools.FileQueryResult, 0)
	fieldName := embeddingField(provider)
	if err := scanChunks(ctx, rdb, fsKey, opts.Path, func(key string, fields map[string]string) error {
		raw := []byte(fields[fieldName])
		vec := queryembedding.DecodeFloat32(raw)
		if len(vec) != provider.Dimension() {
			return nil
		}
		score := queryembedding.Cosine(queryVec, vec)
		if score <= 0 || score < opts.MinScore {
			return nil
		}
		chunkStartLine := atoi(fields["start_line"])
		chunkEndLine := atoi(fields["end_line"])
		snippet, startLine, endLine := semanticSnippet(fields["text"], chunkStartLine, chunkEndLine, opts.Full)
		out = append(out, mcptools.FileQueryResult{
			Path:        fields["path"],
			ChunkID:     key,
			StartLine:   startLine,
			EndLine:     endLine,
			Score:       score,
			Snippet:     snippet,
			SearchTypes: []string{mcptools.FileQuerySearchVec},
			Metadata: map[string]any{
				"backend":          "local_vector",
				"model":            provider.Model(),
				"provider":         provider.Name(),
				"inode_id":         fields["inode_id"],
				"content_hash":     fields["content_hash"],
				"seq":              atoi(fields["seq"]),
				"chunk_start_line": chunkStartLine,
				"chunk_end_line":   chunkEndLine,
			},
		})
		return nil
	}); err != nil {
		return nil, err
	}
	sortResults(out)
	if !opts.All && opts.Limit >= 0 && len(out) > opts.Limit {
		out = out[:opts.Limit]
	}
	return out, nil
}

func semanticSnippet(text string, startLine, endLine int, full bool) (string, int, int) {
	text = strings.TrimRight(text, "\n")
	if full {
		return text, startLine, endLine
	}
	lines := strings.Split(text, "\n")
	if len(lines) == 1 && strings.TrimSpace(lines[0]) == "" {
		return "", startLine, startLine
	}
	if len(lines) > semanticSnippetMaxLines {
		lines = append([]string(nil), lines[:semanticSnippetMaxLines]...)
		lines[len(lines)-1] = strings.TrimRight(lines[len(lines)-1], " \t") + " ..."
	}
	snippet := strings.TrimRight(strings.Join(lines, "\n"), "\n")
	snippet = truncateUTF8(snippet, queryindex.SnippetMaxBytes)
	lineCount := len(strings.Split(snippet, "\n"))
	if lineCount <= 0 {
		lineCount = 1
	}
	return snippet, startLine, startLine + lineCount - 1
}

func buildVectorQuery(scopePath string, limit int, provider queryembedding.Provider, fieldName string) string {
	filter := "@type:{chunk}"
	filter += " @embedding_provider:{" + searchindex.EscapeTagValue(provider.Name()) + "}"
	filter += " @embedding_model:{" + searchindex.EscapeTagValue(provider.Model()) + "}"
	filter += " @embedding_dim:[" + strconv.Itoa(provider.Dimension()) + " " + strconv.Itoa(provider.Dimension()) + "]"
	filter += " @embedding_projection_ver:{" + searchindex.EscapeTagValue(ProjectionVersion) + "}"
	scopePath = normalizePath(scopePath)
	if scopePath != "/" {
		filter += " @path_ancestors:{" + searchindex.EscapeTagValue(scopePath) + "}"
	}
	return fmt.Sprintf("(%s)=>[KNN %d @%s $vec AS %s]", filter, limit, fieldName, distanceField)
}

func semanticLimit(opts SearchOptions) int {
	limit := opts.Limit
	if opts.All {
		limit = 1000
	} else if limit <= 0 {
		limit = 10
	}
	if opts.CandidateLimit > 0 && opts.CandidateLimit > limit {
		limit = opts.CandidateLimit
	}
	if limit <= 0 {
		return 10
	}
	return limit
}

func semanticScoreFromDistance(distance float64) float64 {
	if math.IsNaN(distance) || math.IsInf(distance, 0) {
		return 0
	}
	score := 1 - distance
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

func sortResults(results []mcptools.FileQueryResult) {
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			if results[i].Path == results[j].Path {
				return results[i].StartLine < results[j].StartLine
			}
			return results[i].Path < results[j].Path
		}
		return results[i].Score > results[j].Score
	})
}

func scanChunks(ctx context.Context, rdb *redis.Client, fsKey, scopePath string, fn func(string, map[string]string) error) error {
	scopePath = normalizePath(scopePath)
	pattern := queryindex.ChunkPrefix(fsKey) + "*"
	var cursor uint64
	for {
		keys, next, err := rdb.Scan(ctx, cursor, pattern, 500).Result()
		if err != nil {
			return err
		}
		sort.Strings(keys)
		for _, key := range keys {
			fields, err := rdb.HGetAll(ctx, key).Result()
			if err != nil {
				return err
			}
			if fields["type"] != "chunk" {
				continue
			}
			if !pathInScope(fields["path"], scopePath) {
				continue
			}
			if err := fn(key, fields); err != nil {
				return err
			}
		}
		cursor = next
		if cursor == 0 {
			return nil
		}
	}
}

func embeddingHash(provider queryembedding.Provider, text string) string {
	sum := sha256.Sum256([]byte(provider.Name() + "\x00" + provider.Model() + "\x00" + strconv.Itoa(provider.Dimension()) + "\x00" + text))
	return hex.EncodeToString(sum[:])
}

func providerDigest(provider queryembedding.Provider) string {
	if provider == nil {
		return "none"
	}
	sum := sha256.Sum256([]byte(provider.Name() + "\x00" + provider.Model() + "\x00" + strconv.Itoa(provider.Dimension())))
	return hex.EncodeToString(sum[:8])
}

func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	clean := path.Clean(p)
	if clean == "." {
		return "/"
	}
	return clean
}

func pathInScope(filePath, scopePath string) bool {
	filePath = normalizePath(filePath)
	scopePath = normalizePath(scopePath)
	if scopePath == "/" {
		return true
	}
	return filePath == scopePath || strings.HasPrefix(filePath, strings.TrimSuffix(scopePath, "/")+"/")
}

func truncateUTF8(text string, maxBytes int) string {
	if len(text) <= maxBytes {
		return text
	}
	if maxBytes <= 3 {
		return strings.Repeat(".", maxBytes)
	}
	cut := maxBytes - 3
	for cut > 0 && !utf8.ValidString(text[:cut]) {
		cut--
	}
	return text[:cut] + "..."
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func isVectorUnavailable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unknown command") ||
		strings.Contains(msg, "module") ||
		strings.Contains(msg, "not supported") ||
		strings.Contains(msg, "unsupported") ||
		strings.Contains(msg, "db != 0") ||
		strings.Contains(msg, "vector") ||
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
