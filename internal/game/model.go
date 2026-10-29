// Package game is the typeghast typing game.
package game

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cliffkoome/typeghast/internal/narrator"
	"github.com/cliffkoome/typeghast/internal/session"
)

type Config struct {
	// Threshold is the context size, in tokens, at which the haunting peaks.
	Threshold int
	// MaxGhosts is how many ghosts are on screen at the threshold.
	MaxGhosts int
	// Tracker supplies the agent's context usage. Nil means demo mode, where
	// the player sets the context with pgup/pgdn.
	Tracker *session.Tracker
	// Narrator turns the agent's actions into sentences to type. Without one,
	// commands and edits are typed as short plain lines.
	Narrator *narrator.Narrator
	// Initial is the tracker's first snapshot, which holds the session's
	// history so far.
	Initial session.Snapshot
	// Notice is shown under the meter, e.g. why we fell back to demo mode.
	Notice string
}

const (
	fps        = 30
	headerRows = 3
	footerRows = 2
	textMargin = 4
	textLines  = 4
)

type tickMsg time.Time
type pollMsg session.Snapshot
type narrationMsg struct {
	lines []string
	err   error
}

type Model struct {
	cfg Config
	rng *rand.Rand

	w, h   int
	pw, ph int // play area

	words []string
	typed [][]rune
	cur   int
	// fromAgent marks words that came from the agent's actions rather than
	// the filler pool.
	fromAgent []bool
	// queue holds agent sentences not yet added to the word stream.
	queue []string

	pending     []session.Event
	narrating   bool
	lastNarrate time.Time
	narrErr     error

	ghosts    []*ghost
	lastSpawn time.Time
	cursorX   float64
	cursorY   float64

	ctxTokens int
	modelName string
	sessionID string

	exorcisedAt time.Time

	started     time.Time
	correctChrs int
	keystrokes  int
	correctKeys int
	score       float64
	lastTick    time.Time
}

func New(cfg Config) *Model {
	m := &Model{cfg: cfg, rng: rand.New(rand.NewSource(time.Now().UnixNano()))}
	m.reset()
	if cfg.Tracker == nil {
		m.ctxTokens = cfg.Threshold / 4
	} else {
		m.ctxTokens = cfg.Initial.ContextTokens
		m.modelName, m.sessionID = cfg.Initial.Model, cfg.Initial.SessionID
		m.addEvents(cfg.Initial.Events)
	}
	return m
}

func (m *Model) reset() {
	m.words, m.typed, m.fromAgent = nil, nil, nil
	m.cur = 0
	m.refill()
	m.started = time.Time{}
	m.correctChrs, m.keystrokes, m.correctKeys = 0, 0, 0
	m.score = 0
}

func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{tick(), m.maybeNarrate()}
	if m.cfg.Tracker != nil {
		cmds = append(cmds, m.poll(time.Second))
	}
	return tea.Batch(cmds...)
}

