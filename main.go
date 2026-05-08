package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"

	"github.com/meain/fin/internal/provider"
	t "github.com/meain/fin/internal/types"
	"golang.org/x/term"
)

func main() {
	model := flag.String("model", "", "model to use (provider/model)")
	flag.StringVar(model, "m", "", "model to use (short)")
	configPath := flag.String("config", "", "path to config file")
	cont := flag.Bool("continue", false, "continue last session")
	flag.BoolVar(cont, "c", false, "continue last session (short)")
	session := flag.String("session", "", "session UUID (for -continue or -export)")
	flag.StringVar(session, "s", "", "session UUID (short)")
	name := flag.String("name", "", "named session (resumes if exists, creates if not)")
	flag.StringVar(name, "n", "", "named session (short)")
	sessions := flag.Bool("sessions", false, "list saved sessions")
	allSessions := flag.Bool("all", false, "show all sessions (with -sessions)")
	export := flag.String("export", "", "export format: json, html, message")
	approve := flag.String("approve", "", "tool approval mode: all, safe, none")
	yolo := flag.Bool("yolo", false, "alias for -approve all")
	uiMode := flag.String("ui", "", "output mode: default, quiet")
	match := flag.Bool("match", false, "search recent sessions and offer to continue a matching one")
	colorMode := flag.String("color", "auto", "color output: auto, always, never")
	flag.Parse()

	switch *colorMode {
	case "never":
		disableColors()
	case "auto":
		if _, ok := os.LookupEnv("NO_COLOR"); ok || !term.IsTerminal(int(os.Stderr.Fd())) {
			disableColors()
		}
	case "always":
		// colors stay enabled
	}

	config, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}

	if *sessions {
		limit := 10
		if *allSessions {
			limit = -1
		}
		ListSessions(limit)
		return
	}

	loadSession := func() (*Session, error) {
		if *name != "" {
			return LoadSessionByName(*name)
		}
		if *session != "" {
			// Try as numeric index first
			if idx, err := strconv.Atoi(*session); err == nil {
				return LoadSessionByIndex(idx)
			}
			// Try as UUID prefix, then fall back to name
			if sess, err := LoadSessionByID(*session); err == nil {
				return sess, nil
			}
			return LoadSessionByName(*session)
		}
		return LoadLastSession()
	}

	if *export != "" {
		sess, err := loadSession()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			os.Exit(1)
		}
		switch *export {
		case "json":
			ExportJSON(sess, os.Stdout)
		case "html":
			ExportHTML(sess, os.Stdout)
		case "message":
			for i := len(sess.Messages) - 1; i >= 0; i-- {
				if sess.Messages[i].Role == t.RoleAssistant && sess.Messages[i].Content != "" {
					fmt.Println(sess.Messages[i].Content)
					return
				}
			}
			fmt.Fprintf(os.Stderr, "no assistant message found\n")
			os.Exit(1)
		default:
			fmt.Fprintf(os.Stderr, "unknown export format: %s (use json, html, or message)\n", *export)
			os.Exit(1)
		}
		return
	}

	approveMode := *approve
	if *yolo {
		approveMode = "all"
	}
	if approveMode == "" {
		approveMode = config.Settings.AutoApprove
	}
	config.Settings.AutoApprove = approveMode
	if approveMode == "all" {
		for name := range config.Tools {
			tc := config.Tools[name]
			tc.Approval = "auto"
			config.Tools[name] = tc
		}
	}

	modelExplicit := *model != ""
	modelStr := *model
	if modelStr == "" {
		modelStr = config.Settings.DefaultModel
	}

	outMode := parseOutputMode(config.Settings.UI)
	if *uiMode != "" {
		outMode = parseOutputMode(*uiMode)
	}

	skills := DiscoverSkills(config)

	args := flag.Args()

	var pipedInput string
	if stat, _ := os.Stdin.Stat(); stat.Mode()&os.ModeCharDevice == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err == nil && len(data) > 0 {
			pipedInput = string(data)
		}
	}

	// Resolve session first so we can inherit the model if needed.
	var resumedSession *Session
	var sw *SessionWriter
	if *name != "" {
		sess, err := LoadSessionByName(*name)
		if err == nil {
			resumedSession = sess
		}
	} else if *cont || *session != "" {
		sess, err := loadSession()
		if err != nil {
			ui := NewUI(nil, outMode)
			ui.Error(err.Error())
			ui.Close()
			os.Exit(1)
		}
		resumedSession = sess
	} else if *match && pipedInput == "" && len(args) > 0 {
		query := strings.Join(args, " ")
		resumedSession = promptSessionMatch(query)
	}

	// Inherit model from resumed session unless explicitly overridden.
	if resumedSession != nil && !modelExplicit && resumedSession.Model != "" {
		modelStr = resumedSession.Model
	}

	providerName, modelName := resolveModel(modelStr, config)
	fullModel := providerName + "/" + modelName
	providerCfg, ok := config.Providers[providerName]
	if !ok {
		fmt.Fprintf(os.Stderr, "error: unknown provider %q\n", providerName)
		os.Exit(1)
	}

	p, err := provider.New(providerName, provider.Config{
		BaseURL:   providerCfg.BaseURL,
		APIKeyEnv: providerCfg.APIKeyEnv,
		Headers:   providerCfg.Headers,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}

	ui := NewUI(nil, outMode)
	defer ui.Close()
	agent := NewAgent(&modelInjector{provider: p, model: modelName}, fullModel, config, ui, skills)

	if resumedSession != nil {
		agent.SetMessages(resumedSession.Messages)
		sw = SessionWriterForExisting(resumedSession)
		label := resumedSession.ID
		if resumedSession.Name != "" {
			label = resumedSession.Name
		}
		ui.Info(fmt.Sprintf("resumed session %s (%s)", label, resumedSession.StartedAt.Format("2006-01-02 15:04")))
	}
	if sw == nil {
		if *name != "" {
			sw = NewSessionWriter(fullModel, *name)
			ui.Info(fmt.Sprintf("new session [%s]", *name))
		} else {
			sw = NewSessionWriter(fullModel, "")
		}
	}
	agent.OnUpdate = func(msgs []t.Message) {
		_ = sw.Save(msgs)
	}
	agent.OnCompact = func() {
		prevID := sw.id
		sw = NewSessionWriter(fullModel, "")
		sw.previousSession = prevID
	}

	if len(args) == 0 && pipedInput == "" {
		fmt.Fprintf(os.Stderr, "usage: fin [flags] \"prompt\"\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	prompt := strings.Join(args, " ")
	if pipedInput != "" {
		if prompt != "" {
			prompt = prompt + "\n\n" + pipedInput
		} else {
			prompt = pipedInput
		}
	}

	if err := agent.AddUserMessage(ctx, prompt); err != nil {
		ui.Error(err.Error())
		ui.Close()
		os.Exit(1)
	}

	if outMode == OutputDebug {
		u := agent.Usage
		if u.InputTokens > 0 || u.OutputTokens > 0 {
			usage := fmt.Sprintf("tokens: %d in, %d out", u.InputTokens, u.OutputTokens)
			if u.CacheReadInputTokens > 0 || u.CacheCreationInputTokens > 0 {
				usage += fmt.Sprintf(" (cache: %d read, %d write)", u.CacheReadInputTokens, u.CacheCreationInputTokens)
			}
			fmt.Fprintf(os.Stderr, "%s%s%s\n", dim, usage, reset)
		}
	}
}

// promptSessionMatch searches recent sessions for matches to the query and
// asks the user whether to continue one. Returns the chosen session or nil.
func promptSessionMatch(query string) *Session {
	const (
		searchLimit = 24
		minScore    = 1.5
		maxShow     = 3
	)

	matches := FindMatchingSessions(query, searchLimit, minScore)
	if len(matches) == 0 {
		return nil
	}
	if len(matches) > maxShow {
		matches = matches[:maxShow]
	}

	if len(matches) == 1 {
		m := matches[0]
		age := relativeTime(lastMessageTime(m.Session))
		fmt.Fprintf(os.Stderr, "%ssimilar session:%s %s %s(%s)%s\n",
			dim, reset, m.Session.Title, dim, age, reset)
		fmt.Fprintf(os.Stderr, "continue? [y/N] ")
		var input string
		fmt.Scanln(&input)
		if strings.ToLower(strings.TrimSpace(input)) == "y" {
			return &matches[0].Session
		}
		return nil
	}

	fmt.Fprintf(os.Stderr, "%ssimilar sessions:%s\n", dim, reset)
	for i, m := range matches {
		age := relativeTime(lastMessageTime(m.Session))
		fmt.Fprintf(os.Stderr, "  %d. %s %s(%s)%s\n",
			i+1, m.Session.Title, dim, age, reset)
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

// modelInjector wraps a Provider to inject the model name into every request.
type modelInjector struct {
	provider provider.Provider
	model    string
}

func (m *modelInjector) StreamCompletion(ctx context.Context, req t.CompletionRequest) (provider.Stream, error) {
	req.Model = m.model
	return m.provider.StreamCompletion(ctx, req)
}
