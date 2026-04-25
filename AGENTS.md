# fin

Opinionated CLI agent harness in Go. Minimal dependencies, raw HTTP to LLM providers, streaming, TOML config.

## Architecture

Single `main` package. All files are in the repo root.

- `main.go` — Entry point, flag parsing, one-shot execution
- `config.go` — TOML config (`~/.config/fin/config.toml`), model alias resolution
- `message.go` — Provider-agnostic message types (Message, ToolCall, StreamDelta, CompletionRequest). Each message has a Timestamp.
- `provider.go` — Provider interface, factory, header transport
- `provider_anthropic.go` — Anthropic Messages API (raw HTTP + SSE streaming)
- `provider_openai.go` — OpenAI-compatible API (raw HTTP + SSE streaming)
- `agent.go` — Agent loop: stream response, accumulate tool calls, execute, repeat
- `tool.go` — Tool interface, registry, AllTools()
- `tool_read.go` — Read file with line numbers, images (base64 for vision), or directory tree
- `tool_write.go` — Write/create files (expands `~`)
- `tool_edit.go` — Exact string replacement in files (expands `~`)
- `tool_shell.go` — Shell command execution via `sh -c`
- `tool_skill.go` — Activate agent skills (progressive disclosure)
- `skill.go` — Skill discovery (follows symlinks) and SKILL.md YAML frontmatter parsing
- `prompt.go` — System prompt assembly (base + ~/.agents/AGENTS.md + project AGENTS.md + skills)
- `session.go` — Conversation persistence to ~/.local/share/fin/sessions/ with UUID, title, timestamps
- `export.go` — Export sessions as JSON, HTML (with markdown rendering, diff views, foldable tool results), or last message
- `ui.go` — Terminal output with 3 modes (default/minimal/quiet), ANSI colors, streaming, tool call display, diffs
- `input.go` — Terminal input with x/term raw mode, stdin mux for type-ahead, Esc/Ctrl+C cancellation

## Conventions

- No sub-packages. Everything is `package main`.
- Raw HTTP for all LLM providers — no provider SDKs.
- Minimal deps: `BurntSushi/toml`, `google/uuid`, `gopkg.in/yaml.v3`, `yuin/goldmark`, `golang.org/x/term`. Avoid adding more.
- Tools return `ToolResult` (Content string + optional Images) — not bare strings.
- Piped stdin is detected and prepended to the prompt.
- Rate limit (429) and server errors (5xx) are retried with exponential backoff + jitter (max 3 retries).
- ANSI escape codes directly — no color/TUI libraries.
- Tools implement the `Tool` interface (Name, Description, Parameters, Run).
- Provider-agnostic message types in `message.go` — each provider converts to/from its own wire format.
- `expandHome()` in `config.go` handles `~/` paths — use it in tools that accept file paths.

## CLI flags

```
fin "prompt"                    # run with prompt
fin -c "follow up"              # continue last session
fin -s <uuid> "follow up"      # continue specific session (prefix match works)
fin -sessions                   # list last 10 sessions
fin -all -sessions              # list all sessions
fin -export json|html|message   # export session (uses -s for specific, else last)
fin -model provider/model       # override model
fin -ui default|minimal|quiet   # output mode
fin -yolo                       # auto-approve all tools
```

## Config

TOML at `~/.config/fin/config.toml`. Key sections:

- `[settings]` — `default_model`, `project_file`, `max_turns`, `yolo`, `ui`
- `[model_aliases]` — short names mapping to `provider/model`
- `[providers.*]` — `base_url`, `api_key_env`, `headers`
- `[tools.*]` — `approval` (auto/confirm/deny), `allow`/`deny` glob patterns for shell

## Adding a new provider

1. Create `provider_name.go` implementing the `Provider` interface
2. Add a case in `NewProvider()` in `provider.go`
3. The provider must handle SSE streaming and convert to/from `message.go` types

## Adding a new tool

1. Create `tool_name.go` implementing the `Tool` interface (Name, Description, Parameters, Run)
2. Add it to `AllTools()` in `tool.go`
3. Add a default approval level in `defaultConfig()` in `config.go`
4. Add display handling in `ToolCallStart` and `toolCallMinimal` in `ui.go`
5. If it needs special HTML export rendering, add handling in `renderToolCall` in `export.go`
