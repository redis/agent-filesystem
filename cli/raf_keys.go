package main

import "fmt"

func rafWorkspaceMetaKey(workspace string) string {
	return fmt.Sprintf("afs:{%s}:workspace:meta", workspace)
}

func rafWorkspaceSavepointsKey(workspace string) string {
	return fmt.Sprintf("afs:{%s}:workspace:savepoints", workspace)
}

func rafWorkspaceAuditKey(workspace string) string {
	return fmt.Sprintf("afs:{%s}:workspace:audit", workspace)
}

func rafSavepointMetaKey(workspace, savepoint string) string {
	return fmt.Sprintf("afs:{%s}:savepoint:%s:meta", workspace, savepoint)
}

func rafSavepointManifestKey(workspace, savepoint string) string {
	return fmt.Sprintf("afs:{%s}:savepoint:%s:manifest", workspace, savepoint)
}

func rafBlobKey(workspace, blobID string) string {
	return fmt.Sprintf("afs:{%s}:blob:%s", workspace, blobID)
}

func rafBlobRefKey(workspace, blobID string) string {
	return fmt.Sprintf("afs:{%s}:blobref:%s", workspace, blobID)
}

func rafWorkspacePattern(workspace string) string {
	return fmt.Sprintf("afs:{%s}:*", workspace)
}
