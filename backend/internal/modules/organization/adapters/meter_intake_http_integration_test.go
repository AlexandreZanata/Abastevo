//go:build integration

package adapters_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type meterIntakeHTTPFixture struct {
	syncFixtureState
	server      *httptest.Server
	asset       string
	meter       app.MeterInstallResult
	installedAt time.Time
	path        string
}

func meterIntakeHTTP(t *testing.T) meterIntakeHTTPFixture {
	t.Helper()
	fx := syncFixture(t)
	assets := fleetStore(t, fx.legalFixtureState)
	created, _, err := fleetWrite(t, fx.legalFixtureState, assets, fleetCommand(t, fleetCreateInput()))
	if err != nil {
		t.Fatal("synthetic actual asset creation failed", err)
	}
	meters := meterStore(t, fx.legalFixtureState)
	installed, _, err := meterInstall(t, fx.legalFixtureState, meters, meterInstallCommand(t, meterInstallInput(t, created.AssetID, time.Now().UTC().Add(-time.Hour))))
	if err != nil {
		t.Fatal("actual opening installation unavailable", err)
	}
	view, err := meters.ReadLatestMeter(t.Context(), fx.scope, fleetActor(fx.legalFixtureState), fx.member, created.AssetID, installed.MeterID)
	if err != nil {
		t.Fatal("actual installed service window unavailable", err)
	}
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	accounts := *fx.accounts
	accounts.Store, accounts.Clock = accountpg.NewPGStore(fx.f.Runtime), syncHTTPClock{}
	router := chi.NewRouter()
	// No injected handler, consumer, budget or no-async override: this is the
	// real default runtime producer and its explicit source-owned obligations.
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{93}, 32), Pool: fx.f.Runtime, Accounts: &accounts}); err != nil {
		t.Fatal("default meter runtime composition failed", err)
	}
	server := httptest.NewTLSServer(router)
	t.Cleanup(server.Close)
	server.Client().Timeout = 8 * time.Second
	return meterIntakeHTTPFixture{syncFixtureState: fx, server: server, asset: created.AssetID, meter: installed, installedAt: view.InstalledAt, path: "/v1/orgs/" + fx.scope.OrgID().String() + "/sync/operations"}
}

func (fx meterIntakeHTTPFixture) operation(t *testing.T, value string, edit func(map[string]any)) protocol.Operation {
	t.Helper()
	return syncHTTPOperation(t, fx.deviceFixtureState, fx.source.EnrollmentID, func(raw map[string]any) {
		raw["type"] = fleetpg.MeterReadingOperationType
		raw["entity"] = map[string]any{"type": "meter_reading", "id": onboardingID(t), "expected_revision": nil}
		raw["occurred_at"] = time.Now().UTC().Format(time.RFC3339Nano)
		raw["payload"] = map[string]any{"asset_id": fx.asset, "meter_id": fx.meter.MeterID, "segment_id": fx.meter.SegmentID, "kind": "ODOMETER", "value": value, "method": "TYPED"}
		if edit != nil {
			edit(raw)
		}
	})
}

