package domain

import (
	"strings"
	"unicode/utf8"
)

// Search limits for one query.
const (
	MaxSearchRunes = 200
	MaxSearchTerms = 8
)

// SearchText is the lowercased haystack a notification is searched in: title,
// raw markdown body and tags, with whitespace runs collapsed. Raw markdown
// keeps link URLs findable, and markup does not get in the way of substrings.
func SearchText(title, body string, tags []string) string {
	parts := append([]string{title, body}, tags...)
	return strings.Join(strings.Fields(strings.ToLower(strings.Join(parts, "\n"))), " ")
}

// SearchTerms splits a user query into lowercased words, all of which must
// match. A blank query returns nil: no search.
func SearchTerms(q string) ([]string, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, nil
	}
	if utf8.RuneCountInString(q) > MaxSearchRunes {
		return nil, Invalid("search must be at most %d characters", MaxSearchRunes)
	}
	terms := strings.Fields(strings.ToLower(q))
	if len(terms) > MaxSearchTerms {
		return nil, Invalid("search must have at most %d words", MaxSearchTerms)
	}
	return terms, nil
}
