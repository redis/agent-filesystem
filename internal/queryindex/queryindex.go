package queryindex

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/redis/agent-filesystem/internal/mcptools"
	"github.com/redis/agent-filesystem/internal/rediscontent"
	"github.com/redis/agent-filesystem/internal/searchindex"
	"github.com/redis/go-redis/v9"
)

const (
	MaxIndexedFileBytes = 1 << 20
	MaxChunkBytes       = 8 << 10
	MaxChunkLines       = 120
	ProjectionVersion   = "3"

	StateStale       = "stale"
	StateReady       = "ready"
	StateSkipped     = "skipped"
	StateError       = "error"
	SkipEmpty        = "empty"
	SkipLarge        = "large"
	SkipBinary       = "binary"
	SkipUnsupported  = "unsupported_format"
	SkipMissing      = "missing"
	defaultBatchSize = 128
	defaultInterval  = 2 * time.Second
)

type Chunk struct {
	Key         string
	Path        string
	InodeID     string
	ContentHash string
	Seq         int
	StartLine   int
	EndLine     int
	Text        string
	Preview     string
}

type ReindexResult struct {
	InodeID string
	Path    string
	State   string
	Reason  string
	Chunks  int
	Bytes   int64
}

type ProcessResult struct {
	Processed int `json:"processed"`
	Indexed   int `json:"indexed"`
	Skipped   int `json:"skipped"`
	Deleted   int `json:"deleted"`
	Errors    int `json:"errors"`
	Pending   int `json:"pending"`
}

type Status struct {
	IndexName       string `json:"index_name"`
	State           string `json:"state"`
	SearchAvailable bool   `json:"search_available"`
	Files           int    `json:"files"`
	Ready           int    `json:"ready"`
	Pending         int    `json:"pending"`
	Stale           int    `json:"stale"`
	Skipped         int    `json:"skipped"`
	Errors          int    `json:"errors"`
	Unindexed       int    `json:"unindexed"`
	Chunks          int    `json:"chunks"`
}

type RebuildOptions struct {
	Path       string
	Force      bool
	Wait       bool
	BatchSize  int
	MaxBatches int
}

type RebuildResult struct {
	Enqueued int           `json:"enqueued"`
	Waited   bool          `json:"waited"`
	Process  ProcessResult `json:"process,omitempty"`
	Status   Status        `json:"status"`
}

type SearchOptions struct {
	Path           string
	Limit          int
	All            bool
	MinScore       float64
	CandidateLimit int
	Full           bool
}

var ErrSearchUnavailable = errors.New("redis search is unavailable")
var ErrProjectionStale = errors.New("query projection is stale")

func IndexName(fsKey string) string {
	return "afs:qidx:{" + fsKey + "}:v" + ProjectionVersion
}

func ChunkPrefix(fsKey string) string {
	return "afs:{" + fsKey + "}:qchunk:"
}

func ChunkKey(fsKey, inodeID, contentHash string, seq int) string {
	return fmt.Sprintf("%s%s:%s:%06d", ChunkPrefix(fsKey), inodeID, contentHash, seq)
}

func ChunkSetKey(fsKey, inodeID string) string {
	return "afs:{" + fsKey + "}:qchunks:" + inodeID
}

func DirtySetKey(fsKey string) string {
	return "afs:{" + fsKey + "}:query_index:dirty"
}

func ReadyKey(fsKey string) string {
	return "afs:{" + fsKey + "}:query_index_v" + ProjectionVersion
}

func InodeKey(fsKey, inodeID string) string {
	return "afs:{" + fsKey + "}:inode:" + inodeID
}

func ContentKey(fsKey, inodeID string) string {
	return "afs:{" + fsKey + "}:content:" + inodeID
}

func StaleFields() map[string]interface{} {
	return map[string]interface{}{
		"query_state":         StateStale,
		"query_index_version": ProjectionVersion,
		"query_skip_reason":   "",
		"query_error":         "",
		"query_content_hash":  "",
	}
}

func QueueMarkDirty(ctx context.Context, pipe redis.Pipeliner, fsKey, inodeID string) {
	inodeID = strings.TrimSpace(inodeID)
	if inodeID == "" {
		return
	}
	pipe.HSet(ctx, InodeKey(fsKey, inodeID), StaleFields())
	pipe.SAdd(ctx, DirtySetKey(fsKey), inodeID)
}

