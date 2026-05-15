package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	acp "github.com/coder/acp-go-sdk"
	"github.com/Cali0707/baton/internal/store"
)

// Styles for agent output segments.
var (
	thoughtStyle = lipgloss.NewStyle().
			Foreground(dimText).
			Italic(true)

	toolTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#5B44C0", Dark: "#A78BFA"}).
			Bold(true)

	toolStatusStyle = lipgloss.NewStyle().
			Foreground(dimText)

	messageStyle = lipgloss.NewStyle()
)

type segmentKind int

const (
	segMessage segmentKind = iota
	segThought
	segTool
	segPlan
)

type segment struct {
	kind     segmentKind
	raw      string // raw text (markdown source for messages)
	rendered string // styled output
}

type agentViewModel struct {
	viewport viewport.Model
	spinner  spinner.Model
	running  bool
	ready    bool
	width    int
	height   int
	title    string

	// Segment-based output.
	segments        []segment
	currentKind     segmentKind
	currentBuf      strings.Builder
	hasCurrentBlock bool

	// Cached rendering of completed segments.
	cachedView string
	cacheStale bool

	// Markdown rendering toggle.
	markdownEnabled bool

	// Tool call ID → title mapping.
	toolTitles map[acp.ToolCallId]string
}

func newAgentViewModel() agentViewModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(highlight)
	return agentViewModel{
		spinner:         s,
		running:         true,
		markdownEnabled: true,
		toolTitles:      make(map[acp.ToolCallId]string),
	}
}

func (m *agentViewModel) setTitle(title string) {
	m.title = title
}

func (m *agentViewModel) appendUpdate(update acp.SessionUpdate) {
	switch {
	case update.AgentThoughtChunk != nil:
		c := update.AgentThoughtChunk.Content
		if c.Text == nil || c.Text.Text == "" {
			break
		}
		// Flush message block if transitioning from message to thought.
		if m.hasCurrentBlock && m.currentKind == segMessage {
			m.flushCurrentBlock()
		}
		if !m.hasCurrentBlock {
			m.currentKind = segThought
			m.hasCurrentBlock = true
			m.currentBuf.WriteString("  thinking: ")
		}
		m.currentBuf.WriteString(c.Text.Text)

	case update.AgentMessageChunk != nil:
		c := update.AgentMessageChunk.Content
		if c.Text == nil || c.Text.Text == "" {
			break
		}
		// Flush thought block if transitioning from thought to message.
		if m.hasCurrentBlock && m.currentKind == segThought {
			m.flushCurrentBlock()
		}
		if !m.hasCurrentBlock {
			m.currentKind = segMessage
			m.hasCurrentBlock = true
		}
		m.currentBuf.WriteString(c.Text.Text)

	case update.ToolCall != nil:
		m.flushCurrentBlock()
		tc := update.ToolCall
		m.toolTitles[tc.ToolCallId] = tc.Title

		icon := toolIcon(tc.Kind)
		status := toolStatusStyle.Render(string(tc.Status))
		text := fmt.Sprintf("\n  %s %s  %s\n", icon, toolTitleStyle.Render(tc.Title), status)
		m.segments = append(m.segments, segment{kind: segTool, rendered: text})
		m.cacheStale = true

	case update.ToolCallUpdate != nil:
		m.flushCurrentBlock()
		tcu := update.ToolCallUpdate
		title := string(tcu.ToolCallId)
		if t, ok := m.toolTitles[tcu.ToolCallId]; ok {
			title = t
		}
		if tcu.Title != nil {
			m.toolTitles[tcu.ToolCallId] = *tcu.Title
			title = *tcu.Title
		}
		status := ""
		if tcu.Status != nil {
			status = string(*tcu.Status)
		}
		if status != "" {
			icon := "  "
			if status == "completed" {
				icon = "  +"
			} else if status == "failed" {
				icon = "  !"
			}
			text := fmt.Sprintf("%s %s  %s\n", icon, toolTitleStyle.Render(title), toolStatusStyle.Render(status))
			m.segments = append(m.segments, segment{kind: segTool, rendered: text})
			m.cacheStale = true
		}

	case update.Plan != nil:
		m.flushCurrentBlock()
		var pb strings.Builder
		pb.WriteString("\n  Plan:\n")
		for _, entry := range update.Plan.Entries {
			marker := "  "
			switch entry.Status {
			case acp.PlanEntryStatusCompleted:
				marker = "  [x]"
			case acp.PlanEntryStatusInProgress:
				marker = "  [>]"
			default:
				marker = "  [ ]"
			}
			pb.WriteString(fmt.Sprintf("%s %s\n", marker, entry.Content))
		}
		pb.WriteString("\n")
		m.segments = append(m.segments, segment{kind: segPlan, rendered: pb.String()})
		m.cacheStale = true
	}

	m.rebuildViewport()
}

