---
name: about_fin
description: Learn about fin itself — what it is, how it works, its architecture. Use when asked about fin.
---

# fin

fin is a minimal, opinionated CLI agent harness written in Go by meain.
Source code: https://github.com/meain/fin

## What it does

fin takes a prompt, runs an agent loop (stream LLM response → execute tool calls → repeat), and exits. It supports session persistence, multiple LLM providers, an extensible skill system, and multiple output modes.

## Usage

```sh
fin "explain this code"                # basic prompt
fin -c "follow up"                     # continue last session
fin -s <uuid> "follow up"              # continue specific session (prefix match)
fin -n <name> "prompt"                 # named session (resumes if it exists, else creates)
fin -match "prompt"                    # search recent sessions, offer to resume a match
git diff | fin "review this"           # piped input
fin -export json|html|message          # export session
fin -export message | glow             # pipe last response to glow
fin -ui minimal "what is in go.mod"    # compact output
fin -ui quiet "summarize" > out.txt    # just the response on stdout
fin -sessions                          # list last 10 sessions
fin -all -sessions                     # list all sessions
fin -approve all|safe|none "prompt"    # tool approval mode
fin -yolo "prompt"                     # alias for -approve all
fin --max-turns 5 "prompt"             # cap agent loop iterations
fin -model provider/model "prompt"     # override model for this run
fin -f script.fin                      # read prompt from file (strips shebang line)
fin -f script.fin "extra args"         # file prompt + positional args appended
fin -tools read,shell "prompt"         # restrict tool set (also: all, none)
```

## Shebang scripts

Prompt files can be made executable so they run like a normal CLI tool. See the README for full examples.

```bash
#!/usr/bin/env -S fin -f
Summarize the files in the current directory
```

```bash
#!/usr/bin/env -S fin -yolo --max-turns 3 -f
Read all TODO comments in this project and create a summary
```

The leading `#!` line is stripped before the prompt is sent. Positional args after the script path are appended to the prompt; piped stdin is prepended.

## Configuration

TOML at `~/.config/fin/config.toml`:
- `[models]` — `primary` (main conversation model), `secondary` (cheap tasks like title generation)
- `[settings]` — `project_file` (default: AGENTS.md), `max_turns`, `approve`, `ui`
- `[model_aliases]` — short names mapping to `provider/model` (e.g. `sonnet = "anthropic/claude-sonnet-4-6"`)
- `[providers.*]` — `base_url`, `api_key_env`, `headers`
- `[tools.*]` — `approval` (auto/confirm/deny), `allow`/`deny` glob patterns for shell

## Features

### Multi-provider LLM support
Anthropic Claude, OpenAI, and any OpenAI-compatible API (Groq, OpenRouter, Ollama, local models). All via raw HTTP — no provider SDKs. Configurable per-provider base URLs, API keys, and custom headers.

### Built-in tools
- **read** — files with line numbers, images (base64 for vision models), directory trees
- **write** — creates files and parent directories
- **edit** — exact string replacement (old_string must be unique in the file)
- **shell** — executes via `sh -c`, returns stdout and stderr separately
- **use_skill** — activates a skill, loading its full instructions on demand
- **subagent** — spawns an isolated child agent for a task; child gets the same tools (minus subagent) and config, but a fresh conversation
- **compact** — summarizes the conversation into a new session, dropping older context

### Agent skills (agentskills.io spec)
Progressive disclosure: only skill names and descriptions are loaded at startup. Full instructions load on activation. Skills are discovered from `.agents/skills/` in the project (walks up to root) and `~/.agents/skills/` globally. Follows symlinks. Builtin skills are embedded in the binary.

### Session management
- Sessions saved incrementally as **JSONL** in `~/.local/share/fin/sessions/`. First line is a session header (id, title, model, cwd, started_at); each subsequent line is one message.
- Writes are append-only after the first save. Header changes (e.g. LLM-generated title) and resume both trigger an atomic `tmp + rename` full rewrite.
- mtime conflict detection: refuses to overwrite if another `fin` process modified the file since load, to avoid clobbering concurrent runs.
- Reader tolerates a truncated trailing line (crash mid-append) so earlier messages stay readable.
- UUID-based with prefix matching (`fin -s abc12` works). Named sessions via `-n`. Match recent sessions to the current prompt with `-match`.

### Export
- **JSON** — full session with all messages and metadata
- **HTML** — rendered markdown, foldable tool results, edit diffs with red/green, collapsible sections
- **message** — just the last assistant response (pipeable to `pbcopy`, `glow`, etc.)

### Output modes
- **default** — full ANSI-colored output with tool call display and results
- **minimal** — one-line tool summaries with line counts (e.g. `read main.go (47 lines)`)
- **quiet** — only the response text on stdout, nothing on stderr (for scripting)
- **debug** — like default plus turn timings, token usage, retry events

### Piped input
`git diff | fin "review this"` — stdin pipe detected automatically, content prepended to prompt.

### Tool approval system
Per-tool configurable: auto, confirm, or deny. Shell tool supports allow/deny glob patterns. `-approve all|safe|none` overrides at runtime; `-yolo` is shorthand for `-approve all`.

### Tool selection
`-tools` filters the active tool set. `all` (default) enables everything; `none` disables every tool; a comma list (`-tools read,shell`) enables only the named tools. Filter applies to subagents too. Valid names: `read, write, edit, shell, compact, use_skill, subagent`.

### Retry with backoff
Rate limits (429) and server errors (5xx) retried up to 3 times with exponential backoff + jitter.

### Layered system prompt
Assembled from: embedded base prompt → runtime context (date, OS, cwd) → skill list → `~/.agents/AGENTS.md` → project `AGENTS.md` (walks up to root).

### Live progress
Shows streaming line count during tool call argument generation (e.g. `write (47 lines)` updates in real-time). Esc / Ctrl+C cancels a turn.

### Type-ahead input
Raw-mode TTY multiplexer captures keystrokes during execution so the next prompt can be typed while a turn is still running.

## Exploring the source

Clone into a temporary directory and read through the files:

```sh
git clone https://github.com/meain/fin.git /tmp/fin-source
```

Then use the read tool on /tmp/fin-source to explore any file.

## Design principles

- Minimal dependencies: BurntSushi/toml, google/uuid, gopkg.in/yaml.v3, yuin/goldmark, golang.org/x/term
- Raw HTTP to all LLM providers — no provider SDKs
- Single binary, no config required to start
- Agent skills spec (agentskills.io) for extensibility
- System prompt and builtin skills are embedded markdown files
- Sessions stored as append-only JSONL so nothing is lost if killed mid-execution
- All terminal output flows through a single UI layer; callers pass structured data, the UI decides how to render (so the layer can be swapped for web, audio, etc.)