func tick() tea.Cmd {
	return tea.Tick(time.Second/fps, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *Model) poll(after time.Duration) tea.Cmd {
	t := m.cfg.Tracker
	return tea.Tick(after, func(time.Time) tea.Msg { return pollMsg(t.Poll()) })
}

// ratio is how far the context is toward the haunting threshold.
func (m *Model) ratio() float64 {
	return math.Min(float64(m.ctxTokens)/float64(m.cfg.Threshold), 1.25)
}

func (m *Model) targetGhosts() int {
	r := m.ratio()
	if r < 0.05 {
		return 0
	}
	return int(math.Round(math.Pow(r, 1.4) * float64(m.cfg.MaxGhosts)))
}

// multiplier rewards typing through a heavier haunting.
func (m *Model) multiplier() float64 {
	return 1 + 2*math.Min(m.ratio(), 1)
}

func (m *Model) setContext(n int) {
	// A sharp drop means the session was compacted or cleared.
	if m.ctxTokens > 20_000 && n < m.ctxTokens/2 {
		m.exorcise()
	}
	m.ctxTokens = n
}

func (m *Model) exorcise() {
	m.exorcisedAt = time.Now()
	cx, cy := float64(m.pw)/2, float64(m.ph)/2
	for _, g := range m.ghosts {
		g.leaving = true
		dx, dy := g.x+ghostW/2-cx, g.y+ghostH/2-cy
		d := math.Max(math.Hypot(dx, dy), 1)
		g.vx, g.vy = dx/d*60, dy/d*30
	}
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.pw, m.ph = m.w, max(m.h-headerRows-footerRows, textLines*2+1)
		return m, nil

	case tickMsg:
		now := time.Time(msg)
		dt := 1.0 / fps
		if !m.lastTick.IsZero() {
			dt = math.Min(now.Sub(m.lastTick).Seconds(), 0.1)
		}
		m.lastTick = now
		m.stepGhosts(now, dt)
		return m, tick()

	case pollMsg:
		snap := session.Snapshot(msg)
		m.setContext(snap.ContextTokens)
		m.modelName, m.sessionID = snap.Model, snap.SessionID
		m.addEvents(snap.Events)
		return m, tea.Batch(m.poll(time.Second), m.maybeNarrate())

	case narrationMsg:
		m.narrating, m.lastNarrate, m.narrErr = false, time.Now(), msg.err
		m.enqueue(msg.lines...)
		return m, m.maybeNarrate()

	case tea.KeyMsg:
		return m, m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) handleKey(k tea.KeyMsg) tea.Cmd {
	switch k.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		return tea.Quit
	case tea.KeyCtrlR:
		m.reset()
	case tea.KeyPgUp:
		if m.cfg.Tracker == nil {
			m.setContext(m.ctxTokens + m.cfg.Threshold/10)
		}
	case tea.KeyPgDown:
		if m.cfg.Tracker == nil {
			m.setContext(max(m.ctxTokens-m.cfg.Threshold/10, 0))
		}
	case tea.KeyHome:
		if m.cfg.Tracker == nil {
			m.setContext(m.cfg.Threshold / 20)
		}
	case tea.KeyBackspace:
		m.backspace()
	case tea.KeySpace:
		m.space()
	case tea.KeyRunes:
		for _, r := range k.Runes {
			if r == ' ' {
				m.space()
			} else {
				m.typeRune(r)
			}
		}
	}
	return nil
}

func (m *Model) typeRune(r rune) {
	if m.started.IsZero() {
		m.started = time.Now()
	}
	word := []rune(m.words[m.cur])
	i := len(m.typed[m.cur])
	m.typed[m.cur] = append(m.typed[m.cur], r)
	m.keystrokes++
	if i < len(word) && word[i] == r {
		m.correctKeys++
		m.score += m.multiplier()
	}
}

func (m *Model) space() {
	if len(m.typed[m.cur]) == 0 {
		return
	}
	if string(m.typed[m.cur]) == m.words[m.cur] {
		m.correctChrs += len(m.words[m.cur]) + 1
	}
	m.cur++
	m.refill()
}

// lookahead is kept short so fresh agent sentences show up within a line.
const lookahead = 30

func (m *Model) refill() {
	for len(m.words)-m.cur < lookahead {
		words, agent := randomWords(m.rng, 10), false
		if len(m.queue) > 0 {
			words, agent = strings.Fields(m.queue[0]), true
			m.queue = m.queue[1:]
		}
		for _, w := range words {
			m.words = append(m.words, w)
			m.typed = append(m.typed, nil)
			m.fromAgent = append(m.fromAgent, agent)
		}
	}
}

const maxQueue = 12

func (m *Model) enqueue(lines ...string) {
	m.queue = append(m.queue, lines...)
	// Stay current: if the agent outpaces the player, drop the oldest news.
	if n := len(m.queue); n > maxQueue {
		m.queue = m.queue[n-maxQueue:]
	}
}

func (m *Model) addEvents(events []session.Event) {
	if m.cfg.Narrator == nil {
		for _, e := range events {
			if l := plainLine(e); l != "" {
				m.enqueue(l)
			}
		}
		return
	}
	m.pending = append(m.pending, events...)
	if n := len(m.pending); n > 30 {
		m.pending = m.pending[n-30:]
	}
}