func QueueMarkDeleted(ctx context.Context, pipe redis.Pipeliner, fsKey, inodeID string) {
	inodeID = strings.TrimSpace(inodeID)
	if inodeID == "" {
		return
	}
	pipe.SAdd(ctx, DirtySetKey(fsKey), inodeID)
}

func MarkDirty(ctx context.Context, rdb *redis.Client, fsKey, inodeID string) error {
	pipe := rdb.Pipeline()
	QueueMarkDirty(ctx, pipe, fsKey, inodeID)
	_, err := pipe.Exec(ctx)
	if errors.Is(err, redis.Nil) {
		return nil
	}
	return err
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
			Prefix:    []interface{}{ChunkPrefix(fsKey)},
			NoOffsets: false,
			NoHL:      true,
		},
			&redis.FieldSchema{FieldName: "type", FieldType: redis.SearchFieldTypeTag},
			&redis.FieldSchema{FieldName: "path", FieldType: redis.SearchFieldTypeTag},
			&redis.FieldSchema{FieldName: "path_ancestors", FieldType: redis.SearchFieldTypeTag, Separator: ","},
			&redis.FieldSchema{FieldName: "inode_id", FieldType: redis.SearchFieldTypeTag},
			&redis.FieldSchema{FieldName: "content_hash", FieldType: redis.SearchFieldTypeTag},
			&redis.FieldSchema{FieldName: "seq", FieldType: redis.SearchFieldTypeNumeric},
			&redis.FieldSchema{FieldName: "start_line", FieldType: redis.SearchFieldTypeNumeric},
			&redis.FieldSchema{FieldName: "end_line", FieldType: redis.SearchFieldTypeNumeric},
			&redis.FieldSchema{FieldName: "text", FieldType: redis.SearchFieldTypeText},
			&redis.FieldSchema{FieldName: "preview", FieldType: redis.SearchFieldTypeText, NoStem: true},
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
	_ = rdb.HSet(ctx, ReadyKey(fsKey), "state", StateReady, "projection_version", ProjectionVersion, "updated_at_ms", time.Now().UTC().UnixMilli()).Err()
	return true, nil
}

