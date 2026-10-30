package game

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cliffkoome/typeghast/internal/narrator"
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
		m.Update(tickMsg{m, now.Add(t)})
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

func TestNarratorNamesTheGhosts(t *testing.T) {
	m := newSized(t, 400_000)
	m.names = nil // forget the demo's names
	runFor(m, 2*time.Second)
	m.Update(narrationMsg{m: m, res: narrator.Narration{Ghosts: []string{"flaky test logs", "model.go rewrite"}}})
	worn := map[string]int{}
	for _, g := range m.ghosts {
		if g.name == "" {
			t.Fatal("a ghost was left nameless")
		}
		worn[g.name]++
	}
	if worn["flaky test logs"] == 0 || worn["model.go rewrite"] == 0 {
		t.Fatalf("names not shared out: %v", worn)
	}
	if !strings.Contains(m.View(), "model.go rewrite") {
		t.Fatal("name not drawn")
	}

	m.setContext(30_000)
	if len(m.names) != 0 {
		t.Fatalf("names survived the exorcism: %q", m.names)
	}
}

func typeWords(m *Model, n int) {
	for i := 0; i < n; i++ {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(m.words[m.cur] + " ")})
	}
}

func TestStreakPopsShakesAndTrails(t *testing.T) {
	m := newSized(t, 0)
	typeWords(m, streakEvery)
	if m.fx.streak != streakEvery || !m.popping(m.cur-1) {
		t.Fatalf("streak=%d popping=%v", m.fx.streak, m.popping(m.cur-1))
	}
	if time.Since(m.fx.shakeAt) > time.Second || !strings.Contains(m.header(), "25-word streak") {
		t.Fatal("no shake or banner at the streak")
	}
	// Twenty-five words in an instant is very fast, so speed lines show.
	if !strings.Contains(m.View(), "━") {
		t.Fatal("no speed trail")
	}
	if m.burstWPM(time.Now().Add(burstWindow+time.Millisecond)) != 0 {
		t.Fatal("trail speed didn't fade")
	}

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x ")})
	if m.fx.streak != 0 {
		t.Fatal("a wrong word didn't break the streak")
	}
}

func TestNoShake(t *testing.T) {
	m := New(Config{Threshold: 400_000, MaxGhosts: 14, NoShake: true})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	typeWords(m, streakEvery)
	if m.shakeOffset(time.Now()) != 0 {
		t.Fatal("shook with NoShake set")
	}
}

func TestEveryStyleIsDefined(t *testing.T) {
	for st := stUntyped; st <= stTrailFade; st++ {
		if _, ok := styles[st]; !ok {
			t.Errorf("style %d has no entry in styles", st)
		}
	}
}
