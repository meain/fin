# fin

Opinionated CLI agent harness in Go. Minimal dependencies, raw HTTP to LLM providers, streaming, TOML config.

## Architecture

Root package (`main`) handles CLI, agent loop, config, sessions, UI. Internal packages hold the reusable parts.

### Root files
- `main.go` — Entry point, flag parsing, session management, piped stdin
- `agent.go` — Agent loop (stream → tool calls → execute → repeat), retry with backoff, tool approval
- `config.go` — TOML config (`~/.config/fin/config.toml`), model alias resolution, validation
- `prompt.go` — System prompt assembly (embedded base + runtime context + skills + AGENTS.md layers)
- `session.go` — Incremental session persistence with UUID, title, named sessions, per-message timestamps
- `matching.go` — Session auto-matching: keyword extraction, scoring (title 3x + content + recency), `FindMatchingSessions`
- `export.go` — Export as JSON, HTML (markdown rendering, foldable tool results, edit diffs), or last message
- `skill.go` — Skill discovery from .agents/skills/ (project + parents + global), follows symlinks, YAML frontmatter
- `ui.go` — Terminal output: 3 modes (default/minimal/quiet), ANSI colors, live progress, line counts
- `input.go` — x/term raw mode, stdin multiplexer for type-ahead during execution, Esc/Ctrl+C cancellation
- `embed.go` — Embeds `system_prompt.md` and `skills/` directory

### Internal packages
- `internal/types/` — Shared types: Message, ToolCall, StreamDelta, Usage, CompletionRequest, ToolDef, ToolResult, Image, ExpandHome
- `internal/provider/` — Provider interface + Anthropic (raw HTTP + SSE) and OpenAI-compatible implementations
- `internal/tool/` — Tool interface + read, write, edit, shell, skill tools

### Embedded files
- `system_prompt.md` — Base system prompt
- `skills/about_fin/SKILL.md` — Builtin skill describing fin itself. **Keep this in sync** when adding/removing user-visible features (new tools, flags, persistence changes, config keys, output modes, etc.) — it is loaded into the LLM's context to answer "what is fin / what can fin do" questions, so stale entries actively mislead.

## Conventions

- Raw HTTP for all LLM providers — no provider SDKs
- Minimal deps: `BurntSushi/toml`, `google/uuid`, `gopkg.in/yaml.v3`, `yuin/goldmark`, `golang.org/x/term`
- Tools return `ToolResult` (Content + optional Images)
- Types shared across packages live in `internal/types/`
- `types.ExpandHome()` handles `~/` paths — use it in tools that accept file paths
- Piped stdin is detected and prepended to the prompt
- Rate limits (429) and server errors (5xx) retried with exponential backoff + jitter (max 3)
- All terminal output must go through the UI layer (`ui.go`) — never `fmt.Fprintf(os.Stderr, ...)` directly
- Never send pre-formatted complex data to the UI — pass structured types (e.g. `DebugData`, `RetryData`, `SessionInfoData`) and let the UI layer render. The UI must be replaceable (web, audio, etc.) so callers must not make formatting decisions. Simple static strings for `Info`/`Error` are fine.
- ANSI escape codes directly — no color/TUI libraries
- System prompt and builtin skills are embedded markdown files

## CLI flags

```
fin "prompt"                    # run with prompt
fin -c "follow up"              # continue last session
fin -s <uuid> "follow up"      # continue specific session (prefix match)
fin -n <name> "prompt"          # named session (resumes if exists, creates if not)
fin -sessions                   # list last 10 sessions
fin -all -sessions              # list all sessions
fin -export json|html|message   # export session (uses -s/-n for specific, else last)
fin -model provider/model       # override model
fin -ui default|debug|quiet     # output mode
fin -approve all                # auto-approve all tools (also: safe, none)
fin -yolo                       # alias for -approve all
fin -match "prompt"             # search recent sessions, offer to continue matching one
fin --max-turns 5 "prompt"     # limit agent loop iterations (overrides config)
fin -f script.fin               # read prompt from file (strips shebang line)
fin -f script.fin "extra args"  # file prompt + positional args appended
```

## Config

TOML at `~/.config/fin/config.toml`:

- `[models]` — `primary` (main conversation model), `secondary` (secondary tasks like title generation)
- `[settings]` — `project_file`, `max_turns`, `approve`, `ui`
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

## Generating the demo GIF

The README's demo is rendered with [vhs](https://github.com/charmbracelet/vhs) from `demo.tape`.

```
go install . && vhs demo.tape   # writes demo.gif
```

Notes on the tape:
- `go install .` first so the tape can call `fin` directly (no in-tape build).
- PS1 is set to `\n❯ ` and each command is followed by `Wait+Line /❯/`. VHS auto-advances when the prompt comes back, so per-command timing is driven by reality, not by guessed `Sleep`s. Leading `\n` keeps a blank line between command output and the next prompt. When adding new sections, keep the prompt marker in sync if you change PS1.
- Setup (PS1, cleanup) lives in a `Hide` block so only `fin` commands appear on screen. Commands end with an inline `# what it does` comment so the viewer knows what each one demonstrates.
- The render takes ~2–3 min and produces a multi-MB GIF. Don't open/load the GIF after generation — it's big enough to blow up tooling context.
- When adding a feature worth showing, add a new numbered section and re-run.

### Hosting the demo GIF

`demo.gif` is **gitignored** to keep the repo small. After regenerating, host it elsewhere and update the README link:

1. Open a comment on a tracking issue (e.g. https://github.com/meain/fin/issues/5) and drag-drop the GIF — GitHub uploads it to `user-attachments` and emits an `<img src="...">` snippet.
2. Grab the `src` URL from the comment (`gh api repos/meain/fin/issues/comments/<id> --jq '.body'`).
3. Replace the `![demo](...)` URL in `README.md`.

This avoids bloating the repo with a multi-MB binary on every regen.

## Debugging past runs

Use sessions to review how fin handled a task and identify agent behavior issues:

```
go run . -all -sessions              # list all sessions (grep to find by keyword)
go run . -s <uuid-prefix> -export json   # export full conversation as JSON
go run . -s <uuid-prefix> -export html   # export as readable HTML (good for sharing)
```

The JSON export contains every message (system, user, assistant, tool results) with timestamps and token usage. Look for:
- Repeated failed tool calls (especially edit failures from whitespace mismatches)
- Unnecessary tool calls (grep after already reading the full file)
- Excessive turns for simple tasks
- Verbose narration instead of just doing the work
