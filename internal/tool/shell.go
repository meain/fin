package tool

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	t "github.com/meain/fin/internal/types"
)

const defaultShellTimeout = 30 // seconds

// pipeDrainGrace is how long to keep reading output after sh exits, before
// giving up on background children that still hold stdout/stderr open.
const pipeDrainGrace = 500 * time.Millisecond

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
	// Setpgid puts sh and everything it spawns in its own process group so
	// a timeout signals the whole tree, not just sh.
	cmd := exec.Command("sh", "-c", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Plain os.Pipes (rather than StdoutPipe) let cmd.Wait return as soon
	// as sh exits, even when a background child still holds the pipes.
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		return t.ToolResult{}, fmt.Errorf("stdout pipe: %w", err)
	}
	defer stdoutR.Close()
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		stdoutW.Close()
		return t.ToolResult{}, fmt.Errorf("stderr pipe: %w", err)
	}
	defer stderrR.Close()
	cmd.Stdout = stdoutW
	cmd.Stderr = stderrW

	err = cmd.Start()
	stdoutW.Close()
	stderrW.Close()
	if err != nil {
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
		scanner := bufio.NewScanner(stdoutR)
		// bufio.Scanner's default max token size is 64KB; a single line longer
		// than that (e.g. minified/base64 output with no newline) would make
		// Scan() fail with ErrTooLong and stop reading, leaving the pipe
		// undrained so the child blocks on write() until the timeout.
		// Raise the cap generously so that only pathological output hits it.
		scanner.Buffer(make([]byte, 64<<10), 10<<20) // up to 10 MiB per line
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
		io.Copy(&stderrBuf, stderrR)
	}()

	// Watch for context cancellation and shut down gracefully:
	// SIGTERM to the process group first, then SIGKILL after 2s.
	pgid := cmd.Process.Pid
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			// done is closed before the deferred cancel runs, so this
			// skips the normal-return case and leaves background jobs be.
			select {
			case <-done:
				return
			default:
			}
			syscall.Kill(-pgid, syscall.SIGTERM)
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				syscall.Kill(-pgid, syscall.SIGKILL)
			}
		case <-done:
		}
	}()

	err = cmd.Wait()

	// sh has exited. Give the readers a moment to drain what's already been
	// written, then stop waiting on background processes that still hold
	// the pipes open (e.g. "server &").
	readersDone := make(chan struct{})
	go func() { wg.Wait(); close(readersDone) }()
	select {
	case <-readersDone:
	case <-time.After(pipeDrainGrace):
		stdoutR.Close()
		stderrR.Close()
		<-readersDone
	}

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
