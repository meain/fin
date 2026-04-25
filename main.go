package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
)

func main() {
	model := flag.String("model", "", "model to use (provider/model)")
	configPath := flag.String("config", "", "path to config file")
	cont := flag.Bool("continue", false, "continue last session")
	flag.BoolVar(cont, "c", false, "continue last session (short)")
	session := flag.String("session", "", "session UUID (for -continue or -export)")
	flag.StringVar(session, "s", "", "session UUID (short)")
	sessions := flag.Bool("sessions", false, "list saved sessions")
	allSessions := flag.Bool("all", false, "show all sessions (with -sessions)")
	export := flag.String("export", "", "export format: json, html, message")
	yolo := flag.Bool("yolo", false, "auto-approve all tool calls")
	uiMode := flag.String("ui", "", "output mode: default, minimal, quiet")
	flag.Parse()

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

	// loadSession resolves a session from -session flag or falls back to last
	loadSession := func() (*Session, error) {
		if *session != "" {
			return LoadSessionByID(*session)
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
				if sess.Messages[i].Role == RoleAssistant && sess.Messages[i].Content != "" {
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

	if *yolo || config.Settings.Yolo {
		for name := range config.Tools {
			tc := config.Tools[name]
			tc.Approval = "auto"
			config.Tools[name] = tc
		}
	}

	modelStr := *model
	if modelStr == "" {
		modelStr = config.Settings.DefaultModel
	}

	providerName, modelName := resolveModel(modelStr, config)
	providerCfg, ok := config.Providers[providerName]
	if !ok {
		fmt.Fprintf(os.Stderr, "error: unknown provider %q\n", providerName)
		os.Exit(1)
	}

	provider, err := NewProvider(providerName, providerCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}

	// Determine output mode (flag overrides config)
	outMode := parseOutputMode(config.Settings.UI)
	if *uiMode != "" {
		outMode = parseOutputMode(*uiMode)
	}

	// Discover skills
	skills := DiscoverSkills(config)

	ui := NewUI(nil, outMode)
	agent := NewAgent(provider, config, ui, skills)
	agent.provider = &modelInjector{provider: provider, model: modelName}

	// Set up session writer
	var sw *SessionWriter
	if *cont || *session != "" {
		sess, err := loadSession()
		if err != nil {
			ui.Error(err.Error())
			os.Exit(1)
		}
		agent.SetMessages(sess.Messages)
		sw = SessionWriterForExisting(sess)
		ui.Info(fmt.Sprintf("resumed session %s (%s)", sess.ID, sess.StartedAt.Format("2006-01-02 15:04")))
	} else {
		sw = NewSessionWriter(modelStr)
	}
	agent.OnUpdate = func(msgs []Message) {
		_ = sw.Save(msgs)
	}

	// Build prompt from args + piped stdin
	args := flag.Args()

	var pipedInput string
	if stat, _ := os.Stdin.Stat(); stat.Mode()&os.ModeCharDevice == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err == nil && len(data) > 0 {
			pipedInput = string(data)
		}
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
		os.Exit(1)
	}
}

// modelInjector wraps a Provider to inject the model name into every request.
type modelInjector struct {
	provider Provider
	model    string
}

func (m *modelInjector) StreamCompletion(ctx context.Context, req CompletionRequest) (Stream, error) {
	req.Model = m.model
	return m.provider.StreamCompletion(ctx, req)
}
