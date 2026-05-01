#!/usr/bin/env bash
# Demo script for fin — run inside VHS to showcase features
# Each section echoes the command, runs it, then pauses for readability

set -e
cd "$(dirname "$0")"

# Build fin
go build -o /tmp/fin .
export PATH="/tmp:$PATH"

# Cleanup named session from previous runs
rm -f ~/.local/share/fin/sessions/*_named_demoproject.json

sep() {
  echo ""
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo ""
}

run() {
  echo -e "\033[1;33m\$ $*\033[0m"
  eval "$@"
  echo ""
  sleep 1
}

clear
echo "# fin: a minimal CLI agent harness"
echo ""
sleep 2

sep
echo "# 1. Simple prompt — just ask a question"
run fin "'what is the capital of france?'"

sep
echo "# 2. Pipe input — review code from stdin"
run 'echo "func add(a, b int) int { return a + b }" | fin "review this go function"'

sep
echo "# 3. Tool use — fin reads files to answer"
run fin "'read main.go and tell me what the entry point does in one sentence'"

sep
echo "# 4. Session management — list recent sessions"
run fin -sessions

sep
echo "# 5. Continue last session"
run fin -c "'and what flags does it support?'"

sep
echo "# 6. Named sessions — resume by name"
run fin -n demoproject "'summarize the go.mod dependencies'"

sep
echo "# 7. Quiet mode — just the answer, nothing else"
run fin -ui quiet "'what is 2+2?'"

sep
echo "# 8. Model override"
run fin -model anthropic/claude-sonnet-4-20250514 "'say hi in 5 words'"

sep
echo "# 9. Export session as HTML"
run 'fin -export html > /tmp/session.html && echo "exported to /tmp/session.html"'

sep
echo "# 10. Yolo mode — auto-approve all tool calls"
run fin -yolo "'list files in the current directory'"

sep
echo "# fin: minimal deps, raw HTTP, streaming, TOML config"
echo "# github.com/meain/fin"
sleep 3
