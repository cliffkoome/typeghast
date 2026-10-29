package game

import "math/rand"

// Placeholder word pool until words come from the files the agent is editing.
var wordPool = []string{
	"ghost", "haunt", "crypt", "lantern", "shadow", "whisper", "candle", "grave",
	"spirit", "hollow", "midnight", "phantom", "wraith", "cobweb", "attic", "creak",
	"function", "return", "buffer", "context", "token", "commit", "branch", "merge",
	"refactor", "pointer", "cache", "socket", "thread", "deploy", "kernel", "parse",
	"session", "render", "prompt", "agent", "window", "stack", "queue", "lambda",
	"daemon", "zombie", "orphan", "process", "signal", "fork", "spawn", "kill",
	"null", "void", "panic", "async", "await", "module", "import", "export",
}

func randomWords(rng *rand.Rand, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = wordPool[rng.Intn(len(wordPool))]
	}
	return out
}
