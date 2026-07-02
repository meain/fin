package tool

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	t "github.com/meain/fin/internal/types"
)

const defaultShellTimeout = 30 // seconds

// ShellTool executes shell commands.
type ShellTool struct {
	// OnOutput is called with each new stdout line and the current total
	// line count as output streams in. Set by the agent to update the UI
	// during execution.
	OnOutput func(line string, total int)
}

func (st *ShellTool) Name() string { return "shell" }

func (st *ShellTool) PrimaryArg(args map[string]any) string {
	cmd, _ := args["command"].(string)
	return cmd
}

func (st *ShellTool) Label(args map[string]any) ToolLabel {
	cmd, _ := args["command"].(string)
	if cmd == "" {
		return ToolLabel{}
	}
	cmd = strings.ReplaceAll(cmd, "\n", `\n`)
	return ToolLabel{Primary: "$ " + cmd}
}

func (st *ShellTool) Description() string {
	return "Execute a shell command via sh -c. Returns stdout and stderr separately. If you need them interleaved in order, append 2>&1 to your command."
}

func (st *ShellTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The shell command to execute",
			},
			"timeout": map[string]any{
				"type":        "integer",
				"description": "Timeout in seconds (default 30)",
			},
		},
		"required": []string{"command"},
	}
}

func (st *ShellTool) Run(ctx context.Context, args map[string]any) (t.ToolResult, error) {
	command, _ := args["command"].(string)
	if command == "" {
		return t.ToolResult{}, fmt.Errorf("command is required")
	}

	timeout := defaultShellTimeout
	if v, ok := args["timeout"].(float64); ok && v > 0 {
		timeout = int(v)
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	// Use plain exec.Command so we control shutdown sequencing ourselves.
	// exec.CommandContext would send SIGKILL immediately on cancellation.
	cmd := exec.Command("sh", "-c", command)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return t.ToolResult{}, fmt.Errorf("stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return t.ToolResult{}, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return t.ToolResult{}, fmt.Errorf("start command: %w", err)
	}

	var stdout, stderrBuf bytes.Buffer
	var lineCount int
	var mu sync.Mutex

	// Stream both stdout and stderr, counting lines
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			mu.Lock()
			stdout.Write(scanner.Bytes())
			stdout.WriteByte('\n')
			lineCount++
			lc := lineCount
			line := scanner.Text()
			mu.Unlock()
			if st.OnOutput != nil {
				st.OnOutput(line, lc)
			}
		}
	}()
	go func() {
		defer wg.Done()
		io.Copy(&stderrBuf, stderrPipe)
	}()

	// Watch for context cancellation and shut down gracefully:
	// SIGTERM first, then SIGKILL after 2s if still running.
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			cmd.Process.Signal(syscall.SIGTERM)
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				cmd.Process.Kill()
			}
		case <-done:
		}
	}()

	wg.Wait()
	err = cmd.Wait()
	close(done)

	var result string
	if stdout.Len() > 0 {
		result = stdout.String()
	}
	if stderrBuf.Len() > 0 {
		if result != "" {
			result += "\nstderr:\n"
		} else {
			result = "stderr:\n"
		}
		result += stderrBuf.String()
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return t.ToolResult{Content: result}, fmt.Errorf("command timed out after %ds", timeout)
		}
		exitCode := -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		return t.ToolResult{Content: fmt.Sprintf("%s\nexit code: %d", result, exitCode)}, nil
	}

	return t.ToolResult{Content: result}, nil
}
