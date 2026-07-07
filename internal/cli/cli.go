// Package cli is the command-line entry point. It parses flags, builds the
// agent and UI, runs one user turn, and returns an exit code.
package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/meain/fin/internal/agent"
	"github.com/meain/fin/internal/approval"
	"github.com/meain/fin/internal/config"
	"github.com/meain/fin/internal/export"
	"github.com/meain/fin/internal/jsonui"
	"github.com/meain/fin/internal/provider"
	"github.com/meain/fin/internal/render"
	"github.com/meain/fin/internal/session"
	"github.com/meain/fin/internal/skill"
	"github.com/meain/fin/internal/tool"
	t "github.com/meain/fin/internal/types"
	"github.com/meain/fin/internal/ui"
	"golang.org/x/term"
)

// Run is the entry point. Pure imperative — main.go calls os.Exit(cli.Run()).
func Run() int {
	model := flag.String("model", "", "model to use (provider/model)")
	flag.StringVar(model, "m", "", "model to use (short)")
	configPath := flag.String("config", "", "path to config file")
	cont := flag.Bool("continue", false, "continue last session")
	flag.BoolVar(cont, "c", false, "continue last session (short)")
	sessionFlag := flag.String("session", "", "session UUID (for -continue or -export)")
	flag.StringVar(sessionFlag, "s", "", "session UUID (short)")
	name := flag.String("name", "", "named session (resumes if exists, creates if not)")
	flag.StringVar(name, "n", "", "named session (short)")
	sessions := flag.Bool("sessions", false, "list saved sessions")
	allSessions := flag.Bool("all", false, "show all sessions (with -sessions)")
	since := flag.String("since", "", "filter sessions by age: 1h, 2d, 1w (with -sessions)")
	exportFlag := flag.String("export", "", "export format: json, html, message")
	approve := flag.String("approve", "", "tool approval mode: all, safe, none")
	yolo := flag.Bool("yolo", false, "alias for -approve all")
	uiMode := flag.String("ui", "", "output mode: default, minimal, quiet, debug, json")
	match := flag.Bool("match", false, "search recent sessions and offer to continue a matching one")
	colorMode := flag.String("color", "auto", "color output: auto, always, never")
	maxTurns := flag.Int("max-turns", 0, "max agent loop iterations (overrides config)")
	promptFile := flag.String("f", "", "read prompt from file (for shebang scripts)")
	toolsFlag := flag.String("tools", "", "tools to enable: all, none, or comma-separated list (e.g. read,shell)")
	temp := flag.Bool("temp", false, "mark session as temporary (skipped by -c, -sessions shows [temp])")
	fork := flag.Bool("fork", false, "fork the current (or -s/-n) session into a new one and continue")
	secondaryModel := flag.String("secondary-model", "", "model for title generation (overrides config)")
	queue := flag.Bool("q", false, "queue a message into the running session's FIFO (uses positional args as message)")
	doctor := flag.Bool("doctor", false, "print diagnostic info: tools, models, skills, AGENTS.md files")
	flag.Parse()

	enabledTools, err := parseToolsFlag(*toolsFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}

	switch *colorMode {
	case "never":
		render.Disable()
	case "auto":
		if _, ok := os.LookupEnv("NO_COLOR"); ok || !term.IsTerminal(int(os.Stdout.Fd())) {
			render.Disable()
		}
	case "always":
		// colors stay enabled
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}

	if *doctor {
		printDoctor(cfg)
		return 0
	}

	if *sessions {
		limit := 10
		if *allSessions {
			limit = -1
		}
		var sinceTime time.Time
		if *since != "" {
			t, err := session.ParseSince(*since)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %s\n", err)
				return 1
			}
			sinceTime = t
		}
		printSessions(limit, sinceTime)
		return 0
	}

	loadSession := func() (*session.Session, error) {
		if *name != "" {
			return session.LoadByName(*name)
		}
		if *sessionFlag != "" {
			// Try as numeric index first
			if idx, err := strconv.Atoi(*sessionFlag); err == nil {
				return session.LoadByIndex(idx)
			}
			// Try as UUID prefix, then fall back to name
			if sess, err := session.LoadByID(*sessionFlag); err == nil {
				return sess, nil
			}
			return session.LoadByName(*sessionFlag)
		}
		if *temp {
			return session.LoadLastTemp()
		}
		return session.LoadLast()
	}

	if *queue {
		msg := strings.Join(flag.Args(), " ")
		if msg == "" {
			fmt.Fprintf(os.Stderr, "usage: fin -q \"message\"\n")
			return 1
		}
		sess, err := loadSession()
		if err != nil {
			fmt.Fprintf(os.Stderr, "queue: %v\n", err)
			return 1
		}
		fifoPath := fifoPathForSession(sess.ID)
		wf, err := os.OpenFile(fifoPath, os.O_WRONLY, 0)
		if err != nil {
			fmt.Fprintf(os.Stderr, "queue: no active session (FIFO not found: %s)\n", fifoPath)
			return 1
		}
		defer wf.Close()
		fmt.Fprintln(wf, msg)
		return 0
	}

	if *exportFlag != "" {
		sess, err := loadSession()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			return 1
		}
		switch *exportFlag {
		case "json":
			export.JSON(sess, os.Stdout)
		case "html":
			export.HTML(sess, os.Stdout)
		case "message":
			for i := len(sess.Messages) - 1; i >= 0; i-- {
				if sess.Messages[i].Role == t.RoleAssistant && sess.Messages[i].Content != "" {
					fmt.Println(sess.Messages[i].Content)
					return 0
				}
			}
			fmt.Fprintf(os.Stderr, "no assistant message found\n")
			return 1
		default:
			fmt.Fprintf(os.Stderr, "unknown export format: %s (use json, html, or message)\n", *exportFlag)
			return 1
		}
		return 0
	}

	approveMode := *approve
	if *yolo {
		approveMode = "all"
	}
	if approveMode == "" {
		approveMode = cfg.Settings.Approve
	}
	cfg.Settings.Approve = approveMode
	app := approval.Build(approveMode, cfg.Tools)

	if *maxTurns > 0 {
		cfg.Settings.MaxTurns = *maxTurns
	}

	modelExplicit := *model != ""
	modelStr := *model
	if modelStr == "" {
		modelStr = cfg.Models.Primary
	}
	if *secondaryModel != "" {
		cfg.Models.Secondary = *secondaryModel
	}

	outMode := ui.ParseOutputMode(cfg.Settings.UI)
	if *uiMode != "" {
		outMode = ui.ParseOutputMode(*uiMode)
	}

	// jsonMode drives a machine-readable JSONL frontend (GUI apps). stdin is
	// reserved for approval replies, so piped-input detection is skipped below.
	jsonMode := *uiMode == "json"

	// Auto-detect piped stdout: suppress chrome, only stream response text.
	// Explicit -ui flag overrides this.
	piped := *uiMode == "" && !term.IsTerminal(int(os.Stdout.Fd()))

	// newUI builds the active frontend. Both concrete UIs satisfy this.
	newUI := func() interface {
		agent.UIWriter
		Close()
	} {
		if jsonMode {
			return jsonui.New()
		}
		return ui.New(nil, outMode, piped)
	}

	skills := skill.Discover(cfg)

	args := flag.Args()

	var pipedInput string
	if !jsonMode {
		if stat, _ := os.Stdin.Stat(); stat.Mode()&os.ModeCharDevice == 0 {
			data, err := io.ReadAll(os.Stdin)
			if err == nil && len(data) > 0 {
				pipedInput = string(data)
			}
		}
	}

	// Resolve session first so we can inherit the model if needed.
	var resumedSession *session.Session
	var forkParentID string // set when -fork is used
	var sw *session.Writer
	if *name != "" {
		sess, err := session.LoadByName(*name)
		if err == nil {
			resumedSession = sess
		}
	} else if *cont || *sessionFlag != "" || *fork {
		sess, err := loadSession()
		if err != nil {
			u := newUI()
			u.Error(err.Error())
			u.Close()
			return 1
		}
		if *fork {
			// Fork: copy messages from origin but start a fresh session.
			forkParentID = sess.ID
			resumedSession = sess
		} else {
			resumedSession = sess
		}
	} else if *match && pipedInput == "" && len(args) > 0 {
		query := strings.Join(args, " ")
		resumedSession = promptSessionMatch(query, cfg.Settings.Matching)
	}

	// Inherit model from resumed session unless explicitly overridden.
	if resumedSession != nil && !modelExplicit && resumedSession.Model != "" {
		modelStr = resumedSession.Model
	}

	providerName, modelName := config.ResolveModel(modelStr, cfg)
	fullModel := providerName + "/" + modelName
	providerCfg, ok := cfg.Providers[providerName]
	if !ok {
		fmt.Fprintf(os.Stderr, "error: unknown provider %q\n", providerName)
		return 1
	}

	p, err := provider.New(providerName, provider.Config{
		BaseURL:   providerCfg.BaseURL,
		APIKeyEnv: providerCfg.APIKeyEnv,
		Headers:   providerCfg.Headers,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}

	// Determine session ID early so it can be included in the system prompt.
	sessionID := ""
	if resumedSession != nil && forkParentID == "" {
		sessionID = resumedSession.ID
	} else {
		sessionID = uuid.New().String()
	}

	u := newUI()
	defer u.Close()
	ag := agent.New(agent.NewProviderInjector(p, modelName), fullModel, cfg, app, u, skills, sessionID, enabledTools)

	if resumedSession != nil && forkParentID == "" {
		ag.SetMessages(resumedSession.Messages)
		sw = session.WriterForExisting(resumedSession)
		label := resumedSession.ID
		if resumedSession.Name != "" {
			label = resumedSession.Name
		}
		u.SessionInfo(agent.SessionInfoData{Resumed: true, Label: label, StartedAt: resumedSession.StartedAt})
	}
	if forkParentID != "" {
		// Fork: new session with parent link, messages copied from origin.
		sw = session.NewWriter(sessionID, fullModel, "", *temp)
		sw.SetPreviousSession(forkParentID)
		ag.SetMessages(resumedSession.Messages)
		u.SessionInfo(agent.SessionInfoData{Label: sessionID[:8] + " (fork of " + forkParentID[:8] + ")"})
	}
	if sw == nil {
		if *name != "" {
			sw = session.NewWriter(sessionID, fullModel, *name, *temp)
			u.SessionInfo(agent.SessionInfoData{Label: *name})
		} else {
			sw = session.NewWriter(sessionID, fullModel, "", *temp)
		}
	}
	var saveWarned bool
	ag.OnUpdate = func(msgs []t.Message) {
		if err := sw.Save(msgs); err != nil && !saveWarned {
			u.Error(fmt.Sprintf("session save: %v", err))
			saveWarned = true
		}
	}
	ag.OnCompact = func() {
		prevID := sw.ID()
		sw = session.NewWriter("", fullModel, "", false)
		sw.SetPreviousSession(prevID)
	}

	// Startup debug info
	{
		d := agent.DebugStartup{Model: fullModel, SessionID: sw.ID()}
		if len(skills) > 0 {
			d.Skills = make([]string, len(skills))
			for i, s := range skills {
				d.Skills[i] = s.Name
			}
		}
		if msgs := ag.Messages(); len(msgs) > 0 {
			d.PromptChars = len(msgs[0].Content)
		}
		u.Debug(d)
	}

	// Read prompt from file if -f is set (shebang support).
	var filePrompt string
	if *promptFile != "" {
		data, err := os.ReadFile(*promptFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			return 1
		}
		content := string(data)
		// Strip shebang line
		if strings.HasPrefix(content, "#!") {
			if idx := strings.Index(content, "\n"); idx >= 0 {
				content = content[idx+1:]
			}
		}
		filePrompt = strings.TrimSpace(content)
	}

	if len(args) == 0 && pipedInput == "" && filePrompt == "" {
		fmt.Fprintf(os.Stderr, "usage: fin [flags] \"prompt\"\n")
		flag.PrintDefaults()
		return 1
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Write session header immediately so -q can find this session by ID.
	_ = sw.Save(ag.Messages())

	fifoPath := fifoPathForSession(sw.ID())
	if err := syscall.Mkfifo(fifoPath, 0600); err != nil && !errors.Is(err, syscall.EEXIST) {
		fmt.Fprintf(os.Stderr, "queue: mkfifo: %v\n", err)
		return 1
	}
	defer os.Remove(fifoPath)
	queueCh := startFIFOReader(ctx, fifoPath)

	// Compose prompt: file base + positional args + piped stdin
	prompt := filePrompt
	if argPrompt := strings.Join(args, " "); argPrompt != "" {
		if prompt != "" {
			prompt = prompt + "\n\n" + argPrompt
		} else {
			prompt = argPrompt
		}
	}
	if pipedInput != "" {
		if prompt != "" {
			prompt = prompt + "\n\n" + pipedInput
		} else {
			prompt = pipedInput
		}
	}

	if err := ag.AddUserMessage(ctx, prompt); err != nil {
		u.Error(err.Error())
		u.Close()
		return 1
	}

loop:
	for {
		select {
		case msg, ok := <-queueCh:
			if !ok {
				break loop
			}
			fmt.Fprintf(os.Stderr, "\n[queued] %s\n\n", msg)
			if err := ag.AddUserMessage(ctx, msg); err != nil {
				u.Error(err.Error())
				u.Close()
				return 1
			}
		case <-ctx.Done():
			break loop
		default:
			break loop
		}
	}

	// Generate a descriptive session title in the background.
	// Forks always get a new title even though resumedSession is set.
	titleDone := make(chan struct{})
	if resumedSession == nil || forkParentID != "" {
		go func() {
			defer close(titleDone)
			titleCtx, titleCancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer titleCancel()
			parentTitle := ""
			if forkParentID != "" && resumedSession != nil {
				parentTitle = resumedSession.Title
			}
			if title, err := ag.GenerateTitle(titleCtx, parentTitle); err == nil && title != "" {
				sw.SetTitle(title)
				_ = sw.Save(ag.Messages())
			}
		}()
	} else {
		close(titleDone)
	}

	if outMode == ui.OutputDebug {
		usage := ag.Usage
		if usage.InputTokens > 0 || usage.OutputTokens > 0 {
			msgCount := 0
			for _, m := range ag.Messages() {
				if m.Role != t.RoleSystem {
					msgCount++
				}
			}
			u.Debug(agent.DebugSummary{Usage: &usage, Messages: msgCount})
		}
	}

	<-titleDone
	return 0
}

// printSessions renders the session list. Outputs JSON when stdout is not a
// terminal; otherwise renders a colored, fixed-width table with forks grouped
// under their parents. Pure data lives in session.LoadSummaries.
func printSessions(limit int, since time.Time) {
	sessions, total, err := session.LoadSummaries(limit, since)
	if err != nil || len(sessions) == 0 {
		fmt.Fprintf(os.Stderr, "no sessions found\n")
		return
	}

	if !term.IsTerminal(int(os.Stdout.Fd())) {
		data, _ := session.SummariesJSON(sessions)
		fmt.Println(string(data))
		return
	}

	// Build id -> session map, then load any parent sessions not in the list.
	byID := map[string]*session.Session{}
	for i := range sessions {
		byID[sessions[i].ID] = &sessions[i]
	}
	for _, sess := range sessions {
		cur := sess
		for cur.PreviousSession != "" {
			if _, ok := byID[cur.PreviousSession]; ok {
				break
			}
			parent, err := session.LoadByID(cur.PreviousSession)
			if err != nil {
				break
			}
			byID[parent.ID] = parent
			cur = *parent
		}
	}

	// Build children map and find roots.
	children := map[string][]*session.Session{}
	var roots []*session.Session
	for _, sess := range byID {
		if sess.PreviousSession == "" {
			roots = append(roots, sess)
		} else if _, ok := byID[sess.PreviousSession]; ok {
			children[sess.PreviousSession] = append(children[sess.PreviousSession], sess)
		} else {
			roots = append(roots, sess)
		}
	}

	// Sort roots and children by most recent activity.
	sort.Slice(roots, func(i, j int) bool {
		return session.LastMessageTime(*roots[i]).After(session.LastMessageTime(*roots[j]))
	})
	for k := range children {
		sort.Slice(children[k], func(i, j int) bool {
			return session.LastMessageTime(*children[k][i]).After(session.LastMessageTime(*children[k][j]))
		})
	}

	termWidth, _, _ := term.GetSize(int(os.Stdout.Fd()))
	if termWidth <= 0 {
		termWidth = 80
	}

	sessionTitle := func(sess *session.Session) string {
		title := sess.Title
		if title == "" {
			for _, m := range sess.Messages {
				if m.Role == t.RoleUser {
					title = m.Content
					break
				}
			}
		}
		return strings.ReplaceAll(title, "\n", " ")
	}

	idx := 1
	var printSession func(sess *session.Session, depth int)
	printSession = func(sess *session.Session, depth int) {
		title := sessionTitle(sess)

		msgCount := 0
		for _, m := range sess.Messages {
			if m.Role != t.RoleSystem {
				msgCount++
			}
		}

		age := render.RelativeTime(session.LastMessageTime(*sess))
		short := sess.ID[:8]
		if sess.Name != "" {
			short = fmt.Sprintf("%s [%s]", short, sess.Name)
		}
		if sess.Temp {
			short = short + render.Dim + " [temp]" + render.Reset
		}

		meta := fmt.Sprintf("(%s, %d msgs)", age, msgCount)
		indent := strings.Repeat("  ", depth)
		if depth > 0 {
			indent = strings.Repeat("  ", depth-1) + "↳ "
		}

		var counter string
		if depth == 0 {
			counter = fmt.Sprintf("%d.", idx)
			idx++
		} else {
			counter = strings.Repeat(" ", len(fmt.Sprintf("%d.", idx-1)))
		}

		overhead := len(counter) + len(indent) + len(short) + len(meta) + 3
		maxTitle := termWidth - overhead
		if maxTitle < 10 {
			maxTitle = 10
		}
		titleRunes := []rune(title)
		if len(titleRunes) > maxTitle {
			title = string(titleRunes[:maxTitle-1]) + "…"
		}

		fmt.Printf("%s%s%s %s%s %s %s%s%s\n",
			render.Dim, counter, render.Reset,
			indent, short, title,
			render.Dim, meta, render.Reset)

		for _, child := range children[sess.ID] {
			printSession(child, depth+1)
		}
	}

	for _, root := range roots {
		printSession(root, 0)
	}

	if limit > 0 && total > limit {
		fmt.Fprintf(os.Stderr, "\n%sshowing %d of %d sessions, use -all to see all%s\n", render.Dim, limit, total, render.Reset)
	}
}

// promptSessionMatch searches recent sessions for matches to the query and
// asks the user whether to continue one. Returns the chosen session or nil.
func promptSessionMatch(query string, mc config.MatchingConfig) *session.Session {
	const (
		searchLimit = 24
		minScore    = 1.5
		maxShow     = 3
	)

	matches := session.FindMatching(query, searchLimit, minScore, mc)
	if len(matches) == 0 {
		return nil
	}
	if len(matches) > maxShow {
		matches = matches[:maxShow]
	}

	if len(matches) == 1 {
		m := matches[0]
		age := render.RelativeTime(session.LastMessageTime(m.Session))
		fmt.Fprintf(os.Stderr, "%ssimilar session:%s %s %s(%s)%s\n",
			render.Dim, render.Reset, m.Session.Title, render.Dim, age, render.Reset)
		fmt.Fprintf(os.Stderr, "continue? [y/N] ")
		var input string
		fmt.Scanln(&input)
		if strings.ToLower(strings.TrimSpace(input)) == "y" {
			return &matches[0].Session
		}
		return nil
	}

	fmt.Fprintf(os.Stderr, "%ssimilar sessions:%s\n", render.Dim, render.Reset)
	for i, m := range matches {
		age := render.RelativeTime(session.LastMessageTime(m.Session))
		fmt.Fprintf(os.Stderr, "  %d. %s %s(%s)%s\n",
			i+1, m.Session.Title, render.Dim, age, render.Reset)
	}
	fmt.Fprintf(os.Stderr, "continue [1")
	for i := range matches[1:] {
		fmt.Fprintf(os.Stderr, "/%d", i+2)
	}
	fmt.Fprintf(os.Stderr, "/n]: ")

	var input string
	fmt.Scanln(&input)
	input = strings.ToLower(strings.TrimSpace(input))

	if input == "n" || input == "" {
		return nil
	}
	for i := range matches {
		if input == fmt.Sprintf("%d", i+1) {
			return &matches[i].Session
		}
	}
	return nil
}

// fifoPathForSession returns the FIFO path for a session by ID.
func fifoPathForSession(id string) string {
	return filepath.Join(os.TempDir(), "fin-"+id+".fifo")
}

// startFIFOReader opens the FIFO at path and returns a channel that receives
// one message per line. It holds the FIFO open across multiple external
// write+close cycles (O_RDWR prevents EOF between writers). The goroutine
// exits and the channel is closed when ctx is cancelled.
func startFIFOReader(ctx context.Context, path string) chan string {
	ch := make(chan string, 64)
	go func() {
		defer close(ch)
		// O_RDWR opens immediately (no blocking) and acts as both the read
		// end and a keep-alive write reference, preventing EOF between writers.
		f, err := os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			return
		}
		go func() { <-ctx.Done(); f.Close() }()

		sc := bufio.NewScanner(f)
		for sc.Scan() {
			msg := strings.TrimSpace(sc.Text())
			if msg == "" {
				continue
			}
			select {
			case ch <- msg:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch
}

// parseToolsFlag interprets the -tools flag value.
// Returns: nil set = all tools enabled; non-nil empty map = none; populated map = filter.
func parseToolsFlag(v string) (map[string]bool, error) {
	v = strings.TrimSpace(v)
	if v == "" || v == "all" {
		return nil, nil
	}
	if v == "none" {
		return map[string]bool{}, nil
	}

	valid := map[string]bool{}
	validNames := []string{}
	for _, tl := range tool.BuiltinTools() {
		valid[tl.Name()] = true
		validNames = append(validNames, tl.Name())
	}
	out := map[string]bool{}
	for _, name := range strings.Split(v, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if !valid[name] {
			return nil, fmt.Errorf("unknown tool %q (valid: %s)", name, strings.Join(validNames, ", "))
		}
		out[name] = true
	}
	if len(out) == 0 {
		return map[string]bool{}, nil
	}
	return out, nil
}
