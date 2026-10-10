package registry

// RST-06 review-gate unit tests: corroboration policy against the
// in-memory fake with a pass-through transformer (identity on supported
// CRS only; the CRS gate itself runs before any transform call).
// Real PostGIS/PROJ transform proof lives in review_integration_test.go.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type fakeTransformer struct{}

func (fakeTransformer) Transform(_ context.Context, lat, lon float64, crs string) (float64, float64, error) {
	return lat, lon, nil
}

func evidenceDate(day string) pgtype.Date {
	parsed, err := time.Parse("2006-01-02", day)
	if err != nil {
		panic(err)
	}
	return pgtype.Date{Time: parsed, Valid: true}
}

func evidenceRow(sourceKey string, lat, lon float64, observed string) Assertion {
	return Assertion{
		Source:          SourcePrep,
		SourceKey:       sourceKey,
		Checksum:        "evidence",
		DisplayName:     "[RST06-TEST] evidence",
		AuthState:       "unknown",
		Eligibility:     "pending",
		LocationQuality: "unknown",
		SourceReference: "pmqc-sample.csv#V-001",
		EffectiveDate:   evidenceDate(observed),
		Latitude:        lat,
		Longitude:       lon,
		HasCoords:       true,
		CRS:             "EPSG:4326",
	}
}

// stageEvidenceRow stages one evidence row on a completed run and returns
// its staged identity.
func stageEvidenceRow(t *testing.T, store *fakeStore, snapshot string, row Assertion) string {
	t.Helper()
	ctx := context.Background()
	runID := newUUID()
	if _, created, err := store.CreateRun(ctx, runID, SourcePrep, snapshot, "evidence"); err != nil || !created {
		t.Fatalf("create run: %v", err)
	}
	row.Checksum = snapshot + "-checksum"
	if _, err := store.StageAssertion(ctx, row.WithRun(runID)); err != nil {
		t.Fatalf("stage: %v", err)
	}
	if err := store.FinishRun(ctx, runID, "complete", 1, 0, 0, ""); err != nil {
		t.Fatalf("finish: %v", err)
	}
	assertions, err := store.ListAssertions(ctx, runID)
	if err != nil || len(assertions) != 1 {
		t.Fatalf("list = %+v, err = %v", assertions, err)
	}
	return assertions[0].ID
}

func TestReviewCandidatePromotesCorroboratedPair(t *testing.T) {
	store := newFakeStore()
	primary := stageEvidenceRow(t, store, "snap-primary", evidenceRow("48151623000191", -23.5610, -46.6560, "2024-04-01"))
	corroborator := stageEvidenceRow(t, store, "snap-corroborator", evidenceRow("48151623000191", -23.5607, -46.6557, "2024-04-10"))

	decision, err := ReviewCandidate(context.Background(), store, fakeTransformer{}, primary, corroborator, "reviewer:test")
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if decision.DistanceM <= 0 || decision.DistanceM >= ReviewCorroborationM {
		t.Fatalf("distance = %.1f m, want within (0, 150)", decision.DistanceM)
	}
	if decision.CorroboratorID != corroborator {
		t.Fatalf("corroborator = %q", decision.CorroboratorID)
	}
	reviewed, state, err := store.GetAssertion(context.Background(), decision.AssertionID)
	if err != nil || state != "complete" {
		t.Fatalf("reviewed lookup: %+v, %q, %v", reviewed, state, err)
	}
	if reviewed.LocationQuality != "reviewed" || !reviewed.HasCoords || reviewed.CRS != "EPSG:4326" {
		t.Fatalf("reviewed row = %+v", reviewed)
	}
	if reviewed.SourceKey != "48151623000191" {
		t.Fatalf("reviewed CNPJ = %q", reviewed.SourceKey)
	}

	replay, err := ReviewCandidate(context.Background(), store, fakeTransformer{}, primary, corroborator, "reviewer:test")
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.RunID != decision.RunID || replay.AssertionID != decision.AssertionID {
		t.Fatalf("replay diverged: %+v vs %+v", decision, replay)
	}
}

func TestReviewCandidateCorroborationBoundary(t *testing.T) {
	// Pure-latitude offsets: 1 degree is ~111320 m, so these land just
	// inside and just outside the 150 m corroboration bound.
	for _, tc := range []struct {
		name    string
		offset  float64
		promote bool
	}{
		{"149m promotes", 0.001338, true},
		{"151m rejects", 0.001356, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeStore()
			primary := stageEvidenceRow(t, store, "snap-a", evidenceRow("48151623000191", -23.5610, -46.6560, "2024-04-01"))
			corroborator := stageEvidenceRow(t, store, "snap-b", evidenceRow("48151623000191", -23.5610+tc.offset, -46.6560, "2024-04-01"))
			runsBefore := len(store.runs)
			_, err := ReviewCandidate(context.Background(), store, fakeTransformer{}, primary, corroborator, "reviewer:test")
			if tc.promote && err != nil {
				t.Fatalf("boundary promote: %v", err)
			}
			if !tc.promote {
				if err == nil {
					t.Fatal("over-bound pair promoted, want rejection")
				}
				if !strings.Contains(err.Error(), "disagree") {
					t.Fatalf("rejection = %v, want distance verdict", err)
				}
				if len(store.runs) != runsBefore {
					t.Fatal("rejection staged a run")
				}
			}
		})
	}
}

