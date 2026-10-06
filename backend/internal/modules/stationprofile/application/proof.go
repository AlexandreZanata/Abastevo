package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	profiledomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/domain"
)

// Proof is one accepted private proof with its lifecycle state.
type Proof struct {
	ID        string
	ClaimID   string
	SHA256    string
	BytesSize int64
	Format    string
	Kind      string
	Status    string
	ExpiresAt time.Time
}

// ProofStore persists proof metadata.
type ProofStore interface {
	CreateProof(ctx context.Context, id, claimID, declarationID, sha, format, kind, objectKey string, bytesSize int64, expiresAt time.Time) (ProofRow, bool, error)
	ProofByHash(ctx context.Context, claimID, sha string) (ProofRow, error)
	GetProof(ctx context.Context, id string) (ProofRow, error)
	ListExpiredProofs(ctx context.Context, limit int) ([]ProofRow, error)
	MarkProofExpired(ctx context.Context, id string) error
	MarkProofDeleted(ctx context.Context, id string) error
	SetProofStatus(ctx context.Context, id, status string) error
	BumpAttempts(ctx context.Context, declarationID string) (int64, error)
	ConsumeDeclaration(ctx context.Context, declarationID string) error
	ExpireDeclaration(ctx context.Context, declarationID string) error
}

// ProofRow is the stored proof.
type ProofRow struct {
	ID            string
	ClaimID       string
	DeclarationID string
	SHA256        string
	BytesSize     int64
	Format        string
	Kind          string
	ObjectKey     string
	Status        string
	ExpiresAt     time.Time
}

// ProofBytes stores and deletes immutable proof objects behind
// server-generated keys. Production wiring arrives with the storage
// provisioning scope; until then intake refuses (never approval
// fallback). No URLs, credentials or personal data cross this port.
type ProofBytes interface {
	Put(ctx context.Context, key string, body []byte) error
	Delete(ctx context.Context, key string) error
}

var (
	ErrProofStorage = errors.New("stationprofile: proof storage not configured")
	ErrProofClaim   = errors.New("stationprofile: claim is not open for proof")
)

// ProofPorts isolates proof intake and expiry.
type ProofPorts struct {
	Proofs ProofStore
	Claims ClaimStore
	Clock  func() time.Time
	NewID  func() (string, error)
	Bytes  ProofBytes
}

// objectKeyFor mints the server-side immutable object key: claim and
// content bound, no client input, no guessable sequence.
func objectKeyFor(claimID, sha string) string {
	sum := sha256.Sum256([]byte(claimID + "|" + sha))
	return "claims/" + claimID + "/" + hex.EncodeToString(sum[:]) + ".pdf"
}

// SubmitProof validates, dedups by content hash, stores bytes under a
// server-generated key and records hash-bound metadata. Identical
// redelivery returns the existing proof (never a second binding);
// foreign/closed claims, expired declarations, oversize/unsafe bytes
// and missing storage all fail without approval fallback.
func SubmitProof(ctx context.Context, ports ProofPorts, accountID, claimID, declarationID, filename, contentType, kind string, body []byte) (Proof, bool, error) {
	if accountID == "" || claimID == "" || declarationID == "" {
		return Proof{}, false, ErrClaimNotFound
	}
	claim, err := ownedOpenClaim(ctx, ports.Claims, accountID, claimID)
	if err != nil {
		return Proof{}, false, err
	}
	declaration, err := ports.Claims.ActiveDeclaration(ctx, claim.ID)
	if err != nil {
		return Proof{}, false, ErrClaimProof
	}
	if declaration.ID != declarationID {
		// Proof binds the single active declaration: stale or foreign
		// versions never accept bytes.
		return Proof{}, false, ErrClaimProof
	}
	if !ports.Clock().Before(declaration.ExpiresAt) {
		return Proof{}, false, ErrClaimProof
	}
	_ = claim
	retention := profiledomain.ExpiryForKind(kind)
	if retention <= 0 {
		return Proof{}, false, profiledomain.ErrProofFormat
	}
	if err := profiledomain.ValidateProofBytes(filename, contentType, body); err != nil {
		return Proof{}, false, err
	}
	if ports.Bytes == nil {
		return Proof{}, false, ErrProofStorage
	}
	sum := sha256.Sum256(body)
	sha := hex.EncodeToString(sum[:])
	existing, err := ports.Proofs.ProofByHash(ctx, claimID, sha)
	if err == nil {
		return mapProof(existing), false, nil
	}
	now := ports.Clock()
	id, err := ports.NewID()
	if err != nil {
		return Proof{}, false, err
	}
	key := objectKeyFor(claimID, sha)
	if err := ports.Bytes.Put(ctx, key, body); err != nil {
		return Proof{}, false, err
	}
	row, created, err := ports.Proofs.CreateProof(ctx, id, claimID, declarationID, sha, "application/pdf", kind, key, int64(len(body)), now.Add(retention))
	if err != nil {
		_ = ports.Bytes.Delete(ctx, key)
		return Proof{}, false, err
	}
	if !created {
		_ = ports.Bytes.Delete(ctx, key)
		existing, err := ports.Proofs.ProofByHash(ctx, claimID, sha)
		if err != nil {
			return Proof{}, false, err
		}
		return mapProof(existing), false, nil
	}
	return mapProof(row), true, nil
}

// ownedOpenClaim resolves the owner's open claim for proof binding:
// foreign owners see not-found, terminal records are closed.
func ownedOpenClaim(ctx context.Context, store ClaimStore, accountID, claimID string) (ClaimRow, error) {
	row, err := ownedClaim(ctx, store, accountID, claimID)
	if err != nil {
		return ClaimRow{}, err
	}
	if !profiledomain.ClaimOpen(row.State) {
		return ClaimRow{}, ErrClaimClosed
	}
	return row, nil
}

// PurgeExpired marks due proofs expired and deletes their bytes. Rows
// stay as audit; bytes purge physically. Restore replays this ledger
// before reads/writes (privacy-owned) so expired proof never revives.
func PurgeExpired(ctx context.Context, ports ProofPorts, limit int) (int64, error) {
	if ports.Bytes == nil {
		return 0, ErrProofStorage
	}
	rows, err := ports.Proofs.ListExpiredProofs(ctx, limit)
	if err != nil {
		return 0, err
	}
	var purged int64
	for _, row := range rows {
		if err := ports.Proofs.MarkProofExpired(ctx, row.ID); err != nil {
			return purged, err
		}
		_ = ports.Bytes.Delete(ctx, row.ObjectKey)
		purged++
	}
	return purged, nil
}

func mapProof(row ProofRow) Proof {
	return Proof{
		ID: row.ID, ClaimID: row.ClaimID, SHA256: row.SHA256,
		BytesSize: row.BytesSize, Format: row.Format, Kind: row.Kind,
		Status: row.Status, ExpiresAt: row.ExpiresAt,
	}
}
