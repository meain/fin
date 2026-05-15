package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"path/filepath"
	"strings"

	finembed "github.com/meain/fin/internal/embed"
	"github.com/meain/fin/internal/tool"
	t "github.com/meain/fin/internal/types"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

var md = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
)

// renderMarkdown converts markdown to HTML.
func renderMarkdown(src string) string {
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		return html.EscapeString(src)
	}
	return buf.String()
}

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
	writeHTMLHead(w, title, sess)

	opts := renderOpts{
		UserLabel:      "you",
		CollapseLabels: true,
		ToolMap:        buildToolCallMap(sess.Messages),
		ExpandSubagent: true,
	}
	renderMessageList(w, sess.Messages, opts)

	fmt.Fprint(w, "<script>hljs.highlightAll();</script>\n</body>\n</html>\n")
}

// renderOpts controls how a sequence of messages is rendered to HTML. The
// same renderer is used for the top-level conversation and for nested
// subagent transcripts.
type renderOpts struct {
	UserLabel      string                // "you" for main convo, "task" for subagent
	CollapseLabels bool                  // collapse consecutive same-role labels (main only)
	ToolMap        map[string]t.ToolCall // tool calls indexed by ID
	ExpandSubagent bool                  // recurse into RoleTool subagent SubMessages
}

