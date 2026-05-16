package tui

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	acp "github.com/coder/acp-go-sdk"
	"github.com/Cali0707/baton/internal/agent"
	"github.com/Cali0707/baton/internal/config"
	"github.com/Cali0707/baton/internal/runner"
	"github.com/Cali0707/baton/internal/source"
	"github.com/Cali0707/baton/internal/store"
	bsync "github.com/Cali0707/baton/internal/sync"
	"github.com/Cali0707/baton/internal/workflow"
)

type viewState int

const (
	viewInbox viewState = iota
	viewDetail
	viewWorkflowSelect
	viewAgentSelect
	viewAgentRunning
	viewCompleted
	viewCompletedList
	viewCompletedDetail
	viewRunningList
	viewArchivedList
)

// activeAgent holds per-run state for a background agent.
type activeAgent struct {
	runID     int64
	run       *store.Run
	item      *store.InboxItem
	tracker   *agent.SessionTracker
	cancel    context.CancelFunc
	view      agentViewModel
	startedAt time.Time
}

type Model struct {
	// Current view
	state viewState

	// Sub-models
	inbox           inboxModel
	detail          detailModel
	completedList   completedListModel
	completedDetail completedDetailModel
	archivedList    archivedListModel

	// Workflow selection state
	workflowOptions []workflow.WorkflowType
	workflowCursor  int

	// Agent selection state
	agentOptions []string
	agentCursor  int

	// Dependencies
	cfg    *config.Config
	source source.Source
	db     store.Store
	syncer bsync.SyncService
	runner *runner.Runner
	logger *slog.Logger

	// Multi-agent run state
	activeAgents  map[int64]*activeAgent
	focusedAgent  int64  // run ID of currently attached agent
	runningOrder  []int64
	runningCursor int

	// Navigation context
	currentItem  *store.InboxItem
	previousView viewState

	// Dimensions
	width  int
	height int

	// Loading state
	loading bool
	syncing bool
	spinner spinner.Model
	errMsg  string
}

