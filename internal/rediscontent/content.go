package rediscontent

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/redis/go-redis/v9"
)

const (
	RefExternal = "ext"
	RefArray    = "array"

	// ArrayChunkBytes is the fixed logical chunk size used when AFS stores file
	// bytes in Redis Array values. File metadata still owns the exact logical
	// size; the array only stores chunk payloads.
	ArrayChunkBytes = 4096

	arrayCommandBatchChunks = 512
	arrayMaxPredicates      = 250
)

var arraySupportCache sync.Map

func SupportsArrays(ctx context.Context, rdb *redis.Client) (bool, error) {
	if rdb == nil {
		return false, errors.New("nil redis client")
	}
	cacheKey := arraySupportCacheKey(rdb)
	if cached, ok := arraySupportCache.Load(cacheKey); ok {
		return cached.(bool), nil
	}
	supported, err := probeArraySupport(ctx, rdb)
	if err != nil {
		return false, err
	}
	arraySupportCache.Store(cacheKey, supported)
	return supported, nil
}

func PreferredRef(ctx context.Context, rdb *redis.Client) (string, error) {
	supported, err := SupportsArrays(ctx, rdb)
	if err != nil {
		return "", err
	}
	if supported {
		return RefArray, nil
	}
	return RefExternal, nil
}

func Load(ctx context.Context, rdb *redis.Client, contentKey, ref string, size int64) ([]byte, error) {
	switch strings.TrimSpace(ref) {
	case "", RefExternal:
		value, err := rdb.Get(ctx, contentKey).Bytes()
		if err != nil && !errors.Is(err, redis.Nil) {
			return nil, err
		}
		return value, nil
	case RefArray:
		supported, err := SupportsArrays(ctx, rdb)
		if err != nil {
			return nil, err
		}
		if !supported {
			return nil, fmt.Errorf("redis array content requested for %q but the server does not support array commands", contentKey)
		}
		return loadArray(ctx, rdb, contentKey, size)
	default:
		return nil, fmt.Errorf("unsupported content_ref %q", ref)
	}
}

func QueueWriteFull(ctx context.Context, pipe redis.Pipeliner, contentKey, ref string, data []byte) {
	switch strings.TrimSpace(ref) {
	case "", RefExternal:
		pipe.Set(ctx, contentKey, data, 0)
	case RefArray:
		queueArrayWriteFull(ctx, pipe, contentKey, data)
	default:
		pipe.Set(ctx, contentKey, data, 0)
	}
}

func ReadRange(ctx context.Context, rdb *redis.Client, contentKey, ref string, size, off int64, count int) ([]byte, error) {
	if count <= 0 || off >= size {
		return []byte{}, nil
	}
	if off < 0 {
		return nil, errors.New("invalid offset")
	}
	end := off + int64(count)
	if end > size {
		end = size
	}
	switch strings.TrimSpace(ref) {
	case "", RefExternal:
		value, err := rdb.GetRange(ctx, contentKey, off, end-1).Bytes()
		if err != nil && !errors.Is(err, redis.Nil) {
			return nil, err
		}
		return value, nil
	case RefArray:
		supported, err := SupportsArrays(ctx, rdb)
		if err != nil {
			return nil, err
		}
		if !supported {
			return nil, fmt.Errorf("redis array content requested for %q but the server does not support array commands", contentKey)
		}
		return readArrayRange(ctx, rdb, contentKey, size, off, end)
	default:
		return nil, fmt.Errorf("unsupported content_ref %q", ref)
	}
}

func WriteRange(ctx context.Context, rdb *redis.Client, contentKey string, off int64, payload []byte) error {
	if off < 0 {
		return errors.New("invalid offset")
	}
	if len(payload) == 0 {
		return nil
	}
	supported, err := SupportsArrays(ctx, rdb)
	if err != nil {
		return err
	}
	if !supported {
		return fmt.Errorf("redis array content requested for %q but the server does not support array commands", contentKey)
	}
	return writeArrayRange(ctx, rdb, contentKey, off, payload)
}

func Truncate(ctx context.Context, rdb *redis.Client, contentKey string, oldSize, newSize int64) error {
	if newSize < 0 {
		return errors.New("invalid size")
	}
	supported, err := SupportsArrays(ctx, rdb)
	if err != nil {
		return err
	}
	if !supported {
		return fmt.Errorf("redis array content requested for %q but the server does not support array commands", contentKey)
	}
	return truncateArray(ctx, rdb, contentKey, oldSize, newSize)
}

func MayContainLiteral(ctx context.Context, rdb *redis.Client, contentKey string, size int64, literal string, nocase bool) (bool, error) {
	if size <= 0 || literal == "" {
		return size > 0, nil
	}
	supported, err := SupportsArrays(ctx, rdb)
	if err != nil {
		return false, err
	}
	if !supported {
		return false, nil
	}
	candidates := literalProbeTerms(literal)
	if len(candidates) == 0 {
		return true, nil
	}
	args := make([]interface{}, 0, 5+(len(candidates)*2)+3)
	args = append(args, "ARGREP", contentKey, 0, chunkCountForSize(size)-1)
	for _, candidate := range candidates {
		args = append(args, "MATCH", candidate)
	}
	if nocase {
		args = append(args, "NOCASE")
	}
	args = append(args, "LIMIT", 1)
	reply, err := rdb.Do(ctx, args...).Result()
	if err != nil {
		return false, err
	}
	values := arrayReplyValues(reply)
	return len(values) > 0, nil
}

