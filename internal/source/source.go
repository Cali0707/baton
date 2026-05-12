package source

import (
	"context"
	"time"

	"github.com/Cali0707/baton/internal/store"
)

// Comment represents a comment on an issue or PR.
type Comment struct {
	Author    string
	Body      string
	CreatedAt time.Time
}

// ItemDetail holds fetched detail for an inbox item (comments, diff).
type ItemDetail struct {
	Comments []Comment
	Diff     string // non-empty for PRs
}

// Source fetches detail for inbox items from an external source.
type Source interface {
	FetchDetail(ctx context.Context, item *store.InboxItem) (*ItemDetail, error)
}