func (fx meterIntakeHTTPFixture) dispatch(t *testing.T, op protocol.Operation, want int) protocol.Acknowledgement {
	t.Helper()
	r := deviceSignedRequest(t, fx.registrar, fx.person.signer, fx.path, syncHTTPBody(fx.deviceFixtureState, op.CanonicalBytes()), time.Time{})
	endpoint, err := url.Parse(fx.server.URL + fx.path)
	if err != nil {
		t.Fatal(err)
	}
	r.URL, r.RequestURI = endpoint, ""
	r.Host = deviceTestHost
	before := ""
	if want >= 400 {
		before = matrixPrivateState(t, fx.deviceFixtureState)
	}
	response, err := fx.server.Client().Do(r)
	if err != nil {
		t.Fatal("actual local TLS signed request failed")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 65537))
	if err != nil || len(body) > 65536 || response.StatusCode != want || response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("ETag") != "" {
		t.Fatalf("private signed meter HTTP got%d want%d", response.StatusCode, want)
	}
	for _, private := range []string{fx.person.auth.Session.AccessToken, fx.person.auth.Session.FamilyID, fx.person.signer.keyID, fx.person.signer.fingerprint, fx.person.signer.contributor} {
		if strings.Contains(string(body), private) {
			t.Fatal("signed response disclosed proof material")
		}
	}
	if want >= 400 {
		if before != matrixPrivateState(t, fx.deviceFixtureState) {
			t.Fatal("refused signed request changed private business/ledger state")
		}
		var errorBody struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
			Acknowledgements []json.RawMessage `json:"acknowledgements"`
		}
		if json.Unmarshal(body, &errorBody) != nil || errorBody.Error.Code == "" || len(errorBody.Acknowledgements) != 0 {
			t.Fatal("fatal source refusal exposed buffered acknowledgements")
		}
		return protocol.Acknowledgement{}
	}
	var result struct {
		Version          int               `json:"schema_version"`
		Acknowledgements []json.RawMessage `json:"acknowledgements"`
	}
	if json.Unmarshal(body, &result) != nil || result.Version != 1 || len(result.Acknowledgements) != 1 {
		t.Fatal("signed operation returned invalid bounded response")
	}
	ack, err := protocol.ParseAcknowledgement(result.Acknowledgements[0], op)
	if err != nil || !bytes.Equal(ack.CanonicalBytes(), result.Acknowledgements[0]) {
		t.Fatal("signed response lost immutable canonical ACK", err)
	}
	return ack
}

