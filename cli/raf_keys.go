package main

import "fmt"

func rafWorkspaceMetaKey(workspace string) string {
	return fmt.Sprintf("raf:{%s}:workspace:meta", workspace)
}

func rafWorkspaceSessionsKey(workspace string) string {
	return fmt.Sprintf("raf:{%s}:workspace:sessions", workspace)
}

func rafWorkspaceSavepointsKey(workspace string) string {
	return fmt.Sprintf("raf:{%s}:workspace:savepoints", workspace)
}

func rafWorkspaceAuditKey(workspace string) string {
	return fmt.Sprintf("raf:{%s}:workspace:audit", workspace)
}

func rafSessionMetaKey(workspace, session string) string {
	return fmt.Sprintf("raf:{%s}:session:%s:meta", workspace, session)
}

func rafSessionAuditKey(workspace, session string) string {
	return fmt.Sprintf("raf:{%s}:session:%s:audit", workspace, session)
}

func rafSavepointMetaKey(workspace, savepoint string) string {
	return fmt.Sprintf("raf:{%s}:savepoint:%s:meta", workspace, savepoint)
}

func rafSavepointManifestKey(workspace, savepoint string) string {
	return fmt.Sprintf("raf:{%s}:savepoint:%s:manifest", workspace, savepoint)
}

func rafBlobKey(workspace, blobID string) string {
	return fmt.Sprintf("raf:{%s}:blob:%s", workspace, blobID)
}

func rafBlobRefKey(workspace, blobID string) string {
	return fmt.Sprintf("raf:{%s}:blobref:%s", workspace, blobID)
}

func rafWorkspacePattern(workspace string) string {
	return fmt.Sprintf("raf:{%s}:*", workspace)
}
