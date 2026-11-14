package meter_test

import (
	"errors"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
)

func TestEntryFormIsValidatedByDomain(t *testing.T) {
	for _, tc := range []struct {
		name          string
		kind          meter.Kind
		form          meter.EntryForm
		text, minutes string
		count         int64
		invalid       bool
	}{
		{"base", meter.Odometer, meter.BaseUnits, "1", "", 1, false},
		{"kilometres", meter.Odometer, meter.Kilometres, "184532.4", "", 184532400, false},
		{"decimal-hours", meter.HourMeter, meter.DecimalHours, "4212.5", "", 15165000, false},
		{"hours-minutes", meter.HourMeter, meter.HoursMinutes, "3", "30", 12600, false},
		{"km-kind", meter.HourMeter, meter.Kilometres, "1", "", 0, true},
		{"hours-kind", meter.Odometer, meter.DecimalHours, "1", "", 0, true},
		{"hm-kind", meter.Odometer, meter.HoursMinutes, "1", "0", 0, true},
		{"unknown-form", meter.Odometer, "UNKNOWN", "1", "", 0, true},
		{"unknown-kind", "UNKNOWN", meter.BaseUnits, "1", "", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v, err := meter.ParseEntry(tc.kind, tc.form, tc.text, tc.minutes, nil)
			if tc.invalid {
				if !errors.Is(err, meter.ErrInvalid) {
					t.Fatal("domain accepted incompatible entry", err)
				}
				return
			}
			if err != nil || v.Kind() != tc.kind || v.Count() != tc.count {
				t.Fatal("entry conversion incorrect", err)
			}
		})
	}
}
