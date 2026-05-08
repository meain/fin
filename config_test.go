package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveModel_ProviderSlashModel(t *testing.T) {
	c := &Config{ModelAliases: map[string]string{}}
	prov, model := resolveModel("anthropic/claude-sonnet", c)
	if prov != "anthropic" {
		t.Errorf("expected provider %q, got %q", "anthropic", prov)
	}
	if model != "claude-sonnet" {
		t.Errorf("expected model %q, got %q", "claude-sonnet", model)
	}
}

func TestResolveModel_AliasResolution(t *testing.T) {
	c := &Config{
		ModelAliases: map[string]string{
			"sonnet": "anthropic/claude-sonnet-4-20250514",
		},
	}
	prov, model := resolveModel("sonnet", c)
	if prov != "anthropic" {
		t.Errorf("expected provider %q, got %q", "anthropic", prov)
	}
	if model != "claude-sonnet-4-20250514" {
		t.Errorf("expected model %q, got %q", "claude-sonnet-4-20250514", model)
	}
}

func TestResolveModel_ChainedAlias(t *testing.T) {
	c := &Config{
		ModelAliases: map[string]string{
			"default": "sonnet",
			"sonnet":  "anthropic/claude-sonnet-4-20250514",
		},
	}
	prov, model := resolveModel("default", c)
	if prov != "anthropic" {
		t.Errorf("expected provider %q, got %q", "anthropic", prov)
	}
	if model != "claude-sonnet-4-20250514" {
		t.Errorf("expected model %q, got %q", "claude-sonnet-4-20250514", model)
	}
}

func TestResolveModel_BareModel(t *testing.T) {
	c := &Config{ModelAliases: map[string]string{}}
	prov, model := resolveModel("gpt-4o", c)
	if prov != "" {
		t.Errorf("expected empty provider, got %q", prov)
	}
	if model != "gpt-4o" {
		t.Errorf("expected model %q, got %q", "gpt-4o", model)
	}
}

func TestValidateConfig_CircularAlias(t *testing.T) {
	c := &Config{
		ModelAliases: map[string]string{
			"a": "b",
			"b": "a",
		},
		Providers: map[string]ProviderConfig{},
		Tools:     map[string]ToolConfig{},
	}
	err := validateConfig(c)
	if err == nil {
		t.Fatal("expected error for circular alias, got nil")
	}
	if !strings.Contains(err.Error(), "circular model alias") {
		t.Errorf("expected circular alias error, got: %v", err)
	}
}

func TestValidateConfig_InvalidProviderURL(t *testing.T) {
	c := &Config{
		ModelAliases: map[string]string{},
		Providers: map[string]ProviderConfig{
			"bad": {BaseURL: "ftp://example.com"},
		},
		Tools: map[string]ToolConfig{},
	}
	err := validateConfig(c)
	if err == nil {
		t.Fatal("expected error for invalid provider URL, got nil")
	}
	if !strings.Contains(err.Error(), "base_url must start with http://") {
		t.Errorf("expected base_url error, got: %v", err)
	}
}

func TestValidateConfig_InvalidToolApproval(t *testing.T) {
	c := &Config{
		ModelAliases: map[string]string{},
		Providers:    map[string]ProviderConfig{},
		Tools: map[string]ToolConfig{
			"shell": {Approval: "yolo"},
		},
	}
	err := validateConfig(c)
	if err == nil {
		t.Fatal("expected error for invalid approval level, got nil")
	}
	if !strings.Contains(err.Error(), "invalid approval") {
		t.Errorf("expected invalid approval error, got: %v", err)
	}
}

func TestValidateConfig_Valid(t *testing.T) {
	c := &Config{
		ModelAliases: map[string]string{"s": "anthropic/sonnet"},
		Providers: map[string]ProviderConfig{
			"anthropic": {BaseURL: "https://api.anthropic.com"},
		},
		Tools: map[string]ToolConfig{
			"read": {Approval: "auto"},
		},
	}
	if err := validateConfig(c); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestExpandHome_WithTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}
	result := expandHome("~/some/path")
	expected := filepath.Join(home, "some/path")
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestExpandHome_WithoutTilde(t *testing.T) {
	result := expandHome("/absolute/path")
	if result != "/absolute/path" {
		t.Errorf("expected %q, got %q", "/absolute/path", result)
	}
}

func TestLoadConfig_TempFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")

	content := `
[settings]
model = "openai/gpt-4o"
project_file = "AGENTS.md"
max_turns = 20
approve = "all"

[model_aliases]
fast = "openai/gpt-4o-mini"

[providers.openai]
base_url = "https://api.openai.com"
api_key_env = "OPENAI_API_KEY"

[tools.read]
approval = "auto"
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	cfg, err := loadConfig(configPath)
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}

	if cfg.Settings.DefaultModel != "openai/gpt-4o" {
		t.Errorf("expected model %q, got %q", "openai/gpt-4o", cfg.Settings.DefaultModel)
	}
	if cfg.Settings.MaxTurns != 20 {
		t.Errorf("expected max_turns 20, got %d", cfg.Settings.MaxTurns)
	}
	if cfg.Settings.Approve != "all" {
		t.Errorf("expected approve %q, got %q", "all", cfg.Settings.Approve)
	}
	if alias, ok := cfg.ModelAliases["fast"]; !ok || alias != "openai/gpt-4o-mini" {
		t.Errorf("expected alias fast=openai/gpt-4o-mini, got %q", alias)
	}
	if cfg.Providers["openai"].BaseURL != "https://api.openai.com" {
		t.Errorf("unexpected provider URL: %s", cfg.Providers["openai"].BaseURL)
	}
}

func TestLoadConfig_DefaultCreation(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "sub", "config.toml")

	cfg, err := loadConfig(configPath)
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}

	// Should have default values
	if cfg.Settings.DefaultModel != "anthropic/claude-sonnet-4-20250514" {
		t.Errorf("expected default model, got %q", cfg.Settings.DefaultModel)
	}

	// File should have been created
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("expected config file to be created")
	}
}
