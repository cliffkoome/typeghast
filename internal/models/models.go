// Package models knows each model's context window and price, using the
// catalog LiteLLM maintains.
package models

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cliffkoome/typeghast/internal/cache"
)

const catalogURL = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"

// Info is what the game needs to know about a model. Costs are USD per token.
type Info struct {
	MaxInput        int     `json:"max_input_tokens"`
	InputCost       float64 `json:"input_cost_per_token"`
	OutputCost      float64 `json:"output_cost_per_token"`
	CacheReadCost   float64 `json:"cache_read_input_token_cost"`
	CacheWriteCost  float64 `json:"cache_creation_input_token_cost"`
	LiteLLMProvider string  `json:"litellm_provider"`
}

type Catalog map[string]Info

// DefaultWindow is assumed for models the catalog doesn't know.
const DefaultWindow = 200_000

// Load returns the LiteLLM catalog, refreshed at most daily. It returns an
// empty catalog rather than failing, so lookups fall back to defaults.
func Load() Catalog {
	data, err := cache.Get("litellm.json", 24*time.Hour, func() ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, catalogURL, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("litellm catalog: %s", resp.Status)
		}
		return io.ReadAll(resp.Body)
	})
	if err != nil {
		return Catalog{}
	}
	// Some entries hold strings where numbers are expected, so decode each
	// entry on its own and skip the ones that don't fit.
	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) != nil {
		return Catalog{}
	}
	c := make(Catalog, len(raw))
	for k, v := range raw {
		var info Info
		if json.Unmarshal(v, &info) == nil {
			c[k] = info
		}
	}
	return c
}

// Lookup finds a model by the name an agent logs, trying the bare name and
// then common provider prefixes.
func (c Catalog) Lookup(model string) (Info, bool) {
	model = strings.TrimSpace(model)
	if model == "" {
		return Info{}, false
	}
	candidates := []string{model, strings.TrimSuffix(model, "[1m]")}
	if i := strings.LastIndex(model, "/"); i >= 0 {
		candidates = append(candidates, model[i+1:])
	}
	for _, p := range []string{"anthropic/", "openai/", "gemini/", "vertex_ai/"} {
		candidates = append(candidates, p+model)
	}
	for _, k := range candidates {
		if info, ok := c[k]; ok && info.MaxInput > 0 {
			return info, true
		}
	}
	return Info{}, false
}

// Cost prices token counts in USD at the model's API rates. Cache rates
// fall back to the input rate when the catalog lacks them. ok is false for
// models the catalog doesn't know.
func (c Catalog) Cost(model string, input, cacheWrite, cacheRead, output int) (usd float64, ok bool) {
	info, ok := c.Lookup(model)
	if !ok {
		return 0, false
	}
	read, write := info.CacheReadCost, info.CacheWriteCost
	if read == 0 {
		read = info.InputCost
	}
	if write == 0 {
		write = info.InputCost
	}
	return float64(input)*info.InputCost + float64(cacheWrite)*write +
		float64(cacheRead)*read + float64(output)*info.OutputCost, true
}

// Window is the model's context window. If the session has already grown
// past what the catalog says, the agent must be using a larger window (such
// as Claude's 1M option), so the observed size wins.
func (c Catalog) Window(model string, observed int) int {
	w := DefaultWindow
	if info, ok := c.Lookup(model); ok {
		w = info.MaxInput
	}
	if observed > w {
		w = 1_000_000
		for w < observed {
			w *= 2
		}
	}
	return w
}
