# fin

Opinionated CLI agent harness in Go. Minimal dependencies, raw HTTP to LLM providers, streaming, TOML config.

## Architecture

`main.go` is a 10-LOC entry point that calls `cli.Run()`. Everything else lives under `internal/`. The agent never references the UI directly — all rendering goes through `agent.UIWriter`, which the terminal `ui` package implements. A future TUI or web frontend can swap in by implementing the same interface.

```
main.go                              # os.Exit(cli.Run())
internal/
  cli/         cli.go                # flag parsing, session glue, the user-turn driver, -sessions/-export/-match dispatch
               doctor.go             # -doctor: print diagnostic summary of tools, models, skills, AGENTS.md, providers
  agent/       agent.go              # Agent type, New(), AddUserMessage, Messages, SetMessages
               loop.go               # run(), runTurn, approveAll, runToolsParallel, detectCompactSummary, appendToolResults, consumeStream, approveTool
               retry.go              # streamWithRetry, retryDelay (exponential backoff + jitter on 429/5xx)
               title.go              # GenerateTitle via Models.Secondary
               subagent.go           # runSubagent (fresh convo, nullUI, subagent tool callback)
               tools.go              # BuildTools(skills, enabled, includeSubagent), filterTools, loadBuiltinSkills
               provider_factory.go   # NewProviderInjector (wraps provider w/ fixed model), newProviderForModel
               uiwriter.go           # UIWriter interface + Debug* payloads + SessionInfoData + RetryData + nullUI
  ui/          ui.go                 # Terminal UIWriter impl: ANSI codes, cursor moves, parallel-tool state, debug renderer, approval prompt
  export/      export.go             # JSON, HTML (with markdown rendering, foldable tool results, edit diffs), shared renderMessage for main + subagent
  session/     session.go            # Session, sessionHeader (twin kept — append perf), TitleFromFirstMessage, LastMessageTime
               writer.go             # Writer, NewWriter, WriterForExisting, Save, fullRewrite, appendNew, ErrConflict (mtime guard)
               reader.go             # readFile (tolerates truncated trailing line), parseFiles (parallel), uuidFromFilename
               store.go              # entries, LoadByID/Index/Name/Last/Chain, LoadLastWithFilter, LoadSummaries, SummariesJSON, ParseSince
               filename.go           # buildFilename/parseFilename: <timestamp>---<uuid>---<repo>---<name>---<temp>.jsonl
               migrate.go            # Migrate: renames legacy-format session files, backfills header Repo to match filename
               match.go              # FindMatching, scoreSession, extractKeywords, stopWords
  skill/       skill.go              # Skill, ParseMD, Discover (walks up cwd, then ~), scanDir
  prompt/      prompt.go             # BuildSystem (gates base prompt by enabled tools)
               project.go            # readAgentsMD, findProjectFile (uses fsutil.WalkUpFromCwd)
               claude_memory.go      # ClaudeMemoryDir/Path, readClaudeMemory (picks up Claude Code's auto-memory MEMORY.md)
  config/      config.go             # Config + sub-structs, Default, Load, validate, applyMatchingDefaults
               resolve.go            # ResolveModel (alias chain + provider/model split)
               paths.go              # HomeDir, ConfigPath, SessionPath, AgentsDir/File, SkillFile, SkillsDirName
  approval/    approval.go           # Approval, Build, AutoApprove, ForSubagent (subagent restriction in safe mode)
  input/       input.go              # x/term raw mode, stdin multiplexer, Esc/Ctrl+C cancel — consumed only by ui
  render/      ansi.go               # ANSI vars + Disable()
               text.go               # forEachVisibleRune, VisibleLen, Truncate
               time.go               # FormatElapsed, RelativeTime
               usage.go              # FormatUsage
  fsutil/      walkup.go             # WalkUpFromCwd
               expand.go             # ExpandHome
  embed/       embed.go              # //go:embed of system_prompt.md, skills/, hljs.* (all assets live here)
  provider/    provider.go           # Provider, Stream, Config, New, APIError
               anthropic.go          # SSE-streaming Anthropic impl; Recv split per event with decodeOrSkip[T]
               openai.go             # Newline-delimited OpenAI-compatible impl
  tool/        tool.go               # Tool interface, BuiltinTools, SubagentTools, Find, Defs
               label.go              # ToolLabel{Primary, Detail}, Labeler interface, LabelFor (used by ui + export)
               read.go, write.go, edit.go, shell.go, skill.go, subagent.go, compact.go
  types/       types.go              # Message, ToolCall, ToolCallDelta, StreamDelta, Usage, CompletionRequest, ToolDef, ToolResult, Image, Role
```