func ResetWorkspace(ctx context.Context, rdb *redis.Client, fsKey string) error {
	patterns := []string{
		ChunkPrefix(fsKey) + "*",
		"afs:{" + fsKey + "}:qchunks:*",
	}
	for _, pattern := range patterns {
		var cursor uint64
		for {
			keys, next, err := rdb.Scan(ctx, cursor, pattern, 500).Result()
			if err != nil {
				return err
			}
			if len(keys) > 0 {
				if err := rdb.Del(ctx, keys...).Err(); err != nil {
					return err
				}
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
	}
	return rdb.Del(ctx, DirtySetKey(fsKey), ReadyKey(fsKey)).Err()
}

func QueueResetWorkspace(ctx context.Context, pipe redis.Pipeliner, fsKey string) {
	pipe.Del(ctx, DirtySetKey(fsKey), ReadyKey(fsKey))
}

func ProcessPending(ctx context.Context, rdb *redis.Client, fsKey string, limit int) (ProcessResult, error) {
	if limit <= 0 {
		limit = defaultBatchSize
	}
	ids, err := scanDirtyInodes(ctx, rdb, fsKey, limit)
	if err != nil {
		return ProcessResult{}, err
	}
	result := ProcessResult{}
	for _, inodeID := range ids {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		item, err := ReindexFile(ctx, rdb, fsKey, inodeID)
		if err != nil {
			result.Errors++
			_ = rdb.HSet(ctx, InodeKey(fsKey, inodeID), "query_state", StateError, "query_error", err.Error()).Err()
			continue
		}
		result.Processed++
		switch item.State {
		case StateReady:
			result.Indexed++
		case StateSkipped:
			if item.Reason == SkipMissing {
				result.Deleted++
			} else {
				result.Skipped++
			}
		}
	}
	pending, _ := PendingCount(ctx, rdb, fsKey)
	result.Pending = pending
	return result, nil
}

func Inspect(ctx context.Context, rdb *redis.Client, fsKey, scopePath string) (Status, error) {
	status := Status{
		IndexName: IndexName(fsKey),
		State:     "missing",
	}
	if rdb == nil {
		status.State = "unavailable"
		return status, nil
	}
	ok, err := EnsureIndex(ctx, rdb, fsKey)
	if err != nil {
		return status, err
	}
	status.SearchAvailable = ok
	if !ok {
		status.State = "unavailable"
	}
	pending, err := PendingCount(ctx, rdb, fsKey)
	if err != nil {
		return status, err
	}
	status.Pending = pending

	if err := scanFiles(ctx, rdb, fsKey, scopePath, func(fields map[string]string) error {
		status.Files++
		status.Chunks += atoi(fields["query_chunk_count"])
		version := strings.TrimSpace(fields["query_index_version"])
		switch strings.TrimSpace(fields["query_state"]) {
		case StateReady:
			if version == ProjectionVersion {
				status.Ready++
			} else {
				status.Unindexed++
			}
		case StateStale:
			status.Stale++
		case StateSkipped:
			if version == ProjectionVersion {
				status.Skipped++
			} else {
				status.Unindexed++
			}
		case StateError:
			status.Errors++
		default:
			status.Unindexed++
		}
		return nil
	}); err != nil {
		return status, err
	}

	switch {
	case !status.SearchAvailable:
		status.State = "unavailable"
	case status.Pending > 0 || status.Stale > 0:
		status.State = "indexing"
	case status.Errors > 0:
		status.State = StateError
	case status.Unindexed > 0:
		status.State = "needs_rebuild"
	default:
		status.State = StateReady
	}
	return status, nil
}

func Rebuild(ctx context.Context, rdb *redis.Client, fsKey string, opts RebuildOptions) (RebuildResult, error) {
	if rdb == nil {
		return RebuildResult{}, ErrSearchUnavailable
	}
	enqueued, err := EnqueueFiles(ctx, rdb, fsKey, opts.Path, opts.Force)
	if err != nil {
		return RebuildResult{}, err
	}
	result := RebuildResult{Enqueued: enqueued, Waited: opts.Wait}
	if opts.Wait {
		batchSize := opts.BatchSize
		if batchSize <= 0 {
			batchSize = defaultBatchSize
		}
		maxBatches := opts.MaxBatches
		if maxBatches <= 0 {
			maxBatches = 1000000
		}
		for i := 0; i < maxBatches; i++ {
			batch, err := ProcessPending(ctx, rdb, fsKey, batchSize)
			if err != nil {
				return result, err
			}
			result.Process.Processed += batch.Processed
			result.Process.Indexed += batch.Indexed
			result.Process.Skipped += batch.Skipped
			result.Process.Deleted += batch.Deleted
			result.Process.Errors += batch.Errors
			result.Process.Pending = batch.Pending
			if batch.Pending == 0 || batch.Processed == 0 {
				break
			}
		}
	}
	status, err := Inspect(ctx, rdb, fsKey, opts.Path)
	if err != nil {
		return result, err
	}
	result.Status = status
	return result, nil
}

func EnqueueFiles(ctx context.Context, rdb *redis.Client, fsKey, scopePath string, force bool) (int, error) {
	count := 0
	pipe := rdb.Pipeline()
	queued := 0
	flush := func() error {
		if queued == 0 {
			return nil
		}
		_, err := pipe.Exec(ctx)
		pipe = rdb.Pipeline()
		queued = 0
		if errors.Is(err, redis.Nil) {
			return nil
		}
		return err
	}
	err := scanFiles(ctx, rdb, fsKey, scopePath, func(fields map[string]string) error {
		state := strings.TrimSpace(fields["query_state"])
		version := strings.TrimSpace(fields["query_index_version"])
		if !force && state == StateReady && version == ProjectionVersion {
			return nil
		}
		QueueMarkDirty(ctx, pipe, fsKey, strings.TrimSpace(fields["id"]))
		count++
		queued += 2
		if queued >= 1000 {
			return flush()
		}
		return nil
	})
	if err != nil {
		return count, err
	}
	if err := flush(); err != nil {
		return count, err
	}
	if err := rdb.HSet(ctx, ReadyKey(fsKey), "backfill_checked_at_ms", time.Now().UTC().UnixMilli()).Err(); err != nil {
		return count, err
	}
	return count, nil
}

func ReindexFile(ctx context.Context, rdb *redis.Client, fsKey, inodeID string) (ReindexResult, error) {
	inodeID = strings.TrimSpace(inodeID)
	if inodeID == "" {
		return ReindexResult{}, errors.New("missing inode id")
	}
	inodeKey := InodeKey(fsKey, inodeID)
	values, err := rdb.HMGet(ctx, inodeKey,
		"type", "path", "path_ancestors", "content_ref", "size", "content",
	).Result()
	if err != nil {
		return ReindexResult{}, err
	}
	if len(values) == 0 || values[0] == nil {
		if err := cleanupChunks(ctx, rdb, fsKey, inodeID); err != nil {
			return ReindexResult{}, err
		}
		_ = rdb.SRem(ctx, DirtySetKey(fsKey), inodeID).Err()
		return ReindexResult{InodeID: inodeID, State: StateSkipped, Reason: SkipMissing}, nil
	}
	kind := redisString(values[0])
	filePath := redisString(values[1])
	ancestors := redisString(values[2])
	contentRef := redisString(values[3])
	size := redisInt64(values[4])
	inline := redisString(values[5])
	if kind != "file" {
		if err := cleanupChunks(ctx, rdb, fsKey, inodeID); err != nil {
			return ReindexResult{}, err
		}
		if err := markSkipped(ctx, rdb, fsKey, inodeID, "", SkipUnsupported); err != nil {
			return ReindexResult{}, err
		}
		return ReindexResult{InodeID: inodeID, Path: filePath, State: StateSkipped, Reason: SkipUnsupported}, nil
	}
	if strings.TrimSpace(filePath) == "" {
		filePath = "/" + inodeID
	}
	if ancestors == "" {
		ancestors = IndexedPathAncestors(filePath)
	}
	if IsUnsupportedPath(filePath) {
		if err := cleanupChunks(ctx, rdb, fsKey, inodeID); err != nil {
			return ReindexResult{}, err
		}
		if err := markSkipped(ctx, rdb, fsKey, inodeID, filePath, SkipUnsupported); err != nil {
			return ReindexResult{}, err
		}
		return ReindexResult{InodeID: inodeID, Path: filePath, State: StateSkipped, Reason: SkipUnsupported}, nil
	}
	if size == 0 {
		if contentRef == "" && inline != "" {
			size = int64(len(inline))
		}
		if size == 0 {
			if err := cleanupChunks(ctx, rdb, fsKey, inodeID); err != nil {
				return ReindexResult{}, err
			}
			if err := markSkipped(ctx, rdb, fsKey, inodeID, filePath, SkipEmpty); err != nil {
				return ReindexResult{}, err
			}
			return ReindexResult{InodeID: inodeID, Path: filePath, State: StateSkipped, Reason: SkipEmpty}, nil
		}
	}
	if size > MaxIndexedFileBytes {
		if err := cleanupChunks(ctx, rdb, fsKey, inodeID); err != nil {
			return ReindexResult{}, err
		}
		if err := markSkipped(ctx, rdb, fsKey, inodeID, filePath, SkipLarge); err != nil {
			return ReindexResult{}, err
		}
		return ReindexResult{InodeID: inodeID, Path: filePath, State: StateSkipped, Reason: SkipLarge, Bytes: size}, nil
	}

	var data []byte
	if contentRef == "" {
		data = []byte(inline)
	} else {
		data, err = rediscontent.Load(ctx, rdb, ContentKey(fsKey, inodeID), contentRef, size)
		if err != nil {
			return ReindexResult{}, err
		}
	}
	if !IsPlainText(data) {
		if err := cleanupChunks(ctx, rdb, fsKey, inodeID); err != nil {
			return ReindexResult{}, err
		}
		if err := markSkipped(ctx, rdb, fsKey, inodeID, filePath, SkipBinary); err != nil {
			return ReindexResult{}, err
		}
		return ReindexResult{InodeID: inodeID, Path: filePath, State: StateSkipped, Reason: SkipBinary, Bytes: int64(len(data))}, nil
	}

	hash := contentHash(data)
	chunks := BuildChunks(fsKey, inodeID, filePath, ancestors, hash, string(data))
	if len(chunks) == 0 {
		if err := cleanupChunks(ctx, rdb, fsKey, inodeID); err != nil {
			return ReindexResult{}, err
		}
		if err := markSkipped(ctx, rdb, fsKey, inodeID, filePath, SkipEmpty); err != nil {
			return ReindexResult{}, err
		}
		return ReindexResult{InodeID: inodeID, Path: filePath, State: StateSkipped, Reason: SkipEmpty, Bytes: int64(len(data))}, nil
	}
	if err := replaceChunks(ctx, rdb, fsKey, inodeID, chunks, hash); err != nil {
		return ReindexResult{}, err
	}
	return ReindexResult{InodeID: inodeID, Path: filePath, State: StateReady, Chunks: len(chunks), Bytes: int64(len(data))}, nil
}

func Search(ctx context.Context, rdb *redis.Client, fsKey string, spec SearchSpec, opts SearchOptions) ([]mcptools.FileQueryResult, error) {
	if rdb == nil {
		return nil, ErrSearchUnavailable
	}
	ok, err := EnsureIndex(ctx, rdb, fsKey)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrSearchUnavailable
	}
	pending, err := PendingCount(ctx, rdb, fsKey)
	if err != nil {
		return nil, err
	}
	if pending > 0 {
		catchupLimit := opts.CandidateLimit
		if catchupLimit <= 0 {
			catchupLimit = 256
		}
		if _, err := ProcessPending(ctx, rdb, fsKey, catchupLimit); err != nil {
			return nil, err
		}
		pending, _ = PendingCount(ctx, rdb, fsKey)
		if pending > 0 {
			return nil, ErrProjectionStale
		}
	}
	if pending == 0 {
		checked, err := rdb.HGet(ctx, ReadyKey(fsKey), "backfill_checked_at_ms").Result()
		if (err == nil && strings.TrimSpace(checked) == "") || errors.Is(err, redis.Nil) {
			if _, err := EnqueueFiles(ctx, rdb, fsKey, opts.Path, false); err != nil {
				return nil, err
			}
			pending, err = PendingCount(ctx, rdb, fsKey)
			if err != nil {
				return nil, err
			}
			if pending > 0 {
				catchupLimit := opts.CandidateLimit
				if catchupLimit <= 0 {
					catchupLimit = 256
				}
				if _, err := ProcessPending(ctx, rdb, fsKey, catchupLimit); err != nil {
					return nil, err
				}
				pending, _ = PendingCount(ctx, rdb, fsKey)
				if pending > 0 {
					return nil, ErrProjectionStale
				}
			}
		}
	}

	query := BuildSearchQuery(spec, opts.Path)
	limit := opts.Limit
	if opts.All {
		limit = 1000
	} else if limit <= 0 {
		limit = 10
	}
	if opts.CandidateLimit > 0 && opts.CandidateLimit > limit {
		limit = opts.CandidateLimit
	}
	searchRDB, closeSearch := newSearchRedisClient(rdb)
	defer closeSearch()
	result, err := searchRDB.FTSearchWithArgs(ctx, IndexName(fsKey), query, &redis.FTSearchOptions{
		WithScores:     true,
		Scorer:         "BM25",
		Return:         searchReturns(opts.Full),
		LimitOffset:    0,
		Limit:          limit,
		DialectVersion: 2,
	}).Result()
	if err != nil {
		if isSearchUnavailable(err) {
			return nil, ErrSearchUnavailable
		}
		return nil, err
	}
	out := make([]mcptools.FileQueryResult, 0, len(result.Docs))
	for _, doc := range result.Docs {
		score := 0.0
		if doc.Score != nil {
			score = *doc.Score
		}
		if score < opts.MinScore {
			continue
		}
		snippet := doc.Fields["preview"]
		if opts.Full {
			snippet = doc.Fields["text"]
		}
		out = append(out, mcptools.FileQueryResult{
			Path:        doc.Fields["path"],
			ChunkID:     doc.ID,
			StartLine:   atoi(doc.Fields["start_line"]),
			EndLine:     atoi(doc.Fields["end_line"]),
			Score:       score,
			Snippet:     snippet,
			SearchTypes: append([]string(nil), spec.SearchTypes...),
			Metadata: map[string]any{
				"inode_id":     doc.Fields["inode_id"],
				"content_hash": doc.Fields["content_hash"],
				"seq":          atoi(doc.Fields["seq"]),
				"backend":      "redissearch",
			},
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			if out[i].Path == out[j].Path {
				return out[i].StartLine < out[j].StartLine
			}
			return out[i].Path < out[j].Path
		}
		return out[i].Score > out[j].Score
	})
	if !opts.All && opts.Limit >= 0 && len(out) > opts.Limit {
		out = out[:opts.Limit]
	}
	return out, nil
}

type SearchSpec struct {
	Positive    []string
	Negative    []string
	SearchTypes []string
}

func BuildSearchQuery(spec SearchSpec, scopePath string) string {
	parts := []string{"@type:{chunk}"}
	scopePath = normalizePath(scopePath)
	if scopePath != "/" {
		parts = append(parts, "@path_ancestors:{"+searchindex.EscapeTagValue(scopePath)+"}")
	}
	for _, term := range spec.Positive {
		if term = strings.TrimSpace(term); term != "" {
			parts = append(parts, "@text:"+escapeTextTerm(term))
		}
	}
	for _, term := range spec.Negative {
		if term = strings.TrimSpace(term); term != "" {
			parts = append(parts, "-@text:"+escapeTextTerm(term))
		}
	}
	return strings.Join(parts, " ")
}

func BuildChunks(fsKey, inodeID, filePath, ancestors, hash, text string) []Chunk {
	lines := splitIndexLines(text)
	chunks := make([]Chunk, 0, max(1, len(lines)/MaxChunkLines))
	singleLogicalChunks := queryIndexSingleLogicalChunks(filePath)
	start := 0
	for start < len(lines) {
		end := start
		bytesLen := 0
		for end < len(lines) {
			lineLen := len(lines[end].Text) + 1
			if end > start && (singleLogicalChunks || lines[end].Number == lines[end-1].Number) {
				break
			}
			if end > start && (end-start >= MaxChunkLines || bytesLen+lineLen > MaxChunkBytes) {
				break
			}
			bytesLen += lineLen
			end++
		}
		if end == start {
			end++
		}
		chunkLines := make([]string, 0, end-start)
		for _, line := range lines[start:end] {
			chunkLines = append(chunkLines, line.Text)
		}
		chunkText := strings.Join(chunkLines, "\n")
		if strings.TrimSpace(chunkText) != "" {
			seq := len(chunks)
			chunks = append(chunks, Chunk{
				Key:         ChunkKey(fsKey, inodeID, hash, seq),
				Path:        filePath,
				InodeID:     inodeID,
				ContentHash: hash,
				Seq:         seq,
				StartLine:   lines[start].Number,
				EndLine:     lines[end-1].Number,
				Text:        chunkText,
				Preview:     previewText(chunkText),
			})
		}
		start = end
	}
	return chunks
}

func queryIndexSingleLogicalChunks(filePath string) bool {
	switch strings.ToLower(path.Ext(filePath)) {
	case ".jsonl":
		return true
	default:
		return false
	}
}

type indexLine struct {
	Text   string
	Number int
}

func splitIndexLines(text string) []indexLine {
	physical := splitLines(text)
	lines := make([]indexLine, 0, len(physical))
	for i, line := range physical {
		for _, part := range splitLogicalText(line, MaxChunkBytes) {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			lines = append(lines, indexLine{Text: part, Number: i + 1})
		}
	}
	return lines
}

func splitLogicalText(text string, maxBytes int) []string {
	parts := []string{text}
	parts = splitEachAfter(parts, "} {", 1)
	parts = splitEachAfter(parts, "\\n", len("\\n"))
	if maxBytes > 0 {
		parts = splitEachLongPart(parts, maxBytes)
	}
	return parts
}

func splitEachAfter(parts []string, token string, keep int) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		out = append(out, splitAfterToken(part, token, keep)...)
	}
	return out
}

