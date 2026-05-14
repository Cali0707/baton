package agent

import (
	"context"
	"fmt"
	"log/slog"

	acp "github.com/coder/acp-go-sdk"
	"github.com/Cali0707/baton/internal/config"
)

// Client implements acp.Client, providing filesystem, terminal, and permission
// handling to an ACP agent subprocess.
type Client struct {
	fs      *FilesystemManager
	term    *TerminalManager
	tracker *SessionTracker
	sandbox string // worktree path — all fs ops are sandboxed here
	logger  *slog.Logger
}

var _ acp.Client = (*Client)(nil)

func NewClient(sandbox string, safety config.SafetyConfig, tracker *SessionTracker, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		fs:      NewFilesystemManager(safety.MaxFileWriteBytes),
		term:    NewTerminalManager(safety.AllowedCommands),
		tracker: tracker,
		sandbox: sandbox,
		logger:  logger,
	}
}

// TerminalManager returns the underlying terminal manager for cleanup.
func (c *Client) TerminalManager() *TerminalManager {
	return c.term
}

// --- acp.Client interface ---

func (c *Client) ReadTextFile(ctx context.Context, params acp.ReadTextFileRequest) (acp.ReadTextFileResponse, error) {
	c.logger.Debug("ReadTextFile", "path", params.Path)
	content, err := c.fs.ReadTextFile(c.sandbox, params.Path, params.Line, params.Limit)
	if err != nil {
		return acp.ReadTextFileResponse{}, err
	}
	return acp.ReadTextFileResponse{Content: content}, nil
}

func (c *Client) WriteTextFile(ctx context.Context, params acp.WriteTextFileRequest) (acp.WriteTextFileResponse, error) {
	c.logger.Debug("WriteTextFile", "path", params.Path, "bytes", len(params.Content))
	if err := c.fs.WriteTextFile(c.sandbox, params.Path, params.Content); err != nil {
		return acp.WriteTextFileResponse{}, err
	}
	return acp.WriteTextFileResponse{}, nil
}

func (c *Client) RequestPermission(ctx context.Context, params acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	title := ""
	if params.ToolCall.Title != nil {
		title = *params.ToolCall.Title
	}
	c.logger.Debug("RequestPermission", "title", title, "options", len(params.Options))

	if len(params.Options) == 0 {
		return acp.RequestPermissionResponse{
			Outcome: acp.RequestPermissionOutcome{
				Cancelled: &acp.RequestPermissionOutcomeCancelled{},
			},
		}, nil
	}

	// Auto-approve: select the best "allow" option.
	// Prefer AllowAlways > AllowOnce, fall back to first option.
	bestIdx := 0
	bestPriority := -1
	for i, opt := range params.Options {
		priority := 0
		switch opt.Kind {
		case acp.PermissionOptionKindAllowAlways:
			priority = 2
		case acp.PermissionOptionKindAllowOnce:
			priority = 1
		}
		if priority > bestPriority {
			bestPriority = priority
			bestIdx = i
		}
	}

	return acp.RequestPermissionResponse{
		Outcome: acp.RequestPermissionOutcome{
			Selected: &acp.RequestPermissionOutcomeSelected{
				OptionId: params.Options[bestIdx].OptionId,
			},
		},
	}, nil
}

func (c *Client) SessionUpdate(ctx context.Context, params acp.SessionNotification) error {
	c.tracker.AddUpdate(params.Update)
	return nil
}

func (c *Client) CreateTerminal(ctx context.Context, params acp.CreateTerminalRequest) (acp.CreateTerminalResponse, error) {
	c.logger.Debug("CreateTerminal", "command", params.Command, "args", params.Args)

	cwd := params.Cwd
	if cwd == nil {
		cwd = &c.sandbox
	}

	if err := c.fs.ValidatePath(c.sandbox, *cwd); err != nil {
		return acp.CreateTerminalResponse{}, fmt.Errorf("terminal cwd rejected: %w", err)
	}

	id, err := c.term.Create(params.Command, params.Args, cwd)
	if err != nil {
		return acp.CreateTerminalResponse{}, err
	}
	return acp.CreateTerminalResponse{TerminalId: id}, nil
}

func (c *Client) KillTerminalCommand(ctx context.Context, params acp.KillTerminalCommandRequest) (acp.KillTerminalCommandResponse, error) {
	c.logger.Debug("KillTerminalCommand", "terminalId", params.TerminalId)
	if err := c.term.Kill(params.TerminalId); err != nil {
		return acp.KillTerminalCommandResponse{}, err
	}
	return acp.KillTerminalCommandResponse{}, nil
}

func (c *Client) TerminalOutput(ctx context.Context, params acp.TerminalOutputRequest) (acp.TerminalOutputResponse, error) {
	output, truncated, exitCode, signal, err := c.term.GetOutput(params.TerminalId)
	if err != nil {
		return acp.TerminalOutputResponse{}, err
	}

	resp := acp.TerminalOutputResponse{
		Output:    output,
		Truncated: truncated,
	}
	if exitCode != nil || signal != nil {
		resp.ExitStatus = &acp.TerminalExitStatus{
			ExitCode: exitCode,
			Signal:   signal,
		}
	}
	return resp, nil
}

func (c *Client) ReleaseTerminal(ctx context.Context, params acp.ReleaseTerminalRequest) (acp.ReleaseTerminalResponse, error) {
	c.logger.Debug("ReleaseTerminal", "terminalId", params.TerminalId)
	if err := c.term.Release(params.TerminalId); err != nil {
		return acp.ReleaseTerminalResponse{}, err
	}
	return acp.ReleaseTerminalResponse{}, nil
}

func (c *Client) WaitForTerminalExit(ctx context.Context, params acp.WaitForTerminalExitRequest) (acp.WaitForTerminalExitResponse, error) {
	c.logger.Debug("WaitForTerminalExit", "terminalId", params.TerminalId)
	exitCode, signal, err := c.term.WaitForExit(params.TerminalId)
	if err != nil {
		return acp.WaitForTerminalExitResponse{}, fmt.Errorf("waiting for terminal %s: %w", params.TerminalId, err)
	}
	return acp.WaitForTerminalExitResponse{
		ExitCode: exitCode,
		Signal:   signal,
	}, nil
}
