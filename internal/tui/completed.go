package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/Cali0707/baton/internal/store"
	"github.com/Cali0707/baton/internal/workflow"
)

// Fixed column widths for the completed runs table. Workflow is flexible.
const (
	compColAgentWidth  = 10
	compColStatusWidth = 12
	compColDateWidth   = 20
	compFixedWidth     = compColAgentWidth + compColStatusWidth + compColDateWidth
	compMinWfWidth     = 16
)

// completedListModel shows the history of completed agent runs.
type completedListModel struct {
	table   table.Model
	runs    []*store.Run
	wfWidth int
	width   int
	height  int
}

func newCompletedListModel() completedListModel {
	columns := completedColumns(compMinWfWidth)

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

	return completedListModel{table: t, wfWidth: compMinWfWidth}
}

func completedColumns(wfWidth int) []table.Column {
	return []table.Column{
		{Title: "Workflow", Width: wfWidth},
		{Title: "Agent", Width: compColAgentWidth},
		{Title: "Status", Width: compColStatusWidth},
		{Title: "Date", Width: compColDateWidth},
	}
}

func (m *completedListModel) setRuns(runs []*store.Run) {
	m.runs = runs
	rows := make([]table.Row, len(runs))
	for i, r := range runs {
		rows[i] = table.Row{
			workflow.WorkflowDisplayName(workflow.WorkflowType(r.WorkflowType)),
			r.AgentName,
			string(r.Status),
			r.StartedAt.Local().Format("2006-01-02 15:04"),
		}
	}
	m.table.SetRows(rows)
}

func (m *completedListModel) selectedRun() *store.Run {
	idx := m.table.Cursor()
	if idx >= 0 && idx < len(m.runs) {
		return m.runs[idx]
	}
	return nil
}

func (m *completedListModel) setSize(w, h int) {
	m.width = w
	m.height = h
	m.table.SetWidth(w)
	m.table.SetHeight(h - 5)

	wfWidth := w - compFixedWidth - 4
	if wfWidth < compMinWfWidth {
		wfWidth = compMinWfWidth
	}
	m.wfWidth = wfWidth
	m.table.SetColumns(completedColumns(wfWidth))
}

func (m completedListModel) Update(msg tea.Msg) (completedListModel, tea.Cmd) {
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m completedListModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Completed Runs"))
	b.WriteString("\n")
	if len(m.runs) == 0 {
		b.WriteString("\n  No completed runs yet.\n")
	} else {
		b.WriteString(m.table.View())
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("enter view • tab inbox • q quit"))
	return b.String()
}

// completedDetailModel shows details of a single completed run.
type completedDetailModel struct {
	viewport viewport.Model
	run      *store.Run
	entries  []store.OutputEntry
	ready    bool
	width    int
	height   int
}

func newCompletedDetailModel() completedDetailModel {
	return completedDetailModel{}
}

func (m *completedDetailModel) setRun(r *store.Run) {
	m.run = r
	m.entries = nil
	m.updateContent()
}

func (m *completedDetailModel) setEntries(entries []store.OutputEntry) {
	m.entries = entries
	m.updateContent()
}

func (m *completedDetailModel) updateContent() {
	if m.run == nil {
		return
	}
	r := m.run

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Run:      %d\n", r.ID))
	b.WriteString(fmt.Sprintf("Type:     %s\n", workflow.WorkflowDisplayName(workflow.WorkflowType(r.WorkflowType))))
	b.WriteString(fmt.Sprintf("Agent:    %s\n", r.AgentName))
	b.WriteString(fmt.Sprintf("Status:   %s\n", r.Status))
	b.WriteString(fmt.Sprintf("Started:  %s\n", r.StartedAt.Local().Format("2006-01-02 15:04:05")))
	if r.CompletedAt != nil {
		b.WriteString(fmt.Sprintf("Ended:    %s\n", r.CompletedAt.Local().Format("2006-01-02 15:04:05")))
	}
	b.WriteString(fmt.Sprintf("Worktree: %s\n", r.WorktreePath))
	b.WriteString("\n")

	if r.ResumeCmd != "" {
		b.WriteString("Resume command:\n")
		b.WriteString(fmt.Sprintf("  cd %s && %s\n", r.WorktreePath, r.ResumeCmd))
	}

	if len(m.entries) > 0 {
		b.WriteString("\n-- Agent Output ------------------------------------\n\n")
		b.WriteString(agentOutputStyle.Render(renderEntries(m.entries)))
	}

	m.viewport.SetContent(b.String())
	m.viewport.GotoTop()
}

func (m *completedDetailModel) setSize(w, h int) {
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

func (m completedDetailModel) Update(msg tea.Msg) (completedDetailModel, tea.Cmd) {
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m completedDetailModel) View() string {
	if m.run == nil {
		return "No run selected"
	}

	var b strings.Builder
	badge := completedBadge.Render(string(m.run.Status))
	if m.run.Status == store.StatusFailed {
		badge = failedBadge.Render("FAILED")
	}
	b.WriteString(titleStyle.Render(fmt.Sprintf("Run %d ", m.run.ID)) + badge)
	b.WriteString("\n")
	b.WriteString(m.viewport.View())
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("esc back"))
	return b.String()
}
