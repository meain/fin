package main

import "context"

// Tool is something the agent can invoke.
type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]any // JSON Schema
	Run(ctx context.Context, args map[string]any) (string, error)
}

// AllTools returns the built-in tool set, plus the skill tool if skills are available.
func AllTools(skills []*Skill) []Tool {
	tools := []Tool{
		&readTool{},
		&writeTool{},
		&editTool{},
		&shellTool{},
	}
	if len(skills) > 0 {
		tools = append(tools, newSkillTool(skills))
	}
	return tools
}

// ToolDefs converts tools to the ToolDef format used in completion requests.
func ToolDefsFrom(tools []Tool) []ToolDef {
	defs := make([]ToolDef, len(tools))
	for i, t := range tools {
		defs[i] = ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Parameters(),
		}
	}
	return defs
}

// FindTool looks up a tool by name.
func FindTool(tools []Tool, name string) Tool {
	for _, t := range tools {
		if t.Name() == name {
			return t
		}
	}
	return nil
}
