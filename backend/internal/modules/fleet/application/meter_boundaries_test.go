package application

import (
	"errors"
	"testing"
	"time"

	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

func changeInput(t *testing.T, change MeterChange) MeterChangeInput {
	t.Helper()
	value := func(n int64) meter.Value {
		v, err := meter.NewValue(meter.Odometer, n, nil)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	in := MeterChangeInput{OperationID: "7b0e2a54-0000-4000-8000-000000008511", AssetID: "7b0e2a54-0000-4000-8000-000000008201", MeterID: "7b0e2a54-0000-4000-8000-000000008401",
		ExpectedSegments: 1, Change: change, Declared: orgtime.Declaration{OccurredAt: time.Date(2026, 11, 8, 13, 0, 0, 0, time.UTC), EntryZone: "America/Cuiaba", DeviceOffsetSeconds: -14400}}
	if change == MeterReplace {
		final := value(184_532_000)
		in.Final, in.Label, in.Opening = &final, "Synthetic panel", value(0)
	} else {
		in.Before, in.After = value(184_532_000), value(0)
	}
	return in
}

// EMP-METER-005: the change command validates its own shape, copies its
// pointers and binds every field into its digest.
func TestMeterChangeCommandShapeAndDigest(t *testing.T) {
	org, err := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000008001")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(org)
	if err != nil {
		t.Fatal(err)
	}
	digests := map[string]bool{}
	for _, in := range []MeterChangeInput{changeInput(t, MeterReplace), changeInput(t, MeterReset)} {
		c, err := NewMeterChangeCommand(in)
		if err != nil {
			t.Fatal(in.Change, err)
		}
		digests[c.Digest(scope)] = true
	}
	unknown := changeInput(t, MeterReplace)
	unknown.Final = nil
	modulus := int64(1_000_000_000)
	withModulus := changeInput(t, MeterReplace)
	withModulus.Modulus = &modulus
	later := changeInput(t, MeterReset)
	later.ExpectedSegments = 2
	for _, in := range []MeterChangeInput{unknown, withModulus, later} {
		c, err := NewMeterChangeCommand(in)
		if err != nil {
			t.Fatal(err)
		}
		digests[c.Digest(scope)] = true
	}
	if len(digests) != 5 || digests[""] {
		t.Fatal("the digest does not bind every field", len(digests))
	}
	c, _ := NewMeterChangeCommand(withModulus)
	modulus = 1
	if in, _ := c.Snapshot(); *in.Modulus != 1_000_000_000 {
		t.Fatal("the command retained the caller's modulus pointer")
	}
	hours, _ := meter.NewValue(meter.HourMeter, 0, nil)
	for name, edit := range map[string]func(*MeterChangeInput){
		"unknown change":          func(v *MeterChangeInput) { v.Change = MeterRollover },
		"zero segments":           func(v *MeterChangeInput) { v.ExpectedSegments = 0 },
		"bad operation":           func(v *MeterChangeInput) { v.OperationID = "x" },
		"bad zone":                func(v *MeterChangeInput) { v.Declared.EntryZone = "Nowhere/Synthetic" },
		"reset with a label":      func(v *MeterChangeInput) { v.Change, v.Before, v.After = MeterReset, v.Opening, v.Opening },
		"replace with a before":   func(v *MeterChangeInput) { v.Before = v.Opening },
		"replace without a label": func(v *MeterChangeInput) { v.Label = "" },
		"zero modulus":            func(v *MeterChangeInput) { zero := int64(0); v.Modulus = &zero },
		"mixed kinds":             func(v *MeterChangeInput) { v.Opening = hours },
	} {
		in := changeInput(t, MeterReplace)
		edit(&in)
		if _, err := NewMeterChangeCommand(in); !errors.Is(err, meter.ErrInvalid) {
			t.Fatal("invalid change accepted", name, err)
		}
	}
	rollover := decisionInput(MeterConfirmRollover)
	if _, err := NewMeterDecisionCommand(rollover); err != nil {
		t.Fatal("rollover decision refused", err)
	}
	rollover.Value, _ = meter.NewValue(meter.Odometer, 1, nil)
	if _, err := NewMeterDecisionCommand(rollover); !errors.Is(err, meter.ErrInvalid) {
		t.Fatal("rollover decision with a value accepted", err)
	}
}