func splitAfterToken(text, token string, keep int) []string {
	if token == "" || !strings.Contains(text, token) {
		return []string{text}
	}
	parts := make([]string, 0, 2)
	rest := text
	for {
		idx := strings.Index(rest, token)
		if idx < 0 {
			if rest != "" {
				parts = append(parts, rest)
			}
			return parts
		}
		cut := idx + keep
		parts = append(parts, rest[:cut])
		rest = rest[cut:]
	}
}

func splitEachLongPart(parts []string, maxBytes int) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		out = append(out, splitLongPart(part, maxBytes)...)
	}
	return out
}

func splitLongPart(text string, maxBytes int) []string {
	if maxBytes <= 0 || len(text) <= maxBytes {
		return []string{text}
	}
	parts := make([]string, 0, len(text)/maxBytes+1)
	rest := strings.TrimSpace(text)
	for len(rest) > maxBytes {
		cut := lastWhitespaceBefore(rest, maxBytes)
		if cut <= 0 {
			cut = maxBytes
		}
		parts = append(parts, rest[:cut])
		rest = strings.TrimSpace(rest[cut:])
	}
	if rest != "" {
		parts = append(parts, rest)
	}
	return parts
}

func lastWhitespaceBefore(text string, maxBytes int) int {
	cut := 0
	for idx, r := range text {
		if idx > maxBytes {
			break
		}
		if r == ' ' || r == '\t' {
			cut = idx
		}
	}
	return cut
}

