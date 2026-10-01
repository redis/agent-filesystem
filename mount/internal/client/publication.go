package client

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/redis/agent-filesystem/internal/queryindex"
	"github.com/redis/agent-filesystem/internal/rediscontent"
	"github.com/redis/go-redis/v9"
)

// ErrWriteConflict leaves the caller's candidate intact for the existing sync
// conflict/recovery path. A failed conditional publication is never retried
// against a newly observed remote version.
var ErrWriteConflict = errors.New("remote file changed during publication")
var ErrWorkspaceChanged = errors.New("workspace was restored or deleted; remount required")

type expectedStatKey struct{}
type expectedStat struct{ stat *StatResult }
type workspaceGenerationKey struct{}

// WithExpectedStat binds a mutation to an observation made before reading the
// remote bytes. nil expects an absent path. Copying prevents callers changing
// the condition while an asynchronous operation is running.
func WithExpectedStat(ctx context.Context, stat *StatResult) context.Context {
	if stat != nil {
		copy := *stat
		stat = &copy
	}
	return context.WithValue(ctx, expectedStatKey{}, expectedStat{stat})
}

func WithWorkspaceGeneration(ctx context.Context, generation string) context.Context {
	return context.WithValue(ctx, workspaceGenerationKey{}, generation)
}

func (c *nativeClient) checkGeneration(ctx context.Context) error {
	if checked, _ := ctx.Value(nativeRequestCheckedKey{}).(bool); checked {
		return nil
	}
	generation, ok := ctx.Value(workspaceGenerationKey{}).(string)
	if !ok {
		return nil
	}
	if lease := nativeLease(ctx); lease != "" {
		values, err := c.rdb.MGet(ctx, c.keys.generation(), lease).Result()
		if err != nil {
			return err
		}
		if values[0] != generation || generation == "deleted" {
			return ErrWorkspaceChanged
		}
		if values[1] == nil {
			return ErrNativeSessionLost
		}
		return nil
	}
	actual, err := c.rdb.Get(ctx, c.keys.generation()).Result()
	if errors.Is(err, redis.Nil) || (err == nil && (actual != generation || actual == "deleted")) {
		return ErrWorkspaceChanged
	}
	return err
}

func checkWriteCondition(ctx context.Context, inode *inodeData, creating bool) error {
	expected, ok := ctx.Value(expectedStatKey{}).(expectedStat)
	if !ok {
		return nil
	}
	if expected.stat == nil {
		if !creating {
			return ErrWriteConflict
		}
		return nil
	}
	if creating || expected.stat.Inode != inodeUint64(inode.ID) || expected.stat.Revision != inode.Revision {
		return ErrWriteConflict
	}
	return nil
}

// Only temporary keys are new: the published inode/content representation and
// chunk transfer remain unchanged. An interrupted upload expires without ever
// replacing a live content key. Redis executes publication as one command;
// operation identity makes its automatic network retries safe.
const publicationTTL = time.Hour
const commitReceiptTTL = 7 * 24 * time.Hour

