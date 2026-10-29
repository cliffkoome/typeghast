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

	"github.com/cliffkoome/typeghast/internal/feed"
	"github.com/cliffkoome/typeghast/internal/models"
	"github.com/cliffkoome/typeghast/internal/narrator"
	"github.com/cliffkoome/typeghast/internal/session"
)

type Mode int

const (
	// Haunted fills the screen with ghosts as the agent's context grows.
	Haunted Mode = iota
	// Normal is a plain typing game over trending developer headlines.
	Normal
)

type Config struct {
	Mode Mode
	// Threshold, if set, is the context size in tokens at which the haunting
	// peaks. Otherwise it is HauntAt times the model's context window.
	Threshold int
	HauntAt   float64
	// MaxGhosts is how many ghosts are on screen at the threshold.
	MaxGhosts int
	// Tracker supplies the agent's context usage. In haunted mode, nil means
	// demo mode, where the player sets the context with pgup/pgdn.
	Tracker *session.Tracker
	// Initial is the tracker's first snapshot, which holds the session's
	// history so far.
	Initial  session.Snapshot
	Narrator narrator.Narrator
	Catalog  models.Catalog
	// News is what normal mode types.
	News []feed.Item
	// Notice is shown under the meter, e.g. why we fell back to demo mode.
	Notice string
}

const (
	fps        = 30
	headerRows = 3
	footerRows = 2
	textMargin = 4
	textLines  = 4
	demoWindow = 1_000_000
	agentLabel = "what your agent just did"
	lockedHint = "type it without a mistake to unlock the link"
)

// Messages carry the game that scheduled them, so a game left via the menu
// can't feed its timers into the next one.
type tickMsg struct {
	m *Model
	t time.Time
}
type pollMsg struct {
	m    *Model
	snap session.Snapshot
}
type narrationMsg struct {
	m     *Model
	lines []string
	err   error
}

// sentence is a line queued for typing, with the label shown while it is
// being typed. A url stays locked until the sentence is typed perfectly.
type sentence struct {
	text, label, url string
	unlocked         bool
}

type Model struct {
	cfg Config
	rng *rand.Rand

	w, h   int
	pw, ph int // play area

	words []string
	typed [][]rune
	sent  []int // per word, index into sents
	sents []sentence
	// breaks marks words that start a new sentence, which starts a new line.
	breaks []bool
	cur    int
	queue  []sentence
	news   int // next index into cfg.News
	// unlocked holds the links earned by typing their headline perfectly.
	unlocked []feed.Item
	// browse is the finished headline picked with the arrows, whose link
	// is shown, or -1 while following the cursor.
	browse int

	pending     []session.Event
	narrating   bool
	lastNarrate time.Time
	narrErr     error

	ghosts    []*ghost
	lastSpawn time.Time
	cursorX   float64
	cursorY   float64

	ctxTokens int
	window    int
	modelName string
	sessionID string
	updated   time.Time

	exorcisedAt time.Time

	started     time.Time
	correctChrs int
	keystrokes  int
	correctKeys int
	score       float64
	lastTick    time.Time
}

func New(cfg Config) *Model {
	if cfg.HauntAt <= 0 {
		cfg.HauntAt = 0.4
	}
	if cfg.Narrator == nil {
		cfg.Narrator = narrator.AgentWords{}
	}
	m := &Model{cfg: cfg, rng: rand.New(rand.NewSource(time.Now().UnixNano())), window: demoWindow}
	switch {
	case cfg.Mode == Normal:
	case cfg.Tracker == nil:
		m.ctxTokens = m.threshold() / 4
	default:
		m.apply(cfg.Initial)
	}
	m.reset()
	return m
}

func (m *Model) reset() {
	m.words, m.typed, m.sent, m.sents, m.breaks = nil, nil, nil, nil, nil
	m.cur, m.browse = 0, -1
	m.started = time.Time{}
	m.correctChrs, m.keystrokes, m.correctKeys = 0, 0, 0
	m.score = 0
	m.refill()
}

func (m *Model) haunted() bool { return m.cfg.Mode == Haunted }

func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.tick(), m.maybeNarrate()}
	if m.haunted() && m.cfg.Tracker != nil {
		cmds = append(cmds, m.poll(time.Second))
	}
	return tea.Batch(cmds...)
}

func (m *Model) tick() tea.Cmd {
	return tea.Tick(time.Second/fps, func(t time.Time) tea.Msg { return tickMsg{m, t} })
}

func (m *Model) poll(after time.Duration) tea.Cmd {
	t := m.cfg.Tracker
	return tea.Tick(after, func(time.Time) tea.Msg { return pollMsg{m, t.Poll()} })
}

func (m *Model) apply(snap session.Snapshot) {
	m.setContext(snap.ContextTokens)
	m.modelName, m.sessionID, m.updated = snap.Model, snap.SessionID, snap.Updated
	m.window = m.cfg.Catalog.Window(snap.Model, snap.ContextTokens)
	m.addEvents(snap.Events)
}

func (m *Model) threshold() int {
	if m.cfg.Threshold > 0 {
		return m.cfg.Threshold
	}
	return int(float64(m.window) * m.cfg.HauntAt)
}

