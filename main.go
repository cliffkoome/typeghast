// Command typeghast is a terminal typing game haunted by your coding agent's
// context window.
package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cliffkoome/typeghast/internal/game"
	"github.com/cliffkoome/typeghast/internal/narrator"
	"github.com/cliffkoome/typeghast/internal/session"
)

func main() {
	demo := flag.Bool("demo", false, "simulate context usage instead of reading an agent session")
	sessionPath := flag.String("session", "", "Claude Code transcript (.jsonl) to follow (default: the most recently active session)")
	threshold := flag.Int("threshold", 400_000, "context size in tokens at which the haunting peaks")
	maxGhosts := flag.Int("max-ghosts", 14, "number of ghosts on screen at the threshold")
	model := flag.String("model", "", "Ollama model that narrates the agent's work (default: a Gemma model if installed, else any)")
	ollama := flag.String("ollama", "http://localhost:11434", "Ollama server URL")
	flag.Parse()

	cfg := game.Config{Threshold: *threshold, MaxGhosts: *maxGhosts}
	if !*demo {
		t := session.NewTracker(*sessionPath)
		if cfg.Initial = t.Poll(); cfg.Initial.Path == "" {
			cfg.Notice = "no Claude Code session found — running in demo mode"
		} else {
			cfg.Tracker = t
			n, err := narrator.Detect(*ollama, *model)
			if err != nil {
				cfg.Notice = "no narrator (" + err.Error() + ") — typing plain action lines"
			} else {
				cfg.Narrator = n
			}
		}
	}

	if _, err := tea.NewProgram(game.New(cfg), tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "typeghast:", err)
		os.Exit(1)
	}
}
