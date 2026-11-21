package application

import (
	"errors"
	"testing"

	review "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain"
)

func TestCardReviewRequiresCurrentSequenceAndEveryWarningNote(t *testing.T) {
	facts := review.Facts{
		Actor:       review.Identity{Employees: []string{"756cf02c-c953-4d3b-bef0-477c69aaef9b"}},
		Author:      review.Identity{Employees: []string{"09696a3b-57eb-4a4f-ba16-750f2bc0ea6d"}},
		Beneficiary: &review.Identity{Employees: []string{"7f086807-79c0-412a-8b54-4f0fa4983500"}},
		Permitted:   true,
	}
	history := []review.Entry{{Kind: review.Submit, Revision: 1, State: review.Submitted}}
	open := ReviewInput{Kind: review.Open, ExpectedSequence: 1}
	entry, err := ApplyReview(history, facts, nil, false, open)
	if err != nil || entry.State != review.InReview {
		t.Fatalf("open fueling review: %v %v", entry, err)
	}
	history = append(history, entry)
	approve := ReviewInput{Kind: review.Approve, ExpectedSequence: 2, WarningNotes: map[string]string{"ARITHMETIC_MISMATCH": "Receipt total checked"}}
	warnings := []string{"ARITHMETIC_MISMATCH"}
	if entry, err = ApplyReview(history, facts, warnings, false, approve); err != nil || entry.State != review.Approved {
		t.Fatalf("approve reviewed fueling: %v %v", entry, err)
	}
	if _, err := ApplyReview(history, facts, warnings, true, approve); !errors.Is(err, ErrDuplicateUnresolved) {
		t.Fatalf("duplicate suspicion returned %v", err)
	}
	missing := approve
	missing.WarningNotes = nil
	if _, err := ApplyReview(history, facts, warnings, false, missing); !errors.Is(err, ErrWarningNotesRequired) {
		t.Fatalf("missing warning note returned %v", err)
	}
	stale := approve
	stale.ExpectedSequence--
	if _, err := ApplyReview(history, facts, warnings, false, stale); !errors.Is(err, review.ErrConflict) {
		t.Fatalf("stale decision returned %v", err)
	}
	facts.Actor = *facts.Beneficiary
	if _, err := ApplyReview(history, facts, warnings, false, approve); !errors.Is(err, review.ErrSelfApproval) {
		t.Fatalf("responsible employee returned %v", err)
	}
}
