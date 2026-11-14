package domain_test

import (
	"errors"
	"testing"
	"time"

	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

func TestOfflineDeclarationHasNoReceiptOrReportingContext(t *testing.T) {
	for _, raw := range []string{"0001-01-01T00:00:00Z", "2026-11-14T01:02:03.123456789Z", "9999-12-31T23:59:59.999999999Z"} {
		input := orgtime.Declaration{OccurredAt: instant(t, raw).In(time.FixedZone("device", 3600)), EntryZone: "America/Cuiaba", DeviceOffsetSeconds: 64800}
		validated, err := orgtime.ValidateDeclaration(input)
		if err != nil || validated.OccurredAt.Location() != time.UTC || !validated.OccurredAt.Equal(input.OccurredAt) || validated.EntryZone != input.EntryZone || validated.DeviceOffsetSeconds != 64800 {
			t.Fatal("declaration normalized or rejected valid metadata", err)
		}
		if input.OccurredAt.Location() == time.UTC {
			t.Fatal("input mutated")
		}
	}
}

func TestOfflineDeclarationRetainsExistingValidationProfile(t *testing.T) {
	valid := orgtime.Declaration{OccurredAt: instant(t, "2026-11-14T00:00:00Z"), EntryZone: "UTC"}
	for _, alter := range []func(*orgtime.Declaration){
		func(d *orgtime.Declaration) { d.EntryZone = "Invalid/Zone" },
		func(d *orgtime.Declaration) { d.EntryZone = "" },
		func(d *orgtime.Declaration) { d.DeviceOffsetSeconds = 64801 },
		func(d *orgtime.Declaration) { d.DeviceOffsetSeconds = -64801 },
		func(d *orgtime.Declaration) { d.OccurredAt = time.Date(0, 1, 1, 0, 0, 0, 0, time.UTC) },
		func(d *orgtime.Declaration) { d.OccurredAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC) },
	} {
		in := valid
		alter(&in)
		if _, err := orgtime.ValidateDeclaration(in); !errors.Is(err, orgtime.ErrInvalid) {
			t.Fatal("invalid declaration accepted", err)
		}
	}
	for _, offset := range []int64{-64800, 0, 64800} {
		in := valid
		in.DeviceOffsetSeconds = offset
		if _, err := orgtime.ValidateDeclaration(in); err != nil {
			t.Fatal("existing offset endpoint refused", err)
		}
	}
}

func TestOfflineValidationDoesNotRelaxCaptureReceiptOrReporting(t *testing.T) {
	in := orgtime.Declaration{OccurredAt: instant(t, "0001-01-01T00:00:00Z"), EntryZone: "America/Cuiaba", DeviceOffsetSeconds: -14400}
	if _, err := orgtime.ValidateDeclaration(in); err != nil {
		t.Fatal("device metadata must not manufacture year-zero reporting", err)
	}
	if _, err := orgtime.Capture(in, time.Time{}, orgtime.ReportingZones{Organization: "UTC"}); !errors.Is(err, orgtime.ErrInvalid) {
		t.Fatal("unset receipt accepted", err)
	}
	if _, err := orgtime.Capture(in, instant(t, "2026-11-14T00:00:00Z"), orgtime.ReportingZones{Organization: "America/Cuiaba"}); !errors.Is(err, orgtime.ErrInvalid) {
		t.Fatal("out-of-range reporting day accepted", err)
	}
	record, err := orgtime.Capture(in, instant(t, "2026-11-14T00:00:00Z"), orgtime.ReportingZones{Organization: "UTC"})
	if err != nil || record.ReportingDay != "0001-01-01" || record.ReportingZone != "UTC" || record.EntryZone != "America/Cuiaba" {
		t.Fatal("Capture behavior changed", err)
	}
}
