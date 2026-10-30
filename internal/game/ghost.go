package game

import (
	"math"
	"math/rand"
)

// Two frames so the ghost's tail ripples as it drifts. Spaces are see-through;
// every other cell covers the text underneath. 'o' marks an eye.
var ghostFrames = [2][]string{
	{
		" ▄███▄ ",
		"██o█o██",
		"███████",
		"█▀█▀█▀█",
	},
	{
		" ▄███▄ ",
		"██o█o██",
		"███████",
		"▀█▀█▀█▀",
	},
}

const ghostW, ghostH = 7, 4

type ghost struct {
	x, y, vx, vy float64
	phase        float64
	// Hunters drift toward the word you're typing instead of wandering.
	hunter  bool
	leaving bool
	// name is what the ghost stands for in the agent's context, if known.
	name string
}

func spawnGhost(rng *rand.Rand, pw, ph int, hunter bool) *ghost {
	g := &ghost{hunter: hunter, phase: rng.Float64() * math.Pi * 2}
	// Enter from a random side of the play area.
	switch rng.Intn(4) {
	case 0:
		g.x, g.y = -ghostW, rng.Float64()*float64(ph-ghostH)
	case 1:
		g.x, g.y = float64(pw), rng.Float64()*float64(ph-ghostH)
	case 2:
		g.x, g.y = rng.Float64()*float64(pw-ghostW), -ghostH
	default:
		g.x, g.y = rng.Float64()*float64(pw-ghostW), float64(ph)
	}
	cx, cy := float64(pw)/2, float64(ph)/2
	angle := math.Atan2(cy-g.y, cx-g.x) + (rng.Float64()-0.5)*1.2
	g.vx, g.vy = math.Cos(angle), math.Sin(angle)*0.5
	return g
}

// step advances the ghost by dt seconds. speed is in cells per second;
// (tx, ty) is the cursor, which hunters steer toward.
func (g *ghost) step(dt, speed float64, pw, ph int, tx, ty float64) {
	g.phase += dt * 4
	if g.leaving {
		g.x += g.vx * dt
		g.y += g.vy * dt
		return
	}
	if g.hunter {
		dx, dy := tx-(g.x+ghostW/2), ty-(g.y+ghostH/2)
		if d := math.Hypot(dx, dy); d > 1 {
			g.vx += dx / d * dt * 1.5
			g.vy += dy / d * dt * 0.8
		}
	}
	// Keep direction but normalise magnitude so every ghost moves at speed.
	if m := math.Hypot(g.vx, g.vy*2); m > 0 {
		g.vx, g.vy = g.vx/m, g.vy/m
	}
	g.x += g.vx * speed * dt
	g.y += g.vy * speed * dt

	maxX, maxY := float64(pw-ghostW), float64(ph-ghostH)
	if g.x < 0 && g.vx < 0 || g.x > maxX && g.vx > 0 {
		g.vx = -g.vx
	}
	if g.y < 0 && g.vy < 0 || g.y > maxY && g.vy > 0 {
		g.vy = -g.vy
	}
}

func (g *ghost) offscreen(pw, ph int) bool {
	return g.x < -ghostW-2 || g.x > float64(pw)+2 || g.y < -ghostH-2 || g.y > float64(ph)+2
}

func (g *ghost) draw(c *canvas, rotten bool) {
	body, eye, label := stGhost, stGhostEye, stGhostName
	if rotten {
		body, eye, label = stGhostRot, stGhostRotEye, stGhostRotName
	}
	frame := ghostFrames[int(g.phase/math.Pi)%2]
	ox := int(math.Round(g.x))
	oy := int(math.Round(g.y + math.Sin(g.phase)*0.4))
	for dy, row := range frame {
		for dx, r := range []rune(row) {
			switch r {
			case ' ':
			case 'o':
				c.put(ox+dx, oy+dy, 'o', eye)
			default:
				c.put(ox+dx, oy+dy, r, body)
			}
		}
	}
	if g.name != "" {
		// The name floats above the ghost, or below it at the top edge.
		ly := oy - 1
		if ly < 0 {
			ly = oy + ghostH
		}
		n := []rune(g.name)
		c.text(ox+(ghostW-len(n))/2, ly, len(n), g.name, label)
	}
}
