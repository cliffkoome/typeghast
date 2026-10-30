package narrator

import (
	"context"
	"testing"

	"github.com/cliffkoome/typeghast/internal/session"
)

func TestAgentWords(t *testing.T) {
	res, _ := AgentWords{}.Narrate(context.Background(), []session.Event{
		{Kind: "said", Text: "Now I'll fix the **timer** in model.go so ticks stop doubling. Then tests."},
		{Kind: "command", Text: "ran a command: Run the test suite"},
		{Kind: "edit", Text: "edited game/model.go: x := 1"},
		{Kind: "said", Text: "Ok."},
		{Kind: "write", Text: "wrote game/model.go: y := 2"},
	})
	lines := res.Lines
	want := []string{
		"Now I'll fix the timer in model.go so ticks stop doubling.",
		"Run the test suite",
		"edited game/model.go",
		"wrote game/model.go",
	}
	if len(lines) != len(want) {
		t.Fatalf("got %q", lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, lines[i], want[i])
		}
	}
	if len(res.Ghosts) != 1 || res.Ghosts[0] != "model.go" {
		t.Errorf("ghosts = %q, want [model.go]", res.Ghosts)
	}
}

func TestGhostNameCutsAtWords(t *testing.T) {
	for in, want := range map[string]string{
		"Usage Transcript Logs.":             "usage transcript",
		"model.go":                           "model.go",
		"supercalifragilisticexpialidocious": "supercalifragilist",
		"   ":                                "",
	} {
		if got := ghostName(in); got != want {
			t.Errorf("ghostName(%q) = %q, want %q", in, got, want)
		}
	}
}
