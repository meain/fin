package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/meain/fin/internal/agent"
	"github.com/meain/fin/internal/config"
	"github.com/meain/fin/internal/fsutil"
	"github.com/meain/fin/internal/prompt"
	"github.com/meain/fin/internal/render"
	"github.com/meain/fin/internal/skill"
	"github.com/meain/fin/internal/tool"
)

// printDoctor prints a diagnostic summary of fin's current configuration:
// config file, models, providers, model aliases, tools, builtin skills,
// discovered skills, AGENTS.md files, and session storage.
func printDoctor(cfg *config.Config) {
	header := func(s string) {
		fmt.Printf("%s%s%s\n", render.Bold, s, render.Reset)
	}
	row := func(label, value string) {
		fmt.Printf("  %-22s %s\n", label, value)
	}
	dim := func(s string) string { return render.Dim + s + render.Reset }
	ok := func(s string) string { return render.Green + s + render.Reset }
	warn := func(s string) string { return render.Yellow + s + render.Reset }
	// truncate to first line, then cap at maxLen visible chars
	shortDesc := func(s string, maxLen int) string {
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			s = s[:i]
		}
		s = strings.TrimSpace(s)
		// Truncate by rune, not byte, to avoid cutting multi-byte UTF-8 mid-rune.
		if runes := []rune(s); len(runes) > maxLen {
			s = string(runes[:maxLen-1]) + "…"
		}
		return s
	}

	// ── Config ────────────────────────────────────────────────────────────
	header("Config")
	cfgPath := config.ConfigPath()
	if _, err := os.Stat(cfgPath); err == nil {
		row("path", cfgPath)
	} else {
		row("path", cfgPath+" "+warn("[not found]"))
	}
	fmt.Println()

	// ── Models ────────────────────────────────────────────────────────────
	header("Models")
	formatModel := func(label, raw string) {
		if raw == "" {
			row(label, dim("(not set)"))
			return
		}
		prov, mdl := config.ResolveModel(raw, cfg)
		resolved := raw
		if prov != "" {
			resolved = prov + "/" + mdl
		}
		if resolved != raw {
			row(label, raw+" "+dim("→ "+resolved))
		} else {
			row(label, resolved)
		}
	}
	formatModel("primary", cfg.Models.Primary)
	formatModel("secondary", cfg.Models.Secondary)
	fmt.Println()

	// ── Providers ─────────────────────────────────────────────────────────
	header("Providers")
	provNames := make([]string, 0, len(cfg.Providers))
	for n := range cfg.Providers {
		provNames = append(provNames, n)
	}
	sort.Strings(provNames)
	for _, n := range provNames {
		p := cfg.Providers[n]
		keyStatus := warn("[key not set]")
		if p.APIKeyEnv != "" {
			if v := os.Getenv(p.APIKeyEnv); v != "" {
				keyStatus = ok("[key set]")
			}
		}
		row(n, fmt.Sprintf("%-40s %-24s %s", p.BaseURL, dim(p.APIKeyEnv), keyStatus))
	}
	fmt.Println()

	// ── Model Aliases ─────────────────────────────────────────────────────
	if len(cfg.ModelAliases) > 0 {
		header("Model Aliases")
		aliasNames := make([]string, 0, len(cfg.ModelAliases))
		for k := range cfg.ModelAliases {
			aliasNames = append(aliasNames, k)
		}
		sort.Strings(aliasNames)
		for _, k := range aliasNames {
			row(k, dim("→")+" "+cfg.ModelAliases[k])
		}
		fmt.Println()
	}

	// ── Tools ─────────────────────────────────────────────────────────────
	tools := tool.BuiltinTools()
	tools = append(tools, &tool.SkillTool{})    // include use_skill
	tools = append(tools, &tool.SubagentTool{}) // include subagent
	header(fmt.Sprintf("Tools (%d)", len(tools)))
	for _, tl := range tools {
		approvalMode := "auto"
		if tc, ok := cfg.Tools[tl.Name()]; ok && tc.Approval != "" {
			approvalMode = tc.Approval
		}
		// Pad plain text first so ANSI codes don't skew column width.
		padded := fmt.Sprintf("%-9s", approvalMode)
		var approvalColor string
		switch approvalMode {
		case "confirm":
			approvalColor = warn(padded)
		case "deny":
			approvalColor = render.Red + padded + render.Reset
		default:
			approvalColor = dim(padded)
		}
		row(tl.Name(), approvalColor+shortDesc(tl.Description(), 70))
	}
	fmt.Println()

	// ── Builtin Skills ────────────────────────────────────────────────────
	builtins := agent.BuiltinSkillEntries()
	header(fmt.Sprintf("Builtin Skills (%d)", len(builtins)))
	for _, s := range builtins {
		row(s.Name, shortDesc(s.Description, 80))
	}
	fmt.Println()

	// ── Discovered Skills ─────────────────────────────────────────────────
	discovered := skill.Discover(cfg, false)
	header(fmt.Sprintf("Discovered Skills (%d)", len(discovered)))
	if len(discovered) == 0 {
		fmt.Printf("  %s\n", dim("(none)"))
	} else {
		// Group by parent directory (the skills/ dir), preserving discovery order.
		type skillGroup struct {
			dir    string
			skills []*skill.Skill
		}
		var groups []skillGroup
		dirIndex := map[string]int{}
		home := config.HomeDir()
		for _, s := range discovered {
			parent := filepath.Dir(s.Dir)
			if i, ok := dirIndex[parent]; ok {
				groups[i].skills = append(groups[i].skills, s)
			} else {
				dirIndex[parent] = len(groups)
				groups = append(groups, skillGroup{dir: parent, skills: []*skill.Skill{s}})
			}
		}
		for _, g := range groups {
			displayDir := g.dir
			if home != "" {
				displayDir = strings.Replace(displayDir, home, "~", 1)
			}
			fmt.Printf("  %s\n", dim(displayDir))
			for _, s := range g.skills {
				fmt.Printf("    %-20s %s\n", s.Name, shortDesc(s.Description, 70))
			}
			fmt.Println()
		}
	}

	// ── AGENTS.md Files ───────────────────────────────────────────────────
	header("AGENTS.md Files")
	showAgentsMD := func(label, path string) {
		data, err := os.ReadFile(path)
		if err != nil {
			row(label, fmt.Sprintf("%-50s %s", path, warn("[not found]")))
		} else {
			row(label, fmt.Sprintf("%-50s %s", path, ok(fmt.Sprintf("[%d chars]", len(data)))))
		}
	}
	globalPath := filepath.Join(config.HomeDir(), config.AgentsDir, config.AgentsFile)
	showAgentsMD("global", globalPath)
	projectFile := cfg.Settings.ProjectFile
	if projectFile == "" {
		projectFile = config.AgentsFile
	}
	var foundPaths []string
	fsutil.WalkUpFromCwd(func(dir string) {
		p := filepath.Join(dir, projectFile)
		if _, err := os.Stat(p); err == nil {
			foundPaths = append(foundPaths, p)
		}
	})
	if len(foundPaths) == 0 {
		row("project", warn("[not found]"))
	} else {
		for i, p := range foundPaths {
			label := "project"
			if i > 0 {
				label = ""
			}
			showAgentsMD(label, p)
		}
	}
	fmt.Println()

	// ── Claude Code Auto-Memory ──────────────────────────────────────────
	header("Claude Code Auto-Memory")
	if cfg.Settings.DisableClaudeMemory {
		row("status", dim("disabled (disable_claude_memory = true)"))
	} else {
		memoryPath := prompt.ClaudeMemoryPath()
		if memoryPath == "" {
			row("status", warn("[could not resolve project path]"))
		} else if data, err := os.ReadFile(memoryPath); err == nil {
			row("path", fmt.Sprintf("%-50s %s", memoryPath, ok(fmt.Sprintf("[%d bytes]", len(data)))))
		} else {
			row("path", fmt.Sprintf("%-50s %s", memoryPath, dim("[not found]")))
		}
	}
	fmt.Println()

	// ── Sessions ──────────────────────────────────────────────────────────
	header("Sessions")
	sessPath := config.SessionPath()
	row("path", sessPath)
	entries, err := os.ReadDir(sessPath)
	if err == nil {
		count := 0
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
				count++
			}
		}
		row("count", fmt.Sprintf("%d", count))
	} else {
		row("count", warn("[unreadable]"))
	}
}
