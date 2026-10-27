package application

import (
	"errors"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/domain"
	"testing"
)

func TestDriverReportWindowPeriods(t *testing.T) {
	base := ReportRequest{VehicleID: "22222222-2222-4222-8222-222222222222", Timezone: "America/Cuiaba", Basis: "ODOMETER"}
	cases := []struct{ period, anchor, from, to string }{
		{"DAY", "2026-10-26", "2026-10-26", "2026-10-27"},
		{"WEEK", "2026-10-28", "2026-10-26", "2026-11-02"},
		{"MONTH", "2026-10-26", "2026-10-01", "2026-11-01"},
	}
	for _, tc := range cases {
		r := base
		r.Period, r.Anchor = tc.period, tc.anchor
		key, _, fromDay, toDay, _, _, e := r.validate()
		if e != nil || fromDay != tc.from || toDay != tc.to || key.From != tc.from || key.To != tc.to || key.Timezone != r.Timezone {
			t.Fatal(tc, key, fromDay, toDay, e)
		}
	}
	custom := base
	custom.Period, custom.From, custom.To = "CUSTOM", "2026-10-01", "2026-10-08"
	if key, _, _, _, _, _, e := custom.validate(); e != nil || key.From != "2026-10-01" || key.To != "2026-10-08" {
		t.Fatal("custom", key, e)
	}
	for _, bad := range []ReportRequest{
		{VehicleID: "not-a-uuid", Timezone: "UTC", Period: "DAY", Basis: "ODOMETER"},
		{VehicleID: base.VehicleID, Timezone: "UTC", Period: "YEAR", Basis: "ODOMETER"},
		{VehicleID: base.VehicleID, Timezone: "UTC", Period: "DAY", Basis: "ROUTE_PREDICTION"},
		{VehicleID: base.VehicleID, Timezone: "UTC", Period: "CUSTOM", From: "2026-10-02", To: "2026-10-01", Basis: "ODOMETER"},
	} {
		if _, _, _, _, _, _, e := bad.validate(); !errors.Is(e, domain.ErrInvalid) {
			t.Fatal("bad request", bad, e)
		}
	}
}
