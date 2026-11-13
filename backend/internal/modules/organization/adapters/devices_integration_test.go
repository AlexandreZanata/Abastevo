//go:build integration

package adapters_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	accountd "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	companypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters"
	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/application"
	identitypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters/auth"
	identity "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

const deviceTestOrigin = "https://company.example.invalid"
const deviceTestHost = "company.example.invalid"

type deviceLiveSigner struct {
	key                             *ecdsa.PrivateKey
	fingerprint, contributor, keyID string
}

type devicePerson struct {
	auth             account.AuthResult
	signer           deviceLiveSigner
	actor            app.DeviceActor
	web              app.EmployeeActor
	employee, member string
}

type deviceFixtureState struct {
	legalFixtureState
	devices   *memberpg.DeviceStore
	accounts  *account.Service
	registrar *identitypg.Registrar
	person    devicePerson
}

func deviceSignLines(t *testing.T, key *ecdsa.PrivateKey, lines []string) string {
	t.Helper()
	digest := sha512.Sum512([]byte(strings.Join(lines, "\n")))
	r, s, e := ecdsa.Sign(rand.Reader, key, digest[:])
	if e != nil {
		t.Fatal(e)
	}
	return base64.RawURLEncoding.EncodeToString(append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...))
}

func deviceRegisterSigner(t *testing.T, registrar *identitypg.Registrar) deviceLiveSigner {
	t.Helper()
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	public, e := key.PublicKey.Bytes()
	if e != nil {
		t.Fatal(e)
	}
	x, y := base64.RawURLEncoding.EncodeToString(public[1:33]), base64.RawURLEncoding.EncodeToString(public[33:])
	thumbprint := `{"crv":"P-256","kty":"EC","x":"` + x + `","y":"` + y + `"}`
	fingerprint := fmt.Sprintf("fp:%x", sha256.Sum256([]byte(thumbprint)))
	challenge, e := registrar.IssueChallenge(t.Context(), fingerprint, identity.PurposeRegister)
	if e != nil {
		t.Fatal(e)
	}
	at := time.Now()
	r := httptest.NewRequest(http.MethodPost, deviceTestOrigin+"/v1/contributors", nil)
	lines := auth.BaseLines(r, deviceTestHost, fmt.Sprint(at.Add(-time.Minute).Unix()), fmt.Sprint(at.Add(4*time.Minute).Unix()), fingerprint, challenge.Nonce, nil, false)
	registration, e := registrar.Register(t.Context(), identity.RegistrationRequest{JWKX: x, JWKY: y, Challenge: challenge, BaseLines: lines, Signature: deviceSignLines(t, key, lines), VerifiedAt: at})
	if e != nil {
		t.Fatal(e)
	}
	return deviceLiveSigner{key: key, fingerprint: fingerprint, contributor: registration.ContributorID, keyID: registration.KeyID}
}

func deviceSignedRequest(t *testing.T, registrar *identitypg.Registrar, signer deviceLiveSigner, path string, body any, expires time.Time) *http.Request {
	t.Helper()
	raw, e := json.Marshal(body)
	if e != nil {
		t.Fatal(e)
	}
	challenge, e := registrar.IssueChallenge(t.Context(), signer.fingerprint, identity.PurposeSign)
	if e != nil {
		t.Fatal(e)
	}
	at := time.Now()
	if expires.IsZero() {
		expires = at.Add(4 * time.Minute)
	}
	r := httptest.NewRequest(http.MethodPost, deviceTestOrigin+path, strings.NewReader(string(raw)))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set(auth.HeaderCreated, fmt.Sprint(at.Add(-time.Minute).Unix()))
	r.Header.Set(auth.HeaderExpires, fmt.Sprint(expires.Unix()))
	r.Header.Set(auth.HeaderKeyID, signer.fingerprint)
	r.Header.Set(auth.HeaderNonce, challenge.Nonce)
	lines := auth.BaseLines(r, deviceTestHost, r.Header.Get(auth.HeaderCreated), r.Header.Get(auth.HeaderExpires), signer.fingerprint, challenge.Nonce, raw, true)
	r.Header.Set(auth.HeaderSignature, deviceSignLines(t, signer.key, lines))
	r.Header.Set("If-None-Match", "*")
	return r
}

