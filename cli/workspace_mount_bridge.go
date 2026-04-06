package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/redis/go-redis/v9"
	"github.com/rowantrollope/agent-filesystem/mount/client"
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
	return "afs.mount." + workspace
}

func ensureMountWorkspace(ctx context.Context, cfg config, store *rafStore) (string, bool, error) {
	workspace := strings.TrimSpace(cfg.CurrentWorkspace)
	if workspace == "" {
		return "", false, errors.New("no current workspace is selected")
	}
	if err := validateRAFName("workspace", workspace); err != nil {
		return "", false, err
	}

	exists, err := store.workspaceExists(ctx, workspace)
	if err != nil {
		return "", false, err
	}
	if exists {
		return workspace, false, nil
	}
	if err := createEmptyWorkspace(ctx, cfg, store, workspace); err != nil {
		return "", false, err
	}
	return workspace, true, nil
}

func seedWorkspaceMountKey(ctx context.Context, cfg config, store *rafStore, rdb *redis.Client, workspace string) (string, string, error) {
	workspaceMeta, _, err := ensureMaterializedWorkspace(ctx, store, cfg, workspace)
	if err != nil {
		return "", "", err
	}

	treePath := rafWorkspaceTreePath(cfg, workspace)
	mountKey := mountRedisKeyForWorkspace(workspace)
	if err := syncDirectoryToAFSKey(ctx, rdb, mountKey, treePath); err != nil {
		return "", "", err
	}
	return mountKey, workspaceMeta.HeadSavepoint, nil
}

func syncDirectoryToAFSKey(ctx context.Context, rdb *redis.Client, fsKey, sourceDir string) error {
	if err := deleteNamespace(ctx, rdb, fsKey); err != nil {
		return err
	}
	fsClient := client.New(rdb, fsKey)
	if _, _, _, _, _, err := importDirectory(ctx, fsClient, sourceDir, nil, nil); err != nil {
		return err
	}

	rootInfo, err := os.Stat(sourceDir)
	if err != nil {
		return err
	}
	return applyMetadata(ctx, client.New(rdb, fsKey), "/", rootInfo)
}

func syncMountedWorkspaceBack(ctx context.Context, cfg config, store *rafStore, rdb *redis.Client, workspace, expectedHead string) (bool, error) {
	savepointID := generatedSavepointName()
	mountKey := mountRedisKeyForWorkspace(workspace)

	mountedManifest, blobs, stats, err := buildManifestFromAFS(ctx, client.New(rdb, mountKey), workspace, savepointID)
	if err != nil {
		return false, err
	}

	saved, err := saveRAFManifest(ctx, store, workspace, expectedHead, savepointID, mountedManifest, blobs, stats)
	if err != nil {
		if err == errRAFWorkspaceConflict {
			return false, fmt.Errorf("workspace %q moved while it was mounted; inspect it before stopping AFS", workspace)
		}
		return false, err
	}
	if err := materializeWorkspace(ctx, store, cfg, workspace); err != nil {
		return false, err
	}
	return saved, nil
}

func buildManifestFromAFS(ctx context.Context, fsClient client.Client, workspace, savepoint string) (manifest, map[string][]byte, manifestStats, error) {
	tree, err := fsClient.Tree(ctx, "/", workspaceMountTreeMaxDepth)
	if err != nil {
		return manifest{}, nil, manifestStats{}, err
	}
	sort.Slice(tree, func(i, j int) bool {
		return tree[i].Path < tree[j].Path
	})

	entries := make(map[string]manifestEntry, len(tree))
	blobs := make(map[string][]byte)
	stats := manifestStats{}

	for _, node := range tree {
		if shouldIgnoreMountedPath(node.Path) {
			continue
		}
		stat, err := fsClient.Stat(ctx, node.Path)
		if err != nil {
			return manifest{}, nil, manifestStats{}, err
		}
		if stat == nil {
			return manifest{}, nil, manifestStats{}, fmt.Errorf("mounted filesystem entry %q disappeared while scanning", node.Path)
		}

		entry := manifestEntry{
			Type:    stat.Type,
			Mode:    stat.Mode,
			MtimeMs: stat.Mtime,
			Size:    stat.Size,
		}

		switch stat.Type {
		case "dir":
			entry.Size = 0
			if node.Path != "/" {
				stats.DirCount++
			}
		case "file":
			data, err := fsClient.Cat(ctx, node.Path)
			if err != nil {
				return manifest{}, nil, manifestStats{}, err
			}
			entry.Size = int64(len(data))
			stats.FileCount++
			stats.TotalBytes += int64(len(data))
			if len(data) <= rafInlineThreshold {
				entry.Inline = base64.StdEncoding.EncodeToString(data)
			} else {
				entry.BlobID = sha256Hex(data)
				if _, ok := blobs[entry.BlobID]; !ok {
					blobs[entry.BlobID] = data
				}
			}
		case "symlink":
			target, err := fsClient.Readlink(ctx, node.Path)
			if err != nil {
				return manifest{}, nil, manifestStats{}, err
			}
			entry.Target = target
			entry.Size = int64(len(target))
		default:
			return manifest{}, nil, manifestStats{}, fmt.Errorf("unsupported mounted filesystem entry type %q", stat.Type)
		}

		entries[node.Path] = entry
	}

	if _, ok := entries["/"]; !ok {
		entries["/"] = manifestEntry{Type: "dir", Mode: 0o755}
	}

	return manifest{
		Version:   rafFormatVersion,
		Workspace: workspace,
		Savepoint: savepoint,
		Entries:   entries,
	}, blobs, stats, nil
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
