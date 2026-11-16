package checklist_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/checklist"
)

// EMP-CHECK-002 (B-BR-EMP-440): the server recheck of a completed run.

const (
	runAssetB    = "9c000000-0000-4000-8000-000000002002"
	runTemplate  = "9c000000-0000-4000-8000-000000003001"
	runCategoryC = "9c000000-0000-4000-8000-000000001003"
)

var (
	t0 = time.Date(2026, 11, 15, 8, 0, 0, 0, time.UTC)
	// Items: 1 lights (optional), 2 brakes (critical), 3 tyres.
	runItems = []checklist.Item{{Label: "Luzes", Optional: true}, {Label: "Freios", Critical: true}, {Label: "Pneus"}}
)

func at(minutes int) time.Time { return t0.Add(time.Duration(minutes) * time.Minute) }

// facts: version 1 published at t0, version 2 at +600 min; policy 1 BLOCK at
// t0, policy 2 WARN at +120 min, policy 3 BLOCK at +240 min.
func runFacts() checklist.RunFacts {
	return checklist.RunFacts{
		Context: checklist.Departure, Version: 1,
		Target:   checklist.Target{Kind: checklist.TargetCategories, IDs: []string{categoryA, categoryB}},
		Items:    slices.Clone(runItems),
		Versions: []checklist.VersionStamp{{Version: 1, PublishedAt: t0}, {Version: 2, PublishedAt: at(600)}},
		Policies: []checklist.PolicyRevision{
			{Revision: 1, Policy: checklist.Policy{Mode: checklist.Block, ValiditySeconds: 43200}, EffectiveAt: t0},
			{Revision: 2, Policy: checklist.Policy{Mode: checklist.Warn, ValiditySeconds: 43200}, EffectiveAt: at(120)},
			{Revision: 3, Policy: checklist.Policy{Mode: checklist.Block, ValiditySeconds: 7200}, EffectiveAt: at(240)},
		},
	}
}

func runAnswers(values ...checklist.Answer) []checklist.Response {
	out := make([]checklist.Response, len(values))
	for i, v := range values {
		out[i] = checklist.Response{Answer: v}
	}
	return out
}

func claim(policy int64, completed int, device checklist.Outcome, values ...checklist.Answer) checklist.RunClaim {
	return checklist.RunClaim{Context: checklist.Departure, AssetID: assetA, CategoryID: categoryA, Version: 1, PolicyRevision: policy,
		StartedAt: at(completed - 10), CompletedAt: at(completed), Responses: runAnswers(values...), DeviceOutcome: device}
}

const (
	ok = checklist.AnswerOK
	no = checklist.AnswerNotOK
	na = checklist.AnswerNotApplicable
)

func TestParseOutcomeIsClosed(t *testing.T) {
	for _, raw := range []string{"COMPLETED_OK", "COMPLETED_WITH_ISSUES", "BLOCKING"} {
		if o, err := checklist.ParseOutcome(raw); err != nil || string(o) != raw {
			t.Fatal("outcome", raw, err)
		}
	}
	for _, raw := range []string{"", "IN_PROGRESS", "blocking", "BLOCKING "} {
		if _, err := checklist.ParseOutcome(raw); !errors.Is(err, checklist.ErrInvalid) {
			t.Fatal("outcome accepted", raw)
		}
	}
}

