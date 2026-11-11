package domain_test

import (
	"errors"
	"testing"
	"time"

	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

func instant(t *testing.T, raw string) time.Time {
	t.Helper()
	at, err := orgtime.ParseInstant(raw)
	if err != nil {
		t.Fatal(err)
	}
	return at
}

func TestReceiptAuthorityAndHistoricalSnapshot(t *testing.T) {
	start := instant(t, "2026-11-11T00:00:00Z")
	receipt := instant(t, "2026-11-12T00:00:00Z")
	unit := "America/Cuiaba"
	declaration := orgtime.Declaration{OccurredAt: start, EntryZone: "Asia/Tokyo", DeviceOffsetSeconds: 32400}
	zones := orgtime.ReportingZones{Organization: "America/Sao_Paulo", Unit: &unit}
	// Use a non-UTC server representation to prove storage normalization.
	originalReceipt := receipt.In(time.FixedZone("server", 3600))
	record, err := orgtime.Capture(declaration, originalReceipt, zones)
	if err != nil {
		t.Fatal(err)
	}
	declaredInside, err := orgtime.InPeriod(record.OccurredAt, start, receipt)
	if err != nil || !declaredInside {
		t.Fatal("declared time should be in period", err)
	}
	authorized, err := orgtime.InPeriod(record.AuthorityAt(), start, receipt)
	if err != nil || authorized {
		t.Fatal("device backdating must not extend authority", err)
	}
	for _, occurred := range []time.Time{start.Add(-time.Hour), receipt.Add(time.Hour)} {
		changed := declaration
		changed.OccurredAt = occurred
		other, err := orgtime.Capture(changed, receipt, zones)
		if err != nil || !other.AuthorityAt().Equal(record.AuthorityAt()) {
			t.Fatal("device changed authority", err)
		}
	}
	sample := receipt.Add(12 * time.Minute)
	skew, err := orgtime.AssessClockSkew(sample, receipt, nil)
	if err != nil || skew.Flag != "CLOCK_SKEW" {
		t.Fatal(skew, err)
	}
	unit = "Asia/Tokyo"
	if record.ReportingZone != "America/Cuiaba" || record.ReportingDay != "2026-11-10" ||
		record.ReportingSource != "unit" || !record.OccurredAt.Equal(start) ||
		record.ReceivedAt.Location() != time.UTC || originalReceipt.Location() == time.UTC {
		t.Fatal("snapshot or input was changed", record)
	}
	updated, err := orgtime.Capture(declaration, receipt, zones)
	if err != nil || updated.ReportingDay != "2026-11-11" || record.ReportingDay != "2026-11-10" {
		t.Fatal("new context must not rewrite historical reporting", err)
	}
}

func TestTypedInputsCannotBypassSupportedInstantRange(t *testing.T) {
	valid := instant(t, "2026-11-11T00:00:00Z")
	for _, bad := range []time.Time{
		time.Date(0, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC),
	} {
		d := orgtime.Declaration{OccurredAt: bad, EntryZone: "UTC"}
		zones := orgtime.ReportingZones{Organization: "UTC"}
		if _, err := orgtime.Capture(d, valid, zones); !errors.Is(err, orgtime.ErrInvalid) {
			t.Fatal(err)
		}
		d.OccurredAt = valid
		if _, err := orgtime.Capture(d, bad, zones); !errors.Is(err, orgtime.ErrInvalid) {
			t.Fatal(err)
		}
		for _, args := range [][3]time.Time{{bad, valid, valid.Add(time.Hour)}, {valid, bad, valid}, {valid, valid, bad}} {
			if _, err := orgtime.InPeriod(args[0], args[1], args[2]); !errors.Is(err, orgtime.ErrInvalid) {
				t.Fatal(err)
			}
		}
		if _, err := orgtime.AssessClockSkew(bad, valid, nil); !errors.Is(err, orgtime.ErrInvalid) {
			t.Fatal(err)
		}
		if _, err := orgtime.AssessClockSkew(valid, bad, nil); !errors.Is(err, orgtime.ErrInvalid) {
			t.Fatal(err)
		}
	}
}
