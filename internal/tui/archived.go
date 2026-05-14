package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/Cali0707/baton/internal/store"
)

// Fixed column widths for the archived table. Title is flexible and computed in setSize.
const (
	archColTypeWidth    = 6
	archColRepoWidth    = 20
	archColNumWidth     = 6
	archColAuthorWidth  = 15
	archColUpdatedWidth = 12
	archFixedWidth      = archColTypeWidth + archColRepoWidth + archColNumWidth + archColAuthorWidth + archColUpdatedWidth
	archMinTitleWidth   = 20
)

type archivedListModel struct {
	table      table.Model
	items      []*store.InboxItem
	titleWidth int
	width      int
	height     int
}

func newArchivedListModel() archivedListModel {
	columns := archivedColumns(archMinTitleWidth)

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

	return archivedListModel{table: t, titleWidth: archMinTitleWidth}
}

func archivedColumns(titleWidth int) []table.Column {
	return []table.Column{
		{Title: "Type", Width: archColTypeWidth},
		{Title: "Repo", Width: archColRepoWidth},
		{Title: "#", Width: archColNumWidth},
		{Title: "Title", Width: titleWidth},
		{Title: "Author", Width: archColAuthorWidth},
		{Title: "Updated", Width: archColUpdatedWidth},
	}
}

func (m *archivedListModel) setItems(items []*store.InboxItem) {
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
			item.Owner + "/" + item.Repo,
			number,
			truncate(item.Title, m.titleWidth-2),
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

	titleWidth := w - archFixedWidth - 6
	if titleWidth < archMinTitleWidth {
		titleWidth = archMinTitleWidth
	}
	m.titleWidth = titleWidth
	m.table.SetColumns(archivedColumns(titleWidth))
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