func PendingCount(ctx context.Context, rdb *redis.Client, fsKey string) (int, error) {
	n, err := rdb.SCard(ctx, DirtySetKey(fsKey)).Result()
	return int(n), err
}

func IndexedPathAncestors(p string) string {
	trimmed := strings.TrimSpace(normalizePath(p))
	if trimmed == "" || trimmed == "/" {
		return "/"
	}
	trimmed = strings.TrimSuffix(trimmed, "/")
	parts := strings.Split(strings.TrimPrefix(trimmed, "/"), "/")
	ancestors := make([]string, 0, len(parts)+1)
	current := ""
	for _, part := range parts {
		if part == "" {
			continue
		}
		current += "/" + part
		ancestors = append(ancestors, current)
	}
	if len(ancestors) == 0 {
		return "/"
	}
	return strings.Join(ancestors, ",")
}

func IsPlainText(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	checkLen := len(data)
	if checkLen > 8192 {
		checkLen = 8192
	}
	if bytes.IndexByte(data[:checkLen], '\x00') >= 0 {
		return false
	}
	return utf8.Valid(data)
}

func IsUnsupportedPath(p string) bool {
	switch strings.ToLower(path.Ext(p)) {
	case ".pdf", ".doc", ".docx", ".ppt", ".pptx", ".xls", ".xlsx",
		".png", ".jpg", ".jpeg", ".gif", ".webp", ".ico", ".heic", ".avif",
		".mp3", ".mp4", ".m4a", ".mov", ".avi", ".wav", ".flac",
		".zip", ".gz", ".tgz", ".tar", ".bz2", ".xz", ".7z", ".rar", ".dmg",
		".sqlite", ".sqlite3", ".db", ".so", ".dylib", ".dll", ".exe", ".bin",
		".wasm", ".class", ".jar", ".pyc":
		return true
	default:
		return false
	}
}

