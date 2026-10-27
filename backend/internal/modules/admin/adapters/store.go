// Package adapters persists administrative sessions, login counters,
// idempotency and audit rows. SQL only runs through generated, parameterized
// queries; raw credentials never reach storage.
package adapters

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	qadmin "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/admin"
	accountdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	ipWindow            = 15 * time.Minute
	globalWindow        = time.Hour
	ipFailureLimit      = 5
	globalFailureLimit  = 50
	globalBlockDuration = 15 * time.Minute
)

type Store struct {
	Pool   *pgxpool.Pool
	NewID  func() (string, error)
	Random io.Reader
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{Pool: pool, NewID: newUUID, Random: rand.Reader}
}

func uid(raw string) pgtype.UUID {
	var id pgtype.UUID
	_ = id.Scan(raw)
	return id
}

func idstr(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	raw := hex.EncodeToString(id.Bytes[:])
	return raw[0:8] + "-" + raw[8:12] + "-" + raw[12:16] + "-" + raw[16:20] + "-" + raw[20:32]
}

func stamp(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t.UTC(), Valid: !t.IsZero()}
}

func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	raw := hex.EncodeToString(b[:])
	return raw[0:8] + "-" + raw[8:12] + "-" + raw[12:16] + "-" + raw[16:20] + "-" + raw[20:32], nil
}

func token(random io.Reader, size int) (string, error) {
	raw := make([]byte, size)
	if _, err := io.ReadFull(random, raw); err != nil {
		return "", err
	}
	return domain.NewToken(raw), nil
}

// Login serializes every attempt on the singleton global window row, checks
// both limits, then runs the constant-time key comparison supplied by the
// application. Only real key mismatches increment the failure counters.
func (s *Store) Login(ctx context.Context, in app.LoginInput) (app.LoginResult, error) {
	if s == nil || s.Pool == nil || s.NewID == nil || s.Random == nil {
		return app.LoginResult{}, domain.ErrUnavailable
	}
	if !domain.ValidHash(in.PresentedHash) || !domain.ValidHash(in.IPHash) || in.Verify == nil || in.Now.IsZero() {
		return app.LoginResult{}, domain.ErrInvalid
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return app.LoginResult{}, domain.ErrUnavailable
	}
	defer rollback(tx)
	q := qadmin.New(tx)
	globalStart := in.Now.Truncate(globalWindow)
	ipStart := in.Now.Truncate(ipWindow)
	global, err := q.LockLoginWindow(ctx, qadmin.LockLoginWindowParams{
		Scope: "global", Subject: "global", WindowStart: stamp(globalStart),
	})
	if err != nil {
		return app.LoginResult{}, domain.ErrUnavailable
	}
	ip, err := q.LockLoginWindow(ctx, qadmin.LockLoginWindowParams{
		Scope: "ip", Subject: in.IPHash, WindowStart: stamp(ipStart),
	})
	if err != nil {
		return app.LoginResult{}, domain.ErrUnavailable
	}
	if retry, blocked, err := s.loginLimit(ctx, q, global, ip, globalStart, ipStart, in.Now); err != nil {
		return app.LoginResult{}, err
	} else if blocked {
		return app.LoginResult{}, &domain.RateLimitedError{RetryAfter: retry}
	}

	fingerprint, ok := in.Verify()
	if !ok {
		blockedUntil := in.Now.Add(globalBlockDuration)
		if _, err := q.IncrementLoginFailures(ctx, qadmin.IncrementLoginFailuresParams{
			GlobalLimit: globalFailureLimit, BlockedUntil: stamp(blockedUntil),
			Scope: "global", Subject: "global", WindowStart: stamp(globalStart),
		}); err != nil {
			return app.LoginResult{}, domain.ErrUnavailable
		}
		if _, err := q.IncrementLoginFailures(ctx, qadmin.IncrementLoginFailuresParams{
			GlobalLimit: globalFailureLimit, BlockedUntil: stamp(blockedUntil),
			Scope: "ip", Subject: in.IPHash, WindowStart: stamp(ipStart),
		}); err != nil {
			return app.LoginResult{}, domain.ErrUnavailable
		}
		if err := q.InsertAudit(ctx, qadmin.InsertAuditParams{
			Action: "admin.login.failed", TargetType: "admin",
			RequestID: in.RequestID, ClientIpHash: in.IPHash,
		}); err != nil {
			return app.LoginResult{}, domain.ErrUnavailable
		}
		if err := tx.Commit(ctx); err != nil {
			return app.LoginResult{}, domain.ErrUnavailable
		}
		return app.LoginResult{}, domain.ErrDenied
	}

	sessionID, err := s.NewID()
	if err != nil || !domain.ValidUUID(sessionID) {
		return app.LoginResult{}, domain.ErrUnavailable
	}
	rawToken, err := token(s.Random, domain.SessionTokenBytes)
	if err != nil {
		return app.LoginResult{}, domain.ErrUnavailable
	}
	rawCSRF, err := token(s.Random, domain.SessionTokenBytes)
	if err != nil {
		return app.LoginResult{}, domain.ErrUnavailable
	}
	tokenSalt, err := token(s.Random, domain.SessionSaltBytes)
	if err != nil {
		return app.LoginResult{}, domain.ErrUnavailable
	}
	csrfSalt, err := token(s.Random, domain.SessionSaltBytes)
	if err != nil {
		return app.LoginResult{}, domain.ErrUnavailable
	}
	expires := in.Now.Add(domain.AbsoluteLifetime)
	if err := q.InsertSession(ctx, qadmin.InsertSessionParams{
		ID:             uid(sessionID),
		TokenLookup:    domain.LookupDigest(rawToken),
		TokenSalt:      tokenSalt,
		TokenHash:      domain.SaltedHash(tokenSalt, rawToken),
		CsrfSalt:       csrfSalt,
		CsrfHash:       domain.SaltedHash(csrfSalt, rawCSRF),
		KeyFingerprint: fingerprint,
		CreatedAt:      stamp(in.Now),
		LastSeenAt:     stamp(in.Now),
		ExpiresAt:      stamp(expires),
	}); err != nil {
		return app.LoginResult{}, domain.ErrUnavailable
	}
	if err := q.InsertAudit(ctx, qadmin.InsertAuditParams{
		SessionID: uid(sessionID), Action: "admin.login",
		TargetType: "admin_session", TargetID: sessionID,
		RequestID: in.RequestID, ClientIpHash: in.IPHash,
	}); err != nil {
		return app.LoginResult{}, domain.ErrUnavailable
	}
	if err := tx.Commit(ctx); err != nil {
		return app.LoginResult{}, domain.ErrUnavailable
	}
	return app.LoginResult{Token: rawToken, CSRFToken: rawCSRF, Fingerprint: fingerprint, ExpiresAt: expires}, nil
}