func deviceVerifyActor(t *testing.T, fx deviceFixtureState, person devicePerson, signer deviceLiveSigner, expires time.Time) app.DeviceActor {
	t.Helper()
	r := deviceSignedRequest(t, fx.registrar, signer, "/v1/orgs/"+fx.scope.OrgID().String()+"/devices/enroll", map[string]any{"family_id": person.auth.Session.FamilyID, "access_token": person.auth.Session.AccessToken, "schema_version": 1, "operation_id": onboardingID(t), "label": "Synthetic proof fixture"}, expires)
	proof, e := (auth.Verifier{Pool: fx.f.Admin, Authority: deviceTestHost}).Verify(t.Context(), r)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := fx.accounts.ValidateAccess(t.Context(), person.auth.Session.FamilyID, person.auth.Session.AccessToken); e != nil {
		t.Fatal(e)
	}
	owner, found, e := fx.accounts.Store.FindBindingOwner(t.Context(), proof.ContributorID)
	if e != nil || !found || owner != person.auth.Account.ID {
		t.Fatal("actual signed account binding", e)
	}
	var expiry int64
	if _, e := fmt.Sscan(r.Header.Get(auth.HeaderExpires), &expiry); e != nil {
		t.Fatal(e)
	}
	return app.DeviceActor{AccountID: owner, FamilyID: person.auth.Session.FamilyID, AccessToken: person.auth.Session.AccessToken, ContributorID: proof.ContributorID, KeyID: proof.KeyID, Fingerprint: proof.Fingerprint, ProofExpires: time.Unix(expiry, 0).UTC()}
}

func deviceAddSigner(t *testing.T, fx deviceFixtureState, person devicePerson) (deviceLiveSigner, app.DeviceActor) {
	t.Helper()
	signer := deviceRegisterSigner(t, fx.registrar)
	if e := fx.accounts.Store.BindContributor(t.Context(), accountd.ContributorBinding{AccountID: person.auth.Account.ID, ContributorID: signer.contributor, KeyFingerprint: signer.fingerprint, BoundAt: time.Now().Unix()}, onboardingID(t)); e != nil {
		t.Fatal(e)
	}
	return signer, deviceVerifyActor(t, fx, person, signer, time.Time{})
}

func deviceNewPerson(t *testing.T, fx deviceFixtureState) devicePerson {
	t.Helper()
	key, e := fx.accounts.CreateKeyAccount(t.Context(), "syndevice"+onboardingID(t)[:8])
	if e != nil {
		t.Fatal(e)
	}
	login, e := fx.accounts.LoginWithKey(t.Context(), key.Key)
	if e != nil {
		t.Fatal(e)
	}
	p := devicePerson{auth: login}
	p.employee, p.member = teamDriver(t, fx.legalFixtureState, login.Account.ID)
	p.signer, p.actor = deviceAddSigner(t, fx, p)
	service := &company.Service{Enabled: true, Store: &companypg.Store{Pool: fx.f.Runtime}, Hasher: accountd.SHA256Hasher{}, Prove: companypg.AccountProof(fx.accounts), NewSecret: accountd.GenerateToken, Now: time.Now, NewID: jobs.NewUUIDv4}
	cookie, e := service.Login(t.Context(), login.Session.FamilyID, login.Session.AccessToken, "")
	if e != nil {
		t.Fatal(e)
	}
	p.web = app.EmployeeActor{AccountID: cookie.Row.AccountID, SessionID: cookie.Row.ID}
	return p
}