var publishFileScript = redis.NewScript(`
local receipttype=redis.call('TYPE',KEYS[13]).ok
if receipttype~='none' and receipttype~='string' then return -4 end
local receipt=redis.call('GET',KEYS[13])
if receipt then
 if receipt~=ARGV[10] then return -6 end
 return 2
end
-- Redis scripts do not roll back runtime errors. Reject malformed metadata
-- and bookkeeping before consuming staging or changing live content.
for _,i in ipairs({1,3,5,6}) do
 local t=redis.call('TYPE',KEYS[i]).ok
 if t~='none' and t~='hash' then return -4 end
end
if redis.call('EXISTS',KEYS[14])~=0 then return -4 end
local size=tonumber(ARGV[8])
local old=tonumber(redis.call('HGET',KEYS[1],'size') or '0')
local total=redis.call('HGET',KEYS[5],'total_data_bytes') or '0'
local files=redis.call('HGET',KEYS[5],'files') or '0'
local function integer(s)
 return string.match(s,'^%-?%d+$') and tonumber(s) and math.abs(tonumber(s))<4503599627370496
end
if not integer(total) or not integer(files) or not size or size<0 or size>=4503599627370496 or not old or old<0 or old>=4503599627370496 then return -4 end
if math.abs(tonumber(total)+size-old)>=4503599627370496 then return -4 end
if (#ARGV-10)%2~=0 then return -4 end
if ARGV[7] ~= '' and redis.call('GET', KEYS[7]) ~= ARGV[7] then return -2 end
if KEYS[11] ~= KEYS[7] and redis.call('EXISTS',KEYS[11]) == 0 then return -5 end
local current = redis.call('HGET', KEYS[1], 'revision') or ''
local linked = redis.call('HGET', KEYS[3], ARGV[2])
if current == ARGV[4] and linked == ARGV[1] then return 2 end
local dirtytype=redis.call('TYPE',KEYS[12]).ok
if dirtytype~='none' and dirtytype~='set' then return -4 end
local streamtype=redis.call('TYPE',KEYS[9]).ok
if streamtype~='none' and streamtype~='stream' then return -4 end
if redis.call('EXISTS', KEYS[6]) == 0 then return -1 end
if ARGV[5] == '1' then
 if linked then return -1 end
else
 if linked ~= ARGV[1] or current ~= ARGV[3] then return -1 end
 if redis.call('EXISTS', KEYS[1]) == 0 then return -1 end
end
-- Every publication consumes staging, including an empty file. A missing
-- stage after a concurrent delete must never let a transport retry recreate it.
if redis.call('EXISTS', KEYS[2]) == 0 then return -3 end
local previous = tonumber(redis.call('HGET', KEYS[1], 'size') or '0')
if redis.call('EXISTS',KEYS[4])~=0 then
 redis.call('RENAME',KEYS[4],KEYS[14])
 redis.call('EXPIRE',KEYS[14],3600)
end
if ARGV[8] == '0' then
 redis.call('UNLINK', KEYS[2])
else
 redis.call('RENAME', KEYS[2], KEYS[4])
 redis.call('PERSIST', KEYS[4])
end
for i=11,#ARGV,2 do redis.call('HSET',KEYS[1],ARGV[i],ARGV[i+1]) end
redis.call('HSET',KEYS[1],'revision',ARGV[4])
redis.call('HDEL',KEYS[1],'content')
if ARGV[5] == '1' then
 redis.call('HSETNX',KEYS[3],ARGV[2],ARGV[1])
 redis.call('HINCRBY',KEYS[5],'files',1)
 redis.call('HSET',KEYS[6],'mtime_ms',ARGV[6],'ctime_ms',ARGV[6])
end
redis.call('HINCRBY',KEYS[5],'total_data_bytes',tonumber(ARGV[8])-previous)
redis.call('SET',KEYS[8],'1')
redis.call('SADD',KEYS[12],ARGV[1])
if ARGV[9] ~= '' then
 redis.call('XADD',KEYS[9],'MAXLEN','~',10000,'*','payload',ARGV[9])
 if KEYS[10]~=KEYS[7] then redis.call('PUBLISH',KEYS[10],ARGV[9]) end
end
redis.call('SET',KEYS[13],ARGV[10],'EX',604800)
return 1
`)

