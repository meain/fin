package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

const defaultShellTimeout = 120 // seconds

type shellTool struct{}

func (t *shellTool) Name() string { return "shell" }

func (t *shellTool) Description() string {
	return "Execute a shell command via sh -c. Returns stdout and stderr separately. If you need them interleaved in order, append 2>&1 to your command."
}

func (t *shellTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The shell command to execute",
			},
			"timeout": map[string]any{
				"type":        "integer",
				"description": "Timeout in seconds (default 120)",
			},
		},
		"required": []string{"command"},
	}
}

func (t *shellTool) Run(ctx context.Context, args map[string]any) (ToolResult, error) {
	command, _ := args["command"].(string)
	if command == "" {
		return ToolResult{}, fmt.Errorf("command is required")
	}

	timeout := defaultShellTimeout
	if v, ok := args["timeout"].(float64); ok && v > 0 {
		timeout = int(v)
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	var result string
	if stdout.Len() > 0 {
		result = stdout.String()
	}
	if stderr.Len() > 0 {
		if result != "" {
			result += "\nstderr:\n"
		} else {
			result = "stderr:\n"
		}
		result += stderr.String()
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return ToolResult{Content: result}, fmt.Errorf("command timed out after %ds", timeout)
		}
		exitCode := -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		return ToolResult{Content: fmt.Sprintf("%s\nexit code: %d", result, exitCode)}, nil
	}

	return ToolResult{Content: result}, nil
}
