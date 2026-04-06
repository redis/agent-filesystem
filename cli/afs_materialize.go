package main

import "github.com/rowantrollope/agent-filesystem/cli/internal/worktree"

type manifestBlobLoader = worktree.BlobLoader

type manifestMaterializeOptions struct {
	onProgress       func(importStats)
	preserveMetadata bool
}

func materializeManifestToDirectory(targetDir string, m manifest, loadBlob manifestBlobLoader, opts manifestMaterializeOptions) (importStats, error) {
	worktreeOpts := worktree.MaterializeOptions{
		PreserveMetadata: opts.preserveMetadata,
	}
	if opts.onProgress != nil {
		worktreeOpts.OnProgress = func(progress worktree.ImportStats) {
			opts.onProgress(importStats(progress))
		}
	}
	progress, err := worktree.MaterializeManifestToDirectory(targetDir, m, loadBlob, worktreeOpts)
	return importStats(progress), err
}