// plainLine describes an event without a model: commands by their
// description, file changes by file name.
func plainLine(e session.Event) string {
	switch e.Kind {
	case "command":
		return narrator.Clean(strings.TrimPrefix(e.Text, "ran a command: "))
	case "edit", "write":
		head, _, _ := strings.Cut(e.Text, ":")
		return narrator.Clean(head)
	}
	return ""
}

// maybeNarrate sends recent actions to the model once enough have piled up,
// or once some have waited a while.
func (m *Model) maybeNarrate() tea.Cmd {
	n := m.cfg.Narrator
	if n == nil || m.narrating || len(m.pending) == 0 {
		return nil
	}
	if len(m.pending) < 3 && time.Since(m.lastNarrate) < 20*time.Second {
		return nil
	}
	events := m.pending[max(len(m.pending)-12, 0):]
	m.pending, m.narrating = nil, true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		lines, err := n.Narrate(ctx, events)
		return narrationMsg{lines, err}
	}
}

func (m *Model) backspace() {
	if n := len(m.typed[m.cur]); n > 0 {
		m.typed[m.cur] = m.typed[m.cur][:n-1]
		return
	}
	// Step back into the previous word only if it was mistyped.
	if m.cur > 0 && string(m.typed[m.cur-1]) != m.words[m.cur-1] {
		m.cur--
		if string(m.typed[m.cur]) == m.words[m.cur] {
			m.correctChrs -= len(m.words[m.cur]) + 1
		}
	}
}

func (m *Model) stepGhosts(now time.Time, dt float64) {
	if m.pw == 0 {
		return
	}
	speed := 5 + 10*m.ratio()
	alive := m.ghosts[:0]
	active := 0
	for _, g := range m.ghosts {
		g.step(dt, speed, m.pw, m.ph, m.cursorX, m.cursorY)
		if g.leaving && g.offscreen(m.pw, m.ph) {
			continue
		}
		if !g.leaving {
			active++
		}
		alive = append(alive, g)
	}
	m.ghosts = alive

	if time.Since(m.exorcisedAt) < 2*time.Second {
		return
	}
	target := m.targetGhosts()
	if active < target && now.Sub(m.lastSpawn) > 300*time.Millisecond {
		hunter := m.rng.Float64() < 0.2+0.3*m.ratio()
		m.ghosts = append(m.ghosts, spawnGhost(m.rng, m.pw, m.ph, hunter))
		m.lastSpawn = now
	}
	if active > target {
		for _, g := range m.ghosts {
			if !g.leaving {
				g.leaving, g.vx, g.vy = true, 0, -20
				break
			}
		}
	}
}

func (m *Model) View() string {
	if m.w == 0 {
		return ""
	}
	c := newCanvas(m.pw, m.ph)
	m.drawText(c)
	rotten := m.ratio() >= 0.75
	for _, g := range m.ghosts {
		g.draw(c, rotten)
	}
	return lipgloss.JoinVertical(lipgloss.Left, m.header(), c.render(), m.footer())
}

// drawText wraps the words into lines and draws the few around the current
// word. It also records the cursor position for hunter ghosts.
func (m *Model) drawText(c *canvas) {
	maxW := m.pw - textMargin*2
	type span struct{ word, x int }
	var lines [][]span
	var line []span
	x, curLine := 0, 0
	for i, w := range m.words {
		n := max(len([]rune(w)), len(m.typed[i]))
		if x > 0 && x+n > maxW {
			lines = append(lines, line)
			line, x = nil, 0
		}
		if i == m.cur {
			curLine = len(lines)
		}
		line = append(line, span{i, x})
		x += n + 1
		if i > m.cur && len(lines) > curLine+textLines {
			break
		}
	}
	lines = append(lines, line)

	first := max(curLine-1, 0)
	top := (m.ph - (textLines*2 - 1)) / 2
	if m.fromAgent[m.cur] {
		for i, r := range []rune("what your agent just did") {
			c.put(textMargin+i, top-2, r, stLabel)
		}
	}
	for li := 0; li < textLines && first+li < len(lines); li++ {
		y := top + li*2
		for _, s := range lines[first+li] {
			m.drawWord(c, s.word, textMargin+s.x, y)
		}
	}
}

