package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

type meterRepository struct {
	calls                 int
	after                 func()
	result                app.MeterInstallResult
	view                  app.MeterView
	page                  app.MeterPage
	err                   error
	replay                bool
	member, first, second string
}

func (r *meterRepository) InstallMeter(_ context.Context, _ kernel.OrgScope, _ app.FleetActor, member, a, b string, _ app.MeterInstall) (app.MeterInstallResult, bool, error) {
	r.calls++
	r.member, r.first, r.second = member, a, b
	if r.after != nil {
		r.after()
	}
	return r.result, r.replay, r.err
}
func (r *meterRepository) ReadLatestMeter(context.Context, kernel.OrgScope, app.FleetActor, string, string, string) (app.MeterView, error) {
	r.calls++
	if r.after != nil {
		r.after()
	}
	return r.view, r.err
}
func (r *meterRepository) ListAssetMeters(context.Context, kernel.OrgScope, app.FleetActor, string, string, app.MeterFilter) (app.MeterPage, error) {
	r.calls++
	if r.after != nil {
		r.after()
	}
	return r.page, r.err
}
func installCommand(t *testing.T) app.MeterInstall {
	t.Helper()
	value, _ := meter.NewValue(meter.Odometer, 100, nil)
	v, e := app.NewMeterInstall(app.MeterInstallInput{OperationID: id(30), AssetID: id(20), OpeningReadingID: id(31), ExpectedAssetRevision: 1, Kind: meter.Odometer, Label: "Observed", Value: value, Declared: orgtime.Declaration{OccurredAt: time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC), EntryZone: "UTC"}, Method: meter.Typed})
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func meterResult() app.MeterInstallResult {
	return app.MeterInstallResult{AssetID: id(20), MeterID: id(21), SegmentID: id(22), OpeningReadingID: id(31), AssetRevision: 2}
}
func meterView(t *testing.T, s kernel.OrgScope, n int) app.MeterView {
	t.Helper()
	assetID, _ := fleet.ParseAssetID(id(20))
	a, _ := fleet.NewAssetRef(s, assetID)
	m, e := meter.NewMeterRef(s, a, id(n), meter.Odometer, nil)
	if e != nil {
		t.Fatal(e)
	}
	seg, _ := meter.NewSegmentRef(s, m, id(n+100))
	return app.MeterView{Reference: m, Label: "Observed", InstalledAt: time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC), Segment: seg}
}
func TestMeterServicePublishesOnlyOriginalValidMetadata(t *testing.T) {
	ctx, finish, s, actor, d := decision(t, id(20), true)
	defer finish()
	r := &meterRepository{result: meterResult(), replay: true}
	n := 50
	service := app.MeterService{Enabled: true, Repository: r, NewID: func() (string, error) { n++; return id(n), nil }}
	v, replay, e := service.Install(ctx, s, actor, d, installCommand(t))
	if e != nil || !replay || v != meterResult() || r.member != id(4) || r.first != id(51) || r.second != id(52) {
		t.Fatal("original metadata/current membership lost", e)
	}
	for _, edit := range []func(*app.MeterInstallResult){func(v *app.MeterInstallResult) { v.AssetID = id(99) }, func(v *app.MeterInstallResult) { v.OpeningReadingID = id(99) }, func(v *app.MeterInstallResult) { v.AssetRevision = 1 }, func(v *app.MeterInstallResult) { v.MeterID = "" }, func(v *app.MeterInstallResult) { v.SegmentID = "" }} {
		r.result = meterResult()
		edit(&r.result)
		if _, _, e := service.Install(ctx, s, actor, d, installCommand(t)); !errors.Is(e, app.ErrUnavailable) {
			t.Fatal("invalid provisional metadata published", e)
		}
	}
}
func TestMeterServiceRefusesBeforeRepositoryAndAfterProofEnds(t *testing.T) {
	for _, tc := range []struct {
		name    string
		write   bool
		channel string
		edit    func(*app.FleetActor)
		closed  bool
		missing bool
	}{
		{"wrong-policy", false, "web", nil, false, false}, {"android-channel", true, "android", nil, false, false}, {"wrong-account", true, "web", func(v *app.FleetActor) { v.AccountID = id(99) }, false, false}, {"closed", true, "web", nil, true, false}, {"nil-repository", true, "web", nil, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, finish, s, actor, d := decision(t, id(20), tc.write, tc.channel)
			defer finish()
			r := &meterRepository{result: meterResult()}
			service := app.MeterService{Enabled: !tc.closed, Repository: r, NewID: func() (string, error) { return id(90), nil }}
			if tc.edit != nil {
				tc.edit(&actor)
			}
			if tc.missing {
				service.Repository = nil
			}
			if _, _, e := service.Install(ctx, s, actor, d, installCommand(t)); e == nil || r.calls != 0 {
				t.Fatal("source refused after reaching repository", e)
			}
		})
	}
	ctx, finish, s, actor, d := decision(t, id(20), true)
	defer finish()
	r := &meterRepository{result: meterResult(), after: finish}
	service := app.MeterService{Enabled: true, Repository: r, NewID: func() (string, error) { return id(90), nil }}
	if _, _, e := service.Install(ctx, s, actor, d, installCommand(t)); !errors.Is(e, app.ErrUnauthenticated) {
		t.Fatal("ended proof result escaped", e)
	}
}
func TestMeterServiceReadBoundariesAndDefensivePages(t *testing.T) {
	ctx, finish, s, actor, d := decision(t, id(20), false)
	defer finish()
	r := &meterRepository{view: meterView(t, s, 21)}
	r.page = app.MeterPage{Meters: []app.MeterView{r.view}, NextID: id(21)}
	service := app.MeterService{Enabled: true, Repository: r}
	if _, e := service.Latest(ctx, s, actor, d, id(20), id(21)); e != nil {
		t.Fatal("actual meter view", e)
	}
	page, e := service.List(ctx, s, actor, d, id(20), app.MeterFilter{Limit: 1})
	if e != nil || len(page.Meters) != 1 {
		t.Fatal("valid bounded page", e)
	}
	page.Meters[0].Label = "changed"
	if r.page.Meters[0].Label != "Observed" {
		t.Fatal("returned slice aliases repository")
	}
	r.page.NextID = id(22)
	if _, e := service.List(ctx, s, actor, d, id(20), app.MeterFilter{Limit: 1}); !errors.Is(e, app.ErrUnavailable) {
		t.Fatal("inconsistent page cursor")
	}
	r.view = app.MeterView{}
	if _, e := service.Latest(ctx, s, actor, d, id(20), id(21)); !errors.Is(e, app.ErrUnavailable) {
		t.Fatal("unverified private view published")
	}
	before := r.calls
	if _, e := service.Latest(ctx, s, actor, d, id(99), id(21)); !errors.Is(e, app.ErrUnauthenticated) || before != r.calls {
		t.Fatal("rebound asset reached repository")
	}
}