func devicesFixture(t *testing.T) deviceFixtureState {
	t.Helper()
	legal := legalFixture(t)
	legal.f.GrantDeviceCommands(t)
	if _, _, e := legal.onboarding.Repository.ActivateOrganization(t.Context(), legal.scope, legal.actor, legal.member, onboardingID(t), 1); e != nil {
		t.Fatal(e)
	}
	catalog, e := audit.NewCatalog(memberpg.DeviceAuditDefinitions())
	if e != nil {
		t.Fatal(e)
	}
	store := &memberpg.DeviceStore{Pool: legal.f.Runtime, Authority: &orgpg.DeviceAuthority{OffboardingAuthority: orgpg.OffboardingAuthority{MembershipAccessAuthority: orgpg.MembershipAccessAuthority{Memberships: legal.store.Memberships, Roles: legal.store.Roles, Capabilities: legal.flags}}}, Audit: auditpg.NewStore(legal.f.Runtime, catalog), Capabilities: legal.flags, NewID: jobs.NewUUIDv4}
	accounts := &account.Service{Clock: onboardingClock{time.Now().UTC().Truncate(time.Second)}, Store: accountpg.NewPGStore(legal.f.Admin), Hasher: accountd.SHA256Hasher{}, IDGen: jobs.NewUUIDv4, TokenGen: accountd.GenerateToken, KeyGen: accountd.GenerateAccountKey}
	fx := deviceFixtureState{legalFixtureState: legal, devices: store, accounts: accounts, registrar: identitypg.NewRegistrar(legal.f.Admin)}
	fx.person = deviceNewPerson(t, fx)
	return fx
}

func deviceEnrollCommand(t *testing.T, label string) app.DeviceCommand {
	t.Helper()
	return app.DeviceCommand{OperationID: onboardingID(t), Label: label}
}

func deviceRevokeCommand(t *testing.T, scope kernel.OrgScope, record app.DeviceRecord, class d.RevocationClass) app.DeviceCommand {
	t.Helper()
	snap, e := record.Enrollment.Snapshot(scope)
	if e != nil {
		t.Fatal(e)
	}
	return app.DeviceCommand{OperationID: onboardingID(t), DeviceID: snap.ID, ExpectedRevision: snap.Revision, Class: class, Reason: "  Synthetic exact device revocation  "}
}

func TestOrganizationDevicesRealProofEnrollmentRevokeAndIndependentMemberships(t *testing.T) {
	fx := devicesFixture(t)
	person := fx.person
	c := deviceEnrollCommand(t, "Synthetic first device")
	member, e := fx.devices.PrepareDeviceEnrollment(t.Context(), fx.scope, person.actor)
	if e != nil || member != person.member {
		t.Fatal("actual enrollment preflight", e)
	}
	first, e := fx.devices.EnrollDevice(t.Context(), fx.scope, person.actor, c)
	if e != nil || first.Replayed || first.Device.Validate(fx.scope) != nil || first.Device.LastSyncAt != nil {
		t.Fatal("actual device enrollment", e)
	}
	snap, _ := first.Device.Enrollment.Snapshot(fx.scope)
	if snap.DeviceID != person.actor.KeyID || snap.MembershipID != person.member || snap.Fingerprint != person.actor.Fingerprint || snap.State != d.DeviceEnrolled || snap.Revision != 1 {
		t.Fatal("enrollment did not retain actual scoped proof")
	}
	if _, e := fx.devices.CheckDevice(t.Context(), fx.scope, person.actor); e != nil {
		t.Fatal("next signed source request", e)
	}
	again, e := fx.devices.EnrollDevice(t.Context(), fx.scope, person.actor, c)
	if e != nil || !again.Replayed || !again.Device.EnrolledAt.Equal(first.Device.EnrolledAt) {
		t.Fatal("original enrollment retry", e)
	}
	manager := employeeActor(fx.legalFixtureState)
	page, e := fx.devices.ListDevices(t.Context(), fx.scope, manager, fx.member, person.member, false, "", 1)
	if e != nil || len(page.Devices) != 1 || page.NextID != "" {
		t.Fatal("manager scoped devices", e)
	}
	if own, e := fx.devices.ListDevices(t.Context(), fx.scope, person.web, "", person.member, true, "", 20); e != nil || len(own.Devices) != 1 {
		t.Fatal("account-bound own devices", e)
	}
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	_, otherMember := teamDriver(t, other, person.auth.Account.ID)
	if _, _, e := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); e != nil {
		t.Fatal(e)
	}
	otherEnrollment, e := fx.devices.EnrollDevice(t.Context(), other.scope, person.actor, deviceEnrollCommand(t, "Synthetic independent device"))
	if e != nil {
		t.Fatal("same key independent organization", e)
	}
	if s, _ := otherEnrollment.Device.Enrollment.Snapshot(other.scope); s.MembershipID != otherMember {
		t.Fatal("independent membership attribution")
	}
	revoke := deviceRevokeCommand(t, fx.scope, first.Device, d.LostOrCompromised)
	revoked, e := fx.devices.RevokeDevice(t.Context(), fx.scope, manager, fx.member, person.member, false, revoke)
	if e != nil || revoked.Replayed || revoked.Device.Validate(fx.scope) != nil || revoked.Reason != revoke.Reason {
		t.Fatal("manager device revocation", e)
	}
	if s, _ := revoked.Device.Enrollment.Snapshot(fx.scope); s.State != d.DeviceRevoked || s.Revocation != d.LostOrCompromised || s.Revision != 2 {
		t.Fatal("revocation class/state")
	}
	if _, e := fx.devices.CheckDevice(t.Context(), fx.scope, person.actor); !errors.Is(e, app.ErrDeviceRevoked) {
		t.Fatal("revoked key next request", e)
	}
	for _, command := range []app.DeviceCommand{c, deviceEnrollCommand(t, "Synthetic attempted return")} {
		if _, e := fx.devices.EnrollDevice(t.Context(), fx.scope, person.actor, command); !errors.Is(e, app.ErrDeviceRevoked) {
			t.Fatal("revoked key reenrolled", e)
		}
	}
	if _, e := fx.devices.CheckDevice(t.Context(), other.scope, person.actor); e != nil {
		t.Fatal("revocation crossed organization boundary", e)
	}
	if _, e := fx.accounts.ValidateAccess(t.Context(), person.auth.Session.FamilyID, person.auth.Session.AccessToken); e != nil {
		t.Fatal("organization revocation ended personal session", e)
	}
	retried, e := fx.devices.RevokeDevice(t.Context(), fx.scope, person.web, "", person.member, true, revoke)
	if e == nil || retried.Replayed {
		t.Fatal("different actor reused manager receipt")
	}
	retried, e = fx.devices.RevokeDevice(t.Context(), fx.scope, manager, fx.member, person.member, false, revoke)
	if e != nil || !retried.Replayed || !retried.Device.RevokedAt.Equal(*revoked.Device.RevokedAt) || retried.Reason != revoked.Reason {
		t.Fatal("original revocation retry", e)
	}
}