func replaceChunks(ctx context.Context, rdb *redis.Client, fsKey, inodeID string, chunks []Chunk, hash string) error {
	oldKeys, err := rdb.SMembers(ctx, ChunkSetKey(fsKey, inodeID)).Result()
	if err != nil {
		return err
	}
	pipe := rdb.Pipeline()
	if len(oldKeys) > 0 {
		pipe.Del(ctx, oldKeys...)
	}
	pipe.Del(ctx, ChunkSetKey(fsKey, inodeID))
	for _, chunk := range chunks {
		pipe.HSet(ctx, chunk.Key, map[string]interface{}{
			"type":           "chunk",
			"path":           chunk.Path,
			"path_ancestors": IndexedPathAncestors(chunk.Path),
			"inode_id":       chunk.InodeID,
			"content_hash":   chunk.ContentHash,
			"seq":            chunk.Seq,
			"start_line":     chunk.StartLine,
			"end_line":       chunk.EndLine,
			"text":           chunk.Text,
			"preview":        chunk.Preview,
		})
		pipe.SAdd(ctx, ChunkSetKey(fsKey, inodeID), chunk.Key)
	}
	pipe.HSet(ctx, InodeKey(fsKey, inodeID), map[string]interface{}{
		"query_state":         StateReady,
		"query_index_version": ProjectionVersion,
		"query_skip_reason":   "",
		"query_error":         "",
		"query_content_hash":  hash,
		"query_chunk_count":   len(chunks),
		"query_indexed_at_ms": time.Now().UTC().UnixMilli(),
	})
	pipe.SRem(ctx, DirtySetKey(fsKey), inodeID)
	_, err = pipe.Exec(ctx)
	if errors.Is(err, redis.Nil) {
		return nil
	}
	return err
}

