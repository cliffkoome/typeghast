package session

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

type block struct {
	Type  string `json:"type"`
	ID    string `json:"id"`
	Text  string `json:"text"`
	Name  string `json:"name"`
	Input struct {
		FilePath    string `json:"file_path"`
		NewString   string `json:"new_string"`
		Content     string `json:"content"`
		Command     string `json:"command"`
		Description string `json:"description"`
		Todos       []struct {
			Content string `json:"content"`
			Status  string `json:"status"`
		} `json:"todos"`
	} `json:"input"`
}

// parseEvents pulls the meaningful actions out of an assistant message.
// Reads and searches are skipped: they say what the agent looked at, not what
// it did. The transcript can record a message more than once, so blocks
// already in seen are skipped.
func parseEvents(content json.RawMessage, seen map[string]bool) []Event {
	var blocks []block
	if json.Unmarshal(content, &blocks) != nil {
		return nil
	}
	var out []Event
	for _, b := range blocks {
		key := b.ID
		if key == "" {
			key = "text:" + b.Text
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		in := b.Input
		switch {
		case b.Type == "text" && strings.TrimSpace(b.Text) != "":
			out = append(out, Event{"said", clip(b.Text, 300)})
		case b.Type != "tool_use":
		case b.Name == "Edit" || b.Name == "MultiEdit":
			out = append(out, Event{"edit", fmt.Sprintf("edited %s: %s", short(in.FilePath), clip(in.NewString, 240))})
		case b.Name == "Write":
			out = append(out, Event{"write", fmt.Sprintf("wrote %s: %s", short(in.FilePath), clip(in.Content, 240))})
		case b.Name == "Bash":
			what := in.Description
			if what == "" {
				what = clip(in.Command, 120)
			}
			out = append(out, Event{"command", "ran a command: " + what})
		case b.Name == "TodoWrite" && len(in.Todos) > 0:
			var items []string
			for _, td := range in.Todos {
				items = append(items, td.Status+": "+td.Content)
			}
			out = append(out, Event{"plan", "updated its plan: " + clip(strings.Join(items, "; "), 300)})
		}
	}
	return out
}

// short keeps the last two path elements, enough to recognise a file.
func short(p string) string {
	dir, file := filepath.Split(filepath.Clean(p))
	if parent := filepath.Base(dir); parent != "." && parent != "/" {
		return parent + "/" + file
	}
	return file
}

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
