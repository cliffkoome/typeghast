// Package feed supplies normal mode with trending developer headlines from
// DEV, Lobsters and Hacker News, falling back to a built-in set offline.
package feed

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/cliffkoome/typeghast/internal/cache"
	"github.com/cliffkoome/typeghast/internal/text"
)

// Item is one sentence to type, with where it came from.
type Item struct {
	Text   string `json:"text"`
	Source string `json:"source"`
	URL    string `json:"url"`
}

const userAgent = "typeghast (+https://github.com/cliffkoome/typeghast)"

// Load returns shuffled headlines, cached for an hour.
func Load() []Item {
	data, err := cache.Get("news.json", time.Hour, func() ([]byte, error) {
		items := fetchAll()
		if len(items) == 0 {
			return nil, fmt.Errorf("no headlines fetched")
		}
		return json.Marshal(items)
	})
	var items []Item
	if err != nil || json.Unmarshal(data, &items) != nil || len(items) == 0 {
		items = Builtin()
	}
	rand.Shuffle(len(items), func(i, j int) { items[i], items[j] = items[j], items[i] })
	return items
}

func fetchAll() []Item {
	sources := []func(context.Context) ([]Item, error){fetchDEV, fetchLobsters, fetchHN}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	var mu sync.Mutex
	var wg sync.WaitGroup
	var all []Item
	for _, f := range sources {
		wg.Add(1)
		go func() {
			defer wg.Done()
			items, err := f(ctx)
			if err != nil {
				return
			}
			mu.Lock()
			all = append(all, items...)
			mu.Unlock()
		}()
	}
	wg.Wait()
	return all
}

func getJSON(ctx context.Context, url string, v any) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("User-Agent", userAgent)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", url, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, v)
}

func fetchDEV(ctx context.Context) ([]Item, error) {
	var articles []struct {
		Title string `json:"title"`
		URL   string `json:"url"`
	}
	if err := getJSON(ctx, "https://dev.to/api/articles?top=2&per_page=30", &articles); err != nil {
		return nil, err
	}
	var out []Item
	for _, a := range articles {
		out = appendItem(out, a.Title, "DEV", a.URL)
	}
	return out, nil
}

func fetchLobsters(ctx context.Context) ([]Item, error) {
	var stories []struct {
		Title    string `json:"title"`
		ShortURL string `json:"short_id_url"`
	}
	if err := getJSON(ctx, "https://lobste.rs/hottest.json", &stories); err != nil {
		return nil, err
	}
	var out []Item
	for _, s := range stories {
		out = appendItem(out, s.Title, "Lobsters", s.ShortURL)
	}
	return out, nil
}

func fetchHN(ctx context.Context) ([]Item, error) {
	var res struct {
		Hits []struct {
			Title    string `json:"title"`
			ObjectID string `json:"objectID"`
		} `json:"hits"`
	}
	if err := getJSON(ctx, "https://hn.algolia.com/api/v1/search?tags=front_page&hitsPerPage=30", &res); err != nil {
		return nil, err
	}
	var out []Item
	for _, h := range res.Hits {
		out = appendItem(out, h.Title, "Hacker News", "https://news.ycombinator.com/item?id="+h.ObjectID)
	}
	return out, nil
}

// offTopic catches front-page stories that aren't about building software
// and shouldn't become typing drills.
var offTopic = []string{"has died", " dies", " dead at", "obituary", "killed", "murder"}

func appendItem(out []Item, title, source, url string) []Item {
	lower := strings.ToLower(title)
	for _, w := range offTopic {
		if strings.Contains(lower, w) {
			return out
		}
	}
	t := text.Clean(strings.TrimPrefix(title, "Ask HN: "))
	if n := len(strings.Fields(t)); n < 3 || n > 20 {
		return out
	}
	return append(out, Item{t, source, url})
}

// Builtin is used when no headlines can be fetched.
func Builtin() []Item {
	lines := []string{
		"There are only two hard things in computer science, cache invalidation and naming things.",
		"Premature optimization is the root of all evil, or at least most of it.",
		"A good commit message explains why the change was made, not just what changed.",
		"Make it work, make it right, then make it fast.",
		"Every line of code you write is a line someone has to maintain.",
		"Tests are the documentation that cannot fall out of date.",
		"The best code review comment is a question, not a command.",
		"Read the error message twice before you search for it.",
		"Small pull requests get reviewed quickly, large ones get approved blindly.",
		"Logs are a letter to your future self at three in the morning.",
		"If it hurts, do it more often, then automate it.",
		"A flaky test is worse than no test, because nobody trusts it.",
		"Delete code with confidence when the tests have your back.",
		"Shipping a small fix today beats a perfect rewrite next quarter.",
		"Name variables for the reader, not for the compiler.",
		"The fastest code is the code that never runs.",
		"Version control is a time machine, so commit early and often.",
		"Measure before you optimize, or you are just guessing.",
		"Simple systems fail in simple ways, clever ones fail in clever ways.",
		"Write the README first and the API will thank you.",
		"An outage is a lesson your monitoring forgot to teach you.",
		"Good defaults matter more than endless configuration options.",
		"Code is read far more often than it is written.",
		"Pair programming turns two half-ideas into one good one.",
	}
	items := make([]Item, len(lines))
	for i, l := range lines {
		items[i] = Item{Text: l, Source: "typeghast"}
	}
	return items
}
