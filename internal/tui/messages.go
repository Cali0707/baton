package tui

import (
	acp "github.com/coder/acp-go-sdk"
	"github.com/Cali0707/baton/internal/source"
	"github.com/Cali0707/baton/internal/store"
)

// Messages for Bubble Tea

// WorkItemsLoaded is sent when inbox items are loaded from the database.
type WorkItemsLoaded struct {
	Items []*store.InboxItem
	Err   error
}

// SyncComplete is sent when a background sync from sources finishes.
type SyncComplete struct {
	NewCount     int
	UpdatedCount int
	Err          error
}

// DetailLoaded is sent when detail (comments, diff) for an item finishes loading.
type DetailLoaded struct {
	Detail *source.ItemDetail
	Err    error
}

// AgentUpdateMsg wraps a session update from the ACP agent.
type AgentUpdateMsg struct {
	RunID  int64
	Update acp.SessionUpdate
}

// AgentDoneMsg signals the agent workflow is complete.
type AgentDoneMsg struct {
	RunID int64
	Run   *store.Run
	Err   error
}

// AgentStartedMsg signals the agent started successfully.
type AgentStartedMsg struct {
	RunID int64
}

// SessionOutputLoaded carries the parsed output entries for a completed run.
type SessionOutputLoaded struct {
	RunID   int64
	Entries []store.OutputEntry
	Err     error
}

// RunsLoaded carries the runs for an inbox item.
type RunsLoaded struct {
	Runs []*store.Run
	Err  error
}

// ErrorMsg represents a user-facing error.
type ErrorMsg struct {
	Err error
}