// ratio is how far the context is toward the haunting threshold.
func (m *Model) ratio() float64 {
	if !m.haunted() {
		return 0
	}
	return math.Min(float64(m.ctxTokens)/float64(m.threshold()), 1.25)
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
		m.pw, m.ph = m.w, max(m.h-headerRows-footerRows, textLines*2+3)
		return m, nil

	case tickMsg:
		if msg.m != m {
			return m, nil
		}
		now := msg.t
		dt := 1.0 / fps
		if !m.lastTick.IsZero() {
			dt = math.Min(now.Sub(m.lastTick).Seconds(), 0.1)
		}
		m.lastTick = now
		m.stepGhosts(now, dt)
		return m, m.tick()

	case pollMsg:
		if msg.m != m {
			return m, nil
		}
		m.apply(msg.snap)
		return m, tea.Batch(m.poll(time.Second), m.maybeNarrate())

	case narrationMsg:
		if msg.m != m {
			return m, nil
		}
		m.narrating, m.lastNarrate, m.narrErr = false, time.Now(), msg.err
		for _, l := range msg.lines {
			m.enqueue(sentence{text: l, label: agentLabel})
		}
		return m, m.maybeNarrate()

	case tea.KeyMsg:
		return m, m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) handleKey(k tea.KeyMsg) tea.Cmd {
	demo := m.haunted() && m.cfg.Tracker == nil
	switch k.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		return tea.Quit
	case tea.KeyCtrlR:
		m.reset()
	case tea.KeyPgUp:
		if demo {
			m.setContext(m.ctxTokens + m.threshold()/10)
		}
	case tea.KeyPgDown:
		if demo {
			m.setContext(max(m.ctxTokens-m.threshold()/10, 0))
		}
	case tea.KeyHome:
		if demo {
			m.setContext(m.threshold() / 20)
		}
	case tea.KeyUp:
		m.browseBy(-1)
	case tea.KeyDown:
		m.browseBy(1)
	case tea.KeyCtrlO:
		if s, ok := m.shownLink(); ok && s.unlocked {
			return openURL(s.url)
		}
	// Typing goes back to following the cursor.
	case tea.KeyBackspace:
		m.browse = -1
		m.backspace()
	case tea.KeySpace:
		m.browse = -1
		m.space()
	case tea.KeyEnter:
		m.browse = -1
		// Enter finishes a line, so it only works on a sentence's last word.
		if m.breaks[m.cur+1] {
			m.space()
		}
	case tea.KeyRunes:
		m.browse = -1
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
	if m.sent[m.cur] != m.sent[m.cur-1] {
		m.finishSentence(m.sent[m.cur-1])
	}
}

// finishSentence unlocks the sentence's link if every word was typed right.
func (m *Model) finishSentence(si int) {
	s := m.sents[si]
	if s.url == "" {
		return
	}
	for i := m.cur - 1; i >= 0 && m.sent[i] == si; i-- {
		if string(m.typed[i]) != m.words[i] {
			return
		}
	}
	m.sents[si].unlocked = true
	m.unlocked = append(m.unlocked, feed.Item{Text: s.text, Source: s.label, URL: s.url})
}

// browseBy moves the selection to the previous (-1) or next (+1) finished
// headline. Moving down past the last one goes back to the cursor.
func (m *Model) browseBy(dir int) {
	cur := m.sent[m.cur]
	si := m.browse
	if si < 0 {
		if dir > 0 {
			return
		}
		si = cur
	}
	for si += dir; si >= 0 && si < cur; si += dir {
		if m.sents[si].url != "" {
			m.browse = si
			return
		}
	}
	if dir > 0 {
		m.browse = -1
	}
}

// shownLink is the headline whose link sits under the text: the one picked
// with the arrows, else the latest one unlocked.
func (m *Model) shownLink() (sentence, bool) {
	if m.browse >= 0 {
		return m.sents[m.browse], true
	}
	for si := m.sent[m.cur] - 1; si >= 0; si-- {
		if m.sents[si].unlocked {
			return m.sents[si], true
		}
	}
	return sentence{}, false
}

// Unlocked returns the links earned so far, oldest first.
func (m *Model) Unlocked() []feed.Item { return m.unlocked }

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

// lookahead is kept short so fresh agent sentences show up within a line.
const lookahead = 30

func (m *Model) refill() {
	for len(m.words)-m.cur < lookahead {
		s := m.next()
		// Filler runs on without breaks; real sentences get their own line.
		prev := ""
		if n := len(m.sents); n > 0 {
			prev = m.sents[n-1].label
		}
		m.sents = append(m.sents, s)
		for i, w := range strings.Fields(s.text) {
			m.words = append(m.words, w)
			m.typed = append(m.typed, nil)
			m.sent = append(m.sent, len(m.sents)-1)
			m.breaks = append(m.breaks, i == 0 && (s.label != "" || prev != ""))
		}
	}
}

