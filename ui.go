package main

import (
	"fmt"
	"os"
	"strings"
)

// ANSI escape codes
const (
	reset   = "\033[0m"
	bold    = "\033[1m"
	dim     = "\033[2m"
	red     = "\033[31m"
	green   = "\033[32m"
	yellow  = "\033[33m"
	_       = "\033[34m" // blue
	magenta = "\033[35m"
	_       = "\033[36m" // cyan
)

// UI handles terminal output.
type UI struct{}

func NewUI() *UI {
	return &UI{}
}

// UserPrompt prints the user input prompt.
func (u *UI) UserPrompt() {
	fmt.Fprintf(os.Stderr, "%s%syou>%s ", bold, green, reset)
}

// AssistantLabel prints the assistant label before streaming starts.
func (u *UI) AssistantLabel() {
	fmt.Fprintf(os.Stderr, "\n%s%sfin>%s ", bold, magenta, reset)
}

// StreamText prints a text chunk from the assistant (during streaming).
func (u *UI) StreamText(text string) {
	fmt.Fprint(os.Stderr, text)
}

// EndStream finishes the assistant's streaming output.
func (u *UI) EndStream() {
	fmt.Fprintln(os.Stderr)
}

// ToolCallStart shows a tool being invoked.
func (u *UI) ToolCallStart(name string, args map[string]any) {
	fmt.Fprintf(os.Stderr, "\n  %s%s[%s]%s ", dim, yellow, name, reset)
	switch name {
	case "shell":
		if cmd, ok := args["command"].(string); ok {
			fmt.Fprintf(os.Stderr, "%s%s%s", dim, cmd, reset)
		}
	case "read":
		if path, ok := args["path"].(string); ok {
			fmt.Fprintf(os.Stderr, "%s%s%s", dim, path, reset)
		}
	case "edit":
		if path, ok := args["path"].(string); ok {
			fmt.Fprintf(os.Stderr, "%s%s%s", dim, path, reset)
		}
		fmt.Fprintln(os.Stderr)
		if old, ok := args["old_string"].(string); ok {
			for _, line := range strings.Split(old, "\n") {
				fmt.Fprintf(os.Stderr, "  %s%s- %s%s\n", dim, red, line, reset)
			}
		}
		if new, ok := args["new_string"].(string); ok {
			for _, line := range strings.Split(new, "\n") {
				fmt.Fprintf(os.Stderr, "  %s%s+ %s%s\n", dim, green, line, reset)
			}
		}
		return
	case "write":
		if path, ok := args["path"].(string); ok {
			fmt.Fprintf(os.Stderr, "%s%s%s", dim, path, reset)
		}
		fmt.Fprintln(os.Stderr)
		if content, ok := args["content"].(string); ok {
			lines := strings.Split(content, "\n")
			show := lines
			if len(show) > 15 {
				show = show[:15]
			}
			for _, line := range show {
				fmt.Fprintf(os.Stderr, "  %s%s+ %s%s\n", dim, green, line, reset)
			}
			if len(lines) > 15 {
				fmt.Fprintf(os.Stderr, "  %s... (%d more lines)%s\n", dim, len(lines)-15, reset)
			}
		}
		return
	}
	fmt.Fprintln(os.Stderr)
}

// ToolCallResult shows abbreviated tool output.
func (u *UI) ToolCallResult(result string, err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "  %s%serror: %s%s\n", dim, red, err, reset)
		return
	}

	// Truncate long output
	lines := strings.Split(result, "\n")
	if len(lines) > 10 {
		for _, line := range lines[:5] {
			fmt.Fprintf(os.Stderr, "  %s%s%s\n", dim, line, reset)
		}
		fmt.Fprintf(os.Stderr, "  %s... (%d more lines)%s\n", dim, len(lines)-5, reset)
	} else {
		for _, line := range lines {
			if line != "" {
				fmt.Fprintf(os.Stderr, "  %s%s%s\n", dim, line, reset)
			}
		}
	}
}

// ToolApprovalPrompt asks the user to approve a tool call. Returns true if approved.
func (u *UI) ToolApprovalPrompt(name string, args map[string]any) bool {
	fmt.Fprintf(os.Stderr, "  %s%sallow %s? [y/N]%s ", bold, yellow, name, reset)

	var input string
	fmt.Scanln(&input)
	input = strings.TrimSpace(strings.ToLower(input))
	return input == "y" || input == "yes"
}

// Info prints an informational message.
func (u *UI) Info(msg string) {
	fmt.Fprintf(os.Stderr, "%s%s%s\n", dim, msg, reset)
}

// Error prints an error message.
func (u *UI) Error(msg string) {
	fmt.Fprintf(os.Stderr, "%s%serror: %s%s\n", bold, red, msg, reset)
}
