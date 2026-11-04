package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ivansevill/sevipomodoro/internal/store"
	"github.com/ivansevill/sevipomodoro/internal/tui"
)

const version = "0.1.0"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("sevipomodoro", version)
		return
	}

	fileStore, err := store.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, "sevipomodoro:", err)
		os.Exit(1)
	}
	state, loadErr := fileStore.Load()
	warning := ""
	if loadErr != nil {
		warning = loadErr.Error()
	}

	program := tea.NewProgram(tui.NewModel(state, fileStore, warning), tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "sevipomodoro:", err)
		os.Exit(1)
	}
}