func NewModel(cfg *config.Config, db store.Store, syncer bsync.SyncService, source source.Source, runner *runner.Runner, logger *slog.Logger) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		state:           viewInbox,
		inbox:           newInboxModel(),
		detail:          newDetailModel(),
		completedList:   newCompletedListModel(),
		completedDetail: newCompletedDetailModel(),
		archivedList:    newArchivedListModel(),
		cfg:             cfg,
		source:          source,
		db:              db,
		syncer:          syncer,
		runner:          runner,
		logger:          logger,
		spinner:         s,
		activeAgents:    make(map[int64]*activeAgent),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.loadItemsFromDB(),
		m.syncFromSources(),
		m.loadCompletedRuns(),
		m.spinner.Tick,
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.inbox.setSize(msg.Width, msg.Height)
		m.detail.setSize(msg.Width, msg.Height)
		m.completedList.setSize(msg.Width, msg.Height)
		m.completedDetail.setSize(msg.Width, msg.Height)
		m.archivedList.setSize(msg.Width, msg.Height)
		for _, aa := range m.activeAgents {
			aa.view.setSize(msg.Width, msg.Height)
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case WorkItemsLoaded:
		m.loading = false
		if msg.Err != nil {
			m.errMsg = msg.Err.Error()
		} else {
			m.inbox.setItems(msg.Items, m.repoDisplayLabel)
			m.errMsg = ""
		}

	case SyncComplete:
		m.syncing = false
		if msg.Err != nil {
			m.errMsg = "sync: " + msg.Err.Error()
		} else {
			m.errMsg = ""
			// Reload from DB to pick up any new/updated items.
			cmds = append(cmds, m.loadItemsFromDB())
		}

	case DetailLoaded:
		if msg.Err != nil {
			m.errMsg = msg.Err.Error()
		} else if msg.Detail != nil {
			m.detail.setComments(msg.Detail.Comments)
			if msg.Detail.Diff != "" {
				m.detail.setDiff(msg.Detail.Diff)
			}
		}

	case AgentUpdateMsg:
		if aa, ok := m.activeAgents[msg.RunID]; ok {
			aa.view.appendUpdate(msg.Update)
			if entry := makeOutputEntry(msg.Update); entry != nil {
				m.db.AppendEntry(msg.RunID, *entry)
			}
			cmds = append(cmds, m.listenForUpdates(msg.RunID, aa.tracker))
		}

	case AgentDoneMsg:
		if aa, ok := m.activeAgents[msg.RunID]; ok {
			aa.view.setDone()
			delete(m.activeAgents, msg.RunID)
			m.removeFromRunningOrder(msg.RunID)

			if msg.Err != nil {
				m.errMsg = msg.Err.Error()
			}
			if m.focusedAgent == msg.RunID {
				m.focusedAgent = 0
				if msg.Run != nil {
					m.completedDetail.setRun(msg.Run)
					m.state = viewCompleted
					cmds = append(cmds, m.loadRunOutput(msg.Run.ID))
				} else {
					m.state = viewInbox
				}
			}
			cmds = append(cmds, m.loadCompletedRuns())
		}

	case SessionOutputLoaded:
		if msg.Err != nil {
			m.errMsg = msg.Err.Error()
		} else if m.completedDetail.run != nil && m.completedDetail.run.ID == msg.RunID {
			m.completedDetail.setEntries(msg.Entries)
		}

	case completedRunsLoaded:
		m.completedList.setRuns(msg.runs)

	case archivedItemsLoaded:
		if msg.err != nil {
			m.errMsg = msg.err.Error()
		} else {
			m.archivedList.setItems(msg.items, m.repoDisplayLabel)
		}

	case ErrorMsg:
		m.errMsg = msg.Err.Error()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Update current sub-model
	switch m.state {
	case viewInbox:
		var cmd tea.Cmd
		m.inbox, cmd = m.inbox.Update(msg)
		cmds = append(cmds, cmd)
	case viewDetail:
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		cmds = append(cmds, cmd)
	case viewAgentRunning:
		if aa, ok := m.activeAgents[m.focusedAgent]; ok {
			var cmd tea.Cmd
			aa.view, cmd = aa.view.Update(msg)
			cmds = append(cmds, cmd)
		}
	case viewCompletedList:
		var cmd tea.Cmd
		m.completedList, cmd = m.completedList.Update(msg)
		cmds = append(cmds, cmd)
	case viewArchivedList:
		var cmd tea.Cmd
		m.archivedList, cmd = m.archivedList.Update(msg)
		cmds = append(cmds, cmd)
	case viewCompletedDetail, viewCompleted:
		var cmd tea.Cmd
		m.completedDetail, cmd = m.completedDetail.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.state {
	case viewInbox:
		switch {
		case msg.String() == "q" || msg.String() == "ctrl+c":
			m.cancelAllAgents()
			return m, tea.Quit
		case msg.String() == "enter":
			if item := m.inbox.selectedItem(); item != nil {
				m.currentItem = item
				m.previousView = viewInbox
				m.detail.setItem(item)
				m.state = viewDetail
				return m, m.loadDetail(item)
			}
		case msg.String() == "w":
			if item := m.inbox.selectedItem(); item != nil {
				if m.isItemBeingAnalyzed(item) {
					m.errMsg = fmt.Sprintf("#%d is already being analyzed", safeNumber(item))
					return m, nil
				}
				m.currentItem = item
				m.detail.setItem(item)
				return m.startWorkflowSelect(item)
			}
		case msg.String() == "a":
			if item := m.inbox.selectedItem(); item != nil {
				return m, m.archiveItem(item)
			}
		case msg.String() == "r":
			m.syncing = true
			return m, tea.Batch(m.spinner.Tick, m.syncFromSources())
		case msg.String() == "tab":
			m.state = viewCompletedList
			return m, nil
		case msg.String() == "A":
			m.state = viewArchivedList
			return m, m.loadArchivedItems()
		case msg.String() == "s":
			if len(m.activeAgents) > 0 {
				m.runningCursor = 0
				m.state = viewRunningList
			}
			return m, nil
		default:
			var cmd tea.Cmd
			m.inbox, cmd = m.inbox.Update(msg)
			return m, cmd
		}

	case viewDetail:
		switch {
		case msg.String() == "esc":
			m.state = m.previousView
			return m, nil
		case msg.String() == "w":
			if m.currentItem != nil {
				if m.isItemBeingAnalyzed(m.currentItem) {
					m.errMsg = fmt.Sprintf("#%d is already being analyzed", safeNumber(m.currentItem))
					return m, nil
				}
				return m.startWorkflowSelect(m.currentItem)
			}
		case msg.String() == "a":
			if m.currentItem != nil {
				m.state = m.previousView
				return m, m.archiveItem(m.currentItem)
			}
		case msg.String() == "q":
			m.cancelAllAgents()
			return m, tea.Quit
		default:
			var cmd tea.Cmd
			m.detail, cmd = m.detail.Update(msg)
			return m, cmd
		}

	case viewWorkflowSelect:
		switch {
		case msg.String() == "esc":
			m.state = viewDetail
			return m, nil
		case msg.String() == "j" || msg.String() == "down":
			if m.workflowCursor < len(m.workflowOptions)-1 {
				m.workflowCursor++
			}
		case msg.String() == "k" || msg.String() == "up":
			if m.workflowCursor > 0 {
				m.workflowCursor--
			}
		case msg.String() == "enter":
			wfType := m.workflowOptions[m.workflowCursor]
			return m.startAgentSelect(wfType)
		case msg.String() == "q":
			m.cancelAllAgents()
			return m, tea.Quit
		}

	case viewAgentSelect:
		switch {
		case msg.String() == "esc":
			m.state = viewWorkflowSelect
			return m, nil
		case msg.String() == "j" || msg.String() == "down":
			if m.agentCursor < len(m.agentOptions)-1 {
				m.agentCursor++
			}
		case msg.String() == "k" || msg.String() == "up":
			if m.agentCursor > 0 {
				m.agentCursor--
			}
		case msg.String() == "enter":
			agentName := m.agentOptions[m.agentCursor]
			return m.startAgent(agentName)
		case msg.String() == "q":
			m.cancelAllAgents()
			return m, tea.Quit
		}

	case viewAgentRunning:
		switch {
		case msg.String() == "esc":
			m.focusedAgent = 0
			m.state = viewInbox
			return m, nil
		case msg.String() == "x":
			if aa, ok := m.activeAgents[m.focusedAgent]; ok {
				aa.cancel()
			}
			m.focusedAgent = 0
			m.state = viewInbox
			return m, nil
		case msg.String() == "m":
			if aa, ok := m.activeAgents[m.focusedAgent]; ok {
				aa.view.toggleMarkdown()
			}
			return m, nil
		default:
			if aa, ok := m.activeAgents[m.focusedAgent]; ok {
				var cmd tea.Cmd
				aa.view, cmd = aa.view.Update(msg)
				return m, cmd
			}
		}

	case viewRunningList:
		switch {
		case msg.String() == "esc":
			m.state = viewInbox
			return m, nil
		case msg.String() == "j" || msg.String() == "down":
			if m.runningCursor < len(m.runningOrder)-1 {
				m.runningCursor++
			}
		case msg.String() == "k" || msg.String() == "up":
			if m.runningCursor > 0 {
				m.runningCursor--
			}
		case msg.String() == "enter":
			if m.runningCursor < len(m.runningOrder) {
				rid := m.runningOrder[m.runningCursor]
				if _, ok := m.activeAgents[rid]; ok {
					m.focusedAgent = rid
					m.state = viewAgentRunning
				}
			}
			return m, nil
		case msg.String() == "x":
			if m.runningCursor < len(m.runningOrder) {
				rid := m.runningOrder[m.runningCursor]
				if aa, ok := m.activeAgents[rid]; ok {
					aa.cancel()
				}
			}
			return m, nil
		case msg.String() == "q":
			m.cancelAllAgents()
			return m, tea.Quit
		}

	case viewArchivedList:
		switch {
		case msg.String() == "q" || msg.String() == "ctrl+c":
			m.cancelAllAgents()
			return m, tea.Quit
		case msg.String() == "esc":
			m.state = viewInbox
			return m, nil
		case msg.String() == "u":
			if item := m.archivedList.selectedItem(); item != nil {
				return m, m.unarchiveItem(item)
			}
		case msg.String() == "enter":
			if item := m.archivedList.selectedItem(); item != nil {
				m.currentItem = item
				m.previousView = viewArchivedList
				m.detail.setItem(item)
				m.state = viewDetail
				return m, m.loadDetail(item)
			}
		default:
			var cmd tea.Cmd
			m.archivedList, cmd = m.archivedList.Update(msg)
			return m, cmd
		}

	case viewCompleted:
		switch {
		case msg.String() == "esc" || msg.String() == "q":
			m.state = viewInbox
			return m, nil
		case msg.String() == "m":
			m.completedDetail.toggleMarkdown()
			return m, nil
		default:
			var cmd tea.Cmd
			m.completedDetail, cmd = m.completedDetail.Update(msg)
			return m, cmd
		}

	case viewCompletedList:
		switch {
		case msg.String() == "q" || msg.String() == "ctrl+c":
			m.cancelAllAgents()
			return m, tea.Quit
		case msg.String() == "tab":
			m.state = viewInbox
			return m, nil
		case msg.String() == "enter":
			if r := m.completedList.selectedRun(); r != nil {
				m.completedDetail.setRun(r)
				m.state = viewCompletedDetail
				return m, m.loadRunOutput(r.ID)
			}
		default:
			var cmd tea.Cmd
			m.completedList, cmd = m.completedList.Update(msg)
			return m, cmd
		}

	case viewCompletedDetail:
		switch {
		case msg.String() == "esc":
			m.state = viewCompletedList
			return m, nil
		case msg.String() == "q":
			m.cancelAllAgents()
			return m, tea.Quit
		case msg.String() == "m":
			m.completedDetail.toggleMarkdown()
			return m, nil
		default:
			var cmd tea.Cmd
			m.completedDetail, cmd = m.completedDetail.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

func (m Model) View() string {
	if m.errMsg != "" && m.state == viewInbox {
		return m.renderInboxView() + "\n" + statusBarStyle.Render("Error: "+m.errMsg)
	}

	switch m.state {
	case viewInbox:
		if m.loading {
			return m.spinner.View() + " Loading..."
		}
		return m.renderInboxView()

	case viewDetail:
		return m.detail.View()

	case viewWorkflowSelect:
		return m.renderWorkflowSelect()

	case viewAgentSelect:
		return m.renderAgentSelect()

	case viewAgentRunning:
		if aa, ok := m.activeAgents[m.focusedAgent]; ok {
			return aa.view.View()
		}
		return "Agent not found"

	case viewCompleted:
		return m.completedDetail.View()

	case viewCompletedList:
		return m.completedList.View()

	case viewCompletedDetail:
		return m.completedDetail.View()

	case viewRunningList:
		return m.renderRunningList()

	case viewArchivedList:
		return m.archivedList.View()
	}

	return ""
}

// renderInboxView renders the inbox with badges for running agents and sync status.
func (m Model) renderInboxView() string {
	base := m.inbox.View()
	var badges []string
	if len(m.activeAgents) > 0 {
		badges = append(badges, runningBadge.Render(fmt.Sprintf(" %d running ", len(m.activeAgents))))
	}
	if m.syncing {
		badges = append(badges, statusBarStyle.Render("syncing..."))
	}
	if len(badges) > 0 {
		lines := strings.SplitN(base, "\n", 2)
		if len(lines) == 2 {
			return lines[0] + " " + strings.Join(badges, " ") + "\n" + lines[1]
		}
		return base + " " + strings.Join(badges, " ")
	}
	return base
}

// renderRunningList shows all active agents.
func (m Model) renderRunningList() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Running Agents"))
	b.WriteString("\n\n")

	if len(m.runningOrder) == 0 {
		b.WriteString("  No agents running.\n")
	} else {
		for i, rid := range m.runningOrder {
			aa, ok := m.activeAgents[rid]
			if !ok {
				continue
			}
			cursor := "  "
			style := lipgloss.NewStyle()
			if i == m.runningCursor {
				cursor = "▸ "
				style = selectedStyle
			}
			elapsed := time.Since(aa.startedAt).Truncate(time.Second)
			label := fmt.Sprintf("%s #%d — %s", m.repoDisplayLabel(aa.item.Owner, aa.item.Repo), safeNumber(aa.item), elapsed)
			b.WriteString(cursor + style.Render(label) + "\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("enter attach • x cancel • esc back"))
	return b.String()
}

// --- Workflow helpers ---

func (m *Model) startWorkflowSelect(item *store.InboxItem) (tea.Model, tea.Cmd) {
	if item.Kind == "pr" {
		return m.startAgentSelect(workflow.WorkflowPR)
	}
	m.workflowOptions = []workflow.WorkflowType{workflow.WorkflowBug, workflow.WorkflowFeature}
	m.workflowCursor = 0
	m.state = viewWorkflowSelect
	return *m, nil
}

func (m *Model) startAgentSelect(wfType workflow.WorkflowType) (tea.Model, tea.Cmd) {
	m.agentOptions = nil
	repoConfig := m.repoConfigForItem(m.currentItem)
	defaultAgent := m.cfg.AgentForRepo(repoConfig)

	if defaultAgent != "" {
		m.agentOptions = append(m.agentOptions, defaultAgent)
	}
	for name := range m.cfg.Agents {
		if name != defaultAgent {
			m.agentOptions = append(m.agentOptions, name)
		}
	}
	m.agentCursor = 0

	m.workflowOptions = []workflow.WorkflowType{wfType}

	if len(m.agentOptions) == 1 {
		return m.startAgent(m.agentOptions[0])
	}

	m.state = viewAgentSelect
	return *m, nil
}

func (m *Model) startAgent(agentName string) (tea.Model, tea.Cmd) {
	item := m.currentItem
	wfType := m.workflowOptions[0]

	// Create the run in the DB first to get the integer ID.
	now := time.Now().UTC()
	run := &store.Run{
		InboxItemID:  item.ID,
		WorkflowType: string(wfType),
		AgentName:    agentName,
		Status:       store.StatusRunning,
		StartedAt:    now,
	}
	ctx := context.Background()
	if err := m.db.CreateRun(ctx, run); err != nil {
		m.errMsg = fmt.Sprintf("creating run: %v", err)
		return *m, nil
	}

	// Update item status.
	m.db.UpdateItemStatus(ctx, item.ID, store.ItemStatusInProgress)

	tracker := agent.NewSessionTracker()
	cancelCtx, cancel := context.WithCancel(ctx)

	view := newAgentViewModel()
	view.setTitle(fmt.Sprintf("%s — %s #%d", workflow.WorkflowDisplayName(wfType), m.repoDisplayLabel(item.Owner, item.Repo), safeNumber(item)))
	view.setSize(m.width, m.height)

	aa := &activeAgent{
		runID:     run.ID,
		run:       run,
		item:      item,
		tracker:   tracker,
		cancel:    cancel,
		view:      view,
		startedAt: now,
	}
	m.activeAgents[run.ID] = aa
	m.runningOrder = append(m.runningOrder, run.ID)

	m.state = viewInbox

	return *m, tea.Batch(
		m.spinner.Tick,
		m.runAgent(cancelCtx, run, item, tracker),
		m.listenForUpdates(run.ID, tracker),
	)
}

func (m Model) repoConfigForItem(item *store.InboxItem) config.RepoConfig {
	for _, r := range m.cfg.Repos {
		if r.Owner == item.Owner && r.Name == item.Repo {
			return r
		}
	}
	return config.RepoConfig{Owner: item.Owner, Name: item.Repo}
}

func (m Model) repoDisplayLabel(owner, repo string) string {
	for _, r := range m.cfg.Repos {
		if r.Owner == owner && r.Name == repo {
			return r.DisplayLabel()
		}
	}
	return owner + "/" + repo
}

// --- Duplicate guard ---

func (m Model) isItemBeingAnalyzed(item *store.InboxItem) bool {
	for _, aa := range m.activeAgents {
		if aa.item.ID == item.ID {
			return true
		}
	}
	return false
}

// --- Cancellation ---

func (m *Model) cancelAllAgents() {
	for _, aa := range m.activeAgents {
		aa.cancel()
		if aa.run != nil {
			aa.run.Status = store.StatusCancelled
			now := time.Now().UTC()
			aa.run.CompletedAt = &now
			m.db.UpdateRun(context.Background(), aa.run)
		}
	}
}

func (m *Model) removeFromRunningOrder(runID int64) {
	for i, rid := range m.runningOrder {
		if rid == runID {
			m.runningOrder = append(m.runningOrder[:i], m.runningOrder[i+1:]...)
			if m.runningCursor >= len(m.runningOrder) && m.runningCursor > 0 {
				m.runningCursor--
			}
			return
		}
	}
}

// --- Tea commands ---

func (m Model) loadItemsFromDB() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		items, err := m.db.ListItems(ctx, []store.ItemStatus{
			store.ItemStatusNew,
			store.ItemStatusInProgress,
			store.ItemStatusDone,
		})
		return WorkItemsLoaded{Items: items, Err: err}
	}
}

func (m Model) syncFromSources() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		newCount, updatedCount, err := m.syncer.Sync(ctx)
		return SyncComplete{NewCount: newCount, UpdatedCount: updatedCount, Err: err}
	}
}

func (m Model) loadDetail(item *store.InboxItem) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		detail, err := m.source.FetchDetail(ctx, item)
		if err != nil {
			return DetailLoaded{Err: err}
		}
		return DetailLoaded{Detail: detail}
	}
}

