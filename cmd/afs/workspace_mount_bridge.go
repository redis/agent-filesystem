package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/redis/agent-filesystem/mount/client"
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

func ensureMountWorkspace(ctx context.Context, cfg config, store *afsStore) (string, bool, error) {
	workspace := strings.TrimSpace(cfg.CurrentWorkspace)
	if workspace == "" {
		return "", false, errors.New("no current workspace is selected")
	}
	if err := validateAFSName("workspace", workspace); err != nil {
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

func seedWorkspaceMountKey(ctx context.Context, store *afsStore, workspace string) (string, string, bool, error) {
	mountKey, headSavepoint, initialized, err := store.ensureWorkspaceRoot(ctx, workspace)
	if err != nil {
		return "", "", false, err
	}
	ensureWorkspaceSearchIndex(ctx, store.rdb, mountKey)
	return mountKey, headSavepoint, initialized, nil
}

func saveWorkspaceRootCheckpoint(ctx context.Context, store *afsStore, workspace, expectedHead, savepointID string) (bool, error) {
	rootManifest, blobs, stats, err := buildManifestFromAFS(ctx, client.New(store.rdb, workspaceRedisKey(workspace)), workspace, savepointID)
	if err != nil {
		return false, err
	}

	saved, err := saveAFSManifest(ctx, store, workspace, expectedHead, savepointID, rootManifest, blobs, stats, false)
	if err != nil {
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
			if len(data) <= afsInlineThreshold {
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
		Version:   afsFormatVersion,
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