func (m *Model) drawWord(c *canvas, i, x, y int) {
	word, typed := []rune(m.words[i]), m.typed[i]
	for j := 0; j < max(len(word), len(typed)); j++ {
		var r rune
		var st styleID
		switch {
		case j < len(typed) && j < len(word):
			r, st = word[j], stCorrect
			if typed[j] != word[j] {
				st = stWrong
			}
		case j < len(typed):
			r, st = typed[j], stWrong // extra letters past the word
		case i < m.cur:
			r, st = word[j], stMissed
		default:
			r, st = word[j], stUntyped
		}
		if i == m.cur && j == len(typed) {
			st = stCursor
			m.cursorX, m.cursorY = float64(x+j), float64(y)
		}
		c.put(x+j, y, r, st)
	}
	if i == m.cur && len(typed) >= len(word) {
		c.put(x+len(typed), y, ' ', stCursor)
		m.cursorX, m.cursorY = float64(x+len(typed)), float64(y)
	}
}

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#d9d4ff"))
	statStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#a6adc8"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#585b70"))
	bannerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#f5c542"))
)

func (m *Model) header() string {
	wpm, acc := 0.0, 100.0
	if !m.started.IsZero() {
		if mins := time.Since(m.started).Minutes(); mins > 0 {
			wpm = float64(m.correctChrs) / 5 / mins
		}
	}
	if m.keystrokes > 0 {
		acc = float64(m.correctKeys) / float64(m.keystrokes) * 100
	}
	stats := statStyle.Render(fmt.Sprintf("wpm %3.0f   acc %3.0f%%   score %6.0f   ×%.1f",
		wpm, acc, m.score, m.multiplier()))
	title := titleStyle.Render("typeghast")
	gap := max(m.w-lipgloss.Width(title)-lipgloss.Width(stats)-2, 1)
	row1 := " " + title + strings.Repeat(" ", gap) + stats

	row2 := " " + m.meter()

	row3 := ""
	switch {
	case time.Since(m.exorcisedAt) < 2*time.Second:
		row3 = " " + bannerStyle.Render("✝ EXORCISED — context cleared, the ghosts flee ✝")
	case m.cfg.Notice != "":
		row3 = " " + dimStyle.Render(m.cfg.Notice)
	}
	return lipgloss.JoinVertical(lipgloss.Left, row1, row2, row3)
}

func (m *Model) meter() string {
	const width = 24
	r := m.ratio()
	filled := int(math.Round(math.Min(r, 1) * width))
	color := "#a6e3a1"
	switch {
	case r >= 0.75:
		color = "#ff5f6d"
	case r >= 0.4:
		color = "#f5c542"
	}
	bar := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(strings.Repeat("█", filled)) +
		dimStyle.Render(strings.Repeat("░", width-filled))

	src := "demo"
	if m.cfg.Tracker != nil {
		src = m.modelName
		if m.sessionID != "" {
			src += " · session " + m.sessionID[:min(8, len(m.sessionID))]
		}
	}
	return fmt.Sprintf("%s %s %s  %s",
		statStyle.Render("context"), bar,
		statStyle.Render(fmt.Sprintf("%s / %s", kTokens(m.ctxTokens), kTokens(m.cfg.Threshold))),
		dimStyle.Render(src))
}

func (m *Model) footer() string {
	help := "esc quit · ctrl+r restart"
	if m.cfg.Tracker == nil {
		help += " · pgup/pgdn context · home compact"
	}
	if n := m.cfg.Narrator; n != nil {
		status := "narrator " + n.Model
		switch {
		case m.narrating:
			status += " · summarising your agent…"
		case m.narrErr != nil:
			status += " · " + m.narrErr.Error()
		}
		help += "   " + status
	}
	return "\n " + dimStyle.Render(help)
}

func kTokens(n int) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.2fM", float64(n)/1_000_000)
	}
	return fmt.Sprintf("%dk", n/1000)
}
