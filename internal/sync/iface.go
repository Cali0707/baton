package sync

import "context"

// Syncer defines the interface for syncing external sources into the local store.
type SyncService interface {
	Sync(ctx context.Context) (newCount, updatedCount int, err error)
}
