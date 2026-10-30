package models

import (
	"math"
	"testing"
)

func TestWindow(t *testing.T) {
	c := Catalog{
		"claude-opus-5-5": {MaxInput: 1_000_000},
		"claude-opus-4-5": {MaxInput: 200_000},
		"openai/gpt-x":    {MaxInput: 400_000},
	}
	cases := []struct {
		model    string
		observed int
		want     int
	}{
		{"claude-opus-5-5", 150_000, 1_000_000},
		{"claude-opus-4-5", 50_000, 200_000},
		// Past the catalog's window means a larger window is in use.
		{"claude-opus-4-5", 350_000, 1_000_000},
		{"gpt-x", 10, 400_000},
		{"unknown-model", 10, DefaultWindow},
	}
	for _, tc := range cases {
		if got := c.Window(tc.model, tc.observed); got != tc.want {
			t.Errorf("Window(%q, %d) = %d, want %d", tc.model, tc.observed, got, tc.want)
		}
	}
}

func TestCost(t *testing.T) {
	c := Catalog{
		"claude-x": {MaxInput: 1, InputCost: 1e-6, OutputCost: 10e-6, CacheReadCost: 0.1e-6, CacheWriteCost: 2e-6},
		"gpt-x":    {MaxInput: 1, InputCost: 1e-6, OutputCost: 10e-6},
	}
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
	if got, ok := c.Cost("claude-x", 1_000_000, 1_000_000, 1_000_000, 1_000_000); !ok || !near(got, 13.1) {
		t.Errorf("claude-x cost = %v, %v; want 13.1", got, ok)
	}
	// Without cache rates, cached tokens are priced as input.
	if got, _ := c.Cost("gpt-x", 0, 1_000_000, 1_000_000, 0); !near(got, 2) {
		t.Errorf("gpt-x cost = %v, want 2", got)
	}
	if _, ok := c.Cost("unknown", 1, 1, 1, 1); ok {
		t.Error("unknown model priced")
	}
}
