//go:build integration

package orgapi_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	accountapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	accountdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	identitypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters/auth"
	identitydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/telemetry/metrics"
)

type liveClock struct{ now int64 }

func (clock liveClock) NowUnix() int64 { return clock.now }

type liveSigner struct {
	key                      *ecdsa.PrivateKey
	fingerprint, contributor string
}

func signLines(t *testing.T, key *ecdsa.PrivateKey, lines []string) string {
	t.Helper()
	digest := sha512.Sum512([]byte(strings.Join(lines, "\n")))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...))
}

func enrollSigner(t *testing.T, registrar *identitypg.Registrar) liveSigner {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	public, err := key.PublicKey.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	x, y := base64.RawURLEncoding.EncodeToString(public[1:33]), base64.RawURLEncoding.EncodeToString(public[33:])
	thumb := `{"crv":"P-256","kty":"EC","x":"` + x + `","y":"` + y + `"}`
	fingerprint := fmt.Sprintf("fp:%x", sha256.Sum256([]byte(thumb)))
	challenge, err := registrar.IssueChallenge(t.Context(), fingerprint, identitydomain.PurposeRegister)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	request := httptest.NewRequest("POST", "https://test.invalid/v1/contributors", nil)
	lines := auth.BaseLines(request, "test.invalid", fmt.Sprint(now.Add(-time.Minute).Unix()), fmt.Sprint(now.Add(4*time.Minute).Unix()), fingerprint, challenge.Nonce, nil, false)
	result, err := registrar.Register(t.Context(), identitydomain.RegistrationRequest{
		JWKX: x, JWKY: y, Challenge: challenge, BaseLines: lines, Signature: signLines(t, key, lines), VerifiedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	return liveSigner{key: key, fingerprint: fingerprint, contributor: result.ContributorID}
}

func signedRequest(t *testing.T, registrar *identitypg.Registrar, signer liveSigner, path, body, authority string, at time.Time) *http.Request {
	t.Helper()
	challenge, err := registrar.IssueChallenge(t.Context(), signer.fingerprint, identitydomain.PurposeSign)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", origin+path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(auth.HeaderCreated, fmt.Sprint(at.Add(-time.Minute).Unix()))
	request.Header.Set(auth.HeaderExpires, fmt.Sprint(at.Add(4*time.Minute).Unix()))
	request.Header.Set(auth.HeaderKeyID, signer.fingerprint)
	request.Header.Set(auth.HeaderNonce, challenge.Nonce)
	lines := auth.BaseLines(request, authority, request.Header.Get(auth.HeaderCreated), request.Header.Get(auth.HeaderExpires), signer.fingerprint, challenge.Nonce, []byte(body), true)
	request.Header.Set(auth.HeaderSignature, signLines(t, signer.key, lines))
	request.Header.Set("If-None-Match", "*")
	return request
}

func TestGatewayRealSessionKeyBindingProofAndCurrentAuthority(t *testing.T) {
	f := orgtest.New(t)
	now := time.Now()
	accounts := accountpg.NewPGStore(f.Admin)
	hasher := accountdomain.SHA256Hasher{}
	if err := accounts.CreateAccount(t.Context(), accountdomain.Account{ID: accountID, Alias: "synthetic-enterprise", Status: accountdomain.StatusActive, CreatedAt: now.Unix()}, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	salt, err := hasher.NewSalt()
	if err != nil {
		t.Fatal(err)
	}
	if err := accounts.CreateFamily(t.Context(), accountdomain.SessionFamily{
		ID: familyID, AccountID: accountID, RefreshSalt: salt, RefreshHash: hasher.Hash(salt, "synthetic-refresh"),
		AccessSalt: salt, AccessHash: hasher.Hash(salt, "synthetic-access"), AccessExpires: now.Add(15 * time.Minute).Unix(), IssuedAt: now.Unix(),
	}); err != nil {
		t.Fatal(err)
	}
	service := accountapp.Service{Store: accounts, Hasher: hasher, Clock: liveClock{now: now.Unix()}}
	registrar := identitypg.NewRegistrar(f.Admin)
	signer := enrollSigner(t, registrar)
	if err := accounts.BindContributor(t.Context(), accountdomain.ContributorBinding{
		AccountID: accountID, ContributorID: signer.contributor, KeyFingerprint: signer.fingerprint, BoundAt: now.Unix(),
	}, "7b0e2a54-0000-4000-8000-000000000099"); err != nil {
		t.Fatal(err)
	}
	// Test-only policy rows stand in for the future AUTH-owned provider.
	if _, err := f.Admin.Exec(t.Context(), `CREATE TABLE org_api_membership_fixture
		(org_id UUID PRIMARY KEY,account_id UUID NOT NULL,id UUID NOT NULL,active BOOLEAN NOT NULL);
		INSERT INTO org_api_membership_fixture VALUES ('`+orgA+`','`+accountID+`','`+membershipID+`',true)`); err != nil {
		t.Fatal(err)
	}
	verifier := auth.Verifier{Pool: f.Admin, Authority: "api.example.invalid", Now: func() time.Time { return now }}
	gateway := orgapi.Gateway{
		Enabled: true,
		Origin:  origin, CursorSecret: []byte(strings.Repeat("synthetic", 8)), Sessions: service.ValidateAccess,
		SignedOwner: func(r *http.Request, owner string) (string, string, error) {
			identity, err := verifier.Verify(r.Context(), r)
			if err != nil {
				return "", "", orgapi.Unauthenticated
			}
			bound, found, err := accounts.FindBindingOwner(r.Context(), identity.ContributorID)
			if err != nil || !found || bound != owner {
				return "", "", orgapi.Unauthenticated
			}
			return identity.ContributorID, identity.KeyID, nil
		},
		Resolve: func(ctx context.Context, actor orgapi.Actor, scope kernel.OrgScope, rule orgapi.Rule) (orgapi.Membership, error) {
			var id string
			err := f.Admin.QueryRow(ctx, `SELECT id::text FROM org_api_membership_fixture
				WHERE org_id=$1 AND account_id=$2 AND active`, scope.OrgID().String(), actor.AccountID).Scan(&id)
			if err != nil {
				return orgapi.Membership{}, orgapi.NotFound
			}
			return orgapi.Membership{ID: id, AccountID: actor.AccountID, OrgID: scope.OrgID()}, nil
		},
	}
	router := chi.NewRouter()
	telemetryRegistry := metrics.NewRegistry()
	observe, err := orgapi.Observe(telemetryRegistry)
	if err != nil {
		t.Fatal(err)
	}
	router.Use(observe)
	var calls atomic.Int32
	if err := orgapi.Register(router, route, gateway, orgapi.Rule{Permission: "org.settings.manage", Write: true, RequireRevision: true},
		func(_ context.Context, principal orgapi.Principal, _ commandDTO) (any, error) {
			calls.Add(1)
			if principal.ContributorID != signer.contributor || principal.DeviceKeyID == "" || principal.Scope.OrgID().String() != orgA {
				t.Error("proof attribution lost")
			}
			return map[string]string{"org_id": principal.Scope.OrgID().String()}, nil
		}); err != nil {
		t.Fatal(err)
	}
	path := "/v1/orgs/" + orgA + "/example/read"
	body := `{"family_id":"` + familyID + `","access_token":"synthetic-access","schema_version":1,"operation_id":"` + membershipID + `","revision":1,"value":"original"}`
	dispatch := func(request *http.Request) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	first := signedRequest(t, registrar, signer, path, body, "api.example.invalid", now)
	if response := dispatch(first); response.Code != 200 || response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("ETag") != "" {
		t.Fatal("real proof failed")
	}
	first.Body = io.NopCloser(strings.NewReader(body))
	checkError(t, dispatch(first), orgapi.Unauthenticated, 401)
	checkError(t, dispatch(signedRequest(t, registrar, signer, path, body, "foreign.example.invalid", now)), orgapi.Unauthenticated, 401)
	checkError(t, dispatch(signedRequest(t, registrar, signer, path, body, "api.example.invalid", now.Add(-time.Hour))), orgapi.Unauthenticated, 401)
	tampered := signedRequest(t, registrar, signer, path, body, "api.example.invalid", now)
	changed := strings.Replace(body, "original", "modified", 1)
	tampered.Body = io.NopCloser(strings.NewReader(changed))
	tampered.ContentLength = int64(len(changed))
	checkError(t, dispatch(tampered), orgapi.Unauthenticated, 401)
	if err := accounts.CreateAccount(t.Context(), accountdomain.Account{
		ID: orgB, Alias: "synthetic-other-account", Status: accountdomain.StatusActive, CreatedAt: now.Unix(),
	}, strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	otherFamily := "7b0e2a54-0000-4000-8000-000000000014"
	if err := accounts.CreateFamily(t.Context(), accountdomain.SessionFamily{
		ID: otherFamily, AccountID: orgB, RefreshSalt: salt, RefreshHash: hasher.Hash(salt, "synthetic-refresh"),
		AccessSalt: salt, AccessHash: hasher.Hash(salt, "synthetic-access"), AccessExpires: now.Add(15 * time.Minute).Unix(), IssuedAt: now.Unix(),
	}); err != nil {
		t.Fatal(err)
	}
	foreignOwnerBody := strings.Replace(body, familyID, otherFamily, 1)
	checkError(t, dispatch(signedRequest(t, registrar, signer, path, foreignOwnerBody, "api.example.invalid", now)), orgapi.Unauthenticated, 401)
	foreign := dispatch(signedRequest(t, registrar, signer, "/v1/orgs/"+orgB+"/example/read", body, "api.example.invalid", now))
	checkError(t, foreign, orgapi.NotFound, 404)
	if _, err := f.Admin.Exec(t.Context(), "UPDATE org_api_membership_fixture SET active=false"); err != nil {
		t.Fatal(err)
	}
	offboarded := dispatch(signedRequest(t, registrar, signer, path, body, "api.example.invalid", now))
	checkError(t, offboarded, orgapi.NotFound, 404)
	if offboarded.Body.String() != foreign.Body.String() || calls.Load() != 1 {
		t.Fatal("foreign/offboarded authority reached module or leaked state")
	}
	if _, err := f.Admin.Exec(t.Context(), "UPDATE org_api_membership_fixture SET active=true"); err != nil {
		t.Fatal(err)
	}

	duplicate := signedRequest(t, registrar, signer, path, body, "api.example.invalid", now)
	results := make(chan int, 2)
	for range 2 {
		copy := duplicate.Clone(t.Context())
		copy.Body = io.NopCloser(strings.NewReader(body))
		go func() { results <- dispatch(copy).Code }()
	}
	successes := 0
	for range 2 {
		switch code := <-results; code {
		case 200:
			successes++
		case 401:
		default:
			t.Fatalf("concurrent proof returned %d", code)
		}
	}
	if successes != 1 || calls.Load() != 2 {
		t.Fatal("nonce replay dispatched twice")
	}
	revoked := signedRequest(t, registrar, signer, path, body, "api.example.invalid", now)
	if _, err := f.Admin.Exec(t.Context(), "UPDATE identity_keys SET revoked_at=clock_timestamp() WHERE fingerprint=$1", signer.fingerprint); err != nil {
		t.Fatal(err)
	}
	checkError(t, dispatch(revoked), orgapi.Unauthenticated, 401)
	if _, err := f.Admin.Exec(t.Context(), "UPDATE identity_keys SET revoked_at=NULL WHERE fingerprint=$1", signer.fingerprint); err != nil {
		t.Fatal(err)
	}
	expired := signedRequest(t, registrar, signer, path, body, "api.example.invalid", now)
	if _, err := f.Admin.Exec(t.Context(), "UPDATE account_session_families SET revoked_at=clock_timestamp() WHERE id=$1", familyID); err != nil {
		t.Fatal(err)
	}
	checkError(t, dispatch(expired), orgapi.Unauthenticated, 401)
	if _, err := f.Admin.Exec(t.Context(), "UPDATE account_session_families SET revoked_at=NULL WHERE id=$1", familyID); err != nil {
		t.Fatal(err)
	}
	suspended := signedRequest(t, registrar, signer, path, body, "api.example.invalid", now)
	if _, err := f.Admin.Exec(t.Context(), "UPDATE accounts SET status='suspended' WHERE id=$1", accountID); err != nil {
		t.Fatal(err)
	}
	checkError(t, dispatch(suspended), orgapi.Unauthenticated, 401)
	if calls.Load() != 2 {
		t.Fatal("revoked key or session reached module")
	}
	exposition := telemetryRegistry.Expose()
	for _, secret := range []string{orgA, orgB, accountID, familyID, "synthetic-access", "original", "modified"} {
		if strings.Contains(exposition, secret) {
			t.Fatal("real authority fixture leaked into telemetry")
		}
	}
	for _, code := range []string{"OK", "UNAUTHENTICATED", "NOT_FOUND"} {
		if !strings.Contains(exposition, `code="`+code+`"`) {
			t.Fatal("real authority result not observed", code)
		}
	}
}
