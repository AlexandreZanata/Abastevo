package auth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	identity "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/identity"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/profile"
)

// Transport headers carrying the proof. Single-valued: duplicates fail.
const (
	HeaderSignature = "Signature"
	HeaderCreated   = "Signature-Created"
	HeaderExpires   = "Signature-Expires"
	HeaderKeyID     = "Signature-Keyid"
	HeaderNonce     = "Signature-Nonce"
)

// MaxBodyBytes caps proof input reads; handlers enforce their own lower
// business caps on top.
const MaxBodyBytes = 1 << 20

var (
	ErrAuthMissing   = errors.New("auth: missing proof")
	ErrAuthMalformed = errors.New("auth: malformed proof")
	ErrAuthDenied    = errors.New("auth: proof denied")
	ErrAuthReplayed  = errors.New("auth: nonce already consumed")
	ErrAuthExpired   = errors.New("auth: proof expired")
)

// Identity is the server-derived caller: contributor and key resolved from
// the verified key, never from body-supplied IDs (B-BR-004).
type Identity struct {
	ContributorID string
	KeyID         string
	Fingerprint   string
}

// Verifier checks signed requests. Authority is the configured canonical
// host; the connection host and any proxy headers are ignored.
type Verifier struct {
	Pool      *pgxpool.Pool
	Authority string
	Now       func() time.Time
}

func (v Verifier) now() time.Time {
	if v.Now != nil {
		return v.Now()
	}
	return time.Now()
}

func singleValue(h http.Header, name string) (string, error) {
	values, ok := h[http.CanonicalHeaderKey(name)]
	if !ok || len(values) != 1 {
		return "", fmt.Errorf("%w: %s", ErrAuthMissing, name)
	}
	return strings.TrimSpace(values[0]), nil
}

// BaseLines rebuilds the covered base from the live request and the proof
// headers. hasBody decides whether content lines belong to the set; the
// digest covers the exact transmitted bytes.
func BaseLines(r *http.Request, authority string, created, expires, keyID, nonce string, body []byte, hasBody bool) []string {
	lines := []string{
		`"@method": ` + strings.ToUpper(r.Method),
		`"@authority": ` + authority,
		`"@path": ` + r.URL.EscapedPath(),
		`"@query": ` + r.URL.RawQuery,
	}
	if hasBody {
		sum := sha512.Sum512(body)
		lines = append(lines,
			`"content-type": `+r.Header.Get("Content-Type"),
			`"content-digest": "sha-512=:`+base64.StdEncoding.EncodeToString(sum[:])+`:"`,
		)
	}
	return append(lines,
		`"created": `+created,
		`"expires": `+expires,
		`"keyid": "`+keyID+`"`,
		`"nonce": "`+nonce+`"`,
	)
}

