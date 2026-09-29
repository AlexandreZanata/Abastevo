package adapters

import (
	"context"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmoderation "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/moderation"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/domain"
)

// Store persists moderation cases and serves the operator queue.
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

func uuidOrNull(s string) (pgtype.UUID, error) {
	if s == "" {
		return pgtype.UUID{}, nil
	}
	return mustUUID(s)
}

// rowToCase maps one generated row onto the domain case. The evidence
// reference stays an identifier; no payload is ever copied (B-BR-011).
func rowToCase(row dbmoderation.ModerationCase) domain.Case {
	return domain.Case{
		ID: uuidString(row.ID), TargetType: row.TargetType,
		TargetID: row.TargetID, Status: row.Status,
		Priority: row.Priority, Reason: row.Reason, Detail: row.Detail,
		EvidenceID: uuidString(row.EvidenceID),
		OpenedAt:   row.OpenedAt.Time, PolicyVersion: row.PolicyVersion,
	}
}

// OpenCase inserts one case. An already-open case for the same target
// converges: the open row returns with replayed=true so duplicate
// reports never flood the queue. Retries with the same target converge
// the same way; resolution history stays in P07-T02 actions.
func (s *Store) OpenCase(ctx context.Context, c domain.Case) (string, bool, error) {
	uid, err := mustUUID(c.ID)
	if err != nil {
		return "", false, err
	}
	evidence, err := uuidOrNull(c.EvidenceID)
	if err != nil {
		return "", false, err
	}
	inserted, err := dbmoderation.New(s.pool).InsertCase(ctx, dbmoderation.InsertCaseParams{
		ID: uid, TargetType: c.TargetType, TargetID: c.TargetID,
		Status: c.Status, Priority: c.Priority, Reason: c.Reason,
		Detail: c.Detail, EvidenceID: evidence,
		OpenedAt: pgTime(c.OpenedAt), PolicyVersion: c.PolicyVersion,
	})
	if err == nil {
		return uuidString(inserted), false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, err
	}
	// DO NOTHING hit a conflict: converge on the open case for this
	// target. Any unique violation outside the open-target index
	// surfaces here as the open row missing, which fails loudly.
	row, err := dbmoderation.New(s.pool).GetOpenCase(ctx, dbmoderation.GetOpenCaseParams{
		TargetType: c.TargetType, TargetID: c.TargetID,
	})
	if err != nil {
		return "", false, err
	}
	return uuidString(row.ID), true, nil
}

// Get returns one case by ID. Missing rows share one not-found shape so
// callers cannot probe for other targets.
func (s *Store) Get(ctx context.Context, id string) (domain.Case, error) {
	uid, err := mustUUID(id)
	if err != nil {
		return domain.Case{}, err
	}
	row, err := dbmoderation.New(s.pool).GetCase(ctx, uid)
	if err != nil {
		return domain.Case{}, err
	}
	return rowToCase(row), nil
}

// ListOpen serves the operator queue: actionable cases ordered by
// priority rank then age with a stable UUID tie-break. The cursor is the
// last row's (rank, opened_at, id); the first page passes rank -1 with
// an empty cursor ID, which maps onto epoch sentinels that sort before
// every real row (rank -1 dominates the tuple comparison).
func (s *Store) ListOpen(ctx context.Context, cursorRank int32, cursorAt time.Time, cursorID string, limit int32) ([]domain.Case, error) {
	if limit < 1 || limit > 100 {
		return nil, errors.New("adapters: page limit out of range")
	}
	cursorOpenedAt := pgTime(cursorAt)
	var cursorUUID pgtype.UUID
	if cursorID == "" {
		// First page: valid sentinels that sort before all rows.
		cursorOpenedAt = pgtype.Timestamptz{Time: time.Unix(0, 0).UTC(), Valid: true}
		cursorUUID = pgtype.UUID{Bytes: [16]byte{}, Valid: true}
		if cursorRank > -1 {
			cursorRank = -1
		}
	} else {
		var err error
		cursorUUID, err = mustUUID(cursorID)
		if err != nil {
			return nil, err
		}
	}
	rows, err := dbmoderation.New(s.pool).ListOpenCases(ctx, dbmoderation.ListOpenCasesParams{
		CursorRank: cursorRank, CursorOpenedAt: cursorOpenedAt,
		CursorID: cursorUUID, PageLimit: limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.Case, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowToCase(row))
	}
	return out, nil
}
