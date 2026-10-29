package session

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Found is a session the player can choose to haunt their game.
type Found struct {
	Agent   string
	Project string
	Path    string
	Updated time.Time
}

// Discover lists agent sessions written to within the last `within`, newest
// first. Only Claude Code is supported so far.
func Discover(within time.Duration) []Found {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	matches, _ := filepath.Glob(filepath.Join(home, ".claude", "projects", "*", "*.jsonl"))
	var out []Found
	for _, m := range matches {
		mt := modTime(m)
		if time.Since(mt) > within {
			continue
		}
		out = append(out, Found{Agent: "Claude Code", Project: projectName(m), Path: m, Updated: mt})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Updated.After(out[j].Updated) })
	return out
}

// projectName reads the working directory recorded in the transcript. The
// directory name it's stored under can't be decoded reliably, since both "/"
// and "-" become "-".
func projectName(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return "?"
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 1<<20)
	for i := 0; i < 50; i++ {
		line, err := r.ReadBytes('\n')
		var e entry
		if json.Unmarshal(line, &e) == nil && e.Cwd != "" {
			return filepath.Base(e.Cwd)
		}
		if err != nil {
			break
		}
	}
	return filepath.Base(filepath.Dir(path))
}
