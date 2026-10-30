package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPollFollowsAppendsAndSkipsSidechains(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	write := func(s string, flag int) {
		f, err := os.OpenFile(path, flag|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		f.WriteString(s)
	}

	write(`{"sessionId":"abc","message":{"model":"m1","usage":{"input_tokens":10,"cache_read_input_tokens":1000,"cache_creation_input_tokens":100,"output_tokens":5}}}`+"\n", os.O_CREATE|os.O_TRUNC)
	tr := NewTracker(path, false)
	if got := tr.Poll(); got.ContextTokens != 1115 || got.Model != "m1" || got.SessionID != "abc" {
		t.Fatalf("first poll: %+v", got)
	}

	// Subagent usage must not count toward the main context, and a line still
	// being written must wait for the next poll.
	write(`{"isSidechain":true,"message":{"usage":{"input_tokens":999999}}}`+"\n", os.O_APPEND)
	write(`{"message":{"usage":{"input_tokens":2000}}}`, os.O_APPEND)
	if got := tr.Poll(); got.ContextTokens != 1115 {
		t.Fatalf("partial line was read: %+v", got)
	}
	write("\n", os.O_APPEND)
	if got := tr.Poll(); got.ContextTokens != 2000 {
		t.Fatalf("after completing line: %+v", got)
	}
}

func TestEventsAreDeduplicated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	edit := `{"message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Edit","input":{"file_path":"/a/b/c.go","new_string":"x := 1"}},{"type":"tool_use","id":"t2","name":"Read","input":{"file_path":"/a/b/c.go"}}]}}` + "\n"
	os.WriteFile(path, []byte(edit+edit), 0o644)
	got := NewTracker(path, false).Poll().Events
	if len(got) != 1 || got[0].Text != "edited b/c.go: x := 1" {
		t.Fatalf("events: %+v", got)
	}
}

func TestSpendCountsEachResponseOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	main := `{"message":{"id":"m1","model":"opus","usage":{"input_tokens":10,"cache_read_input_tokens":1000,"cache_creation_input_tokens":100,"output_tokens":5}}}` + "\n"
	sub := `{"isSidechain":true,"message":{"id":"s1","model":"haiku","usage":{"input_tokens":7,"output_tokens":3}}}` + "\n"
	// The transcript repeats a response once per content block.
	os.WriteFile(path, []byte(main+main+sub), 0o644)
	got := NewTracker(path, false).Poll().Spent
	if got["opus"] != (Usage{10, 100, 1000, 5}) || got["haiku"] != (Usage{Input: 7, Output: 3}) {
		t.Fatalf("spent: %+v", got)
	}
}
