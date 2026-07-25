package cache

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"unicode"
)

// Key builds a normalised cache key from a query and intent class.
// Format: opensearch:{intent}:{sha256(normalised_query)}
func Key(query, intent string) string {
	hash := sha256.Sum256([]byte(normalise(query)))
	return fmt.Sprintf("opensearch:%s:%x", intent, hash)
}

// normalise lowercases, strips punctuation, and collapses whitespace.
func normalise(query string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(query) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}