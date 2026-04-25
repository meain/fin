package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
)

func main() {
	model := flag.String("model", "", "model to use (provider/model)")
	configPath := flag.String("config", "", "path to config file")
	cont := flag.Bool("continue", false, "continue last session")
	flag.BoolVar(cont, "c", false, "continue last session (short)")
	resume := flag.String("resume", "", "resume a specific session by UUID")
	flag.StringVar(resume, "r", "", "resume a specific session by UUID (short)")
	sessions := flag.Bool("sessions", false, "list saved sessions")
	allSessions := flag.Bool("all", false, "show all sessions (with -sessions)")
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

	// Resume session if requested
	if *cont || *resume != "" {
		var sess *Session
		var err error
		if *resume != "" {
			sess, err = LoadSessionByID(*resume)
		} else {
			sess, err = LoadLastSession()
		}
		if err != nil {
			ui.Error(err.Error())
			os.Exit(1)
		}
		agent.SetMessages(sess.Messages)
		ui.Info(fmt.Sprintf("resumed session %s (%s)", sess.ID, sess.StartedAt.Format("2006-01-02 15:04")))
	}

	// Require a prompt
	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "usage: fin [flags] \"prompt\"\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	prompt := strings.Join(args, " ")
	if err := agent.AddUserMessage(ctx, prompt); err != nil {
		ui.Error(err.Error())
		os.Exit(1)
	}
	_ = SaveSession(modelStr, agent.Messages())
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