func (c *nativeClient) publishStagedFile(ctx context.Context, p string, inode *inodeData, stage string, creating bool, extra map[string]interface{}) error {
	if err := checkExpectedParent(ctx, p, inode.Parent); err != nil {
		return err
	}
	if err := checkWriteCondition(ctx, inode, creating); err != nil {
		return err
	}
	revision := newOriginID()
	if mode, ok := ctx.Value(fileModeKey{}).(uint32); ok {
		inode.Mode = mode
	}
	generation, _ := ctx.Value(workspaceGenerationKey{}).(string)
	create := "0"
	if creating {
		create = "1"
	}
	if _, ok := extra["search_state"]; !ok {
		searchFields := map[string]interface{}{"search_state": fileSearchStateLarge, "grep_grams_ci": ""}
		if inode.Size <= fileSearchMaxIndexedBytes {
			data, err := rediscontent.Load(ctx, c.rdb, stage, inode.ContentRef, inode.Size)
			if err != nil {
				return err
			}
			searchFields = fileSearchIndexFields(string(data))
		}
		extra = mergeFieldMaps(extra, searchFields)
	}
	// Preserve this operation's exact published bytes for history. Reading the
	// live key after commit can attribute another writer's content to our event.
	var committed *VersionedSnapshot
	if c.observer != nil {
		data, err := rediscontent.Load(ctx, c.rdb, stage, inode.ContentRef, inode.Size)
		if inode.Size == 0 {
			data, err = []byte{}, nil
		}
		if err != nil {
			return err
		}
		committed = &VersionedSnapshot{Path: normalizePath(p), Exists: true, Kind: "file", Mode: inode.Mode, Content: data, SizeBytes: inode.Size}
	}
	fields := mergeFieldMaps(c.inodeFieldsAtPath(inode, p, false), queryindex.StaleFields(), extra)
	if _, ranged := ctx.Value(nativeRangeKey{}).(bool); ranged {
		// An open handle's path is only an invalidation hint. A concurrent
		// ancestor rename must not have its canonical path overwritten here.
		delete(fields, "path")
		delete(fields, "path_ancestors")
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	payload, err := encodeInvalidate(InvalidateEvent{Origin: c.originID, Op: InvalidateOpInode, Paths: []string{p}, OperationID: revision})
	if err != nil {
		return err
	}
	args := []interface{}{inode.ID, inode.Name, inode.Revision, revision, create, inode.MtimeMs, generation, inode.Size, string(payload), ""}
	for _, key := range keys {
		args = append(args, key, fields[key])
	}
	// Bind the receipt to this exact stage and request. An operation ID is
	// allocated once, and go-redis transport retries reuse the same script args.
	encoded, err := json.Marshal(struct {
		Stage string
		Args  []interface{}
	}{stage, args})
	if err != nil {
		return err
	}
	digest := sha256.Sum256(encoded)
	requestDigest := hex.EncodeToString(digest[:])
	args[9] = requestDigest
	receiptKey := "afs:{" + c.key + "}:commit:" + revision
	retiredKey := "afs:{" + c.key + "}:retired:" + revision
	defer c.discardStage(retiredKey)
	result, err := c.runMutationScript(ctx, publishFileScript, false, []string{
		c.keys.inode(inode.ID), stage, c.keys.dirents(inode.Parent), c.keys.content(inode.ID), c.keys.info(), c.keys.inode(inode.Parent), c.keys.generation(), c.keys.rootDirty(), c.keys.changesStream(), c.notificationChannel(), c.leaseGuardKey(ctx), queryindex.DirtySetKey(c.key), receiptKey, retiredKey,
	}, args...)
	if err != nil {
		// A response can be lost after Redis committed. Resolve that uncertainty
		// from the operation token, without replaying the candidate over new data.
		token, readErr := c.rdb.Get(ctx, receiptKey).Result()
		if readErr != nil || token != requestDigest {
			return err
		}
	} else {
		switch result {
		case -1:
			return ErrWriteConflict
		case -2:
			return ErrWorkspaceChanged
		case -3:
			return errors.New("staged file expired before publication")
		case -4:
			return errors.New("workspace change journal has wrong Redis type")
		case -5:
			return ErrNativeSessionLost
		case -6:
			return errors.New("publication operation ID reused with a different request")
		}
	}
	inode.Revision = revision
	inode.committedSnapshot = committed
	captureMutationResult(ctx, inode, revision)
	if _, ranged := ctx.Value(nativeRangeKey{}).(bool); !ranged {
		c.cachePath(p, inode)
	}
	return nil
}

func (c *nativeClient) stageFullFile(ctx context.Context, inode *inodeData) (string, error) {
	stage := c.keys.content(inode.ID) + ":stage:" + newOriginID()
	pipe := c.rdb.TxPipeline()
	if inode.Size == 0 {
		// Empty Arrays need no live content key, but still need consumable staging.
		pipe.Set(ctx, stage, "", 0)
	} else {
		rediscontent.QueueWriteFull(ctx, pipe, stage, inode.ContentRef, []byte(inode.Content))
	}
	pipe.Expire(ctx, stage, publicationTTL)
	_, err := pipe.Exec(ctx)
	return stage, err
}

func (c *nativeClient) discardStage(stage string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = c.rdb.Unlink(ctx, stage).Err()
}

func (c *nativeClient) publishChunks(ctx context.Context, p string, chunks map[int][]byte, chunkSize int, newSize int64, hashes []string) error {
	if chunkSize < 0 || newSize < 0 || (chunkSize == 0 && len(chunks) != 0) {
		return errors.New("invalid chunk size or file size")
	}
	resolved, inode, err := c.resolvePath(ctx, p, true)
	creating := errors.Is(err, redis.Nil)
	if err != nil && !creating {
		return err
	}
	if creating {
		if chunkSize == 0 && newSize > 0 {
			return errors.New("missing complete chunks for new file")
		}
		p = normalizePath(p)
		if err := c.ensureParents(ctx, p); err != nil {
			return err
		}
		_, parent, err := c.resolvePath(ctx, parentOf(p), true)
		if err != nil {
			return err
		}
		id, err := c.allocInodeID(ctx)
		if err != nil {
			return err
		}
		now := nowMs()
		inode = &inodeData{ID: id, Parent: parent.ID, Name: baseName(p), Type: "file", Mode: 0o644, CtimeMs: now, MtimeMs: now, AtimeMs: now}
		resolved = p
		// A new file has no unchanged chunks to copy.
		for off := int64(0); off < newSize; off += int64(chunkSize) {
			idx := int(off / int64(chunkSize))
			data, ok := chunks[idx]
			want := int64(chunkSize)
			if newSize-off < want {
				want = newSize - off
			}
			if !ok || int64(len(data)) != want {
				return fmt.Errorf("missing complete chunk %d for new file", idx)
			}
		}
	}
	if inode.Type != "file" {
		return ErrNotFile
	}
	if err := checkWriteCondition(ctx, inode, creating); err != nil {
		return err
	}
	before, err := c.versionedSnapshotFromResolved(ctx, resolved, inode)
	if err != nil {
		return err
	}
	originalRef := inode.ContentRef
	if err := c.selectContentRef(ctx, inode); err != nil {
		return err
	}
	stage := c.keys.content(inode.ID) + ":stage:" + newOriginID()
	defer c.discardStage(stage)
	if !creating && isExternalContentRef(originalRef) && originalRef == inode.ContentRef {
		// COPY stays inside Redis, preserving delta upload bandwidth.
		pipe := c.rdb.TxPipeline()
		copied := pipe.Copy(ctx, c.keys.content(inode.ID), stage, c.rdb.Options().DB, true)
		pipe.Expire(ctx, stage, publicationTTL)
		if _, err := pipe.Exec(ctx); err != nil {
			return err
		}
		if copied.Val() == 0 && inode.Size > 0 {
			return ErrWriteConflict
		}
	} else {
		if !creating {
			inode.Content, err = c.loadContentExternal(ctx, inode.ID, originalRef)
			if err != nil {
				return err
			}
		}
		pipe := c.rdb.TxPipeline()
		rediscontent.QueueWriteFull(ctx, pipe, stage, inode.ContentRef, []byte(inode.Content))
		pipe.Expire(ctx, stage, publicationTTL)
		if _, err := pipe.Exec(ctx); err != nil {
			return err
		}
	}
	if inode.ContentRef == rediscontent.RefArray && inode.Size == 0 {
		// Seed an expiring empty Array before individual range writes. A
		// crash while creating a large Array must not leave immortal staging.
		pipe := c.rdb.TxPipeline()
		pipe.Do(ctx, "ARSET", stage, 0, "")
		pipe.Expire(ctx, stage, publicationTTL)
		if _, err := pipe.Exec(ctx); err != nil {
			return err
		}
	}
	indices := make([]int, 0, len(chunks))
	for idx := range chunks {
		indices = append(indices, idx)
	}
	sort.Ints(indices)
	if inode.ContentRef == rediscontent.RefArray {
		for _, idx := range indices {
			if idx < 0 {
				return errors.New("negative chunk index")
			}
			if err := rediscontent.WriteRange(ctx, c.rdb, stage, int64(idx)*int64(chunkSize), chunks[idx]); err != nil {
				return err
			}
		}
		if newSize < inode.Size {
			if err := rediscontent.Truncate(ctx, c.rdb, stage, inode.Size, newSize); err != nil {
				return err
			}
		}
	} else {
		pipe := c.rdb.Pipeline()
		for _, idx := range indices {
			if idx < 0 {
				return errors.New("negative chunk index")
			}
			pipe.SetRange(ctx, stage, int64(idx)*int64(chunkSize), string(chunks[idx]))
		}
		pipe.Expire(ctx, stage, publicationTTL)
		if _, err := pipe.Exec(ctx); err != nil {
			return err
		}
		if newSize < inode.Size {
			data := ""
			if newSize > 0 {
				data, err = c.rdb.GetRange(ctx, stage, 0, newSize-1).Result()
				if err != nil {
					return err
				}
			}
			if err := c.rdb.Set(ctx, stage, data, publicationTTL).Err(); err != nil {
				return err
			}
		}
	}
	// COPY of an empty file may have no source key; truncating an Array to
	// zero deletes its stage. Keep an empty marker for the publication to consume.
	if newSize == 0 {
		if err := c.rdb.Set(ctx, stage, "", publicationTTL).Err(); err != nil {
			return err
		}
	} else if err := c.rdb.Expire(ctx, stage, publicationTTL).Err(); err != nil {
		return err
	}
	fields := map[string]interface{}{}
	hashJSON, _ := encodeChunkHashes(hashes)
	fields["chunk_size"] = chunkSize
	fields["chunk_hashes"] = hashJSON
	inode.Size = newSize
	inode.MtimeMs = nowMs()
	inode.AtimeMs = inode.MtimeMs
	if err := c.publishStagedFile(ctx, resolved, inode, stage, creating, fields); err != nil {
		return err
	}
	c.invalidateInode(ctx, resolved)
	if creating {
		c.invalidateDirListing(ctx, parentOf(resolved))
	}
	after, err := c.versionedSnapshotFromResolved(ctx, resolved, inode)
	if err != nil {
		return err
	}
	return c.recordVersionMutation(ctx, before, after)
}

var deleteInodeScript = redis.NewScript(`
if ARGV[4] ~= '' and redis.call('GET',KEYS[7]) ~= ARGV[4] then return -2 end
if KEYS[11] ~= KEYS[7] and redis.call('EXISTS',KEYS[11]) == 0 then return -5 end
local dirtytype=redis.call('TYPE',KEYS[12]).ok
if dirtytype~='none' and dirtytype~='set' then return -4 end
local streamtype=redis.call('TYPE',KEYS[9]).ok
if streamtype~='none' and streamtype~='stream' then return -4 end
local linked = redis.call('HGET',KEYS[3],ARGV[2])
if not linked and redis.call('EXISTS',KEYS[1]) == 0 then return 2 end
if linked ~= ARGV[1] or (redis.call('HGET',KEYS[1],'revision') or '') ~= ARGV[3] then return -1 end
local kind = redis.call('HGET',KEYS[1],'type')
if kind == 'dir' then
 local children=redis.call('HGETALL',KEYS[4])
 for i=1,#children,2 do
  if redis.call('EXISTS',ARGV[6]..children[i+1]) == 1 then return -3 end
 end
end
local size=tonumber(redis.call('HGET',KEYS[1],'size') or '0')
redis.call('DEL',KEYS[1],KEYS[2],KEYS[4])
redis.call('HDEL',KEYS[3],ARGV[2])
redis.call('HSET',KEYS[6],'mtime_ms',ARGV[5],'ctime_ms',ARGV[5])
local counter='files'
if kind=='dir' then counter='directories' elseif kind=='symlink' then counter='symlinks' end
redis.call('HINCRBY',KEYS[5],counter,-1)
-- Redis 7 serializes negative zero as '-0', which HINCRBY rejects.
if kind=='file' and size>0 then redis.call('HINCRBY',KEYS[5],'total_data_bytes',-size) end
redis.call('SET',KEYS[8],'1')
redis.call('SADD',KEYS[12],ARGV[1])
if ARGV[7] ~= '' then
 redis.call('XADD',KEYS[9],'MAXLEN','~',10000,'*','payload',ARGV[7])
 if KEYS[10]~=KEYS[7] then redis.call('PUBLISH',KEYS[10],ARGV[7]) end
end
return 1
`)

func (c *nativeClient) deletePublishedInode(ctx context.Context, p string, inode *inodeData) error {
	if err := checkExpectedParent(ctx, p, inode.Parent); err != nil {
		return err
	}
	if err := checkWriteCondition(ctx, inode, false); err != nil {
		return err
	}
	generation, _ := ctx.Value(workspaceGenerationKey{}).(string)
	code, err := c.runMutationScript(ctx, deleteInodeScript, false, []string{c.keys.inode(inode.ID), c.keys.content(inode.ID), c.keys.dirents(inode.Parent), c.keys.dirents(inode.ID), c.keys.info(), c.keys.inode(inode.Parent), c.keys.generation(), c.keys.rootDirty(), c.keys.changesStream(), c.notificationChannel(), c.leaseGuardKey(ctx), queryindex.DirtySetKey(c.key)}, inode.ID, inode.Name, inode.Revision, generation, nowMs(), c.keys.inodePrefix(), c.invalidationPayload(InvalidateOpInode, p))
	if err != nil {
		return err
	}
	switch code {
	case -1:
		return ErrWriteConflict
	case -2:
		return ErrWorkspaceChanged
	case -3:
		return ErrDirNotEmpty
	case -4:
		return errors.New("workspace change journal has wrong Redis type")
	case -5:
		return ErrNativeSessionLost
	}
	return nil
}

var updateInodeScript = redis.NewScript(`
if ARGV[4] ~= '' and redis.call('GET',KEYS[3]) ~= ARGV[4] then return -2 end
if KEYS[6] ~= KEYS[3] and redis.call('EXISTS',KEYS[6]) == 0 then return -5 end
local streamtype=redis.call('TYPE',KEYS[4]).ok
if streamtype~='none' and streamtype~='stream' then return -4 end
local current=redis.call('HGET',KEYS[1],'revision') or ''
if current==ARGV[5] then return 2 end
if redis.call('EXISTS',KEYS[1]) == 0 then return -1 end
if ARGV[1] ~= '1' and redis.call('HGET',KEYS[2],ARGV[2]) ~= ARGV[1] then return -1 end
if current~=ARGV[3] then return -1 end
for i=7,#ARGV,2 do redis.call('HSET',KEYS[1],ARGV[i],ARGV[i+1]) end
redis.call('HSET',KEYS[1],'revision',ARGV[5])
redis.call('SET',KEYS[7],'1')
if ARGV[6] ~= '' then
 redis.call('XADD',KEYS[4],'MAXLEN','~',10000,'*','payload',ARGV[6])
 if KEYS[5]~=KEYS[3] then redis.call('PUBLISH',KEYS[5],ARGV[6]) end
end
return 1
`)

func (c *nativeClient) updatePublishedInode(ctx context.Context, p string, inode *inodeData, fields map[string]interface{}) error {
	if err := checkExpectedParent(ctx, p, inode.Parent); err != nil {
		return err
	}
	if err := checkWriteCondition(ctx, inode, false); err != nil {
		return err
	}
	revision := newOriginID()
	generation, _ := ctx.Value(workspaceGenerationKey{}).(string)
	args := []interface{}{inode.ID, inode.Name, inode.Revision, generation, revision, c.invalidationPayload(InvalidateOpInode, p)}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		args = append(args, name, fields[name])
	}
	code, err := c.runMutationScript(ctx, updateInodeScript, true, []string{c.keys.inode(inode.ID), c.keys.dirents(inode.Parent), c.keys.generation(), c.keys.changesStream(), c.notificationChannel(), c.leaseGuardKey(ctx), c.keys.rootDirty()}, args...)
	if err != nil {
		return err
	}
	switch code {
	case -1:
		return ErrWriteConflict
	case -2:
		return ErrWorkspaceChanged
	case -4:
		return errors.New("workspace change journal has wrong Redis type")
	case -5:
		return ErrNativeSessionLost
	}
	inode.Revision = revision
	captureMutationResult(ctx, inode, revision)
	return nil
}
