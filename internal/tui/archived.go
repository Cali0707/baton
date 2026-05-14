package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/Cali0707/baton/internal/store"
)

type archivedListModel struct {
	table  table.Model
	items  []*store.InboxItem
	width  int
	height int
}

func newArchivedListModel() archivedListModel {
	columns := []table.Column{
		{Title: "Type", Width: 6},
		{Title: "Repo", Width: 20},
		{Title: "#", Width: 6},
		{Title: "Title", Width: 40},
		{Title: "Author", Width: 15},
		{Title: "Updated", Width: 12},
	}

	t := table.New(
		table.WithColumns(columns),
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

	return archivedListModel{table: t}
}

func (m *archivedListModel) setItems(items []*store.InboxItem, repoLabel func(owner, repo string) string) {
	m.items = items
	rows := make([]table.Row, len(items))
	for i, item := range items {
		kind := "ISSUE"
		if item.Kind == "pr" {
			kind = "PR"
		}
		number := ""
		if item.Number != nil {
			number = fmt.Sprintf("%d", *item.Number)
		}
		updatedAt := item.UpdatedAt
		if item.SourceUpdatedAt != nil {
			updatedAt = *item.SourceUpdatedAt
		}
		rows[i] = table.Row{
			kind,
			repoLabel(item.Owner, item.Repo),
			number,
			truncate(item.Title, 38),
			item.Author,
			relativeTime(updatedAt),
		}
	}
	m.table.SetRows(rows)
}

func (m *archivedListModel) selectedItem() *store.InboxItem {
	idx := m.table.Cursor()
	if idx >= 0 && idx < len(m.items) {
		return m.items[idx]
	}
	return nil
}

func (m *archivedListModel) setSize(w, h int) {
	m.width = w
	m.height = h
	m.table.SetWidth(w)
	m.table.SetHeight(h - 5)
}

func (m archivedListModel) Update(msg tea.Msg) (archivedListModel, tea.Cmd) {
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m archivedListModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Archived Items"))
	b.WriteString("\n")
	if len(m.items) == 0 {
		b.WriteString("\n  No archived items.\n")
	} else {
		b.WriteString(m.table.View())
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("j/k navigate • enter view • u unarchive • esc back • q quit"))
	return b.String()
}
