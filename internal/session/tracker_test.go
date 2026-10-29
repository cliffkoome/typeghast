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
	tr := NewTracker(path)
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