func (m Model) loadCompletedRuns() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		runs, err := m.db.ListRuns(ctx, []store.SessionStatus{store.StatusCompleted, store.StatusFailed})
		if err != nil {
			return ErrorMsg{Err: err}
		}
		return completedRunsLoaded{runs: runs}
	}
}

func (m Model) loadRunOutput(runID int64) tea.Cmd {
	return func() tea.Msg {
		entries, err := m.db.LoadEntries(runID)
		return SessionOutputLoaded{RunID: runID, Entries: entries, Err: err}
	}
}

func (m Model) archiveItem(item *store.InboxItem) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		if err := m.db.UpdateItemStatus(ctx, item.ID, store.ItemStatusArchived); err != nil {
			return ErrorMsg{Err: err}
		}
		// Reload inbox.
		items, err := m.db.ListItems(ctx, []store.ItemStatus{
			store.ItemStatusNew,
			store.ItemStatusInProgress,
			store.ItemStatusDone,
		})
		return WorkItemsLoaded{Items: items, Err: err}
	}
}

type completedRunsLoaded struct {
	runs []*store.Run
}

type archivedItemsLoaded struct {
	items []*store.InboxItem
	err   error
}

func (m Model) loadArchivedItems() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		items, err := m.db.ListItems(ctx, []store.ItemStatus{store.ItemStatusArchived})
		return archivedItemsLoaded{items: items, err: err}
	}
}

