package controlplane

import (
	"context"
	"fmt"
	"sort"

	"github.com/redis/agent-filesystem/mount/client"
	"github.com/redis/go-redis/v9"
)

func WorkspaceFSKey(workspace string) string {
	return workspace
}

func EnsureWorkspaceRoot(ctx context.Context, store *Store, workspace string) (string, string, bool, error) {
	meta, err := store.GetWorkspaceMeta(ctx, workspace)
	if err != nil {
		return "", "", false, err
	}

	exists, err := workspaceRootExists(ctx, store.rdb, workspace)
	if err != nil {
		return "", "", false, err
	}
	if exists {
		return WorkspaceFSKey(workspace), meta.HeadSavepoint, false, nil
	}

	headManifest, err := store.GetManifest(ctx, workspace, meta.HeadSavepoint)
	if err != nil {
		return "", "", false, err
	}
	if err := SyncWorkspaceRoot(ctx, store, workspace, headManifest); err != nil {
		return "", "", false, err
	}
	return WorkspaceFSKey(workspace), meta.HeadSavepoint, true, nil
}

func SyncWorkspaceRoot(ctx context.Context, store *Store, workspace string, m Manifest) error {
	fsKey := WorkspaceFSKey(workspace)
	if err := resetWorkspaceFSNamespace(ctx, store.rdb, fsKey); err != nil {
		return err
	}
	fsClient := client.New(store.rdb, fsKey)
	return materializeManifestToWorkspaceFS(ctx, store, workspace, fsClient, m)
}

func workspaceRootExists(ctx context.Context, rdb *redis.Client, workspace string) (bool, error) {
	fsKey := WorkspaceFSKey(workspace)
	count, err := rdb.Exists(ctx,
		"afs:{"+fsKey+"}:info",
		"afs:{"+fsKey+"}:inode:1",
	).Result()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func resetWorkspaceFSNamespace(ctx context.Context, rdb *redis.Client, fsKey string) error {
	patterns := []string{
		"afs:{" + fsKey + "}:inode:*",
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
	).Err(); err != nil {
		return err
	}
	return nil
}

func materializeManifestToWorkspaceFS(ctx context.Context, store *Store, workspace string, fsClient client.Client, m Manifest) error {
	rootEntry := ManifestEntry{Type: "dir", Mode: 0o755}
	if entry, ok := m.Entries["/"]; ok {
		rootEntry = entry
	}

	if err := fsClient.Mkdir(ctx, "/"); err != nil {
		return err
	}

	dirs, others := manifestPathsForWorkspaceFS(m)
	for _, manifestPath := range dirs {
		if err := fsClient.Mkdir(ctx, manifestPath); err != nil {
			return fmt.Errorf("mkdir %s: %w", manifestPath, err)
		}
	}

	for _, manifestPath := range others {
		entry := m.Entries[manifestPath]
		switch entry.Type {
		case "file":
			data, err := ManifestEntryData(entry, func(blobID string) ([]byte, error) {
				return store.GetBlob(ctx, workspace, blobID)
			})
			if err != nil {
				return err
			}
			if err := fsClient.EchoCreate(ctx, manifestPath, data, manifestEntryModeForWorkspaceFS(entry)); err != nil {
				return fmt.Errorf("write %s: %w", manifestPath, err)
			}
		case "symlink":
			if err := fsClient.Ln(ctx, entry.Target, manifestPath); err != nil {
				return fmt.Errorf("symlink %s: %w", manifestPath, err)
			}
		default:
			return fmt.Errorf("unsupported manifest entry type %q", entry.Type)
		}
		if err := applyManifestEntryMetadataToWorkspaceFS(ctx, fsClient, manifestPath, entry); err != nil {
			return err
		}
	}

	for i := len(dirs) - 1; i >= 0; i-- {
		manifestPath := dirs[i]
		if err := applyManifestEntryMetadataToWorkspaceFS(ctx, fsClient, manifestPath, m.Entries[manifestPath]); err != nil {
			return err
		}
	}

	return applyManifestEntryMetadataToWorkspaceFS(ctx, fsClient, "/", rootEntry)
}

func applyManifestEntryMetadataToWorkspaceFS(ctx context.Context, fsClient client.Client, path string, entry ManifestEntry) error {
	if err := fsClient.Chmod(ctx, path, manifestEntryModeForWorkspaceFS(entry)); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}
	if err := fsClient.Utimens(ctx, path, entry.MtimeMs, entry.MtimeMs); err != nil {
		return fmt.Errorf("utimens %s: %w", path, err)
	}
	return nil
}

func manifestPathsForWorkspaceFS(m Manifest) ([]string, []string) {
	paths := make([]string, 0, len(m.Entries))
	for manifestPath := range m.Entries {
		paths = append(paths, manifestPath)
	}
	sort.Strings(paths)

	dirs := make([]string, 0, len(paths))
	others := make([]string, 0, len(paths))
	for _, manifestPath := range paths {
		if manifestPath == "/" {
			continue
		}
		if m.Entries[manifestPath].Type == "dir" {
			dirs = append(dirs, manifestPath)
		} else {
			others = append(others, manifestPath)
		}
	}

	sort.Slice(dirs, func(i, j int) bool {
		if len(dirs[i]) == len(dirs[j]) {
			return dirs[i] < dirs[j]
		}
		return len(dirs[i]) < len(dirs[j])
	})
	return dirs, others
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
