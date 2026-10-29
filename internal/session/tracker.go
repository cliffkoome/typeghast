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
}

func NewTracker(path string, follow bool) *Tracker {
	return &Tracker{path: path, follow: follow, snap: Snapshot{Path: path}, seen: map[string]bool{}}
}

type entry struct {
	IsSidechain bool   `json:"isSidechain"`
	SessionID   string `json:"sessionId"`
	Cwd         string `json:"cwd"`
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
