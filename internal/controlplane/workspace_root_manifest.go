package controlplane

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
)

// mountedArtifactNames are OS-generated filesystem artifacts that should be
// excluded when building a manifest from a live workspace root.
var mountedArtifactNames = map[string]struct{}{
	".DS_Store":       {},
	".Spotlight-V100": {},
	".TemporaryItems": {},
	".Trashes":        {},
	".fseventsd":      {},
	".nfs-check":      {},
}

// BuildManifestFromWorkspaceRoot reads the live workspace root in Redis via
// pipelined BFS and returns a Manifest plus any blob data that exceeds the
// inline threshold. This is the server-side equivalent of the client-side
// buildManifestFromWorkspaceRoot in cmd/afs/workspace_mount_bridge.go.
func BuildManifestFromWorkspaceRoot(ctx context.Context, rdb *redis.Client, workspace, savepoint string) (Manifest, map[string][]byte, int, int, int64, error) {
	fsKey := WorkspaceFSKey(workspace)
	entries := make(map[string]ManifestEntry)
	blobs := make(map[string][]byte)
	var fileCount, dirCount int
	var totalBytes int64

	type bfsItem struct {
		inodeID string
		path    string
	}
	queue := []bfsItem{{workspaceFSRootInodeID, "/"}}

	for len(queue) > 0 {
		if ctx.Err() != nil {
			return Manifest{}, nil, 0, 0, 0, ctx.Err()
		}

		// Pipeline: read all inodes in this BFS level.
		pipe := rdb.Pipeline()
		inodeCmds := make([]*redis.SliceCmd, len(queue))
		for i, item := range queue {
			inodeCmds[i] = pipe.HMGet(ctx, workspaceFSInodeKey(fsKey, item.inodeID),
				"type", "mode", "size", "mtime_ms", "target", "content", "content_ref")
		}
		if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
			return Manifest{}, nil, 0, 0, 0, err
		}

		// For inodes with content_ref="ext", fetch content from separate key.
		type extContent struct {
			idx int
			cmd *redis.StringCmd
		}
		type parsedInode struct {
			typ        string
			mode       uint32
			size       int64
			mtimeMs    int64
			target     string
			content    string
			contentRef string
		}

		parsed := make([]*parsedInode, len(queue))
		for i := range queue {
			values, err := inodeCmds[i].Result()
			if err != nil || len(values) < 6 || values[0] == nil {
				continue
			}
			p := &parsedInode{
				typ:     redisString(values[0]),
				mode:    uint32(redisInt64(values[1])),
				size:    redisInt64(values[2]),
				mtimeMs: redisInt64(values[3]),
				target:  redisString(values[4]),
				content: redisString(values[5]),
			}
			if len(values) > 6 {
				p.contentRef = redisString(values[6])
			}
			parsed[i] = p
		}

		// Second pipeline: fetch external content.
		var extFetches []extContent
		pipe2 := rdb.Pipeline()
		for i, p := range parsed {
			if p != nil && p.typ == "file" && p.contentRef == "ext" {
				cmd := pipe2.Get(ctx, workspaceFSContentKey(fsKey, queue[i].inodeID))
				extFetches = append(extFetches, extContent{idx: i, cmd: cmd})
			}
		}
		if len(extFetches) > 0 {
			if _, err := pipe2.Exec(ctx); err != nil && err != redis.Nil {
				return Manifest{}, nil, 0, 0, 0, err
			}
			for _, ef := range extFetches {
				if v, err := ef.cmd.Result(); err == nil {
					parsed[ef.idx].content = v
				}
			}
		}

		var dirItems []bfsItem
		for i, p := range parsed {
			if p == nil {
				continue
			}
			item := queue[i]
			if shouldIgnoreMountPath(item.path) {
				continue
			}

			entry := ManifestEntry{
				Type:    p.typ,
				Mode:    p.mode,
				MtimeMs: p.mtimeMs,
				Size:    p.size,
			}
			switch p.typ {
			case "dir":
				entry.Size = 0
				entries[item.path] = entry
				if item.path != "/" {
					dirCount++
				}
				dirItems = append(dirItems, item)
			case "file":
				data := []byte(p.content)
				entry.Size = int64(len(data))
				fileCount++
				totalBytes += int64(len(data))
				if len(data) <= inlineThreshold {
					entry.Inline = base64.StdEncoding.EncodeToString(data)
				} else {
					entry.BlobID = blobSHA256(data)
					if _, ok := blobs[entry.BlobID]; !ok {
						blobs[entry.BlobID] = data
					}
				}
				entries[item.path] = entry
			case "symlink":
				entry.Target = p.target
				entry.Size = int64(len(p.target))
				entries[item.path] = entry
			}
		}

		// Pipeline: read dirents for all directories in this level.
		if len(dirItems) == 0 {
			break
		}
		pipe = rdb.Pipeline()
		direntCmds := make([]*redis.MapStringStringCmd, len(dirItems))
		for i, item := range dirItems {
			direntCmds[i] = pipe.HGetAll(ctx, workspaceFSDirentsKey(fsKey, item.inodeID))
		}
		if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
			return Manifest{}, nil, 0, 0, 0, err
		}

		queue = queue[:0]
		for i, item := range dirItems {
			children, err := direntCmds[i].Result()
			if err != nil {
				continue
			}
			childNames := make([]string, 0, len(children))
			for name := range children {
				childNames = append(childNames, name)
			}
			sort.Strings(childNames)
			for _, name := range childNames {
				childPath := item.path
				if childPath == "/" {
					childPath = "/" + name
				} else {
					childPath = item.path + "/" + name
				}
				queue = append(queue, bfsItem{
					inodeID: children[name],
					path:    childPath,
				})
			}
		}
	}

	if _, ok := entries["/"]; !ok {
		entries["/"] = ManifestEntry{Type: "dir", Mode: 0o755}
	}

	return Manifest{
		Version:   formatVersion,
		Workspace: workspace,
		Savepoint: savepoint,
		Entries:   entries,
	}, blobs, fileCount, dirCount, totalBytes, nil
}

// workspaceFSContentKey returns the Redis key for external file content.
func workspaceFSContentKey(fsKey, inodeID string) string {
	return "afs:{" + fsKey + "}:content:" + inodeID
}

func shouldIgnoreMountPath(p string) bool {
	if p == "" || p == "/" {
		return false
	}
	trimmed := strings.Trim(p, "/")
	if trimmed == "" {
		return false
	}
	for _, part := range strings.Split(trimmed, "/") {
		if strings.HasPrefix(part, "._") {
			return true
		}
		if _, ok := mountedArtifactNames[part]; ok {
			return true
		}
	}
	return false
}

func blobSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func redisString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func redisInt64(v interface{}) int64 {
	s := redisString(v)
	if s == "" {
		return 0
	}
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

// joinWorkspaceRootPath joins a parent path and child name for workspace root paths.
func joinWorkspaceRootPath(parentPath, name string) string {
	if parentPath == "/" {
		return "/" + name
	}
	return path.Join(parentPath, name)
}
