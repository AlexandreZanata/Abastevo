package adapters

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	identity "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/identity"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/profile"
)

// Registrar implements domain.Registrar.
type Registrar struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

// NewRegistrar wires the owned generated queries to a pool.
func NewRegistrar(pool *pgxpool.Pool) *Registrar {
	return &Registrar{pool: pool, now: time.Now}
}

func newUUIDv4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	hexed := hex.EncodeToString(b[:])
	return hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" +
		hexed[16:20] + "-" + hexed[20:32], nil
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

func nonceHash(nonce string) string {
	sum := sha256.Sum256([]byte(nonce))
	return hex.EncodeToString(sum[:])
}

// IssueChallenge mints a bound proof opportunity for one fingerprint and
// purpose. The full nonce is returned once; only its hash is stored.
func (r *Registrar) IssueChallenge(ctx context.Context, fingerprint, purpose string) (domain.Challenge, error) {
	fp, err := domain.ParseFingerprint(fingerprint)
	if err != nil {
		return domain.Challenge{}, err
	}
	purpose, err = domain.ParsePurpose(purpose)
	if err != nil {
		return domain.Challenge{}, err
	}
	id, err := newUUIDv4()
	if err != nil {
		return domain.Challenge{}, err
	}
	var randPart [16]byte
	if _, err := rand.Read(randPart[:]); err != nil {
		return domain.Challenge{}, err
	}
	nonce := id + "." + hex.EncodeToString(randPart[:])
	uid, err := mustUUID(id)
	if err != nil {
		return domain.Challenge{}, err
	}
	expires := r.now().Add(domain.ChallengeTTL)
	if _, err := identity.New(r.pool).CreateChallenge(ctx, identity.CreateChallengeParams{
		ID:          uid,
		NonceHash:   nonceHash(nonce),
		Fingerprint: fp,
		Purpose:     purpose,
		ExpiresAt:   pgtype.Timestamptz{Time: expires, Valid: true},
	}); err != nil {
		return domain.Challenge{}, err
	}
	return domain.Challenge{
		ID: id, Nonce: nonce, Fingerprint: fp, Purpose: purpose, ExpiresAt: expires,
	}, nil
}

