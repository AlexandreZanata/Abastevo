package application_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/checklist"
)

// EDR-284/285: the run policies are fixed and equal to the existing ones.
func TestRunPoliciesAreFixed(t *testing.T) {
	if p := app.RunPolicy(app.RunWrite); p.Permission != "check.checklist.execute" || !p.Effect.Write() || p.Fresh {
		t.Fatal("run write policy", p)
	}
	if p := app.RunPolicy(app.RunReceipt); p.Permission != "check.checklist.execute" || p.Effect.Write() {
		t.Fatal("run receipt policy", p)
	}
	if app.RunCompanyPolicy(app.RunRead) != app.Policy(app.ApplicableRead) || app.RunCompanyPolicy(app.RunRead).Permission != "fleet.asset.read" {
		t.Fatal("run read is not the applicable read")
	}
	if p := app.RunCompanyPolicy(app.RunRelease); p != app.IncidentPolicy(app.IncidentDecide) || p.Permission != "check.incident.manage" || !p.Fresh {
		t.Fatal("run release is not the fresh incident handling policy", p)
	}
	if app.RunPolicy(0).Permission != "" || app.RunCompanyPolicy(0).Permission != "" {
		t.Fatal("unknown action has a policy")
	}
}

// B-BR-EMP-442: the release shape; state, revision and separation are the
// store's; the digest binds every field.
func TestReleaseCommandValidatesAndDigests(t *testing.T) {
	op, run := "9c000000-0000-4000-8000-000000004001", "9c000000-0000-4000-8000-000000004002"
	ok := app.ReleaseInput{OperationID: op, RunID: run, ExpectedRevision: 1, Reason: "Freios verificados"}
	command, err := app.NewReleaseCommand(ok)
	if err != nil {
		t.Fatal(err)
	}
	for name, edit := range map[string]func(*app.ReleaseInput){
		"operation": func(v *app.ReleaseInput) { v.OperationID = "x" },
		"run":       func(v *app.ReleaseInput) { v.RunID = strings.ToUpper(run) },
		"revision":  func(v *app.ReleaseInput) { v.ExpectedRevision = 0 },
		"above":     func(v *app.ReleaseInput) { v.ExpectedRevision = 3 },
		"blank":     func(v *app.ReleaseInput) { v.Reason = "   " },
		"empty":     func(v *app.ReleaseInput) { v.Reason = "" },
		"control":   func(v *app.ReleaseInput) { v.Reason = "a\nb" },
		"long":      func(v *app.ReleaseInput) { v.Reason = strings.Repeat("a", 501) },
	} {
		v := ok
		edit(&v)
		if _, err := app.NewReleaseCommand(v); !errors.Is(err, checklist.ErrInvalid) {
			t.Fatal("invalid release accepted", name, err)
		}
	}
	org, err := kernel.ParseOrgID("9c000000-0000-4000-8000-000000004003")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(org)
	if err != nil {
		t.Fatal(err)
	}
	digest := command.Digest(scope)
	other := ok
	other.Reason = "Outro motivo"
	otherCommand, _ := app.NewReleaseCommand(other)
	if len(digest) != 64 || digest == otherCommand.Digest(scope) || command.Digest(kernel.OrgScope{}) != "" {
		t.Fatal("digest does not bind the reason or the scope")
	}
}

// EMP-CHECK-004 (B-BR-EMP-506, EDR-328): the queue policy is the incident
// queue's; keys order BLOCKING, then review, then the rest, newest first;
// cursors round-trip only in their canonical form; filters are bounded.
func TestRunQueueKeysFiltersAndPolicyAreBounded(t *testing.T) {
	if p := app.RunCompanyPolicy(app.RunQueue); p != app.IncidentPolicy(app.IncidentQueue) || p.Permission != "check.incident.manage" || p.Fresh || p.Effect.Write() {
		t.Fatal("run queue is not the incident queue policy", p)
	}
	at := time.Date(2026, 11, 16, 12, 0, 0, 123456789, time.UTC)
	run := func(id string, outcome, device checklist.Outcome, completed time.Time) app.RunView {
		return app.RunView{RunID: id, Outcome: outcome, RecheckOutcome: outcome, DeviceOutcome: device, Differs: device != outcome, CompletedAt: completed}
	}
	a, b := "9c000000-0000-4000-8000-000000004101", "9c000000-0000-4000-8000-000000004102"
	blocking := app.KeyOf(run(b, checklist.Blocking, checklist.Blocking, at.Add(-time.Hour)))
	review := app.KeyOf(run(a, checklist.CompletedWithIssues, checklist.CompletedOK, at))
	plainNew := app.KeyOf(run(a, checklist.CompletedOK, checklist.CompletedOK, at))
	plainOld := app.KeyOf(run(b, checklist.CompletedOK, checklist.CompletedOK, at.Add(-time.Minute)))
	tie := app.KeyOf(run(b, checklist.CompletedOK, checklist.CompletedOK, at))
	if blocking.Group != app.RunGroupBlocking || review.Group != app.RunGroupReview || plainNew.Group != app.RunGroupOther || !plainNew.CompletedAt.Equal(at.Truncate(time.Microsecond)) {
		t.Fatal("run groups", blocking.Group, review.Group, plainNew.Group)
	}
	for name, pair := range map[string][2]app.RunQueueKey{"blocking first": {blocking, review}, "review before others": {review, plainOld}, "newest first": {plainNew, plainOld}, "id breaks ties": {plainNew, tie}} {
		if !pair[0].Before(pair[1]) || pair[1].Before(pair[0]) || pair[0].Same(pair[1]) {
			t.Fatal("queue order", name)
		}
	}
	for _, key := range []app.RunQueueKey{blocking, review, plainNew, plainOld} {
		parsed, err := app.ParseRunQueueKey(key.Cursor())
		if err != nil || parsed == nil || !parsed.Same(key) {
			t.Fatal("cursor round trip", key.Cursor(), err)
		}
	}
	if k, err := app.ParseRunQueueKey(""); k != nil || err != nil {
		t.Fatal("empty cursor is the first page", k, err)
	}
	for _, raw := range []string{"3|1|" + a, "0|x|" + a, "0|1|" + strings.ToUpper(a), "00|1|" + a, "0|01|" + a, "0|1", "0|1|" + a + "|x", "-1|1|" + a, "0|1|00000000-0000-0000-0000-000000000000"} {
		if _, err := app.ParseRunQueueKey(raw); err == nil {
			t.Fatal("non-canonical cursor accepted", raw)
		}
	}
	if (app.RunQueueKey{Group: 3, CompletedAt: at, RunID: a}).Cursor() != "" {
		t.Fatal("invalid key has a cursor")
	}
	for name, f := range map[string]app.RunQueueFilter{"zero": {Limit: 0}, "large": {Limit: 101}, "bad key": {Limit: 1, After: &app.RunQueueKey{Group: 9, CompletedAt: at, RunID: a}}} {
		if f.Validate() == nil {
			t.Fatal("invalid filter", name)
		}
	}
	if (app.RunQueueFilter{Limit: 100, After: &plainOld}).Validate() != nil {
		t.Fatal("valid filter refused")
	}
}
