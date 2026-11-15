package application

import (
	"errors"
	"strings"
	"testing"
	"time"

	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

func decisionInput(kind MeterDecision) MeterDecisionInput {
	v := MeterDecisionInput{OperationID: "7b0e2a54-0000-4000-8000-000000008501", AssetID: "7b0e2a54-0000-4000-8000-000000008201", MeterID: "7b0e2a54-0000-4000-8000-000000008401", ReadingID: "7b0e2a54-0000-4000-8000-000000008404", ExpectedRevision: 1, Decision: kind, Reason: "Dígitos trocados"}
	if kind == MeterSupersede {
		v.Value, _ = meter.NewValue(meter.Odometer, 184_190_000, nil)
	}
	return v
}

// B-BR-EMP-286..288: the decision command is exact, closed and bound by its
// digest; the reason is required, bounded and never free of the digest.
func TestMeterDecisionCommandShapeAndDigest(t *testing.T) {
	org, err := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000008001")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(org)
	if err != nil {
		t.Fatal(err)
	}
	digests := map[string]bool{}
	for _, kind := range []MeterDecision{MeterSupersede, MeterVoid, MeterConfirm} {
		c, err := NewMeterDecisionCommand(decisionInput(kind))
		if err != nil {
			t.Fatal(kind, err)
		}
		digest := c.Digest(scope)
		if len(digest) != 64 || digests[digest] {
			t.Fatal("digest not bound to the decision", kind)
		}
		digests[digest] = true
		again := decisionInput(kind)
		again.Reason = "Another reason"
		other, _ := NewMeterDecisionCommand(again)
		if other.Digest(scope) == digest {
			t.Fatal("reason not bound by the digest")
		}
	}
	corrected := decisionInput(MeterSupersede)
	corrected.Declared = &orgtime.Declaration{OccurredAt: time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC), EntryZone: "America/Cuiaba", DeviceOffsetSeconds: -14400}
	if c, err := NewMeterDecisionCommand(corrected); err != nil || c.Digest(scope) == "" {
		t.Fatal("corrected declaration refused", err)
	}
	value, _ := meter.NewValue(meter.Odometer, 1, nil)
	for name, edit := range map[string]func(*MeterDecisionInput){
		"empty reason":      func(v *MeterDecisionInput) { v.Reason = "" },
		"blank reason":      func(v *MeterDecisionInput) { v.Reason = "   " },
		"long reason":       func(v *MeterDecisionInput) { v.Reason = strings.Repeat("a", 281) },
		"control reason":    func(v *MeterDecisionInput) { v.Reason = "a\nb" },
		"negative revision": func(v *MeterDecisionInput) { v.ExpectedRevision = -1 },
		"unknown decision":  func(v *MeterDecisionInput) { v.Decision = "REOPEN" },
		"bad reading":       func(v *MeterDecisionInput) { v.ReadingID = "R4" },
		"void with value":   func(v *MeterDecisionInput) { v.Decision, v.Value = MeterVoid, value },
		"confirm with time": func(v *MeterDecisionInput) {
			v.Decision, v.Value, v.Declared = MeterConfirm, meter.Value{}, &orgtime.Declaration{}
		},
		"supersede no value": func(v *MeterDecisionInput) { v.Value = meter.Value{} },
		"bad declaration": func(v *MeterDecisionInput) {
			v.Declared = &orgtime.Declaration{OccurredAt: time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC), EntryZone: "Mars/Base", DeviceOffsetSeconds: 0}
		},
	} {
		v := decisionInput(MeterSupersede)
		edit(&v)
		if _, err := NewMeterDecisionCommand(v); !errors.Is(err, meter.ErrInvalid) {
			t.Fatal("invalid decision accepted", name, err)
		}
	}
	if (MeterHistoryFilter{Limit: 0}).Validate() == nil || (MeterHistoryFilter{Limit: 10, State: "HELD"}).Validate() == nil || (MeterHistoryFilter{Limit: 10, AfterID: "x"}).Validate() == nil || (MeterHistoryFilter{Limit: 10, State: meter.Voided}).Validate() != nil {
		t.Fatal("history filter bounds")
	}
}
