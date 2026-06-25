// Package config loads, validates, and exposes fin's TOML configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config is the top-level TOML configuration.
type Config struct {
	Models       ModelsConfig              `toml:"models"`
	Settings     Settings                  `toml:"settings"`
	ModelAliases map[string]string         `toml:"model_aliases"`
	Providers    map[string]ProviderConfig `toml:"providers"`
	Tools        map[string]ToolConfig     `toml:"tools"`
}

// Settings holds non-model knobs.
type Settings struct {
	ProjectFile string          `toml:"project_file"`
	MaxTurns    int             `toml:"max_turns"`
	Approve     string          `toml:"approve"` // "all", "safe", "none"
	UI          string          `toml:"ui"`      // "default", "quiet"
	Matching    MatchingConfig  `toml:"matching"`
}

// MatchingConfig holds tuning constants for session matching.
type MatchingConfig struct {
	TitleWeight     float64 `toml:"title_weight"`     // weight applied to title hits (default 3)
	ContentCap      int     `toml:"content_cap"`      // cap on content hits per keyword (default 5)
	RecencyDecayDay int     `toml:"recency_decay_d"`  // decay window in days (default 7)
	RecencyBonus    float64 `toml:"recency_bonus"`    // max recency multiplier (default 0.5)
}

// ModelsConfig holds the conversation and secondary model identifiers.
type ModelsConfig struct {
	Primary   string `toml:"primary"`
	Secondary string `toml:"secondary"`
}

// ProviderConfig describes how to talk to an LLM provider.
type ProviderConfig struct {
	BaseURL   string            `toml:"base_url"`
	APIKeyEnv string            `toml:"api_key_env"`
	Headers   map[string]string `toml:"headers"`
}

// ToolConfig configures a single tool's approval policy and shell allow/deny lists.
type ToolConfig struct {
	Approval       string   `toml:"approval"`         // "auto", "confirm", "deny"
	Allow          []string `toml:"allow"`             // glob patterns (shell tool)
	Deny           []string `toml:"deny"`              // glob patterns (shell tool)
	MaxOutputBytes int      `toml:"max_output_bytes"`  // 0 = use default (40000 bytes)
}

// Default returns the built-in defaults used when no config file exists.
func Default() Config {
	return Config{
		Models: ModelsConfig{
			Primary: "anthropic/claude-sonnet-4-20250514",
		},
		Settings: Settings{
			ProjectFile: AgentsFile,
			MaxTurns:    50,
			Matching: MatchingConfig{
				TitleWeight:     3,
				ContentCap:      5,
				RecencyDecayDay: 7,
				RecencyBonus:    0.5,
			},
		},
		ModelAliases: map[string]string{},
		Providers: map[string]ProviderConfig{
			"anthropic": {
				BaseURL:   "https://api.anthropic.com",
				APIKeyEnv: "ANTHROPIC_API_KEY",
			},
			"openai": {
				BaseURL:   "https://api.openai.com",
				APIKeyEnv: "OPENAI_API_KEY",
			},
		},
		Tools: map[string]ToolConfig{
			"read":      {Approval: "auto"},
			"write":     {Approval: "confirm"},
			"edit":      {Approval: "confirm"},
			"shell":     {Approval: "confirm"},
			"use_skill": {Approval: "auto"},
			"subagent":  {Approval: "auto"},
			"compact":   {Approval: "auto"},
		},
	}
}

// Load reads the config TOML at path (or the default path if empty). When the
// file does not exist, defaults are written to disk and returned.
func Load(path string) (*Config, error) {
	if path == "" {
		path = ConfigPath()
	}

	cfg := Default()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, err
		}
		f, err := os.Create(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		if err := toml.NewEncoder(f).Encode(cfg); err != nil {
			return nil, err
		}
		return &cfg, nil
	}

	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	applyMatchingDefaults(&cfg.Settings.Matching)
	return &cfg, nil
}

func validate(c *Config) error {
	for alias := range c.ModelAliases {
		visited := map[string]bool{}
		cur := alias
		for {
			visited[cur] = true
			next, ok := c.ModelAliases[cur]
			if !ok {
				break
			}
			if visited[next] {
				return fmt.Errorf("circular model alias: %s", alias)
			}
			cur = next
		}
	}

	for name, p := range c.Providers {
		if p.BaseURL != "" && !strings.HasPrefix(p.BaseURL, "http://") && !strings.HasPrefix(p.BaseURL, "https://") {
			return fmt.Errorf("provider %q: base_url must start with http:// or https://", name)
		}
	}

	for name, tc := range c.Tools {
		switch tc.Approval {
		case "auto", "confirm", "deny", "":
		default:
			return fmt.Errorf("tool %q: invalid approval %q", name, tc.Approval)
		}
	}

	return nil
}

// applyMatchingDefaults fills in zero-valued fields with their defaults so
// a partial [settings.matching] block in user TOML doesn't disable matching.
func applyMatchingDefaults(m *MatchingConfig) {
	if m.TitleWeight == 0 {
		m.TitleWeight = 3
	}
	if m.ContentCap == 0 {
		m.ContentCap = 5
	}
	if m.RecencyDecayDay == 0 {
		m.RecencyDecayDay = 7
	}
	if m.RecencyBonus == 0 {
		m.RecencyBonus = 0.5
	}
}
