package tool

import "fmt"

// ToolLabel is a structured, presentation-agnostic summary of a tool call.
// Primary is the subject (path, command, skill name). Detail is optional
// extra context (line range, edit size, truncated task). Renderers compose
// these with their own formatting (ANSI for terminal, HTML for export, JSON
// for a web client).
type ToolLabel struct {
	Primary string
	Detail  string
}

// Labeler is implemented by tools that want a richer per-call summary than
// just the tool name.
type Labeler interface {
	Label(args map[string]any) ToolLabel
}

// LabelFor looks up a labeler for the given tool name and computes a
// ToolLabel. Tools without a registered labeler fall back to an empty
// label (only the name is meaningful).
func LabelFor(name string, args map[string]any) ToolLabel {
	if l, ok := labelers[name]; ok {
		return l.Label(args)
	}
	return ToolLabel{}
}

// labelers is the static dispatch table used by LabelFor. Tools that need a
// label register a stateless instance here. SubagentTool is included with a
// nil RunSubagent — its Label method only inspects args.
var labelers = map[string]Labeler{
	"shell":     &ShellTool{},
	"read":      &ReadTool{},
	"write":     &WriteTool{},
	"edit":      &EditTool{},
	"use_skill": &SkillTool{},
	"subagent":  &SubagentTool{},
}

// rangeDetail formats a read offset/limit pair as "N:M", "N:", or ":M".
// Returns the empty string when neither is set.
func rangeDetail(offset, limit float64, hasOffset, hasLimit bool) string {
	switch {
	case hasOffset && hasLimit:
		return fmt.Sprintf("%d:%d", int(offset), int(offset)+int(limit))
	case hasOffset:
		return fmt.Sprintf("%d:", int(offset))
	case hasLimit:
		return fmt.Sprintf(":%d", int(limit))
	}
	return ""
}
