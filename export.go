package main

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"strings"
)

// ExportJSON writes the session as formatted JSON to w.
func ExportJSON(sess *Session, w io.Writer) {
	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		fmt.Fprintf(w, "error: %s\n", err)
		return
	}
	w.Write(data)
	w.Write([]byte("\n"))
}

// ExportHTML writes the session as a self-contained HTML page to w.
func ExportHTML(sess *Session, w io.Writer) {
	title := sess.Title
	if title == "" {
		title = "fin session"
	}

	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { font-family: system-ui, -apple-system, sans-serif; background: #fff; color: #1a1a1a; max-width: 800px; margin: 0 auto; padding: 2rem 1rem; }
  .meta { color: #888; font-size: 0.85rem; margin-bottom: 2rem; border-bottom: 1px solid #eee; padding-bottom: 1rem; }
  .msg { margin-bottom: 1.5rem; }
  .msg-role { font-size: 0.75rem; font-weight: 600; text-transform: uppercase; letter-spacing: 0.05em; margin-bottom: 0.25rem; }
  .msg-time { font-size: 0.7rem; color: #999; margin-left: 0.5rem; font-weight: normal; text-transform: none; letter-spacing: normal; }
  .msg-content { white-space: pre-wrap; font-size: 0.95rem; line-height: 1.6; }
  .role-user .msg-role { color: #16a34a; }
  .role-assistant .msg-role { color: #9333ea; }
  .role-tool .msg-role { color: #ca8a04; }
  .role-system { display: none; }
  .tool-calls { margin-top: 0.5rem; }
  .tool-call { background: #f9fafb; border: 1px solid #e5e7eb; border-radius: 6px; padding: 0.5rem 0.75rem; margin-top: 0.25rem; font-family: monospace; font-size: 0.85rem; }
  .tool-name { color: #ca8a04; font-weight: 600; }
  .tool-result { background: #f9fafb; border-left: 3px solid #e5e7eb; padding: 0.5rem 0.75rem; font-family: monospace; font-size: 0.85rem; white-space: pre-wrap; max-height: 300px; overflow-y: auto; }
  .diff { font-family: monospace; font-size: 0.85rem; margin-top: 0.25rem; border-radius: 6px; overflow-y: auto; max-height: 300px; }
  .diff-del { background: #fef2f2; color: #991b1b; padding: 0 0.5rem; }
  .diff-add { background: #f0fdf4; color: #166534; padding: 0 0.5rem; }
  .diff-file { color: #6b7280; padding: 0 0.5rem; font-size: 0.8rem; }
</style>
</head>
<body>
<h1>%s</h1>
<div class="meta">%s &middot; %s &middot; %s</div>
`,
		html.EscapeString(title),
		html.EscapeString(title),
		html.EscapeString(sess.ID),
		html.EscapeString(sess.Model),
		html.EscapeString(sess.StartedAt.Format("2006-01-02 15:04")),
	)

	// Build a map of tool call ID → ToolCall for labeling tool results
	toolCallMap := map[string]ToolCall{}
	for _, m := range sess.Messages {
		for _, tc := range m.ToolCalls {
			toolCallMap[tc.ID] = tc
		}
	}

	for _, m := range sess.Messages {
		ts := ""
		if !m.Timestamp.IsZero() {
			ts = m.Timestamp.Format("15:04:05")
		}

		switch m.Role {
		case RoleSystem:
			fmt.Fprintf(w, `<div class="msg role-system"><div class="msg-role">system</div>`)
			fmt.Fprintf(w, `<div class="msg-content">%s</div></div>`+"\n", html.EscapeString(m.Content))

		case RoleUser:
			fmt.Fprintf(w, `<div class="msg role-user"><div class="msg-role">you`)
			if ts != "" {
				fmt.Fprintf(w, `<span class="msg-time">%s</span>`, ts)
			}
			fmt.Fprintf(w, `</div><div class="msg-content">%s</div></div>`+"\n", html.EscapeString(m.Content))

		case RoleAssistant:
			// Only show text content, skip tool call display (shown on tool results instead)
			if m.Content != "" {
				fmt.Fprintf(w, `<div class="msg role-assistant"><div class="msg-role">fin`)
				if ts != "" {
					fmt.Fprintf(w, `<span class="msg-time">%s</span>`, ts)
				}
				fmt.Fprintf(w, `</div><div class="msg-content">%s</div></div>`+"\n", html.EscapeString(m.Content))
			}
			// Render edit diffs inline (they don't have a separate tool result with useful content)
			for _, tc := range m.ToolCalls {
				if tc.Name == "edit" {
					fmt.Fprint(w, `<div class="msg role-tool">`)
					renderToolCall(w, tc)
					fmt.Fprint(w, "</div>\n")
				}
			}

		case RoleTool:
			fmt.Fprint(w, `<div class="msg role-tool">`)
			// Show what tool was called as the header
			if tc, ok := toolCallMap[m.ToolCallID]; ok {
				if tc.Name == "edit" {
					// Edit already rendered inline with diff, skip the result
					fmt.Fprint(w, "</div>\n")
					continue
				}
				renderToolCall(w, tc)
			}
			fmt.Fprintf(w, `<div class="tool-result">%s</div></div>`+"\n", html.EscapeString(m.Content))
		}
	}

	fmt.Fprint(w, "</body>\n</html>\n")
}

func renderToolCall(w io.Writer, tc ToolCall) {
	var args map[string]any
	if tc.Arguments != "" {
		_ = json.Unmarshal([]byte(tc.Arguments), &args)
	}
	if args == nil {
		args = map[string]any{}
	}

	if tc.Name == "edit" {
		path, _ := args["path"].(string)
		oldStr, _ := args["old_string"].(string)
		newStr, _ := args["new_string"].(string)

		fmt.Fprintf(w, `<div class="tool-call"><span class="tool-name">edit</span> %s</div>`, html.EscapeString(path))
		fmt.Fprint(w, `<div class="diff">`)
		for _, line := range strings.Split(oldStr, "\n") {
			fmt.Fprintf(w, `<div class="diff-del">- %s</div>`, html.EscapeString(line))
		}
		for _, line := range strings.Split(newStr, "\n") {
			fmt.Fprintf(w, `<div class="diff-add">+ %s</div>`, html.EscapeString(line))
		}
		fmt.Fprint(w, `</div>`)
		return
	}

	summary := toolCallSummary(tc.Name, tc.Arguments)
	fmt.Fprintf(w, `<div class="tool-call"><span class="tool-name">%s</span> %s</div>`,
		html.EscapeString(tc.Name), html.EscapeString(summary))
}

func toolCallSummary(name, argsJSON string) string {
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ""
	}
	switch name {
	case "shell":
		cmd, _ := args["command"].(string)
		return "$ " + cmd
	case "read":
		path, _ := args["path"].(string)
		return path
	case "write":
		path, _ := args["path"].(string)
		return path
	case "edit":
		path, _ := args["path"].(string)
		return path
	case "use_skill":
		s, _ := args["name"].(string)
		return s
	default:
		parts := make([]string, 0)
		for k, v := range args {
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}
		return strings.Join(parts, " ")
	}
}
