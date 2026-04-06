package main

import (
	"github.com/rowantrollope/agent-filesystem/cli/internal/controlplane"
	"github.com/rowantrollope/agent-filesystem/cli/internal/worktree"
)

func controlPlaneConfigFromCLI(cfg config) controlplane.Config {
	return controlplane.Config{
		RedisAddr:     cfg.RedisAddr,
		RedisUsername: cfg.RedisUsername,
		RedisPassword: cfg.RedisPassword,
		RedisDB:       cfg.RedisDB,
		RedisTLS:      cfg.RedisTLS,
	}
}

func controlPlaneServiceFromStore(cfg config, store *afsStore) *controlplane.Service {
	return controlplane.NewService(controlPlaneConfigFromCLI(cfg), controlPlaneStoreFromAFS(store))
}

func controlPlaneStoreFromAFS(store *afsStore) *controlplane.Store {
	if store == nil {
		return nil
	}
	if store.cp != nil {
		return store.cp
	}
	return controlplane.NewStore(store.rdb)
}

func controlPlaneManifestFromAFS(m manifest) controlplane.Manifest {
	entries := make(map[string]controlplane.ManifestEntry, len(m.Entries))
	for p, entry := range m.Entries {
		entries[p] = controlplane.ManifestEntry{
			Type:    entry.Type,
			Mode:    entry.Mode,
			MtimeMs: entry.MtimeMs,
			Size:    entry.Size,
			BlobID:  entry.BlobID,
			Inline:  entry.Inline,
			Target:  entry.Target,
		}
	}
	return controlplane.Manifest{
		Version:   m.Version,
		Workspace: m.Workspace,
		Savepoint: m.Savepoint,
		Entries:   entries,
	}
}

func worktreeConfigFromCLI(cfg config) worktree.Config {
	return worktree.Config{
		WorkRoot: cfg.WorkRoot,
	}
}