func probeArraySupport(ctx context.Context, rdb *redis.Client) (bool, error) {
	reply, err := rdb.Do(ctx, "COMMAND", "INFO", "ARSET").Result()
	if err == nil {
		return commandInfoHasCommand(reply), nil
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	reply, err = rdb.Do(ctx, "ARLEN", "__afs_array_capability_probe__").Result()
	if err == nil {
		_, _ = reply.(int64)
		return true, nil
	}
	if isUnknownCommand(err) {
		return false, nil
	}
	return false, err
}

func commandInfoHasCommand(reply interface{}) bool {
	values := arrayReplyValues(reply)
	if len(values) != 1 || values[0] == nil {
		return false
	}
	cmdInfo := arrayReplyValues(values[0])
	if len(cmdInfo) == 0 {
		return false
	}
	return strings.EqualFold(string(arrayValueBytes(cmdInfo[0])), "arset")
}

func loadArray(ctx context.Context, rdb *redis.Client, contentKey string, size int64) ([]byte, error) {
	if size <= 0 {
		return []byte{}, nil
	}
	totalChunks := chunkCountForSize(size)
	result := make([]byte, size)
	for start := 0; start < totalChunks; start += arrayCommandBatchChunks {
		end := min(start+arrayCommandBatchChunks, totalChunks) - 1
		values, err := arrayGetRange(ctx, rdb, contentKey, start, end)
		if err != nil {
			return nil, err
		}
		for i, value := range values {
			if value == nil {
				continue
			}
			raw := arrayValueBytes(value)
			offset := int64(start+i) * ArrayChunkBytes
			limit := min(int64(len(raw)), size-offset)
			if limit <= 0 {
				continue
			}
			startIdx := int(offset)
			endIdx := startIdx + int(limit)
			copy(result[startIdx:endIdx], raw[:int(limit)])
		}
	}
	return result, nil
}

func queueArrayWriteFull(ctx context.Context, pipe redis.Pipeliner, contentKey string, data []byte) {
	pipe.Del(ctx, contentKey)
	if len(data) == 0 {
		return
	}
	chunks := splitArrayChunks(data)
	for start := 0; start < len(chunks); start += arrayCommandBatchChunks {
		end := min(start+arrayCommandBatchChunks, len(chunks))
		args := make([]interface{}, 0, 3+(end-start))
		args = append(args, "ARSET", contentKey, start)
		for _, chunk := range chunks[start:end] {
			args = append(args, string(chunk))
		}
		pipe.Do(ctx, args...)
	}
}

func readArrayRange(ctx context.Context, rdb *redis.Client, contentKey string, size, startByte, endByte int64) ([]byte, error) {
	if endByte <= startByte {
		return []byte{}, nil
	}
	startChunk := int(startByte / ArrayChunkBytes)
	endChunk := int((endByte - 1) / ArrayChunkBytes)
	values, err := arrayGetRange(ctx, rdb, contentKey, startChunk, endChunk)
	if err != nil {
		return nil, err
	}
	result := make([]byte, endByte-startByte)
	for i, value := range values {
		chunkStart := int64(startChunk+i) * ArrayChunkBytes
		chunkEnd := chunkStart + ArrayChunkBytes
		segmentStart := max(startByte, chunkStart)
		segmentEnd := min(endByte, chunkEnd)
		if segmentEnd <= segmentStart || value == nil {
			continue
		}
		raw := arrayValueBytes(value)
		srcStart := segmentStart - chunkStart
		srcEnd := min(int64(len(raw)), segmentEnd-chunkStart)
		if srcEnd <= srcStart {
			continue
		}
		dstStart := segmentStart - startByte
		copy(
			result[int(dstStart):int(dstStart+(srcEnd-srcStart))],
			raw[int(srcStart):int(srcEnd)],
		)
	}
	return result, nil
}

func writeArrayRange(ctx context.Context, rdb *redis.Client, contentKey string, off int64, payload []byte) error {
	startChunk := int(off / ArrayChunkBytes)
	endByte := off + int64(len(payload))
	endChunk := int((endByte - 1) / ArrayChunkBytes)

	existing, err := arrayGetRange(ctx, rdb, contentKey, startChunk, endChunk)
	if err != nil {
		return err
	}
	chunks := make([][]byte, len(existing))
	chunkLens := make([]int, len(existing))
	payloadOffset := 0
	for i, value := range existing {
		raw := arrayValueBytes(value)
		chunk := make([]byte, ArrayChunkBytes)
		copy(chunk, raw)

		chunkStart := int64(startChunk+i) * ArrayChunkBytes
		chunkEnd := chunkStart + ArrayChunkBytes
		writeStart := max(off, chunkStart)
		writeEnd := min(endByte, chunkEnd)
		if writeEnd > writeStart {
			dstStart := int(writeStart - chunkStart)
			n := int(writeEnd - writeStart)
			copy(chunk[dstStart:dstStart+n], payload[payloadOffset:payloadOffset+n])
			payloadOffset += n
		}

		storeLen := len(raw)
		if writeEnd > chunkStart {
			storeLen = max(storeLen, int(writeEnd-chunkStart))
		}
		chunks[i] = chunk
		chunkLens[i] = min(storeLen, ArrayChunkBytes)
	}

	args := make([]interface{}, 0, 3+len(chunks))
	args = append(args, "ARSET", contentKey, startChunk)
	for i, chunk := range chunks {
		args = append(args, string(chunk[:chunkLens[i]]))
	}
	return rdb.Do(ctx, args...).Err()
}

func truncateArray(ctx context.Context, rdb *redis.Client, contentKey string, oldSize, newSize int64) error {
	if newSize == oldSize {
		return nil
	}
	if newSize == 0 {
		return rdb.Del(ctx, contentKey).Err()
	}
	if newSize > oldSize {
		return nil
	}

	oldChunks := chunkCountForSize(oldSize)
	newChunks := chunkCountForSize(newSize)
	if newChunks == 0 {
		return rdb.Del(ctx, contentKey).Err()
	}

	lastChunkSize := int(newSize % ArrayChunkBytes)
	if lastChunkSize == 0 {
		if newChunks < oldChunks {
			return rdb.Do(ctx, "ARDELRANGE", contentKey, newChunks, oldChunks-1).Err()
		}
		return nil
	}

	values, err := arrayGetRange(ctx, rdb, contentKey, newChunks-1, newChunks-1)
	if err != nil {
		return err
	}

	pipe := rdb.Pipeline()
	if len(values) > 0 && values[0] != nil {
		raw := arrayValueBytes(values[0])
		keep := min(lastChunkSize, len(raw))
		args := []interface{}{"ARSET", contentKey, newChunks - 1, string(raw[:keep])}
		pipe.Do(ctx, args...)
	}
	if newChunks < oldChunks {
		pipe.Do(ctx, "ARDELRANGE", contentKey, newChunks, oldChunks-1)
	}
	_, err = pipe.Exec(ctx)
	return err
}

func arrayGetRange(ctx context.Context, rdb *redis.Client, contentKey string, start, end int) ([]interface{}, error) {
	if end < start {
		return nil, nil
	}
	reply, err := rdb.Do(ctx, "ARGETRANGE", contentKey, start, end).Result()
	if err != nil {
		return nil, err
	}
	return arrayReplyValues(reply), nil
}

func arrayReplyValues(reply interface{}) []interface{} {
	if reply == nil {
		return nil
	}
	values, ok := reply.([]interface{})
	if !ok {
		return nil
	}
	return values
}

func arrayValueBytes(value interface{}) []byte {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		return []byte(v)
	case []byte:
		return v
	default:
		return []byte(fmt.Sprint(v))
	}
}

