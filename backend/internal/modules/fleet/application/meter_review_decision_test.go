package application

import (
	"fmt"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"math"
	"strings"
	"testing"
)

func TestMeterReviewDecisionPreservesPrivateExactIntent(t *testing.T) {
	in := MeterReviewDecisionInput{OperationID: meterAppID, ReviewItemID: "00000000-0000-4000-8000-000000000002", ExpectedRevision: 1, Decision: MeterReviewRefuse, Reason: "  Observed source needs correction 🚀  "}
	command, e := NewMeterReviewDecision(in)
	if e != nil {
		t.Fatal("valid reasoned refusal", e)
	}
	snapshot, e := command.Snapshot()
	if e != nil || snapshot != in {
		t.Fatal("reason or command normalized")
	}
	scope, _ := kernel.NewOrgScope(mustMeterOrg(t, meterAppID))
	digest := command.Digest(scope)
	if len(digest) != 64 {
		t.Fatal("exact decision digest missing")
	}
	for _, edit := range []func(*MeterReviewDecisionInput){func(v *MeterReviewDecisionInput) { v.ExpectedRevision++ }, func(v *MeterReviewDecisionInput) { v.Reason += " " }, func(v *MeterReviewDecisionInput) { v.Decision = MeterReviewAccept }, func(v *MeterReviewDecisionInput) { v.ReviewItemID = meterAppID }} {
		v := in
		edit(&v)
		changed, e := NewMeterReviewDecision(v)
		if e != nil || changed.Digest(scope) == digest {
			t.Fatal("distinct review intent collapsed", e)
		}
	}
	for _, private := range []string{in.OperationID, in.Reason} {
		if strings.Contains(fmt.Sprintf("%+v", command), private) {
			t.Fatal("decision leaked private diagnostic")
		}
	}
	accepted := in
	accepted.Decision = MeterReviewAccept
	accepted.Reason = ""
	if _, e := NewMeterReviewDecision(accepted); e != nil {
		t.Fatal("acceptance invented mandatory reason", e)
	}
}

func TestMeterReviewDecisionRejectsInvalidShapeAndUnicodeBounds(t *testing.T) {
	base := MeterReviewDecisionInput{OperationID: meterAppID, ReviewItemID: meterAppID, ExpectedRevision: 1, Decision: MeterReviewRefuse, Reason: strings.Repeat("🚀", 280)}
	if _, e := NewMeterReviewDecision(base); e != nil {
		t.Fatal("280 scalar bound miscounted as bytes", e)
	}
	for name, edit := range map[string]func(*MeterReviewDecisionInput){"missing-op": func(v *MeterReviewDecisionInput) { v.OperationID = "" }, "missing-item": func(v *MeterReviewDecisionInput) { v.ReviewItemID = "" }, "zero-revision": func(v *MeterReviewDecisionInput) { v.ExpectedRevision = 0 }, "overflow-revision": func(v *MeterReviewDecisionInput) { v.ExpectedRevision = math.MaxInt64 }, "unknown-choice": func(v *MeterReviewDecisionInput) { v.Decision = "AUTO_ACCEPT" }, "no-refusal-reason": func(v *MeterReviewDecisionInput) { v.Reason = "" }, "blank": func(v *MeterReviewDecisionInput) { v.Reason = " \t " }, "control": func(v *MeterReviewDecisionInput) { v.Reason = "line\nreason" }, "malformed-utf8": func(v *MeterReviewDecisionInput) { v.Reason = string([]byte{0xff}) }, "281-scalars": func(v *MeterReviewDecisionInput) { v.Reason += "🚀" }} {
		t.Run(name, func(t *testing.T) {
			in := base
			edit(&in)
			if _, e := NewMeterReviewDecision(in); e == nil {
				t.Fatal("invalid decision accepted")
			}
		})
	}
}
