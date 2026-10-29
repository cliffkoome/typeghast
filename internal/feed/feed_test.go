package feed

import "testing"

func TestAppendItemFiltersAndCleans(t *testing.T) {
	var out []Item
	out = appendItem(out, "Patrick Stewart has died", "Hacker News", "")
	out = appendItem(out, "Hi", "DEV", "")
	out = appendItem(out, "Ask HN: What’s your favourite “boring” tech stack?", "Hacker News", "u")
	if len(out) != 1 || out[0].Text != "What's your favourite boring tech stack" {
		t.Fatalf("got %+v", out)
	}
}
