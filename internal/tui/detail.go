package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/Cali0707/baton/internal/source"
	"github.com/Cali0707/baton/internal/store"
)

type detailModel struct {
	viewport viewport.Model
	item     *store.InboxItem
	comments []source.Comment
	diff     string
	ready    bool
	width    int
	height   int
}

func newDetailModel() detailModel {
	return detailModel{}
}

func (m *detailModel) setItem(item *store.InboxItem) {
	m.item = item
	m.comments = nil
	m.diff = ""
	m.updateContent()
}

func (m *detailModel) setComments(comments []source.Comment) {
	m.comments = comments
	m.updateContent()
}

func (m *detailModel) setDiff(diff string) {
	m.diff = diff
	m.updateContent()
}

func (m *detailModel) updateContent() {
	if m.item == nil {
		return
	}

	var b strings.Builder
	item := m.item

	kind := "Issue"
	if item.Kind == "pr" {
		kind = "Pull Request"
	}

	number := ""
	if item.Number != nil {
		number = fmt.Sprintf("#%d", *item.Number)
	}

	header := fmt.Sprintf("# %s %s: %s\n\n", kind, number, item.Title)
	if item.SourceState == "closed" || item.SourceState == "merged" {
		header = fmt.Sprintf("# %s %s (%s): %s\n\n", kind, number, item.SourceState, item.Title)
	}
	b.WriteString(header)
	b.WriteString(fmt.Sprintf("**Author:** %s  \n", item.Author))
	labels := ParseLabels(item.Labels)
	if len(labels) > 0 {
		b.WriteString(fmt.Sprintf("**Labels:** %s  \n", strings.Join(labels, ", ")))
	}
	b.WriteString("\n---\n\n")
	b.WriteString(item.Body)
	b.WriteString("\n")

	if len(m.comments) > 0 {
		b.WriteString("\n---\n\n## Comments\n\n")
		for _, c := range m.comments {
			b.WriteString(fmt.Sprintf("**%s:**\n%s\n\n", c.Author, c.Body))
		}
	}

	rendered, err := glamour.Render(b.String(), "dark")
	if err != nil {
		rendered = b.String()
	}

	m.viewport.SetContent(rendered)
	m.viewport.GotoTop()
}

func (m *detailModel) setSize(w, h int) {
	m.width = w
	m.height = h
	if !m.ready {
		m.viewport = viewport.New(w, h-4)
		m.viewport.YPosition = 2
		m.ready = true
	} else {
		m.viewport.Width = w
		m.viewport.Height = h - 4
	}
	m.updateContent()
}

func (m detailModel) Update(msg tea.Msg) (detailModel, tea.Cmd) {
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m detailModel) View() string {
	if m.item == nil {
		return "No item selected"
	}

	var b strings.Builder
	kind := "Issue"
	if m.item.Kind == "pr" {
		kind = "PR"
	}
	number := ""
	if m.item.Number != nil {
		number = fmt.Sprintf(" #%d", *m.item.Number)
	}
	b.WriteString(titleStyle.Render(fmt.Sprintf("%s%s", kind, number)))
	b.WriteString("\n")
	b.WriteString(m.viewport.View())
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("w workflow • a archive • esc back • up/down scroll"))
	return b.String()
}