// Register verifies the key proof, then consumes the challenge and creates
// the contributor plus key atomically. A valid proof for a known fingerprint
// returns the existing identity (Existed); failures consume nothing.
func (r *Registrar) Register(ctx context.Context, req domain.RegistrationRequest) (domain.Registration, error) {
	fp, err := profile.Thumbprint(req.JWKX, req.JWKY)
	if err != nil {
		return domain.Registration{}, fmt.Errorf("%w: bad key", domain.ErrProofRequired)
	}
	chID, _, err := domain.SplitNonce(req.Challenge.Nonce)
	if err != nil {
		return domain.Registration{}, err
	}
	if chID != req.Challenge.ID {
		return domain.Registration{}, fmt.Errorf("%w: nonce not bound to challenge", domain.ErrProofRequired)
	}
	if err := req.Challenge.Validate(r.now()); err != nil {
		return domain.Registration{}, err
	}
	if req.Challenge.Purpose != domain.PurposeRegister {
		return domain.Registration{}, fmt.Errorf("%w: wrong purpose", domain.ErrProofRequired)
	}
	challengeUUID, err := mustUUID(req.Challenge.ID)
	if err != nil {
		return domain.Registration{}, fmt.Errorf("%w: malformed challenge", domain.ErrProofRequired)
	}
	stored, err := identity.New(r.pool).GetChallenge(ctx, challengeUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Registration{}, fmt.Errorf("%w: unknown challenge", domain.ErrProofRequired)
		}
		return domain.Registration{}, err
	}
	if stored.ConsumedAt.Valid {
		return domain.Registration{}, domain.ErrChallengeSpent
	}
	if !r.now().Before(stored.ExpiresAt.Time) {
		return domain.Registration{}, domain.ErrChallengeExpired
	}
	if stored.Fingerprint != fp || stored.Fingerprint != req.Challenge.Fingerprint {
		return domain.Registration{}, fmt.Errorf("%w: fingerprint mismatch", domain.ErrProofRequired)
	}
	if stored.Purpose != domain.PurposeRegister || !proofBinds(req.BaseLines, fp, req.Challenge.Nonce) {
		return domain.Registration{}, domain.ErrProofRequired
	}
	if stored.NonceHash != nonceHash(req.Challenge.Nonce) {
		return domain.Registration{}, fmt.Errorf("%w: nonce mismatch", domain.ErrProofRequired)
	}
	if err := profile.Verify(req.JWKX, req.JWKY, req.BaseLines, req.Signature, r.now()); err != nil {
		return domain.Registration{}, fmt.Errorf("%w: %v", domain.ErrProofRequired, err)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Registration{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tq := identity.New(tx)
	consumed, err := tq.ConsumeChallenge(ctx, challengeUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Registration{}, domain.ErrChallengeSpent
		}
		return domain.Registration{}, err
	}
	_ = consumed
	if existing, err := tq.FindKeyByFingerprint(ctx, fp); err == nil {
		if err := tx.Commit(ctx); err != nil {
			return domain.Registration{}, err
		}
		return domain.Registration{
			ContributorID: uuidString(existing.ContributorID),
			KeyID:         uuidString(existing.ID),
			Existed:       true,
		}, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Registration{}, err
	}
	contributorText, err := newUUIDv4()
	if err != nil {
		return domain.Registration{}, err
	}
	contributorID, err := mustUUID(contributorText)
	if err != nil {
		return domain.Registration{}, err
	}
	if _, err := tq.CreateContributor(ctx, contributorID); err != nil {
		return domain.Registration{}, err
	}
	keyText, err := newUUIDv4()
	if err != nil {
		return domain.Registration{}, err
	}
	keyID, err := mustUUID(keyText)
	if err != nil {
		return domain.Registration{}, err
	}
	created, err := tq.CreateKey(ctx, identity.CreateKeyParams{
		ID:            keyID,
		ContributorID: contributorID,
		Algorithm:     "ecdsa-p256-sha512",
		PublicJwk:     canonicalJWK(req.JWKX, req.JWKY),
		Fingerprint:   fp,
	})
	if err != nil {
		// Lost the fingerprint race after a valid proof: ON CONFLICT DO
		// NOTHING yields no row instead of a unique violation. Roll back
		// the orphan contributor and resolve to the winner instead of
		// forking identity. The challenge stays unconsumed, so a plain
		// retry with a fresh challenge also converges.
		if errors.Is(err, pgx.ErrNoRows) {
			_ = tx.Rollback(ctx)
			if existing, rerr := identity.New(r.pool).FindKeyByFingerprint(ctx, fp); rerr == nil {
				return domain.Registration{
					ContributorID: uuidString(existing.ContributorID),
					KeyID:         uuidString(existing.ID),
					Existed:       true,
				}, nil
			}
		}
		return domain.Registration{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Registration{}, err
	}
	return domain.Registration{
		ContributorID: uuidString(created.ContributorID),
		KeyID:         uuidString(created.ID),
	}, nil
}

// canonicalJWK stores the public key in thumbprint order.
func canonicalJWK(x, y string) string {
	return `{"crv":"P-256","kty":"EC","x":"` + x + `","y":"` + y + `"}`
}

// AttributionToken resolves a contributor to its stable random token,
// minting it once on first use. Observations carry the token, never the
// contributor UUID, so public content cannot pivot to identity rows.
func (r *Registrar) AttributionToken(ctx context.Context, contributorID string) (string, error) {
	uid, err := mustUUID(contributorID)
	if err != nil {
		return "", err
	}
	q := identity.New(r.pool)
	token, err := q.GetAttributionToken(ctx, uid)
	if err != nil {
		return "", err
	}
	if token.String != "" {
		return token.String, nil
	}
	var randPart [16]byte
	if _, err := rand.Read(randPart[:]); err != nil {
		return "", err
	}
	fresh := "tok-" + hex.EncodeToString(randPart[:])
	n, err := q.SetAttributionToken(ctx, identity.SetAttributionTokenParams{ID: uid, Token: pgtype.Text{String: fresh, Valid: true}})
	if err != nil {
		return "", err
	}
	if n == 0 {
		after, rerr := q.GetAttributionToken(ctx, uid)
		if rerr != nil {
			return "", rerr
		}
		return after.String, nil
	}
	return fresh, nil
}

// ContributorProfile is the owner identity section for export
// (P07-T03): status and dates only, never keys, tokens or challenges.
type ContributorProfile struct {
	ContributorID string
	Status        string
	CreatedAt     time.Time
	Deleted       bool
}

// Profile resolves one contributor's export-safe identity section.
func (r *Registrar) Profile(ctx context.Context, contributorID string) (ContributorProfile, error) {
	uid, err := mustUUID(contributorID)
	if err != nil {
		return ContributorProfile{}, err
	}
	row, err := identity.New(r.pool).GetContributor(ctx, uid)
	if err != nil {
		return ContributorProfile{}, err
	}
	return ContributorProfile{
		ContributorID: uuidString(row.ID), Status: row.Status,
		CreatedAt: row.CreatedAt.Time, Deleted: row.DeletedAt.Valid,
	}, nil
}

// DeleteContributor revokes one owner's writes and unlinks identity in
// the narrow erasure workflow (P07-T04, B-BR-016): status moves to
// deleted, the attribution token is cleared so owner rows no longer
// resolve, and every live key is revoked. Re-running on an already
// deleted contributor converges (deleted=false, keys 0).
func (r *Registrar) DeleteContributor(ctx context.Context, contributorID string) (deleted bool, keysRevoked int64, err error) {
	uid, err := mustUUID(contributorID)
	if err != nil {
		return false, 0, err
	}
	q := identity.New(r.pool)
	n, err := q.DeleteContributor(ctx, uid)
	if err != nil {
		return false, 0, err
	}
	keys, err := q.RevokeContributorKeys(ctx, uid)
	if err != nil {
		return false, 0, err
	}
	return n == 1, keys, nil
}

// PurgeExpiredChallenges deletes expired challenges in bounded batches
// (P07-T05 retention): each pass removes at most batch rows, oldest
// first, and reports the total plus the oldest overdue expiry observed
// before purging (zero time when nothing was overdue).
func (r *Registrar) PurgeExpiredChallenges(ctx context.Context, now time.Time, batch int32) (purged int64, oldest time.Time, err error) {
	q := identity.New(r.pool)
	ts, oerr := q.OldestExpiredChallenge(ctx, pgtype.Timestamptz{Time: now, Valid: true})
	if oerr != nil && !errors.Is(oerr, pgx.ErrNoRows) {
		return 0, time.Time{}, oerr
	}
	oldest = ts.Time
	for {
		n, derr := q.PurgeExpiredChallenges(ctx, identity.PurgeExpiredChallengesParams{
			Now: pgtype.Timestamptz{Time: now, Valid: true}, Batch: batch,
		})
		if derr != nil {
			return purged, oldest, derr
		}
		purged += n
		if n < int64(batch) {
			return purged, oldest, nil
		}
	}
}

// Challenge resolves server-owned metadata, rejecting substituted nonces.
func (r *Registrar) Challenge(ctx context.Context, id, nonce string) (domain.Challenge, error) {
	uid, err := mustUUID(id)
	if err != nil {
		return domain.Challenge{}, domain.ErrProofRequired
	}
	nonceID, _, err := domain.SplitNonce(nonce)
	if err != nil || nonceID != id {
		return domain.Challenge{}, domain.ErrProofRequired
	}
	row, err := identity.New(r.pool).GetChallenge(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Challenge{}, domain.ErrProofRequired
	}
	if err != nil {
		return domain.Challenge{}, err
	}
	if row.NonceHash != nonceHash(nonce) {
		return domain.Challenge{}, domain.ErrProofRequired
	}
	if row.ConsumedAt.Valid {
		return domain.Challenge{}, domain.ErrChallengeSpent
	}
	if !r.now().Before(row.ExpiresAt.Time) {
		return domain.Challenge{}, domain.ErrChallengeExpired
	}
	return domain.Challenge{ID: id, Nonce: nonce, Fingerprint: row.Fingerprint, Purpose: row.Purpose, ExpiresAt: row.ExpiresAt.Time}, nil
}

// Proof metadata must name the exact server-owned challenge being consumed.
func proofBinds(lines []string, fingerprint, nonce string) bool {
	return len(lines) >= 2 && lines[len(lines)-2] == `"keyid": "`+fingerprint+`"` && lines[len(lines)-1] == `"nonce": "`+nonce+`"`
}
