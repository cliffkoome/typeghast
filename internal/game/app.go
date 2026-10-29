package game

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cliffkoome/typeghast/internal/feed"
	"github.com/cliffkoome/typeghast/internal/models"
	"github.com/cliffkoome/typeghast/internal/session"
)

// AppConfig sets up the menu. Base holds the settings shared by every game;
// its Mode, Tracker, News and Catalog are filled in by the menu.
type AppConfig struct {
	Base Config
	// Mode skips the mode menu when set.
	Mode *Mode
	// SessionPath pins a transcript and skips the agent menu.
	SessionPath string
	Demo        bool
}

type screen int

const (
	pickMode screen = iota
	pickSession
	loading
	playing
)

type option struct {
	title, detail string
	run           func(*App) tea.Cmd
}

type newsMsg []feed.Item
type catalogMsg models.Catalog

// App is the top-level program: the menus, then the game.
type App struct {
	cfg     AppConfig
	w, h    int
	screen  screen
	options []option
	sel     int
	game    *Model

	news    []feed.Item
	catalog models.Catalog
	// links are those unlocked in games already left for the menu.
	links []feed.Item
	// waiting is the game to start once loading finishes.
	waiting *Config
}

func NewApp(cfg AppConfig) *App {
	a := &App{cfg: cfg}
	a.showModes()
	return a
}

func (a *App) Init() tea.Cmd {
	cmds := []tea.Cmd{
		func() tea.Msg { return newsMsg(feed.Load()) },
		func() tea.Msg { return catalogMsg(models.Load()) },
	}
	if a.cfg.Mode != nil {
		cmds = append(cmds, a.choose(*a.cfg.Mode))
	}
	return tea.Batch(cmds...)
}

func (a *App) showModes() {
	a.screen, a.sel = pickMode, 0
	a.options = []option{
		{"👻  Haunted", "ghosts swarm the words as your agent's context fills up", func(a *App) tea.Cmd { return a.choose(Haunted) }},
		{"⌨   Normal", "type trending developer headlines, no ghosts", func(a *App) tea.Cmd { return a.choose(Normal) }},
	}
}

func (a *App) choose(mode Mode) tea.Cmd {
	if mode == Normal {
		cfg := a.cfg.Base
		cfg.Mode = Normal
		return a.start(cfg)
	}
	if a.cfg.Demo {
		return a.startDemo("")
	}
	if a.cfg.SessionPath != "" {
		return a.startSession(a.cfg.SessionPath, false)
	}

	sessions := session.Discover(24 * time.Hour)
	var active []session.Found
	for _, s := range sessions {
		if time.Since(s.Updated) < 30*time.Minute {
			active = append(active, s)
		}
	}
	switch {
	case len(sessions) == 0:
		return a.startDemo("no agent session in the last 24h — demo mode, pgup/pgdn sets the context")
	case len(active) == 1:
		return a.startSession(active[0].Path, true)
	}

	a.screen, a.sel = pickSession, 0
	a.options = nil
	for _, s := range sessions[:min(len(sessions), 8)] {
		path := s.Path
		a.options = append(a.options, option{
			fmt.Sprintf("%s · %s", s.Agent, s.Project),
			"active " + ago(s.Updated),
			func(a *App) tea.Cmd { return a.startSession(path, true) },
		})
	}
	a.options = append(a.options, option{"Demo", "no agent, pgup/pgdn sets the context",
		func(a *App) tea.Cmd { return a.startDemo("") }})
	return nil
}

func (a *App) startDemo(notice string) tea.Cmd {
	cfg := a.cfg.Base
	cfg.Mode, cfg.Notice = Haunted, notice
	return a.start(cfg)
}

func (a *App) startSession(path string, follow bool) tea.Cmd {
	cfg := a.cfg.Base
	cfg.Mode = Haunted
	cfg.Tracker = session.NewTracker(path, follow)
	cfg.Initial = cfg.Tracker.Poll()
	return a.start(cfg)
}

