package github

import "time"

type WorkItemKind string

const (
	KindIssue WorkItemKind = "issue"
	KindPR    WorkItemKind = "pr"
)

type WorkItem struct {
	Kind      WorkItemKind
	Owner     string
	Repo      string
	Number    int
	Title     string
	Body      string
	Author    string
	Labels    []string
	State     string // "open", "closed", or "merged" (PR only)
	HeadRef   string // PR only
	Diff      string // PR only — populated after FetchPRDiff
	UpdatedAt time.Time
	Comments  []Comment
}

type Comment struct {
	Author    string
	Body      string
	CreatedAt time.Time
}
