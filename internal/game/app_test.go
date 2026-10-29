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
	if v := a.View(); !strings.Contains(v, "DEV · https://dev.to/x") {
		t.Fatal("headline source not shown")
	}

	// Esc returns to the menu instead of quitting.
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if a.screen != pickMode {
		t.Fatalf("after esc screen = %v", a.screen)
	}
}
