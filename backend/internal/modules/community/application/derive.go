package application

import (
	"context"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
)

// SignalStore is the owned persistence port for derived signals: one
// observation fact plus the rebuildable bands projection.
type SignalStore interface {
	Observation(ctx context.Context, id string) (domain.Observation, error)
	UpsertSignals(ctx context.Context, observationID string, b Bands, computedAt time.Time) error
}

// SignalPorts declares every derivation read with stdlib-shaped
// signatures. Station quality arrives pre-mapped to StationSite (failing
// closed to unknown); photo reports presence plus independent-duplicate
// count; regional reports the benchmark mean with ok=false when absent
// (ANP lag never vetoes alone). Unavailable signals fail the run — they
// never count as verified.
type SignalPorts struct {
	Clock    func() time.Time
	Store    SignalStore
	Station  func(ctx context.Context, stationID string) (StationSite, error)
	Photo    func(ctx context.Context, evidenceID string) (present bool, duplicates int, err error)
	Regional func(ctx context.Context, stationID, product, unit string) (meanMilli int64, ok bool, err error)
}

// DeriveSignals loads one observation, reduces every available input to
// bands and persists the projection, converging recomputation. Claimant
// position intake does not exist yet, so proximity derives UNKNOWN
// honestly until it lands; the pure matrix (including NEAR/FAR) is
// proven in the policy suite and activates with the intake.
func DeriveSignals(ctx context.Context, p SignalPorts, observationID string) (Bands, error) {
	now := time.Now()
	if p.Clock != nil {
		now = p.Clock()
	}
	obs, err := p.Store.Observation(ctx, observationID)
	if err != nil {
		return Bands{}, err
	}
	site, err := p.Station(ctx, obs.StationID)
	if err != nil {
		return Bands{}, err
	}
	present, dups := false, 0
	if obs.EvidenceID != "" {
		present, dups, err = p.Photo(ctx, obs.EvidenceID)
		if err != nil {
			return Bands{}, err
		}
	}
	var mean *int64
	if m, ok, err := p.Regional(ctx, obs.StationID, obs.Product, obs.Unit); err != nil {
		return Bands{}, err
	} else if ok {
		mean = &m
	}
	bands := DeriveBands(BandInput{
		ReceivedAt: obs.ReceivedAt, ClaimedCapturedAt: obs.ClaimedCapturedAt,
		StationSite: site, Claim: nil,
		HasPhoto: present, DuplicateCount: dups,
		AmountMilli: obs.AmountMilli, RegionalMeanMilli: mean,
	}, now)
	if err := p.Store.UpsertSignals(ctx, obs.ID, bands, now); err != nil {
		return Bands{}, err
	}
	return bands, nil
}