func (s *Store) loginLimit(ctx context.Context, q *qadmin.Queries, global, ip qadmin.LockLoginWindowRow, globalStart, ipStart, now time.Time) (time.Duration, bool, error) {
	if global.BlockedUntil.Valid && now.Before(global.BlockedUntil.Time) {
		return retryAfter(global.BlockedUntil.Time, now), true, nil
	}
	if global.Failures >= globalFailureLimit {
		until := now.Add(globalBlockDuration)
		if err := q.BlockLoginWindow(ctx, qadmin.BlockLoginWindowParams{
			BlockedUntil: stamp(until), Scope: "global", Subject: "global", WindowStart: stamp(globalStart),
		}); err != nil {
			return 0, false, domain.ErrUnavailable
		}
		return globalBlockDuration, true, nil
	}
	if ip.Failures >= ipFailureLimit {
		return retryAfter(ipStart.Add(ipWindow), now), true, nil
	}
	return 0, false, nil
}

func retryAfter(until, now time.Time) time.Duration {
	wait := until.Sub(now)
	if wait < time.Millisecond {
		return time.Millisecond
	}
	return wait
}

func (s *Store) Authenticate(ctx context.Context, rawToken string, now time.Time) (app.Session, error) {
	if s == nil || s.Pool == nil || !domain.ValidToken(rawToken) {
		return app.Session{}, domain.ErrUnauthorized
	}
	row, err := qadmin.New(s.Pool).TouchSession(ctx, qadmin.TouchSessionParams{
		TokenLookup: domain.LookupDigest(rawToken),
		Now:         stamp(now),
		IdleCutoff:  stamp(now.Add(-domain.IdleTimeout)),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return app.Session{}, domain.ErrUnauthorized
	}
	if err != nil {
		return app.Session{}, domain.ErrUnavailable
	}
	want := domain.SaltedHash(row.TokenSalt, rawToken)
	if subtle.ConstantTimeCompare([]byte(want), []byte(row.TokenHash)) != 1 {
		return app.Session{}, domain.ErrUnauthorized
	}
	return app.Session{
		ID:             idstr(row.ID),
		KeyFingerprint: row.KeyFingerprint,
		CSRFSalt:       row.CsrfSalt,
		CSRFHash:       row.CsrfHash,
		CreatedAt:      row.CreatedAt.Time,
		LastSeenAt:     row.LastSeenAt.Time,
		ExpiresAt:      row.ExpiresAt.Time,
	}, nil
}

func (s *Store) Logout(ctx context.Context, session app.Session, m app.Mutation) (app.MutationResult, error) {
	return s.mutate(ctx, m, func(ctx context.Context, tx pgx.Tx) (app.MutationResult, []app.AuditEvent, error) {
		q := qadmin.New(tx)
		n, err := q.RevokeSession(ctx, qadmin.RevokeSessionParams{ID: uid(session.ID), Now: stamp(m.Now)})
		if err != nil {
			return app.MutationResult{}, nil, domain.ErrUnavailable
		}
		if n != 1 {
			return app.MutationResult{}, nil, domain.ErrUnauthorized
		}
		return app.MutationResult{Status: 204}, nil, nil
	})
}

func (s *Store) RevokeAll(ctx context.Context, _ app.Session, m app.Mutation) (app.MutationResult, error) {
	return s.mutate(ctx, m, func(ctx context.Context, tx pgx.Tx) (app.MutationResult, []app.AuditEvent, error) {
		if _, err := qadmin.New(tx).RevokeAllSessions(ctx, stamp(m.Now)); err != nil {
			return app.MutationResult{}, nil, domain.ErrUnavailable
		}
		return app.MutationResult{Status: 204}, nil, nil
	})
}

// Mutate is the exported transactional mutation helper used by later admin
// surfaces (grants and roles).
func (s *Store) Mutate(ctx context.Context, m app.Mutation, fn func(context.Context, pgx.Tx) (app.MutationResult, []app.AuditEvent, error)) (app.MutationResult, error) {
	return s.mutate(ctx, m, fn)
}

// RunInTx runs one read-only composition inside a bounded transaction.
func (s *Store) RunInTx(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	if s == nil || s.Pool == nil {
		return domain.ErrUnavailable
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return domain.ErrUnavailable
	}
	defer rollback(tx)
	if err := fn(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.ErrUnavailable
	}
	return nil
}

func (s *Store) SearchAccounts(ctx context.Context, query string) ([]app.Account, error) {
	if s == nil || s.Pool == nil {
		return nil, domain.ErrUnavailable
	}
	params := qadmin.SearchAccountsParams{Query: query, Prefix: likePrefix(query)}
	if domain.ValidUUID(query) {
		params.ID = uid(query)
	}
	if accountdomain.ValidEmail(query) {
		params.AddressHash = accountdomain.AddressHash(query)
	}
	rows, err := qadmin.New(s.Pool).SearchAccounts(ctx, params)
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	out := make([]app.Account, 0, len(rows))
	for _, row := range rows {
		out = append(out, app.Account{ID: idstr(row.ID), Alias: row.Alias, Status: row.Status, CreatedAt: row.CreatedAt.Time})
	}
	return out, nil
}

func (s *Store) GetAccountTx(ctx context.Context, tx pgx.Tx, accountID string) (app.Account, error) {
	row, err := qadmin.New(tx).GetAccount(ctx, uid(accountID))
	if errors.Is(err, pgx.ErrNoRows) {
		return app.Account{}, domain.ErrNotFound
	}
	if err != nil {
		return app.Account{}, domain.ErrUnavailable
	}
	return app.Account{ID: idstr(row.ID), Alias: row.Alias, Status: row.Status, CreatedAt: row.CreatedAt.Time}, nil
}

func (s *Store) ListAudit(ctx context.Context, filter app.AuditFilter) ([]app.AuditRecord, error) {
	if s == nil || s.Pool == nil {
		return nil, domain.ErrUnavailable
	}
	limit := filter.Limit
	if limit < 1 || limit > 101 {
		limit = 20
	}
	action, target := pgtype.Text{}, pgtype.Text{}
	if filter.Action != "" {
		action = pgtype.Text{String: filter.Action, Valid: true}
	}
	if filter.TargetID != "" {
		target = pgtype.Text{String: filter.TargetID, Valid: true}
	}
	rows, err := qadmin.New(s.Pool).ListAuditEvents(ctx, qadmin.ListAuditEventsParams{
		Action: action, TargetID: target, BeforeAt: stamp(filter.BeforeAt),
		BeforeID: pgtype.Int8{Int64: filter.BeforeID, Valid: filter.BeforeID != 0}, RowLimit: int32(limit),
	})
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	out := make([]app.AuditRecord, 0, len(rows))
	for _, row := range rows {
		record := app.AuditRecord{
			ID: row.ID, At: row.At.Time, SessionID: idstr(row.SessionID),
			Action: row.Action, TargetType: row.TargetType, TargetID: row.TargetID,
			Reason: row.Reason, RequestID: row.RequestID, ClientIPHash: row.ClientIpHash,
		}
		if len(row.Before) > 0 {
			record.Before = json.RawMessage(row.Before)
		}
		if len(row.After) > 0 {
			record.After = json.RawMessage(row.After)
		}
		out = append(out, record)
	}
	return out, nil
}

func (s *Store) PaidSummaryTx(ctx context.Context, tx pgx.Tx, accountID string) (app.PaidSummary, error) {
	row, err := qadmin.New(tx).AccountPaidSummary(ctx, uid(accountID))
	if err != nil {
		return app.PaidSummary{}, domain.ErrUnavailable
	}
	summary := app.PaidSummary{State: "NONE"}
	if row.PaidActive > 0 {
		summary.State = "ACTIVE"
	}
	if row.PaidUntil.Valid {
		until := row.PaidUntil.Time
		summary.ValidUntil = &until
	}
	return summary, nil
}

func likePrefix(query string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(query) + "%"
}

// mutate reserves one idempotency key, runs fn, writes the append-only audit
// rows and stores the safe outcome in the same transaction.
func (s *Store) mutate(ctx context.Context, m app.Mutation, fn func(context.Context, pgx.Tx) (app.MutationResult, []app.AuditEvent, error)) (app.MutationResult, error) {
	if s == nil || s.Pool == nil || !domain.ValidUUID(m.SessionID) || !domain.ValidUUID(m.Key) || !domain.ValidHash(m.RequestHash) {
		return app.MutationResult{}, domain.ErrInvalid
	}
	sessionID := uid(m.SessionID)
	key := uid(m.Key)
	for {
		tx, err := s.Pool.Begin(ctx)
		if err != nil {
			return app.MutationResult{}, domain.ErrUnavailable
		}
		q := qadmin.New(tx)
		_, err = q.ReserveMutation(ctx, qadmin.ReserveMutationParams{
			SessionID: sessionID, Key: key, Action: m.Action, RequestHash: m.RequestHash,
		})
		if err == nil {
			out, events, err := fn(ctx, tx)
			if err != nil {
				rollback(tx)
				return app.MutationResult{}, err
			}
			if len(events) == 0 {
				events = []app.AuditEvent{{Action: m.Action, TargetType: m.TargetType, TargetID: m.TargetID, Reason: m.Reason}}
			}
			for _, event := range events {
				if err := q.InsertAudit(ctx, qadmin.InsertAuditParams{
					SessionID: sessionID, Action: event.Action, TargetType: event.TargetType,
					TargetID: event.TargetID, Reason: event.Reason, Before: event.Before,
					After: event.After, RequestID: m.RequestID, ClientIpHash: m.ClientIPHash,
				}); err != nil {
					rollback(tx)
					return app.MutationResult{}, domain.ErrUnavailable
				}
			}
			if err := q.CompleteMutation(ctx, qadmin.CompleteMutationParams{
				ResponseCode: pgtype.Int4{Int32: int32(out.Status), Valid: true},
				ResponseBody: out.Body, SessionID: sessionID, Key: key,
			}); err != nil {
				rollback(tx)
				return app.MutationResult{}, domain.ErrUnavailable
			}
			if err := tx.Commit(ctx); err != nil {
				return app.MutationResult{}, domain.ErrUnavailable
			}
			return out, nil
		}
		rollback(tx)
		if !errors.Is(err, pgx.ErrNoRows) {
			return app.MutationResult{}, domain.ErrUnavailable
		}

		lockTx, err := s.Pool.Begin(ctx)
		if err != nil {
			return app.MutationResult{}, domain.ErrUnavailable
		}
		stored, err := qadmin.New(lockTx).LockMutation(ctx, qadmin.LockMutationParams{SessionID: sessionID, Key: key})
		if err != nil {
			rollback(lockTx)
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			return app.MutationResult{}, domain.ErrUnavailable
		}
		rollback(lockTx)
		if !stored.ResponseCode.Valid {
			if err := qadmin.New(s.Pool).DeleteMutation(ctx, qadmin.DeleteMutationParams{SessionID: sessionID, Key: key}); err != nil {
				return app.MutationResult{}, domain.ErrUnavailable
			}
			continue
		}
		if stored.Action != m.Action || stored.RequestHash != m.RequestHash {
			return app.MutationResult{}, domain.ErrConflict
		}
		return app.MutationResult{Status: int(stored.ResponseCode.Int32), Body: stored.ResponseBody, Replayed: true}, nil
	}
}

func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}
