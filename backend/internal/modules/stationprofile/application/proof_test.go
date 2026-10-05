package application

import (
	"context"
	"sync"
	"testing"
	"time"
)

type fakeProofStore struct {
	mu       sync.Mutex
	proofs   map[string]ProofRow
	attempts map[string]int
}

func (f *fakeProofStore) CreateProof(_ context.Context, id, claimID, declarationID, sha, format, kind, objectKey string, bytesSize int64, expiresAt time.Time) (ProofRow, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, row := range f.proofs {
		if row.ClaimID == claimID && row.SHA256 == sha && (row.Status == "received" || row.Status == "verified") {
			return ProofRow{}, false, nil
		}
	}
	row := ProofRow{ID: id, ClaimID: claimID, DeclarationID: declarationID, SHA256: sha, BytesSize: bytesSize, Format: format, Kind: kind, ObjectKey: objectKey, Status: "received", ExpiresAt: expiresAt}
	if f.proofs == nil {
		f.proofs = map[string]ProofRow{}
	}
	f.proofs[id] = row
	return row, true, nil
}

func (f *fakeProofStore) ProofByHash(_ context.Context, claimID, sha string) (ProofRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, row := range f.proofs {
		if row.ClaimID == claimID && row.SHA256 == sha && row.Status != "rejected" && row.Status != "deleted" {
			return row, nil
		}
	}
	return ProofRow{}, ErrClaimNotFound
}

func (f *fakeProofStore) ListExpiredProofs(_ context.Context, _ int) ([]ProofRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Frozen clock advances past the 24h scan retention: scans expire,
	// multi-year authorizations do not (fail-closed retention check).
	now := time.Date(2026, 10, 6, 13, 0, 0, 0, time.UTC)
	var out []ProofRow
	for _, row := range f.proofs {
		if row.Status != "expired" && row.Status != "deleted" && !now.Before(row.ExpiresAt) {
			out = append(out, row)
		}
	}
	return out, nil
}

func (f *fakeProofStore) MarkProofExpired(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	row := f.proofs[id]
	row.Status = "expired"
	f.proofs[id] = row
	return nil
}

func (f *fakeProofStore) MarkProofDeleted(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	row := f.proofs[id]
	row.Status = "deleted"
	f.proofs[id] = row
	return nil
}

type fakeProofBytes struct {
	mu      sync.Mutex
	objects map[string][]byte
	deleted []string
}

func (f *fakeProofBytes) Put(_ context.Context, key string, body []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.objects == nil {
		f.objects = map[string][]byte{}
	}
	f.objects[key] = body
	return nil
}

type errorString string

func (e errorString) Error() string { return string(e) }

var errProofBytesGone = errorString("proof bytes gone")

func (f *fakeProofBytes) Get(_ context.Context, key string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body, ok := f.objects[key]
	if !ok {
		return nil, errProofBytesGone
	}
	return body, nil
}

func (f *fakeProofBytes) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objects, key)
	f.deleted = append(f.deleted, key)
	return nil
}

func proofTestPorts(proofs *fakeProofStore, bytes ProofBytes, claims ClaimStore) ProofPorts {
	n := 0
	return ProofPorts{
		Proofs: proofs,
		Claims: claims,
		Clock:  func() time.Time { return time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC) },
		NewID: func() (string, error) {
			n++
			return "proof-id-" + string(rune('0'+n)), nil
		},
		Bytes: bytes,
	}
}

func openProofClaim(t *testing.T, claims *fakeClaimStore) (ClaimRow, DeclarationRow) {
	t.Helper()
	claim, created, err := OpenClaim(context.Background(), claimPorts(claims), "acc-1", "station-1", "administrator", []string{"profile.edit"}, "proof-key-1")
	if err != nil || !created {
		t.Fatalf("open = %+v, %v, %v", claim, created, err)
	}
	declaration, err := claims.ActiveDeclaration(context.Background(), claim.ID)
	if err != nil {
		t.Fatalf("declaration: %v", err)
	}
	row, err := claims.Claim(context.Background(), claim.ID)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	_ = declaration
	return row, DeclarationRow{ID: "decl-1", ClaimID: row.ID, ExpiresAt: time.Date(2026, 10, 5, 12, 30, 0, 0, time.UTC)}
}