func writeHTMLHead(w io.Writer, title string, sess *Session) {
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
  .msg { padding-left: 0.75rem; border-left: 1px solid transparent; padding-top: 0.25rem; padding-bottom: 0.25rem; }
  .msg-gap { margin-top: 1rem; }
  .msg-role { font-size: 0.75rem; font-weight: 600; text-transform: uppercase; letter-spacing: 0.05em; margin-bottom: 0.25rem; }
  .msg-time { font-size: 0.7rem; color: #999; margin-left: 0.5rem; font-weight: normal; text-transform: none; letter-spacing: normal; }
  .msg-content { font-size: 0.95rem; line-height: 1.6; }
  .role-user { border-left-color: #93c5fd; background: #eff6ff; }
  .role-assistant, .role-tool { border-left-color: #c7d2fe; }
  .msg-content p { margin: 0.5em 0; }
  .msg-content pre { background: #f3f4f6; padding: 0.75rem; border-radius: 6px; overflow-x: auto; margin: 0.5em 0; }
  .msg-content code { font-family: monospace; font-size: 0.9em; background: #f3f4f6; padding: 0.1em 0.3em; border-radius: 3px; }
  .msg-content pre code { background: none; padding: 0; }
  .msg-content ul, .msg-content ol { padding-left: 1.5em; margin: 0.5em 0; }
  .msg-content h1, .msg-content h2, .msg-content h3 { margin: 0.75em 0 0.25em; }
  .msg-content blockquote { border-left: 3px solid #d1d5db; padding-left: 0.75rem; color: #6b7280; margin: 0.5em 0; }
  .msg-content table { border-collapse: collapse; margin: 0.5em 0; width: auto; }
  .msg-content th, .msg-content td { border: 1px solid #d1d5db; padding: 0.35rem 0.75rem; text-align: left; }
  .msg-content th { background: #f3f4f6; font-weight: 600; }
  .msg-content ul.contains-task-list { list-style: none; padding-left: 0.5em; }
  .msg-content li > input[type="checkbox"] { margin-right: 0.4em; }
  .role-user .msg-role { color: #2563eb; }
  .role-assistant .msg-role { color: #6366f1; }
  .role-tool .msg-role { color: #6366f1; }
  .role-system { border-left-color: #d1d5db; }
  .role-system .msg-role { color: #6b7280; }
  .role-system .msg-content { font-size: 0.85rem; color: #6b7280; white-space: pre-wrap; font-family: monospace; max-height: 200px; overflow-y: auto; }
  .subagent { margin: 0.5rem 0; padding: 0.5rem 0.75rem; border: 1px solid #e5e7eb; border-radius: 6px; background: #fafafa; }
  .subagent-label { font-size: 0.75rem; font-weight: 600; text-transform: uppercase; letter-spacing: 0.05em; color: #ca8a04; margin-bottom: 0.25rem; }
  .subagent .msg { font-size: 0.9rem; }
  .subagent .role-system .msg-content { max-height: 100px; }
  .tool-calls { margin-top: 0.5rem; }
  .tool-call { background: #f9fafb; border: 1px solid #e5e7eb; border-radius: 6px; padding: 0.5rem 0.75rem; margin-top: 0.25rem; font-family: monospace; font-size: 0.85rem; cursor: pointer; }
  summary.tool-call::marker { color: #d1d5db; font-size: 0.75rem; }
  details .tool-result, details .diff { margin-top: 0.25rem; }
  .tool-name { color: #ca8a04; font-weight: 600; }
  .tool-result { background: #f9fafb; border-left: 3px solid #e5e7eb; padding: 0.5rem 0.75rem; font-family: monospace; font-size: 0.85rem; white-space: pre-wrap; max-height: 300px; overflow-y: auto; }
  .tool-result pre { background: none; margin: 0; padding: 0; border-radius: 0; }
  .tool-result pre code { background: none; padding: 0; font-size: inherit; }
  .diff { font-family: monospace; font-size: 0.85rem; margin-top: 0.25rem; border-radius: 6px; overflow-y: auto; max-height: 300px; }
  .diff pre { background: none; margin: 0; padding: 0; border-radius: 0; }
  .diff pre code { background: none; padding: 0; font-size: inherit; }
</style>
`,
		html.EscapeString(title),
	)
	fmt.Fprintf(w, "<style>%s</style>\n", finembed.HljsCSS)
	fmt.Fprintf(w, "<script>%s</script>\n", finembed.HljsJS)
	fmt.Fprintf(w, `</head>
<body>
<h1>%s</h1>
<div class="meta">%s &middot; %s &middot; %s</div>
`,
		html.EscapeString(title),
		html.EscapeString(sess.ID),
		html.EscapeString(sess.Model),
		html.EscapeString(sess.StartedAt.Format("2006-01-02 15:04")),
	)
}

// buildToolCallMap indexes a message slice by tool call ID for fast lookup
// when rendering tool results.
func buildToolCallMap(msgs []t.Message) map[string]t.ToolCall {
	out := map[string]t.ToolCall{}
	for _, m := range msgs {
		for _, tc := range m.ToolCalls {
			out[tc.ID] = tc
		}
	}
	return out
}

// displayRole maps a message role to its visual label. Assistant and tool
// messages collapse to one display role ("fin") so consecutive labels can
// be hidden in the main conversation.
func displayRole(r t.Role) string {
	switch r {
	case t.RoleAssistant, t.RoleTool:
		return "fin"
	case t.RoleUser:
		return "you"
	case t.RoleSystem:
		return "system"
	default:
		return string(r)
	}
}

// renderMessageList iterates messages and emits HTML per opts. Returns
// nothing — the renderer is purely side-effecting on w.
func renderMessageList(w io.Writer, msgs []t.Message, opts renderOpts) {
	prevDisplay := ""
	for _, m := range msgs {
		prevDisplay = renderMessage(w, m, opts, prevDisplay)
	}
}

// renderMessage renders a single message and returns the updated previous
// display role used for label collapsing on the next call.
func renderMessage(w io.Writer, m t.Message, opts renderOpts, prevDisplay string) string {
	ts := ""
	if !m.Timestamp.IsZero() {
		ts = m.Timestamp.Format("15:04:05")
	}

	curDisplay := displayRole(m.Role)
	showLabel := !opts.CollapseLabels || curDisplay != prevDisplay
	gap := ""
	if opts.CollapseLabels && showLabel && prevDisplay != "" {
		gap = " msg-gap"
	}

	switch m.Role {
	case t.RoleSystem:
		fmt.Fprintf(w, `<div class="msg role-system%s">`, gap)
		fmt.Fprint(w, `<details><summary class="msg-role">system</summary>`)
		fmt.Fprintf(w, `<div class="msg-content">%s</div></details></div>`+"\n", html.EscapeString(m.Content))

	case t.RoleUser:
		fmt.Fprintf(w, `<div class="msg role-user%s">`, gap)
		if showLabel {
			fmt.Fprintf(w, `<div class="msg-role">%s`, html.EscapeString(opts.UserLabel))
			if ts != "" && opts.CollapseLabels {
				fmt.Fprintf(w, `<span class="msg-time">%s</span>`, ts)
			}
			fmt.Fprint(w, `</div>`)
		}
		fmt.Fprintf(w, `<div class="msg-content">%s</div></div>`+"\n", renderMarkdown(m.Content))

	case t.RoleAssistant:
		hasBody := m.Content != "" || len(m.ToolCalls) > 0
		if showLabel && hasBody {
			fmt.Fprintf(w, `<div class="msg role-assistant%s">`, gap)
			if opts.CollapseLabels {
				fmt.Fprint(w, `<div class="msg-role">fin`)
				if ts != "" {
					fmt.Fprintf(w, `<span class="msg-time">%s</span>`, ts)
				}
				fmt.Fprint(w, `</div>`)
			}
			if m.Content != "" {
				fmt.Fprintf(w, `<div class="msg-content">%s</div>`, renderMarkdown(m.Content))
			}
			fmt.Fprint(w, "</div>\n")
		} else if m.Content != "" {
			fmt.Fprint(w, `<div class="msg role-assistant">`)
			fmt.Fprintf(w, `<div class="msg-content">%s</div></div>`+"\n", renderMarkdown(m.Content))
		}
		for _, tc := range m.ToolCalls {
			if tc.Name == "edit" || tc.Name == "write" {
				fmt.Fprint(w, `<div class="msg role-tool">`)
				renderToolCall(w, tc)
				fmt.Fprint(w, "</div>\n")
			}
		}

	case t.RoleTool:
		fmt.Fprint(w, `<div class="msg role-tool">`)
		if tc, ok := opts.ToolMap[m.ToolCallID]; ok {
			if tc.Name == "edit" || tc.Name == "write" {
				fmt.Fprint(w, "</div>\n")
				return curDisplay
			}
			summary := toolCallSummary(tc.Name, tc.Arguments)
			if opts.ExpandSubagent && tc.Name == "subagent" && len(m.SubMessages) > 0 {
				fmt.Fprintf(w, `<details><summary class="tool-call"><span class="tool-name">subagent</span> %s</summary>`,
					html.EscapeString(summary))
				renderSubagentConversation(w, m.SubMessages)
				fmt.Fprint(w, `</details>`)
			} else {
				renderToolResult(w, tc, summary, m.Content)
			}
		} else {
			fmt.Fprintf(w, `<div class="tool-result">%s</div>`, html.EscapeString(m.Content))
		}
		fmt.Fprint(w, "</div>\n")
	}

	return curDisplay
}

func renderToolCall(w io.Writer, tc t.ToolCall) {
	var args map[string]any
	if tc.Arguments != "" {
		_ = json.Unmarshal([]byte(tc.Arguments), &args)
	}
	if args == nil {
		args = map[string]any{}
	}

	if tc.Name == "write" {
		path, _ := args["path"].(string)
		content, _ := args["content"].(string)

		fmt.Fprintf(w, `<details open><summary class="tool-call"><span class="tool-name">write</span> %s</summary>`, html.EscapeString(path))
		fmt.Fprint(w, `<div class="tool-result">`)
		renderCodeBlock(w, content, langFromPath(path))
		fmt.Fprint(w, `</div></details>`)
		return
	}

	if tc.Name == "edit" {
		path, _ := args["path"].(string)
		oldStr, _ := args["old_string"].(string)
		newStr, _ := args["new_string"].(string)

		var diff strings.Builder
		for _, line := range strings.Split(oldStr, "\n") {
			diff.WriteString("- ")
			diff.WriteString(line)
			diff.WriteByte('\n')
		}
		for _, line := range strings.Split(newStr, "\n") {
			diff.WriteString("+ ")
			diff.WriteString(line)
			diff.WriteByte('\n')
		}

		fmt.Fprintf(w, `<details open><summary class="tool-call"><span class="tool-name">edit</span> %s</summary>`, html.EscapeString(path))
		fmt.Fprint(w, `<div class="diff">`)
		renderCodeBlock(w, diff.String(), "diff")
		fmt.Fprint(w, `</div></details>`)
		return
	}

	summary := toolCallSummary(tc.Name, tc.Arguments)
	fmt.Fprintf(w, `<div class="tool-call"><span class="tool-name">%s</span> %s</div>`,
		html.EscapeString(tc.Name), html.EscapeString(summary))
}

// renderSubagentConversation renders a subagent's full message history inside
// a styled container. Reuses renderMessageList with a subagent-flavored
// renderOpts (no label collapsing, "task" user label, no nested subagent
// expansion).
func renderSubagentConversation(w io.Writer, msgs []t.Message) {
	fmt.Fprint(w, `<div class="subagent">`)
	fmt.Fprint(w, `<div class="subagent-label">subagent</div>`)

	opts := renderOpts{
		UserLabel:      "task",
		CollapseLabels: false,
		ToolMap:        buildToolCallMap(msgs),
		ExpandSubagent: false,
	}
	renderMessageList(w, msgs, opts)

	fmt.Fprint(w, `</div>`)
}

// toolCallSummary returns the plain-text subject line shown next to a tool
// call in the HTML export. Wraps tool.LabelFor and falls back to
// "key=value..." for tools without a registered labeler.
func toolCallSummary(name, argsJSON string) string {
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ""
	}
	label := tool.LabelFor(name, args)
	if label.Primary != "" {
		return label.Primary
	}
	parts := make([]string, 0, len(args))
	for k, v := range args {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	return strings.Join(parts, " ")
}

// renderToolResult renders a tool result as a collapsible block, with syntax highlighting for read tools.
func renderToolResult(w io.Writer, tc t.ToolCall, summary, content string) {
	fmt.Fprintf(w, `<details><summary class="tool-call"><span class="tool-name">%s</span> %s</summary>`,
		html.EscapeString(tc.Name), html.EscapeString(summary))
	if tc.Name == "read" {
		if lang := langFromPath(summary); lang != "" {
			fmt.Fprint(w, `<div class="tool-result">`)
			renderCodeBlock(w, content, lang)
			fmt.Fprint(w, `</div>`)
		} else {
			fmt.Fprintf(w, `<div class="tool-result">%s</div>`, html.EscapeString(content))
		}
	} else {
		fmt.Fprintf(w, `<div class="tool-result">%s</div>`, html.EscapeString(content))
	}
	fmt.Fprint(w, `</details>`)
}

// langFromPath returns a highlight.js language hint from a file extension, or empty string.
func langFromPath(path string) string {
	ext := strings.TrimPrefix(filepath.Ext(path), ".")
	switch ext {
	case "go", "py", "js", "ts", "rs", "rb", "java", "c", "cpp", "css", "sql", "lua", "r",
		"swift", "kotlin", "scala", "dart", "perl", "php", "zig", "nim", "haskell", "elixir",
		"clojure", "graphql", "proto":
		return ext
	case "jsx":
		return "javascript"
	case "tsx":
		return "typescript"
	case "yml":
		return "yaml"
	case "md":
		return "markdown"
	case "sh", "bash", "zsh":
		return "bash"
	case "toml":
		return "ini"
	case "tf", "hcl":
		return "hcl"
	case "html", "htm":
		return "html"
	case "xml", "svg":
		return "xml"
	case "json":
		return "json"
	case "yaml":
		return "yaml"
	case "dockerfile":
		return "dockerfile"
	default:
		return ""
	}
}

// renderCodeBlock writes content as a highlighted code block with optional language hint.
func renderCodeBlock(w io.Writer, content, lang string) {
	if lang != "" {
		fmt.Fprintf(w, `<pre><code class="language-%s">%s</code></pre>`, lang, html.EscapeString(content))
	} else {
		fmt.Fprintf(w, `<pre><code>%s</code></pre>`, html.EscapeString(content))
	}
}
