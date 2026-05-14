package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"tilde prefix", "~/foo/bar", filepath.Join(home, "foo/bar")},
		{"absolute path", "/usr/local/bin", "/usr/local/bin"},
		{"relative path", "relative/path", "relative/path"},
		{"tilde only slash", "~/", home},
		{"tilde without slash", "~notapath", "~notapath"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandPath(tt.in)
			if got != tt.want {
				t.Errorf("expandPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestPollDuration(t *testing.T) {
	tests := []struct {
		name     string
		interval string
		want     time.Duration
	}{
		{"valid 5m", "5m", 5 * time.Minute},
		{"valid 30s", "30s", 30 * time.Second},
		{"valid 1h", "1h", time.Hour},
		{"invalid falls back to 5m", "notaduration", 5 * time.Minute},
		{"empty falls back to 5m", "", 5 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := GeneralConfig{PollInterval: tt.interval}
			got := g.PollDuration()
			if got != tt.want {
				t.Errorf("PollDuration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRepoConfigFullName(t *testing.T) {
	r := RepoConfig{Owner: "myorg", Name: "myrepo"}
	if got := r.FullName(); got != "myorg/myrepo" {
		t.Errorf("FullName() = %q, want %q", got, "myorg/myrepo")
	}
}

func TestRepoConfigDisplayLabel(t *testing.T) {
	t.Run("falls back to FullName when DisplayName is empty", func(t *testing.T) {
		r := RepoConfig{Owner: "myorg", Name: "myrepo"}
		if got := r.DisplayLabel(); got != "myorg/myrepo" {
			t.Errorf("DisplayLabel() = %q, want %q", got, "myorg/myrepo")
		}
	})
	t.Run("returns DisplayName when set", func(t *testing.T) {
		r := RepoConfig{Owner: "kubernetes-sigs", Name: "mcp-lifecycle-operator", DisplayName: "k-sigs/mcp-lc-op"}
		if got := r.DisplayLabel(); got != "k-sigs/mcp-lc-op" {
			t.Errorf("DisplayLabel() = %q, want %q", got, "k-sigs/mcp-lc-op")
		}
	})
}

func TestValidate_NoRepos(t *testing.T) {
	cfg := Config{
		Agents: map[string]AgentDef{"claude": {Cmd: "claude"}},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty repos")
	}
	if got := err.Error(); got != "config: at least one [[repos]] entry is required" {
		t.Errorf("unexpected error: %s", got)
	}
}

func TestValidate_RepoMissingOwner(t *testing.T) {
	cfg := Config{
		Repos:  []RepoConfig{{Name: "repo", Path: "/path"}},
		Agents: map[string]AgentDef{"claude": {Cmd: "claude"}},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing owner")
	}
}

func TestValidate_RepoMissingPath(t *testing.T) {
	cfg := Config{
		Repos:  []RepoConfig{{Owner: "org", Name: "repo"}},
		Agents: map[string]AgentDef{"claude": {Cmd: "claude"}},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestValidate_NoAgents(t *testing.T) {
	cfg := Config{
		Repos: []RepoConfig{{Owner: "org", Name: "repo", Path: "/p"}},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for no agents")
	}
}

func TestValidate_AgentMissingCmd(t *testing.T) {
	cfg := Config{
		Repos:  []RepoConfig{{Owner: "org", Name: "repo", Path: "/p"}},
		Agents: map[string]AgentDef{"claude": {}},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for agent missing cmd")
	}
}

func TestValidate_DefaultAgentNotInMap(t *testing.T) {
	cfg := Config{
		Repos:    []RepoConfig{{Owner: "org", Name: "repo", Path: "/p"}},
		Agents:   map[string]AgentDef{"claude": {Cmd: "claude"}},
		Defaults: DefaultsConfig{Agent: "gemini"},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for default agent not in agents map")
	}
}

func TestValidate_Valid(t *testing.T) {
	cfg := Config{
		Repos:    []RepoConfig{{Owner: "org", Name: "repo", Path: "/p"}},
		Agents:   map[string]AgentDef{"claude": {Cmd: "claude"}},
		Defaults: DefaultsConfig{Agent: "claude"},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestValidate_ValidNoDefault(t *testing.T) {
	cfg := Config{
		Repos:  []RepoConfig{{Owner: "org", Name: "repo", Path: "/p"}},
		Agents: map[string]AgentDef{"claude": {Cmd: "claude"}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestAgentForRepo(t *testing.T) {
	cfg := Config{
		Defaults: DefaultsConfig{Agent: "claude"},
		Agents: map[string]AgentDef{
			"claude": {Cmd: "claude"},
			"gemini": {Cmd: "gemini"},
		},
	}

	t.Run("repo override wins", func(t *testing.T) {
		repo := RepoConfig{DefaultAgent: "gemini"}
		got := cfg.AgentForRepo(repo)
		if got != "gemini" {
			t.Errorf("AgentForRepo() = %q, want %q", got, "gemini")
		}
	})

	t.Run("falls back to config default", func(t *testing.T) {
		repo := RepoConfig{}
		got := cfg.AgentForRepo(repo)
		if got != "claude" {
			t.Errorf("AgentForRepo() = %q, want %q", got, "claude")
		}
	})

	t.Run("falls back to first agent when no defaults", func(t *testing.T) {
		cfgNoDefault := Config{
			Agents: map[string]AgentDef{
				"only": {Cmd: "only"},
			},
		}
		repo := RepoConfig{}
		got := cfgNoDefault.AgentForRepo(repo)
		if got != "only" {
			t.Errorf("AgentForRepo() = %q, want %q", got, "only")
		}
	})

	t.Run("empty agents returns empty string", func(t *testing.T) {
		cfgEmpty := Config{}
		repo := RepoConfig{}
		got := cfgEmpty.AgentForRepo(repo)
		if got != "" {
			t.Errorf("AgentForRepo() = %q, want empty", got)
		}
	})
}

func TestSessionsDir(t *testing.T) {
	cfg := Config{General: GeneralConfig{DataDir: "/tmp/testdata"}}
	got := cfg.SessionsDir()
	want := "/tmp/testdata/sessions"
	if got != want {
		t.Errorf("SessionsDir() = %q, want %q", got, want)
	}
}

func TestLoad_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[general]
data_dir = "/tmp/data"
poll_interval = "10m"

[defaults]
agent = "claude"

[[repos]]
owner = "myorg"
name = "myrepo"
path = "/home/user/repos/myrepo"
labels = ["bug", "enhancement"]
display_name = "myorg/short"

[agents.claude]
cmd = "claude"
args = ["--acp"]
resume_cmd = "claude"
resume_args = ["--resume", "{session_id}"]

[safety]
max_file_write_bytes = 2097152

  [[safety.allowed_commands]]
  cmd = "go"
  allow_all = true

  [[safety.allowed_commands]]
  cmd = "git"
  args_prefix = ["status"]
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.General.DataDir != "/tmp/data" {
		t.Errorf("DataDir = %q, want /tmp/data", cfg.General.DataDir)
	}
	if cfg.General.PollDuration() != 10*time.Minute {
		t.Errorf("PollDuration = %v, want 10m", cfg.General.PollDuration())
	}
	if len(cfg.Repos) != 1 {
		t.Fatalf("len(Repos) = %d, want 1", len(cfg.Repos))
	}
	if cfg.Repos[0].Owner != "myorg" {
		t.Errorf("Repos[0].Owner = %q", cfg.Repos[0].Owner)
	}
	if len(cfg.Repos[0].Labels) != 2 {
		t.Errorf("Repos[0].Labels = %v, want [bug enhancement]", cfg.Repos[0].Labels)
	}
	if cfg.Repos[0].DisplayName != "myorg/short" {
		t.Errorf("Repos[0].DisplayName = %q, want %q", cfg.Repos[0].DisplayName, "myorg/short")
	}
	if cfg.Defaults.Agent != "claude" {
		t.Errorf("Defaults.Agent = %q", cfg.Defaults.Agent)
	}
	if a, ok := cfg.Agents["claude"]; !ok {
		t.Fatal("agent 'claude' not found")
	} else {
		if a.Cmd != "claude" {
			t.Errorf("Agent cmd = %q", a.Cmd)
		}
		if len(a.Args) != 1 || a.Args[0] != "--acp" {
			t.Errorf("Agent args = %v", a.Args)
		}
	}
	if cfg.Safety.MaxFileWriteBytes != 2097152 {
		t.Errorf("MaxFileWriteBytes = %d", cfg.Safety.MaxFileWriteBytes)
	}
	if len(cfg.Safety.AllowedCommands) != 2 {
		t.Errorf("AllowedCommands = %d, want 2", len(cfg.Safety.AllowedCommands))
	}
}

func TestLoad_TildeExpansion(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[general]
data_dir = "~/mydata"

[[repos]]
owner = "org"
name = "repo"
path = "~/repos/myrepo"

[agents.a]
cmd = "agent"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.General.DataDir != filepath.Join(home, "mydata") {
		t.Errorf("DataDir = %q, want %q", cfg.General.DataDir, filepath.Join(home, "mydata"))
	}
	if cfg.Repos[0].Path != filepath.Join(home, "repos/myrepo") {
		t.Errorf("Repos[0].Path = %q", cfg.Repos[0].Path)
	}
}

func TestLoad_InvalidTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("not valid [[[toml"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid TOML")
	}
}

func TestLoad_NonexistentFile(t *testing.T) {
	_, err := Load("/nonexistent/path/config.toml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoad_FailsValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	// Valid TOML but missing required fields
	content := `
[general]
data_dir = "/tmp"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestFindConfigPath_XDGConfig(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "baton")
	os.MkdirAll(configDir, 0o755)
	configPath := filepath.Join(configDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	origXDG := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", dir)
	defer os.Setenv("XDG_CONFIG_HOME", origXDG)

	got := FindConfigPath()
	if got != configPath {
		t.Errorf("FindConfigPath() = %q, want %q", got, configPath)
	}
}

func TestFindConfigPath_NotFound(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	// Unset XDG so it doesn't accidentally find something
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "nonexistent"))
	defer os.Setenv("XDG_CONFIG_HOME", origXDG)

	got := FindConfigPath()
	if got != "" {
		t.Errorf("FindConfigPath() = %q, want empty", got)
	}
}

func TestDefaultSafetyConfig(t *testing.T) {
	sc := DefaultSafetyConfig()
	if sc.MaxFileWriteBytes != 1<<20 {
		t.Errorf("MaxFileWriteBytes = %d, want %d", sc.MaxFileWriteBytes, 1<<20)
	}
	if len(sc.AllowedCommands) == 0 {
		t.Fatal("expected default allowed commands")
	}

	// Check a few specific entries
	foundGo := false
	foundGitDiff := false
	for _, ac := range sc.AllowedCommands {
		if ac.Cmd == "go" && ac.AllowAll {
			foundGo = true
		}
		if ac.Cmd == "git" && len(ac.ArgsPrefix) == 1 && ac.ArgsPrefix[0] == "diff" {
			foundGitDiff = true
		}
	}
	if !foundGo {
		t.Error("default safety config missing 'go' with allow_all")
	}
	if !foundGitDiff {
		t.Error("default safety config missing 'git diff'")
	}
}
