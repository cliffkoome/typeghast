package models

import "testing"

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
