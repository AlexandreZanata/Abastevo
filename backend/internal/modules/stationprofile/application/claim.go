package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	profiledomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/domain"
)

// Claim is one private representation request with its active
// declaration for export.
type Claim struct {
	ID            string
	AccountID     string
	StationID     string
	OperatorCNPJ  string
	Role          string
	Scopes        []string
	PolicyVersion string
	State         string
	Declaration   string
	ExpiresAt     time.Time
	Version       int
}

// ClaimStore persists claims and declarations.
type ClaimStore interface {
	CreateClaim(ctx context.Context, id, accountID, stationID, operatorCNPJ, operatorSource, role, scopes, clientKey string) (ClaimRow, bool, error)
	Claim(ctx context.Context, id string) (ClaimRow, error)
	ClaimByKey(ctx context.Context, accountID, clientKey string) (ClaimRow, error)
	ListOwnedClaims(ctx context.Context, accountID string, limit, offset int) ([]ClaimRow, error)
	CountOpenClaims(ctx context.Context, accountID string) (int64, error)
	SetClaimState(ctx context.Context, id, expected, state string) (int64, error)
	CreateDeclaration(ctx context.Context, id, claimID string, version int, nonceDigest, expectedDigest, declaration string, expiresAt time.Time) (DeclarationRow, error)
	SupersedeDeclarations(ctx context.Context, claimID string) error
	ActiveDeclaration(ctx context.Context, claimID string) (DeclarationRow, error)
	GetDeclaration(ctx context.Context, id string) (DeclarationRow, error)
}

// ClaimRow is the stored claim.
type ClaimRow struct {
	ID             string
	AccountID      string
	StationID      string
	OperatorCNPJ   string
	OperatorSource string
	Role           string
	Scopes         []string
	PolicyVersion  string
	State          string
	ClientKey      string
}

// DeclarationRow is the stored declaration version.
type DeclarationRow struct {
	ID             string
	ClaimID        string
	Version        int
	NonceDigest    string
	ExpectedDigest string
	Declaration    string
	State          string
	ExpiresAt      time.Time
}

var (
	ErrClaimAuth     = errors.New("stationprofile: account and station are required")
	ErrClaimQuota    = errors.New("stationprofile: too many open claims")
	ErrClaimConflict = errors.New("stationprofile: same key with different request")
	ErrClaimNotFound = errors.New("stationprofile: claim not found")
	ErrClaimClosed   = errors.New("stationprofile: claim is no longer open")
	ErrClaimProof    = errors.New("stationprofile: proof window expired or consumed")
	ErrClaimBusy     = errors.New("stationprofile: claim is being published, retry the same request")
)

// ClaimPorts isolates claim orchestration.
type ClaimPorts struct {
	Store      ClaimStore
	Clock      func() time.Time
	NewID      func() (string, error)
	OperatorOf func(ctx context.Context, stationID string) (cnpj, source string, found bool, err error)
}

// OpenClaim creates the private request with its first one-use
// declaration. The operator link is recorded when known; unknown
// operators leave the claim pending verification (never inferred).
// Same key + same request replays; same key + changed request
// conflicts; over-quota accounts are refused before any write. No
// grant, CNPJ insight or client flag creates authority here.
func OpenClaim(ctx context.Context, ports ClaimPorts, accountID, stationID, role string, scopes []string, clientKey string) (Claim, bool, error) {
	if accountID == "" || stationID == "" {
		return Claim{}, false, ErrClaimAuth
	}
	if _, err := validScopes(role, scopes); err != nil {
		return Claim{}, false, err
	}
	open, err := ports.Store.CountOpenClaims(ctx, accountID)
	if err != nil {
		return Claim{}, false, err
	}
	if open >= 3 {
		return Claim{}, false, ErrClaimQuota
	}
	cnpj, source, _, err := ports.OperatorOf(ctx, stationID)
	if err != nil {
		return Claim{}, false, err
	}
	id, err := ports.NewID()
	if err != nil {
		return Claim{}, false, err
	}
	row, created, err := ports.Store.CreateClaim(ctx, id, accountID, stationID, cnpj, source, role, joinScopes(scopes), clientKey)
	if err != nil {
		return Claim{}, false, err
	}
	if !created {
		existing, err := ports.Store.ClaimByKey(ctx, accountID, clientKey)
		if err != nil {
			return Claim{}, false, err
		}
		if existing.Role != role || existing.StationID != stationID || strings.Join(existing.Scopes, ",") != strings.Join(scopes, ",") {
			return Claim{}, false, ErrClaimConflict
		}
		declaration, err := ports.Store.ActiveDeclaration(ctx, existing.ID)
		if err != nil {
			if errors.Is(err, ErrClaimNotFound) {
				// The winning writer created the claim but has not
				// published its first declaration yet. Report a
				// retryable transient instead of a misleading
				// not-found: the same request is safe to replay.
				return Claim{}, false, ErrClaimBusy
			}
			return Claim{}, false, err
		}
		return mapClaim(existing, declaration), false, nil
	}
	declaration, err := issueDeclaration(ctx, ports, row.ID, accountID, stationID, cnpj, role, scopes, 1)
	if err != nil {
		return Claim{}, false, err
	}
	return mapClaim(row, declaration), true, nil
}

