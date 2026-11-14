package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

var allocationFrom = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

type allocationRepository struct {
	calls   int
	member  string
	asset   string
	history app.AllocationHistory
	filter  app.AllocationFilter
	page    app.AllocationPage
	err     error
}

func (r *allocationRepository) ListAssetAllocations(_ context.Context, _ kernel.OrgScope, _ app.FleetActor, member, asset string, history app.AllocationHistory, filter app.AllocationFilter) (app.AllocationPage, error) {
	r.calls++
	r.member, r.asset, r.history, r.filter = member, asset, history, filter
	return r.page, r.err
}

func allocationEntry(t *testing.T, target int, from time.Time, to *time.Time) app.AllocationEntry {
	t.Helper()
	p, err := fleet.NewAllocationPeriod(from, to)
	if err != nil {
		t.Fatal(err)
	}
	return app.AllocationEntry{TargetID: id(target), Period: p, RecordedAt: from.Add(time.Second)}
}

func instant(v time.Time) *time.Time { return &v }

func TestAllocationFilterBoundsTheKeysetPage(t *testing.T) {
	if (app.AllocationFilter{Limit: 1}).Validate() != nil || (app.AllocationFilter{Limit: 100, After: instant(allocationFrom)}).Validate() != nil {
		t.Fatal("valid allocation filter refused")
	}
	for name, bad := range map[string]app.AllocationFilter{
		"zero limit":       {Limit: 0},
		"oversized":        {Limit: 101},
		"unaligned keyset": {Limit: 2, After: instant(allocationFrom.Add(time.Nanosecond))},
		"unset keyset":     {Limit: 2, After: instant(time.Time{})},
		"year 10000":       {Limit: 2, After: instant(time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC))},
	} {
		if err := bad.Validate(); !errors.Is(err, fleet.ErrInvalid) {
			t.Fatal("invalid allocation filter accepted", name)
		}
	}
}

