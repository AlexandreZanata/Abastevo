package domain_test

import (
	"errors"
	"testing"
	"time"

	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

func TestReportingAttributionNeedsNoReceiptAndMatchesCapture(t *testing.T) {
	cuiaba, apia := "America/Cuiaba", "Pacific/Apia"
	cases := []struct {
		at                string
		zones             orgtime.ReportingZones
		day, zone, source string
	}{
		{"2026-11-01T02:30:00Z", orgtime.ReportingZones{Organization: "America/Sao_Paulo"}, "2026-10-31", "America/Sao_Paulo", "organization"},
		{"2026-11-01T03:30:00Z", orgtime.ReportingZones{Organization: "America/Sao_Paulo"}, "2026-11-01", "America/Sao_Paulo", "organization"},
		{"2026-11-01T03:30:00Z", orgtime.ReportingZones{Organization: "America/Sao_Paulo", Unit: &cuiaba}, "2026-10-31", "America/Cuiaba", "unit"},
		{"2011-12-30T10:00:00Z", orgtime.ReportingZones{Organization: "UTC", Unit: &apia}, "2011-12-31", "Pacific/Apia", "unit"},
		{"0001-01-01T00:00:00Z", orgtime.ReportingZones{Organization: "UTC"}, "0001-01-01", "UTC", "organization"},
		{"9999-12-31T23:59:59.999999999Z", orgtime.ReportingZones{Organization: "UTC"}, "9999-12-31", "UTC", "organization"},
	}
	for _, c := range cases {
		at := instant(t, c.at).In(time.FixedZone("device", 9*3600))
		got, err := orgtime.AttributeReportingDay(at, c.zones)
		if err != nil || got != (orgtime.ReportingAttribution{Day: c.day, Zone: c.zone, Source: c.source}) {
			t.Fatal("reporting attribution changed", c.at, got, err)
		}
		record, err := orgtime.Capture(orgtime.Declaration{OccurredAt: at, EntryZone: "Asia/Tokyo", DeviceOffsetSeconds: 32400},
			instant(t, "2026-11-14T00:00:00Z"), c.zones)
		if err != nil || record.ReportingDay != got.Day || record.ReportingZone != got.Zone || record.ReportingSource != got.Source {
			t.Fatal("Capture and receipt-free attribution disagree", c.at, err)
		}
	}
}

func TestReportingAttributionRefusesWithoutFallback(t *testing.T) {
	valid, empty, mars := "America/Cuiaba", "", "Mars/Olympus"
	at := instant(t, "2026-11-14T12:00:00Z")
	for name, c := range map[string]struct {
		at    time.Time
		zones orgtime.ReportingZones
	}{
		"invalid-unit":        {at, orgtime.ReportingZones{Organization: "America/Sao_Paulo", Unit: &mars}},
		"empty-unit":          {at, orgtime.ReportingZones{Organization: "America/Sao_Paulo", Unit: &empty}},
		"invalid-org-unit":    {at, orgtime.ReportingZones{Organization: "Mars/Olympus", Unit: &valid}},
		"empty-org":           {at, orgtime.ReportingZones{}},
		"entry-not-org":       {at, orgtime.ReportingZones{Organization: "Asia/Tokyo "}},
		"local-year-zero":     {instant(t, "0001-01-01T00:00:00Z"), orgtime.ReportingZones{Organization: "America/Sao_Paulo"}},
		"year-zero-instant":   {time.Date(0, 12, 31, 0, 0, 0, 0, time.UTC), orgtime.ReportingZones{Organization: "UTC"}},
		"year-10000-instant":  {time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC), orgtime.ReportingZones{Organization: "UTC"}},
		"local-year-10000-up": {instant(t, "9999-12-31T23:00:00Z"), orgtime.ReportingZones{Organization: "Asia/Tokyo"}},
	} {
		if _, err := orgtime.AttributeReportingDay(c.at, c.zones); !errors.Is(err, orgtime.ErrInvalid) {
			t.Fatal("invalid attribution accepted", name, err)
		}
	}
}
