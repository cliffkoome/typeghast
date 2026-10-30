package narrator

import (
	"context"
	"path"
	"slices"
	"strings"

	"github.com/cliffkoome/typeghast/internal/session"
	"github.com/cliffkoome/typeghast/internal/text"
)

// AgentWords needs no model: agents already describe their work in plain
// language, in their messages, command descriptions and plans.
type AgentWords struct{}

func (AgentWords) Name() string  { return "agent's own words" }
func (AgentWords) Batches() bool { return false }

// Narrate also names ghosts after the files the agent touched, since those
// are what fill its context.
func (AgentWords) Narrate(_ context.Context, events []session.Event) (Narration, error) {
	var res Narration
	for _, e := range events {
		if l := plain(e); len(strings.Fields(l)) >= 2 {
			res.Lines = append(res.Lines, l)
		}
		if e.Kind == "edit" || e.Kind == "write" {
			head, _, _ := strings.Cut(e.Text, ":")
			_, file, _ := strings.Cut(head, " ")
			if g := ghostName(path.Base(file)); g != "" && !slices.Contains(res.Ghosts, g) {
				res.Ghosts = append(res.Ghosts, g)
			}
		}
	}
	return res, nil
}

func plain(e session.Event) string {
	switch e.Kind {
	case "said":
		return firstSentence(e.Text)
	case "command":
		return text.Clean(strings.TrimPrefix(e.Text, "ran a command: "))
	case "edit", "write":
		head, _, _ := strings.Cut(e.Text, ":")
		return text.Clean(head)
	}
	return ""
}

// firstSentence keeps the opening sentence of a message, capped at a
// comfortable typing length.
func firstSentence(s string) string {
	s = text.Clean(s)
	// Split on ". " rather than "." so file names like model.go survive.
	for _, sep := range []string{". ", ": "} {
		if i := strings.Index(s, sep); i > 0 {
			s = s[:i+1]
		}
	}
	words := strings.Fields(s)
	if len(words) > 16 {
		words = words[:16]
	}
	return strings.TrimSuffix(strings.Join(words, " "), ":")
}
