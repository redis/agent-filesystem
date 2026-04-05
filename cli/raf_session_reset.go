package main

import (
	"context"
	"errors"
	"os"
	"time"
)

type rafSessionResetResult struct {
	treePath    string
	archivePath string
}

func resetRAFSessionHead(ctx context.Context, cfg config, store *rafStore, workspace, session, savepoint string) (rafSessionResetResult, error) {
	result := rafSessionResetResult{
		treePath: rafSessionTreePath(cfg, workspace, session),
	}

	if _, err := os.Stat(result.treePath); err == nil {
		archivePath, err := archiveLocalTree(cfg, workspace, session)
		if err != nil {
			return result, err
		}
		result.archivePath = archivePath
	} else if !errors.Is(err, os.ErrNotExist) {
		return result, err
	}

	if err := store.moveSessionHead(ctx, workspace, session, savepoint, time.Now().UTC()); err != nil {
		if result.archivePath != "" {
			_ = os.Rename(result.archivePath, result.treePath)
		}
		return result, err
	}

	if err := materializeSession(ctx, store, cfg, workspace, session); err != nil {
		return result, err
	}

	return result, nil
}
