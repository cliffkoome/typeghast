package game

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type styleID int

const (
	stBlank styleID = iota
	stUntyped
	stCorrect
	stWrong
	stMissed
	stCursor
	stGhost
	stGhostEye
	stGhostRot
	stGhostRotEye
)

var styles = map[styleID]lipgloss.Style{
	stUntyped:     lipgloss.NewStyle().Foreground(lipgloss.Color("#6c6f85")),
	stCorrect:     lipgloss.NewStyle().Foreground(lipgloss.Color("#e6e6f0")),
	stWrong:       lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5f6d")),
	stMissed:      lipgloss.NewStyle().Foreground(lipgloss.Color("#a1404a")).Underline(true),
	stCursor:      lipgloss.NewStyle().Foreground(lipgloss.Color("#1e1e2e")).Background(lipgloss.Color("#f5c542")),
	stGhost:       lipgloss.NewStyle().Foreground(lipgloss.Color("#d9d4ff")),
	stGhostEye:    lipgloss.NewStyle().Foreground(lipgloss.Color("#1e1e2e")).Background(lipgloss.Color("#d9d4ff")),
	stGhostRot:    lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e36b")),
	stGhostRotEye: lipgloss.NewStyle().Foreground(lipgloss.Color("#1e1e2e")).Background(lipgloss.Color("#a6e36b")),
}

type cell struct {
	r  rune
	st styleID
}

// canvas is a grid of styled cells. Text is drawn first and ghosts are drawn
// over it, so a ghost hides whatever letters it passes in front of.
type canvas struct {
	w, h  int
	cells []cell
}

func newCanvas(w, h int) *canvas {
	c := &canvas{w: w, h: h, cells: make([]cell, w*h)}
	for i := range c.cells {
		c.cells[i] = cell{' ', stBlank}
	}
	return c
}

func (c *canvas) put(x, y int, r rune, st styleID) {
	if x < 0 || y < 0 || x >= c.w || y >= c.h {
		return
	}
	c.cells[y*c.w+x] = cell{r, st}
}

func (c *canvas) render() string {
	var b strings.Builder
	for y := 0; y < c.h; y++ {
		row := c.cells[y*c.w : (y+1)*c.w]
		for i := 0; i < len(row); {
			j := i
			var run strings.Builder
			for j < len(row) && row[j].st == row[i].st {
				run.WriteRune(row[j].r)
				j++
			}
			if row[i].st == stBlank {
				b.WriteString(run.String())
			} else {
				b.WriteString(styles[row[i].st].Render(run.String()))
			}
			i = j
		}
		if y < c.h-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