func TestReviewCandidateRejectsWithoutStaging(t *testing.T) {
	far := evidenceRow("48151623000191", -23.4000, -46.5000, "2024-04-01")
	other := evidenceRow("00428184000195", -23.5610, -46.6560, "2024-04-01")
	reviewed := evidenceRow("48151623000191", -23.5607, -46.6557, "2024-04-10")
	reviewed.LocationQuality = "reviewed"
	coordinateless := evidenceRow("48151623000191", -23.5607, -46.6557, "2024-04-10")
	coordinateless.HasCoords = false
	foreignCRS := evidenceRow("48151623000191", -23.5607, -46.6557, "2024-04-10")
	foreignCRS.CRS = "MARS-2000"
	undated := evidenceRow("48151623000191", -23.5607, -46.6557, "2024-04-10")
	undated.EffectiveDate = pgtype.Date{}
	stale := evidenceRow("48151623000191", -23.5607, -46.6557, "2024-08-01")

	cases := map[string]struct {
		primary, corroborator Assertion
		reviewer              string
	}{
		"two establishments":  {evidenceRow("48151623000191", -23.5610, -46.6560, "2024-04-01"), other, "reviewer:test"},
		"far pair":            {evidenceRow("48151623000191", -23.5610, -46.6560, "2024-04-01"), far, "reviewer:test"},
		"already reviewed":    {reviewed, evidenceRow("48151623000191", -23.5610, -46.6560, "2024-04-01"), "reviewer:test"},
		"missing point":       {coordinateless, evidenceRow("48151623000191", -23.5610, -46.6560, "2024-04-01"), "reviewer:test"},
		"unsupported CRS":     {foreignCRS, evidenceRow("48151623000191", -23.5610, -46.6560, "2024-04-01"), "reviewer:test"},
		"undated evidence":    {undated, evidenceRow("48151623000191", -23.5610, -46.6560, "2024-04-01"), "reviewer:test"},
		"stale pairing":       {stale, evidenceRow("48151623000191", -23.5610, -46.6560, "2024-04-01"), "reviewer:test"},
		"anonymous reviewer":  {evidenceRow("48151623000191", -23.5610, -46.6560, "2024-04-01"), evidenceRow("48151623000191", -23.5607, -46.6557, "2024-04-10"), ""},
		"self pairing":        {evidenceRow("48151623000191", -23.5610, -46.6560, "2024-04-01"), evidenceRow("48151623000191", -23.5610, -46.6560, "2024-04-01"), "reviewer:test"},
		"incomplete evidence": {evidenceRow("48151623000191", -23.5610, -46.6560, "2024-04-01"), evidenceRow("48151623000191", -23.5607, -46.6557, "2024-04-10"), "reviewer:test"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			store := newFakeStore()
			primary := stageEvidenceRow(t, store, "snap-primary", tc.primary)
			corroborator := stageEvidenceRow(t, store, "snap-corroborator", tc.corroborator)
			if name == "incomplete evidence" {
				// Reopen the corroborator run: evidence from a running
				// snapshot must not promote.
				for snapshot, report := range store.runs {
					if report.RunID == corroboratorRunID(t, store, corroborator) {
						report.State = "running"
						store.runs[snapshot] = report
					}
				}
			}
			if name == "self pairing" {
				corroborator = primary
			}
			runsBefore := len(store.runs)
			if _, err := ReviewCandidate(context.Background(), store, fakeTransformer{}, primary, corroborator, tc.reviewer); err == nil {
				t.Fatalf("%s promoted, want rejection", name)
			} else if len(store.runs) != runsBefore {
				t.Fatalf("%s staged %d runs, want none", name, len(store.runs)-runsBefore)
			}
		})
	}
}

func corroboratorRunID(t *testing.T, store *fakeStore, corroborator string) string {
	t.Helper()
	assertion, _, err := store.GetAssertion(context.Background(), corroborator)
	if err != nil {
		t.Fatal(err)
	}
	return assertion.RunID()
}

func TestReviewCandidateWiresIntoReconcile(t *testing.T) {
	store := newFakeStore()
	canon := newFakeCanon()
	primary := stageEvidenceRow(t, store, "snap-primary", evidenceRow("48151623000191", -23.5610, -46.6560, "2024-04-01"))
	corroborator := stageEvidenceRow(t, store, "snap-corroborator", evidenceRow("48151623000191", -23.5607, -46.6557, "2024-04-10"))

	decision, err := ReviewCandidate(context.Background(), store, fakeTransformer{}, primary, corroborator, "reviewer:test")
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	report, err := ReconcileRun(ctxForTest(), store, canon, SourceReview, "review:"+primary)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if report.Reconciled != 1 || report.Skipped != 0 {
		t.Fatalf("report = %+v", report)
	}
	point, ok := canon.points["station-for-48151623000191"]
	if !ok {
		t.Fatal("reviewed point never reached the canonicalizer")
	}
	if point[0] != -23.5610 || point[1] != -46.6560 {
		t.Fatalf("projected point = %v", point)
	}
	_ = decision
}

func ctxForTest() context.Context { return context.Background() }
