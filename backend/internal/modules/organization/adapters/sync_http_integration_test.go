//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	identity "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	eventpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/adapters"
	event "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgquota"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type syncHTTPClock struct{}

func (syncHTTPClock) NowUnix() int64 { return time.Now().Unix() }

func syncHTTPOperation(t *testing.T, fx deviceFixtureState, enrollment string, edit func(map[string]any)) protocol.Operation {
	t.Helper()
	value := map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t),
		"owner_account_id": fx.person.auth.Account.ID, "api_origin": deviceTestOrigin,
		"org_id": fx.scope.OrgID().String(), "membership_id": fx.person.member,
		"enrollment_id": enrollment, "type": "synthetic.record", "operation_schema": 1,
		"entity":      map[string]any{"type": "synthetic", "id": onboardingID(t), "expected_revision": nil},
		"occurred_at": "2000-01-01T00:00:00Z", "entry_zone": "America/Cuiaba", "device_offset_seconds": 0,
		"depends_on": []string{}, "soft_references": []any{}, "attachments": []any{},
		"payload": map[string]any{"label": "Synthetic offline fact"},
	}
	if edit != nil {
		edit(value)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := protocol.NewBinding(fx.person.auth.Account.ID, deviceTestOrigin, fx.scope.OrgID().String())
	if err != nil {
		t.Fatal(err)
	}
	op, err := protocol.Seal(raw, binding)
	if err != nil {
		t.Fatal("synthetic signed intake operation", err)
	}
	return op
}

func syncHTTPBody(fx deviceFixtureState, operations ...json.RawMessage) map[string]any {
	if operations == nil {
		operations = []json.RawMessage{}
	}
	return map[string]any{"schema_version": 1, "family_id": fx.person.auth.Session.FamilyID,
		"access_token": fx.person.auth.Session.AccessToken, "operations": operations}
}

