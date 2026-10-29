// Package text prepares strings for typing.
package text

import (
	"strings"
	"unicode"
)

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
