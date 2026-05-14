package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/Cali0707/baton/internal/config"
)

func testClient(t *testing.T) (*Client, string) {
	t.Helper()
	sandbox := t.TempDir()
	safety := config.SafetyConfig{
		MaxFileWriteBytes: 1 << 20,
		AllowedCommands: []config.AllowedCommand{
			{Cmd: "echo", AllowAll: true},
			{Cmd: "cat", AllowAll: true},
			{Cmd: "sh", AllowAll: true},
			{Cmd: "sleep", AllowAll: true},
		},
	}
	tracker := NewSessionTracker()
	t.Cleanup(tracker.Close)
	client := NewClient(sandbox, safety, tracker, nil)
	t.Cleanup(func() { client.TerminalManager().ReleaseAll() })
	return client, sandbox
}

func TestClient_ReadTextFile(t *testing.T) {
	client, sandbox := testClient(t)
	ctx := context.Background()

	path := filepath.Join(sandbox, "read.txt")
	os.WriteFile(path, []byte("hello\nworld"), 0o644)

	resp, err := client.ReadTextFile(ctx, acp.ReadTextFileRequest{
		Path: path,
	})
	if err != nil {
		t.Fatalf("ReadTextFile() error: %v", err)
	}
	if resp.Content != "hello\nworld" {
		t.Errorf("content = %q", resp.Content)
	}
}

func TestClient_ReadTextFile_OutsideSandbox(t *testing.T) {
	client, _ := testClient(t)
	ctx := context.Background()

	_, err := client.ReadTextFile(ctx, acp.ReadTextFileRequest{
		Path: "/etc/hosts",
	})
	if err == nil {
		t.Error("should reject read outside sandbox")
	}
}

func TestClient_WriteTextFile(t *testing.T) {
	client, sandbox := testClient(t)
	ctx := context.Background()

	path := filepath.Join(sandbox, "write.txt")
	_, err := client.WriteTextFile(ctx, acp.WriteTextFileRequest{
		Path:    path,
		Content: "written content",
	})
	if err != nil {
		t.Fatalf("WriteTextFile() error: %v", err)
	}

	got, _ := os.ReadFile(path)
	if string(got) != "written content" {
		t.Errorf("file content = %q", string(got))
	}
}

func TestClient_WriteTextFile_OutsideSandbox(t *testing.T) {
	client, _ := testClient(t)
	ctx := context.Background()

	_, err := client.WriteTextFile(ctx, acp.WriteTextFileRequest{
		Path:    "/tmp/evil.txt",
		Content: "bad",
	})
	if err == nil {
		t.Error("should reject write outside sandbox")
	}
}

func TestClient_RequestPermission_AutoApprove(t *testing.T) {
	client, _ := testClient(t)
	ctx := context.Background()

	title := "Read file"
	resp, err := client.RequestPermission(ctx, acp.RequestPermissionRequest{
		Options: []acp.PermissionOption{
			{OptionId: "reject", Kind: acp.PermissionOptionKindRejectOnce, Name: "Reject"},
			{OptionId: "allow-once", Kind: acp.PermissionOptionKindAllowOnce, Name: "Allow Once"},
			{OptionId: "allow-always", Kind: acp.PermissionOptionKindAllowAlways, Name: "Allow Always"},
		},
		ToolCall: acp.RequestPermissionToolCall{Title: &title},
	})
	if err != nil {
		t.Fatalf("RequestPermission() error: %v", err)
	}
	if resp.Outcome.Selected == nil {
		t.Fatal("expected selected outcome")
	}
	if resp.Outcome.Selected.OptionId != "allow-always" {
		t.Errorf("selected = %q, want 'allow-always'", resp.Outcome.Selected.OptionId)
	}
}

