# fin

Opinionated CLI agent harness in Go

![demo](https://github.com/user-attachments/assets/751eb81a-794e-4da2-9775-bbee956a62c9)

## Features

- **Multi-provider support**: Anthropic Claude, OpenAI, and any OpenAI-compatible APIs via raw HTTP
- **Tool execution**: Read/write/edit files, shell commands, images (vision), directory trees, subagents, conversation compaction
- **Agent skills**: [agentskills.io](https://agentskills.io) spec — progressive disclosure, project and global skills
- **Session persistence**: Incremental JSONL saves, resume, named sessions (`-n`), prefix-match UUIDs, export as JSON/HTML/message, auto-generated titles via secondary model
- **Session forking**: `-fork` branches a session at its last message — the new fork gets its own title, `parent_id` in JSON, and grouped display under the parent in `fin -sessions`; exports walk the full ancestor chain
- **Session matching**: `-match` searches recent sessions by keyword relevance (title-weighted + recency-decayed) and offers to continue one
- **Prompt caching**: Anthropic system prompt and tool definitions cached automatically
- **Token usage**: Input/output token counts and cache stats displayed in debug mode
- **Piped input**: `git diff | fin "review this"`
- **Shebang scripts**: `#!/usr/bin/env -S fin -f` — make prompt files executable
- **Output modes**: `default` (ANSI, streaming, parallel tool UI), `debug` (adds turn timings, token usage), `quiet` (stdout only for scripting)
- **Tool selection**: `-tools` restricts the active tool set (`all`, `none`, or `read,shell,...`)
- **Configurable approval**: Per-tool auto/confirm/deny, glob patterns for shell commands
- **Tool output spill**: Large tool outputs are written to `/tmp/fin/<id>.txt` with a truncated result pointing to the file — configurable via `max_output_bytes` per tool
- **Retry with backoff**: Automatic retry on rate limits and server errors
- **Replaceable UI**: agent talks to the UI through a small interface — a TUI, web, or audio frontend can drop in without touching the agent

## Installation

```bash
go install github.com/meain/fin@latest
```

## Usage

```bash
# Run with a prompt
fin "explain this code"

# Session management
fin -sessions              # list last 10 sessions
fin -all -sessions         # list all sessions
fin -c "follow up"         # continue last session
fin -s <uuid> "follow up"  # continue specific session (prefix match)
fin -n mycalc "what is 2+2"   # named session (creates if new, resumes if exists)
fin -n mycalc "what about 3+3" # continues the "mycalc" session
fin -match "fix the auth bug"  # find relevant past session and offer to continue it

# Fork sessions
fin -fork "try a different approach"       # fork last session into a new one
fin -s <uuid> -fork "try differently"      # fork a specific session

# Export sessions
fin -export json           # export last session as JSON
fin -export html           # export as HTML with rendered markdown
fin -export message        # just the last response text
fin -s <uuid> -export html # export specific session
fin -n mycalc -export html # export named session

# Model selection
fin -model openai/gpt-4o "analyze this PR"
fin -secondary-model anthropic/claude-haiku-4-5 "prompt"  # override title-generation model

# Auto-approve all tools
fin -yolo "refactor this file"

# Limit agent loop iterations
fin --max-turns 3 "quick summary of this repo"

# Output modes
fin -ui debug "what is in go.mod"     # default + turn timings + token usage
fin -ui quiet "summarize this file"   # only response text on stdout (good for scripting)

# Tool restrictions
fin -tools read,shell "list go files"   # only enable read and shell
fin -tools none "what can you do"       # no tools, just chat
fin -approve safe "fix the bug"         # auto-approve safe tools, prompt for destructive ones
fin --max-turns 3 "quick summary"       # cap agent loop iterations

# Piped input
git diff | fin "review this diff"
cat error.log | fin "what went wrong"
echo "func add(a, b int) string { return a + b }" | fin "fix this"

# Prompt from file (shebang support)
fin -f prompt.txt                            # read prompt from file
fin -f prompt.txt "focus on error handling"  # file prompt + extra args
echo "extra context" | fin -f prompt.txt     # file prompt + piped stdin
```

### Examples

Default mode streams the response while tool calls run in parallel:

```
$ fin "what is in go.mod? be brief"
read go.mod
Go 1.25.7, minimal deps: BurntSushi/toml, google/uuid, yuin/goldmark, golang.org/x/term, gopkg.in/yaml.v3.
```

Export the last response for use with other tools:

```
$ fin -export message | pbcopy           # copy to clipboard
$ fin -export message | glow             # render markdown in terminal
```

Shebang scripts — make prompt files directly executable:

```bash
#!/usr/bin/env -S fin -f
Summarize the files in the current directory
```

```bash
$ chmod +x summarize.fin
$ ./summarize.fin                          # run the prompt
$ ./summarize.fin "focus on Go files"      # append extra instructions
$ git diff | ./summarize.fin "review this" # pipe input works too
```

Bake in flags:

```bash
#!/usr/bin/env -S fin -yolo --max-turns 3 -f
Read all TODO comments in this project and create a summary
```

Skills activate automatically when the task matches:

```
$ fin "what tickets are assigned to me?"
use_skill jira
shell $ jira issue list -a "me" --plain
Your open tickets:

- **PROJ-421** Fix auth timeout on token refresh (In Progress, High)
- **PROJ-398** Add retry logic to webhook delivery (In Progress)
- **PROJ-445** Update API docs for v2 endpoints (To Do)
- **PROJ-412** Migrate config store to new schema (To Do)
```

## Configuration

Create `~/.config/fin/config.toml`:

```toml
[models]
primary = "anthropic/claude-sonnet-4-20250514"
secondary = "anthropic/claude-haiku-4-5"  # used for secondary tasks like title generation

[settings]
project_file = "AGENTS.md"   # per-project instructions file
max_turns = 50                # max agent loop iterations
approve = "safe"              # tool approval: "all", "safe", "none"
ui = "default"                # "default", "debug", or "quiet"

[settings.matching]
title_weight = 3              # weight for title hits in -match (default 3)
content_cap = 5               # cap on content hits per keyword (default 5)
recency_decay_d = 7           # recency decay window in days (default 7)
recency_bonus = 0.5           # max recency bonus multiplier (default 0.5)

[model_aliases]
sonnet = "anthropic/claude-sonnet-4-20250514"
haiku = "anthropic/claude-haiku-4-5"
gpt4 = "openai/gpt-4o"

[providers.anthropic]
base_url = "https://api.anthropic.com"
api_key_env = "ANTHROPIC_API_KEY"

[providers.openai]
base_url = "https://api.openai.com"
api_key_env = "OPENAI_API_KEY"

[tools.read]
approval = "auto"

[tools.write]
approval = "confirm"

[tools.edit]
approval = "confirm"

[tools.shell]
approval = "confirm"
allow = ["ls", "git status", "cat *"]
deny = ["rm -rf *", "sudo *"]
max_output_bytes = 51200  # spill to /tmp/fin/<id>.txt if output exceeds this
```

## Available Tools

- **read**: Read files (with line ranges), images (png/jpg/gif/webp for vision), or directory trees
- **write**: Create or overwrite files (creates parent directories)
- **edit**: Exact string replacement in files (old_string must be unique, with whitespace-mismatch hints)
- **shell**: Execute commands via `sh -c` (stdout and stderr returned separately, line-count streaming, graceful SIGTERM → SIGKILL on timeout)
- **use_skill**: Activate agent skills for specialized workflows
- **subagent**: Spawn an isolated child agent for a focused task (fresh conversation, same tools minus subagent)
- **compact**: Summarize the conversation into a new session and drop older context

## Skills

Skills follow the [agentskills.io](https://agentskills.io) spec with progressive disclosure — only names and descriptions load at startup, full instructions load on activation. Place skills in:

- `~/.agents/skills/` (global)
- `.agents/skills/` (project-specific, walks up to root)

Builtin skills are embedded in the binary (e.g. `about-fin`).

## Agent Instructions

The system prompt is assembled from layers:

1. Base prompt (embedded `system_prompt.md`)
2. Runtime context (date, OS, working directory)
3. Available skills (names + descriptions)
4. `~/.agents/AGENTS.md` (global user instructions)
5. Project `AGENTS.md` (walks up from cwd)

## Session Storage

Sessions are saved incrementally to `~/.local/share/fin/sessions/` as JSONL files. The first line is a session header (id, title, model, cwd, started_at); each subsequent line is one message. Writes are append-only after the first save; header changes (e.g. LLM-generated title) trigger an atomic `tmp + rename` full rewrite. mtime conflict detection refuses to clobber the file if another `fin` process has written to it since load.

Named sessions (`-n`) embed a human-readable name in the filename for fast lookup. UUID-based lookup works with any prefix (`fin -s abc12`).

If `models.secondary` is configured, session titles are automatically generated by the secondary model after each conversation. Without it, the first user message (truncated to 50 chars) is used as the title.

**Forking** (`-fork`) creates a new session with `previous_session` pointing to the origin, copies all messages, and generates a fresh title from the fork prompt. In `fin -sessions` (TTY), forks appear grouped under their parent with `↳` indentation; JSON output includes a `parent_id` field. Export walks the full ancestor chain root-first — HTML renders each session as a labeled section.

> Why "fin"?
> 1. **Easy to type** — three letters, fast to invoke
> 2. **Final form** — evolved from my [esa agent](https://github.com/meain/esa)
