package main

import (
	"fmt"
	"os"
	"strings"
)

var stderr = os.Stderr

// ANSI escape codes
const (
	reset   = "\033[0m"
	bold    = "\033[1m"
	dim     = "\033[2m"
	red     = "\033[31m"
	green   = "\033[32m"
	yellow  = "\033[33m"
	magenta = "\033[35m"
)

// UI handles terminal output. When a Terminal is set, all output goes through
// it so the user's input line is preserved. Falls back to direct stderr writes.
// OutputMode controls how much the UI displays.
type OutputMode int

const (
	OutputNormal  OutputMode = iota // full output with colors
	OutputMinimal                   // just tool names + streamed text
	OutputQuiet                     // only final response text (stdout)
)

type UI struct {
	term              *Terminal
	mode              OutputMode
	wroteText         bool // tracks if text was written since last newline
	hasProgress       bool // tracks if a progress line is showing
	lastProgressLines int  // last line count shown in progress (for throttling)
}

func parseOutputMode(s string) OutputMode {
	switch s {
	case "minimal":
		return OutputMinimal
	case "quiet":
		return OutputQuiet
	default:
		return OutputNormal
	}
}

func NewUI(t *Terminal, mode OutputMode) *UI {
	return &UI{term: t, mode: mode}
}

func (u *UI) write(s string) {
	if u.term != nil {
		u.term.WriteString(s)
	} else {
		fmt.Fprint(stderr, s)
	}
}

// AssistantLabel prints the assistant label before streaming starts.
func (u *UI) AssistantLabel() {
	if u.mode != OutputNormal {
		return
	}
	u.write(fmt.Sprintf("\n%s%sfin>%s ", bold, magenta, reset))
}

// StreamText prints a text chunk from the assistant (during streaming).
func (u *UI) StreamText(text string) {
	if u.mode == OutputQuiet {
		fmt.Fprint(os.Stdout, text)
		return
	}
	if text != "" {
		u.wroteText = true
	}
	u.write(text)
}

// ensureNewline emits a newline if text was written without a trailing newline.
func (u *UI) ensureNewline() {
	if u.wroteText {
		u.write("\n")
		u.wroteText = false
	}
}

// EndStream finishes the assistant's streaming output.
func (u *UI) EndStream() {
	u.ensureNewline()
	if u.mode == OutputNormal {
		u.write("\n")
	}
}

// ToolCallProgress shows live progress while tool call arguments are streaming.
func (u *UI) ToolCallProgress(name, argsSoFar string) {
	if u.mode == OutputQuiet {
		return
	}

	lines := strings.Count(argsSoFar, "\\n") + strings.Count(argsSoFar, "\n")
	if lines == 0 {
		return
	}

	// Throttle: only update if line count changed
	if lines == u.lastProgressLines {
		return
	}
	u.lastProgressLines = lines

	if !u.hasProgress {
		u.ensureNewline()
	}

	fmt.Fprintf(stderr, "\r\033[2K%s%s%s %s(%d lines)%s", yellow, name, reset, dim, lines, reset)
	stderr.Sync()
	u.hasProgress = true
}

// ToolCallStart shows a tool being invoked.
func (u *UI) ToolCallStart(name string, args map[string]any) {
	if u.hasProgress {
		fmt.Fprint(stderr, "\033[2K\r")
		u.hasProgress = false
		u.lastProgressLines = 0
	} else {
		u.ensureNewline()
	}
	if u.mode == OutputQuiet {
		return
	}
	if u.mode == OutputMinimal {
		u.toolCallMinimal(name, args)
		return
	}
	u.write(fmt.Sprintf("\n  %s%s%s%s", bold, yellow, name, reset))
	switch name {
	case "shell":
		if cmd, ok := args["command"].(string); ok {
			u.write(fmt.Sprintf(" %s$ %s%s", dim, cmd, reset))
		}
	case "read":
		if path, ok := args["path"].(string); ok {
			u.write(fmt.Sprintf(" %s%s%s", dim, path, reset))
		}
	case "edit":
		if path, ok := args["path"].(string); ok {
			u.write(fmt.Sprintf(" %s%s%s", dim, path, reset))
		}
		u.write("\n")
		if old, ok := args["old_string"].(string); ok {
			for _, line := range strings.Split(old, "\n") {
				u.write(fmt.Sprintf("  %s%s- %s%s\n", dim, red, line, reset))
			}
		}
		if nw, ok := args["new_string"].(string); ok {
			for _, line := range strings.Split(nw, "\n") {
				u.write(fmt.Sprintf("  %s%s+ %s%s\n", dim, green, line, reset))
			}
		}
		return
	case "write":
		if path, ok := args["path"].(string); ok {
			u.write(fmt.Sprintf(" %s%s%s", dim, path, reset))
		}
		u.write("\n")
		if content, ok := args["content"].(string); ok {
			lines := strings.Split(content, "\n")
			show := lines
			if len(show) > 15 {
				show = show[:15]
			}
			for _, line := range show {
				u.write(fmt.Sprintf("  %s%s+ %s%s\n", dim, green, line, reset))
			}
			if len(lines) > 15 {
				u.write(fmt.Sprintf("  %s… %d more lines%s\n", dim, len(lines)-15, reset))
			}
		}
		return
	}
	u.write("\n")
}