// ReissueDeclaration versions the challenge and supersedes prior
// active versions in one guarded step: exactly one active declaration
// ever binds proof.
func ReissueDeclaration(ctx context.Context, ports ClaimPorts, accountID, claimID string) (Claim, error) {
	row, err := ownedClaim(ctx, ports.Store, accountID, claimID)
	if err != nil {
		return Claim{}, err
	}
	if !profiledomain.ClaimOpen(row.State) {
		return Claim{}, ErrClaimClosed
	}
	current, err := ports.Store.ActiveDeclaration(ctx, row.ID)
	if err != nil {
		return Claim{}, err
	}
	if err := ports.Store.SupersedeDeclarations(ctx, row.ID); err != nil {
		return Claim{}, err
	}
	cnpj, _, _, err := ports.OperatorOf(ctx, row.StationID)
	if err != nil {
		return Claim{}, err
	}
	declaration, err := issueDeclaration(ctx, ports, row.ID, accountID, row.StationID, cnpj, row.Role, row.Scopes, current.Version+1)
	if err != nil {
		return Claim{}, err
	}
	_ = declaration
	fresh, err := ports.Store.Claim(ctx, row.ID)
	if err != nil {
		return Claim{}, err
	}
	active, err := ports.Store.ActiveDeclaration(ctx, row.ID)
	if err != nil {
		return Claim{}, err
	}
	return mapClaim(fresh, active), nil
}

// ClaimStatus returns the private request only to its owner.
func ClaimStatus(ctx context.Context, store ClaimStore, accountID, claimID string) (Claim, error) {
	row, err := ownedClaim(ctx, store, accountID, claimID)
	if err != nil {
		return Claim{}, err
	}
	declaration, err := store.ActiveDeclaration(ctx, row.ID)
	if err != nil {
		return Claim{}, err
	}
	return mapClaim(row, declaration), nil
}

// CancelClaim closes an owner's open claim. Terminal records,
// foreign owners and unknown ids fail without leaking which fired.
func CancelClaim(ctx context.Context, store ClaimStore, accountID, claimID string) error {
	row, err := store.Claim(ctx, claimID)
	if err != nil {
		return ErrClaimNotFound
	}
	if row.AccountID != accountID {
		return ErrClaimNotFound
	}
	if !profiledomain.ClaimOpen(row.State) {
		return ErrClaimClosed
	}
	affected, err := store.SetClaimState(ctx, row.ID, row.State, "cancelled")
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrClaimClosed
	}
	return nil
}

// OwnedClaimSummary is one private list row: id, state and version
// only — never another owner's data.
type OwnedClaimSummary struct {
	ID      string
	State   string
	Version int
}

// ListMine returns the owner's private requests, newest first.
func ListMine(ctx context.Context, store ClaimStore, accountID string) ([]OwnedClaimSummary, error) {
	if accountID == "" {
		return nil, ErrClaimNotFound
	}
	rows, err := store.ListOwnedClaims(ctx, accountID, 20, 0)
	if err != nil {
		return nil, err
	}
	out := make([]OwnedClaimSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, OwnedClaimSummary{ID: row.ID, State: row.State})
	}
	return out, nil
}

