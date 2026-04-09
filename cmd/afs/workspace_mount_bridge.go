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
		return "", errors.New("no current workspace is selected")
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
	ensureWorkspaceSearchIndex(ctx, store.rdb, mountKey)
	return mountKey, headSavepoint, initialized, nil
}

func saveWorkspaceRootCheckpoint(ctx context.Context, store *afsStore, workspace, expectedHead, savepointID string) (bool, error) {
	rootManifest, blobs, stats, err := buildManifestFromWorkspaceRoot(ctx, store.rdb, workspaceRedisKey(workspace), workspace, savepointID)
	if err != nil {
		return false, err
	}

	saved, err := saveAFSManifest(ctx, store, workspace, expectedHead, savepointID, rootManifest, blobs, stats, false)
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
	entries := make(map[string]manifestEntry)
	blobs := make(map[string][]byte)
	stats := manifestStats{}
	if err := appendWorkspaceRootManifest(ctx, rdb, fsKey, workspaceFSRootInodeID, "/", entries, blobs, &stats); err != nil {
		return manifest{}, nil, manifestStats{}, err
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
	Type    string
	Mode    uint32
	Size    int64
	MtimeMs int64
	Target  string
	Content string
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
		"type", "mode", "size", "mtime_ms", "target", "content",
	).Result()
	if err != nil {
		return nil, err
	}
	if len(values) < 6 || values[0] == nil {
		return nil, nil
	}
	return &workspaceRootInode{
		Type:    workspaceRootString(values[0]),
		Mode:    uint32(workspaceRootInt64(values[1])),
		Size:    workspaceRootInt64(values[2]),
		MtimeMs: workspaceRootInt64(values[3]),
		Target:  workspaceRootString(values[4]),
		Content: workspaceRootString(values[5]),
	}, nil
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