func (m Model) unarchiveItem(item *store.InboxItem) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		if err := m.db.UpdateItemStatus(ctx, item.ID, store.ItemStatusNew); err != nil {
			return ErrorMsg{Err: err}
		}
		// Reload both archived list and inbox.
		items, err := m.db.ListItems(ctx, []store.ItemStatus{store.ItemStatusArchived})
		return archivedItemsLoaded{items: items, err: err}
	}
}

func (m Model) runAgent(ctx context.Context, run *store.Run, item *store.InboxItem, tracker *agent.SessionTracker) tea.Cmd {
	return func() tea.Msg {
		err := m.runner.Execute(ctx, run, item, tracker)
		return AgentDoneMsg{RunID: run.ID, Run: run, Err: err}
	}
}

func (m Model) listenForUpdates(runID int64, tracker *agent.SessionTracker) tea.Cmd {
	return func() tea.Msg {
		update, ok := <-tracker.UpdateChan()
		if !ok {
			return nil
		}
		return AgentUpdateMsg{RunID: runID, Update: update}
	}
}

// --- Selector views ---

func (m Model) renderWorkflowSelect() string {
	var b fmt.Stringer = &workflowSelectView{
		options: m.workflowOptions,
		cursor:  m.workflowCursor,
		item:    m.currentItem,
	}
	return b.String()
}

