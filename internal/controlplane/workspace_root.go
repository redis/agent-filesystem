package controlplane

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/redis/agent-filesystem/internal/rediscontent"
	"github.com/redis/agent-filesystem/internal/searchindex"
	"github.com/redis/go-redis/v9"
)

const (
	workspaceFSRootInodeID       = "1"
	workspaceFSSchemaVersion     = "2"
	workspaceFSWriteBatchEntries = 128
	workspaceFSWriteBatchBytes   = 4 << 20
)

type workspaceFSNode struct {
	ID       string
	Path     string
	ParentID string
	Name     string
	Entry    ManifestEntry
}

func WorkspaceFSKey(workspace string) string {
	return workspace
}

func workspaceRootHeadKey(workspace string) string {
	return "afs:{" + WorkspaceFSKey(workspace) + "}:root_head_savepoint"
}

func workspaceRootDirtyKey(workspace string) string {
	return "afs:{" + WorkspaceFSKey(workspace) + "}:root_dirty"
}

func WorkspaceRootDirtyKey(workspace string) string {
	return workspaceRootDirtyKey(workspace)
}

func WorkspaceRootDirtyState(ctx context.Context, store *Store, workspace string) (dirty bool, known bool, err error) {
	storageID, err := store.resolveWorkspaceStorageOrRaw(ctx, workspace)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, false, nil
		}
		return false, false, err
	}
	value, err := store.rdb.Get(ctx, workspaceRootDirtyKey(storageID)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return false, false, nil
		}
		return false, false, err
	}
	return strings.TrimSpace(value) == "1", true, nil
}

func MarkWorkspaceRootDirty(ctx context.Context, store *Store, workspace string) error {
	storageID, err := store.resolveWorkspaceStorageOrRaw(ctx, workspace)
	if err != nil {
		return err
	}
	return store.rdb.Set(ctx, workspaceRootDirtyKey(storageID), "1", 0).Err()
}

func MarkWorkspaceRootClean(ctx context.Context, store *Store, workspace, headSavepoint string) error {
	storageID, err := store.resolveWorkspaceStorageOrRaw(ctx, workspace)
	if err != nil {
		return err
	}
	pipe := store.rdb.TxPipeline()
	pipe.Set(ctx, workspaceRootHeadKey(storageID), headSavepoint, 0)
	pipe.Set(ctx, workspaceRootDirtyKey(storageID), "0", 0)
	_, err = pipe.Exec(ctx)
	return err
}

func EnsureWorkspaceRoot(ctx context.Context, store *Store, workspace string) (string, string, bool, error) {
	meta, storageID, err := store.resolveWorkspaceMeta(ctx, workspace)
	if err != nil {
		return "", "", false, err
	}

	exists, err := workspaceRootExists(ctx, store.rdb, storageID, meta.HeadSavepoint)
	if err != nil {
		return "", "", false, err
	}
	if exists {
		return WorkspaceFSKey(storageID), meta.HeadSavepoint, false, nil
	}

	headManifest, err := store.GetManifest(ctx, storageID, meta.HeadSavepoint)
	if err != nil {
		return "", "", false, err
	}
	if err := SyncWorkspaceRoot(ctx, store, storageID, headManifest); err != nil {
		return "", "", false, err
	}
	return WorkspaceFSKey(storageID), meta.HeadSavepoint, true, nil
}

func SyncWorkspaceRoot(ctx context.Context, store *Store, workspace string, m Manifest) error {
	return SyncWorkspaceRootWithOptions(ctx, store, workspace, m, SyncOptions{})
}

// SyncOptions tunes SyncWorkspaceRootWithOptions for callers that can avoid
// round trips the default path must make.
type SyncOptions struct {
	// BlobProvider, when set, short-circuits GetBlob for materialized file
	// nodes. It returns (data, true) if the caller has the blob in memory;
	// otherwise (nil, false) and the sync falls back to Redis.
	BlobProvider func(blobID string) ([]byte, bool)
	// SkipNamespaceReset bypasses the SCAN+DEL pass over the existing FS
	// namespace. Safe only when the workspace is known to be empty
	// (fresh import, or right after DeleteWorkspace).
	SkipNamespaceReset bool
}