### Dependency graph (acyclic)

```
types  embed  fsutil  render          (leaves)
config → fsutil
input  → render
approval → config
skill  → config, fsutil
prompt → skill, embed, config, fsutil
tool   → types
provider → types
session → types, config, fsutil
agent  → types, provider, tool, approval, prompt, skill, config, embed
export → types, session, tool, embed
ui     → types, render, input, tool, agent      (for Debug* payload types only)
cli    → all of the above except types/render/fsutil leaves
main   → cli
```

`agent` does not import `ui`/`input`/`export`/`session`. `session` and `export` do not import `ui`. Enforced by:

```
go list -deps ./internal/agent | grep -E 'internal/(ui|input|export|session)'   # zero hits
go list -deps ./internal/session | grep internal/ui                              # zero hits
grep -rn '"\\033\[' --include='*.go' | grep -v 'internal/(render|ui|input)'      # zero hits
```

### Embedded assets

Live under `internal/embed/`:
- `system_prompt.md` — base system prompt
- `skills/about_fin/SKILL.md` — builtin skill describing fin itself. **Keep in sync** when adding/removing user-visible features — it is loaded into the LLM's context to answer "what is fin / what can fin do" questions; stale entries actively mislead.
- `hljs.min.js`, `hljs.min.css` — bundled into HTML export

## Documentation checklist

When adding a user-visible feature, update all three places:
- **`AGENTS.md` CLI flags section** — add the new flag(s) with a comment
- **`internal/embed/skills/about_fin/SKILL.md`** — update the usage block and any relevant feature description; this file is loaded into the LLM context so stale entries actively mislead
- **`internal/embed/skills/about_fin/SKILL.md` session management / feature section** — update prose descriptions if the feature touches sessions or a major subsystem

## Conventions

- Raw HTTP for all LLM providers — no provider SDKs
- Minimal deps: `BurntSushi/toml`, `google/uuid`, `gopkg.in/yaml.v3`, `yuin/goldmark`, `golang.org/x/term`
- Tools return `types.ToolResult` (Content + optional Images + optional SubMessages)
- Types shared across packages live in `internal/types/`
- `fsutil.ExpandHome()` handles `~/` paths — use in tools that accept file paths
- Piped stdin is detected and prepended to the prompt
- Rate limits (429) and server errors (5xx) retried with exponential backoff + jitter (max 3)
- **UI swappability**: agent talks to UI only through `agent.UIWriter`. Payloads crossing the boundary are structured data — no ANSI escapes, no pre-formatted strings, no terminal-width assumptions. `tool.ToolLabel` is data; ui renders. `Debug*` types, `SessionInfoData`, `RetryData` are structured. A future web UI implements the same interface; no agent change needed.
- ANSI escape sequences live **only** in `internal/render`, `internal/ui`, `internal/input`. Any other package emitting `\033[` is a layering bug.
- All terminal output from the agent flows through `UIWriter`. Pre-UI setup errors in `cli` may still write to stderr directly (no UI exists yet).
- ANSI escape codes used directly — no color/TUI libraries
- System prompt and builtin skills are embedded markdown files in `internal/embed/`

## CLI flags