func (m Model) renderAgentSelect() string {
	var b fmt.Stringer = &agentSelectView{
		options: m.agentOptions,
		cursor:  m.agentCursor,
	}
	return b.String()
}

type workflowSelectView struct {
	options []workflow.WorkflowType
	cursor  int
	item    *store.InboxItem
}

func (v *workflowSelectView) String() string {
	var b string
	b += titleStyle.Render(fmt.Sprintf("Analyze #%d: %s", safeNumber(v.item), v.item.Title))
	b += "\n\n"
	b += "  Select workflow type:\n\n"
	for i, opt := range v.options {
		cursor := "  "
		style := lipgloss.NewStyle()
		if i == v.cursor {
			cursor = "▸ "
			style = selectedStyle
		}
		b += cursor + style.Render(workflow.WorkflowDisplayName(opt)) + "\n"
	}
	b += "\n"
	b += helpStyle.Render("j/k navigate • enter select • esc back")
	return b
}

type agentSelectView struct {
	options []string
	cursor  int
}

func (v *agentSelectView) String() string {
	var b string
	b += titleStyle.Render("Select Agent")
	b += "\n\n"
	for i, opt := range v.options {
		cursor := "  "
		style := lipgloss.NewStyle()
		if i == v.cursor {
			cursor = "▸ "
			style = selectedStyle
		}
		label := opt
		if i == 0 {
			label += " (default)"
		}
		b += cursor + style.Render(label) + "\n"
	}
	b += "\n"
	b += helpStyle.Render("j/k navigate • enter select • esc back")
	return b
}

