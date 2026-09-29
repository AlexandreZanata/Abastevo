package adapters

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	community "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/community"
	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
)

// SaveProjection persists one versioned projection with its restricted
// audit inputs atomically under the price-key advisory lock: versions
// move forward only, so a stale worker can never overwrite fresher
// output. A zero affected row fails loudly instead of hiding a guard
// regression.
func (s *Store) SaveProjection(ctx context.Context, key domain.PriceKey, result domain.Result, representativeID string, anchorAt, cutoff time.Time, next *time.Time, policyVersion string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tq := community.New(tx)
	if err := tq.LockPriceKey(ctx, application.PriceKeyString(key)); err != nil {
		return err
	}
	var version int64
	params, err := projectionParams(key)
	if err != nil {
		return err
	}
	if row, err := tq.GetProjection(ctx, params); err == nil {
		version = row.ProjectionVersion
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	amount := pgtype.Int8{Int64: result.AmountMilli, Valid: result.Verdict == domain.VerdictPrice}
	var rep pgtype.UUID
	if representativeID != "" {
		rep, err = mustUUID(representativeID)
		if err != nil {
			return err
		}
	}
	var nextAt pgtype.Timestamptz
	if next != nil {
		nextAt = pgTime(*next)
	}
	station, err := mustUUID(key.StationID)
	if err != nil {
		return err
	}
	n, err := tq.UpsertProjection(ctx, community.UpsertProjectionParams{
		StationID: station, FuelProduct: key.Product, Unit: key.Unit,
		ConditionKind: key.ConditionKind, QualifierKey: key.Qualifier,
		AmountMilliBrl: amount, Availability: availabilityOf(result), Confidence: result.Confidence,
		RepresentativeObservationID: rep,
		IndependentSupporters:       int32(result.Supporters), ConfirmationCount: int32(result.Confirmations),
		AnchorReceivedAt: pgTime(anchorAt), ExpiresAt: pgTime(result.ExpiresAt),
		NextRecomputeAt: nextAt, ComputedAt: pgTime(result.ComputedAt),
		ProjectionVersion: version + 1,
		AlgorithmVersion:  result.AlgorithmVersion, PolicyConfigVersion: policyVersion,
	})
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.New("adapters: projection version guard refused")
	}
	supporting := append(append([]string{}, result.WinnerObservationIDs...), result.WinnerConfirmationIDs...)
	m, err := tq.UpsertProjectionInputs(ctx, community.UpsertProjectionInputsParams{
		ProjectionKey: application.PriceKeyString(key), Version: version + 1,
		InputCutoff: pgTime(cutoff), SupportingEventIds: supporting,
		ReasonCodes: append([]string{}, result.Reasons...),
		ComputedAt:  pgTime(result.ComputedAt),
	})
	if err != nil {
		return err
	}
	if m != 1 {
		return errors.New("adapters: projection inputs guard refused")
	}
	return tx.Commit(ctx)
}

func projectionParams(key domain.PriceKey) (community.GetProjectionParams, error) {
	station, err := mustUUID(key.StationID)
	if err != nil {
		return community.GetProjectionParams{}, err
	}
	return community.GetProjectionParams{
		StationID: station, FuelProduct: key.Product,
		Unit: key.Unit, ConditionKind: key.ConditionKind, QualifierKey: key.Qualifier,
	}, nil
}

func availabilityOf(result domain.Result) string {
	switch result.Verdict {
	case domain.VerdictPrice:
		return "AVAILABLE"
	case domain.VerdictDisputed:
		return "DISPUTED"
	default:
		return "UNKNOWN"
	}
}

// EligibleAnchors lists validated facts for one exact key inside the
// consensus window, oldest first. State comes from the latest decision
// in SQL; rejected and decision-less rows never appear.
func (s *Store) EligibleAnchors(ctx context.Context, key domain.PriceKey, cutoff time.Time) ([]domain.Observation, error) {
	station, err := mustUUID(key.StationID)
	if err != nil {
		return nil, err
	}
	rows, err := community.New(s.pool).EligibleAnchors(ctx, community.EligibleAnchorsParams{
		StationID: station, FuelProduct: key.Product, Unit: key.Unit,
		ConditionKind: key.ConditionKind, QualifierKey: key.Qualifier,
		Cutoff: pgTime(cutoff),
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.Observation, 0, len(rows))
	for _, row := range rows {
		var claimed time.Time
		if row.ClaimedCapturedAt.Valid {
			claimed = row.ClaimedCapturedAt.Time
		}
		out = append(out, domain.Observation{
			ID: uuidString(row.ID), ContributorRef: row.ContributorRef,
			ClientSubmissionID: row.ClientSubmissionID,
			StationID:          uuidString(row.StationID),
			Product:            row.FuelProduct, Unit: row.Unit,
			AmountMilli: row.AmountMilliBrl, RawText: row.RawPriceText,
			ConditionKind: row.ConditionKind, QualifierKey: row.QualifierKey,
			EvidenceID: uuidString(row.EvidenceID),
			ReceivedAt: row.ReceivedAt.Time, ClaimedCapturedAt: claimed,
			SupersedesID:  uuidString(row.SupersedesID),
			PolicyVersion: row.PolicyVersion,
			Freshness:     domain.CaptureFreshness(claimed, row.ReceivedAt.Time),
		})
	}
	return out, nil
}

// ConfirmationsForAnchors lists support votes for the given anchors,
// oldest first. Eligibility against the anchor set stays with the
// caller: confirmations affirming ineligible anchors drop there.
func (s *Store) ConfirmationsForAnchors(ctx context.Context, anchorIDs []string) ([]application.ConfirmationVote, error) {
	if len(anchorIDs) == 0 {
		return nil, nil
	}
	ids := make([]pgtype.UUID, 0, len(anchorIDs))
	for _, id := range anchorIDs {
		uid, err := mustUUID(id)
		if err != nil {
			return nil, err
		}
		ids = append(ids, uid)
	}
	rows, err := community.New(s.pool).ConfirmationsForAnchors(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]application.ConfirmationVote, 0, len(rows))
	for _, row := range rows {
		out = append(out, application.ConfirmationVote{
			ID: uuidString(row.ID), ObservationID: uuidString(row.ObservationID),
			ContributorRef: row.ContributorRef, ReceivedAt: row.ReceivedAt.Time,
		})
	}
	return out, nil
}

// DueProjections lists price keys whose scheduled recompute passed,
// oldest wake first, for the boundary sweeper.
func (s *Store) DueProjections(ctx context.Context, now time.Time, batch int) ([]domain.PriceKey, error) {
	if batch < 1 {
		return nil, errors.New("adapters: due batch must be positive")
	}
	rows, err := community.New(s.pool).DueProjections(ctx, community.DueProjectionsParams{
		Now: pgTime(now), Batch: int32(batch),
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.PriceKey, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.PriceKey{
			StationID: uuidString(row.StationID), Product: row.FuelProduct,
			Unit: row.Unit, ConditionKind: row.ConditionKind, Qualifier: row.QualifierKey,
		})
	}
	return out, nil
}
