package store

import "encoding/json"

type SessionStatus string

const (
	StatusRunning   SessionStatus = "running"
	StatusCompleted SessionStatus = "completed"
	StatusFailed    SessionStatus = "failed"
	StatusCancelled SessionStatus = "cancelled"
)

// OutputEntry is a structured log entry stored as JSONL in the output file.
type OutputEntry struct {
	Type    string      `json:"type"`              // "thought", "message", "tool_call", "tool_update", "plan"
	Text    string      `json:"text,omitempty"`    // thought/message text
	Kind    string      `json:"kind,omitempty"`    // tool kind
	Title   string      `json:"title,omitempty"`   // tool title
	Status  string      `json:"status,omitempty"`  // tool status
	Entries []PlanEntry `json:"entries,omitempty"` // plan entries
}

// PlanEntry represents a single step in a plan.
type PlanEntry struct {
	Status  string `json:"status"`
	Content string `json:"content"`
}

// MarshalEntry marshals an OutputEntry to JSON bytes.
func MarshalEntry(entry OutputEntry) ([]byte, error) {
	return json.Marshal(entry)
}
