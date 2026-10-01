package client

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"

	"github.com/redis/agent-filesystem/internal/rediscontent"
	"github.com/redis/go-redis/v9"
)

type nativeRangeKey struct{}

// Resolve live parent/name links in one Redis command. Stored indexed paths
// are hints updated after rename and must not identify an NFS file handle.
var inodePathScript = redis.NewScript(`
if ARGV[4] ~= '' and redis.call('GET',KEYS[2]) ~= ARGV[4] then return -2 end
if KEYS[3] ~= KEYS[2] and redis.call('EXISTS',KEYS[3]) == 0 then return -5 end
local id=ARGV[3]
local parts={}
local seen={}
while id~='1' do
 if seen[id] or #parts>=4096 then return -1 end
 seen[id]=true
 local parent=redis.call('HGET',ARGV[1]..id,'parent')
 local name=redis.call('HGET',ARGV[1]..id,'name')
 if not parent or not name or redis.call('HGET',ARGV[2]..parent,name)~=id then return -1 end
 table.insert(parts,1,name)
 id=parent
end
if redis.call('EXISTS',ARGV[1]..'1')==0 then return -1 end
return '/'..table.concat(parts,'/')
`)

func (c *nativeClient) InodePath(ctx context.Context, inode uint64) (string, error) {
	generation, _ := ctx.Value(workspaceGenerationKey{}).(string)
	result, err := inodePathScript.Run(ctx, c.rdb, []string{c.keys.inode(strconv.FormatUint(inode, 10)), c.keys.generation(), c.leaseGuardKey(ctx)}, c.keys.inodePrefix(), c.keys.direntsPrefix(), strconv.FormatUint(inode, 10), generation).Result()
	if err != nil {
		return "", err
	}
	if path, ok := result.(string); ok {
		return path, nil
	}
	if code, ok := result.(int64); ok {
		if code == -2 {
			return "", ErrWorkspaceChanged
		}
		if code == -5 {
			return "", ErrNativeSessionLost
		}
	}
	return "", ErrNotFound
}

func (c *nativeClient) StatInode(ctx context.Context, inode uint64) (*StatResult, error) {
	if err := c.checkGeneration(ctx); err != nil {
		return nil, err
	}
	data, err := c.loadInodeByID(ctx, strconv.FormatUint(inode, 10))
	if err != nil || data == nil {
		return nil, err
	}
	return data.toStat(), nil
}

func (c *nativeClient) ReadInodeAt(ctx context.Context, inode uint64, off int64, size int) ([]byte, error) {
	if off < 0 || size < 0 || int64(size) > math.MaxInt64-off {
		return nil, errors.New("invalid range")
	}
	id := strconv.FormatUint(inode, 10)
	for attempt := 0; attempt < 16; attempt++ {
		if err := c.checkGeneration(ctx); err != nil {
			return nil, err
		}
		data, err := c.loadInodeByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if data == nil {
			return nil, ErrNotFound
		}
		if data.Type != "file" {
			return nil, ErrNotFile
		}
		var result []byte
		if off < data.Size && size != 0 {
			if isExternalContentRef(data.ContentRef) {
				result, err = rediscontent.ReadRange(ctx, c.rdb, c.keys.content(id), data.ContentRef, data.Size, off, size)
			} else {
				var raw string
				raw, err = c.loadContentExternal(ctx, id, data.ContentRef)
				if off < int64(len(raw)) {
					result = []byte(raw[off:min(int64(len(raw)), off+int64(size))])
				}
			}
		}
		// Metadata and range bytes must describe one publication. This also
		// covers multi-command Array reads and a concurrent content-ref change.
		current, readErr := c.loadInodeByID(ctx, id)
		if readErr != nil {
			return nil, readErr
		}
		if current == nil {
			return nil, ErrNotFound
		}
		if current.Revision != data.Revision {
			continue
		}
		if err != nil {
			return nil, err
		}
		if err := c.checkGeneration(ctx); err != nil {
			return nil, err
		}
		return result, nil
	}
	return nil, ErrWriteConflict
}

func (c *nativeClient) WriteInodeAt(ctx context.Context, inode uint64, payload []byte, off int64) error {
	return c.WriteInodeAtPath(ctx, inode, "", payload, off)
}