func cleanupChunks(ctx context.Context, rdb *redis.Client, fsKey, inodeID string) error {
	oldKeys, err := rdb.SMembers(ctx, ChunkSetKey(fsKey, inodeID)).Result()
	if err != nil {
		return err
	}
	pipe := rdb.Pipeline()
	if len(oldKeys) > 0 {
		pipe.Del(ctx, oldKeys...)
	}
	pipe.Del(ctx, ChunkSetKey(fsKey, inodeID))
	_, err = pipe.Exec(ctx)
	if errors.Is(err, redis.Nil) {
		return nil
	}
	return err
}

func markSkipped(ctx context.Context, rdb *redis.Client, fsKey, inodeID, filePath, reason string) error {
	pipe := rdb.Pipeline()
	pipe.HSet(ctx, InodeKey(fsKey, inodeID), map[string]interface{}{
		"query_state":         StateSkipped,
		"query_index_version": ProjectionVersion,
		"query_skip_reason":   reason,
		"query_error":         "",
		"query_content_hash":  "",
		"query_chunk_count":   0,
		"query_indexed_at_ms": time.Now().UTC().UnixMilli(),
	})
	pipe.SRem(ctx, DirtySetKey(fsKey), inodeID)
	_, err := pipe.Exec(ctx)
	if errors.Is(err, redis.Nil) {
		return nil
	}
	return err
}

