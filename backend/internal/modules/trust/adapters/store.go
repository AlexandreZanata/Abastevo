package adapters

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	trust "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/trust"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trust/domain"
)

// Store persists trust decisions and the current-tier view.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore wires the owned generated queries to a pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func mustUUID(text string) (pgtype.UUID, error) {
	raw, err := hex.DecodeString(stripDashes(text))
	if err != nil || len(raw) != 16 {
		return pgtype.UUID{}, errors.New("adapters: malformed UUID")
	}
	var id pgtype.UUID
	copy(id.Bytes[:], raw)
	id.Valid = true
	return id, nil
}

func stripDashes(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '-' {
			out = append(out, s[i])
		}
	}
	return string(out)
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	hexed := hex.EncodeToString(id.Bytes[:])
	return hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" +
		hexed[16:20] + "-" + hexed[20:32]
}

func pgTime(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// AppendDecision persists one verdict with the current-tier projection
// and the affected-key recompute job in one transaction. Concurrent
// appends all land in history; the current view converges on the
// latest committed verdict with a bumped version.
func (s *Store) AppendDecision(ctx context.Context, d domain.Decision, enqueue func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error) (string, error) {
	uid, err := mustUUID(d.ID)
	if err != nil {
		return "", err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tq := trust.New(tx)
	inserted, err := tq.InsertDecision(ctx, trust.InsertDecisionParams{
		ID: uid, ContributorRef: d.ContributorRef, Tier: d.Tier,
		Reason: d.Reason, CaseRefs: append([]string{}, d.CaseRefs...),
		PolicyVersion: d.PolicyVersion, OccurredAt: pgTime(d.OccurredAt),
	})
	_ = inserted
	if err != nil {
		return "", err
	}
	if _, err := tq.UpsertCurrent(ctx, trust.UpsertCurrentParams{
		ContributorRef: d.ContributorRef, Tier: d.Tier, UpdatedAt: pgTime(d.OccurredAt),
	}); err != nil {
		return "", err
	}
	payload, _ := json.Marshal(map[string]any{"version": 1, "contributor_ref": d.ContributorRef})
	if err := enqueue(ctx, tx, "trust-affected-recompute", payload, "trust:"+d.ContributorRef); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return d.ID, nil
}

// Tier reads the current tier, defaulting unknown contributors to NEW:
// absence of history is newness, never suspicion.
func (s *Store) Tier(ctx context.Context, contributorRef string) (string, error) {
	row, err := trust.New(s.pool).GetCurrent(ctx, contributorRef)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.TierNew, nil
		}
		return "", err
	}
	return row.Tier, nil
}

// History lists one contributor's verdicts oldest-first for audit and
// rebuilds.
func (s *Store) History(ctx context.Context, contributorRef string) ([]domain.Decision, error) {
	rows, err := trust.New(s.pool).ListDecisions(ctx, contributorRef)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Decision, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.Decision{
			ID: uuidString(row.ID), ContributorRef: row.ContributorRef,
			Tier: row.Tier, Reason: row.Reason,
			CaseRefs:   append([]string{}, row.CaseRefs...),
			OccurredAt: row.OccurredAt.Time, PolicyVersion: row.PolicyVersion,
		})
	}
	return out, nil
}
