package main

import (
	"context"
	"time"

	"github.com/redis/agent-filesystem/internal/controlplane"
	"github.com/redis/go-redis/v9"
)

type afsStore struct {
	rdb *redis.Client
	cp  *controlplane.Store
}

type auditRecord = controlplane.AuditRecord

func newAFSStore(rdb *redis.Client) *afsStore {
	return &afsStore{rdb: rdb, cp: controlplane.NewStore(rdb)}
}

func (s *afsStore) workspaceExists(ctx context.Context, workspace string) (bool, error) {
	return s.cp.WorkspaceExists(ctx, workspace)
}

func (s *afsStore) deleteWorkspace(ctx context.Context, workspace string) error {
	return s.cp.DeleteWorkspace(ctx, workspace)
}

func (s *afsStore) putWorkspaceMeta(ctx context.Context, meta workspaceMeta) error {
	return s.cp.PutWorkspaceMeta(ctx, meta)
}

func (s *afsStore) getWorkspaceMeta(ctx context.Context, workspace string) (workspaceMeta, error) {
	return s.cp.GetWorkspaceMeta(ctx, workspace)
}

func (s *afsStore) listWorkspaces(ctx context.Context) ([]workspaceMeta, error) {
	return s.cp.ListWorkspaces(ctx)
}

func (s *afsStore) putSavepoint(ctx context.Context, meta savepointMeta, m manifest) error {
	return s.cp.PutSavepoint(ctx, meta, m)
}

func (s *afsStore) savepointExists(ctx context.Context, workspace, savepoint string) (bool, error) {
	return s.cp.SavepointExists(ctx, workspace, savepoint)
}

func (s *afsStore) getSavepointMeta(ctx context.Context, workspace, savepoint string) (savepointMeta, error) {
	return s.cp.GetSavepointMeta(ctx, workspace, savepoint)
}

func (s *afsStore) getManifest(ctx context.Context, workspace, savepoint string) (manifest, error) {
	return s.cp.GetManifest(ctx, workspace, savepoint)
}

func (s *afsStore) listSavepoints(ctx context.Context, workspace string, limit int64) ([]savepointMeta, error) {
	return s.cp.ListSavepoints(ctx, workspace, limit)
}

func (s *afsStore) saveBlobs(ctx context.Context, workspace string, blobs map[string][]byte) error {
	return s.cp.SaveBlobs(ctx, workspace, blobs)
}

func (s *afsStore) addBlobRefs(ctx context.Context, workspace string, m manifest, createdAt time.Time) error {
	return s.cp.AddBlobRefs(ctx, workspace, m, createdAt)
}

func (s *afsStore) getBlob(ctx context.Context, workspace, blobID string) ([]byte, error) {
	return s.cp.GetBlob(ctx, workspace, blobID)
}

func (s *afsStore) blobStats(ctx context.Context, workspace string) (workspaceBlobStats, error) {
	return s.cp.BlobStats(ctx, workspace)
}

func (s *afsStore) moveWorkspaceHead(ctx context.Context, workspace, savepoint string, updatedAt time.Time) error {
	return s.cp.MoveWorkspaceHead(ctx, workspace, savepoint, updatedAt)
}

func (s *afsStore) audit(ctx context.Context, workspace, op string, extra map[string]any) error {
	return s.cp.Audit(ctx, workspace, op, extra)
}

func (s *afsStore) listAudit(ctx context.Context, workspace string, limit int64) ([]auditRecord, error) {
	return s.cp.ListAudit(ctx, workspace, limit)
}

func (s *afsStore) ensureWorkspaceRoot(ctx context.Context, workspace string) (string, string, bool, error) {
	return controlplane.EnsureWorkspaceRoot(ctx, s.cp, workspace)
}

func (s *afsStore) syncWorkspaceRoot(ctx context.Context, workspace string, m manifest) error {
	return s.syncWorkspaceRootWithOptions(ctx, workspace, m, controlplane.SyncOptions{})
}

func (s *afsStore) syncWorkspaceRootWithOptions(ctx context.Context, workspace string, m manifest, opts controlplane.SyncOptions) error {
	if err := controlplane.SyncWorkspaceRootWithOptions(ctx, s.cp, workspace, m, opts); err != nil {
		return err
	}
	searchRDB := newSearchRedisClient(s.rdb)
	defer searchRDB.Close()
	_, err := ensureWorkspaceSearchIndex(ctx, searchRDB, workspaceRedisKey(workspace))
	return err
}

func (s *afsStore) acquireImportLock(ctx context.Context, workspace string) (*controlplane.ImportLock, error) {
	return controlplane.AcquireImportLock(ctx, s.cp, workspace)
}

func (s *afsStore) checkImportLock(ctx context.Context, workspace string) error {
	return controlplane.CheckImportLock(ctx, s.cp, workspace)
}

func (s *afsStore) newBlobWriter(workspace string, createdAt time.Time) *controlplane.BlobWriter {
	return controlplane.NewBlobWriter(s.rdb, workspace, createdAt)
}

func (s *afsStore) workspaceRootDirtyState(ctx context.Context, workspace string) (bool, bool, error) {
	return controlplane.WorkspaceRootDirtyState(ctx, s.cp, workspace)
}

func (s *afsStore) markWorkspaceRootDirty(ctx context.Context, workspace string) error {
	return controlplane.MarkWorkspaceRootDirty(ctx, s.cp, workspace)
}

func (s *afsStore) markWorkspaceRootClean(ctx context.Context, workspace, headSavepoint string) error {
	return controlplane.MarkWorkspaceRootClean(ctx, s.cp, workspace, headSavepoint)
}

func workspaceRedisKey(workspace string) string {
	return controlplane.WorkspaceFSKey(workspace)
}
