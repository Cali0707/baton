package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	General  GeneralConfig        `toml:"general"`
	Defaults DefaultsConfig       `toml:"defaults"`
	Repos    []RepoConfig         `toml:"repos"`
	Agents   map[string]AgentDef  `toml:"agents"`
	Safety   SafetyConfig         `toml:"safety"`
}

type GeneralConfig struct {
	DataDir      string `toml:"data_dir"`
	PollInterval string `toml:"poll_interval"`
}

func (g GeneralConfig) PollDuration() time.Duration {
	d, err := time.ParseDuration(g.PollInterval)
	if err != nil {
		return 5 * time.Minute
	}
	return d
}

type DefaultsConfig struct {
	Agent string `toml:"agent"`
}

type RepoConfig struct {
	Owner        string   `toml:"owner"`
	Name         string   `toml:"name"`
	Path         string   `toml:"path"`
	DefaultAgent string   `toml:"default_agent"`
	Labels       []string `toml:"labels"`
}

func (r RepoConfig) FullName() string {
	return r.Owner + "/" + r.Name
}

type AgentDef struct {
	Cmd        string   `toml:"cmd"`
	Args       []string `toml:"args"`
	ResumeCmd  string   `toml:"resume_cmd"`
	ResumeArgs []string `toml:"resume_args"`
}

// BuildResumeCmd returns the full resume command with {session_id} replaced.
func (a AgentDef) BuildResumeCmd(sessionID string) string {
	parts := []string{a.ResumeCmd}
	for _, arg := range a.ResumeArgs {
		parts = append(parts, strings.ReplaceAll(arg, "{session_id}", sessionID))
	}
	return strings.Join(parts, " ")
}

type SafetyConfig struct {
	AllowedCommands   []AllowedCommand `toml:"allowed_commands"`
	MaxFileWriteBytes int64            `toml:"max_file_write_bytes"`
}

type AllowedCommand struct {
	Cmd       string   `toml:"cmd"`
	ArgsPrefix []string `toml:"args_prefix"`
	AllowAll  bool     `toml:"allow_all"`
}

func expandPath(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return filepath.Join(home, p[2:])
	}
	return p
}

func Load(path string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("loading config %s: %w", path, err)
	}

	if cfg.General.DataDir == "" {
		cfg.General.DataDir = DefaultDataDir()
	}
	if cfg.General.DataDir == "" {
		return nil, fmt.Errorf("could not determine data directory: set data_dir in config or ensure $HOME is set")
	}
	cfg.General.DataDir = expandPath(cfg.General.DataDir)

	for i := range cfg.Repos {
		cfg.Repos[i].Path = expandPath(cfg.Repos[i].Path)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) Validate() error {
	if len(c.Repos) == 0 {
		return fmt.Errorf("config: at least one [[repos]] entry is required")
	}
	for i, r := range c.Repos {
		if r.Owner == "" || r.Name == "" {
			return fmt.Errorf("config: repos[%d]: owner and name are required", i)
		}
		if r.Path == "" {
			return fmt.Errorf("config: repos[%d] (%s): path is required", i, r.FullName())
		}
	}
	if c.Defaults.Agent != "" {
		if _, ok := c.Agents[c.Defaults.Agent]; !ok {
			return fmt.Errorf("config: default agent %q not found in [agents]", c.Defaults.Agent)
		}
	}
	if len(c.Agents) == 0 {
		return fmt.Errorf("config: at least one agent must be configured in [agents]")
	}
	for name, a := range c.Agents {
		if a.Cmd == "" {
			return fmt.Errorf("config: agents.%s: cmd is required", name)
		}
	}
	return nil
}

func (c *Config) AgentForRepo(repo RepoConfig) string {
	if repo.DefaultAgent != "" {
		return repo.DefaultAgent
	}
	if c.Defaults.Agent != "" {
		return c.Defaults.Agent
	}
	// Return the first agent name
	for name := range c.Agents {
		return name
	}
	return ""
}

func (c *Config) SessionsDir() string {
	return filepath.Join(c.General.DataDir, "sessions")
}

// DefaultDataDir returns the default data directory for baton,
// following XDG conventions (~/.local/share/baton).
func DefaultDataDir() string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "baton")
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		return filepath.Join(home, ".local", "share", "baton")
	}
	return ""
}

func DefaultConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "baton")
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		return filepath.Join(home, ".config", "baton")
	}
	return ""
}

func FindConfigPath() string {
	if dir := DefaultConfigDir(); dir != "" {
		p := filepath.Join(dir, "config.toml")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
