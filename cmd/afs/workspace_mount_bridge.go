package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/redis/agent-filesystem/internal/controlplane"
	"github.com/redis/agent-filesystem/internal/rediscontent"
	"github.com/redis/go-redis/v9"
)

const workspaceMountTreeMaxDepth = 4096

var mountedArtifactNames = map[string]struct{}{
	".DS_Store":       {},
	".Spotlight-V100": {},
	".TemporaryItems": {},
	".Trashes":        {},
	".fseventsd":      {},
	".nfs-check":      {},
}

func mountRedisKeyForWorkspace(workspace string) string {
	return workspaceRedisKey(workspace)
}

func ensureMountWorkspace(ctx context.Context, cfg config, store *afsStore) (string, error) {
	workspace := strings.TrimSpace(cfg.CurrentWorkspace)
	if workspace == "" {
		return "", errors.New("workspace is required")
	}
	if err := validateAFSName("workspace", workspace); err != nil {
		return "", err
	}

	exists, err := store.workspaceExists(ctx, workspace)
	if err != nil {
		return "", err
	}
	if exists {
		return workspace, nil
	}
	return "", fmt.Errorf("workspace %q does not exist", workspace)
}

func seedWorkspaceMountKey(ctx context.Context, store *afsStore, workspace string) (string, string, bool, error) {
	mountKey, headSavepoint, initialized, err := store.ensureWorkspaceRoot(ctx, workspace)
	if err != nil {
		return "", "", false, err
	}
	searchRDB := newSearchRedisClient(store.rdb)
	defer searchRDB.Close()
	_, _ = ensureWorkspaceSearchIndex(ctx, searchRDB, mountKey)
	return mountKey, headSavepoint, initialized, nil
}

func saveWorkspaceRootCheckpoint(ctx context.Context, store *afsStore, workspace, expectedHead, savepointID string, options ...controlplane.SaveCheckpointFromLiveOptions) (bool, error) {
	redisKey, err := store.resolveWorkspaceRedisKey(ctx, workspace)
	if err != nil {
		return false, err
	}
	rootManifest, blobs, stats, err := buildManifestFromWorkspaceRoot(ctx, store.rdb, redisKey, workspace, savepointID)
	if err != nil {
		return false, err
	}

	saved, err := saveAFSManifest(ctx, store, workspace, expectedHead, savepointID, rootManifest, blobs, stats, false, options...)
	if err != nil {
		return false, err
	}
	if !saved {
		if err := store.markWorkspaceRootClean(ctx, workspace, expectedHead); err != nil {
			return false, err
		}
	}
	return saved, nil
}

func buildManifestFromWorkspaceRoot(ctx context.Context, rdb *redis.Client, fsKey, workspace, savepoint string) (manifest, map[string][]byte, manifestStats, error) {
	return buildManifestFromWorkspaceRootWithProgress(ctx, rdb, fsKey, workspace, savepoint, nil)
}

