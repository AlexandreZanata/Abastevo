package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	dbprivacy "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/privacy"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
)

// Store persists privacy requests and their bounded outcomes.
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

// ArchiveSHA256 records the canonical content hash of export bytes.
func ArchiveSHA256(archive []byte) string {
	sum := sha256.Sum256(archive)
	return hex.EncodeToString(sum[:])
}

// exportPayload is the versioned build envelope: the request ID only,
// never owner data (B-BR-011).
func exportPayload(requestID string) []byte {
	raw, _ := json.Marshal(map[string]any{"version": 1, "request_id": requestID})
	return raw
}

func rowToRequest(row dbprivacy.PrivacyRequest) domain.Request {
	return domain.Request{
		ID: uuidString(row.ID), ContributorID: row.ContributorID,
		ContributorRef:     row.ContributorRef,
		ClientSubmissionID: row.ClientSubmissionID, Type: row.Type,
		Status: row.Status, ArchiveSHA256: row.ArchiveSha256,
		RequestedAt: row.RequestedAt.Time, ReadyAt: row.ReadyAt.Time,
		ExpiresAt: row.ExpiresAt.Time, CompletedAt: row.CompletedAt.Time,
		PolicyVersion: row.PolicyVersion,
	}
}

// RequestExport persists one intent with its durable build job in a
// single transaction. Identical retries converge on the owner natural
// key; divergent payloads surface the conflicting row so the caller
// maps a stable 409 instead of forking history.
func (s *Store) RequestExport(ctx context.Context, r domain.Request, enqueue func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error) (string, bool, error) {
	uid, err := mustUUID(r.ID)
	if err != nil {
		return "", false, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tq := dbprivacy.New(tx)
	inserted, err := tq.InsertRequest(ctx, dbprivacy.InsertRequestParams{
		ID: uid, ContributorID: r.ContributorID,
		ContributorRef:     r.ContributorRef,
		ClientSubmissionID: r.ClientSubmissionID, Type: r.Type,
		Status: r.Status, RequestedAt: pgTime(r.RequestedAt),
		PolicyVersion: r.PolicyVersion,
	})
	_ = inserted
	if err != nil {
		_ = tx.Rollback(ctx)
		if errors.Is(err, pgx.ErrNoRows) {
			return s.resolveRequestConflict(ctx, r)
		}
		return "", false, err
	}
	if err := enqueue(ctx, tx, "privacy-export-build", exportPayload(r.ID), "privacy-export:"+r.ID); err != nil {
		return "", false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", false, err
	}
	return r.ID, false, nil
}

// resolveRequestConflict compares a retried intent against the stored row.
// Identical retries converge on the stored identity regardless of the
// newly minted request ID; divergent payloads (type or owner reference)
// conflict instead of forking history.
func (s *Store) resolveRequestConflict(ctx context.Context, r domain.Request) (string, bool, error) {
	row, err := dbprivacy.New(s.pool).GetRequestByNaturalKey(ctx, dbprivacy.GetRequestByNaturalKeyParams{
		ContributorID: r.ContributorID, ClientSubmissionID: r.ClientSubmissionID,
	})
	if err != nil {
		return "", false, err
	}
	if row.Type != r.Type || row.ContributorRef != r.ContributorRef {
		return "", false, domain.ErrConflict
	}
	return uuidString(row.ID), true, nil
}

// CompleteExport stores one bounded archive with its hash through the
// guarded REQUESTED→READY transition. Oversize archives and wrong-state
// completions refuse before any write.
func (s *Store) CompleteExport(ctx context.Context, id string, archive []byte, sha string, readyAt time.Time) error {
	if len(archive) == 0 || len(archive) > domain.MaxArchiveBytes {
		return domain.ErrTooLarge
	}
	if ArchiveSHA256(archive) != sha {
		return domain.ErrMismatch
	}
	uid, err := mustUUID(id)
	if err != nil {
		return err
	}
	if _, err := dbprivacy.New(s.pool).MarkReady(ctx, dbprivacy.MarkReadyParams{
		ID: uid, Archive: archive, ArchiveSha256: sha,
		ReadyAt: pgTime(readyAt), ExpiresAt: pgTime(readyAt.Add(domain.DownloadTTL)),
		CompletedAt: pgTime(readyAt),
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrBadState
		}
		return err
	}
	return nil
}

// FailExport records one build failure through the guarded
// REQUESTED→FAILED transition.
func (s *Store) FailExport(ctx context.Context, id string, at time.Time) error {
	uid, err := mustUUID(id)
	if err != nil {
		return err
	}
	if _, err := dbprivacy.New(s.pool).MarkFailed(ctx, dbprivacy.MarkFailedParams{
		ID: uid, CompletedAt: pgTime(at),
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrBadState
		}
		return err
	}
	return nil
}

// Get returns one request for the durable build job. This unscoped
// read never serves owner traffic: every owner-facing path uses
// GetForOwner.
func (s *Store) Get(ctx context.Context, id string) (domain.Request, error) {
	uid, err := mustUUID(id)
	if err != nil {
		return domain.Request{}, err
	}
	row, err := dbprivacy.New(s.pool).GetRequest(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Request{}, domain.ErrNotFound
		}
		return domain.Request{}, err
	}
	return rowToRequest(row), nil
}

// GetForOwner returns one request only to its owner: contributor ID
// mismatch shares one not-found shape, so export existence is never an
// ownership oracle (B-BR-011).
func (s *Store) GetForOwner(ctx context.Context, contributorID, id string) (domain.Request, []byte, error) {
	uid, err := mustUUID(id)
	if err != nil {
		return domain.Request{}, nil, err
	}
	row, err := dbprivacy.New(s.pool).GetRequest(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Request{}, nil, domain.ErrNotFound
		}
		return domain.Request{}, nil, err
	}
	if row.ContributorID != contributorID {
		return domain.Request{}, nil, domain.ErrNotFound
	}
	return rowToRequest(row), row.Archive, nil
}