func TestRecheckAgreesWithTheDeviceUnderThePolicyInForce(t *testing.T) {
	for _, tc := range []struct {
		name     string
		claim    checklist.RunClaim
		outcome  checklist.Outcome
		policy   int64
		issues   []int
		critical []int
		ack      bool
	}{
		{"all ok under BLOCK", claim(1, 30, checklist.CompletedOK, ok, ok, ok), checklist.CompletedOK, 1, nil, nil, false},
		{"optional not applicable", claim(1, 30, checklist.CompletedOK, na, ok, ok), checklist.CompletedOK, 1, nil, nil, false},
		{"non critical issue", claim(1, 30, checklist.CompletedWithIssues, ok, ok, no), checklist.CompletedWithIssues, 1, []int{3}, nil, false},
		{"critical under BLOCK", claim(1, 30, checklist.Blocking, ok, no, ok), checklist.Blocking, 1, []int{2}, []int{2}, false},
		{"critical under WARN", claim(2, 180, checklist.CompletedWithIssues, ok, no, no), checklist.CompletedWithIssues, 2, []int{2, 3}, []int{2}, true},
		{"policy boundary is in force", claim(2, 120, checklist.CompletedWithIssues, ok, no, ok), checklist.CompletedWithIssues, 2, []int{2}, []int{2}, true},
		{"third revision BLOCK", claim(3, 300, checklist.Blocking, no, no, ok), checklist.Blocking, 3, []int{1, 2}, []int{2}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.ack {
				tc.claim.Acknowledged = true
			}
			got, err := checklist.RecheckRun(runFacts(), tc.claim)
			if err != nil {
				t.Fatal(err)
			}
			if got.Evaluation.Outcome != tc.outcome || got.PolicyRevision != tc.policy || got.DeviceOutcome != tc.claim.DeviceOutcome || got.Differs || got.Superseded ||
				!slices.Equal(got.Evaluation.Issues, tc.issues) || !slices.Equal(got.Evaluation.Critical, tc.critical) || got.Evaluation.AcknowledgementRequired != tc.ack {
				t.Fatalf("%+v", got)
			}
		})
	}
}

func TestRecheckStoresTheServerOutcomeWhenTheDeviceDisagrees(t *testing.T) {
	// A stale WARN revision on the device while BLOCK was in force (BR-CHECK-007, maintenance §12 AC3).
	stale := claim(2, 300, checklist.CompletedWithIssues, ok, no, ok)
	stale.Acknowledged = true
	got, err := checklist.RecheckRun(runFacts(), stale)
	if err != nil || got.Evaluation.Outcome != checklist.Blocking || got.PolicyRevision != 3 || got.DeviceOutcome != checklist.CompletedWithIssues || !got.Differs {
		t.Fatalf("stale WARN: %+v %v", got, err)
	}
	// A stale BLOCK revision while WARN was in force: the server is less strict and still governs.
	lenient, err := checklist.RecheckRun(runFacts(), claim(1, 150, checklist.Blocking, ok, no, ok))
	if err != nil || lenient.Evaluation.Outcome != checklist.CompletedWithIssues || lenient.PolicyRevision != 2 || !lenient.Differs || !lenient.Evaluation.AcknowledgementRequired {
		t.Fatalf("stale BLOCK: %+v %v", lenient, err)
	}
	// A critical block ignored by the device under its own BLOCK revision.
	ignored, err := checklist.RecheckRun(runFacts(), claim(1, 30, checklist.CompletedOK, ok, no, ok))
	if err != nil || ignored.Evaluation.Outcome != checklist.Blocking || ignored.DeviceOutcome != checklist.CompletedOK || !ignored.Differs {
		t.Fatalf("ignored block: %+v %v", ignored, err)
	}
	// A completion declared before the first revision uses the first revision.
	early := claim(1, 0, checklist.Blocking, ok, no, ok)
	early.StartedAt, early.CompletedAt = t0.Add(-time.Hour), t0.Add(-time.Minute)
	first, err := checklist.RecheckRun(runFacts(), early)
	if err != nil || first.PolicyRevision != 1 || first.Evaluation.Outcome != checklist.Blocking || first.Differs {
		t.Fatalf("before first revision: %+v %v", first, err)
	}
}

func TestRecheckFlagsASupersededTemplate(t *testing.T) {
	late := claim(3, 600, checklist.Blocking, ok, no, ok)
	got, err := checklist.RecheckRun(runFacts(), late)
	if err != nil || !got.Superseded || got.Differs {
		t.Fatalf("version 2 published at the completion: %+v %v", got, err)
	}
	before := claim(3, 599, checklist.Blocking, ok, no, ok)
	if got, err := checklist.RecheckRun(runFacts(), before); err != nil || got.Superseded {
		t.Fatalf("version 2 not yet published: %+v %v", got, err)
	}
	archived := runFacts()
	when := at(60)
	archived.ArchivedAt = &when
	if got, err := checklist.RecheckRun(archived, claim(1, 60, checklist.CompletedOK, ok, ok, ok)); err != nil || !got.Superseded {
		t.Fatalf("archived at the completion: %+v %v", got, err)
	}
	if got, err := checklist.RecheckRun(archived, claim(1, 59, checklist.CompletedOK, ok, ok, ok)); err != nil || got.Superseded {
		t.Fatalf("archived after the completion: %+v %v", got, err)
	}
	// The second version itself is current.
	second := runFacts()
	second.Version = 2
	c := claim(3, 700, checklist.CompletedOK, ok, ok, ok)
	c.Version = 2
	if got, err := checklist.RecheckRun(second, c); err != nil || got.Superseded {
		t.Fatalf("current version: %+v %v", got, err)
	}
}

