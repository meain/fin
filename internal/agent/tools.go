package agent

import (
	"github.com/meain/fin/internal/config"
	finembed "github.com/meain/fin/internal/embed"
	"github.com/meain/fin/internal/skill"
	"github.com/meain/fin/internal/tool"
)

// BuildTools assembles the tool list for an agent. includeSubagent controls
// whether the subagent tool is registered (main agent: true; subagents
// themselves: false). enabled is the allow-list filter — nil keeps all,
// empty drops all.
func BuildTools(skills []*skill.Skill, enabled map[string]bool, includeSubagent bool) []tool.Tool {
	tools := tool.BuiltinTools()

	entries := loadBuiltinSkills()
	for _, s := range skills {
		entries = append(entries, tool.SkillEntry{
			Name:          s.Name,
			Description:   s.Description,
			Compatibility: s.Compatibility,
			Dir:           s.Dir,
		})
	}
	tools = append(tools, &tool.SkillTool{Skills: entries})

	if includeSubagent {
		tools = append(tools, &tool.SubagentTool{})
	}

	return filterTools(tools, enabled)
}

// filterTools applies the -tools allow-list. nil keeps all, empty drops all.
func filterTools(tools []tool.Tool, enabled map[string]bool) []tool.Tool {
	if enabled == nil {
		return tools
	}
	out := make([]tool.Tool, 0, len(tools))
	for _, tl := range tools {
		if enabled[tl.Name()] {
			out = append(out, tl)
		}
	}
	return out
}

// loadBuiltinSkills reads the embedded skills FS and returns one
// tool.SkillEntry per directory containing a valid SKILL.md.
func loadBuiltinSkills() []tool.SkillEntry {
	var entries []tool.SkillEntry

	dirs, err := finembed.BuiltinSkillsFS.ReadDir("skills")
	if err != nil {
		return entries
	}

	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		data, err := finembed.BuiltinSkillsFS.ReadFile("skills/" + d.Name() + "/" + config.SkillFile)
		if err != nil {
			continue
		}
		sk, err := skill.ParseMD(data)
		if err != nil {
			continue
		}
		entries = append(entries, tool.SkillEntry{
			Name:        sk.Name,
			Description: sk.Description,
			Body:        sk.Body,
		})
	}

	return entries
}
