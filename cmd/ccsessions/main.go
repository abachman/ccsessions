package main

import (
	"flag"
	"fmt"
	"os"

	"ccsessions/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	claudeDir := flag.String("claude-dir", "", "Claude config directory to read session history from (defaults to ~/.claude)")
	debug := flag.Bool("debug", false, "Show Claude session discovery diagnostics")
	flag.Parse()

	model, err := ui.NewModel(*claudeDir, *debug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "startup error: %v\n", err)
		os.Exit(1)
	}

	program := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "runtime error: %v\n", err)
		os.Exit(1)
	}
}