func TestRecheckTargetsCoverTheAsset(t *testing.T) {
	asset := runFacts()
	asset.Target = checklist.Target{Kind: checklist.TargetAssets, IDs: []string{assetA}}
	org := runFacts()
	org.Target = checklist.Target{Kind: checklist.TargetOrganization}
	for name, facts := range map[string]checklist.RunFacts{"category": runFacts(), "asset": asset, "organization": org} {
		if _, err := checklist.RecheckRun(facts, claim(1, 30, checklist.CompletedOK, ok, ok, ok)); err != nil {
			t.Fatal(name, err)
		}
	}
	other := claim(1, 30, checklist.CompletedOK, ok, ok, ok)
	other.AssetID, other.CategoryID = runAssetB, runCategoryC
	for name, facts := range map[string]checklist.RunFacts{"category": runFacts(), "asset": asset} {
		if _, err := checklist.RecheckRun(facts, other); !errors.Is(err, checklist.ErrInvalid) {
			t.Fatal("wrong template accepted", name, err)
		}
	}
	if _, err := checklist.RecheckRun(org, other); err != nil {
		t.Fatal("organization default covers every asset", err)
	}
}

func TestRecheckRefusesMalformedRuns(t *testing.T) {
	base := func() checklist.RunClaim { return claim(1, 30, checklist.CompletedOK, ok, ok, ok) }
	for name, mutate := range map[string]func(*checklist.RunFacts, *checklist.RunClaim){
		"context":         func(_ *checklist.RunFacts, c *checklist.RunClaim) { c.Context = checklist.Return },
		"bad context":     func(_ *checklist.RunFacts, c *checklist.RunClaim) { c.Context = "PARKING" },
		"unknown version": func(_ *checklist.RunFacts, c *checklist.RunClaim) { c.Version = 3 },
		"zero version":    func(_ *checklist.RunFacts, c *checklist.RunClaim) { c.Version = 0 },
		"facts other":     func(f *checklist.RunFacts, _ *checklist.RunClaim) { f.Version = 2 },
		"version gap":     func(f *checklist.RunFacts, _ *checklist.RunClaim) { f.Versions[1].Version = 3 },
		"version order":   func(f *checklist.RunFacts, _ *checklist.RunClaim) { f.Versions[1].PublishedAt = t0.Add(-time.Second) },
		"no versions":     func(f *checklist.RunFacts, _ *checklist.RunClaim) { f.Versions = nil },
		"unknown policy":  func(_ *checklist.RunFacts, c *checklist.RunClaim) { c.PolicyRevision = 4 },
		"zero policy":     func(_ *checklist.RunFacts, c *checklist.RunClaim) { c.PolicyRevision = 0 },
		"no policies":     func(f *checklist.RunFacts, _ *checklist.RunClaim) { f.Policies = nil },
		"policy gap":      func(f *checklist.RunFacts, _ *checklist.RunClaim) { f.Policies[2].Revision = 4 },
		"fewer answers":   func(_ *checklist.RunFacts, c *checklist.RunClaim) { c.Responses = c.Responses[:2] },
		"more answers": func(_ *checklist.RunFacts, c *checklist.RunClaim) {
			c.Responses = append(c.Responses, checklist.Response{Answer: ok})
		},
		"start after": func(_ *checklist.RunFacts, c *checklist.RunClaim) { c.StartedAt = c.CompletedAt.Add(time.Microsecond) },
		"zero completion": func(_ *checklist.RunFacts, c *checklist.RunClaim) {
			c.CompletedAt, c.StartedAt = time.Time{}, time.Time{}
		},
		"device outcome":    func(_ *checklist.RunFacts, c *checklist.RunClaim) { c.DeviceOutcome = "IN_PROGRESS" },
		"empty outcome":     func(_ *checklist.RunFacts, c *checklist.RunClaim) { c.DeviceOutcome = "" },
		"acknowledged ok":   func(_ *checklist.RunFacts, c *checklist.RunClaim) { c.Acknowledged = true },
		"asset id":          func(_ *checklist.RunFacts, c *checklist.RunClaim) { c.AssetID = "x" },
		"category id":       func(_ *checklist.RunFacts, c *checklist.RunClaim) { c.CategoryID = "" },
		"bad answer":        func(_ *checklist.RunFacts, c *checklist.RunClaim) { c.Responses[0].Answer = "MAYBE" },
		"bad note":          func(_ *checklist.RunFacts, c *checklist.RunClaim) { c.Responses[0].Note = "a\nb" },
		"facts context":     func(f *checklist.RunFacts, _ *checklist.RunClaim) { f.Context = checklist.Return },
		"facts items":       func(f *checklist.RunFacts, _ *checklist.RunClaim) { f.Items = nil },
		"critical optional": func(f *checklist.RunFacts, _ *checklist.RunClaim) { f.Items[1].Optional = true },
		"bad target": func(f *checklist.RunFacts, _ *checklist.RunClaim) {
			f.Target = checklist.Target{Kind: checklist.TargetAssets}
		},
		"bad policy history": func(f *checklist.RunFacts, _ *checklist.RunClaim) { f.Policies[1].EffectiveAt = t0.Add(-time.Second) },
	} {
		t.Run(name, func(t *testing.T) {
			facts, c := runFacts(), base()
			mutate(&facts, &c)
			if _, err := checklist.RecheckRun(facts, c); !errors.Is(err, checklist.ErrInvalid) {
				t.Fatalf("accepted: %v", err)
			}
		})
	}
	for name, tc := range map[string]struct {
		c   checklist.RunClaim
		err error
	}{
		"not applicable on a required item": {claim(1, 30, checklist.CompletedOK, ok, na, ok), checklist.ErrAnswerNotAllowed},
		"unanswered item":                   {claim(1, 30, checklist.CompletedOK, ok, checklist.Unanswered, ok), checklist.ErrIncomplete},
	} {
		if _, err := checklist.RecheckRun(runFacts(), tc.c); !errors.Is(err, tc.err) {
			t.Fatal(name, err)
		}
	}
	photo := runFacts()
	photo.Items[2].PhotoOnNotOK = true
	if _, err := checklist.RecheckRun(photo, claim(1, 30, checklist.CompletedWithIssues, ok, ok, no)); !errors.Is(err, checklist.ErrPhotoRequired) {
		t.Fatal("photo required", err)
	}
	withPhoto := claim(1, 30, checklist.CompletedWithIssues, ok, ok, no)
	withPhoto.Responses[2].Photo = true
	if got, err := checklist.RecheckRun(photo, withPhoto); err != nil || got.Evaluation.Outcome != checklist.CompletedWithIssues {
		t.Fatal("photo declared", got, err)
	}
}

func TestRecheckDoesNotAliasItsInputs(t *testing.T) {
	facts, c := runFacts(), claim(1, 30, checklist.Blocking, ok, no, no)
	got, err := checklist.RecheckRun(facts, c)
	if err != nil {
		t.Fatal(err)
	}
	got.Evaluation.Issues[0] = 99
	again, err := checklist.RecheckRun(facts, c)
	if err != nil || again.Evaluation.Issues[0] != 2 || facts.Items[1].Label != "Freios" || c.Responses[1].Answer != no {
		t.Fatal("aliased", again, err)
	}
}

// B-BR-EMP-439: a present note is 1..140 scalars, nonblank, control-free.
func TestValidNoteIsThePresentNoteProfile(t *testing.T) {
	for _, ok := range []string{"Freios rangendo", strings.Repeat("é", 140), "a"} {
		if !checklist.ValidNote(ok) {
			t.Fatal("note refused", ok)
		}
	}
	for _, bad := range []string{"", "   ", "a\nb", strings.Repeat("é", 141), "\x7f"} {
		if checklist.ValidNote(bad) {
			t.Fatal("note accepted", bad)
		}
	}
}