// SyncWorkspaceRootWithOptions materializes a manifest into the live workspace
// FS namespace. Callers that have already loaded file bodies (e.g., during
// import) can pass a BlobProvider to avoid reloading them from Redis.
func SyncWorkspaceRootWithOptions(ctx context.Context, store *Store, workspace string, m Manifest, opts SyncOptions) error {
	storageID, err := store.resolveWorkspaceStorageOrRaw(ctx, workspace)
	if err != nil {
		return err
	}
	fsKey := WorkspaceFSKey(storageID)
	if !opts.SkipNamespaceReset {
		if err := resetWorkspaceFSNamespace(ctx, store.rdb, fsKey); err != nil {
			return err
		}
	} else {
		// Even when skipping the namespace scan we still need to drop the
		// cached head/dirty markers so workspaceRootExists re-verifies.
		if err := store.rdb.Del(ctx,
			"afs:{"+fsKey+"}:info",
			"afs:{"+fsKey+"}:next_inode",
			searchindex.ReadyKey(fsKey),
			workspaceRootHeadKey(fsKey),
			workspaceRootDirtyKey(fsKey),
		).Err(); err != nil {
			return err
		}
	}
	if err := materializeManifestToWorkspaceFS(ctx, store, storageID, fsKey, m, opts); err != nil {
		return err
	}
	return MarkWorkspaceRootClean(ctx, store, storageID, m.Savepoint)
}

func workspaceRootExists(ctx context.Context, rdb *redis.Client, workspace, headSavepoint string) (bool, error) {
	fsKey := WorkspaceFSKey(workspace)
	count, err := rdb.Exists(ctx,
		"afs:{"+fsKey+"}:info",
		"afs:{"+fsKey+"}:inode:1",
	).Result()
	if err != nil {
		return false, err
	}
	if count == 0 {
		return false, nil
	}

	rootHead, err := rdb.Get(ctx, workspaceRootHeadKey(workspace)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return false, nil
		}
		return false, err
	}
	return rootHead == headSavepoint, nil
}