// BUC-EMP-114: current fleet.asset.read, exact delegation and a validated,
// cloned start-ordered page of either history.
func TestAllocationHistoryReadRequiresCurrentReadAndValidatesEveryPeriod(t *testing.T) {
	ctx, finish, s, actor, read := decision(t, id(20), false)
	defer finish()
	change, later := allocationFrom.Add(31*24*time.Hour), allocationFrom.Add(61*24*time.Hour)
	units := []app.AllocationEntry{allocationEntry(t, 30, allocationFrom, instant(change)), allocationEntry(t, 31, change, instant(later)), allocationEntry(t, 30, later, nil)}
	repo := &allocationRepository{page: app.AllocationPage{Entries: units[:2], Next: instant(change)}}
	service := app.AllocationHistoryService{Enabled: true, Repository: repo}
	page, err := service.History(ctx, s, actor, read, id(20), app.UnitAllocationHistory, app.AllocationFilter{Limit: 2})
	if err != nil || len(page.Entries) != 2 || page.Next == nil || !page.Next.Equal(change) || repo.member != id(4) || repo.asset != id(20) || repo.history != app.UnitAllocationHistory {
		t.Fatal("valid unit allocation history refused or not delegated exactly", err)
	}
	*page.Entries[0].Period.To = time.Time{}
	*page.Next = time.Time{}
	if !units[0].Period.To.Equal(change) || !repo.page.Next.Equal(change) {
		t.Fatal("allocation history escaped the repository copy")
	}
	repo.page = app.AllocationPage{Entries: units[2:]}
	if page, err := service.History(ctx, s, actor, read, id(20), app.UnitAllocationHistory, app.AllocationFilter{Limit: 2, After: instant(change)}); err != nil || len(page.Entries) != 1 || !page.Entries[0].Period.Open() || page.Next != nil {
		t.Fatal("last unit page refused", err)
	}
	gap := []app.AllocationEntry{allocationEntry(t, 40, allocationFrom, instant(change)), allocationEntry(t, 41, later, nil)}
	repo.page = app.AllocationPage{Entries: gap}
	if _, err := service.History(ctx, s, actor, read, id(20), app.CostCentreAllocationHistory, app.AllocationFilter{Limit: 2}); err != nil || repo.history != app.CostCentreAllocationHistory {
		t.Fatal("optional cost-centre gap refused", err)
	}
	if _, err := service.History(ctx, s, actor, read, id(20), app.UnitAllocationHistory, app.AllocationFilter{Limit: 2}); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("gap in the unit history accepted", err)
	}
	for name, bad := range map[string]app.AllocationPage{
		"descending":        {Entries: []app.AllocationEntry{units[1], units[0]}},
		"overlap":           {Entries: []app.AllocationEntry{allocationEntry(t, 30, allocationFrom, instant(later)), units[1]}},
		"open not last":     {Entries: []app.AllocationEntry{allocationEntry(t, 30, allocationFrom, nil), units[1]}},
		"next after open":   {Entries: units[2:], Next: instant(later)},
		"next not full":     {Entries: units[:1], Next: instant(allocationFrom)},
		"next not last":     {Entries: units[:2], Next: instant(allocationFrom)},
		"oversized":         {Entries: units},
		"bad target":        {Entries: []app.AllocationEntry{{TargetID: "unit", Period: units[0].Period, RecordedAt: allocationFrom}}},
		"unset recording":   {Entries: []app.AllocationEntry{{TargetID: id(30), Period: units[0].Period}}},
		"unvalidated bound": {Entries: []app.AllocationEntry{{TargetID: id(30), Period: fleet.AllocationPeriod{From: allocationFrom.Add(time.Nanosecond)}, RecordedAt: allocationFrom}}},
	} {
		repo.page = bad
		if _, err := service.History(ctx, s, actor, read, id(20), app.UnitAllocationHistory, app.AllocationFilter{Limit: 2}); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("invalid allocation page accepted", name, err)
		}
	}
	repo.page = app.AllocationPage{Entries: units[:1]}
	if _, err := service.History(ctx, s, actor, read, id(20), app.UnitAllocationHistory, app.AllocationFilter{Limit: 2, After: instant(allocationFrom)}); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("period at the keyset accepted")
	}
	before := repo.calls
	for name, call := range map[string]func() error{
		"bad asset": func() error {
			_, e := service.History(ctx, s, actor, read, "bad", app.UnitAllocationHistory, app.AllocationFilter{Limit: 2})
			return e
		},
		"bad history": func() error {
			_, e := service.History(ctx, s, actor, read, id(20), "TEAM", app.AllocationFilter{Limit: 2})
			return e
		},
		"bad filter": func() error {
			_, e := service.History(ctx, s, actor, read, id(20), app.UnitAllocationHistory, app.AllocationFilter{})
			return e
		},
		"other asset": func() error {
			_, e := service.History(ctx, s, actor, read, id(21), app.UnitAllocationHistory, app.AllocationFilter{Limit: 2})
			return e
		},
		"disabled": func() error {
			_, e := (app.AllocationHistoryService{Repository: repo}).History(ctx, s, actor, read, id(20), app.UnitAllocationHistory, app.AllocationFilter{Limit: 2})
			return e
		},
		"no repository": func() error {
			_, e := (app.AllocationHistoryService{Enabled: true}).History(ctx, s, actor, read, id(20), app.UnitAllocationHistory, app.AllocationFilter{Limit: 2})
			return e
		},
	} {
		if err := call(); err == nil {
			t.Fatal("allocation history call accepted", name)
		}
	}
	if repo.calls != before {
		t.Fatal("refused allocation history reached the repository")
	}
	repo.err = app.ErrNotFound
	if _, err := service.History(ctx, s, actor, read, id(20), app.UnitAllocationHistory, app.AllocationFilter{Limit: 2}); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("repository refusal changed", err)
	}
	ctxWrite, finishWrite, sw, actorWrite, write := decision(t, id(20), true)
	defer finishWrite()
	repo.err = nil
	repo.page = app.AllocationPage{}
	if _, err := service.History(ctxWrite, sw, actorWrite, write, id(20), app.UnitAllocationHistory, app.AllocationFilter{Limit: 2}); !errors.Is(err, app.ErrUnauthenticated) {
		t.Fatal("manage decision reused for the read policy", err)
	}
}

// B-BR-EMP-226: the snapshot keeps the exact asset, the floored instant, a
// canonical unit and an optional cost centre that is never invented.
func TestAllocationSnapshotIsExactAndNeverInventsACostCentre(t *testing.T) {
	at := allocationFrom.Add(1500 * time.Nanosecond)
	v, err := app.NewAllocationSnapshot(id(20), at, id(30), "")
	if err != nil || v.AssetID != id(20) || !v.At.Equal(allocationFrom.Add(time.Microsecond)) || v.UnitID != id(30) || v.CostCentreID != "" {
		t.Fatal("snapshot without cost centre refused or changed", err)
	}
	if v, err := app.NewAllocationSnapshot(id(20), at, id(30), id(40)); err != nil || v.CostCentreID != id(40) {
		t.Fatal("snapshot with cost centre refused", err)
	}
	for name, tc := range map[string]struct {
		asset, unit, centre string
		at                  time.Time
	}{
		"bad asset":       {"asset", id(30), "", at},
		"missing unit":    {id(20), "", "", at},
		"bad unit":        {id(20), "Norte", "", at},
		"bad cost centre": {id(20), id(30), "CC-1", at},
		"upper case":      {id(20), "AD000000-0000-4000-8000-000000000030", "", at},
		"unset instant":   {id(20), id(30), "", time.Time{}},
		"year 10000":      {id(20), id(30), "", time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)},
	} {
		if _, err := app.NewAllocationSnapshot(tc.asset, tc.at, tc.unit, tc.centre); !errors.Is(err, fleet.ErrInvalid) {
			t.Fatal("invalid allocation snapshot accepted", name)
		}
	}
	if text := v.String(); text != "EnterpriseFleetCommand(redacted)" {
		t.Fatal("allocation snapshot is not redacted", text)
	}
}
