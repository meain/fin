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
	resume := flag.String("resume", "", "resume a specific session by UUID")
	sessions := flag.Bool("sessions", false, "list saved sessions")
	yolo := flag.Bool("yolo", false, "auto-approve all tool calls")
	flag.Parse()

	config, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}

	if *sessions {
		ListSessions()
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

	ui := NewUI()

	// Discover skills (progressive disclosure: only name+description loaded)
	skills := DiscoverSkills(config)
	if len(skills) > 0 {
		ui.Info(fmt.Sprintf("found %d skill(s)", len(skills)))
	}

	agent := NewAgent(provider, config, ui, skills)

	// The provider doesn't know the model — we inject it into requests.
	// We need to patch the agent to set the model on each request.
	// For now, wrap the provider to inject the model.
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

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// One-shot mode: fin "prompt"
	args := flag.Args()
	if len(args) > 0 {
		prompt := strings.Join(args, " ")
		if err := agent.AddUserMessage(ctx, prompt); err != nil {
			ui.Error(err.Error())
			os.Exit(1)
		}
		_ = SaveSession(modelStr, agent.Messages())
		return
	}

	// REPL mode
	ui.Info(fmt.Sprintf("fin — %s/%s", providerName, modelName))
	ui.Info("type /quit to exit")

	input := NewStdinInput()
	for {
		ui.UserPrompt()
		line, err := input.ReadLine("")
		if err != nil {
			if err == io.EOF {
				fmt.Fprintln(os.Stderr)
				break
			}
			ui.Error(err.Error())
			break
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == "/quit" || line == "/exit" {
			break
		}

		if err := agent.AddUserMessage(ctx, line); err != nil {
			ui.Error(err.Error())
			if ctx.Err() != nil {
				break
			}
		}
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
