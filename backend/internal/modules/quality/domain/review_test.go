package domain

import (
	"errors"
	"testing"
	"time"
)

func TestDocumentFreeReviewDecisions(t *testing.T) {
	for _, tc := range []struct {
		action, reason, status string
		clear                  bool
	}{{"REVIEW", "TRIAGE", "", false}, {"RESOLVE", "CLAIM_RECORDED", "REVIEWED", false}, {"DISMISS", "DUPLICATE", "HIDDEN", false}, {"DISMISS", "INSUFFICIENT_EVIDENCE", "HIDDEN", false}, {"DISMISS", "UNSUPPORTED_LINK", "HIDDEN", false}, {"INVALIDATE", "ABUSE", "HIDDEN", false}, {"INVALIDATE", "PRIVACY", "HIDDEN", true}} {
		got, e := DecideReportReview(tc.action, tc.reason)
		if e != nil || got.Status != tc.status || got.ClearText != tc.clear {
			t.Fatal(tc, got, e)
		}
	}
	for _, tc := range [][2]string{{"BLOCK", "ABUSE"}, {"RESOLVE", "PURCHASE_VERIFIED"}, {"RESOLVE", "ABUSE"}, {"INVALIDATE", "CLAIM_RECORDED"}, {"DISMISS", "free personal text"}} {
		if _, e := DecideReportReview(tc[0], tc[1]); !errors.Is(e, ErrReportInvalid) {
			t.Fatal("unsupported decision", tc, e)
		}
	}
}

func TestDocumentFreeAppealEnvelope(t *testing.T) {
	c := ReportCommand{CommandID: "11111111-1111-4111-8111-111111111111", ReportID: "22222222-2222-4222-8222-222222222222", Action: "APPEAL", ExpectedRevision: 2, Text: "Please review this claim again."}
	if e := c.Validate(time.Now()); e != nil {
		t.Fatal(e)
	}
	c.StationID = "33333333-3333-4333-8333-333333333333"
	if !errors.Is(c.Validate(time.Now()), ErrReportInvalid) {
		t.Fatal("appeal changes target")
	}
}

func TestDocumentFreeSignalsNeverCertifyIndependence(t *testing.T) {
	claims := []SignalClaim{{Owner: "one", Status: "REVIEWED", OwnerActive: true}, {Owner: "one", Status: "REVIEWED", OwnerActive: true}, {Owner: "two", Status: "PENDING_REVIEW", OwnerActive: true}, {Owner: "three", Status: "HIDDEN", OwnerActive: true}, {Owner: "four", Status: "WITHDRAWN", OwnerActive: true}, {Owner: "five", Status: "REVIEWED", OwnerActive: false}, {Owner: "six", Status: "REVIEWED", OwnerActive: true, OfficialLinkSuperseded: true}}
	s := ReviewOnlySignal(claims)
	if s.ReviewedAccounts != 1 || s.DuplicateAccountClaims != 1 || s.Pending != 1 || s.Excluded != 4 || s.Publication != "REVIEW_ONLY" || s.Independence != "UNPROVEN" {
		t.Fatal(s)
	}
	for i := 0; i < 100; i++ {
		claims = append(claims, SignalClaim{Owner: string(rune(1000 + i)), Status: "REVIEWED", OwnerActive: true})
	}
	s = ReviewOnlySignal(claims)
	if s.Publication != "REVIEW_ONLY" || s.Independence != "UNPROVEN" {
		t.Fatal("account farm promoted a claim", s)
	}
}