func pdfBody() []byte {
	body := append([]byte("%PDF-1.7\n"), make([]byte, 200)...)
	for i := range body[9:] {
		body[9+i] = 'a'
	}
	return body
}

func TestSubmitProofAcceptsAndDedups(t *testing.T) {
	claims := &fakeClaimStore{}
	proofs := &fakeProofStore{}
	bytes := &fakeProofBytes{}
	row, declaration := openProofClaim(t, claims)
	ports := proofTestPorts(proofs, bytes, &claimStoreAdapter{claims: claims, declaration: declaration})

	first, created, err := SubmitProof(context.Background(), ports, "acc-1", row.ID, declaration.ID, "autorizacao.pdf", "application/pdf", "authorization", pdfBody())
	if err != nil || !created {
		t.Fatalf("submit = %+v, %v, %v", first, created, err)
	}
	if first.Status != "received" || first.BytesSize != int64(len(pdfBody())) {
		t.Fatalf("proof = %+v", first)
	}
	second, created, err := SubmitProof(context.Background(), ports, "acc-1", row.ID, declaration.ID, "autorizacao.pdf", "application/pdf", "authorization", pdfBody())
	if err != nil || created || second.ID != first.ID {
		t.Fatalf("redelivery = %+v, %v, %v (must dedup, never rebind)", second, created, err)
	}
	if len(bytes.objects) != 1 {
		t.Fatalf("objects = %d, want 1", len(bytes.objects))
	}
}

// claimStoreAdapter exposes the claim fake through the ClaimStore
// interface for proof tests.
type claimStoreAdapter struct {
	claims      *fakeClaimStore
	declaration DeclarationRow
}

func (a *claimStoreAdapter) CreateClaim(ctx context.Context, id, accountID, stationID, operatorCNPJ, operatorSource, role, scopes, clientKey string) (ClaimRow, bool, error) {
	return a.claims.CreateClaim(ctx, id, accountID, stationID, operatorCNPJ, operatorSource, role, scopes, clientKey)
}

func (a *claimStoreAdapter) Claim(ctx context.Context, id string) (ClaimRow, error) {
	return a.claims.Claim(ctx, id)
}

func (a *claimStoreAdapter) ClaimByKey(ctx context.Context, accountID, clientKey string) (ClaimRow, error) {
	return a.claims.ClaimByKey(ctx, accountID, clientKey)
}

func (a *claimStoreAdapter) ListOwnedClaims(ctx context.Context, accountID string, limit, offset int) ([]ClaimRow, error) {
	return nil, nil
}

func (a *claimStoreAdapter) CountOpenClaims(ctx context.Context, accountID string) (int64, error) {
	return 0, nil
}

func (a *claimStoreAdapter) SetClaimState(ctx context.Context, id, expected, state string) (int64, error) {
	return a.claims.SetClaimState(ctx, id, expected, state)
}

func (a *claimStoreAdapter) CreateDeclaration(ctx context.Context, id, claimID string, version int, nonceDigest, expectedDigest, declaration string, expiresAt time.Time) (DeclarationRow, error) {
	return a.claims.CreateDeclaration(ctx, id, claimID, version, nonceDigest, expectedDigest, declaration, expiresAt)
}

func (a *claimStoreAdapter) SupersedeDeclarations(ctx context.Context, claimID string) error {
	return a.claims.SupersedeDeclarations(ctx, claimID)
}

func (a *claimStoreAdapter) LatestDeclaration(ctx context.Context, claimID string) (DeclarationRow, error) {
	return a.declaration, nil
}

func (a *claimStoreAdapter) ActiveDeclaration(ctx context.Context, claimID string) (DeclarationRow, error) {
	return a.declaration, nil
}

func (a *claimStoreAdapter) GetDeclaration(ctx context.Context, id string) (DeclarationRow, error) {
	if a.declaration.ID == id {
		return a.declaration, nil
	}
	return DeclarationRow{}, ErrClaimNotFound
}