```
fin "prompt"                    # run with prompt
fin -c "follow up"              # continue last session
fin -s <uuid> "follow up"       # continue specific session (prefix match or numeric index)
fin -n <name> "prompt"          # named session (resumes if exists, creates if not)
fin -sessions                   # list last 10 sessions (JSON if piped, ANSI table on TTY)
fin -all -sessions              # list all sessions
fin -since 1h -sessions         # filter sessions by age (1h, 2d, 1w, 30m)
fin -export json|html|message   # export session (uses -s/-n for specific, else last)
fin -model provider/model       # override model (alias names from [model_aliases] work)
fin -ui default|debug|quiet     # output mode
fin -approve all                # auto-approve all tools (also: safe, none)
fin -yolo                       # alias for -approve all
fin -match "prompt"             # search recent sessions, offer to continue matching one
fin --max-turns 5 "prompt"      # limit agent loop iterations (overrides config)
fin -f script.fin               # read prompt from file (strips shebang line)
fin -f script.fin "extra args"  # file prompt + positional args appended
fin -tools read,shell "prompt"  # restrict tool set (all, none, or comma list)
fin -color auto|always|never    # color output
fin -config <path>              # override config file path
fin -fork "prompt"              # fork the last session into a new one, continue with prompt
fin -s <uuid> -fork "prompt"    # fork a specific session
fin -secondary-model provider/model "prompt"  # override secondary model (title generation)
fin -temp "quick question"       # mark session as temporary (skipped by -c, shown as [temp])
fin -c -temp "follow up"        # continue the last temp session
fin -sessions -temp             # list only temp sessions
fin -tag work "prompt"          # tag session as "work"
fin -c -t work "follow up"      # continue last session tagged "work"
fin -c -t -work "follow up"     # continue last session NOT tagged "work"
fin -sessions -t work           # list sessions tagged "work"
fin -sessions -t -work          # list sessions NOT tagged "work"
fin -c -repo "follow up"        # continue last session created in the current repo
fin -sessions -repo             # list sessions created in the current repo
fin -q message words here       # queue a message into the last running session's FIFO (injected between turns, not just at the end)
fin -q -s <uuid> message        # queue into a specific session
fin -q -n <name> message        # queue into a named session
pbpaste | fin -q "message"      # queued message can include piped stdin, same as fin/fin -c
fin -sessions -running          # list only sessions with a live process (FIFO has an active reader)
fin -doctor                     # print diagnostic summary: tools, models, skills, AGENTS.md files, providers, sessions
fin -migrate                    # rename existing session files to the current filename format, backfilling repo where possible
fin -no-project "prompt"        # skip project-specific AGENTS.md and skill directories (global-only context)
fin -h                          # grouped help output (also shown when run with no prompt/stdin/-f)
```

## Config

TOML at `~/.config/fin/config.toml`:

- `[models]` — `primary` (main conversation model), `secondary` (title generation and any secondary tasks)
- `[settings]` — `project_file`, `max_turns`, `approve`, `ui`, `disable_claude_memory`, `skills_dirs` (extra directories to scan for skills, each holding `<name>/SKILL.md` subdirs)
- `[settings.matching]` — `title_weight` (default 3), `content_cap` (default 5), `recency_decay_d` (default 7), `recency_bonus` (default 0.5)
- `[model_aliases]` — short names mapping to `provider/model` (alias chains resolved up to 10 hops)
- `[providers.*]` — `base_url`, `api_key_env`, `headers`
- `[tools.*]` — `approval` (auto/confirm/deny), `allow`/`deny` glob patterns for shell, `max_output_bytes` (spill large output to `/tmp/fin/<id>.txt`)

## Adding a new provider

1. Create `internal/provider/name.go` implementing the `Provider` interface
2. Add a case in `New()` in `internal/provider/provider.go`
3. Provider handles streaming (SSE or NDJSON or whatever the upstream uses) and converts to/from `types.Message` / `types.StreamDelta`

## Adding a new tool

1. Create `internal/tool/name.go` implementing the `Tool` interface (Name, Description, Parameters, Run returning ToolResult)
2. Add it to `BuiltinTools()` in `internal/tool/tool.go`
3. Add a default approval level in `Default()` in `internal/config/config.go`
4. If the tool has a non-trivial display, implement `Labeler` (returns `ToolLabel{Primary, Detail}`) and register it in `labelers` in `internal/tool/label.go`. Both terminal UI and HTML export pick this up automatically.
5. If it needs special HTML export rendering, add handling in `renderToolCall` in `internal/export/export.go`

## Adding a builtin skill

1. Create `internal/embed/skills/<name>/SKILL.md` with YAML frontmatter (name, description) and markdown body
2. It's embedded automatically via `internal/embed/embed.go` and loaded by `agent.loadBuiltinSkills`

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
go run . -all -sessions                  # list all sessions
go run . -s <uuid-prefix> -export json   # export full conversation as JSON
go run . -s <uuid-prefix> -export html   # export as readable HTML (good for sharing)
```

The JSON export contains every message (system, user, assistant, tool results) with timestamps and token usage. Look for:
- Repeated failed tool calls (especially edit failures from whitespace mismatches)
- Unnecessary tool calls (grep after already reading the full file)
- Excessive turns for simple tasks
- Verbose narration instead of just doing the work
