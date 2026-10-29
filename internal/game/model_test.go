package game

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func newSized(t *testing.T, ctx int) *Model {
	t.Helper()
	m := New(Config{Threshold: 400_000, MaxGhosts: 14})
	m.ctxTokens = ctx
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return m
}

func runFor(m *Model, d time.Duration) {
	now := time.Now()
	for t := time.Duration(0); t < d; t += time.Second / fps {
		m.Update(tickMsg(now.Add(t)))
	}
}

func activeGhosts(m *Model) int {
	n := 0
	for _, g := range m.ghosts {
		if !g.leaving {
			n++
		}
	}
	return n
}

func TestGhostCountFollowsContext(t *testing.T) {
	low := newSized(t, 10_000)
	runFor(low, 8*time.Second)
	if n := activeGhosts(low); n != 0 {
		t.Fatalf("low context: %d ghosts, want 0", n)
	}

	high := newSized(t, 400_000)
	runFor(high, 8*time.Second)
	if n := activeGhosts(high); n != 14 {
		t.Fatalf("at threshold: %d ghosts, want 14", n)
	}
}

func TestCompactionExorcisesGhosts(t *testing.T) {
	m := newSized(t, 400_000)
	runFor(m, 8*time.Second)
	m.setContext(30_000)
	if n := activeGhosts(m); n != 0 {
		t.Fatalf("%d ghosts still haunting after compaction", n)
	}
}

func TestTypingScoresAndAdvances(t *testing.T) {
	m := newSized(t, 0)
	m.words[0], m.words[1] = "ghost", "crypt"
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ghost crypx")})
	if m.cur != 1 || m.correctChrs != 6 || m.keystrokes != 10 || m.correctKeys != 9 {
		t.Fatalf("cur=%d correct=%d keys=%d/%d", m.cur, m.correctChrs, m.correctKeys, m.keystrokes)
	}
	if m.View() == "" {
		t.Fatal("empty view")
	}
}