func TestOrganizationDevicesLastSlotAndSameOperationContention(t *testing.T) {
	for _, same := range []bool{false, true} {
		t.Run(map[bool]string{false: "last_slot", true: "same_operation"}[same], func(t *testing.T) {
			fx := devicesFixture(t)
			fx.devices.Limit = 1
			actors := make([]app.DeviceActor, 12)
			commands := make([]app.DeviceCommand, 12)
			for i := range actors {
				actors[i] = fx.person.actor
				if !same && i > 0 {
					_, actors[i] = deviceAddSigner(t, fx, fx.person)
				}
				commands[i] = deviceEnrollCommand(t, "Synthetic contention device")
				if same && i > 0 {
					commands[i] = commands[0]
				}
			}
			values, errs := make([]app.DeviceResult, 12), make([]error, 12)
			start := make(chan struct{})
			var wg sync.WaitGroup
			for i := range actors {
				wg.Go(func() {
					<-start
					values[i], errs[i] = fx.devices.EnrollDevice(t.Context(), fx.scope, actors[i], commands[i])
				})
			}
			close(start)
			wg.Wait()
			winners, replays := 0, 0
			for i, e := range errs {
				if e == nil {
					winners++
					if values[i].Replayed {
						replays++
					}
				} else if same || !errors.Is(e, app.ErrDeviceLimit) {
					t.Fatal("device contention", e)
				}
			}
			if same && (winners != 12 || replays != 11) || !same && winners != 1 {
				t.Fatal("device slot effects", winners, replays)
			}
			page, e := fx.devices.ListDevices(t.Context(), fx.scope, fx.person.web, "", fx.person.member, true, "", 20)
			if e != nil || len(page.Devices) != 1 {
				t.Fatal("contended enrollment count", e)
			}
			if same {
				changed := commands[0]
				changed.Label = "Synthetic changed intent"
				if _, e := fx.devices.EnrollDevice(t.Context(), fx.scope, fx.person.actor, changed); e == nil {
					t.Fatal("changed enrollment intent replayed")
				}
			}
		})
	}
}
