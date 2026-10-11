//go:build integration

package http

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	communityadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/adapters"
	communitydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
)

func newTestUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	hexed := fmt.Sprintf("%x", b)
	return hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" + hexed[16:20] + "-" + hexed[20:32], nil
}

// unenrolledKey mints a valid keypair that never registers: signatures
// verify cryptographically but proof resolution must still refuse it.
func unenrolledKey(t *testing.T) *callerKey {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := priv.PublicKey.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	enc := base64.RawURLEncoding.EncodeToString
	thumb := `{"crv":"P-256","kty":"EC","x":"` + enc(raw[1:33]) + `","y":"` + enc(raw[33:65]) + `"}`
	sum := sha256.Sum256([]byte(thumb))
	return &callerKey{priv: priv, fp: fmt.Sprintf("fp:%x", sum)}
}

// submitValidated submits through the router with real signed proof,
// then records the worker decisions that admit the observation to
// VALIDATED at sequence 2, and returns the public observation id.
func submitValidated(t *testing.T, r chi.Router, pool *pgxpool.Pool, station string, author *callerKey, submission string) string {
	t.Helper()
	req := signedRequest(t, pool, author, http.MethodPost, "/v1/observations", submitBody(station, submission))
	req.Header.Set("Idempotency-Key", "help-cmd-"+submission)
	w := doRequest(t, r, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("submit = %d: %s", w.Code, w.Body.String())
	}
	var receipt map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	id, _ := receipt["id"].(string)
	if id == "" {
		t.Fatalf("submit receipt lacks id: %v", receipt)
	}
	ctx := context.Background()
	store := communityadapters.NewStore(pool)
	obs, err := store.Observation(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	claim, err := communitydomain.ClaimValidation(obs, communitydomain.StateReceived, "job-help", communitydomain.ActorWorker, now, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordDecision(ctx, claim); err != nil {
		t.Fatal(err)
	}
	admit, err := communitydomain.Admit(obs, communitydomain.StateValidating, communitydomain.ActorWorker, now, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordDecision(ctx, admit); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestHelpfulnessEndToEnd(t *testing.T) {
	r, pool, station := fixture(t)
	author := enrollCaller(t, pool)
	voter := enrollCaller(t, pool)

	obsID := submitValidated(t, r, pool, station, author, "help-e2e-1")
	target := "/v1/observations/" + obsID + "/helpfulness"
	cast := `{"choice":"HELPFUL","decision_sequence":2,"client_submission_id":"help-vote-1"}`

	// Signed voter casts through real proof: 201 with exact tally.
	w := doRequest(t, r, signedRequest(t, pool, voter, http.MethodPost, target, cast))
	if w.Code != http.StatusCreated {
		t.Fatalf("cast = %d: %s", w.Code, w.Body.String())
	}
	var receipt map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt["vote_id"] == nil || receipt["helpful"] != 1.0 || receipt["my_choice"] != "HELPFUL" {
		t.Fatalf("receipt = %v", receipt)
	}
	firstID := receipt["vote_id"]

	// Lost response replays the same vote end to end: same id, totals kept.
	w = doRequest(t, r, signedRequest(t, pool, voter, http.MethodPost, target, cast))
	if w.Code != http.StatusCreated {
		t.Fatalf("replay = %d: %s", w.Code, w.Body.String())
	}
	var replay map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &replay)
	if replay["vote_id"] != firstID || replay["helpful"] != 1.0 {
		t.Fatalf("replay = %v, want same %v", replay, firstID)
	}

	// The author cannot vote on the owned contribution, even signed.
	w = doRequest(t, r, signedRequest(t, pool, author, http.MethodPost, target, cast))
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "community.self-vote") {
		t.Fatalf("self = %d: %s", w.Code, w.Body.String())
	}

	// Anonymous tally reads counts with no owner overlay or internals.
	req, err := http.NewRequest(http.MethodGet, target+"?decision_sequence=2", nil)
	if err != nil {
		t.Fatal(err)
	}
	w = doRequest(t, r, req)
	if w.Code != http.StatusOK {
		t.Fatalf("tally = %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "my_choice") {
		t.Fatalf("anonymous tally leaks overlay: %s", w.Body.String())
	}

	// Stale revision refuses: 409 with the frozen code.
	stale := `{"choice":"HELPFUL","decision_sequence":1,"client_submission_id":"help-stale"}`
	w = doRequest(t, r, signedRequest(t, pool, voter, http.MethodPost, target, stale))
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "community.stale-revision") {
		t.Fatalf("stale = %d: %s", w.Code, w.Body.String())
	}

	// Signed removal zeroes the tally.
	w = doRequest(t, r, signedRequest(t, pool, voter, http.MethodPost, target+"/remove", `{"decision_sequence":2}`))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"helpful":0`) {
		t.Fatalf("remove = %d: %s", w.Code, w.Body.String())
	}
}

func TestHelpfulnessProofNegativesEndToEnd(t *testing.T) {
	r, pool, station := fixture(t)
	author := enrollCaller(t, pool)
	voter := enrollCaller(t, pool)

	obsID := submitValidated(t, r, pool, station, author, "help-e2e-2")
	target := "/v1/observations/" + obsID + "/helpfulness"
	cast := `{"choice":"HELPFUL","decision_sequence":2,"client_submission_id":"help-neg"}`

	// Tampered signature refuses before any business logic.
	req := signedRequest(t, pool, voter, http.MethodPost, target, cast)
	req.Header.Set("Signature", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	if w := doRequest(t, r, req); w.Code != http.StatusUnauthorized {
		t.Fatalf("tampered = %d: %s", w.Code, w.Body.String())
	}

	// Unknown key (valid signature, never enrolled) refuses.
	req = signedRequest(t, pool, unenrolledKey(t), http.MethodPost, target, cast)
	if w := doRequest(t, r, req); w.Code != http.StatusUnauthorized {
		t.Fatalf("unknown key = %d: %s", w.Code, w.Body.String())
	}

	// Oversized body refuses before decode.
	big := fmt.Sprintf(`{"choice":"HELPFUL","decision_sequence":2,"client_submission_id":%q}`, strings.Repeat("y", 70<<10))
	if w := doRequest(t, r, signedRequest(t, pool, voter, http.MethodPost, target, big)); w.Code != http.StatusBadRequest {
		t.Fatalf("oversized = %d", w.Code)
	}

	// Unknown observation 404s after proof.
	w := doRequest(t, r, signedRequest(t, pool, voter, http.MethodPost,
		"/v1/observations/00000000-0000-0000-0000-000000000000/helpfulness", cast))
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown target = %d: %s", w.Code, w.Body.String())
	}
}
