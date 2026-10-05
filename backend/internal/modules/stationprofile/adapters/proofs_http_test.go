package adapters

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/application"
)

type fakeProofStore struct {
	mu     sync.Mutex
	proofs map[string]application.ProofRow
}

func (f *fakeProofStore) CreateProof(_ context.Context, id, claimID, declarationID, sha, format, kind, objectKey string, bytesSize int64, expiresAt time.Time) (application.ProofRow, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, row := range f.proofs {
		if row.ClaimID == claimID && row.SHA256 == sha && row.Status != "rejected" && row.Status != "deleted" {
			return application.ProofRow{}, false, nil
		}
	}
	row := application.ProofRow{ID: id, ClaimID: claimID, SHA256: sha, BytesSize: bytesSize, Format: format, Kind: kind, ObjectKey: objectKey, Status: "received", ExpiresAt: expiresAt}
	if f.proofs == nil {
		f.proofs = map[string]application.ProofRow{}
	}
	f.proofs[id] = row
	return row, true, nil
}

func (f *fakeProofStore) ProofByHash(_ context.Context, claimID, sha string) (application.ProofRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, row := range f.proofs {
		if row.ClaimID == claimID && row.SHA256 == sha && row.Status != "rejected" && row.Status != "deleted" {
			return row, nil
		}
	}
	return application.ProofRow{}, application.ErrClaimNotFound
}

func (f *fakeProofStore) ListExpiredProofs(_ context.Context, _ int) ([]application.ProofRow, error) {
	return nil, nil
}

func (f *fakeProofStore) MarkProofExpired(_ context.Context, _ string) error { return nil }

func (f *fakeProofStore) MarkProofDeleted(_ context.Context, _ string) error { return nil }

func proofTestHandler(proofs *fakeProofStore, bytes application.ProofBytes, claims *fakeClaimStore, declaration application.DeclarationRow) ProofHandler {
	n := 0
	return ProofHandler{
		Ports: application.ProofPorts{
			Proofs: proofs,
			Claims: claims,
			Clock:  func() time.Time { return time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC) },
			NewID: func() (string, error) {
				n++
				return "proof-id-" + string(rune('0'+n)), nil
			},
			Bytes: bytes,
		},
		Sessions: func(context.Context, string, string) (string, error) {
			return "acc-1", nil
		},
	}
}

type fakeProofBytes struct {
	objects map[string][]byte
}

func (f *fakeProofBytes) Put(_ context.Context, key string, body []byte) error {
	if f.objects == nil {
		f.objects = map[string][]byte{}
	}
	f.objects[key] = body
	return nil
}

func (f *fakeProofBytes) Delete(_ context.Context, key string) error {
	delete(f.objects, key)
	return nil
}

func proofBody(declarationID string) string {
	raw := append([]byte("%PDF-1.7\n"), make([]byte, 200)...)
	for i := range raw[9:] {
		raw[9+i] = 'a'
	}
	payload, _ := json.Marshal(map[string]any{
		"family_id": "fam", "access_token": "tok",
		"declaration_id": declarationID, "filename": "autorizacao.pdf",
		"content_type": "application/pdf", "kind": "authorization",
		"content_base64": base64.StdEncoding.EncodeToString(raw),
	})
	return string(payload)
}

func doProofRequest(h ProofHandler, target, body string) *httptest.ResponseRecorder {
	router := chi.NewRouter()
	h.RegisterRoutes(router)
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func openHTTPClaim(t *testing.T, store *fakeClaimStore, key string) (application.ClaimRow, application.DeclarationRow) {
	t.Helper()
	n := 0
	ports := application.ClaimPorts{
		Store: store,
		Clock: func() time.Time { return time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC) },
		NewID: func() (string, error) {
			n++
			return "claim-id-" + string(rune('0'+n)), nil
		},
		OperatorOf: func(context.Context, string) (string, string, bool, error) {
			return "04218406000104", "registry", true, nil
		},
	}
	claim, created, err := application.OpenClaim(context.Background(), ports, "acc-1", "station-1", "administrator", []string{"profile.edit"}, key)
	if err != nil || !created {
		t.Fatalf("open = %+v, %v, %v", claim, created, err)
	}
	declaration, err := store.ActiveDeclaration(context.Background(), claim.ID)
	if err != nil {
		t.Fatalf("declaration: %v", err)
	}
	row, err := store.Claim(context.Background(), claim.ID)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	return row, declaration
}

func TestProofSubmitAcceptsAndDedups(t *testing.T) {
	claims := &fakeClaimStore{}
	row, declaration := openHTTPClaim(t, claims, "proof-http-1")
	h := proofTestHandler(&fakeProofStore{}, &fakeProofBytes{}, claims, declaration)
	target := "/v1/profile/claims/" + row.ID + "/proof"

	first := doProofRequest(h, target, proofBody(declaration.ID))
	if first.Code != http.StatusCreated {
		t.Fatalf("submit = %d: %s", first.Code, first.Body.String())
	}
	if first.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("private response without no-store")
	}
	second := doProofRequest(h, target, proofBody(declaration.ID))
	if second.Code != http.StatusOK {
		t.Fatalf("redelivery = %d (must dedup, never rebind)", second.Code)
	}
	var firstDoc, secondDoc map[string]any
	if err := json.Unmarshal(first.Body.Bytes(), &firstDoc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondDoc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if firstDoc["id"] != secondDoc["id"] {
		t.Fatal("redelivery bound a second proof")
	}
}

func TestProofSubmitRefusesUnsafeAndMissingStorage(t *testing.T) {
	claims := &fakeClaimStore{}
	row, declaration := openHTTPClaim(t, claims, "proof-http-2")
	target := "/v1/profile/claims/" + row.ID + "/proof"

	unsafe := strings.Replace(proofBody(declaration.ID), "autorizacao.pdf", "cert.pfx", 1)
	res := doProofRequest(proofTestHandler(&fakeProofStore{}, &fakeProofBytes{}, claims, declaration), target, unsafe)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("pfx = %d, want 400", res.Code)
	}
	// Missing storage blocks intake with 503, never approval fallback.
	h := proofTestHandler(&fakeProofStore{}, nil, claims, declaration)
	res = doProofRequest(h, target, proofBody(declaration.ID))
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("no storage = %d, want 503", res.Code)
	}
}