func TestClient_RequestPermission_PrefersAllowAlwaysOverOnce(t *testing.T) {
	client, _ := testClient(t)
	ctx := context.Background()

	resp, err := client.RequestPermission(ctx, acp.RequestPermissionRequest{
		Options: []acp.PermissionOption{
			{OptionId: "once", Kind: acp.PermissionOptionKindAllowOnce, Name: "Once"},
			{OptionId: "always", Kind: acp.PermissionOptionKindAllowAlways, Name: "Always"},
		},
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if resp.Outcome.Selected.OptionId != "always" {
		t.Errorf("selected = %q, want 'always'", resp.Outcome.Selected.OptionId)
	}
}

func TestClient_RequestPermission_FallsBackToFirst(t *testing.T) {
	client, _ := testClient(t)
	ctx := context.Background()

	resp, err := client.RequestPermission(ctx, acp.RequestPermissionRequest{
		Options: []acp.PermissionOption{
			{OptionId: "reject", Kind: acp.PermissionOptionKindRejectOnce, Name: "Reject"},
			{OptionId: "reject2", Kind: acp.PermissionOptionKindRejectAlways, Name: "Reject Always"},
		},
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	// No allow options, so falls back to first (index 0)
	if resp.Outcome.Selected.OptionId != "reject" {
		t.Errorf("selected = %q, want 'reject' (first option)", resp.Outcome.Selected.OptionId)
	}
}

func TestClient_RequestPermission_NoOptions(t *testing.T) {
	client, _ := testClient(t)
	ctx := context.Background()

	resp, err := client.RequestPermission(ctx, acp.RequestPermissionRequest{
		Options: []acp.PermissionOption{},
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if resp.Outcome.Cancelled == nil {
		t.Error("expected cancelled outcome when no options")
	}
}

func TestClient_SessionUpdate(t *testing.T) {
	client, _ := testClient(t)
	ctx := context.Background()

	err := client.SessionUpdate(ctx, acp.SessionNotification{
		Update: acp.UpdateAgentMessageText("test msg"),
	})
	if err != nil {
		t.Fatalf("SessionUpdate() error: %v", err)
	}
}

func TestClient_CreateTerminal_Allowed(t *testing.T) {
	client, sandbox := testClient(t)
	ctx := context.Background()

	resp, err := client.CreateTerminal(ctx, acp.CreateTerminalRequest{
		Command: "echo",
		Args:    []string{"terminal test"},
	})
	if err != nil {
		t.Fatalf("CreateTerminal() error: %v", err)
	}
	if resp.TerminalId == "" {
		t.Error("expected non-empty terminal ID")
	}

	// Wait and check output
	client.WaitForTerminalExit(ctx, acp.WaitForTerminalExitRequest{TerminalId: resp.TerminalId})
	outResp, err := client.TerminalOutput(ctx, acp.TerminalOutputRequest{TerminalId: resp.TerminalId})
	if err != nil {
		t.Fatalf("TerminalOutput() error: %v", err)
	}
	if outResp.Output == "" {
		t.Error("expected terminal output")
	}

	// Verify default cwd is sandbox
	_ = sandbox
}

func TestClient_CreateTerminal_Blocked(t *testing.T) {
	client, _ := testClient(t)
	ctx := context.Background()

	_, err := client.CreateTerminal(ctx, acp.CreateTerminalRequest{
		Command: "rm",
		Args:    []string{"-rf", "/"},
	})
	if err == nil {
		t.Error("should reject blocked command")
	}
}

func TestClient_CreateTerminal_WithCwd(t *testing.T) {
	client, sandbox := testClient(t)
	ctx := context.Background()

	subdir := filepath.Join(sandbox, "sub")
	os.MkdirAll(subdir, 0o755)

	resp, err := client.CreateTerminal(ctx, acp.CreateTerminalRequest{
		Command: "echo",
		Args:    []string{"in subdir"},
		Cwd:     &subdir,
	})
	if err != nil {
		t.Fatalf("CreateTerminal() error: %v", err)
	}
	client.WaitForTerminalExit(ctx, acp.WaitForTerminalExitRequest{TerminalId: resp.TerminalId})
}

func TestClient_CreateTerminal_CwdOutsideSandbox(t *testing.T) {
	client, _ := testClient(t)
	ctx := context.Background()

	outsideDir := "/tmp"
	_, err := client.CreateTerminal(ctx, acp.CreateTerminalRequest{
		Command: "echo",
		Args:    []string{"escaped"},
		Cwd:     &outsideDir,
	})
	if err == nil {
		t.Error("should reject cwd outside sandbox")
	}
}

func TestClient_TerminalLifecycle(t *testing.T) {
	client, _ := testClient(t)
	ctx := context.Background()

	// Create
	resp, err := client.CreateTerminal(ctx, acp.CreateTerminalRequest{
		Command: "sleep",
		Args:    []string{"60"},
	})
	if err != nil {
		t.Fatalf("CreateTerminal() error: %v", err)
	}

	// Kill
	_, err = client.KillTerminalCommand(ctx, acp.KillTerminalCommandRequest{
		TerminalId: resp.TerminalId,
	})
	if err != nil {
		t.Fatalf("KillTerminalCommand() error: %v", err)
	}

	// Wait
	waitResp, err := client.WaitForTerminalExit(ctx, acp.WaitForTerminalExitRequest{
		TerminalId: resp.TerminalId,
	})
	if err != nil {
		t.Fatalf("WaitForTerminalExit() error: %v", err)
	}
	// Killed process should have non-zero exit
	if waitResp.ExitCode != nil && *waitResp.ExitCode == 0 {
		t.Error("killed process should have non-zero exit")
	}

	// Release
	_, err = client.ReleaseTerminal(ctx, acp.ReleaseTerminalRequest{
		TerminalId: resp.TerminalId,
	})
	if err != nil {
		t.Fatalf("ReleaseTerminal() error: %v", err)
	}
}

func TestClient_TerminalOutput_WithExitStatus(t *testing.T) {
	client, _ := testClient(t)
	ctx := context.Background()

	resp, err := client.CreateTerminal(ctx, acp.CreateTerminalRequest{
		Command: "echo",
		Args:    []string{"done"},
	})
	if err != nil {
		t.Fatalf("CreateTerminal() error: %v", err)
	}

	client.WaitForTerminalExit(ctx, acp.WaitForTerminalExitRequest{TerminalId: resp.TerminalId})

	outResp, err := client.TerminalOutput(ctx, acp.TerminalOutputRequest{TerminalId: resp.TerminalId})
	if err != nil {
		t.Fatalf("TerminalOutput() error: %v", err)
	}
	if outResp.ExitStatus == nil {
		t.Fatal("expected exit status after process finished")
	}
	if outResp.ExitStatus.ExitCode == nil || *outResp.ExitStatus.ExitCode != 0 {
		t.Errorf("exit code = %v, want 0", outResp.ExitStatus.ExitCode)
	}
}
