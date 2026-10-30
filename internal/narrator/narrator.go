// Package narrator turns the agent's recent actions into short sentences for
// the player to type.
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

	"github.com/cliffkoome/typeghast/internal/session"
	"github.com/cliffkoome/typeghast/internal/text"
)

// Narrator writes typeable sentences about what the agent did.
type Narrator interface {
	// Name is shown in the footer.
	Name() string
	// Batches reports whether calls are expensive enough that actions should
	// be collected before narrating.
	Batches() bool
	Narrate(ctx context.Context, events []session.Event) (Narration, error)
}

// Narration is what a narrator makes of a batch of actions.
type Narration struct {
	// Lines are the sentences to type.
	Lines []string
	// Ghosts name what is piling up in the agent's context. The game puts
	// them on the ghosts.
	Ghosts []string
}

// maxGhostName keeps a name short enough to float above a ghost.
const maxGhostName = 18

// Ollama asks a local open-weight model to write the sentences. Nothing
// leaves the machine.
type Ollama struct {
	URL   string
	Model string
	http  *http.Client
}

func (n *Ollama) Name() string  { return n.Model }
func (n *Ollama) Batches() bool { return true }

// Detect connects to Ollama and picks a model: the one asked for if it is
// installed, otherwise a Gemma model, otherwise the first one available.
func Detect(url, preferred string) (*Ollama, error) {
	n := &Ollama{URL: strings.TrimRight(url, "/"), http: &http.Client{Timeout: 2 * time.Minute}}
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

Also name 1 to 3 ghosts. Each ghost is something from these actions that now fills the agent's context window: a file it keeps editing, a long command output, a failing test, a plan it carries around. Take each name from the actions above, never invent one. Names are 1 to 3 lowercase words, at most 18 characters.

Respond with JSON only: {"lines": ["...", "..."], "ghosts": ["...", "..."]}`

// Narrate summarises events into typeable sentences.
func (n *Ollama) Narrate(ctx context.Context, events []session.Event) (Narration, error) {
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
		// Thinking models like Gemma 4 otherwise reason for minutes before
		// answering, and sometimes run out before writing the JSON.
		"think": false,
		// Narration comes in bursts; don't reload the model between them.
		"keep_alive": "30m",
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": "Recent actions, oldest first:\n" + b.String()},
		},
		"options": map[string]any{"temperature": 0.3},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.URL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return Narration{}, err
	}
	resp, err := n.http.Do(req)
	if err != nil {
		return Narration{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Narration{}, fmt.Errorf("ollama: %s", resp.Status)
	}
	var out struct {
		Message struct{ Content string } `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Narration{}, err
	}
	var parsed struct {
		Lines  []string `json:"lines"`
		Ghosts []string `json:"ghosts"`
	}
	if err := json.Unmarshal([]byte(out.Message.Content), &parsed); err != nil {
		return Narration{}, fmt.Errorf("model returned invalid JSON: %w", err)
	}
	var res Narration
	for _, l := range parsed.Lines {
		if l = text.Clean(l); len(strings.Fields(l)) >= 3 {
			res.Lines = append(res.Lines, l)
		}
	}
	if len(res.Lines) == 0 {
		return Narration{}, errors.New("model returned no usable lines")
	}
	for _, g := range parsed.Ghosts {
		if g = ghostName(g); g != "" {
			res.Ghosts = append(res.Ghosts, g)
		}
	}
	return res, nil
}

// ghostName tidies a name for display, or returns "" if nothing usable is
// left.
func ghostName(s string) string {
	words := strings.Fields(strings.ToLower(strings.Trim(text.Clean(s), ". ")))
	// Drop whole words until it fits, but keep at least the first.
	for len(words) > 1 && len(strings.Join(words, " ")) > maxGhostName {
		words = words[:len(words)-1]
	}
	s = strings.Join(words, " ")
	if r := []rune(s); len(r) > maxGhostName {
		s = string(r[:maxGhostName])
	}
	return s
}