// start launches a game, first waiting for whatever data it needs.
func (a *App) start(cfg Config) tea.Cmd {
	if (cfg.Mode == Normal && a.news == nil) || (cfg.Tracker != nil && a.catalog == nil) {
		a.screen, a.waiting = loading, &cfg
		return nil
	}
	cfg.News, cfg.Catalog = a.news, a.catalog
	a.game = New(cfg)
	a.screen, a.waiting = playing, nil
	a.game.Update(tea.WindowSizeMsg{Width: a.w, Height: a.h})
	return a.game.Init()
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.w, a.h = msg.Width, msg.Height
	case newsMsg:
		a.news = msg
		return a, a.resume()
	case catalogMsg:
		a.catalog = models.Catalog(msg)
		return a, a.resume()
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return a, tea.Quit
		}
		if msg.Type == tea.KeyEsc {
			if a.screen == pickMode {
				return a, tea.Quit
			}
			// Back to the menu; the old game's ticks stop being delivered.
			a.links = a.Links()
			a.game = nil
			a.showModes()
			return a, nil
		}
		if a.screen == pickMode || a.screen == pickSession {
			return a, a.menuKey(msg)
		}
	}
	if a.screen == playing && a.game != nil {
		_, cmd := a.game.Update(msg)
		return a, cmd
	}
	return a, nil
}

// Links returns every headline link the player unlocked, oldest first.
func (a *App) Links() []feed.Item {
	links := a.links
	if a.game != nil {
		links = append(links[:len(links):len(links)], a.game.Unlocked()...)
	}
	return links
}

func (a *App) resume() tea.Cmd {
	if a.screen == loading && a.waiting != nil {
		return a.start(*a.waiting)
	}
	return nil
}

func (a *App) menuKey(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "up", "k":
		a.sel = (a.sel - 1 + len(a.options)) % len(a.options)
	case "down", "j", "tab":
		a.sel = (a.sel + 1) % len(a.options)
	case "enter", " ":
		return a.options[a.sel].run(a)
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		if i := int(k.String()[0] - '1'); i < len(a.options) {
			return a.options[i].run(a)
		}
	}
	return nil
}

var (
	logo = []string{
		" ▄███▄ ",
		"██o█o██",
		"███████",
		"█▀█▀█▀█",
	}
	selStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#f5c542"))
	optionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#e6e6f0"))
	logoStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#d9d4ff"))
)

func (a *App) View() string {
	if a.screen == playing && a.game != nil {
		return a.game.View()
	}
	head := lipgloss.JoinVertical(lipgloss.Center,
		logoStyle.Render(strings.Join(logo, "\n")), "",
		titleStyle.Render("typeghast"),
		dimStyle.Render("a typing game for while your agent works"), "")

	var body []string
	switch a.screen {
	case loading:
		body = append(body, statStyle.Render("summoning…"))
	default:
		heading := "choose a mode"
		if a.screen == pickSession {
			heading = "which agent session should haunt you?"
		}
		body = append(body, statStyle.Render(heading), "")
		titleW := 0
		for _, o := range a.options {
			titleW = max(titleW, lipgloss.Width(o.title))
		}
		for i, o := range a.options {
			title := o.title + strings.Repeat(" ", titleW-lipgloss.Width(o.title))
			line := fmt.Sprintf("%d  %s", i+1, title)
			if i == a.sel {
				line = selStyle.Render("› " + line)
			} else {
				line = "  " + optionStyle.Render(line)
			}
			body = append(body, line+"  "+dimStyle.Render(o.detail))
		}
		body = append(body, "", dimStyle.Render("↑/↓ choose · enter start · esc back/quit"))
	}
	// Options line up on the left; the block as a whole sits in the middle.
	block := lipgloss.JoinVertical(lipgloss.Center, head, lipgloss.JoinVertical(lipgloss.Left, body...))
	return lipgloss.Place(a.w, a.h, lipgloss.Center, lipgloss.Center, block)
}

func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
}
