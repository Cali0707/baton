package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/Cali0707/baton/internal/store"
)

var inboxLayout = []columnSpec{
	{Title: "St", Width: 5},
	{Title: "Type", Width: 6},
	{Title: "Repo", Width: 20},
	{Title: "#", Width: 6},
	{Title: "Title", Flex: true, MinWidth: 20},
	{Title: "Author", Width: 15},
	{Title: "Updated", Width: 12},
}

type inboxModel struct {
	table  table.Model
	layout tableLayout
	items  []*store.InboxItem
	width  int
	height int
}

func newInboxModel() inboxModel {
	layout := newTableLayout(inboxLayout)

	t := table.New(
		table.WithColumns(layout.columns()),
		table.WithFocused(true),
		table.WithHeight(20),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(subtle).
		BorderBottom(true).
		Bold(true)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#7D56F4")).
		Bold(true)
	t.SetStyles(s)

	return inboxModel{table: t, layout: layout}
}

func (m *inboxModel) setItems(items []*store.InboxItem, repoLabel func(owner, repo string) string) {
	m.items = items
	rows := make([]table.Row, len(items))
	for i, item := range items {
		kind := "ISSUE"
		if item.Kind == "pr" {
			kind = "PR"
		}
		status := statusLabel(item.Status)
		number := ""
		if item.Number != nil {
			number = fmt.Sprintf("%d", *item.Number)
		}
		updatedAt := item.UpdatedAt
		if item.SourceUpdatedAt != nil {
			updatedAt = *item.SourceUpdatedAt
		}
		rows[i] = table.Row{
			status,
			kind,
			repoLabel(item.Owner, item.Repo),
			number,
			truncate(item.Title, m.layout.FlexWidth()-2),
			item.Author,
			relativeTime(updatedAt),
		}
	}
	m.table.SetRows(rows)
}

func statusLabel(s store.ItemStatus) string {
	switch s {
	case store.ItemStatusNew:
		return "NEW"
	case store.ItemStatusInProgress:
		return "RUN"
	case store.ItemStatusDone:
		return "DONE"
	case store.ItemStatusArchived:
		return "ARCH"
	default:
		return string(s)
	}
}

func (m *inboxModel) selectedItem() *store.InboxItem {
	idx := m.table.Cursor()
	if idx >= 0 && idx < len(m.items) {
		return m.items[idx]
	}
	return nil
}

func (m inboxModel) Update(msg tea.Msg) (inboxModel, tea.Cmd) {
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m inboxModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Inbox"))
	b.WriteString("\n")
	b.WriteString(m.table.View())
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("j/k navigate • enter view • w workflow • a archive • A archived • s running • r refresh • tab history • q quit"))
	return b.String()
}

func (m *inboxModel) setSize(w, h int) {
	m.width = w
	m.height = h
	m.table.SetWidth(w)
	m.table.SetHeight(h - 5)
	m.table.SetColumns(m.layout.recompute(w))
}

// ParseLabels returns the labels from the JSON-encoded labels field.
func ParseLabels(labelsJSON string) []string {
	var labels []string
	json.Unmarshal([]byte(labelsJSON), &labels)
	return labels
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	}
}
