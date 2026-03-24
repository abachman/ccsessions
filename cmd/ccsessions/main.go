package main

import (
	"fmt"
	"os"

	"ccsessions/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	model, err := ui.NewModel()
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