// next picks the upcoming sentence: queued agent news first in haunted
// mode, headlines in normal mode, and filler words when there's nothing else.
func (m *Model) next() sentence {
	if len(m.queue) > 0 {
		s := m.queue[0]
		m.queue = m.queue[1:]
		return s
	}
	if !m.haunted() {
		news := m.cfg.News
		if len(news) == 0 {
			news = feed.Builtin()
		}
		it := news[m.news%len(news)]
		m.news++
		return sentence{text: it.Text, label: it.Source, url: it.URL}
	}
	return sentence{text: strings.Join(randomWords(m.rng, 10), " ")}
}

const maxQueue = 12

func (m *Model) enqueue(s sentence) {
	m.queue = append(m.queue, s)
	// Stay current: if the agent outpaces the player, drop the oldest news.
	if n := len(m.queue); n > maxQueue {
		m.queue = m.queue[n-maxQueue:]
	}
}

func (m *Model) addEvents(events []session.Event) {
	m.pending = append(m.pending, events...)
	if n := len(m.pending); n > 30 {
		m.pending = m.pending[n-30:]
	}
}

// maybeNarrate turns pending actions into sentences. A model narrator waits
// until a few actions pile up, or until one has waited a while.
func (m *Model) maybeNarrate() tea.Cmd {
	n := m.cfg.Narrator
	if !m.haunted() || m.narrating || len(m.pending) == 0 {
		return nil
	}
	if n.Batches() && len(m.pending) < 3 && time.Since(m.lastNarrate) < 20*time.Second {
		return nil
	}
	keep := 12
	if !n.Batches() {
		keep = 6
	}
	events := m.pending[max(len(m.pending)-keep, 0):]
	m.pending, m.narrating = nil, true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		lines, err := n.Narrate(ctx, events)
		return narrationMsg{m, lines, err}
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
	x, curLine, selLine := 0, 0, -1
	for i, w := range m.words {
		n := max(len([]rune(w)), len(m.typed[i]))
		if x > 0 && (x+n > maxW || m.breaks[i]) {
			lines = append(lines, line)
			line, x = nil, 0
		}
		if i == m.cur {
			curLine = len(lines)
		}
		if selLine < 0 && m.browse >= 0 && m.sent[i] == m.browse {
			selLine = len(lines)
		}
		line = append(line, span{i, x})
		x += n + 1
		if i > m.cur && len(lines) > curLine+textLines {
			break
		}
	}
	lines = append(lines, line)

	// Keep the line in focus second from the top, so the one before shows.
	anchor := curLine
	if selLine >= 0 {
		anchor = selLine
	}
	first := max(anchor-1, 0)
	top := (m.ph - (textLines*2 - 1)) / 2
	if selLine >= 0 {
		c.text(textMargin-2, top+(selLine-first)*2, 1, "›", stLink)
	}
	s := m.sents[m.sent[m.cur]]
	label := s.label
	switch {
	case m.browse >= 0:
		label = "picking a headline · type to carry on"
	case s.url != "":
		label += " · " + lockedHint
	}
	c.text(textMargin, top-2, maxW, label, stLabel)
	if s, ok := m.shownLink(); ok {
		if s.unlocked {
			c.text(textMargin, top+textLines*2, maxW, s.label+" · "+s.url, stLink)
		} else {
			c.text(textMargin, top+textLines*2, maxW, s.label+" · locked, it had a typo", stLabel)
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
	stats := fmt.Sprintf("wpm %3.0f   acc %3.0f%%", wpm, acc)
	if m.haunted() {
		stats += fmt.Sprintf("   score %6.0f   ×%.1f", m.score, m.multiplier())
	}
	stats = statStyle.Render(stats)
	title := titleStyle.Render("typeghast")
	gap := max(m.w-lipgloss.Width(title)-lipgloss.Width(stats)-2, 1)
	row1 := " " + title + strings.Repeat(" ", gap) + stats

	row2 := " " + dimStyle.Render("normal mode · trending on DEV, Lobsters and Hacker News")
	if m.haunted() {
		row2 = " " + m.meter()
	}

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
		if idle := time.Since(m.updated); !m.updated.IsZero() && idle > 2*time.Minute {
			src += " · idle " + idle.Round(time.Minute).String()
		}
	}
	return fmt.Sprintf("%s %s %s  %s",
		statStyle.Render("context"), bar,
		statStyle.Render(fmt.Sprintf("%s · haunting peaks at %s of %s", kTokens(m.ctxTokens), kTokens(m.threshold()), kTokens(m.window))),
		dimStyle.Render(src))
}

func (m *Model) footer() string {
	help := "esc quit · ctrl+r restart"
	if n := len(m.unlocked); n > 0 {
		help += fmt.Sprintf(" · ↑↓ pick a headline · ctrl+o open link · %d unlocked, listed when you quit", n)
	}
	if m.haunted() && m.cfg.Tracker == nil {
		help += " · pgup/pgdn context · home compact"
	}
	if m.haunted() && m.cfg.Tracker != nil {
		status := "narrator: " + m.cfg.Narrator.Name()
		switch {
		case m.narrating && m.cfg.Narrator.Batches():
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
		return strings.TrimSuffix(fmt.Sprintf("%.2f", float64(n)/1_000_000), ".00") + "M"
	}
	return fmt.Sprintf("%dk", n/1000)
}
