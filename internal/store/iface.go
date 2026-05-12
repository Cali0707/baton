package store

import "context"

// Store defines the persistence interface for inbox items, runs, and output.
type Store interface {
	// Inbox items
	UpsertItem(ctx context.Context, item *InboxItem) error
	GetItem(ctx context.Context, id int64) (*InboxItem, error)
	GetItemBySourceID(ctx context.Context, sourceID string) (*InboxItem, error)
	ListItems(ctx context.Context, statuses []ItemStatus) ([]*InboxItem, error)
	UpdateItemStatus(ctx context.Context, id int64, status ItemStatus) error
	DeleteItem(ctx context.Context, id int64) error

	// Runs
	CreateRun(ctx context.Context, run *Run) error
	GetRun(ctx context.Context, id int64) (*Run, error)
	UpdateRun(ctx context.Context, run *Run) error
	ListRuns(ctx context.Context, statuses []SessionStatus) ([]*Run, error)
	ListRunsForItem(ctx context.Context, itemID int64) ([]*Run, error)
	DeleteRunsForItem(ctx context.Context, itemID int64) error

	// Output entries (file-based transcript storage)
	AppendEntry(runID int64, entry OutputEntry) error
	LoadEntries(runID int64) ([]OutputEntry, error)
}
