package application

import (
	"context"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
)

func (s MeterService) Install(ctx context.Context, scope kernel.OrgScope, actor FleetActor, decision access.Decision, command MeterInstall) (MeterInstallResult, bool, error) {
	if !s.Enabled {
		return MeterInstallResult{}, false, ErrSurfaceDisabled
	}
	in, e := command.Snapshot()
	if e != nil {
		return MeterInstallResult{}, false, e
	}
	if e = current(ctx, scope, actor, decision, in.AssetID, true); e != nil {
		return MeterInstallResult{}, false, e
	}
	if unavailable(s.Repository) || s.NewID == nil {
		return MeterInstallResult{}, false, ErrUnavailable
	}
	ids := [2]string{}
	for i := range ids {
		ids[i], e = s.NewID()
		if e != nil || !canonicalID(ids[i]) {
			return MeterInstallResult{}, false, ErrUnavailable
		}
	}
	result, replay, e := s.Repository.InstallMeter(ctx, scope, actor, decision.Subject().MembershipID(), ids[0], ids[1], command)
	if guard := current(ctx, scope, actor, decision, in.AssetID, true); guard != nil {
		return MeterInstallResult{}, false, guard
	}
	if e != nil {
		return MeterInstallResult{}, false, e
	}
	if result.AssetID != in.AssetID || result.OpeningReadingID != in.OpeningReadingID || result.AssetRevision != in.ExpectedAssetRevision+1 || !canonicalID(result.MeterID) || !canonicalID(result.SegmentID) {
		return MeterInstallResult{}, false, ErrUnavailable
	}
	return result, replay, nil
}

func validMeterView(v MeterView, scope kernel.OrgScope, assetID, meterID string) bool {
	if v.Reference.Asset().Validate(scope) != nil || v.Reference.Asset().ID().Value() != assetID || !canonicalID(v.Reference.ID()) || meterID != "" && v.Reference.ID() != meterID || !validMeterLabel(v.Label) || v.Segment.Meter() != v.Reference || !canonicalID(v.Segment.ID()) {
		return false
	}
	if _, e := meter.NewMeterRef(scope, v.Reference.Asset(), v.Reference.ID(), v.Reference.Kind(), v.Reference.Modulus()); e != nil {
		return false
	}
	if v.InstalledAt.UTC().Year() < 1 || v.InstalledAt.UTC().Year() > 9999 || v.RemovedAt != nil && (!v.RemovedAt.After(v.InstalledAt) || v.RemovedAt.UTC().Year() > 9999) {
		return false
	}
	if v.Latest != nil {
		in := v.Latest.Snapshot()
		if _, e := meter.NewReading(scope, in); e != nil || in.Reference.Segment() != v.Segment || in.State != meter.Valid || in.Declared.OccurredAt.Before(v.InstalledAt) || v.RemovedAt != nil && !in.Declared.OccurredAt.Before(*v.RemovedAt) {
			return false
		}
	}
	return true
}

func (s MeterService) Latest(ctx context.Context, scope kernel.OrgScope, actor FleetActor, decision access.Decision, assetID, meterID string) (MeterView, error) {
	if !s.Enabled {
		return MeterView{}, ErrSurfaceDisabled
	}
	if !canonicalID(assetID) || !canonicalID(meterID) {
		return MeterView{}, meter.ErrInvalid
	}
	if e := current(ctx, scope, actor, decision, assetID, false); e != nil {
		return MeterView{}, e
	}
	if unavailable(s.Repository) {
		return MeterView{}, ErrUnavailable
	}
	v, e := s.Repository.ReadLatestMeter(ctx, scope, actor, decision.Subject().MembershipID(), assetID, meterID)
	if guard := current(ctx, scope, actor, decision, assetID, false); guard != nil {
		return MeterView{}, guard
	}
	if e != nil {
		return MeterView{}, e
	}
	if !validMeterView(v, scope, assetID, meterID) {
		return MeterView{}, ErrUnavailable
	}
	return v.Clone(), nil
}
func (s MeterService) List(ctx context.Context, scope kernel.OrgScope, actor FleetActor, decision access.Decision, assetID string, filter MeterFilter) (MeterPage, error) {
	if !s.Enabled {
		return MeterPage{}, ErrSurfaceDisabled
	}
	if !canonicalID(assetID) {
		return MeterPage{}, meter.ErrInvalid
	}
	if e := filter.Validate(); e != nil {
		return MeterPage{}, e
	}
	if e := current(ctx, scope, actor, decision, assetID, false); e != nil {
		return MeterPage{}, e
	}
	if unavailable(s.Repository) {
		return MeterPage{}, ErrUnavailable
	}
	page, e := s.Repository.ListAssetMeters(ctx, scope, actor, decision.Subject().MembershipID(), assetID, filter)
	if guard := current(ctx, scope, actor, decision, assetID, false); guard != nil {
		return MeterPage{}, guard
	}
	if e != nil {
		return MeterPage{}, e
	}
	if len(page.Meters) > filter.Limit {
		return MeterPage{}, ErrUnavailable
	}
	prior := filter.AfterID
	values := make([]MeterView, 0, len(page.Meters))
	for _, v := range page.Meters {
		if !validMeterView(v, scope, assetID, "") || v.Reference.ID() <= prior {
			return MeterPage{}, ErrUnavailable
		}
		prior = v.Reference.ID()
		values = append(values, v.Clone())
	}
	if page.NextID != "" && (len(page.Meters) != filter.Limit || page.NextID != prior || !canonicalID(page.NextID)) {
		return MeterPage{}, ErrUnavailable
	}
	return MeterPage{Meters: values, NextID: page.NextID}, nil
}
