# fin

Opinionated CLI agent harness in Go

## Features

- **Multi-provider support**: Anthropic Claude, OpenAI, and any OpenAI-compatible APIs
- **Tool execution**: File operations, shell commands, and extensible skill system
- **Session persistence**: Resume conversations across CLI invocations
- **Export capabilities**: JSON, HTML, and message-only export formats
- **Flexible UI modes**: Default, minimal, and quiet output modes
- **Configurable approval**: Auto-approve, confirm, or deny tool execution per tool type

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
fin -continue "follow up"  # continue last session
fin -c "follow up"         # short form
fin -s <uuid> "follow up"  # continue specific session

# Export sessions
fin -export json           # export last session as JSON
fin -export html           # export last session as HTML
fin -export message        # export just the last response text
fin -s <uuid> -export html # export specific session

# Model selection
fin -model openai/gpt-4o "analyze this PR"

# Auto-approve all tools
fin -yolo "refactor this file"

# Output modes
fin -ui minimal "what is in go.mod"   # compact tool display
fin -ui quiet "summarize this file"   # only response text on stdout
```

### Examples

Minimal mode shows tool names, their key argument, and the response:

```
$ fin -ui minimal "what is in go.mod? be brief"
read go.mod
Go 1.25.7, minimal deps: BurntSushi/toml, google/uuid, yuin/goldmark, golang.org/x/term, gopkg.in/yaml.v3.
```

Export just the last assistant response with `-export message`:

```
$ fin -export message | pbcopy           # copy to clipboard
$ fin -export message | glow             # render markdown in terminal
$ fin "summarize this" && fin -export message > summary.txt
```

Skills activate automatically when the task matches. Here fin activates the `jira` skill to look up tickets:

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
[settings]
default_model = "anthropic/claude-sonnet-4-20250514"
project_file = "AGENTS.md"   # per-project instructions file
max_turns = 50                # max agent loop iterations
yolo = false                  # auto-approve all tools
ui = "default"                # "default", "minimal", or "quiet"

[model_aliases]
sonnet = "anthropic/claude-sonnet-4-20250514"
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

- **read**: Read file contents with optional line ranges
- **write**: Create or overwrite files
- **edit**: Surgical string replacement in files
- **shell**: Execute shell commands via `sh -c`
- **use_skill**: Activate agent skills for specialized workflows

## Skills System

Skills provide specialized agent workflows with progressive disclosure. Place skill definitions in:

- `~/.agents/skills/` (global skills)
- `.agents/skills/` (project-specific skills)

Each skill has a `SKILL.md` with triggers, description, and full instructions.

## Agent Instructions

The system prompt is assembled from:

1. Base agent instructions
2. `~/.agents/AGENTS.md` (global)
3. Project `.agents/AGENTS.md` 
4. Activated skills

## Session Storage

Conversations are saved to `~/.local/share/fin/sessions/` as JSON files with UUIDs. Sessions include full message history and can be resumed at any time.

## Tool Approval Levels

- `auto` — Execute without prompting
- `confirm` — Ask for approval (default)
- `deny` — Block execution

Configure per tool type in `config.toml`.

> What is with the name "fin":
> 1. **Easy to type** — Three letters, easy to remember, fast to invoke
> 2. **Final form** — This is the evolved, final form of my [esa agent](https://github.com/meain/esa)