func ownedClaim(ctx context.Context, store ClaimStore, accountID, claimID string) (ClaimRow, error) {
	if accountID == "" || claimID == "" {
		return ClaimRow{}, ErrClaimNotFound
	}
	row, err := store.Claim(ctx, claimID)
	if err != nil {
		return ClaimRow{}, ErrClaimNotFound
	}
	if row.AccountID != accountID {
		return ClaimRow{}, ErrClaimNotFound
	}
	return row, nil
}

// issueDeclaration mints 256-bit entropy, builds the exact frozen
// declaration, stores digests (never the nonce) and returns the
// exportable row. One challenge binds one successful proof; identical
// retry is idempotent downstream, changed/reused proof fails there.
func issueDeclaration(ctx context.Context, ports ClaimPorts, claimID, accountID, stationID, cnpj, role string, scopes []string, version int) (DeclarationRow, error) {
	var entropy [32]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return DeclarationRow{}, err
	}
	nonce := hex.EncodeToString(entropy[:])
	now := ports.Clock()
	expires := now.Add(30 * time.Minute)
	declaration := buildDeclaration(claimID, accountID, stationID, cnpj, role, scopes, nonce, now, expires)
	nonceSum := sha256.Sum256([]byte(nonce))
	contentSum := sha256.Sum256([]byte(declaration))
	id, err := ports.NewID()
	if err != nil {
		return DeclarationRow{}, err
	}
	return ports.Store.CreateDeclaration(ctx, id, claimID, version,
		hex.EncodeToString(nonceSum[:]), hex.EncodeToString(contentSum[:]),
		declaration, expires)
}

// buildDeclaration renders the exact frozen 11-field template.
func buildDeclaration(claimID, accountID, stationID, cnpj, role string, scopes []string, nonce string, issued, expires time.Time) string {
	_ = nonce
	return "STATION-PROFILE-DECLARATIONv1" +
		"|station_id=" + stationID +
		"|operator_cnpj=" + cnpj +
		"|operator_revision=" + "" +
		"|claim_id=" + claimID +
		"|account_ref=" + accountID +
		"|scopes=" + strings.Join(scopes, ",") +
		"|purpose=station-representation" +
		"|policy_version=profile-v1" +
		"|challenge=" + challengeOf(claimID, nonce) +
		"|issued_at=" + issued.Format(time.RFC3339) +
		"|expires_at=" + expires.Format(time.RFC3339)
}

func challengeOf(claimID, nonce string) string {
	sum := sha256.Sum256([]byte(claimID + "|" + nonce))
	return hex.EncodeToString(sum[:])
}

func validScopes(role string, scopes []string) ([]string, error) {
	allowed, err := validRoleScopes(role)
	if err != nil {
		return nil, err
	}
	allowedSet := map[string]bool{}
	for _, scope := range allowed {
		allowedSet[scope] = true
	}
	for _, scope := range scopes {
		if !allowedSet[scope] {
			return nil, fmt.Errorf("stationprofile: scope %q not granted to %s", scope, role)
		}
	}
	if len(scopes) == 0 {
		return nil, fmt.Errorf("stationprofile: at least one scope is required")
	}
	return scopes, nil
}

func validRoleScopes(role string) ([]string, error) {
	switch role {
	case "administrator":
		return []string{"profile.edit", "reply.official", "invite.propose", "revoke.request"}, nil
	case "manager":
		return []string{"profile.edit", "reply.official"}, nil
	default:
		return nil, fmt.Errorf("stationprofile: unknown role %q", role)
	}
}

func joinScopes(scopes []string) string {
	return strings.Join(scopes, ",")
}

func mapClaim(row ClaimRow, declaration DeclarationRow) Claim {
	return Claim{
		ID: row.ID, AccountID: row.AccountID, StationID: row.StationID,
		OperatorCNPJ: row.OperatorCNPJ, Role: row.Role, Scopes: row.Scopes,
		PolicyVersion: row.PolicyVersion, State: row.State,
		Declaration: declaration.Declaration, ExpiresAt: declaration.ExpiresAt,
		Version: declaration.Version,
	}
}