func (c *nativeClient) WriteInodeAtPath(ctx context.Context, inode uint64, path string, payload []byte, off int64) error {
	if off < -1 || (off >= 0 && int64(len(payload)) > math.MaxInt64-off) {
		return errors.New("invalid range")
	}
	return c.changeInodeRange(ctx, inode, path, payload, off, -1)
}

func (c *nativeClient) TruncateInode(ctx context.Context, inode uint64, size int64) error {
	return c.TruncateInodeAtPath(ctx, inode, "", size)
}

func (c *nativeClient) TruncateInodeAtPath(ctx context.Context, inode uint64, path string, size int64) error {
	if size < 0 {
		return errors.New("invalid size")
	}
	return c.changeInodeRange(ctx, inode, path, nil, 0, size)
}

// Range requests are operations, not full-file snapshots. A rejected CAS can
// safely reapply the same range to a fresh private copy. Explicit expected-stat
// callers instead keep their original conflict boundary, like folder sync.
func (c *nativeClient) changeInodeRange(ctx context.Context, inode uint64, pathHint string, payload []byte, off, truncate int64) error {
	id := strconv.FormatUint(inode, 10)
	ctx = context.WithValue(ctx, nativeRangeKey{}, true)
	for attempt := 0; attempt < 32; attempt++ {
		if err := c.checkGeneration(ctx); err != nil {
			return err
		}
		data, err := c.loadInodeByID(ctx, id)
		if err != nil {
			return err
		}
		if data == nil {
			return ErrNotFound
		}
		if data.Type != "file" {
			return ErrNotFile
		}
		if err := checkWriteCondition(ctx, data, false); err != nil {
			return err
		}
		// The inode's current path wins over an open handle's stale hint. The
		// inode/link checks in publication still prevent resurrection on unlink.
		p, err := c.InodePath(ctx, inode)
		if err != nil {
			return err
		}
		oldSize := data.Size
		writeOffset := off
		if writeOffset == -1 {
			writeOffset = oldSize
		}
		if int64(len(payload)) > math.MaxInt64-writeOffset {
			return errors.New("invalid range")
		}
		newSize := oldSize
		if truncate >= 0 {
			newSize = truncate
		} else if len(payload) > 0 {
			newSize = max(newSize, writeOffset+int64(len(payload)))
		}
		if (truncate >= 0 && newSize == oldSize) || (truncate < 0 && len(payload) == 0) {
			return nil
		}
		before, err := c.versionedSnapshotFromResolved(ctx, p, data)
		if err != nil {
			return err
		}
		var after VersionedSnapshot
		stage, err := c.stageInodeRange(ctx, data, payload, writeOffset, newSize)
		if err == nil {
			data.Size = newSize
			data.MtimeMs = nowMs()
			data.AtimeMs = data.MtimeMs
			data.CtimeMs = data.MtimeMs
			if c.observer != nil {
				content, readErr := rediscontent.Load(ctx, c.rdb, stage, data.ContentRef, newSize)
				if readErr != nil {
					c.discardStage(stage)
					return readErr
				}
				after = VersionedSnapshot{Path: p, Exists: true, Kind: "file", Mode: data.Mode, Content: content, SizeBytes: newSize}
			}
			// A byte-range update invalidates hashes from the previous complete
			// chunk upload; retaining them could make a sync client skip changed data.
			err = c.publishStagedFile(ctx, p, data, stage, false, map[string]interface{}{"chunk_size": 0, "chunk_hashes": ""})
		}
		if stage != "" {
			c.discardStage(stage)
		}
		if err == nil {
			// Keep warm ancestor/path caches: only this inode's attributes and
			// its parent's listing became stale. A supplied old handle path is
			// invalidated too, but is never used to publish or recache an inode.
			if c.cache != nil {
				c.cache.Invalidate(p)
				c.cache.Invalidate(dirCacheKey(parentOf(p)))
				if pathHint != "" && pathHint != p {
					c.cache.Invalidate(normalizePath(pathHint))
				}
			}
			return c.recordVersionMutation(ctx, before, after)
		}
		if !errors.Is(err, ErrWriteConflict) {
			return err
		}
		if _, conditional := ctx.Value(expectedStatKey{}).(expectedStat); conditional {
			return err
		}
	}
	return ErrWriteConflict
}

