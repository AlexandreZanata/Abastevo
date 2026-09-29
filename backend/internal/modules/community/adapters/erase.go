package adapters

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	community "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/community"
)

// EraseContributor unlinks one owner's community footprint in the
// narrow erasure workflow (P07-T04, B-BR-016, ADR-008): observations
// keep their price facts with an anonymized reference, reporter
// references unlink per row (pair uniqueness survives), and every
// affected price key gets exactly one consensus recompute job so
// projections converge without the erased votes. The unlink triple
// and the job intents commit atomically; a concurrent new observation
// converges on ledger replay.
func (s *Store) EraseContributor(ctx context.Context, ref, anon string, enqueue func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error) (unlinkedObs, unlinkedVotes int64, keys int, err error) {
	// Affected keys resolve before unlinking, while the owner
	// reference still selects them. Reads page bounded; erasure is a
	// rare background operation, not a hot path.
	type priceKey struct {
		station, product, unit, condition, qualifier, observation string
	}
	seen := map[priceKey]bool{}
	var order []priceKey
	var afterTime pgtype.Timestamptz
	var afterID string
	hasCursor := false
	for total := 0; total < 100000; {
		rows, err := community.New(s.pool).ListByContributor(ctx, community.ListByContributorParams{
			ContributorRef: ref, HasCursor: hasCursor,
			AfterTime: afterTime, AfterID: afterID, LimitPlusOne: 500,
		})
		if err != nil {
			return 0, 0, 0, err
		}
		if len(rows) == 0 {
			break
		}
		for _, row := range rows {
			short := priceKey{
				station: uuidString(row.StationID), product: row.FuelProduct,
				unit: row.Unit, condition: row.ConditionKind,
				qualifier: row.QualifierKey,
			}
			if !seen[short] {
				seen[short] = true
				short.observation = uuidString(row.ID)
				order = append(order, short)
			}
			total++
		}
		last := rows[len(rows)-1]
		afterTime, afterID, hasCursor = last.ReceivedAt, uuidString(last.ID), true
		if len(rows) < 500 {
			break
		}
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, 0, 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tq := community.New(tx)
	// The anonymized reference is an opaque token: it never parses as
	// identity and never resolves to a contributor.
	unlinkedObs, err = tq.UnlinkObservations(ctx, community.UnlinkObservationsParams{
		ContributorRef: ref, AnonRef: anon,
	})
	if err != nil {
		return 0, 0, 0, err
	}
	var conf, disp int64
	if conf, err = tq.UnlinkConfirmations(ctx, ref); err != nil {
		return 0, 0, 0, err
	}
	if disp, err = tq.UnlinkDisputes(ctx, ref); err != nil {
		return 0, 0, 0, err
	}
	unlinkedVotes = conf + disp
	for _, k := range order {
		if err := enqueue(ctx, tx, "community-consensus", consensusPayload(k.observation), "consensus:"+k.observation); err != nil {
			return 0, 0, 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, 0, 0, err
	}
	return unlinkedObs, unlinkedVotes, len(order), nil
}
