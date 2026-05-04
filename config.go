package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const defaultConfigPath = "~/.config/fin/config.toml"

type Config struct {
	Settings     Settings                  `toml:"settings"`
	ModelAliases map[string]string         `toml:"model_aliases"`
	Providers    map[string]ProviderConfig `toml:"providers"`
	Tools        map[string]ToolConfig     `toml:"tools"`
}

type Settings struct {
	DefaultModel string `toml:"default_model"`
	ProjectFile  string `toml:"project_file"`
	MaxTurns     int    `toml:"max_turns"`
	Yolo         bool   `toml:"yolo"`
	UI           string `toml:"ui"` // "default", "minimal", "quiet"
}

type ProviderConfig struct {
	BaseURL    string            `toml:"base_url"`
	APIKeyEnv  string            `toml:"api_key_env"`
	Headers    map[string]string `toml:"headers"`
}

type ToolConfig struct {
	Approval string   `toml:"approval"` // "auto", "confirm", "deny"
	Allow    []string `toml:"allow"`    // glob patterns (shell tool)
	Deny     []string `toml:"deny"`     // glob patterns (shell tool)
}

func defaultConfig() Config {
	return Config{
		Settings: Settings{
			DefaultModel: "anthropic/claude-sonnet-4-20250514",
			ProjectFile:  "AGENTS.md",
			MaxTurns:     50,
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

func loadConfig(path string) (*Config, error) {
	if path == "" {
		path = defaultConfigPath
	}
	path = expandHome(path)

	config := defaultConfig()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		// No config file — ensure dir exists and write defaults
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, err
		}
		f, err := os.Create(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		if err := toml.NewEncoder(f).Encode(config); err != nil {
			return nil, err
		}
		return &config, nil
	}

	if _, err := toml.DecodeFile(path, &config); err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	if err := validateConfig(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

func validateConfig(c *Config) error {
	// Check for circular aliases
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

	for name, t := range c.Tools {
		switch t.Approval {
		case "auto", "confirm", "deny", "":
		default:
			return fmt.Errorf("tool %q: invalid approval %q", name, t.Approval)
		}
	}

	return nil
}

// resolveModel resolves aliases and splits "provider/model" into parts.
func resolveModel(model string, c *Config) (providerName, modelName string) {
	// Resolve aliases
	for i := 0; i < 10; i++ { // cap depth
		if resolved, ok := c.ModelAliases[model]; ok {
			model = resolved
		} else {
			break
		}
	}

	parts := strings.SplitN(model, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", model
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
