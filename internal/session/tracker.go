// Package session reads a Claude Code session transcript and reports how full
// the agent's context window is.
package session

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Snapshot is the agent's state as of the latest transcript entry.
type Snapshot struct {
	Path          string
	SessionID     string
	Model         string
	ContextTokens int
	// Events are the actions read by this poll only, oldest first.
	Events []Event
}

// Event is one thing the agent did, described in a line or two of text that
// a model can summarise.
type Event struct {
	Kind string // "edit", "write", "command", "plan", "said"
	Text string
}

// Tracker tails a transcript file. With no fixed path it follows whichever
// transcript under ~/.claude/projects was written most recently, so it moves
// to the new file when the user runs /clear or starts another session.
type Tracker struct {
	fixed string
	path  string
	off   int64
	snap  Snapshot
	seen  map[string]bool
}

func NewTracker(path string) *Tracker {
	return &Tracker{fixed: path}
}

type entry struct {
	IsSidechain bool   `json:"isSidechain"`
	SessionID   string `json:"sessionId"`
	Message     *struct {
		Role    string          `json:"role"`
		Model   string          `json:"model"`
		Content json.RawMessage `json:"content"`
		Usage   *struct {
			InputTokens   int `json:"input_tokens"`
			CacheCreation int `json:"cache_creation_input_tokens"`
			CacheRead     int `json:"cache_read_input_tokens"`
			OutputTokens  int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// Poll reads any entries appended since the last call and returns the
// current snapshot. A zero Path means no transcript was found.
func (t *Tracker) Poll() Snapshot {
	path := t.fixed
	if path == "" {
		path = latestTranscript()
	}
	if path == "" {
		return Snapshot{}
	}
	if path != t.path {
		t.path, t.off = path, 0
		t.snap = Snapshot{Path: path}
		t.seen = map[string]bool{}
	}

	f, err := os.Open(path)
	if err != nil {
		return t.snap
	}
	defer f.Close()
	if _, err := f.Seek(t.off, io.SeekStart); err != nil {
		return t.snap
	}

	t.snap.Events = nil
	r := bufio.NewReaderSize(f, 1<<20)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			// A partial line is still being written; pick it up next poll.
			break
		}
		t.off += int64(len(line))

		var e entry
		if json.Unmarshal(line, &e) != nil || e.IsSidechain || e.Message == nil {
			continue
		}
		if e.Message.Role == "assistant" {
			t.snap.Events = append(t.snap.Events, parseEvents(e.Message.Content, t.seen)...)
		}
		if e.Message.Usage == nil {
			continue
		}
		u := e.Message.Usage
		t.snap.ContextTokens = u.InputTokens + u.CacheCreation + u.CacheRead + u.OutputTokens
		if e.Message.Model != "" {
			t.snap.Model = e.Message.Model
		}
		if e.SessionID != "" {
			t.snap.SessionID = e.SessionID
		}
	}
	return t.snap
}

func latestTranscript() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	matches, _ := filepath.Glob(filepath.Join(home, ".claude", "projects", "*", "*.jsonl"))
	var best string
	var bestMod time.Time
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil {
			continue
		}
		if info.ModTime().After(bestMod) {
			best, bestMod = m, info.ModTime()
		}
	}
	return best
}