func TestSubmitProofRefusesUnsafeAndForeign(t *testing.T) {
	claims := &fakeClaimStore{}
	proofs := &fakeProofStore{}
	bytes := &fakeProofBytes{}
	row, declaration := openProofClaim(t, claims)
	ports := proofTestPorts(proofs, bytes, &claimStoreAdapter{claims: claims, declaration: declaration})

	if _, _, err := SubmitProof(context.Background(), ports, "acc-2", row.ID, declaration.ID, "a.pdf", "application/pdf", "authorization", pdfBody()); err == nil {
		t.Fatal("foreign claim must fail")
	}
	if _, _, err := SubmitProof(context.Background(), ports, "acc-1", row.ID, declaration.ID, "a.pdf", "application/pdf", "mystery", pdfBody()); err == nil {
		t.Fatal("unknown kind must fail closed")
	}
	if _, _, err := SubmitProof(context.Background(), ports, "acc-1", row.ID, declaration.ID, "a.pdf", "application/pdf", "authorization", []byte("hello")); err == nil {
		t.Fatal("non-PDF must fail")
	}
	if _, _, err := SubmitProof(context.Background(), ports, "acc-1", row.ID, "decl-other", "a.pdf", "application/pdf", "authorization", pdfBody()); err == nil {
		t.Fatal("stale declaration must fail")
	}
}

func TestSubmitProofWithoutStorageRefuses(t *testing.T) {
	claims := &fakeClaimStore{}
	row, declaration := openProofClaim(t, claims)
	ports := proofTestPorts(&fakeProofStore{}, nil, &claimStoreAdapter{claims: claims, declaration: declaration})
	if _, _, err := SubmitProof(context.Background(), ports, "acc-1", row.ID, declaration.ID, "a.pdf", "application/pdf", "authorization", pdfBody()); err == nil {
		t.Fatal("missing storage must block intake, never approve")
	}
}

func TestPurgeExpiredDeletesBytesKeepsRows(t *testing.T) {
	claims := &fakeClaimStore{}
	proofs := &fakeProofStore{}
	bytes := &fakeProofBytes{}
	row, declaration := openProofClaim(t, claims)
	ports := proofTestPorts(proofs, bytes, &claimStoreAdapter{claims: claims, declaration: declaration})

	scan := append([]byte("%PDF-1.7\n"), make([]byte, 100)...)
	for i := range scan[9:] {
		scan[9+i] = 'b'
	}
	submitted, _, err := SubmitProof(context.Background(), ports, "acc-1", row.ID, declaration.ID, "scan.pdf", "application/pdf", "scan", scan)
	if err != nil {
		t.Fatalf("submit scan: %v", err)
	}
	// Scans expire 24h after the frozen clock: purge must delete bytes
	// and mark the row while keeping it as audit.
	purged, err := PurgeExpired(context.Background(), ports, 100)
	if err != nil || purged != 1 {
		t.Fatalf("purged = %d, err = %v", purged, err)
	}
	if len(bytes.objects) != 0 || len(bytes.deleted) != 1 {
		t.Fatalf("objects = %d, deleted = %v", len(bytes.objects), bytes.deleted)
	}
	if proofs.proofs[submitted.ID].Status != "expired" {
		t.Fatal("row must stay as expired audit")
	}
}

func (f *fakeProofStore) GetProof(_ context.Context, id string) (ProofRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.proofs[id]
	if !ok {
		return ProofRow{}, ErrClaimNotFound
	}
	return row, nil
}

func (f *fakeProofStore) SetProofStatus(_ context.Context, id, status string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.proofs[id]
	if !ok {
		return ErrClaimNotFound
	}
	row.Status = status
	f.proofs[id] = row
	return nil
}

func (f *fakeProofStore) BumpAttempts(_ context.Context, declarationID string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.attempts == nil {
		f.attempts = map[string]int{}
	}
	f.attempts[declarationID]++
	return int64(f.attempts[declarationID]), nil
}

func (f *fakeProofStore) ConsumeDeclaration(_ context.Context, declarationID string) error {
	return nil
}

func (f *fakeProofStore) ExpireDeclaration(_ context.Context, declarationID string) error {
	return nil
}