// buildManifestFromWorkspaceRootWithProgress reads the entire workspace tree
// using pipelined BFS — one Redis pipeline per tree depth level. This replaces
// the sequential recursive walk (one HMGet per inode) and is orders of magnitude
// faster on WAN: a tree of depth 10 takes ~10 round trips regardless of file
// count, vs. N round trips in the old code.
//
// onProgress is called with (entriesProcessed, -1) during the walk so callers
// can show a live counter. Pass nil to skip.
func buildManifestFromWorkspaceRootWithProgress(ctx context.Context, rdb *redis.Client, fsKey, workspace, savepoint string, onProgress func(int64, int64)) (manifest, map[string][]byte, manifestStats, error) {
	entries := make(map[string]manifestEntry)
	blobs := make(map[string][]byte)
	stats := manifestStats{}
	var processed int64

	report := func() {
		processed++
		if onProgress != nil {
			onProgress(processed, -1)
		}
	}

	// BFS queue: each item is (inodeID, path).
	type bfsItem struct {
		inodeID string
		path    string
	}
	queue := []bfsItem{{workspaceFSRootInodeID, "/"}}

	for len(queue) > 0 {
		if ctx.Err() != nil {
			return manifest{}, nil, manifestStats{}, ctx.Err()
		}

		// Pipeline: read all inodes in this BFS level. We read content_ref
		// to determine whether content is inline or in a separate STRING key.
		pipe := rdb.Pipeline()
		inodeCmds := make([]*redis.SliceCmd, len(queue))
		for i, item := range queue {
			inodeCmds[i] = pipe.HMGet(ctx, workspaceRootInodeKey(fsKey, item.inodeID),
				"type", "mode", "size", "mtime_ms", "target", "content", "content_ref")
		}
		if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
			return manifest{}, nil, manifestStats{}, err
		}

		// For externally stored file content, fetch bytes from the dedicated
		// content key after the inode metadata pass.
		type externalContent struct {
			idx int
			id  string
		}
		var externalFetches []externalContent

		// Collect directories that need their children enumerated.
		var dirItems []bfsItem
		type parsedInode struct {
			inode *workspaceRootInode
			item  bfsItem
		}
		parsedInodes := make([]parsedInode, 0, len(queue))
		for i, item := range queue {
			values, err := inodeCmds[i].Result()
			if err != nil || len(values) < 6 || values[0] == nil {
				parsedInodes = append(parsedInodes, parsedInode{})
				continue
			}
			inode := &workspaceRootInode{
				Type:    workspaceRootString(values[0]),
				Mode:    uint32(workspaceRootInt64(values[1])),
				Size:    workspaceRootInt64(values[2]),
				MtimeMs: workspaceRootInt64(values[3]),
				Target:  workspaceRootString(values[4]),
				Content: workspaceRootString(values[5]),
			}
			// Check content_ref (index 6) for external content.
			contentRef := ""
			if len(values) > 6 {
				contentRef = workspaceRootString(values[6])
			}
			inode.ContentRef = contentRef
			parsedInodes = append(parsedInodes, parsedInode{inode: inode, item: item})
		}

		// Second pass: fetch external content.
		if len(parsedInodes) > 0 {
			for i, p := range parsedInodes {
				if p.inode != nil && p.inode.Type == "file" && p.inode.ContentRef != "" {
					externalFetches = append(externalFetches, externalContent{idx: i, id: queue[i].inodeID})
				}
			}
			for _, fetch := range externalFetches {
				content, err := rediscontent.Load(ctx, rdb, workspaceRootContentKey(fsKey, fetch.id), parsedInodes[fetch.idx].inode.ContentRef, parsedInodes[fetch.idx].inode.Size)
				if err != nil {
					return manifest{}, nil, manifestStats{}, err
				}
				parsedInodes[fetch.idx].inode.Content = string(content)
			}
		}

		for i, p := range parsedInodes {
			if p.inode == nil {
				continue
			}
			item := queue[i]
			inode := p.inode
			if shouldIgnoreMountedPath(item.path) {
				continue
			}
			report()

			entry := manifestEntry{
				Type:    inode.Type,
				Mode:    inode.Mode,
				MtimeMs: inode.MtimeMs,
				Size:    inode.Size,
			}
			switch inode.Type {
			case "dir":
				entry.Size = 0
				entries[item.path] = entry
				if item.path != "/" {
					stats.DirCount++
				}
				dirItems = append(dirItems, item)
			case "file":
				data := []byte(inode.Content)
				entry.Size = int64(len(data))
				stats.FileCount++
				stats.TotalBytes += int64(len(data))
				if len(data) <= afsInlineThreshold {
					entry.Inline = base64.StdEncoding.EncodeToString(data)
				} else {
					entry.BlobID = sha256Hex(data)
					if _, ok := blobs[entry.BlobID]; !ok {
						blobs[entry.BlobID] = data
					}
				}
				entries[item.path] = entry
			case "symlink":
				entry.Target = inode.Target
				entry.Size = int64(len(inode.Target))
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
			direntCmds[i] = pipe.HGetAll(ctx, workspaceRootDirentsKey(fsKey, item.inodeID))
		}
		if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
			return manifest{}, nil, manifestStats{}, err
		}

		// Build next BFS level from children.
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
				queue = append(queue, bfsItem{
					inodeID: children[name],
					path:    joinWorkspaceRootPath(item.path, name),
				})
			}
		}
	}

	if _, ok := entries["/"]; !ok {
		entries["/"] = manifestEntry{Type: "dir", Mode: 0o755}
	}

	return manifest{
		Version:   afsFormatVersion,
		Workspace: workspace,
		Savepoint: savepoint,
		Entries:   entries,
	}, blobs, stats, nil
}

const workspaceFSRootInodeID = "1"

type workspaceRootInode struct {
	Type       string
	Mode       uint32
	Size       int64
	MtimeMs    int64
	Target     string
	Content    string
	ContentRef string
}

