package meter_test

import (
	"errors"
	"testing"
	"time"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

func TestExactConversions(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		kind      meter.Kind
		count     int64
	}{
		{"km", "184532.4", meter.Odometer, 184532400}, {"km-zero", "0", meter.Odometer, 0}, {"km-max", "1000000000", meter.Odometer, 1000000000000}, {"km-three", "0.001", meter.Odometer, 1},
		{"hours", "4212.5", meter.HourMeter, 15165000}, {"hours-zero", "0.00", meter.HourMeter, 0}, {"hours-max", "1000000", meter.HourMeter, 3600000000}, {"hours-two", "0.01", meter.HourMeter, 36},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var value meter.Value
			var err error
			if tc.kind == meter.Odometer {
				value, err = meter.ParseKilometres(tc.raw, nil)
			} else {
				value, err = meter.ParseHours(tc.raw, nil)
			}
			if err != nil || value.Kind() != tc.kind || value.Count() != tc.count {
				t.Fatalf("exact conversion: got %d/%s, error %v", value.Count(), value.Kind(), err)
			}
		})
	}
	v, err := meter.ParseHoursMinutes("3", "30", nil)
	if err != nil || v.Count() != 12600 {
		t.Fatal("hours/minutes must be exact", err)
	}
}
func TestConversionRefusals(t *testing.T) {
	for _, raw := range []string{"", "-1", "+1", "01", "1.", ".1", "1e3", "1,2", " 1", "1 ", "١", "1\x00", "0.0001", "1000000000.001", "922337203685477580799999"} {
		if _, err := meter.ParseKilometres(raw, nil); !errors.Is(err, meter.ErrInvalid) {
			t.Errorf("invalid km input accepted: %q", raw)
		}
	}
	for _, raw := range []string{"1.001", "0.0001", "1000000.01"} {
		if _, err := meter.ParseHours(raw, nil); !errors.Is(err, meter.ErrInvalid) {
			t.Errorf("invalid hours: %q", raw)
		}
	}
	for _, minutes := range []string{"60", "01", "1.0", "-1", " 1"} {
		if _, err := meter.ParseHoursMinutes("1", minutes, nil); !errors.Is(err, meter.ErrInvalid) {
			t.Errorf("invalid minutes: %q", minutes)
		}
	}
	for _, n := range []int64{0, -1} {
		if _, err := meter.NewValue(meter.Odometer, 0, &n); !errors.Is(err, meter.ErrInvalid) {
			t.Fatal("invalid modulus")
		}
	}
	m := int64(10)
	if _, err := meter.NewValue(meter.Odometer, 10, &m); !errors.Is(err, meter.ErrInvalid) {
		t.Fatal("modulus must strictly exceed value")
	}
}
func uuid(n int) string {
	const digits = "0123456789abcdef"
	return "7b0e2a54-0000-4000-8000-0000000000" + string([]byte{digits[n/16], digits[n%16]})
}
func scope(t *testing.T, n int) kernel.OrgScope {
	t.Helper()
	id, err := kernel.ParseOrgID(uuid(n))
	if err != nil {
		t.Fatal(err)
	}
	s, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func instant(t *testing.T, raw string) time.Time {
	t.Helper()
	v, err := orgtime.ParseInstant(raw)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func segment(t *testing.T, s kernel.OrgScope, kind meter.Kind) meter.SegmentRef {
	t.Helper()
	id, err := fleet.ParseAssetID(uuid(10))
	if err != nil {
		t.Fatal(err)
	}
	asset, err := fleet.NewAssetRef(s, id)
	if err != nil {
		t.Fatal(err)
	}
	m, err := meter.NewMeterRef(s, asset, uuid(11), kind, nil)
	if err != nil {
		t.Fatal(err)
	}
	segment, err := meter.NewSegmentRef(s, m, uuid(12))
	if err != nil {
		t.Fatal(err)
	}
	return segment
}
func reading(t *testing.T, s kernel.OrgScope, seg meter.SegmentRef, id, op int, value int64, at string, state meter.State) meter.Reading {
	t.Helper()
	ref, err := meter.NewReadingRef(s, seg, uuid(id))
	if err != nil {
		t.Fatal(err)
	}
	v, err := meter.NewValue(seg.Meter().Kind(), value, nil)
	if err != nil {
		t.Fatal(err)
	}
	author, err := meter.NewMembershipRef(s, uuid(13))
	if err != nil {
		t.Fatal(err)
	}
	device, err := meter.NewDeviceRef(s, uuid(14))
	if err != nil {
		t.Fatal(err)
	}
	operation, err := meter.NewOperationRef(s, uuid(op))
	if err != nil {
		t.Fatal(err)
	}
	out, err := meter.NewReading(s, meter.ReadingInput{Reference: ref, Value: v, Declared: &orgtime.Declaration{OccurredAt: instant(t, at), EntryZone: "America/Cuiaba", DeviceOffsetSeconds: -14400}, ReceivedAt: instant(t, "2026-11-14T12:00:00Z"), Author: author, Device: device, Operation: operation, Origin: meter.Standalone, Method: meter.Typed, State: state})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func fact(t *testing.T, s kernel.OrgScope, id, op int) meter.FactInput {
	t.Helper()
	operation, err := meter.NewOperationRef(s, uuid(op))
	if err != nil {
		t.Fatal(err)
	}
	author, err := meter.NewMembershipRef(s, uuid(13))
	if err != nil {
		t.Fatal(err)
	}
	device, err := meter.NewDeviceRef(s, uuid(14))
	if err != nil {
		t.Fatal(err)
	}
	return meter.FactInput{ID: uuid(id), Operation: operation, Author: author, Device: device, ReceivedAt: instant(t, "2026-11-14T00:00:00Z"), Reason: "dígitos trocados"}
}

func TestImmutableSupersessionAndUsage(t *testing.T) {
	s := scope(t, 1)
	seg := segment(t, s, meter.Odometer)
	opening := reading(t, s, seg, 20, 30, 183932000, "2026-11-05T11:00:00Z", meter.Valid)
	bad := reading(t, s, seg, 21, 31, 148190000, "2026-11-06T12:00:00Z", meter.Suspect)
	corrected := reading(t, s, seg, 22, 32, 184190000, "2026-11-06T12:00:00Z", meter.Valid)
	series, err := meter.NewSeries(s, seg, opening)
	if err != nil {
		t.Fatal(err)
	}
	series, err = series.Append(s, bad)
	if err != nil {
		t.Fatal(err)
	}
	before := series
	series, err = series.Supersede(s, bad.Snapshot().Reference, corrected, fact(t, s, 40, 32))
	if err != nil {
		t.Fatal(err)
	}
	if st, _ := before.State(s, bad.Snapshot().Reference); st != meter.Suspect {
		t.Fatal("original series mutated")
	}
	if st, _ := series.State(s, bad.Snapshot().Reference); st != meter.Voided {
		t.Fatal("original evidence must derive VOIDED")
	}
	if _, err = series.Supersede(s, bad.Snapshot().Reference, reading(t, s, seg, 23, 33, 184200000, "2026-11-06T12:00:00Z", meter.Valid), fact(t, s, 41, 33)); !errors.Is(err, meter.ErrAlreadySuperseded) {
		t.Fatal("second supersession", err)
	}
	usage, err := series.Usage(s, opening.Snapshot().Declared.OccurredAt, corrected.Snapshot().Declared.OccurredAt)
	if err != nil {
		t.Fatal(err)
	}
	delta, ok := usage.Delta()
	if !ok || delta != 258000 || usage.From().Snapshot().Reference.ID() != uuid(20) || usage.To().Snapshot().Reference.ID() != uuid(22) {
		t.Fatal("usage must retain actual anchors")
	}
	missing, err := series.Usage(s, instant(t, "2026-11-01T00:00:00Z"), corrected.Snapshot().Declared.OccurredAt)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok = missing.Delta(); ok || missing.Reason() != "NO_READING_BEFORE" || missing.From() != nil {
		t.Fatal("missing anchor fabricated")
	}
}