func TestMeterIntakeDefaultRuntimeSignedTLSAuthorizationMatrix(t *testing.T) {
	roles, cells := matrixOracle(t)
	fx := meterIntakeHTTP(t)
	allowed, denied := 0, 0
	var first protocol.Operation
	var original protocol.Acknowledgement
	for _, role := range roles {
		t.Run(role, func(t *testing.T) {
			matrixAssign(t, fx.deviceFixtureState, matrixPerson{member: fx.person.member}, role, "ORG", "ACTIVE")
			op := fx.operation(t, "184190000", nil)
			if strings.HasPrefix(cells["meter.reading.record"][role], "O") {
				ack := fx.dispatch(t, op, http.StatusOK)
				if ack.Result() != protocol.Accepted || ack.Snapshot().EntityRevision == nil || *ack.Snapshot().EntityRevision != 1 {
					t.Fatal("current source-owned meter command was not accepted")
				}
				if allowed == 0 {
					first, original = op, ack
				}
				allowed++
			} else {
				fx.dispatch(t, op, http.StatusForbidden)
				denied++
			}
		})
	}
	if allowed != 4 || denied != 6 {
		t.Fatal("signed ordinary role applicability drift", allowed, denied)
	}
	for _, role := range []string{"DRIVER", "OPERATIONS_STAFF"} {
		t.Run(role+"-actual-self-unavailable", func(t *testing.T) {
			matrixAssign(t, fx.deviceFixtureState, matrixPerson{member: fx.person.member}, role, "SELF", "ACTIVE")
			fx.dispatch(t, fx.operation(t, "184190000", nil), http.StatusForbidden)
		})
	}
	matrixAssign(t, fx.deviceFixtureState, matrixPerson{member: fx.person.member}, "ADMIN", "ORG", "ACTIVE")
	replay := fx.dispatch(t, first, http.StatusOK)
	if !bytes.Equal(replay.CanonicalBytes(), original.CanonicalBytes()) {
		t.Fatal("actual cached receipt re-created or changed original ACK")
	}
	var exact bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT
 (SELECT count(*) FROM org_meter_readings WHERE org_id=$1)=5 AND
 (SELECT count(*) FROM org_sync_operations WHERE org_id=$1)=4 AND
 (SELECT count(*) FROM org_sync_acknowledgements WHERE org_id=$1)=4 AND
 (SELECT count(*) FROM org_sync_effects WHERE org_id=$1)=4 AND
 (SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND action='offl.operation.accepted')=4 AND
 (SELECT count(*) FROM org_event_intents WHERE org_id=$1)=0`, fx.scope.OrgID().String()).Scan(&exact); err != nil || !exact {
		t.Fatal("actual signed role requests duplicated/lost reading/audit/ACK/effect or invented async work", err)
	}
	matrixAssign(t, fx.deviceFixtureState, matrixPerson{member: fx.person.member}, "ADMIN", "ORG", "NONE")
	fx.dispatch(t, first, http.StatusForbidden)
}

func TestMeterIntakeDefaultRuntimeStrictRefusalsAndActualMissingParents(t *testing.T) {
	fx := meterIntakeHTTP(t)
	good := fx.operation(t, "184190000", nil)
	original := fx.dispatch(t, good, http.StatusOK)
	if original.Result() != protocol.Accepted {
		t.Fatal("actual runtime operation not accepted")
	}
	reuse := fx.operation(t, "184190001", func(raw map[string]any) {
		raw["operation_id"] = good.ID()
		raw["entity"].(map[string]any)["id"] = good.Snapshot().Entity.ID
	})
	if ack := fx.dispatch(t, reuse, http.StatusOK); ack.Result() != protocol.Rejected || ack.Code() != "OPERATION_ID_REUSED" {
		t.Fatal("changed immutable intent replaced original ACK")
	}
	if ack := fx.dispatch(t, good, http.StatusOK); !bytes.Equal(ack.CanonicalBytes(), original.CanonicalBytes()) {
		t.Fatal("refused changed intent mutated stored receipt")
	}
	for _, name := range []string{"extra-author", "future-schema"} {
		t.Run(name, func(t *testing.T) {
			op := fx.operation(t, "184190000", func(raw map[string]any) {
				if name == "extra-author" {
					raw["payload"].(map[string]any)["author_id"] = fx.member
				} else {
					raw["operation_schema"] = 2
					raw["payload"] = map[string]any{"future_payload": "must not be interpreted as schema1"}
				}
			})
			if ack := fx.dispatch(t, op, http.StatusOK); ack.Result() != protocol.Rejected || ack.Code() != "MALFORMED" {
				t.Fatal("strict or unknown schema did not retain bounded immutable refusal")
			}
		})
	}
	t.Run("before-actual-installation", func(t *testing.T) {
		op := fx.operation(t, "184190000", func(raw map[string]any) {
			raw["occurred_at"] = fx.installedAt.Add(-time.Nanosecond).UTC().Format(time.RFC3339Nano)
		})
		if ack := fx.dispatch(t, op, http.StatusOK); ack.Result() != protocol.Rejected || ack.Code() != "MALFORMED" {
			t.Fatal("known outside-service input retried instead of retaining bounded refusal")
		}
	})
	missing := fx.operation(t, "184190000", func(raw map[string]any) { raw["payload"].(map[string]any)["meter_id"] = onboardingID(t) })
	if ack := fx.dispatch(t, missing, http.StatusOK); ack.IsFinal() || ack.Code() != "DEPENDENCY_PENDING" {
		t.Fatal("missing covered meter became an effect or fabricated target")
	}
	if facts, audit, async := syncNoAsyncCounts(t, fx.syncFixtureState, missing); facts+audit+async != 0 {
		t.Fatal("unavailable target retained a new claim or private effect")
	}
	foreign := fx.operation(t, "184190000", func(raw map[string]any) { raw["payload"].(map[string]any)["asset_id"] = onboardingID(t) })
	fx.dispatch(t, foreign, http.StatusNotFound)
	var exact bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT
 (SELECT count(*) FROM org_meter_readings WHERE org_id=$1)=2 AND
 (SELECT count(*) FROM org_sync_operations WHERE org_id=$1)=4 AND
 (SELECT count(*) FROM org_sync_acknowledgements WHERE org_id=$1)=4 AND
 (SELECT count(*) FROM org_sync_effects WHERE org_id=$1)=1 AND
 (SELECT count(*) FROM org_event_intents WHERE org_id=$1)=0`, fx.scope.OrgID().String()).Scan(&exact); err != nil || !exact {
		t.Fatal("refused/missing operation created duplicate or partial facts", err)
	}
}
