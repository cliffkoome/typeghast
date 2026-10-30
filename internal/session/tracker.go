// Package session reads a coding agent's session transcript and reports how
// full its context window is and what it has been doing.
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
	// Updated is when the transcript was last written, to tell a working
	// agent from an idle or finished one.
	Updated time.Time
	// Events are the actions read by this poll only, oldest first.
	Events []Event
	// Spent is every token the agent has used since tracking began, by
	// model, subagents included. It carries across /clear.
	Spent map[string]Usage
}

// Usage counts tokens the way the API bills them.
type Usage struct {
	Input, CacheWrite, CacheRead, Output int
}

func (u Usage) add(o Usage) Usage {
	return Usage{u.Input + o.Input, u.CacheWrite + o.CacheWrite, u.CacheRead + o.CacheRead, u.Output + o.Output}
}

// Event is one thing the agent did, described in a line or two of text that
// a model can summarise.
type Event struct {
	Kind string // "edit", "write", "command", "plan", "said"
	Text string
}

// Tracker tails one session's transcript. When follow is set it also moves to
// a newer transcript in the same project directory, which is what Claude Code
// creates on /clear.
type Tracker struct {
	path   string
	follow bool
	off    int64
	snap   Snapshot
	seen   map[string]bool
	// billed holds each API response's usage by message id, because the
	// transcript repeats a response once per content block.
	billed map[string]billed
}

type billed struct {
	model string
	usage Usage
}

func NewTracker(path string, follow bool) *Tracker {
	return &Tracker{path: path, follow: follow, snap: Snapshot{Path: path}, seen: map[string]bool{}, billed: map[string]billed{}}
}

type entry struct {
	IsSidechain bool   `json:"isSidechain"`
	SessionID   string `json:"sessionId"`
	Cwd         string `json:"cwd"`
	Message     *struct {
		ID      string          `json:"id"`
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
// current snapshot.
func (t *Tracker) Poll() Snapshot {
	if t.follow {
		if newer := newestIn(filepath.Dir(t.path)); newer != "" && newer != t.path && modTime(newer).After(modTime(t.path)) {
			t.path, t.off = newer, 0
			t.snap = Snapshot{Path: newer}
			t.seen = map[string]bool{}
		}
	}

	f, err := os.Open(t.path)
	if err != nil {
		return t.snap
	}
	defer f.Close()
	if info, err := f.Stat(); err == nil {
		t.snap.Updated = info.ModTime()
	}
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
		if json.Unmarshal(line, &e) != nil || e.Message == nil {
			continue
		}
		if u := e.Message.Usage; u != nil && e.Message.ID != "" {
			t.billed[e.Message.ID] = billed{e.Message.Model, Usage{u.InputTokens, u.CacheCreation, u.CacheRead, u.OutputTokens}}
		}
		if e.IsSidechain {
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
	t.snap.Spent = map[string]Usage{}
	for _, b := range t.billed {
		t.snap.Spent[b.model] = t.snap.Spent[b.model].add(b.usage)
	}
	return t.snap
}

func modTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

func newestIn(dir string) string {
	matches, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	var best string
	var bestMod time.Time
	for _, m := range matches {
		if mt := modTime(m); mt.After(bestMod) {
			best, bestMod = m, mt
		}
	}
	return best
}