func resetWorkspaceFSNamespace(ctx context.Context, rdb *redis.Client, fsKey string) error {
	patterns := []string{
		"afs:{" + fsKey + "}:inode:*",
		"afs:{" + fsKey + "}:content:*",
		"afs:{" + fsKey + "}:dirents:*",
		"afs:{" + fsKey + "}:locks:*",
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
	if err := rdb.Del(ctx,
		"afs:{"+fsKey+"}:info",
		"afs:{"+fsKey+"}:next_inode",
		searchindex.ReadyKey(fsKey),
		workspaceRootHeadKey(fsKey),
		workspaceRootDirtyKey(fsKey),
	).Err(); err != nil {
		return err
	}
	return nil
}

func materializeManifestToWorkspaceFS(ctx context.Context, store *Store, workspace, fsKey string, m Manifest, opts SyncOptions) error {
	nodes, err := buildWorkspaceFSNodes(m)
	if err != nil {
		return err
	}
	return writeWorkspaceFSNodes(ctx, store, workspace, fsKey, nodes, opts)
}

func buildWorkspaceFSNodes(m Manifest) ([]workspaceFSNode, error) {
	entries := workspaceFSManifestEntries(m)
	paths := workspaceFSOrderedPaths(entries)
	nodes := make([]workspaceFSNode, 0, len(paths))
	nodeIDs := map[string]string{
		"/": workspaceFSRootInodeID,
	}

	rootEntry := entries["/"]
	if rootEntry.Type != "dir" {
		return nil, fmt.Errorf("workspace root %q must be a directory", "/")
	}
	nodes = append(nodes, workspaceFSNode{
		ID:    workspaceFSRootInodeID,
		Path:  "/",
		Entry: rootEntry,
	})

	nextID := int64(1)
	for _, manifestPath := range paths {
		if manifestPath == "/" {
			continue
		}
		entry := entries[manifestPath]
		parentPath := workspaceFSParentPath(manifestPath)
		parentEntry, ok := entries[parentPath]
		if !ok {
			return nil, fmt.Errorf("manifest path %q is missing parent %q", manifestPath, parentPath)
		}
		if parentEntry.Type != "dir" {
			return nil, fmt.Errorf("manifest path %q has non-directory parent %q", manifestPath, parentPath)
		}
		parentID, ok := nodeIDs[parentPath]
		if !ok {
			return nil, fmt.Errorf("manifest path %q is missing parent inode %q", manifestPath, parentPath)
		}

		nextID++
		nodeID := strconv.FormatInt(nextID, 10)
		nodeIDs[manifestPath] = nodeID
		nodes = append(nodes, workspaceFSNode{
			ID:       nodeID,
			Path:     manifestPath,
			ParentID: parentID,
			Name:     path.Base(manifestPath),
			Entry:    entry,
		})
	}
	return nodes, nil
}

func writeWorkspaceFSNodes(ctx context.Context, store *Store, workspace, fsKey string, nodes []workspaceFSNode, opts SyncOptions) error {
	if len(nodes) == 0 {
		return errors.New("workspace manifest is missing a root entry")
	}
	contentRef, err := rediscontent.PreferredRef(ctx, store.rdb)
	if err != nil {
		return err
	}

	var (
		fileCount    int64
		dirCount     int64
		symlinkCount int64
		totalData    int64
	)

	pipe := store.rdb.Pipeline()
	queuedEntries := 0
	queuedBytes := int64(0)
	flush := func() error {
		if queuedEntries == 0 {
			return nil
		}
		if _, err := pipe.Exec(ctx); err != nil {
			return err
		}
		pipe = store.rdb.Pipeline()
		queuedEntries = 0
		queuedBytes = 0
		return nil
	}
	queueCapacity := func(weight int64) error {
		if queuedEntries >= workspaceFSWriteBatchEntries || (queuedBytes > 0 && queuedBytes+weight > workspaceFSWriteBatchBytes) {
			if err := flush(); err != nil {
				return err
			}
		}
		queuedEntries++
		queuedBytes += weight
		return nil
	}

	for _, node := range nodes {
		fields, content, size, err := workspaceFSNodeFields(ctx, store, workspace, node, opts, contentRef)
		if err != nil {
			return fmt.Errorf("materialize %s: %w", node.Path, err)
		}
		switch node.Entry.Type {
		case "file":
			fileCount++
			totalData += size
		case "dir":
			dirCount++
		case "symlink":
			symlinkCount++
		default:
			return fmt.Errorf("unsupported manifest entry type %q", node.Entry.Type)
		}

		if err := queueCapacity(workspaceFSNodeWeight(node, size)); err != nil {
			return err
		}
		if node.Entry.Type == "file" {
			rediscontent.QueueWriteFull(ctx, pipe, workspaceFSContentKey(fsKey, node.ID), contentRef, content)
		}
		pipe.HSet(ctx, workspaceFSInodeKey(fsKey, node.ID), fields)
		if node.ParentID != "" {
			pipe.HSet(ctx, workspaceFSDirentsKey(fsKey, node.ParentID), node.Name, node.ID)
		}
	}

	if err := queueCapacity(512); err != nil {
		return err
	}
	pipe.HSet(ctx, workspaceFSInfoKey(fsKey), map[string]interface{}{
		"schema_version":   workspaceFSSchemaVersion,
		"files":            fileCount,
		"directories":      dirCount,
		"symlinks":         symlinkCount,
		"total_data_bytes": totalData,
	})
	pipe.Set(ctx, workspaceFSNextInodeKey(fsKey), nodes[len(nodes)-1].ID, 0)
	return flush()
}

func workspaceFSNodeFields(ctx context.Context, store *Store, workspace string, node workspaceFSNode, opts SyncOptions, contentRef string) (map[string]interface{}, []byte, int64, error) {
	fields := map[string]interface{}{
		"type":           node.Entry.Type,
		"mode":           manifestEntryModeForWorkspaceFS(node.Entry),
		"uid":            0,
		"gid":            0,
		"ctime_ms":       node.Entry.MtimeMs,
		"mtime_ms":       node.Entry.MtimeMs,
		"atime_ms":       node.Entry.MtimeMs,
		"parent":         node.ParentID,
		"name":           node.Name,
		"path":           node.Path,
		"path_ancestors": workspaceFSIndexedPathAncestors(node.Path),
	}

	switch node.Entry.Type {
	case "dir":
		fields["size"] = 0
		return fields, nil, 0, nil
	case "file":
		data, err := ManifestEntryData(node.Entry, func(blobID string) ([]byte, error) {
			if opts.BlobProvider != nil {
				if cached, ok := opts.BlobProvider(blobID); ok {
					return cached, nil
				}
			}
			return store.GetBlob(ctx, workspace, blobID)
		})
		if err != nil {
			return nil, nil, 0, err
		}
		fields["size"] = int64(len(data))
		fields["content_ref"] = contentRef
		indexFields := searchindex.BuildFileFields(data)
		fields["search_state"] = indexFields.SearchState
		fields["grep_grams_ci"] = indexFields.GrepGramsCI
		return fields, data, int64(len(data)), nil
	case "symlink":
		size := node.Entry.Size
		if size == 0 && node.Entry.Target != "" {
			size = int64(len(node.Entry.Target))
		}
		fields["size"] = size
		fields["target"] = node.Entry.Target
		return fields, nil, 0, nil
	default:
		return nil, nil, 0, fmt.Errorf("unsupported manifest entry type %q", node.Entry.Type)
	}
}

func workspaceFSManifestEntries(m Manifest) map[string]ManifestEntry {
	entries := make(map[string]ManifestEntry, len(m.Entries)+1)
	for manifestPath, entry := range m.Entries {
		entries[workspaceFSNormalizePath(manifestPath)] = entry
	}

	rootEntry := ManifestEntry{Type: "dir", Mode: 0o755}
	if entry, ok := entries["/"]; ok {
		rootEntry = entry
	}
	rootEntry.Type = "dir"
	rootEntry.Mode = manifestEntryModeForWorkspaceFS(rootEntry)
	entries["/"] = rootEntry

	paths := make([]string, 0, len(entries))
	for manifestPath := range entries {
		paths = append(paths, manifestPath)
	}
	for _, manifestPath := range paths {
		for parentPath := workspaceFSParentPath(manifestPath); parentPath != "/"; parentPath = workspaceFSParentPath(parentPath) {
			if _, ok := entries[parentPath]; ok {
				continue
			}
			entries[parentPath] = ManifestEntry{Type: "dir", Mode: 0o755}
		}
	}
	return entries
}

func workspaceFSOrderedPaths(entries map[string]ManifestEntry) []string {
	paths := make([]string, 0, len(entries))
	for manifestPath := range entries {
		paths = append(paths, manifestPath)
	}
	sort.Slice(paths, func(i, j int) bool {
		leftDepth := workspaceFSPathDepth(paths[i])
		rightDepth := workspaceFSPathDepth(paths[j])
		if leftDepth == rightDepth {
			return paths[i] < paths[j]
		}
		return leftDepth < rightDepth
	})
	return paths
}

func manifestEntryModeForWorkspaceFS(entry ManifestEntry) uint32 {
	switch entry.Type {
	case "dir":
		if entry.Mode == 0 {
			return 0o755
		}
	case "symlink":
		if entry.Mode == 0 {
			return 0o777
		}
	default:
		if entry.Mode == 0 {
			return 0o644
		}
	}
	return entry.Mode
}

func workspaceFSInodeKey(fsKey, inodeID string) string {
	return "afs:{" + fsKey + "}:inode:" + inodeID
}

func workspaceFSDirentsKey(fsKey, inodeID string) string {
	return "afs:{" + fsKey + "}:dirents:" + inodeID
}

func workspaceFSInfoKey(fsKey string) string {
	return "afs:{" + fsKey + "}:info"
}

func workspaceFSNextInodeKey(fsKey string) string {
	return "afs:{" + fsKey + "}:next_inode"
}

func workspaceFSNodeWeight(node workspaceFSNode, contentSize int64) int64 {
	weight := int64(len(node.Path) + len(node.Name) + 256)
	switch node.Entry.Type {
	case "file":
		return weight + contentSize
	case "symlink":
		return weight + int64(len(node.Entry.Target))
	default:
		return weight
	}
}

func workspaceFSNormalizePath(manifestPath string) string {
	if manifestPath == "" {
		return "/"
	}
	if !strings.HasPrefix(manifestPath, "/") {
		manifestPath = "/" + manifestPath
	}
	clean := path.Clean(manifestPath)
	if clean == "." {
		return "/"
	}
	return clean
}

func workspaceFSParentPath(manifestPath string) string {
	manifestPath = workspaceFSNormalizePath(manifestPath)
	if manifestPath == "/" {
		return "/"
	}
	parentPath := path.Dir(manifestPath)
	if parentPath == "." {
		return "/"
	}
	return parentPath
}

func workspaceFSPathDepth(manifestPath string) int {
	manifestPath = workspaceFSNormalizePath(manifestPath)
	if manifestPath == "/" {
		return 0
	}
	return strings.Count(strings.TrimPrefix(manifestPath, "/"), "/") + 1
}

func workspaceFSIndexedPathAncestors(manifestPath string) string {
	trimmed := strings.TrimSpace(workspaceFSNormalizePath(manifestPath))
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
