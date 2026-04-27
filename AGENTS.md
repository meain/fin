# fin

Opinionated CLI agent harness in Go. Minimal dependencies, raw HTTP to LLM providers, streaming, TOML config.

## Architecture

Root package (`main`) handles CLI, agent loop, config, sessions, UI. Internal packages hold the reusable parts.

### Root files
- `main.go` — Entry point, flag parsing, session management, piped stdin
- `agent.go` — Agent loop (stream → tool calls → execute → repeat), retry with backoff, tool approval
- `config.go` — TOML config (`~/.config/fin/config.toml`), model alias resolution, validation
- `prompt.go` — System prompt assembly (embedded base + runtime context + skills + AGENTS.md layers)
- `session.go` — Incremental session persistence with UUID, title, per-message timestamps
- `matching.go` — Session auto-matching: keyword extraction, scoring (title 3x + content + recency), `FindMatchingSessions`
- `export.go` — Export as JSON, HTML (markdown rendering, foldable tool results, edit diffs), or last message
- `skill.go` — Skill discovery from .agents/skills/ (project + parents + global), follows symlinks, YAML frontmatter
- `ui.go` — Terminal output: 3 modes (default/minimal/quiet), ANSI colors, live progress, line counts
- `input.go` — x/term raw mode, stdin multiplexer for type-ahead during execution, Esc/Ctrl+C cancellation
- `embed.go` — Embeds `system_prompt.md` and `skills/` directory

### Internal packages
- `internal/types/` — Shared types: Message, ToolCall, StreamDelta, CompletionRequest, ToolDef, ToolResult, Image, ExpandHome
- `internal/provider/` — Provider interface + Anthropic (raw HTTP + SSE) and OpenAI-compatible implementations
- `internal/tool/` — Tool interface + read, write, edit, shell, skill tools

### Embedded files
- `system_prompt.md` — Base system prompt
- `skills/about-fin/SKILL.md` — Builtin skill describing fin itself

## Conventions

- Raw HTTP for all LLM providers — no provider SDKs
- Minimal deps: `BurntSushi/toml`, `google/uuid`, `gopkg.in/yaml.v3`, `yuin/goldmark`, `golang.org/x/term`
- Tools return `ToolResult` (Content + optional Images)
- Types shared across packages live in `internal/types/`
- `types.ExpandHome()` handles `~/` paths — use it in tools that accept file paths
- Piped stdin is detected and prepended to the prompt
- Rate limits (429) and server errors (5xx) retried with exponential backoff + jitter (max 3)
- ANSI escape codes directly — no color/TUI libraries
- System prompt and builtin skills are embedded markdown files

## CLI flags

```
fin "prompt"                    # run with prompt
fin -c "follow up"              # continue last session
fin -s <uuid> "follow up"      # continue specific session (prefix match)
fin -sessions                   # list last 10 sessions
fin -all -sessions              # list all sessions
fin -export json|html|message   # export session (uses -s for specific, else last)
fin -model provider/model       # override model
fin -ui default|minimal|quiet   # output mode
fin -yolo                       # auto-approve all tools
fin -match "prompt"             # search recent sessions, offer to continue matching one
```

## Config

TOML at `~/.config/fin/config.toml`:

- `[settings]` — `default_model`, `project_file`, `max_turns`, `yolo`, `ui`
- `[model_aliases]` — short names mapping to `provider/model`
- `[providers.*]` — `base_url`, `api_key_env`, `headers`
- `[tools.*]` — `approval` (auto/confirm/deny), `allow`/`deny` glob patterns for shell

## Adding a new provider

1. Create `internal/provider/name.go` implementing the `Provider` interface
2. Add a case in `New()` in `internal/provider/provider.go`
3. The provider must handle SSE streaming and convert to/from `types.Message` / `types.StreamDelta`

## Adding a new tool

1. Create `internal/tool/name.go` implementing the `Tool` interface (Name, Description, Parameters, Run returning ToolResult)
2. Add it to `BuiltinTools()` in `internal/tool/tool.go`
3. Add a default approval level in `defaultConfig()` in `config.go`
4. Add display handling in `ToolCallStart` and `toolCallMinimal` in `ui.go`
5. If it needs special HTML export rendering, add handling in `renderToolCall` in `export.go`

## Adding a builtin skill

1. Create `skills/<name>/SKILL.md` with YAML frontmatter (name, description) and markdown body
2. It gets embedded automatically via `embed.go` and loaded in `agent.go`'s `loadBuiltinSkills()`
