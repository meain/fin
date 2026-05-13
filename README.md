# fin

Opinionated CLI agent harness in Go

![demo](demo.gif)

## Features

- **Multi-provider support**: Anthropic Claude, OpenAI, and any OpenAI-compatible APIs via raw HTTP
- **Tool execution**: Read/write/edit files, shell commands, images (vision), directory trees
- **Agent skills**: [agentskills.io](https://agentskills.io) spec — progressive disclosure, project and global skills
- **Session persistence**: Incremental saves, resume, named sessions (`-n`), export as JSON/HTML/message, auto-generated titles via secondary model
- **Session matching**: `-match` searches recent sessions by keyword relevance and offers to continue one
- **Prompt caching**: Anthropic system prompt and tool definitions cached automatically
- **Token usage**: Input/output token counts and cache stats displayed after each run
- **Piped input**: `git diff | fin "review this"`
- **Shebang scripts**: `#!/usr/bin/env -S fin -f` — make prompt files executable
- **Output modes**: Default (full ANSI), minimal (one-line tool summaries), quiet (stdout only)
- **Configurable approval**: Per-tool auto/confirm/deny, glob patterns for shell commands
- **Retry with backoff**: Automatic retry on rate limits and server errors

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

# Export sessions
fin -export json           # export last session as JSON
fin -export html           # export as HTML with rendered markdown
fin -export message        # just the last response text
fin -s <uuid> -export html # export specific session
fin -n mycalc -export html # export named session

# Model selection
fin -model openai/gpt-4o "analyze this PR"

# Auto-approve all tools
fin -yolo "refactor this file"

# Limit agent loop iterations
fin --max-turns 3 "quick summary of this repo"

# Output modes
fin -ui minimal "what is in go.mod"   # compact tool display
fin -ui quiet "summarize this file"   # only response text on stdout

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

Minimal mode shows tool names, their key argument, and line counts:

```
$ fin "what is in go.mod? be brief"
read go.mod (14 lines)
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
ui = "default"                # "default", "minimal", or "quiet"

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
```

## Available Tools

- **read**: Read files (with line ranges), images (png/jpg/gif/webp for vision), or directory trees
- **write**: Create or overwrite files (creates parent directories)
- **edit**: Exact string replacement in files (old_string must be unique)
- **shell**: Execute commands via `sh -c` (stdout and stderr returned separately)
- **use_skill**: Activate agent skills for specialized workflows

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

Sessions are saved incrementally to `~/.local/share/fin/sessions/` as JSON files. Each session has a UUID, auto-generated title, and per-message timestamps. Sessions are saved after every agent turn so nothing is lost if killed mid-execution. Named sessions (`-n`) use a human-readable name embedded in the filename for fast lookup.

If `models.secondary` is configured, session titles are automatically generated by the secondary model after each conversation. Without it, the first user message (truncated to 50 chars) is used as the title.

> Why "fin"?
> 1. **Easy to type** — three letters, fast to invoke
> 2. **Final form** — evolved from my [esa agent](https://github.com/meain/esa)
