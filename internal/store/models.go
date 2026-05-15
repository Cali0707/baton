package store

import "time"

type ItemStatus string

const (
	ItemStatusNew        ItemStatus = "new"
	ItemStatusInProgress ItemStatus = "in_progress"
	ItemStatusDone       ItemStatus = "done"
	ItemStatusArchived   ItemStatus = "archived"
)

// InboxItem represents a work item in the unified inbox.
type InboxItem struct {
	ID              int64      `db:"id"`
	SourceType      string     `db:"source_type"`
	SourceID        string     `db:"source_id"`
	Kind            string     `db:"kind"`
	Number          *int       `db:"number"`
	Title           string     `db:"title"`
	Body            string     `db:"body"`
	Author          string     `db:"author"`
	Labels          string     `db:"labels"`
	Owner           string     `db:"owner"`
	Repo            string     `db:"repo"`
	Metadata        string     `db:"metadata"`
	SourceState     string     `db:"source_state"`
	Status          ItemStatus `db:"status"`
	WorktreePath    string     `db:"worktree_path"`
	SourceUpdatedAt *time.Time `db:"source_updated_at"`
	CreatedAt       time.Time  `db:"created_at"`
	UpdatedAt       time.Time  `db:"updated_at"`
}

// Run represents a workflow execution attached to an inbox item.
type Run struct {
	ID             int64         `db:"id"`
	InboxItemID    int64         `db:"inbox_item_id"`
	WorkflowType   string        `db:"workflow_type"`
	AgentName      string        `db:"agent_name"`
	AgentSessionID string        `db:"agent_session_id"`
	WorktreePath   string        `db:"worktree_path"`
	ResumeCmd      string        `db:"resume_cmd"`
	Status         SessionStatus `db:"status"`
	OutputFile     string        `db:"output_file"`
	StartedAt      time.Time     `db:"started_at"`
	CompletedAt    *time.Time    `db:"completed_at"`
}
