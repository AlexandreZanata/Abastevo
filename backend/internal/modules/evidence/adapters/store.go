package adapters

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	evidence "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/evidence"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
)

// Community-facing bind errors. The composition root maps them onto the
// community sentinels; this package never imports community code.
var (
	ErrUnknownObject = errors.New("evidence: unknown object")
	ErrNotOwner      = errors.New("evidence: object owned by another contributor")
	ErrAlreadyBound  = errors.New("evidence: object already bound to another observation")
	ErrNoObject      = errors.New("evidence: no verified object for session")
)

// Store persists sessions and verified objects.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore wires the owned generated queries to a pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// ObjectData carries one verified snapshot for atomic persistence.
type ObjectData struct {
	ID              string
	FinalKey        string
	SourceSHA256    string
	SanitizedSHA256 string
	Width           int
	Height          int
	DHash           uint64
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

// ReserveSession persists one reservation, converging identical retries
// on the natural key and conflicting on divergent payloads (B-BR-005).
// It returns the stored row — the winner on convergence — so callers
// presign from persisted facts, never from loser intent.
func (s *Store) ReserveSession(ctx context.Context, sess domain.Session) (domain.Session, bool, error) {
	uid, err := mustUUID(sess.ID)
	if err != nil {
		return domain.Session{}, false, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Session{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tq := evidence.New(tx)
	inserted, err := tq.InsertSession(ctx, evidence.InsertSessionParams{
		ID: uid, ContributorRef: sess.ContributorRef,
		ClientSessionID: sess.ClientSessionID, Mime: sess.MIME,
		DeclaredBytes: sess.DeclaredBytes, ClaimedSha256: sess.ClaimedSHA256,
		QuarantineKey: sess.QuarantineKey, Status: sess.Status,
		CreatedAt: pgTime(sess.CreatedAt), ExpiresAt: pgTime(sess.ExpiresAt),
		PolicyVersion: sess.PolicyVersion,
	})
	_ = inserted
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			_ = tx.Rollback(ctx)
			return s.resolveConflict(ctx, sess)
		}
		return domain.Session{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Session{}, false, err
	}
	return sess, false, nil
}

// resolveConflict compares a retried reservation against the stored row:
// identical replays converge on the winner, divergent payloads conflict.
func (s *Store) resolveConflict(ctx context.Context, sess domain.Session) (domain.Session, bool, error) {
	row, err := evidence.New(s.pool).GetSessionByNaturalKey(ctx, evidence.GetSessionByNaturalKeyParams{
		ContributorRef: sess.ContributorRef, ClientSessionID: sess.ClientSessionID,
	})
	if err != nil {
		return domain.Session{}, false, err
	}
	if row.Mime != sess.MIME || row.DeclaredBytes != sess.DeclaredBytes ||
		!strings.EqualFold(row.ClaimedSha256, sess.ClaimedSHA256) {
		return domain.Session{}, false, domain.ErrConflict
	}
	return mapSession(row.ID, row.ContributorRef, row.ClientSessionID, row.Mime, row.DeclaredBytes, row.ClaimedSha256, row.QuarantineKey, row.Status, row.CreatedAt, row.ExpiresAt, row.UpdatedAt, row.PolicyVersion), true, nil
}

// CompleteSession claims ISSUED→VERIFYING and enqueues the verify job in
// one transaction: no accepted completion waits without work. An already
// VERIFYING session converges (its job dedupe converges too); any other
// state refuses.
func (s *Store) CompleteSession(ctx context.Context, sessionID string, enqueue func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error) error {
	uid, err := mustUUID(sessionID)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tq := evidence.New(tx)
	n, err := tq.ClaimVerifying(ctx, uid)
	if err != nil {
		return err
	}
	if n == 0 {
		_ = tx.Rollback(ctx)
		return s.convergeComplete(ctx, uid, sessionID, enqueue)
	}
	payload, _ := json.Marshal(map[string]any{"version": 1, "upload_id": sessionID})
	if err := enqueue(ctx, tx, "verify-evidence", payload, "verify:"+sessionID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// convergeComplete handles a lost claim race: VERIFYING converges with a
// (deduplicated) job, terminal states refuse.
func (s *Store) convergeComplete(ctx context.Context, uid pgtype.UUID, sessionID string, enqueue func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error) error {
	row, err := evidence.New(s.pool).GetSession(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("adapters: unknown session")
		}
		return err
	}
	if row.Status != domain.StateVerifying {
		return domain.ErrBadTransition
	}
	payload, _ := json.Marshal(map[string]any{"version": 1, "upload_id": sessionID})
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := enqueue(ctx, tx, "verify-evidence", payload, "verify:"+sessionID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// RecordVerified persists VERIFYING→READY with its object row atomically:
// the DB points at the final object only together with the READY state.
// It returns the object id (fresh or converged): a concurrent terminal
// converges when it already holds this outcome; the uploaded final bytes
// stay referenced exactly once, and a commit failure leaves them
// unreferenced for the orphan sweeper.
func (s *Store) RecordVerified(ctx context.Context, sessionID string, obj ObjectData) (string, error) {
	sessionUUID, err := mustUUID(sessionID)
	if err != nil {
		return "", err
	}
	objectUUID, err := mustUUID(obj.ID)
	if err != nil {
		return "", err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tq := evidence.New(tx)
	n, err := tq.MarkSessionReady(ctx, sessionUUID)
	if err != nil {
		return "", err
	}
	if n == 0 {
		_ = tx.Rollback(ctx)
		return s.convergeVerified(ctx, sessionUUID, obj)
	}
	if _, err := tq.InsertObject(ctx, evidence.InsertObjectParams{
		ID: objectUUID, SessionID: sessionUUID, FinalKey: obj.FinalKey,
		SourceSha256: obj.SourceSHA256, SanitizedSha256: obj.SanitizedSHA256,
		Width: int32(obj.Width), Height: int32(obj.Height),
		Dhash: int64(obj.DHash), CreatedAt: pgTime(time.Now()),
	}); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return obj.ID, nil
}

// convergeVerified replays a lost READY race: same outcome converges with
// the existing object id, anything else refuses instead of forking
// history.
func (s *Store) convergeVerified(ctx context.Context, sessionUUID pgtype.UUID, obj ObjectData) (string, error) {
	q := evidence.New(s.pool)
	row, err := q.GetObjectBySession(ctx, sessionUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", domain.ErrBadTransition
		}
		return "", err
	}
	if row.FinalKey != obj.FinalKey || row.SourceSha256 != obj.SourceSHA256 {
		return "", domain.ErrBadTransition
	}
	return uuidString(row.ID), nil
}

// RecordRejected persists VERIFYING→REJECTED with stable reason codes.
func (s *Store) RecordRejected(ctx context.Context, sessionID string, reasons []string) error {
	uid, err := mustUUID(sessionID)
	if err != nil {
		return err
	}
	n, err := evidence.New(s.pool).MarkSessionRejected(ctx, evidence.MarkSessionRejectedParams{
		ID: uid, ReasonCodes: append([]string{}, reasons...),
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrBadTransition
	}
	return nil
}

// Session loads one reservation by ID.
func (s *Store) Session(ctx context.Context, id string) (domain.Session, error) {
	uid, err := mustUUID(id)
	if err != nil {
		return domain.Session{}, err
	}
	row, err := evidence.New(s.pool).GetSession(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Session{}, errors.New("adapters: unknown session")
		}
		return domain.Session{}, err
	}
	return mapSession(row.ID, row.ContributorRef, row.ClientSessionID, row.Mime, row.DeclaredBytes, row.ClaimedSha256, row.QuarantineKey, row.Status, row.CreatedAt, row.ExpiresAt, row.UpdatedAt, row.PolicyVersion), nil
}

// SessionByNaturalKey loads one reservation by its retry key.
func (s *Store) SessionByNaturalKey(ctx context.Context, ref, client string) (domain.Session, error) {
	row, err := evidence.New(s.pool).GetSessionByNaturalKey(ctx, evidence.GetSessionByNaturalKeyParams{
		ContributorRef: ref, ClientSessionID: client,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Session{}, errors.New("adapters: unknown session")
		}
		return domain.Session{}, err
	}
	return mapSession(row.ID, row.ContributorRef, row.ClientSessionID, row.Mime, row.DeclaredBytes, row.ClaimedSha256, row.QuarantineKey, row.Status, row.CreatedAt, row.ExpiresAt, row.UpdatedAt, row.PolicyVersion), nil
}

// ObjectSignals resolves one object's duplicate-detection signals for
// consensus input derivation: the perceptual hash plus the owner the
// community port already trusts. Missing objects report ErrNoObject;
// purged payloads keep their hashes by design (bounded signals).
func (s *Store) ObjectSignals(ctx context.Context, objectID string) (dhash uint64, ownerRef string, err error) {
	uid, err := mustUUID(objectID)
	if err != nil {
		return 0, "", err
	}
	row, err := evidence.New(s.pool).GetObject(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, "", ErrNoObject
		}
		return 0, "", err
	}
	return uint64(row.Dhash), row.ContributorRef, nil
}

// ObjectIDBySession resolves the verified object of one session for
// owner status reads. Sessions without an object (never READY) report
// ErrNoObject instead of an empty id.
func (s *Store) ObjectIDBySession(ctx context.Context, sessionID string) (string, error) {
	uid, err := mustUUID(sessionID)
	if err != nil {
		return "", err
	}
	row, err := evidence.New(s.pool).GetObjectBySession(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNoObject
		}
		return "", err
	}
	return uuidString(row.ID), nil
}

// CountSince counts reservations by one contributor since a bound: the
// daily photo cap input for the reservation use-case.
func (s *Store) CountSince(ctx context.Context, ref string, since time.Time) (int, error) {
	n, err := evidence.New(s.pool).CountSessionsSince(ctx, evidence.CountSessionsSinceParams{
		ContributorRef: ref, Since: pgTime(since),
	})
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

func checkBatch(batch int) (int32, error) {
	if batch < 1 {
		return 0, errors.New("adapters: sweep batch must be positive")
	}
	return int32(batch), nil
}

// ExpireIdleSessions flips past-deadline ISSUED sessions to EXPIRED,
// returning their quarantine keys for object cleanup.
func (s *Store) ExpireIdleSessions(ctx context.Context, now time.Time, batch int) ([]domain.SessionRef, error) {
	n, err := checkBatch(batch)
	if err != nil {
		return nil, err
	}
	rows, err := evidence.New(s.pool).ExpireIdleSessions(ctx, evidence.ExpireIdleSessionsParams{
		Now: pgTime(now), Batch: n,
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.SessionRef, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.SessionRef{ID: uuidString(row.ID), QuarantineKey: row.QuarantineKey})
	}
	return out, nil
}

// ListStuckVerifying lists VERIFYING sessions older than the cutoff: the
// sweeper expires the ones without a live job and requeues the young
// ones it cannot see a job for.
func (s *Store) ListStuckVerifying(ctx context.Context, cutoff time.Time, batch int) ([]domain.SessionRef, error) {
	n, err := checkBatch(batch)
	if err != nil {
		return nil, err
	}
	rows, err := evidence.New(s.pool).ListStuckVerifying(ctx, evidence.ListStuckVerifyingParams{
		Cutoff: pgTime(cutoff), Batch: n,
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.SessionRef, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.SessionRef{
			ID: uuidString(row.ID), QuarantineKey: row.QuarantineKey,
			Status: domain.StateVerifying, CreatedAt: row.CreatedAt.Time,
		})
	}
	return out, nil
}

// ListYoungVerifying lists recent VERIFYING sessions for live-job
// reconciliation: missing jobs are requeued, never deleted.
func (s *Store) ListYoungVerifying(ctx context.Context, cutoff time.Time, batch int) ([]domain.SessionRef, error) {
	n, err := checkBatch(batch)
	if err != nil {
		return nil, err
	}
	rows, err := evidence.New(s.pool).ListYoungVerifying(ctx, evidence.ListYoungVerifyingParams{
		Cutoff: pgTime(cutoff), Batch: n,
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.SessionRef, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.SessionRef{
			ID: uuidString(row.ID), QuarantineKey: row.QuarantineKey,
			Status: domain.StateVerifying, CreatedAt: row.CreatedAt.Time,
		})
	}
	return out, nil
}

// ExpireStuckSession flips one stuck VERIFYING session to EXPIRED after
// the sweeper confirmed no live job holds it. The age cutoff rides in
// the guarded statement itself, so a young session can never expire
// through this path even if a caller skips the list step.
func (s *Store) ExpireStuckSession(ctx context.Context, id string, cutoff time.Time) error {
	uid, err := mustUUID(id)
	if err != nil {
		return err
	}
	n, err := evidence.New(s.pool).ExpireStuckSession(ctx, evidence.ExpireStuckSessionParams{
		ID: uid, Cutoff: pgTime(cutoff),
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrBadTransition
	}
	return nil
}

// MarkQuarantineDeleted records a removed original upload. Set-if-unset
// converges repeated runs; unknown ids surface as errors.
func (s *Store) MarkQuarantineDeleted(ctx context.Context, id string) error {
	uid, err := mustUUID(id)
	if err != nil {
		return err
	}
	if _, err := evidence.New(s.pool).MarkQuarantineDeleted(ctx, uid); err != nil {
		return err
	}
	return nil
}

// QuarantineCandidates lists sessions whose originals may leave storage:
// terminal or older-than-cap sessions with no deletion marker yet.
// VERIFYING sessions only appear here past the hard cap, after the stuck
// pass already expired the jobless ones.
func (s *Store) QuarantineCandidates(ctx context.Context, oldCutoff time.Time, batch int) ([]domain.SessionRef, error) {
	n, err := checkBatch(batch)
	if err != nil {
		return nil, err
	}
	rows, err := evidence.New(s.pool).QuarantineCandidates(ctx, evidence.QuarantineCandidatesParams{
		OldCutoff: pgTime(oldCutoff), Batch: n,
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.SessionRef, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.SessionRef{
			ID: uuidString(row.ID), QuarantineKey: row.QuarantineKey,
			Status: row.Status, CreatedAt: row.CreatedAt.Time,
		})
	}
	return out, nil
}

// StaleFinalCandidates lists sanitized objects past the default
// retention for policy evaluation in Go (extension and absolute cap).
func (s *Store) StaleFinalCandidates(ctx context.Context, youngCutoff time.Time, batch int) ([]domain.ObjectRef, error) {
	n, err := checkBatch(batch)
	if err != nil {
		return nil, err
	}
	rows, err := evidence.New(s.pool).StaleFinalCandidates(ctx, evidence.StaleFinalCandidatesParams{
		YoungCutoff: pgTime(youngCutoff), Batch: n,
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.ObjectRef, 0, len(rows))
	for _, row := range rows {
		obj := domain.ObjectRef{
			ID: uuidString(row.ID), FinalKey: row.FinalKey,
			CreatedAt:    row.CreatedAt.Time,
			HasExtension: row.RetentionExtendedUntil.Valid,
		}
		if obj.HasExtension {
			obj.ExtendedUntil = row.RetentionExtendedUntil.Time
		}
		out = append(out, obj)
	}
	return out, nil
}

// MarkFinalDeleted records removed sanitized bytes, keeping the row with
// its duplicate-signal hashes to the 90-day bound. Convergent on repeat.
func (s *Store) MarkFinalDeleted(ctx context.Context, id string) error {
	uid, err := mustUUID(id)
	if err != nil {
		return err
	}
	if _, err := evidence.New(s.pool).MarkFinalDeleted(ctx, uid); err != nil {
		return err
	}
	return nil
}

// PurgeObjectsBefore deletes object rows past the hash-retention bound,
// returning the purged count for the observable sweep report.
func (s *Store) PurgeObjectsBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	return evidence.New(s.pool).PurgeObjectsBefore(ctx, pgTime(cutoff))
}

// FindByDHash resolves recent objects sharing one perceptual hash: the
// bounded duplicate-media signal for independence checks downstream.
func (s *Store) FindByDHash(ctx context.Context, dhash uint64, since time.Time) ([]domain.ObjectRef, error) {
	rows, err := evidence.New(s.pool).FindByDHash(ctx, evidence.FindByDHashParams{
		Dhash: int64(dhash), Since: pgTime(since),
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.ObjectRef, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.ObjectRef{
			ID: uuidString(row.ID), SessionID: uuidString(row.SessionID),
			FinalKey: row.FinalKey, CreatedAt: row.CreatedAt.Time,
		})
	}
	return out, nil
}

func mapSession(id pgtype.UUID, contributorRef, clientSessionID, mime string, declaredBytes int64, claimedSha256, quarantineKey, status string, createdAt, expiresAt, updatedAt pgtype.Timestamptz, policyVersion string) domain.Session {
	return domain.Session{
		ID: uuidString(id), ContributorRef: contributorRef,
		ClientSessionID: clientSessionID, MIME: mime,
		DeclaredBytes: declaredBytes, MaxBytes: domain.MaxUploadBytes,
		ClaimedSHA256: claimedSha256, QuarantineKey: quarantineKey,
		Status: status, CreatedAt: createdAt.Time,
		ExpiresAt: expiresAt.Time, UpdatedAt: updatedAt.Time,
		PolicyVersion: policyVersion,
	}
}
