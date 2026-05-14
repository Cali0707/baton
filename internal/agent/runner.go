package agent

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"

	acp "github.com/coder/acp-go-sdk"
	"github.com/Cali0707/baton/internal/config"
)

// RunResult holds the outcome of an agent run.
type RunResult struct {
	SessionID  string
	StopReason acp.StopReason
	Updates    []acp.SessionUpdate
}

// Runner orchestrates an ACP agent subprocess lifecycle.
type Runner struct {
	agentDef config.AgentDef
	safety   config.SafetyConfig
	logger   *slog.Logger
}

func NewRunner(agentDef config.AgentDef, safety config.SafetyConfig, logger *slog.Logger) *Runner {
	if logger == nil {
		logger = slog.Default()
	}
	return &Runner{
		agentDef: agentDef,
		safety:   safety,
		logger:   logger,
	}
}

// Run starts the agent, creates a session, sends the prompt, and streams
// updates to the tracker until the agent completes. Returns the result.
func (r *Runner) Run(ctx context.Context, worktreePath, prompt string, tracker *SessionTracker) (*RunResult, error) {
	client := NewClient(worktreePath, r.safety, tracker, r.logger)

	// Start agent subprocess
	cmd := exec.CommandContext(ctx, r.agentDef.Cmd, r.agentDef.Args...)
	cmd.Dir = worktreePath

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stdout pipe: %w", err)
	}
	// Capture stderr from the agent for debugging.
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stderr pipe: %w", err)
	}
	go func() {
		stderrBytes, _ := io.ReadAll(stderrPipe)
		if len(stderrBytes) > 0 {
			r.logger.Error("agent stderr", "output", string(stderrBytes))
		}
	}()

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting agent %q: %w", r.agentDef.Cmd, err)
	}

	// Ensure cleanup on all exit paths
	defer func() {
		client.TerminalManager().ReleaseAll()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()

	conn := acp.NewClientSideConnection(client, stdin, stdout)
	conn.SetLogger(r.logger)

	// Initialize
	initResp, err := conn.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
		ClientCapabilities: acp.ClientCapabilities{
			Fs: acp.FileSystemCapability{
				ReadTextFile:  true,
				WriteTextFile: true,
			},
			Terminal: true,
		},
		ClientInfo: &acp.Implementation{
			Name:    "baton",
			Version: "0.1.0",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("ACP initialize: %w", err)
	}
	r.logger.Info("agent initialized", "protocol", initResp.ProtocolVersion)

	// New session
	sessResp, err := conn.NewSession(ctx, acp.NewSessionRequest{
		Cwd:        worktreePath,
		McpServers: []acp.McpServer{},
	})
	if err != nil {
		return nil, fmt.Errorf("ACP new session: %w", err)
	}
	sessionID := string(sessResp.SessionId)
	r.logger.Info("session created", "id", sessionID)

	// Send prompt
	promptResp, err := conn.Prompt(ctx, acp.PromptRequest{
		SessionId: sessResp.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock(prompt)},
	})
	if err != nil {
		return nil, fmt.Errorf("ACP prompt: %w", err)
	}

	return &RunResult{
		SessionID:  sessionID,
		StopReason: promptResp.StopReason,
		Updates:    tracker.Updates(),
	}, nil
}
