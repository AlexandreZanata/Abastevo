package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	identity "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/identity"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/profile"
)

// Rotate moves a contributor from its current key to a fresh one. Both
// proofs verify first: the old proof against the server-stored key (never
// client-supplied coordinates) and the new proof against the candidate key.
// Then one transaction consumes both challenges, revokes the old key and
// binds the new one. A new fingerprint already owned elsewhere refuses with
// ErrKeyTakeover; one already owned here returns the existing binding.
// Without the old private key there is no recovery, by design.
func (r *Registrar) Rotate(ctx context.Context, req domain.RotationRequest) (domain.Rotation, error) {
	now := r.now()
	if !req.VerifiedAt.IsZero() {
		now = req.VerifiedAt
	}
	newFp, err := profile.Thumbprint(req.NewJWKX, req.NewJWKY)
	if err != nil {
		return domain.Rotation{}, fmt.Errorf("%w: new key", domain.ErrProofRequired)
	}
	old, err := r.checkRotationSide(ctx, req.Old, domain.PurposeSign, now, true)
	if err != nil {
		return domain.Rotation{}, err
	}
	newSide, err := r.checkRotationSide(ctx, req.New, domain.PurposeSign, now, false)
	if err != nil {
		return domain.Rotation{}, err
	}
	if old.challenge.Fingerprint != old.keyFp {
		return domain.Rotation{}, fmt.Errorf("%w: old side mismatch", domain.ErrProofRequired)
	}
	if newSide.challenge.Fingerprint != newFp {
		return domain.Rotation{}, fmt.Errorf("%w: new side mismatch", domain.ErrProofRequired)
	}
	oldJWKX, oldJWKY := old.jwkX, old.jwkY
	if err := profile.Verify(oldJWKX, oldJWKY, req.Old.BaseLines, req.Old.Signature, now); err != nil {
		return domain.Rotation{}, fmt.Errorf("%w: old proof: %v", domain.ErrProofRequired, err)
	}
	if err := profile.Verify(req.NewJWKX, req.NewJWKY, req.New.BaseLines, req.New.Signature, now); err != nil {
		return domain.Rotation{}, fmt.Errorf("%w: new proof: %v", domain.ErrProofRequired, err)
	}
	if old.revoked {
		// The old key already rotated away: only an idempotent replay of
		// that same rotation may pass, returning the existing binding.
		// Anything else stays denied — a revoked key rotates nothing new.
		return r.replayRotation(ctx, old, req.New.Challenge.ID, newFp)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Rotation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tq := identity.New(tx)
	if _, err := consumeTx(ctx, tq, req.Old.Challenge.ID); err != nil {
		return domain.Rotation{}, err
	}
	if _, err := consumeTx(ctx, tq, req.New.Challenge.ID); err != nil {
		return domain.Rotation{}, err
	}
	oldKeyUUID, err := mustUUID(old.keyID)
	if err != nil {
		return domain.Rotation{}, err
	}
	if _, err := tq.RevokeKey(ctx, oldKeyUUID); err != nil {
		return domain.Rotation{}, err
	}
	if existing, err := tq.FindKeyByFingerprint(ctx, newFp); err == nil {
		if uuidString(existing.ContributorID) != old.contributorID {
			return domain.Rotation{}, domain.ErrKeyTakeover
		}
		if err := tx.Commit(ctx); err != nil {
			return domain.Rotation{}, err
		}
		return domain.Rotation{
			ContributorID: old.contributorID,
			OldKeyID:      old.keyID,
			NewKeyID:      uuidString(existing.ID),
			Existed:       true,
		}, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Rotation{}, err
	}
	newText, err := newUUIDv4()
	if err != nil {
		return domain.Rotation{}, err
	}
	newUUID, err := mustUUID(newText)
	if err != nil {
		return domain.Rotation{}, err
	}
	contribUUID, err := mustUUID(old.contributorID)
	if err != nil {
		return domain.Rotation{}, err
	}
	created, err := tq.CreateKey(ctx, identity.CreateKeyParams{
		ID:            newUUID,
		ContributorID: contribUUID,
		Algorithm:     "ecdsa-p256-sha512",
		PublicJwk:     canonicalJWK(req.NewJWKX, req.NewJWKY),
		Fingerprint:   newFp,
	})
	if err != nil {
		return domain.Rotation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Rotation{}, err
	}
	return domain.Rotation{
		ContributorID: old.contributorID,
		OldKeyID:      old.keyID,
		NewKeyID:      uuidString(created.ID),
	}, nil
}

type rotationSide struct {
	challenge     domain.Challenge
	keyFp         string
	keyID         string
	contributorID string
	jwkX          string
	jwkY          string
	revoked       bool
}

// checkRotationSide validates one proof side: challenge shape and binding,
// liveness and fingerprint match. With resolve=true it also resolves the
// fingerprint to a live key (the old side); the new side skips resolution
// because its key is new by definition.
func (r *Registrar) checkRotationSide(ctx context.Context, proof domain.RotationProof, purpose string, now time.Time, resolve bool) (rotationSide, error) {
	var out rotationSide
	chID, _, err := domain.SplitNonce(proof.Challenge.Nonce)
	if err != nil {
		return out, err
	}
	if chID != proof.Challenge.ID {
		return out, fmt.Errorf("%w: nonce not bound to challenge", domain.ErrProofRequired)
	}
	if err := proof.Challenge.Validate(now); err != nil {
		return out, err
	}
	if proof.Challenge.Purpose != purpose {
		return out, fmt.Errorf("%w: wrong purpose", domain.ErrProofRequired)
	}
	q := identity.New(r.pool)
	chUUID, err := mustUUID(proof.Challenge.ID)
	if err != nil {
		return out, fmt.Errorf("%w: malformed challenge", domain.ErrProofRequired)
	}
	stored, err := q.GetChallenge(ctx, chUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return out, fmt.Errorf("%w: unknown challenge", domain.ErrProofRequired)
		}
		return out, err
	}
	if stored.ConsumedAt.Valid {
		return out, domain.ErrChallengeSpent
	}
	out = rotationSide{
		challenge: proof.Challenge,
		keyFp:     proof.Challenge.Fingerprint,
	}
	if !resolve {
		return out, nil
	}
	key, err := q.FindKeyByFingerprint(ctx, proof.Challenge.Fingerprint)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return out, fmt.Errorf("%w: unknown key", domain.ErrProofRequired)
		}
		return out, err
	}
	var doc struct {
		X string `json:"x"`
		Y string `json:"y"`
	}
	if err := json.Unmarshal([]byte(key.PublicJwk), &doc); err != nil || doc.X == "" || doc.Y == "" {
		return out, fmt.Errorf("%w: stored key", domain.ErrProofRequired)
	}
	out.jwkX, out.jwkY = doc.X, doc.Y
	out.revoked = key.RevokedAt.Valid
	out.keyID = uuidString(key.ID)
	out.contributorID = uuidString(key.ContributorID)
	return out, nil
}