// Redis COPY keeps unchanged bytes on the server. All writes target a unique,
// expiring stage; only the shared revision-checked publication touches live data.
func (c *nativeClient) stageInodeRange(ctx context.Context, inode *inodeData, payload []byte, off, newSize int64) (string, error) {
	stage := c.keys.content(inode.ID) + ":stage:" + newOriginID()
	originalRef := inode.ContentRef
	if err := c.selectContentRef(ctx, inode); err != nil {
		return stage, err
	}
	pipe := c.rdb.TxPipeline()
	if isExternalContentRef(originalRef) && originalRef == inode.ContentRef && inode.Size > 0 {
		copy := pipe.Copy(ctx, c.keys.content(inode.ID), stage, c.rdb.Options().DB, true)
		pipe.Expire(ctx, stage, publicationTTL)
		if _, err := pipe.Exec(ctx); err != nil {
			return stage, err
		}
		if copy.Val() == 0 {
			return stage, ErrWriteConflict
		}
	} else {
		var content string
		var err error
		if inode.Size > 0 {
			content, err = c.loadContentExternal(ctx, inode.ID, originalRef)
		}
		if err != nil {
			return stage, c.rangeStageError(ctx, inode, err)
		}
		if inode.ContentRef == rediscontent.RefArray && content == "" {
			pipe.Do(ctx, "ARSET", stage, 0, "")
		} else {
			rediscontent.QueueWriteFull(ctx, pipe, stage, inode.ContentRef, []byte(content))
		}
		pipe.Expire(ctx, stage, publicationTTL)
		if _, err := pipe.Exec(ctx); err != nil {
			return stage, err
		}
	}
	if inode.ContentRef == rediscontent.RefArray {
		if len(payload) > 0 {
			if err := rediscontent.WriteRangeWithTTL(ctx, c.rdb, stage, off, payload, publicationTTL); err != nil {
				return stage, c.rangeStageError(ctx, inode, err)
			}
		}
		if newSize < inode.Size {
			if err := rediscontent.TruncateWithTTL(ctx, c.rdb, stage, inode.Size, newSize, publicationTTL); err != nil {
				return stage, err
			}
		}
	} else {
		if len(payload) > 0 {
			if err := c.writeStringStageRange(ctx, stage, off, string(payload)); err != nil {
				return stage, c.rangeStageError(ctx, inode, err)
			}
		}
		if newSize < inode.Size {
			// Shrink inside Redis, without downloading the retained prefix.
			if err := shrinkStringStage.Run(ctx, c.rdb, []string{stage}, newSize, publicationTTL.Milliseconds()).Err(); err != nil {
				return stage, err
			}
		} else if newSize > inode.Size && len(payload) == 0 {
			if err := c.writeStringStageRange(ctx, stage, newSize-1, "\x00"); err != nil {
				return stage, err
			}
		}
	}
	if newSize == 0 {
		if err := c.rdb.Set(ctx, stage, "", publicationTTL).Err(); err != nil {
			return stage, err
		}
	}
	return stage, nil
}

func (c *nativeClient) writeStringStageRange(ctx context.Context, stage string, off int64, payload string) error {
	_, err := c.rdb.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.SetRange(ctx, stage, off, payload)
		pipe.Expire(ctx, stage, publicationTTL)
		return nil
	})
	return err
}

var shrinkStringStage = redis.NewScript(`
local size=tonumber(ARGV[1])
local data=''
if size>0 then data=redis.call('GETRANGE',KEYS[1],0,size-1) end
redis.call('SET',KEYS[1],data,'PX',ARGV[2])
return 1
`)

func (c *nativeClient) rangeStageError(ctx context.Context, inode *inodeData, cause error) error {
	if strings.Contains(cause.Error(), "WRONGTYPE") {
		current, err := c.loadInodeByID(ctx, inode.ID)
		if err == nil && (current == nil || current.Revision != inode.Revision) {
			return ErrWriteConflict
		}
	}
	return cause
}
