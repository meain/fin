# fin — opinionated CLI agent harness
# Pre-requisite: go build -o /tmp/fin .
# Run: vhs demo.tape

Output demo.gif

Set Shell "bash"
Set FontSize 16
Set Width 1200
Set Height 700
Set Theme "Catppuccin Mocha"
Set TypingSpeed 40ms
Set Padding 20

# --- Setup: clean prompt, PATH, cleanup ---
Hide
Type "export PS1='$ '"
Enter
Sleep 300ms
Type "export PATH=/tmp:/usr/bin:/bin:/usr/sbin:/sbin"
Enter
Sleep 300ms
Type "rm -f ~/.local/share/fin/sessions/*_named_demoproject.json"
Enter
Sleep 300ms
Type "cd /Users/meain/dev/src/fin"
Enter
Sleep 300ms
Type "clear"
Enter
Sleep 500ms
Show

# --- Title ---
Type "# fin: a minimal CLI agent harness"
Enter
Sleep 1.5s
Type "clear"
Enter
Sleep 300ms

# --- 1. Simple prompt ---
Type "fin 'what is the capital of france?'"
Enter
Sleep 5.5s
Type "clear"
Enter
Sleep 300ms

# --- 2. Pipe input ---
Type "echo 'func add(a, b int) int { return a + b }' | fin 'review this go function'"
Enter
Sleep 13s
Type "clear"
Enter
Sleep 300ms

# --- 3. Tool use: reading files ---
Type "fin 'read main.go and tell me what the entry point does in one sentence'"
Enter
Sleep 7s
Type "clear"
Enter
Sleep 300ms

# --- 4. Session management ---
Type "fin -sessions"
Enter
Sleep 3s
Type "clear"
Enter
Sleep 300ms

# --- 5. Continue a session ---
Type "fin -c 'and what flags does it support?'"
Enter
Sleep 15s
Type "clear"
Enter
Sleep 300ms

# --- 6. Named sessions ---
Type "fin -n demoproject 'summarize the go.mod dependencies'"
Enter
Sleep 8.5s
Type "clear"
Enter
Sleep 300ms

# --- 7. Quiet mode (just the answer) ---
Type "fin -ui quiet 'echo hello world'"
Enter
Sleep 4.5s
Type "clear"
Enter
Sleep 300ms

# --- 8. Model override ---
Type "fin -model anthropic/claude-sonnet-4-20250514 'say hi in 5 words'"
Enter
Sleep 3s
Type "clear"
Enter
Sleep 300ms

# --- 9. Export session as HTML ---
Type "fin -export html > /tmp/session.html && echo 'exported!'"
Enter
Sleep 2s
Type "clear"
Enter
Sleep 300ms

# --- 10. Yolo mode (auto-approve all tools) ---
Type "fin -yolo 'list files in the current directory'"
Enter
Sleep 12s
Type "clear"
Enter
Sleep 300ms

# --- Fin ---
Type "# fin: minimal deps, raw HTTP, streaming, TOML config"
Enter
Type "# github.com/meain/fin"
Enter
Sleep 3s
