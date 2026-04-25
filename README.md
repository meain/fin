# fin

Opinionated CLI agent harness in Go. Minimal dependencies, raw HTTP to LLM providers, streaming, TOML config.

## Features

- **Multi-provider support**: Anthropic Claude, OpenAI, and any OpenAI-compatible APIs
- **Streaming responses**: Real-time output with Server-Sent Events
- **Tool execution**: File operations, shell commands, and extensible skill system
- **Session persistence**: Resume conversations across CLI invocations
- **Zero external dependencies**: Raw HTTP, no provider SDKs
- **Configurable approval**: Auto-approve, confirm, or deny tool execution per tool type

## Installation

```bash
go build -o fin .
```

## Usage

```bash
# Run with a prompt
./fin "explain this code"

# Session management
./fin -sessions         # list all sessions
./fin -continue         # resume last session
./fin -resume <uuid>    # resume specific session

# Model selection
./fin -model anthropic/claude-3-sonnet "write a function"
./fin -model openai/gpt-4o "analyze this PR"

# Auto-approve all tools (use with caution)
./fin -yolo "refactor this file"

# Output modes
./fin -ui minimal "what is in go.mod"   # compact tool display
./fin -ui quiet "summarize this file"   # only response text on stdout
```

### Minimal mode

Minimal mode shows just tool names and their key argument, followed by the response text:

```
$ fin -ui minimal "what is in go.mod? be brief"
read go.mod
Go 1.25.7, minimal deps: toml for config, uuid for sessions, sys/term for terminal, yaml for parsing.
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

## Architecture

Single `main` package with focused responsibilities:

- `main.go` — CLI entry point
- `config.go` — TOML configuration and model aliases
- `provider_*.go` — LLM provider implementations
- `agent.go` — Tool execution loop
- `tool_*.go` — Individual tool implementations
- `skill.go` — Skill discovery and activation
- `session.go` — Conversation persistence
- `ui.go` — Terminal output and formatting

## Tool Approval Levels

- `auto` — Execute without prompting
- `confirm` — Ask for approval (default)
- `deny` — Block execution

Configure per tool type in `config.toml`.

## Contributing

This is a personal CLI tool optimized for a specific workflow. The architecture prioritizes:

- Minimal dependencies
- Raw HTTP over SDKs
- Single package simplicity
- Fast startup time

## License

MIT

## Why "fin"?

Two reasons:

1. **Easy to type** — Three letters, easy to remember, fast to invoke
2. **Final form** — This is the evolved, final form of my [esa agent](https://github.com/meain/esa)