// Verify checks the request proof and consumes its nonce atomically,
// returning the server-derived identity.
func (v Verifier) Verify(ctx context.Context, r *http.Request) (Identity, error) {
	sig, err := singleValue(r.Header, HeaderSignature)
	if err != nil {
		return Identity{}, err
	}
	created, err := singleValue(r.Header, HeaderCreated)
	if err != nil {
		return Identity{}, err
	}
	expires, err := singleValue(r.Header, HeaderExpires)
	if err != nil {
		return Identity{}, err
	}
	keyID, err := singleValue(r.Header, HeaderKeyID)
	if err != nil {
		return Identity{}, err
	}
	nonce, err := singleValue(r.Header, HeaderNonce)
	if err != nil {
		return Identity{}, err
	}
	if v.Authority == "" {
		return Identity{}, fmt.Errorf("%w: no canonical authority configured", ErrAuthDenied)
	}
	var body []byte
	hasBody := r.Body != nil && r.ContentLength != 0
	if hasBody {
		body, err = io.ReadAll(io.LimitReader(r.Body, MaxBodyBytes+1))
		if err != nil {
			return Identity{}, fmt.Errorf("%w: body", ErrAuthMalformed)
		}
		if int64(len(body)) > MaxBodyBytes {
			return Identity{}, fmt.Errorf("%w: body too large", ErrAuthMalformed)
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
	}
	lines := BaseLines(r, v.Authority, created, expires, keyID, nonce, body, hasBody)
	q := identity.New(v.Pool)
	stored, err := q.FindKeyByFingerprint(ctx, keyID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Identity{}, ErrAuthDenied
		}
		return Identity{}, err
	}
	if stored.RevokedAt.Valid {
		return Identity{}, ErrAuthDenied
	}
	contrib, err := q.GetContributor(ctx, stored.ContributorID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Identity{}, ErrAuthDenied
		}
		return Identity{}, err
	}
	if contrib.Status != "active" {
		return Identity{}, ErrAuthDenied
	}
	var jwkX, jwkY string
	if err := parseJWK(stored.PublicJwk, &jwkX, &jwkY); err != nil {
		return Identity{}, fmt.Errorf("%w: stored key", ErrAuthDenied)
	}
	if err := profile.Verify(jwkX, jwkY, lines, sig, v.now()); err != nil {
		if err.Error() == "profile: expired" {
			return Identity{}, ErrAuthExpired
		}
		return Identity{}, fmt.Errorf("%w: %v", ErrAuthDenied, err)
	}
	nonceID, _, _ := splitNonce(nonce)
	if err := consumeNonce(ctx, v.Pool, nonceID, keyID, nonce); err != nil {
		return Identity{}, err
	}
	return Identity{
		ContributorID: uuidString(stored.ContributorID),
		KeyID:         uuidString(stored.ID),
		Fingerprint:   keyID,
	}, nil
}

func splitNonce(nonce string) (string, string, error) {
	id, cn, ok := strings.Cut(nonce, ".")
	if !ok || id == "" || cn == "" {
		return "", "", ErrAuthMalformed
	}
	return id, cn, nil
}

// consumeNonce validates the challenge binding (fingerprint, SIGN purpose,
// presented nonce hash) and marks it consumed exactly once. Zero consumed
// rows means a concurrent replay already won: deny without touching
// anything else. A failed proof never reaches this point, so its challenge
// stays usable.
func consumeNonce(ctx context.Context, pool *pgxpool.Pool, challengeID, fingerprint, nonce string) error {
	chUUID, err := parseUUID(challengeID)
	if err != nil {
		return ErrAuthMalformed
	}
	q := identity.New(pool)
	row, err := q.GetChallenge(ctx, chUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrAuthReplayed
		}
		return err
	}
	if row.ConsumedAt.Valid {
		return ErrAuthReplayed
	}
	if row.Fingerprint != fingerprint || row.Purpose != "SIGN" {
		return ErrAuthDenied
	}
	if row.NonceHash != nonceHash(nonce) {
		return ErrAuthDenied
	}
	if _, err := q.ConsumeChallenge(ctx, chUUID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrAuthReplayed
		}
		return err
	}
	return nil
}

func nonceHash(nonce string) string {
	sum := sha256.Sum256([]byte(nonce))
	return hex.EncodeToString(sum[:])
}

func parseUUID(text string) (pgtype.UUID, error) {
	raw, err := hex.DecodeString(strings.ReplaceAll(text, "-", ""))
	if err != nil || len(raw) != 16 {
		return pgtype.UUID{}, errors.New("auth: malformed UUID")
	}
	var id pgtype.UUID
	copy(id.Bytes[:], raw)
	id.Valid = true
	return id, nil
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	hexed := hex.EncodeToString(id.Bytes[:])
	return hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" +
		hexed[16:20] + "-" + hexed[20:32]
}

// parseJWK extracts coordinates from canonical public JWK JSON.
func parseJWK(raw string, x, y *string) error {
	var doc struct {
		Kty string `json:"kty"`
		Crv string `json:"crv"`
		X   string `json:"x"`
		Y   string `json:"y"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return err
	}
	if doc.Kty != "EC" || doc.Crv != "P-256" || doc.X == "" || doc.Y == "" {
		return errors.New("auth: bad stored JWK")
	}
	*x, *y = doc.X, doc.Y
	return nil
}
