package tool

import (
	"context"

	t "github.com/meain/fin/internal/types"
)

// Tool is something the agent can invoke.
type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]any // JSON Schema
	Run(ctx context.Context, args map[string]any) (t.ToolResult, error)
}

// Defs converts tools to the ToolDef format used in completion requests.
func Defs(tools []Tool) []t.ToolDef {
	defs := make([]t.ToolDef, len(tools))
	for i, tl := range tools {
		defs[i] = t.ToolDef{
			Name:        tl.Name(),
			Description: tl.Description(),
			Parameters:  tl.Parameters(),
		}
	}
	return defs
}

// Find looks up a tool by name.
func Find(tools []Tool, name string) Tool {
	for _, tl := range tools {
		if tl.Name() == name {
			return tl
		}
	}
	return nil
}

// BuiltinTools returns the built-in tool set (read, write, edit, shell).
func BuiltinTools() []Tool {
	return []Tool{
		&ReadTool{},
		&WriteTool{},
		&EditTool{},
		&ShellTool{},
	}
}
