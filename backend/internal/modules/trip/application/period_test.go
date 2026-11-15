package application_test

import (
	"errors"
	"testing"
	"time"

	reviewapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/application"
	period "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain/period"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
)

// periodFact is a gate answer for the stored start day 2026-10-31.
func periodFact(state period.State, revision int64) reviewapp.PeriodFact {
	checked := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	f := reviewapp.PeriodFact{Day: "2026-10-31", Month: "2026-10", State: state, Revision: revision, CheckedAt: checked}
	if state != period.Open {
		f.PeriodID = id(90)
	}
	if state == period.Reopened {
		f.Reopening = &reviewapp.ReopeningFact{Revision: revision, Authority: period.ReopenPermission, Reason: "Correção de outubro", ActorAccountID: id(91), ActorMembershipID: id(92), At: checked.Add(-time.Hour)}
	}
	return f
}

// B-BR-EMP-359/361, EDR-232: a decision is admitted only in a writable
// period of exactly the trip's stored start reporting day.
func TestDecisionPeriodAdmitsOnlyAWritablePeriodOfTheStoredDay(t *testing.T) {
	for name, tc := range map[string]struct {
		day  string
		fact reviewapp.PeriodFact
		want error
	}{
		"open":                   {"2026-10-31", periodFact(period.Open, 1), nil},
		"reopened with proof":    {"2026-10-31", periodFact(period.Reopened, 3), nil},
		"locked":                 {"2026-10-31", periodFact(period.Locked, 2), app.ErrPeriodLocked},
		"relocked":               {"2026-10-31", periodFact(period.Locked, 4), app.ErrPeriodLocked},
		"another day":            {"2026-10-30", periodFact(period.Open, 1), app.ErrUnavailable},
		"another month":          {"2026-11-01", periodFact(period.Open, 1), app.ErrUnavailable},
		"no stored day":          {"", periodFact(period.Open, 1), app.ErrUnavailable},
		"zero fact":              {"2026-10-31", reviewapp.PeriodFact{}, app.ErrUnavailable},
		"reopened without proof": {"2026-10-31", func() reviewapp.PeriodFact { f := periodFact(period.Reopened, 3); f.Reopening = nil; return f }(), app.ErrUnavailable},
		"reopening of an older revision": {"2026-10-31", func() reviewapp.PeriodFact {
			f := periodFact(period.Reopened, 5)
			f.Reopening.Revision = 3
			return f
		}(), app.ErrUnavailable},
		"reopening after the check": {"2026-10-31", func() reviewapp.PeriodFact {
			f := periodFact(period.Reopened, 3)
			f.Reopening.At = f.CheckedAt.Add(time.Second)
			return f
		}(), app.ErrUnavailable},
		"locked with parity broken": {"2026-10-31", periodFact(period.Locked, 3), app.ErrUnavailable},
	} {
		if err := app.DecisionPeriod(tc.day, tc.fact); !errors.Is(err, tc.want) || (tc.want == nil) != (err == nil) {
			t.Fatalf("%s: got %v, want %v", name, err, tc.want)
		}
	}
	if app.ErrPeriodLocked == app.ErrUnavailable || errors.Is(app.ErrPeriodLocked, app.ErrUnavailable) {
		t.Fatal("a locked period must stay distinguishable from an unavailable source")
	}
}