// flushCurrentBlock finalizes the in-progress block as a completed segment.
func (m *agentViewModel) flushCurrentBlock() {
	if !m.hasCurrentBlock {
		return
	}
	raw := m.currentBuf.String()
	m.currentBuf.Reset()
	m.hasCurrentBlock = false

	seg := segment{kind: m.currentKind, raw: raw}
	switch m.currentKind {
	case segMessage:
		if m.markdownEnabled {
			seg.rendered = renderMarkdown(raw, m.width-2)
		} else {
			seg.rendered = messageStyle.Render(raw)
		}
	case segThought:
		seg.rendered = thoughtStyle.Render(raw)
	}
	m.segments = append(m.segments, seg)
	m.cacheStale = true
}

// rebuildViewport renders completed segments (cached) plus in-progress text.
func (m *agentViewModel) rebuildViewport() {
	if m.cacheStale {
		var b strings.Builder
		for i, seg := range m.segments {
			if i > 0 {
				b.WriteString(segmentSeparator(m.segments[i-1].kind, seg.kind))
			}
			b.WriteString(seg.rendered)
		}
		m.cachedView = b.String()
		m.cacheStale = false
	}

	var full strings.Builder
	full.WriteString(m.cachedView)

	if m.hasCurrentBlock {
		if len(m.segments) > 0 {
			full.WriteString(segmentSeparator(m.segments[len(m.segments)-1].kind, m.currentKind))
		}
		raw := m.currentBuf.String()
		switch m.currentKind {
		case segThought:
			full.WriteString(thoughtStyle.Render(raw))
		case segMessage:
			if m.markdownEnabled {
				full.WriteString(renderMarkdown(raw, m.width-2))
			} else {
				full.WriteString(messageStyle.Render(raw))
			}
		}
	}

	m.viewport.SetContent(agentOutputStyle.Render(full.String()))
	m.viewport.GotoBottom()
}

// segmentSeparator returns spacing between two adjacent segment kinds.
func segmentSeparator(prev, next segmentKind) string {
	if prev == segThought && next == segMessage {
		return "\n\n"
	}
	if prev == segMessage && next == segThought {
		return "\n"
	}
	return "\n"
}

func (m *agentViewModel) toggleMarkdown() {
	m.markdownEnabled = !m.markdownEnabled
	for i := range m.segments {
		if m.segments[i].kind == segMessage {
			if m.markdownEnabled {
				m.segments[i].rendered = renderMarkdown(m.segments[i].raw, m.width-2)
			} else {
				m.segments[i].rendered = messageStyle.Render(m.segments[i].raw)
			}
		}
	}
	m.cacheStale = true
	m.rebuildViewport()
}

func (m *agentViewModel) setDone() {
	m.flushCurrentBlock()
	m.running = false
	m.rebuildViewport()
}

func (m *agentViewModel) reset() {
	m.segments = nil
	m.currentBuf.Reset()
	m.hasCurrentBlock = false
	m.cachedView = ""
	m.cacheStale = false
	m.running = true
	m.toolTitles = make(map[acp.ToolCallId]string)
	m.viewport.SetContent("")
	m.viewport.GotoTop()
}

func (m *agentViewModel) setSize(w, h int) {
	oldWidth := m.width
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
	if oldWidth != w && m.markdownEnabled {
		for i := range m.segments {
			if m.segments[i].kind == segMessage {
				m.segments[i].rendered = renderMarkdown(m.segments[i].raw, w-2)
			}
		}
		m.cacheStale = true
		m.rebuildViewport()
	}
}

