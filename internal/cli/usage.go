package cli

import (
	"flag"
	"fmt"
	"os"
)

// usageGroup is a named cluster of related flags shown together in -h output.
type usageGroup struct {
	name  string
	flags []usageFlag
}

type usageFlag struct {
	names string // e.g. "-c, -continue"
	desc  string
}

var quickStart = []struct {
	cmd  string
	desc string
}{
	{`fin "prompt"`, "run with a prompt"},
	{`fin -c "follow up"`, "continue the last session"},
	{`fin -sessions`, "list recent sessions"},
	{`fin -yolo "prompt"`, "run without approval prompts"},
}

var usageGroups = []usageGroup{
	{
		name: "Session",
		flags: []usageFlag{
			{"-c, -continue", "continue last session"},
			{"-sessions", "list saved sessions"},
			{"-s, -session <uuid>", "continue/select session by UUID (prefix or index)"},
			{"-n, -name <name>", "named session (resumes if exists, creates if not)"},
			{"-fork", "fork the current (or -s/-n) session into a new one"},
			{"-match", "search recent sessions, offer to continue a matching one"},
			{"-f <file>", "read prompt from file (strips shebang line)"},
			{"-q", "queue a message (positional args) into the running session's FIFO"},
		},
	},
	{
		name: "Session filters",
		flags: []usageFlag{
			{"-tag, -t <tag>", "with -c/-sessions, filter by tag (prefix - to exclude)"},
			{"-repo", "with -c/-sessions, filter to sessions in the current repo"},
			{"-temp", "mark session temporary; with -sessions, show only temp sessions"},
			{"-since <dur>", "filter sessions by age: 1h, 2d, 1w (with -sessions)"},
			{"-running", "with -sessions, filter to sessions with a live process"},
			{"-all", "show all sessions (with -sessions)"},
		},
	},
	{
		name: "Model & behavior",
		flags: []usageFlag{
			{"-model, -m <model>", "model to use (provider/model, or alias)"},
			{"-secondary-model <model>", "model for title generation (overrides config)"},
			{"-approve <mode>", "tool approval mode: all, safe, none"},
			{"-yolo", "alias for -approve all"},
			{"-max-turns <n>", "max agent loop iterations (overrides config)"},
			{"-tools <list>", "tools to enable: all, none, or comma list (e.g. read,shell)"},
			{"-no-project", "skip project-specific AGENTS.md and skill directories"},
		},
	},
	{
		name: "Other",
		flags: []usageFlag{
			{"-config <path>", "path to config file"},
			{"-doctor", "print diagnostic info: tools, models, skills, AGENTS.md files"},
			{"-migrate", "rename existing session files to the current filename format"},
			{"-export <fmt>", "export format: json, html, message"},
			{"-ui <mode>", "output mode: default, minimal, quiet, debug, json"},
			{"-color <mode>", "color output: auto, always, never"},
		},
	},
}

// printUsage writes a grouped, hand-curated help listing. flag.PrintDefaults
// only offers a flat alphabetical dump, which gets hard to scan as flags grow;
// grouping by concern (session, model, io, ...) makes -h actually skimmable.
func printUsage() {
	w := os.Stderr
	fmt.Fprintln(w, "fin - opinionated CLI agent harness")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  fin [flags] \"prompt\"")

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Quick start:")
	qsWidth := 0
	for _, q := range quickStart {
		if len(q.cmd) > qsWidth {
			qsWidth = len(q.cmd)
		}
	}
	for _, q := range quickStart {
		fmt.Fprintf(w, "  %-*s  # %s\n", qsWidth, q.cmd, q.desc)
	}

	width := 0
	for _, g := range usageGroups {
		for _, f := range g.flags {
			if len(f.names) > width {
				width = len(f.names)
			}
		}
	}

	for _, g := range usageGroups {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "%s:\n", g.name)
		for _, f := range g.flags {
			fmt.Fprintf(w, "  %-*s  %s\n", width, f.names, f.desc)
		}
	}
}

func init() {
	flag.Usage = printUsage
}
