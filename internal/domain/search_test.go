package domain

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestSearchText(t *testing.T) {
	for _, tc := range []struct {
		title, body string
		tags        []string
		want        string
	}{
		{"Бэкап Завершён", "", nil, "бэкап завершён"},
		{"Disk", "**FULL**\n\n on  /srv", []string{"Prod"}, "disk **full** on /srv prod"},
		{"", "", nil, ""},
	} {
		if got := SearchText(tc.title, tc.body, tc.tags); got != tc.want {
			t.Errorf("SearchText(%q, %q, %v) = %q, want %q", tc.title, tc.body, tc.tags, got, tc.want)
		}
	}
}

func TestSearchTerms(t *testing.T) {
	for _, tc := range []struct {
		name    string
		q       string
		want    []string
		invalid bool
	}{
		{"blank", "  ", nil, false},
		{"two words", "Disk  FULL", []string{"disk", "full"}, false},
		{"at limits", strings.Repeat("я", 186) + " a b c d e f g", nil, false},
		{"too long", strings.Repeat("я", 201), nil, true},
		{"too many words", "a b c d e f g h i", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SearchTerms(tc.q)
			if tc.invalid {
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("err = %v, want invalid", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.want != nil && !slices.Equal(got, tc.want) {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
			if tc.q == "  " && got != nil {
				t.Fatalf("blank query: got %q", got)
			}
		})
	}
}