func (m agentViewModel) Update(msg tea.Msg) (agentViewModel, tea.Cmd) {
	var cmds []tea.Cmd

	if m.running {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	return m, tea.Batch(cmds...)
}

func (m agentViewModel) View() string {
	var b strings.Builder

	header := m.title
	if m.running {
		header = m.spinner.View() + " " + header + " (running)"
	} else {
		header = "+ " + header + " (complete)"
	}
	b.WriteString(titleStyle.Render(header))
	b.WriteString("\n")
	b.WriteString(m.viewport.View())
	b.WriteString("\n")
	if m.running {
		b.WriteString(helpStyle.Render("esc detach • x cancel • m markdown • up/down scroll"))
	} else {
		b.WriteString(helpStyle.Render("esc back • m markdown • up/down scroll"))
	}
	return b.String()
}

// renderEntries formats stored output entries, optionally rendering markdown for messages.
func renderEntries(entries []store.OutputEntry, markdownEnabled bool, width int) string {
	var b strings.Builder
	var messageBuf strings.Builder
	inThought := false

	flushMessage := func() {
		if messageBuf.Len() == 0 {
			return
		}
		raw := messageBuf.String()
		messageBuf.Reset()
		if markdownEnabled {
			b.WriteString(renderMarkdown(raw, width))
		} else {
			b.WriteString(messageStyle.Render(raw))
		}
	}

	endBlocks := func() {
		if inThought {
			b.WriteString("\n")
			inThought = false
		}
		flushMessage()
	}

	for _, e := range entries {
		switch e.Type {
		case "thought":
			flushMessage()
			if e.Text == "" {
				continue
			}
			if !inThought {
				b.WriteString(thoughtStyle.Render("  thinking: "))
				inThought = true
			}
			b.WriteString(thoughtStyle.Render(e.Text))

		case "message":
			if e.Text == "" {
				continue
			}
			if inThought {
				b.WriteString("\n\n")
				inThought = false
			}
			messageBuf.WriteString(e.Text)

		case "tool_call":
			endBlocks()
			icon := toolIcon(acp.ToolKind(e.Kind))
			status := toolStatusStyle.Render(e.Status)
			b.WriteString(fmt.Sprintf("\n  %s %s  %s\n", icon, toolTitleStyle.Render(e.Title), status))

		case "tool_update":
			endBlocks()
			icon := "  "
			if e.Status == "completed" {
				icon = "  +"
			} else if e.Status == "failed" {
				icon = "  !"
			}
			b.WriteString(fmt.Sprintf("%s %s  %s\n", icon, toolTitleStyle.Render(e.Title), toolStatusStyle.Render(e.Status)))

		case "plan":
			endBlocks()
			b.WriteString("\n  Plan:\n")
			for _, pe := range e.Entries {
				marker := "  "
				switch pe.Status {
				case "completed":
					marker = "  [x]"
				case "in_progress":
					marker = "  [>]"
				default:
					marker = "  [ ]"
				}
				b.WriteString(fmt.Sprintf("%s %s\n", marker, pe.Content))
			}
			b.WriteString("\n")
		}
	}
	endBlocks()
	return b.String()
}

var (
	mdRenderer      *glamour.TermRenderer
	mdRendererWidth int
)

func getMarkdownRenderer(width int) *glamour.TermRenderer {
	if width <= 0 {
		width = 80
	}
	if mdRenderer != nil && mdRendererWidth == width {
		return mdRenderer
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil
	}
	mdRenderer = r
	mdRendererWidth = width
	return r
}

// renderMarkdown renders text as styled terminal markdown using glamour.
func renderMarkdown(text string, width int) string {
	r := getMarkdownRenderer(width)
	if r == nil {
		return messageStyle.Render(text)
	}
	out, err := r.Render(text)
	if err != nil {
		return messageStyle.Render(text)
	}
	return strings.TrimRight(out, "\n")
}

// toolIcon returns a short icon string based on tool kind.
func toolIcon(kind acp.ToolKind) string {
	switch kind {
	case acp.ToolKindRead:
		return "R"
	case acp.ToolKindEdit:
		return "E"
	case acp.ToolKindDelete:
		return "D"
	case acp.ToolKindSearch:
		return "?"
	case acp.ToolKindExecute:
		return "$"
	case acp.ToolKindFetch:
		return ">"
	case acp.ToolKindThink:
		return "~"
	default:
		return "*"
	}
}