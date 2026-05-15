package runner

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Cali0707/baton/internal/agent"
	"github.com/Cali0707/baton/internal/config"
	"github.com/Cali0707/baton/internal/source"
	"github.com/Cali0707/baton/internal/store"
	"github.com/Cali0707/baton/internal/workflow"
	"github.com/Cali0707/baton/internal/worktree"
)

// Runner executes agent workflows against inbox items.
type Runner struct {
	store  store.Store
	source source.Source
	agents map[string]config.AgentDef
	safety config.SafetyConfig
	repos  []config.RepoConfig
	wtMgr  *worktree.Manager
	logger *slog.Logger
}

// New creates a Runner with the given dependencies.
func New(store store.Store, source source.Source, cfg *config.Config, logger *slog.Logger) *Runner {
	return &Runner{
		store:  store,
		source: source,
		agents: cfg.Agents,
		safety: cfg.Safety,
		repos:  cfg.Repos,
		wtMgr:  worktree.NewManager(),
		logger: logger,
	}
}

// Execute runs a workflow for the given run and item. It:
// 1. Finds repo config and creates/reuses a worktree
// 2. Fetches detail (comments, diff) via source
// 3. Builds prompt from workflow template
// 4. Runs the ACP agent subprocess
// 5. Updates the run record in the store on completion
func (r *Runner) Execute(ctx context.Context, run *store.Run, item *store.InboxItem, tracker *agent.SessionTracker) error {
	agentDef := r.agents[run.AgentName]
	repoConfig := r.repoConfigForItem(item)
	wfType := workflow.WorkflowType(run.WorkflowType)

	// Create worktree
	number := 0
	if item.Number != nil {
		number = *item.Number
	}
	var wtPath string
	var err error
	if item.Kind == "pr" {
		wtPath, err = r.wtMgr.CreateForPR(repoConfig.Path, number)
	} else {
		wtPath, err = r.wtMgr.CreateForIssue(repoConfig.Path, number)
	}
	if err != nil {
		return fmt.Errorf("creating worktree: %w", err)
	}

	// Update run with worktree path.
	run.WorktreePath = wtPath
	r.store.UpdateRun(ctx, run)

	// Build prompt
	promptData := workflow.PromptData{
		Title:  item.Title,
		Author: item.Author,
		Body:   item.Body,
		Number: number,
		Repo:   item.Owner + "/" + item.Repo,
	}

	// Fetch detail (comments, diff) from source.
	detail, err := r.source.FetchDetail(ctx, item)
	if err != nil {
		completedAt := time.Now().UTC()
		run.CompletedAt = &completedAt
		run.Status = store.StatusFailed
		r.store.UpdateRun(ctx, run)
		return fmt.Errorf("fetching detail for %s/%s#%d: %w", item.Owner, item.Repo, number, err)
	}
	if detail != nil {
		for _, c := range detail.Comments {
			promptData.Comments = append(promptData.Comments, workflow.CommentData{
				Author: c.Author,
				Body:   c.Body,
			})
		}
		promptData.Diff = detail.Diff
	}

	prompt, err := workflow.BuildPrompt(wfType, promptData)
	if err != nil {
		return fmt.Errorf("building prompt: %w", err)
	}

	// Run the agent
	agentRunner := agent.NewRunner(agentDef, r.safety, r.logger)
	defer tracker.Close()
	result, err := agentRunner.Run(ctx, wtPath, prompt, tracker)

	completedAt := time.Now().UTC()
	run.CompletedAt = &completedAt

	if err != nil {
		run.Status = store.StatusFailed
		r.store.UpdateRun(ctx, run)
		return err
	}

	run.Status = store.StatusCompleted
	run.AgentSessionID = result.SessionID
	if agentDef.ResumeCmd != "" {
		run.ResumeCmd = agentDef.BuildResumeCmd(result.SessionID)
	}
	r.store.UpdateRun(ctx, run)

	return nil
}

func (r *Runner) repoConfigForItem(item *store.InboxItem) config.RepoConfig {
	for _, repo := range r.repos {
		if repo.Owner == item.Owner && repo.Name == item.Repo {
			return repo
		}
	}
	return config.RepoConfig{Owner: item.Owner, Name: item.Repo}
}
