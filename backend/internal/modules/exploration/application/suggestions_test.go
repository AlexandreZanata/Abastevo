package application

import (
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/exploration/domain"
	"time"
)

func TestSuggestionsAreFrozenResolvableAndLabelled(t *testing.T) {
	suggestions := Suggestions()
	if len(suggestions) != 6 {
		t.Fatal("suggestion set drifted", len(suggestions))
	}
	seen := map[string]bool{}
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	for _, suggestion := range suggestions {
		if suggestion.Label == "" || suggestion.Question == "" || seen[suggestion.Question] {
			t.Fatal("suggestion malformed or duplicated", suggestion)
		}
		seen[suggestion.Question] = true
		got := domain.Interpret(suggestion.Question, now)
		if got.State != domain.StateResolved {
			t.Fatalf("suggestion %q unresolved: %+v", suggestion.Label, got)
		}
	}
}
