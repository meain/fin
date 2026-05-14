# fin — opinionated CLI agent harness
# Render: `go install . && vhs demo.tape`  (writes demo.gif)
#
# The tape assumes `fin` is on PATH (run `go install .` first).
# Per-section timing is driven by `Wait+Line /❯/` which blocks until the
# next shell prompt appears, so we never have to guess Sleep durations.
# Setup (PS1, cleanup) is done inside Hide; only the `fin` commands are
# visible on screen.

Output demo.gif

Set Shell "bash"
Set FontFamily "Victor Mono"
Set FontSize 16
Set Width 1200
Set Height 700
Set Theme "Catppuccin Latte"
Set Padding 20
Set TypingSpeed 10ms

# --- Setup (hidden) ----------------------------------------------------------
Hide
Type `export PS1='\n❯ ' PROMPT_COMMAND= PS2='> '`
Enter
Wait+Line /❯/
Type "cd /Users/meain/dev/src/fin"
Enter
Wait+Line /❯/
Type "rm -f ~/.local/share/fin/sessions/*_named_demoproject.json"
Enter
Wait+Line /❯/
Type "clear"
Enter
Wait+Line /❯/
Show
Sleep 3s

# 1 · ask anything
Type `fin 'what is the capital of france?'  # ask anything`
Enter
Wait+Line /❯/
Sleep 3s

# 2 · tool use — read a file and answer
Type `fin 'read go.mod and tell me the go version in one short sentence'  # tool use`
Enter
Wait+Line /❯/
Sleep 3s

# 3 · pipe input — review code from stdin
Type `echo 'func add(a, b int) int { return a - b }' | fin 'spot the bug in one short sentence'  # pipe stdin`
Enter
Wait+Line /❯/
Sleep 3s

# 4 · minimal output mode
Type `fin -ui minimal 'count the lines in main.go using the read tool'  # minimal output`
Enter
Wait+Line /❯/
Sleep 3s

# 5 · list recent sessions
Type `fin -sessions  # list recent sessions`
Enter
Wait+Line /❯/
Sleep 3s

# 6 · continue last session
Type `fin -c 'in one short sentence, what did we just talk about?'  # continue last session`
Enter
Wait+Line /❯/
Sleep 3s

# 7 · named sessions — resume by name
Type `fin -n demoproject 'one-line summary of go.mod dependencies'  # named session`
Enter
Wait+Line /❯/
Sleep 3s

# 8 · model override — switch model per run
Type `fin -model haiku 'say hello in exactly five words'  # model override`
Enter
Wait+Line /❯/
Sleep 3s

# 9 · export last response (pipeable)
Type `fin -export message | head -3  # export last response`
Enter
Wait+Line /❯/
Sleep 3s

# 10 · shebang scripts — prompt files are executable
Hide
Type `cat > /tmp/summarize.fin <<'EOF'`
Enter
Type `#!/usr/bin/env -S fin -yolo --max-turns 3 -f`
Enter
Type `List the top-level files in this directory in one short line.`
Enter
Type `EOF`
Enter
Wait+Line /❯/
Type "chmod +x /tmp/summarize.fin"
Enter
Wait+Line /❯/
Show
Type `cat /tmp/summarize.fin  # shebang script — fin prompt file`
Enter
Wait+Line /❯/
Sleep 3s
Type `/tmp/summarize.fin  # run prompt file directly`
Enter
Wait+Line /❯/
Sleep 3s

# 11 · yolo — auto-approve all tools
Type `fin -yolo 'run shell to count .go files in this directory'  # auto-approve tools`
Enter
Wait+Line /❯/
Sleep 3s