func appendWorkspaceRootManifest(ctx context.Context, rdb *redis.Client, fsKey, inodeID, currentPath string, entries map[string]manifestEntry, blobs map[string][]byte, stats *manifestStats) error {
	inode, err := loadWorkspaceRootInode(ctx, rdb, fsKey, inodeID)
	if err != nil {
		return err
	}
	if inode == nil {
		return fmt.Errorf("workspace root inode %q at %q is missing", inodeID, currentPath)
	}
	if shouldIgnoreMountedPath(currentPath) {
		return nil
	}

	entry := manifestEntry{
		Type:    inode.Type,
		Mode:    inode.Mode,
		MtimeMs: inode.MtimeMs,
		Size:    inode.Size,
	}

	switch inode.Type {
	case "dir":
		entry.Size = 0
		entries[currentPath] = entry
		if currentPath != "/" {
			stats.DirCount++
		}

		children, err := loadWorkspaceRootDirents(ctx, rdb, fsKey, inodeID)
		if err != nil {
			return err
		}
		childNames := make([]string, 0, len(children))
		for name := range children {
			childNames = append(childNames, name)
		}
		sort.Strings(childNames)
		for _, name := range childNames {
			childPath := joinWorkspaceRootPath(currentPath, name)
			if err := appendWorkspaceRootManifest(ctx, rdb, fsKey, children[name], childPath, entries, blobs, stats); err != nil {
				return err
			}
		}
		return nil
	case "file":
		data := []byte(inode.Content)
		entry.Size = int64(len(data))
		stats.FileCount++
		stats.TotalBytes += int64(len(data))
		if len(data) <= afsInlineThreshold {
			entry.Inline = base64.StdEncoding.EncodeToString(data)
		} else {
			entry.BlobID = sha256Hex(data)
			if _, ok := blobs[entry.BlobID]; !ok {
				blobs[entry.BlobID] = data
			}
		}
	case "symlink":
		entry.Target = inode.Target
		entry.Size = int64(len(inode.Target))
	default:
		return fmt.Errorf("unsupported mounted filesystem entry type %q", inode.Type)
	}

	entries[currentPath] = entry
	return nil
}

func loadWorkspaceRootInode(ctx context.Context, rdb *redis.Client, fsKey, inodeID string) (*workspaceRootInode, error) {
	values, err := rdb.HMGet(ctx, workspaceRootInodeKey(fsKey, inodeID),
		"type", "mode", "size", "mtime_ms", "target", "content", "content_ref",
	).Result()
	if err != nil {
		return nil, err
	}
	if len(values) < 6 || values[0] == nil {
		return nil, nil
	}
	inode := &workspaceRootInode{
		Type:    workspaceRootString(values[0]),
		Mode:    uint32(workspaceRootInt64(values[1])),
		Size:    workspaceRootInt64(values[2]),
		MtimeMs: workspaceRootInt64(values[3]),
		Target:  workspaceRootString(values[4]),
		Content: workspaceRootString(values[5]),
	}
	if len(values) > 6 {
		inode.ContentRef = workspaceRootString(values[6])
	}
	// If content is external, fetch from the dedicated content key.
	if inode.ContentRef != "" && inode.Type == "file" {
		data, err := rediscontent.Load(ctx, rdb, workspaceRootContentKey(fsKey, inodeID), inode.ContentRef, inode.Size)
		if err == nil {
			inode.Content = string(data)
		}
	}
	return inode, nil
}

func loadWorkspaceRootDirents(ctx context.Context, rdb *redis.Client, fsKey, inodeID string) (map[string]string, error) {
	values, err := rdb.HGetAll(ctx, workspaceRootDirentsKey(fsKey, inodeID)).Result()
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return map[string]string{}, nil
	}
	return values, nil
}

func workspaceRootInodeKey(fsKey, inodeID string) string {
	return "afs:{" + fsKey + "}:inode:" + inodeID
}

func workspaceRootContentKey(fsKey, inodeID string) string {
	return "afs:{" + fsKey + "}:content:" + inodeID
}

func workspaceRootDirentsKey(fsKey, inodeID string) string {
	return "afs:{" + fsKey + "}:dirents:" + inodeID
}

func joinWorkspaceRootPath(parentPath, name string) string {
	if parentPath == "/" {
		return "/" + name
	}
	return path.Join(parentPath, name)
}

func workspaceRootString(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(value)
	}
}

func workspaceRootInt64(value interface{}) int64 {
	switch typed := value.(type) {
	case nil:
		return 0
	case int64:
		return typed
	case string:
		n, _ := strconv.ParseInt(typed, 10, 64)
		return n
	case []byte:
		n, _ := strconv.ParseInt(string(typed), 10, 64)
		return n
	default:
		return 0
	}
}

func shouldIgnoreMountedPath(path string) bool {
	if path == "" || path == "/" {
		return false
	}
	trimmed := strings.Trim(path, "/")
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
