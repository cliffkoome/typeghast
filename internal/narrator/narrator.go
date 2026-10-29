// Package narrator asks a local open-weight model, served by Ollama, to turn
// the agent's recent actions into short sentences for the player to type.
// Nothing leaves the machine.
package narrator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/cliffkoome/typeghast/internal/session"
)

type Narrator struct {
	URL   string
	Model string
	http  *http.Client
}

// Detect connects to Ollama and picks a model: the one asked for if it is
// installed, otherwise a Gemma model, otherwise the first one available.
func Detect(url, preferred string) (*Narrator, error) {
	n := &Narrator{URL: strings.TrimRight(url, "/"), http: &http.Client{Timeout: 2 * time.Minute}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, n.URL+"/api/tags", nil)
	resp, err := n.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama not reachable at %s", n.URL)
	}
	defer resp.Body.Close()
	var tags struct {
		Models []struct{ Name string } `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil, err
	}
	if len(tags.Models) == 0 {
		return nil, errors.New("ollama has no models installed")
	}
	names := make([]string, len(tags.Models))
	for i, m := range tags.Models {
		names[i] = m.Name
	}
	n.Model = pick(names, preferred)
	if n.Model == "" {
		return nil, fmt.Errorf("model %q is not installed (run: ollama pull %s)", preferred, preferred)
	}
	return n, nil
}

func pick(installed []string, preferred string) string {
	if preferred != "" {
		for _, m := range installed {
			if m == preferred || strings.TrimSuffix(m, ":latest") == preferred {
				return m
			}
		}
		return ""
	}
	for _, m := range installed {
		if strings.HasPrefix(m, "gemma") {
			return m
		}
	}
	return installed[0]
}

const systemPrompt = `You turn a coding agent's recent actions into short sentences that a developer will type in a typing game while the agent keeps working.

Write like a good changelog: past tense, plain English, saying what was implemented, changed or fixed and why it matters. Merge related actions into one sentence. Skip trivial steps.

Rules:
- 2 to 4 sentences, each 6 to 16 words.
- Only letters, digits, spaces, commas, periods, hyphens and apostrophes.
- You may name files, functions and tools.
- No markdown, quotes, emoji or lists.

Respond with JSON only: {"lines": ["...", "..."]}`

// Narrate summarises events into typeable sentences.
func (n *Narrator) Narrate(ctx context.Context, events []session.Event) ([]string, error) {
	var b strings.Builder
	for _, e := range events {
		b.WriteString("- ")
		b.WriteString(e.Text)
		b.WriteByte('\n')
	}
	body, _ := json.Marshal(map[string]any{
		"model":  n.Model,
		"stream": false,
		"format": "json",
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": "Recent actions, oldest first:\n" + b.String()},
		},
		"options": map[string]any{"temperature": 0.3},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.URL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	resp, err := n.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama: %s", resp.Status)
	}
	var out struct {
		Message struct{ Content string } `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	var parsed struct {
		Lines []string `json:"lines"`
	}
	if err := json.Unmarshal([]byte(out.Message.Content), &parsed); err != nil {
		return nil, fmt.Errorf("model returned invalid JSON: %w", err)
	}
	var lines []string
	for _, l := range parsed.Lines {
		if l = Clean(l); len(strings.Fields(l)) >= 3 {
			lines = append(lines, l)
		}
	}
	if len(lines) == 0 {
		return nil, errors.New("model returned no usable lines")
	}
	return lines, nil
}

// Clean reduces a line to characters that are easy to type on any keyboard.
func Clean(s string) string {
	repl := strings.NewReplacer("’", "'", "‘", "'", "—", ", ", "–", "-", "“", "", "”", "")
	s = repl.Replace(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(r)
		case strings.ContainsRune(" ,.-'_/", r):
			b.WriteRune(r)
		default:
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
