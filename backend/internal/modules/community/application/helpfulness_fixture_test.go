package application

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
)

// CS01 contract freeze: golden helpfulness vectors pin the wire shapes
// (reference + immutable revision + frozen choice, exact tallies, owner
// overlay). Valid vectors must construct through the domain; invalid
// ones must refuse. Money/product vocabularies stay pinned by the
// existing consensus and apicontract goldens, not duplicated here.

type helpfulVector struct {
	ID    string `json:"id"`
	Valid bool   `json:"valid"`
	Value struct {
		ObservationID      string  `json:"observation_id"`
		DecisionSequence   int64   `json:"decision_sequence"`
		Choice             string  `json:"choice"`
		ClientSubmissionID string  `json:"client_submission_id"`
		Helpful            int64   `json:"helpful"`
		NotHelpful         int64   `json:"not_helpful"`
		HelpfulBasisPoints int64   `json:"helpful_basis_points"`
		HasVotes           bool    `json:"has_votes"`
		MyChoice           *string `json:"my_choice"`
		HasBasisPoints     bool    `json:"-"`
	} `json:"value"`
}

func TestHelpfulnessGoldenVectors(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	dir := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "..", "contracts", "testdata", "community-helpfulness")
	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil || len(matches) != 4 {
		t.Fatalf("expected 4 helpfulness vectors, got %d (%v)", len(matches), err)
	}
	now := time.Unix(1_700_000_000, 0).UTC()
	for _, path := range matches {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", filepath.Base(path), err)
		}
		var v helpfulVector
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatalf("decode %s: %v", filepath.Base(path), err)
		}
		switch v.ID {
		case "helpful-cast-valid", "helpful-cast-invalid-choice":
			_, _, err := domain.NewVote(domain.VoteParams{
				ID: "vote-fixture", ContributorRef: "contrib-a", AuthorRef: "contrib-b",
				ObservationID: v.Value.ObservationID, DecisionSequence: v.Value.DecisionSequence,
				Choice: domain.Choice(v.Value.Choice), ClientSubmissionID: v.Value.ClientSubmissionID,
				ReceivedAt: now,
			})
			if v.Valid && err != nil {
				t.Fatalf("%s must construct: %v", v.ID, err)
			}
			if !v.Valid && err == nil {
				t.Fatalf("%s must refuse", v.ID)
			}
		case "helpful-tally-with-votes", "helpful-tally-empty":
			share, err := domain.ComputeShare(v.Value.Helpful, v.Value.NotHelpful)
			if err != nil {
				t.Fatalf("%s must compute: %v", v.ID, err)
			}
			if share.HasVotes != v.Value.HasVotes {
				t.Fatalf("%s has_votes mismatch: %+v", v.ID, share)
			}
			if v.Value.HasVotes && share.BasisPoints != v.Value.HelpfulBasisPoints {
				t.Fatalf("%s basis points mismatch: %+v", v.ID, share)
			}
			if v.Value.MyChoice != nil && !domain.ValidChoice(domain.Choice(*v.Value.MyChoice)) {
				t.Fatalf("%s owner overlay must be a frozen choice", v.ID)
			}
		default:
			t.Fatalf("unexpected vector %s", v.ID)
		}
	}
}
