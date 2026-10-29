package narrator

import (
	"context"
	"testing"

	"github.com/cliffkoome/typeghast/internal/session"
)

func TestAgentWords(t *testing.T) {
	lines, _ := AgentWords{}.Narrate(context.Background(), []session.Event{
		{Kind: "said", Text: "Now I'll fix the **timer** in model.go so ticks stop doubling. Then tests."},
		{Kind: "command", Text: "ran a command: Run the test suite"},
		{Kind: "edit", Text: "edited game/model.go: x := 1"},
		{Kind: "said", Text: "Ok."},
	})
	want := []string{
		"Now I'll fix the timer in model.go so ticks stop doubling.",
		"Run the test suite",
		"edited game/model.go",
	}
	if len(lines) != len(want) {
		t.Fatalf("got %q", lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, lines[i], want[i])
		}
	}
}
