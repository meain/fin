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
fin -s <uuid> "follow up"             # continue specific session
git diff | fin "review this"           # piped input
fin -export html                       # export session as HTML
fin -export message | glow             # pipe last response to glow
fin -ui minimal "what is in go.mod"   # compact output
fin -ui quiet "summarize" > out.txt    # just the response on stdout
fin -sessions                          # list sessions
```

## Configuration

TOML at `~/.config/fin/config.toml`:
- `[settings]` — default_model, project_file (default: AGENTS.md), max_turns, yolo, ui mode
- `[model_aliases]` — short names (e.g. `sonnet = "anthropic/claude-sonnet-4-20250514"`)
- `[providers.*]` — base_url, api_key_env, headers
- `[tools.*]` — approval (auto/confirm/deny), allow/deny glob patterns for shell

## Features

### Multi-provider LLM support
Works with Anthropic Claude, OpenAI, and any OpenAI-compatible API (Groq, Together, local models). All via raw HTTP — no provider SDKs. Configurable per-provider base URLs, API keys, and custom headers.

### Built-in tools
- **read** — files with line numbers, images (base64 for vision models), directory trees
- **write** — creates files and parent directories
- **edit** — exact string replacement (old_string must be unique in the file)
- **shell** — executes via `sh -c`, returns stdout and stderr separately

### Agent skills (agentskills.io spec)
Progressive disclosure: only skill names and descriptions are loaded at startup. Full instructions load on activation. Skills are discovered from `.agents/skills/` in the project (walks up to root) and `~/.agents/skills/` globally. Follows symlinks. Builtin skills are embedded in the binary.

### Session management
- Sessions saved incrementally after every turn — nothing lost if killed mid-execution
- UUID-based with prefix matching (`fin -s abc12` works)
- Continue last session (`fin -c`) or a specific one (`fin -s <uuid>`)
- List sessions with relative timestamps and first-message preview

### Export
- **JSON** — full session with all messages and metadata
- **HTML** — rendered markdown, foldable tool results, edit diffs with red/green, collapsible sections
- **message** — just the last assistant response (pipeable to `pbcopy`, `glow`, etc.)

### Output modes
- **default** — full ANSI-colored output with tool call display and results
- **minimal** — one-line tool summaries with line counts (e.g. `read main.go (47 lines)`)
- **quiet** — only the response text on stdout, nothing on stderr (for scripting)

### Piped input
`git diff | fin "review this"` — stdin pipe detected automatically, content prepended to prompt.

### Tool approval system
Per-tool configurable: auto, confirm, or deny. Shell tool supports allow/deny glob patterns. Yolo mode auto-approves everything.

### Retry with backoff
Rate limits (429) and server errors (5xx) retried up to 3 times with exponential backoff + jitter.

### Layered system prompt
Assembled from: embedded base prompt → runtime context (date, OS, cwd) → skill list → `~/.agents/AGENTS.md` → project `AGENTS.md`

### Live progress
Shows streaming line count during tool call argument generation (e.g. `write (47 lines)` updates in real-time).

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
- Sessions saved incrementally so nothing is lost if killed mid-execution