func syncHTTPAssertNoIntake(t *testing.T, fx deviceFixtureState) {
	t.Helper()
	err := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		for _, table := range []string{"org_sync_operations", "org_sync_acknowledgements", "org_sync_effects"} {
			var count int
			if err := tx.QueryRow(t.Context(), "SELECT count(*) FROM "+table).Scan(&count); err != nil {
				return err
			}
			if count != 0 {
				t.Fatalf("unavailable or invalid intake changed %s", table)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestOrganizationSyncDefaultRuntimeAndSignedBoundary(t *testing.T) {
	fx := devicesFixture(t)
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT ON org_sync_operations,org_sync_acknowledgements,org_sync_effects TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	enrolled, err := fx.devices.EnrollDevice(t.Context(), fx.scope, fx.person.actor, deviceEnrollCommand(t, "Synthetic signed intake"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := enrolled.Device.Enrollment.Snapshot(fx.scope)
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	accounts := *fx.accounts
	accounts.Store, accounts.Clock = accountpg.NewPGStore(fx.f.Runtime), syncHTTPClock{}
	// The normal synthetic runtime now owns the actual meter producer and its
	// explicit reviewed budget. Unknown owners remain nonfinal and cannot claim effects.
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: deviceTestOrigin,
		CursorSecret: bytes.Repeat([]byte{92}, 32), Pool: fx.f.Runtime, Accounts: &accounts}); err != nil {
		t.Fatal(err)
	}
	path := "/v1/orgs/" + fx.scope.OrgID().String() + "/sync/operations"
	dispatch := func(t *testing.T, body map[string]any, want int, edit func(*http.Request)) []json.RawMessage {
		t.Helper()
		r := deviceSignedRequest(t, fx.registrar, fx.person.signer, path, body, time.Time{})
		if edit != nil {
			edit(r)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != want || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("ETag") != "" {
			t.Fatalf("signed sync status=%d want=%d; private cache headers=%v", w.Code, want, w.Header().Get("Cache-Control"))
		}
		var response struct {
			SchemaVersion    int               `json:"schema_version"`
			Acknowledgements []json.RawMessage `json:"acknowledgements"`
			Error            struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal(w.Body.Bytes(), &response) != nil {
			t.Fatal("signed intake envelope")
		}
		if want >= 400 {
			if response.Error.Code == "" {
				t.Fatal("signed intake error envelope")
			}
		} else if response.SchemaVersion != 1 || response.Acknowledgements == nil || response.Error.Code != "" {
			t.Fatal("signed intake acknowledgement envelope")
		}
		for _, private := range []string{fx.person.auth.Session.AccessToken, fx.person.auth.Session.FamilyID,
			fx.person.signer.keyID, fx.person.signer.fingerprint, fx.person.signer.contributor} {
			if strings.Contains(w.Body.String(), private) {
				t.Fatal("signed intake disclosed source proof")
			}
		}
		return response.Acknowledgements
	}
	op := syncHTTPOperation(t, fx, snapshot.ID, nil)
	valid := syncHTTPBody(fx, op.CanonicalBytes())
	t.Run("actual-runtime-budget-with-unknown-owner-closed", func(t *testing.T) {
		if acks := dispatch(t, syncHTTPBody(fx), http.StatusOK, nil); len(acks) != 0 {
			t.Fatal("empty batch invented an operation")
		}
		acks := dispatch(t, valid, http.StatusOK, nil)
		if len(acks) != 1 {
			t.Fatal("missing unavailable acknowledgement")
		}
		ack, e := protocol.ParseAcknowledgement(acks[0], op)
		if e != nil || ack.Result() != protocol.Retry || ack.Code() != "UNAVAILABLE" || ack.IsFinal() {
			t.Fatal("unknown owner became a final effect", e)
		}
	})
	t.Run("full-envelope-before-availability", func(t *testing.T) {
		dispatch(t, syncHTTPBody(fx, op.CanonicalBytes(), json.RawMessage(`{"schema_version":1}`)), http.StatusBadRequest, nil)
		wrong := syncHTTPOperation(t, fx, onboardingID(t), nil)
		dispatch(t, syncHTTPBody(fx, op.CanonicalBytes(), wrong.CanonicalBytes()), http.StatusBadRequest, nil)
		var changed map[string]any
		if json.Unmarshal(op.CanonicalBytes(), &changed) != nil {
			t.Fatal("operation fixture")
		}
		changed["payload"] = map[string]any{"label": "Changed after sealing"}
		raw, err := json.Marshal(changed)
		if err != nil {
			t.Fatal(err)
		}
		dispatch(t, syncHTTPBody(fx, op.CanonicalBytes(), raw), http.StatusBadRequest, nil)
	})
	t.Run("exclusive-signed-channel", func(t *testing.T) {
		for _, edit := range []func(*http.Request){
			func(r *http.Request) { r.Header.Del("Signature") },
			func(r *http.Request) { r.Header.Set("Cookie", "__Host-empresas-session=synthetic") },
			func(r *http.Request) { r.Header.Set("Authorization", "Bearer synthetic") },
			func(r *http.Request) { r.Header.Set("X-CSRF", "synthetic") },
			func(r *http.Request) { r.Header.Set("Origin", "https://foreign.example.invalid") },
		} {
			dispatch(t, valid, http.StatusUnauthorized, edit)
		}
	})
	t.Run("nonce-remains-one-use-after-unavailable", func(t *testing.T) {
		r := deviceSignedRequest(t, fx.registrar, fx.person.signer, path, valid, time.Time{})
		first := httptest.NewRecorder()
		router.ServeHTTP(first, r)
		if first.Code != http.StatusOK {
			t.Fatal("unknown owner transport proof", first.Code)
		}
		var response struct {
			Acknowledgements []json.RawMessage `json:"acknowledgements"`
		}
		if json.Unmarshal(first.Body.Bytes(), &response) != nil || len(response.Acknowledgements) != 1 {
			t.Fatal("unknown owner acknowledgement")
		}
		ack, e := protocol.ParseAcknowledgement(response.Acknowledgements[0], op)
		if e != nil || ack.Result() != protocol.Retry || ack.Code() != "UNAVAILABLE" || ack.IsFinal() {
			t.Fatal("missing owner admitted a final effect", e)
		}
		raw, err := json.Marshal(valid)
		if err != nil {
			t.Fatal(err)
		}
		r.Body, r.ContentLength = io.NopCloser(bytes.NewReader(raw)), int64(len(raw))
		reused := httptest.NewRecorder()
		router.ServeHTTP(reused, r)
		if reused.Code != http.StatusUnauthorized || reused.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("unavailable intake permitted replayed signature nonce", reused.Code)
		}
	})
	t.Run("signature-covers-exact-transmitted-body", func(t *testing.T) {
		r := deviceSignedRequest(t, fx.registrar, fx.person.signer, path, valid, time.Time{})
		raw, err := json.Marshal(valid)
		if err != nil {
			t.Fatal(err)
		}
		tampered := bytes.Replace(raw, []byte("Synthetic offline fact"), []byte("Synthetic altered fact"), 1)
		if bytes.Equal(tampered, raw) {
			t.Fatal("signature tampering fixture did not change body")
		}
		r.Body, r.ContentLength = io.NopCloser(bytes.NewReader(tampered)), int64(len(tampered))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != http.StatusUnauthorized || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("signed intake accepted modified bytes", w.Code)
		}
	})
	syncHTTPAssertNoIntake(t, fx)
}

type syncHTTPFixtureState struct {
	syncFixtureState
	router     chi.Router
	dispatcher jobs.Dispatcher
	path       string
}

func syncHTTPFixture(t *testing.T) syncHTTPFixtureState {
	t.Helper()
	fx := syncFixture(t)
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	var super, bypass bool
	if err := fx.f.Runtime.QueryRow(t.Context(), `SELECT rolsuper,rolbypassrls FROM pg_roles WHERE rolname=current_user`).Scan(&super, &bypass); err != nil || super || bypass {
		t.Fatal("signed intake runtime must be ordinary", err)
	}
	consumers := map[string]eventpg.Effect{syncTestConsumer: func(ctx context.Context, scope kernel.OrgScope, tx pgx.Tx, value event.Event) error {
		_, err := tx.Exec(ctx, `INSERT INTO org_sync_test_followups(org_id,operation_id,event_name)VALUES($1,$2,$3)`, scope.OrgID().String(), value.ID, value.Name)
		return err
	}}
	_, followup, err := enterprisesuite.IntakeEvents(fx.f.Runtime, consumers)
	if err != nil || followup == nil {
		t.Fatal("actual intake follow-up registration", err)
	}
	accounts := *fx.accounts
	accounts.Store, accounts.Clock = accountpg.NewPGStore(fx.f.Runtime), syncHTTPClock{}
	quota := identity.OperationQuota{Limit: 2000, Window: time.Hour}
	budget := orgquota.TransportDefinition{Class: "validation.intake", Organization: quota, Account: quota, Device: quota,
		Rationale: "Explicit synthetic HTTP acceptance only:2000 operations/hour exceeds this named test's bounded attempts; no production default."}
	router := chi.NewRouter()
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: deviceTestOrigin,
		CursorSecret: bytes.Repeat([]byte{93}, 32), Pool: fx.f.Runtime, Accounts: &accounts,
		IntakeHandlers: []syncapp.Handler{fx.handler}, IntakeBudget: &budget, IntakePlans: fx.config.Plans, IntakeConsumers: consumers}); err != nil {
		t.Fatal(err)
	}
	return syncHTTPFixtureState{syncFixtureState: fx, router: router,
		path: "/v1/orgs/" + fx.scope.OrgID().String() + "/sync/operations",
		dispatcher: jobs.Dispatcher{Queue: jobs.NewQueue(fx.f.Runtime), Handlers: map[string]jobs.Handler{followup.Kind(): followup},
			Kind: followup.Kind(), WorkerID: "synthetic-sync-http", RetryDelay: time.Microsecond}}
}

func syncHTTPCall(t *testing.T, fx syncHTTPFixtureState, want int, operations ...protocol.Operation) []json.RawMessage {
	t.Helper()
	items := make([]json.RawMessage, len(operations))
	for i, op := range operations {
		items[i] = op.CanonicalBytes()
	}
	r := deviceSignedRequest(t, fx.registrar, fx.person.signer, fx.path, syncHTTPBody(fx.deviceFixtureState, items...), time.Time{})
	w := httptest.NewRecorder()
	fx.router.ServeHTTP(w, r)
	if w.Code != want || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("ETag") != "" {
		t.Fatalf("actual signed intake status=%d want=%d", w.Code, want)
	}
	for _, private := range []string{fx.person.auth.Session.AccessToken, fx.person.auth.Session.FamilyID,
		fx.person.signer.keyID, fx.person.signer.fingerprint, fx.person.signer.contributor} {
		if strings.Contains(w.Body.String(), private) {
			t.Fatal("actual intake exposed source identity proof")
		}
	}
	if want != http.StatusOK {
		if strings.Contains(w.Body.String(), "acknowledgements") {
			t.Fatal("global denial exposed buffered private acknowledgements")
		}
		return nil
	}
	var value struct {
		Version int               `json:"schema_version"`
		Items   []json.RawMessage `json:"acknowledgements"`
	}
	decoder := json.NewDecoder(bytes.NewReader(w.Body.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil || value.Version != 1 || len(value.Items) != len(operations) {
		t.Fatal("actual intake response wrapper", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		t.Fatal("actual intake response has trailing data")
	}
	for i, raw := range value.Items {
		ack, err := protocol.ParseAcknowledgement(raw, operations[i])
		if err != nil || !bytes.Equal(raw, ack.CanonicalBytes()) {
			t.Fatal("actual response omitted original canonical intent-bound ACK", err)
		}
	}
	return value.Items
}

func TestOrganizationSyncActualSignedIndependentRecoveryAndQueue(t *testing.T) {
	fx := syncHTTPFixture(t)
	parentID := onboardingID(t)
	child := syncOperation(t, fx.syncFixtureState, onboardingID(t), "Synthetic dependent fact", nil, []string{parentID}, nil)
	malformed := syncOperation(t, fx.syncFixtureState, onboardingID(t), "", nil, nil, nil)
	parent := syncOperation(t, fx.syncFixtureState, parentID, "Synthetic dependency producer", nil, nil, nil)
	orphan := syncOperation(t, fx.syncFixtureState, onboardingID(t), "Synthetic held orphan", nil, nil,
		[]protocol.SoftReference{{Type: "setting", ID: onboardingID(t)}})
	old := syncHTTPOperation(t, fx.deviceFixtureState, fx.source.EnrollmentID, func(value map[string]any) {
		value["type"], value["entity"], value["payload"] = syncTestType,
			map[string]any{"type": "setting", "id": fx.target, "expected_revision": nil}, map[string]any{"value": "Synthetic legitimately delayed fact"}
	})
	unsupported := syncHTTPOperation(t, fx.deviceFixtureState, fx.source.EnrollmentID, func(value map[string]any) {
		value["type"], value["operation_schema"], value["entity"], value["payload"] = syncTestType, 2,
			map[string]any{"type": "setting", "id": fx.target, "expected_revision": nil}, map[string]any{"value": "Synthetic future schema"}
	})
	operations := []protocol.Operation{child, malformed, parent, orphan, old, unsupported}
	quotaCount := func() int64 {
		t.Helper()
		var count int64
		if err := fx.f.Admin.QueryRow(t.Context(), `SELECT COALESCE(sum(count),0)::bigint FROM identity_rate_windows WHERE subject_digest LIKE 'orgquota:v1:%'`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	before := quotaCount()
	foreign := syncHTTPOperation(t, fx.deviceFixtureState, onboardingID(t), nil)
	syncHTTPCall(t, fx, http.StatusBadRequest, parent, foreign)
	syncHTTPCall(t, fx, http.StatusBadRequest, parent, parent)
	syncHTTPAssertNoIntake(t, fx.deviceFixtureState)
	if quotaCount() != before {
		t.Fatal("invalid whole-body binding spent intake budget")
	}
	syncHTTPCall(t, fx, http.StatusOK)
	if quotaCount() != before+3 {
		t.Fatal("empty signed transport did not spend exactly one unit per opaque axis")
	}
	first := syncHTTPCall(t, fx, http.StatusOK, operations...)
	results := []string{protocol.Retry, protocol.Rejected, protocol.Accepted, protocol.AcceptedForReview, protocol.Accepted, protocol.Rejected}
	codes := []string{"DEPENDENCY_PENDING", "MALFORMED", "", "ORPHAN_REFERENCE", "", "MALFORMED"}
	for i, raw := range first {
		ack, err := protocol.ParseAcknowledgement(raw, operations[i])
		if err != nil || ack.Result() != results[i] || ack.Code() != codes[i] || len(ack.Warnings()) != 0 {
			t.Fatalf("independent signed outcome %d result=%s code=%s", i, ack.Result(), ack.Code())
		}
	}
	for table, want := range map[string]int{"org_sync_operations": 6, "org_sync_acknowledgements": 5,
		"org_sync_effects": 2, "org_sync_test_records": 3, "org_sync_test_followups": 0} {
		if got := syncCount(t, fx.syncFixtureState, table); got != want {
			t.Fatalf("first signed batch %s got%d want%d", table, got, want)
		}
	}
	second := syncHTTPCall(t, fx, http.StatusOK, operations...)
	childACK, err := protocol.ParseAcknowledgement(second[0], child)
	if err != nil || childACK.Result() != protocol.Accepted {
		t.Fatal("actual committed dependency did not release child", err)
	}
	for i := 1; i < len(first); i++ {
		if !bytes.Equal(first[i], second[i]) {
			t.Fatal("resend changed immutable original ACK bytes")
		}
	}
	third := syncHTTPCall(t, fx, http.StatusOK, operations...)
	for i := range second {
		if !bytes.Equal(second[i], third[i]) {
			t.Fatal("lost batch answer duplicated or replaced an acknowledgement")
		}
	}
	changed := syncOperation(t, fx.syncFixtureState, parentID, "Synthetic changed immutable intent", nil, nil, nil)
	conflict := syncHTTPCall(t, fx, http.StatusOK, changed)
	reused, err := protocol.ParseAcknowledgement(conflict[0], changed)
	if err != nil || reused.Result() != protocol.Rejected || reused.Code() != "OPERATION_ID_REUSED" {
		t.Fatal("changed signed intent reused a final operation", err)
	}
	unchanged := syncHTTPCall(t, fx, http.StatusOK, parent)
	if !bytes.Equal(unchanged[0], first[2]) {
		t.Fatal("changed attempt replaced the original final receipt")
	}
	if quotaCount() != before+63 {
		t.Fatal("signed transport did not meter every operation, including replay attempts")
	}
	for range 6 {
		found, err := fx.dispatcher.RunOnce(t.Context())
		if err != nil || !found {
			t.Fatal("actual follow-up queue did not dispatch", err)
		}
	}
	if found, err := fx.dispatcher.RunOnce(t.Context()); err != nil || found {
		t.Fatal("replay duplicated a final follow-up", err)
	}
	for table, want := range map[string]int{"org_sync_operations": 6, "org_sync_acknowledgements": 6,
		"org_sync_effects": 3, "org_sync_test_records": 4, "org_sync_test_followups": 6} {
		if got := syncCount(t, fx.syncFixtureState, table); got != want {
			t.Fatalf("recovered signed batch %s got%d want%d", table, got, want)
		}
	}
}

func TestOrganizationSyncLateSourceDenialWithholdsCommittedPrefix(t *testing.T) {
	fx := syncHTTPFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	done := make(chan error, 1)
	fx.handler.afterApply = func(context.Context, syncapp.DBTX) error {
		go func() {
			done <- accountpg.NewPGStore(fx.f.Runtime).RevokeFamily(ctx, fx.source.Actor.FamilyID, time.Now().Unix())
		}()
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return errors.New("synthetic source revocation wait was not observed")
			case err := <-done:
				if err != nil {
					return err
				}
				return errors.New("source revocation bypassed retained source locks")
			case <-ticker.C:
				var blocked bool
				if err := fx.f.Admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND usename=$1 AND wait_event_type='Lock' AND query LIKE '%name: RevokeFamily%')`, fx.f.RuntimeRole).Scan(&blocked); err != nil {
					return err
				}
				if blocked {
					return nil
				}
			}
		}
	}
	accepted := syncOperation(t, fx.syncFixtureState, onboardingID(t), "Synthetic committed private prefix", nil, nil, nil)
	unknown := syncHTTPOperation(t, fx.deviceFixtureState, fx.source.EnrollmentID, nil)
	syncHTTPCall(t, fx, http.StatusUnauthorized, accepted, unknown)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal("actual source revocation", err)
		}
	case <-ctx.Done():
		t.Fatal("actual source revocation did not complete")
	}
	for _, table := range []string{"org_sync_operations", "org_sync_acknowledgements", "org_sync_effects", "org_sync_test_records"} {
		if got := syncCount(t, fx.syncFixtureState, table); got != 1 {
			t.Fatal("late source denial rolled back or duplicated a committed prefix", table, got)
		}
	}
}