func (u *UI) toolCallMinimal(name string, args map[string]any) {
	switch name {
	case "shell":
		cmd, _ := args["command"].(string)
		fmt.Fprintf(stderr, "%s%s%s %s$ %s%s", yellow, name, reset, dim, cmd, reset)
	case "read":
		path, _ := args["path"].(string)
		fmt.Fprintf(stderr, "%s%s%s %s%s%s", yellow, name, reset, dim, path, reset)
	case "write":
		path, _ := args["path"].(string)
		content, _ := args["content"].(string)
		lines := strings.Count(content, "\n") + 1
		fmt.Fprintf(stderr, "%s%s%s %s%s (%d lines)%s", yellow, name, reset, dim, path, lines, reset)
	case "edit":
		path, _ := args["path"].(string)
		old, _ := args["old_string"].(string)
		nw, _ := args["new_string"].(string)
		oldLines := strings.Count(old, "\n") + 1
		newLines := strings.Count(nw, "\n") + 1
		fmt.Fprintf(stderr, "%s%s%s %s%s (-%d +%d lines)%s", yellow, name, reset, dim, path, oldLines, newLines, reset)
	case "use_skill":
		skill, _ := args["name"].(string)
		fmt.Fprintf(stderr, "%s%s%s %s%s%s", yellow, name, reset, dim, skill, reset)
	default:
		fmt.Fprintf(stderr, "%s%s%s", yellow, name, reset)
	}
}

// ToolCallDone shows a completed tool call with its name, args, and result on a compact line.
func (u *UI) ToolCallDone(name string, args map[string]any, result string, err error) {
	if u.mode == OutputQuiet {
		return
	}

	label := u.toolLabel(name, args)
	lines := strings.Count(result, "\n")

	if u.mode == OutputMinimal {
		if err != nil {
			fmt.Fprintf(stderr, "%s%s %serror: %s%s\n", yellow, label, red, err, reset)
		} else if lines > 0 {
			fmt.Fprintf(stderr, "%s%s %s(%d lines)%s\n", yellow, label, dim, lines, reset)
		} else {
			fmt.Fprintf(stderr, "%s%s%s\n", yellow, label, reset)
		}
		return
	}

	u.write(fmt.Sprintf("  %s%s%s%s", bold, yellow, label, reset))
	if err != nil {
		u.write(fmt.Sprintf(" %s%serror: %s%s\n", dim, red, err, reset))
		return
	}
	if lines > 0 {
		u.write(fmt.Sprintf(" %s(%d lines)%s\n", dim, lines, reset))
	} else {
		u.write("\n")
	}
}

// toolLabel returns a short description like "read agent.go" or "shell $ ls".
func (u *UI) toolLabel(name string, args map[string]any) string {
	switch name {
	case "shell":
		if cmd, ok := args["command"].(string); ok {
			return name + reset + " " + dim + "$ " + cmd
		}
	case "read":
		if path, ok := args["path"].(string); ok {
			return name + reset + " " + dim + path
		}
	case "write":
		if path, ok := args["path"].(string); ok {
			return name + reset + " " + dim + path
		}
	case "edit":
		if path, ok := args["path"].(string); ok {
			old, _ := args["old_string"].(string)
			nw, _ := args["new_string"].(string)
			oldLines := strings.Count(old, "\n") + 1
			newLines := strings.Count(nw, "\n") + 1
			return fmt.Sprintf("%s%s %s%s (-%d +%d)", name, reset, dim, path, oldLines, newLines)
		}
	case "use_skill":
		if skill, ok := args["name"].(string); ok {
			return name + reset + " " + dim + skill
		}
	}
	return name
}

// ToolCallResult shows abbreviated tool output.
func (u *UI) ToolCallResult(result string, err error) {
	if u.mode == OutputQuiet {
		return
	}
	if u.mode == OutputMinimal {
		if err != nil {
			fmt.Fprintf(stderr, " %serror: %s%s\n", red, err, reset)
		} else if result != "" {
			lines := strings.Count(result, "\n")
			if lines > 0 {
				fmt.Fprintf(stderr, " %s(%d lines)%s\n", dim, lines, reset)
			} else {
				fmt.Fprintln(stderr)
			}
		} else {
			fmt.Fprintln(stderr)
		}
		return
	}

	if err != nil {
		u.write(fmt.Sprintf("  %s%serror: %s%s\n", dim, red, err, reset))
		return
	}

	lines := strings.Split(result, "\n")
	if len(lines) > 10 {
		for _, line := range lines[:5] {
			u.write(fmt.Sprintf("  %s%s%s\n", dim, line, reset))
		}
		u.write(fmt.Sprintf("  %s… %d lines total%s\n", dim, len(lines), reset))
	} else {
		for _, line := range lines {
			if line != "" {
				u.write(fmt.Sprintf("  %s%s%s\n", dim, line, reset))
			}
		}
	}
}

// ToolApprovalPrompt asks the user to approve a tool call. Returns true if approved.
func (u *UI) ToolApprovalPrompt(name string, args map[string]any) bool {
	if u.term != nil {
		u.term.WriteString(fmt.Sprintf("  %s%sallow %s? [y/N]%s ", bold, yellow, name, reset))
		line, err := u.term.ReadLine("")
		if err != nil {
			return false
		}
		line = strings.TrimSpace(strings.ToLower(line))
		return line == "y" || line == "yes"
	}
	u.write(fmt.Sprintf("  %s%sallow %s? [y/N]%s ", bold, yellow, name, reset))
	var input string
	fmt.Scanln(&input)
	input = strings.TrimSpace(strings.ToLower(input))
	return input == "y" || input == "yes"
}

// Info prints an informational message.
func (u *UI) Info(msg string) {
	if u.mode != OutputNormal {
		return
	}
	u.write(fmt.Sprintf("%s%s%s\n", dim, msg, reset))
}

// Error prints an error message.
func (u *UI) Error(msg string) {
	u.write(fmt.Sprintf("%s%serror: %s%s\n", bold, red, msg, reset))
}
