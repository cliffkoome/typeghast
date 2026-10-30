package game

import (
	"math"
	"time"
)

// Feedback that makes typing feel good: a word pops when it's typed right,
// speed lines stream behind the cursor when the player is fast, and the play
// area shakes on big moments.

const (
	popDur    = 160 * time.Millisecond
	shakeDur  = 360 * time.Millisecond
	bannerDur = 1500 * time.Millisecond
	// streakEvery correct words in a row earns a shake and a banner.
	streakEvery = 25
	// burstWindow is how far back the trail's speed looks, so it fades soon
	// after the player slows down.
	burstWindow = 2 * time.Second
	// The trail appears at trailFrom WPM and is longest at trailFull.
	trailFrom, trailFull = 70.0, 130.0
	minTrail, maxTrail   = 4, 24
)

type juice struct {
	// Words popFrom..popTo flash bright after popAt.
	popFrom, popTo int
	popAt          time.Time
	shakeAt        time.Time
	shakeAmp       int
	streak         int
	banner         string
	bannerAt       time.Time
	// keys are the times of recent correct keystrokes.
	keys []time.Time
}

type point struct{ x, y int }

func (m *Model) pop(from, to int) {
	m.fx.popFrom, m.fx.popTo, m.fx.popAt = from, to, time.Now()
}

func (m *Model) popping(i int) bool {
	return i >= m.fx.popFrom && i <= m.fx.popTo && time.Since(m.fx.popAt) < popDur
}

// shake jolts the play area sideways by up to amp columns. A shake already
// running keeps the larger amplitude.
func (m *Model) shake(amp int) {
	if m.cfg.NoShake {
		return
	}
	if time.Since(m.fx.shakeAt) < shakeDur {
		amp = max(amp, m.fx.shakeAmp)
	}
	m.fx.shakeAt, m.fx.shakeAmp = time.Now(), amp
}

// shakeOffset is how far the play area is pushed sideways at now. It
// alternates direction and dies down over shakeDur.
func (m *Model) shakeOffset(now time.Time) int {
	t := now.Sub(m.fx.shakeAt)
	if t < 0 || t >= shakeDur {
		return 0
	}
	left := 1 - float64(t)/float64(shakeDur)
	mag := int(math.Ceil(float64(m.fx.shakeAmp) * left))
	if int(t/(shakeDur/8))%2 == 1 {
		return -mag
	}
	return mag
}

func (m *Model) announce(s string) {
	m.fx.banner, m.fx.bannerAt = s, time.Now()
}

// burstWPM is the typing speed over the last burstWindow.
func (m *Model) burstWPM(now time.Time) float64 {
	keys := m.fx.keys
	for len(keys) > 0 && now.Sub(keys[0]) > burstWindow {
		keys = keys[1:]
	}
	m.fx.keys = keys
	return float64(len(keys)) / 5 * float64(time.Minute) / float64(burstWindow)
}

// drawTrail streams speed lines along the blank rows under the letters just
// typed, wrapping back to the line above. They grow longer and hotter with
// speed.
func (m *Model) drawTrail(c *canvas, now time.Time) {
	wpm := m.burstWPM(now)
	if wpm < trailFrom || m.browse >= 0 {
		return
	}
	f := math.Min((wpm-trailFrom)/(trailFull-trailFrom), 1)
	n := minTrail + int(f*float64(maxTrail-minTrail))
	head := stTrailWarm
	switch {
	case wpm >= 110:
		head = stTrailHot
	case wpm >= 90:
		head = stTrailMid
	}
	// Gaps drift away from the cursor, so the lines seem to stream past.
	frame := int(now.UnixMilli() / 60)
	cells := m.typedCells
	for k := 0; k < n && k < len(cells); k++ {
		if ((k-frame)%7+7)%7 == 0 {
			continue
		}
		r, st := '━', head
		switch {
		case k >= n*2/3:
			r, st = '·', stTrailFade
		case k >= n/3:
			r, st = '─', stTrailWarm
		}
		p := cells[len(cells)-1-k]
		c.put(p.x, p.y+1, r, st)
	}
}