func splitArrayChunks(data []byte) [][]byte {
	if len(data) == 0 {
		return nil
	}
	chunks := make([][]byte, 0, chunkCountForSize(int64(len(data))))
	for start := 0; start < len(data); start += ArrayChunkBytes {
		end := min(start+ArrayChunkBytes, len(data))
		chunks = append(chunks, data[start:end])
	}
	return chunks
}

func chunkCountForSize(size int64) int {
	if size <= 0 {
		return 0
	}
	return int((size + ArrayChunkBytes - 1) / ArrayChunkBytes)
}

func literalProbeTerms(literal string) []string {
	if literal == "" {
		return nil
	}
	data := []byte(literal)
	if len(data) <= 2 {
		return uniqueByteTerms(data, 1)
	}
	seen := make(map[string]struct{}, len(data))
	terms := make([]string, 0, min(len(data)-1, arrayMaxPredicates))
	for i := 0; i+1 < len(data); i++ {
		term := string(data[i : i+2])
		if _, ok := seen[term]; ok {
			continue
		}
		seen[term] = struct{}{}
		terms = append(terms, term)
		if len(terms) == arrayMaxPredicates {
			return terms
		}
	}
	if len(terms) > 0 {
		return terms
	}
	return uniqueByteTerms(data, 1)
}

func uniqueByteTerms(data []byte, width int) []string {
	if len(data) == 0 || width <= 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(data))
	terms := make([]string, 0, min(len(data), arrayMaxPredicates))
	for i := 0; i+width <= len(data); i++ {
		term := string(data[i : i+width])
		if _, ok := seen[term]; ok {
			continue
		}
		seen[term] = struct{}{}
		terms = append(terms, term)
		if len(terms) == arrayMaxPredicates {
			break
		}
	}
	return terms
}

func arraySupportCacheKey(rdb *redis.Client) string {
	opts := rdb.Options()
	if opts == nil {
		return "unknown"
	}
	return strings.Join([]string{
		opts.Network,
		opts.Addr,
		opts.Username,
		strconv.Itoa(opts.DB),
	}, "|")
}

func isUnknownCommand(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "unknown command")
}