// replayRotation completes an idempotent replay after the old key rotated:
// both fresh challenges consume, no key changes, and the existing binding
// returns. A new fingerprint outside this contributor stays denied.
func (r *Registrar) replayRotation(ctx context.Context, old rotationSide, newChallengeID, newFp string) (domain.Rotation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Rotation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tq := identity.New(tx)
	if _, err := consumeTx(ctx, tq, old.challenge.ID); err != nil {
		return domain.Rotation{}, err
	}
	if _, err := consumeTx(ctx, tq, newChallengeID); err != nil {
		return domain.Rotation{}, err
	}
	existing, err := tq.FindKeyByFingerprint(ctx, newFp)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Rotation{}, fmt.Errorf("%w: revoked key", domain.ErrProofRequired)
		}
		return domain.Rotation{}, err
	}
	if uuidString(existing.ContributorID) != old.contributorID {
		return domain.Rotation{}, domain.ErrKeyTakeover
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Rotation{}, err
	}
	return domain.Rotation{
		ContributorID: old.contributorID,
		OldKeyID:      old.keyID,
		NewKeyID:      uuidString(existing.ID),
		Existed:       true,
	}, nil
}

// consumeTx marks one challenge consumed, failing on replay or expiry.
func consumeTx(ctx context.Context, tq *identity.Queries, challengeID string) (pgtype.UUID, error) {
	chUUID, err := mustUUID(challengeID)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("%w: malformed challenge", domain.ErrProofRequired)
	}
	consumed, err := tq.ConsumeChallenge(ctx, chUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, domain.ErrChallengeSpent
		}
		return pgtype.UUID{}, err
	}
	return consumed.ID, nil
}
