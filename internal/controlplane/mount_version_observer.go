package controlplane

import (
	"context"

	afsclient "github.com/redis/agent-filesystem/mount/client"
	"github.com/redis/go-redis/v9"
)

type mountVersionObserver struct {
	store *Store
}

func NewMountVersionObserver(rdb *redis.Client) afsclient.MutationObserver {
	return &mountVersionObserver{store: NewStore(rdb)}
}

func (o *mountVersionObserver) RecordMutation(ctx context.Context, workspace string, before, after afsclient.VersionedSnapshot) error {
	_, err := o.store.RecordFileVersionMutation(ctx, workspace, VersionedFileSnapshot{
		Path:      before.Path,
		Exists:    before.Exists,
		Kind:      before.Kind,
		Mode:      before.Mode,
		Content:   before.Content,
		Target:    before.Target,
		SizeBytes: before.SizeBytes,
	}, VersionedFileSnapshot{
		Path:      after.Path,
		Exists:    after.Exists,
		Kind:      after.Kind,
		Mode:      after.Mode,
		Content:   after.Content,
		Target:    after.Target,
		SizeBytes: after.SizeBytes,
	}, FileVersionMutationMetadata{
		Source: ChangeSourceMount,
	})
	return err
}