func scanDirtyInodes(ctx context.Context, rdb *redis.Client, fsKey string, limit int) ([]string, error) {
	out := make([]string, 0, limit)
	var cursor uint64
	for len(out) < limit {
		values, next, err := rdb.SScan(ctx, DirtySetKey(fsKey), cursor, "", int64(limit-len(out))).Result()
		if err != nil {
			return nil, err
		}
		out = append(out, values...)
		cursor = next
		if cursor == 0 {
			break
		}
	}
	sort.Strings(out)
	return out, nil
}

func scanFiles(ctx context.Context, rdb *redis.Client, fsKey, scopePath string, fn func(map[string]string) error) error {
	scopePath = normalizePath(scopePath)
	pattern := InodeKey(fsKey, "*")
	var cursor uint64
	for {
		keys, next, err := rdb.Scan(ctx, cursor, pattern, 500).Result()
		if err != nil {
			return err
		}
		for _, key := range keys {
			fields, err := rdb.HGetAll(ctx, key).Result()
			if err != nil {
				return err
			}
			if fields["type"] != "file" {
				continue
			}
			filePath := normalizePath(fields["path"])
			if !pathInScope(filePath, scopePath) {
				continue
			}
			fields["id"] = strings.TrimPrefix(key, InodeKey(fsKey, ""))
			if err := fn(fields); err != nil {
				return err
			}
		}
		cursor = next
		if cursor == 0 {
			return nil
		}
	}
}

func pathInScope(filePath, scopePath string) bool {
	filePath = normalizePath(filePath)
	scopePath = normalizePath(scopePath)
	if scopePath == "/" {
		return true
	}
	return filePath == scopePath || strings.HasPrefix(filePath, strings.TrimSuffix(scopePath, "/")+"/")
}

func searchReturns(full bool) []redis.FTSearchReturn {
	fields := []redis.FTSearchReturn{
		{FieldName: "path"},
		{FieldName: "inode_id"},
		{FieldName: "content_hash"},
		{FieldName: "seq"},
		{FieldName: "start_line"},
		{FieldName: "end_line"},
		{FieldName: "preview"},
	}
	if full {
		fields = append(fields, redis.FTSearchReturn{FieldName: "text"})
	}
	return fields
}

func splitLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func previewText(text string) string {
	return strings.TrimSpace(text)
}

func contentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func escapeTextTerm(term string) string {
	term = strings.TrimSpace(term)
	if term == "" {
		return ""
	}
	if strings.ContainsAny(term, " \t\n") {
		return `"` + strings.ReplaceAll(term, `"`, `\"`) + `"`
	}
	var b strings.Builder
	for _, r := range term {
		if strings.ContainsRune(",.<>{}[]\"':;!@#$%^&*()-+=~|\\/", r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
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

func redisString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func redisInt64(v interface{}) int64 {
	s := redisString(v)
	if s == "" {
		return 0
	}
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
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
