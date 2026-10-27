package adapters

import (
	"context"
	"encoding/hex"
	"errors"
	qsub "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/subscription"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"sort"
	"time"
)

type Store struct{ Pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{Pool: pool} }
func uid(s string) pgtype.UUID           { var id pgtype.UUID; _ = id.Scan(s); return id }
func idstr(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	s := hex.EncodeToString(id.Bytes[:])
	return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:]
}
func digestOK(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
func stamp(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t.UTC(), Valid: !t.IsZero()}
}
func (s *Store) authorize(ctx context.Context, q *qsub.Queries, p app.Principal, interactive, signing bool) error {
	if !domain.ValidID(p.AccountID) {
		return domain.ErrUnauthorized
	}
	if _, err := q.LockOwner(ctx, uid(p.AccountID)); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrUnauthorized
		}
		return domain.ErrUnavailable
	}
	if !interactive {
		return nil
	}
	if !domain.ValidID(p.FamilyID) {
		return domain.ErrUnauthorized
	}
	if _, err := q.LockSession(ctx, qsub.LockSessionParams{FamilyID: uid(p.FamilyID), AccountID: uid(p.AccountID)}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrUnauthorized
		}
		return domain.ErrUnavailable
	}
	if signing {
		if !domain.ValidID(p.ContributorID) || !domain.ValidID(p.KeyID) {
			return domain.ErrUnauthorized
		}
		if _, err := q.LockBinding(ctx, qsub.LockBindingParams{AccountID: uid(p.AccountID), ContributorID: p.ContributorID}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrUnauthorized
			}
			return domain.ErrUnavailable
		}
		if _, err := q.LockSigningKey(ctx, qsub.LockSigningKeyParams{KeyID: uid(p.KeyID), ContributorID: uid(p.ContributorID)}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrUnauthorized
			}
			return domain.ErrUnavailable
		}
	}
	return nil
}
func (s *Store) Begin(ctx context.Context, a app.Attempt) (app.Attempt, error) {
	return s.begin(ctx, a, false)
}
func (s *Store) begin(ctx context.Context, a app.Attempt, background bool) (app.Attempt, error) {
	if s == nil || s.Pool == nil {
		return app.Attempt{}, domain.ErrUnavailable
	}
	if !domain.ValidID(a.OperationID) || !digestOK(a.Digest) || !digestOK(a.PayloadHash) || len(a.CipherToken) == 0 || len(a.CipherToken) > 4200 {
		return app.Attempt{}, domain.ErrInvalid
	}
	if a.Environment != "test" && a.Environment != "production" {
		return app.Attempt{}, domain.ErrInvalid
	}
	if a.PackageName == "" || len(a.PackageName) > 150 || (!background && a.Kind != "verify" && a.Kind != "restore") {
		return app.Attempt{}, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return app.Attempt{}, domain.ErrUnavailable
	}
	defer rollback(tx)
	q := qsub.New(tx)
	if err = s.authorize(ctx, q, a.Principal, !background, !background); err != nil {
		return app.Attempt{}, err
	}
	old, err := q.GetOperation(ctx, qsub.GetOperationParams{AccountID: uid(a.Principal.AccountID), OperationID: uid(a.OperationID)})
	if err == nil {
		kind := a.Kind
		if background {
			kind = "refresh"
		}
		if old.PayloadHash != a.PayloadHash || old.TokenDigest != a.Digest || old.Kind != kind {
			return app.Attempt{}, domain.ErrConflict
		}
		if old.Status != "pending" {
			a.Replay = true
			a.Result = old.Status
			return a, tx.Commit(ctx)
		}
		if old.Attempts >= 5 {
			return app.Attempt{}, domain.ErrQuota
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return app.Attempt{}, domain.ErrUnavailable
	}
	if errors.Is(err, pgx.ErrNoRows) {
		count, e := q.HourlyAdmissions(ctx, uid(a.Principal.AccountID))
		if e != nil {
			return app.Attempt{}, domain.ErrUnavailable
		}
		if count >= 20 {
			return app.Attempt{}, domain.ErrQuota
		}
	}
	pending, err := q.PendingAdmissions(ctx, uid(a.Principal.AccountID))
	if err != nil {
		return app.Attempt{}, domain.ErrUnavailable
	}
	if pending >= 2 {
		return app.Attempt{}, domain.ErrQuota
	}
	if err = q.LockToken(ctx, a.Digest); err != nil {
		return app.Attempt{}, domain.ErrUnavailable
	}
	if owner, e := q.GetRetiredToken(ctx, a.Digest); e == nil {
		_ = owner
		return app.Attempt{}, domain.ErrDenied
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return app.Attempt{}, domain.ErrUnavailable
	}
	if purchase, e := q.GetPurchase(ctx, a.Digest); e == nil {
		if idstr(purchase.AccountID) != a.Principal.AccountID || purchase.Retired || purchase.State == string(domain.Revoked) {
			return app.Attempt{}, domain.ErrDenied
		}
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return app.Attempt{}, domain.ErrUnavailable
	}
	gen, err := q.ClaimSlot(ctx, qsub.ClaimSlotParams{TokenDigest: a.Digest, AccountID: uid(a.Principal.AccountID), OperationID: uid(a.OperationID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return app.Attempt{}, domain.ErrBusy
	}
	if err != nil {
		return app.Attempt{}, domain.ErrUnavailable
	}
	a.Generation = gen
	a.Background = background
	kind := a.Kind
	if background {
		kind = "refresh"
	}
	if err = q.StartOperation(ctx, qsub.StartOperationParams{AccountID: uid(a.Principal.AccountID), OperationID: uid(a.OperationID), Kind: kind, TokenDigest: a.Digest, PayloadHash: a.PayloadHash, TokenCipher: a.CipherToken, Generation: gen}); err != nil {
		return app.Attempt{}, domain.ErrUnavailable
	}
	if err = tx.Commit(ctx); err != nil {
		return app.Attempt{}, domain.ErrUnavailable
	}
	return a, nil
}
func (s *Store) transaction(ctx context.Context, a app.Attempt, linked string) (pgx.Tx, *qsub.Queries, error) {
	ctxDeadline, ok := ctx.Deadline()
	if !ok || time.Until(ctxDeadline) > 5*time.Second {
		return nil, nil, domain.ErrInvalid
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, nil, domain.ErrUnavailable
	}
	fail := func(err error) (pgx.Tx, *qsub.Queries, error) {
		rollback(tx)
		return nil, nil, err
	}
	q := qsub.New(tx)
	if err = s.authorize(ctx, q, a.Principal, !a.Background, !a.Background); err != nil {
		return fail(err)
	}
	digests := []string{a.Digest}
	if linked != "" {
		digests = append(digests, linked)
	}
	sort.Strings(digests)
	for _, d := range digests {
		if err = q.LockToken(ctx, d); err != nil {
			return fail(domain.ErrUnavailable)
		}
	}
	slot, err := q.GetSlot(ctx, a.Digest)
	if err != nil || slot.Generation != a.Generation || idstr(slot.AccountID) != a.Principal.AccountID || idstr(slot.OperationID) != a.OperationID || !slot.LeaseLive || time.Until(slot.LeaseUntil.Time) < 5*time.Second {
		return fail(domain.ErrStale)
	}
	op, err := q.GetOperation(ctx, qsub.GetOperationParams{AccountID: uid(a.Principal.AccountID), OperationID: uid(a.OperationID)})
	if err != nil || op.Generation != a.Generation || op.Status != "pending" || op.PayloadHash != a.PayloadHash {
		return fail(domain.ErrStale)
	}
	return tx, q, nil
}
func (s *Store) Commit(ctx context.Context, a app.Attempt, v app.VerifiedPurchase) error {
	if s == nil || s.Pool == nil {
		return domain.ErrUnavailable
	}
	if !digestOK(a.Digest) || !domain.ValidID(a.OperationID) || !domain.ValidState(v.Snapshot.State) || v.Snapshot.State == domain.None || v.Snapshot.ProductID == "" || v.Snapshot.BasePlanID == "" || len(v.Snapshot.ProductID) > 150 || len(v.Snapshot.BasePlanID) > 150 {
		return domain.ErrInvalid
	}
	if v.LinkedDigest != "" && (!digestOK(v.LinkedDigest) || v.LinkedDigest == a.Digest) {
		return domain.ErrDenied
	}
	now := time.Now()
	if v.Snapshot.VerifiedAt.IsZero() || now.Before(v.Snapshot.VerifiedAt) || now.Sub(v.Snapshot.VerifiedAt) >= domain.ProofFreshness {
		return domain.ErrStale
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, q, err := s.transaction(ctx, a, v.LinkedDigest)
	if err != nil {
		return err
	}
	defer rollback(tx)
	if _, e := q.GetRetiredToken(ctx, a.Digest); e == nil {
		return domain.ErrDenied
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return domain.ErrUnavailable
	}
	var linked pgtype.Text
	if v.LinkedDigest != "" {
		if old, e := q.GetPurchase(ctx, v.LinkedDigest); e == nil && idstr(old.AccountID) != a.Principal.AccountID {
			return domain.ErrDenied
		} else if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return domain.ErrUnavailable
		}
		if (v.Snapshot.State == domain.Active || v.Snapshot.State == domain.Canceled || v.Snapshot.State == domain.Grace) && now.Before(v.Snapshot.ValidUntil) {
			if _, err = q.RetireToken(ctx, qsub.RetireTokenParams{TokenDigest: v.LinkedDigest, AccountID: uid(a.Principal.AccountID)}); err != nil {
				return domain.ErrDenied
			}
			if err = q.RetirePurchase(ctx, qsub.RetirePurchaseParams{TokenDigest: v.LinkedDigest, AccountID: uid(a.Principal.AccountID)}); err != nil {
				return domain.ErrUnavailable
			}
		}

		linked = pgtype.Text{String: v.LinkedDigest, Valid: true}
	}
	cipherToken := a.CipherToken
	if v.Snapshot.State == domain.Revoked {
		cipherToken = nil
	}
	if _, err = q.SavePurchase(ctx, qsub.SavePurchaseParams{TokenDigest: a.Digest, AccountID: uid(a.Principal.AccountID), Environment: a.Environment, PackageName: a.PackageName, ProductID: v.Snapshot.ProductID, BasePlanID: v.Snapshot.BasePlanID, State: string(v.Snapshot.State), ValidUntil: stamp(v.Snapshot.ValidUntil), VerifiedAt: stamp(v.Snapshot.VerifiedAt), TokenCipher: cipherToken, Acknowledged: v.Snapshot.Acknowledged, LinkedDigest: linked}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrDenied
		}
		return domain.ErrUnavailable
	}
	// Benefit and acknowledgement intent commit together; payload is digest-only.
	if !v.Snapshot.Acknowledged && (v.Snapshot.State == domain.Active || v.Snapshot.State == domain.Canceled || v.Snapshot.State == domain.Grace) {
		payload := []byte(`{"version":1,"digest":"` + a.Digest + `"}`)
		if _, err = jobs.Enqueue(ctx, tx, "subscription-acknowledge", payload, "subscription-ack:"+a.Digest, 5, time.Time{}); err != nil {
			return domain.ErrUnavailable
		}
	}
	n, err := q.FinishOperation(ctx, qsub.FinishOperationParams{Status: "accepted", AccountID: uid(a.Principal.AccountID), OperationID: uid(a.OperationID), Generation: a.Generation})
	if err != nil || n != 1 {
		return domain.ErrStale
	}
	if err = q.ReleaseSlot(ctx, qsub.ReleaseSlotParams{TokenDigest: a.Digest, Generation: a.Generation}); err != nil {
		return domain.ErrUnavailable
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.ErrUnavailable
	}
	return nil
}
func (s *Store) Reject(ctx context.Context, a app.Attempt) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, q, err := s.transaction(ctx, a, "")
	if err != nil {
		return err
	}
	defer rollback(tx)
	if _, err = q.FinishOperation(ctx, qsub.FinishOperationParams{Status: "denied", AccountID: uid(a.Principal.AccountID), OperationID: uid(a.OperationID), Generation: a.Generation}); err != nil {
		return domain.ErrUnavailable
	}
	if err = q.ReleaseSlot(ctx, qsub.ReleaseSlotParams{TokenDigest: a.Digest, Generation: a.Generation}); err != nil {
		return domain.ErrUnavailable
	}
	return tx.Commit(ctx)
}
func snapshot(p qsub.SubscriptionPurchase) *domain.Snapshot {
	s := &domain.Snapshot{State: domain.State(p.State), VerifiedAt: p.VerifiedAt.Time, ProductID: p.ProductID, BasePlanID: p.BasePlanID, Retired: p.Retired, Acknowledged: p.Acknowledged}
	if p.ValidUntil.Valid {
		s.ValidUntil = p.ValidUntil.Time
	}
	if p.RefreshRequired {
		s.VerifiedAt = time.Time{}
	}
	return s
}
func (s *Store) Status(ctx context.Context, p app.Principal) (*domain.Snapshot, error) {
	if s == nil || s.Pool == nil {
		return nil, domain.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	defer rollback(tx)
	q := qsub.New(tx)
	if err = s.authorize(ctx, q, p, true, false); err != nil {
		return nil, err
	}
	row, err := q.ReadStatus(ctx, uid(p.AccountID))
	var result *domain.Snapshot
	if err == nil {
		result = snapshot(row)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrUnavailable
	}
	now := time.Now()
	webCapabilities, webValidUntil, err := webGrants(ctx, tx, p.AccountID, now)
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	adminCapabilities, adminValidUntil, adminTiers, err := adminGrants(ctx, tx, p.AccountID, now)
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	if result == nil && len(webCapabilities) == 0 && len(adminCapabilities) == 0 {
		return nil, nil
	}
	if result == nil {
		result = &domain.Snapshot{}
	}
	result.WebCapabilities = webCapabilities
	result.WebValidUntil = webValidUntil
	result.AdminCapabilities = adminCapabilities
	result.AdminValidUntil = adminValidUntil
	result.Tiers = adminTiers
	if err = tx.Commit(ctx); err != nil {
		return nil, domain.ErrUnavailable
	}
	return result, nil
}

var _ app.Store = (*Store)(nil)

func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}