func makeOutputEntry(u acp.SessionUpdate) *store.OutputEntry {
	switch {
	case u.AgentMessageChunk != nil && u.AgentMessageChunk.Content.Text != nil:
		return &store.OutputEntry{Type: "message", Text: u.AgentMessageChunk.Content.Text.Text}
	case u.AgentThoughtChunk != nil && u.AgentThoughtChunk.Content.Text != nil:
		return &store.OutputEntry{Type: "thought", Text: u.AgentThoughtChunk.Content.Text.Text}
	case u.ToolCall != nil:
		return &store.OutputEntry{
			Type:   "tool_call",
			Kind:   string(u.ToolCall.Kind),
			Title:  u.ToolCall.Title,
			Status: string(u.ToolCall.Status),
		}
	case u.ToolCallUpdate != nil:
		status := ""
		if u.ToolCallUpdate.Status != nil {
			status = string(*u.ToolCallUpdate.Status)
		}
		title := string(u.ToolCallUpdate.ToolCallId)
		if u.ToolCallUpdate.Title != nil {
			title = *u.ToolCallUpdate.Title
		}
		if status != "" {
			return &store.OutputEntry{Type: "tool_update", Title: title, Status: status}
		}
	case u.Plan != nil:
		entries := make([]store.PlanEntry, len(u.Plan.Entries))
		for i, e := range u.Plan.Entries {
			entries[i] = store.PlanEntry{Status: string(e.Status), Content: e.Content}
		}
		return &store.OutputEntry{Type: "plan", Entries: entries}
	}
	return nil
}

// safeNumber returns the issue/PR number or 0 if nil.
func safeNumber(item *store.InboxItem) int {
	if item.Number != nil {
		return *item.Number
	}
	return 0
}
