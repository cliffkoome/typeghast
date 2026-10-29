package game

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cliffkoome/typeghast/internal/feed"
	"github.com/cliffkoome/typeghast/internal/models"
)

func TestNormalModeWaitsForNewsThenTypesHeadlines(t *testing.T) {
	a := NewApp(AppConfig{Base: Config{MaxGhosts: 14}})
	a.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	if a.screen != loading {
		t.Fatalf("screen = %v, want loading until headlines arrive", a.screen)
	}
	a.Update(catalogMsg(models.Catalog{}))
	a.Update(newsMsg([]feed.Item{{Text: "Go ships a new release", Source: "DEV", URL: "https://dev.to/x"}}))
	if a.screen != playing || a.game.cfg.Mode != Normal {
		t.Fatalf("screen = %v", a.screen)
	}
	if got := strings.Join(a.game.words[:5], " "); got != "Go ships a new release" {
		t.Fatalf("words = %q", got)
	}
	if a.game.targetGhosts() != 0 {
		t.Fatal("normal mode must not spawn ghosts")
	}
	if v := a.View(); !strings.Contains(v, "DEV · "+lockedHint) || strings.Contains(v, "https://dev.to/x") {
		t.Fatal("link should start locked")
	}

	typeText := func(s string) { a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}) }
	// A typo, even one left in, keeps the link locked.
	typeText("Go shipz a new release ")
	if len(a.Links()) != 0 {
		t.Fatal("headline with a typo unlocked its link")
	}
	typeText("Go ships a new release ")
	if len(a.Links()) != 1 || a.Links()[0].URL != "https://dev.to/x" {
		t.Fatalf("links = %v", a.Links())
	}
	if v := a.View(); !strings.Contains(v, "DEV · https://dev.to/x") {
		t.Fatal("unlocked link not shown")
	}

	// Enter only moves on at the end of a sentence.
	typeText("Go")
	a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if a.game.cur != 10 {
		t.Fatalf("enter mid-sentence moved to word %d", a.game.cur)
	}
	typeText(" ships a new release")
	a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if a.game.cur != 15 || len(a.Links()) != 2 {
		t.Fatalf("enter at sentence end: word %d, %d links", a.game.cur, len(a.Links()))
	}

	// Up picks finished headlines, newest first, and shows each one's link.
	key := func(k tea.KeyType) { a.Update(tea.KeyMsg{Type: k}) }
	key(tea.KeyUp)
	key(tea.KeyUp)
	if a.game.browse != 1 || !strings.Contains(a.View(), "› Go ships a new release") {
		t.Fatalf("browse = %d", a.game.browse)
	}
	key(tea.KeyUp)
	key(tea.KeyUp) // already at the oldest
	if a.game.browse != 0 || !strings.Contains(a.View(), "DEV · locked, it had a typo") {
		t.Fatalf("browse = %d, want the mistyped first headline", a.game.browse)
	}
	for range 3 {
		key(tea.KeyDown)
	}
	if a.game.browse != -1 {
		t.Fatalf("down past the newest should follow the cursor, browse = %d", a.game.browse)
	}
	key(tea.KeyUp)
	typeText("G")
	if a.game.browse != -1 {
		t.Fatal("typing should stop browsing")
	}

	// Esc returns to the menu instead of quitting, keeping the links.
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if a.screen != pickMode {
		t.Fatalf("after esc screen = %v", a.screen)
	}
	if len(a.Links()) != 2 {
		t.Fatal("links lost on returning to the menu")
	}
}
