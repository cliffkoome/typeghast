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
)

func main() {
	mode := flag.String("mode", "", "skip the menu: haunted or normal")
	demo := flag.Bool("demo", false, "haunted mode without an agent; pgup/pgdn sets the context")
	sessionPath := flag.String("session", "", "Claude Code transcript (.jsonl) to follow instead of choosing one")
	threshold := flag.Int("threshold", 0, "context size in tokens at which the haunting peaks (default: --haunt-at of the model's window)")
	hauntAt := flag.Float64("haunt-at", 0.4, "fraction of the context window at which the haunting peaks")
	maxGhosts := flag.Int("max-ghosts", 14, "number of ghosts on screen at the peak")
	model := flag.String("model", "", "Ollama model that narrates the agent's work (default: a Gemma model if installed, else any)")
	ollama := flag.String("ollama", "http://localhost:11434", "Ollama server URL")
	noModel := flag.Bool("no-model", false, "don't use a local model; type the agent's own words")
	flag.Parse()

	cfg := game.AppConfig{
		Base:        game.Config{Threshold: *threshold, HauntAt: *hauntAt, MaxGhosts: *maxGhosts},
		SessionPath: *sessionPath,
		Demo:        *demo,
	}
	switch *mode {
	case "":
	case "haunted":
		m := game.Haunted
		cfg.Mode = &m
	case "normal":
		m := game.Normal
		cfg.Mode = &m
	default:
		fmt.Fprintln(os.Stderr, "typeghast: --mode must be haunted or normal")
		os.Exit(2)
	}
	// A local model narrates when one is available; otherwise the game uses
	// the agent's own words and needs nothing installed.
	if !*noModel {
		if n, err := narrator.Detect(*ollama, *model); err == nil {
			cfg.Base.Narrator = n
		} else if *model != "" {
			fmt.Fprintln(os.Stderr, "typeghast:", err)
			os.Exit(1)
		}
	}

	final, err := tea.NewProgram(game.NewApp(cfg), tea.WithAltScreen()).Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "typeghast:", err)
		os.Exit(1)
	}
	// The alt screen is gone now, so these stay in the terminal to click.
	if app, ok := final.(*game.App); ok {
		if links := app.Links(); len(links) > 0 {
			fmt.Println("Links you unlocked:")
			for _, l := range links {
				fmt.Printf("  %s · %s\n    %s\n", l.Source, l.Text, l.URL)
			}
		}
	}
}
