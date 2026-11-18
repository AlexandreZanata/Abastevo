//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/storage"
	expenseapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/expense/application"
	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	fuelapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/application"
	identity "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	maintpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/adapters"
	maintapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/checklist"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	mediapg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/adapters"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgquota"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// EMP-OFFL-013 server half of the sync resilience suite (B-BR-EMP-540..543,
// BUC-EMP-277, EDR-351..353): the real default enterprise runtime (every
// default owner, nothing injected) behind the signed TLS intake on real
// PostGIS, driven through in-order and disrupted delivery of one field day of
// the real signed operations. Run alone with
// go test -race -count=1 -tags=integration -p 2 -run '^TestSyncResilience' ./internal/modules/organization/adapters/
// (the receipt test needs the disposable ANPFUEL_TEST_STORAGE_* bucket).

// resilienceState is the trip fixture (ADMIN device member with an assigned,
// metered, tanked vehicle) plus an ACTIVE DEPARTURE checklist template, a
// mission assigned to the member and the composed signed TLS runtime. Its
// transport can lose the next intake answer after the server committed.
type resilienceState struct {
	tripState
	m        missionState
	template maintapp.TemplateAck
	mission  string
	router   chi.Router
	server   *httptest.Server
	path     string
	mu       sync.Mutex
	lose     bool
	lost     [][]byte
}

func resilienceFixture(t *testing.T, documents *mediapg.StorageConfig) *resilienceState {
	t.Helper()
	x := &resilienceState{tripState: tripFixtureOn(t, selfIntakeFixture(t))}
	x.f.GrantReferenceFeed(t)
	tripReviewGrants(t, x.f)
	grantEmployeeTables(t, x.legalFixtureState)
	documentGrants(t, x.deviceFixtureState)
	fuelingGrants(t, x.legalFixtureState)
	grantChecklistTables(t, x.legalFixtureState)
	runGrants(t, x.f)
	role := pgx.Identifier{x.f.RuntimeRole}.Sanitize()
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_trip_events,org_trip_held_events,org_document_links TO "+role+
		";GRANT SELECT(org_id,employee_id,asset_id,valid_from,valid_to,end_cause) ON org_asset_assignments TO "+role+";GRANT INSERT ON org_meter_held_readings TO "+role+
		";GRANT SELECT ON org_work_orders TO "+role); err != nil {
		t.Fatal(err)
	}
	x.m = missionState{tripState: x.tripState, missions: missionStoreFor(t, x.legalFixtureState, x.tripState)}
	catalog, err := audit.NewCatalog(append(maintpg.ChecklistAuditDefinitions(), maintpg.RunAuditDefinitions()...))
	if err != nil {
		t.Fatal(err)
	}
	templates := &maintpg.ChecklistStore{Pool: x.f.Runtime, Authority: &orgpg.ChecklistAuthority{Memberships: x.legalFixtureState.store.Memberships, Roles: x.legalFixtureState.store.Roles,
		Capabilities: x.flags, Structure: &orgpg.ResourceStructure{Kind: orgpg.AssetStructure, Pool: x.f.Runtime, Allocations: x.assets},
		Assignee: &orgpg.ResourceAssignee{Pool: x.f.Runtime, Assignments: x.assets}}, Audit: auditpg.NewStore(x.f.Runtime, catalog)}
	if x.template, _, err = templateRun(t, x.legalFixtureState, templates, templateCommand(t, maintapp.TemplateCommandInput{Kind: maintapp.CommandCreate,
		Context: checklist.Departure, Name: "Checklist de saída sintético", Target: organizationTarget(), Items: runItems(), Policy: checklist.DefaultPolicy()})); err != nil {
		t.Fatal("departure template", err)
	}
	created := x.m.create(t, x.unit, x.vehicle.AssetID)
	if _, err := x.m.assignTo(t, created.MissionID, 1, x.person.employee); err != nil {
		t.Fatal("mission assignment", err)
	}
	x.mission = created.MissionID
	accounts := *x.accounts
	accounts.Store, accounts.Clock = accountpg.NewPGStore(x.f.Runtime), syncHTTPClock{}
	quota := identity.OperationQuota{Limit: 5000, Window: time.Hour}
	budget := orgquota.TransportDefinition{Class: "validation.resilience", Organization: quota, Account: quota, Device: quota,
		Rationale: "Explicit synthetic EMP-OFFL-013 resilience acceptance only: repeated resends of one field day stay below5000/hour; no production default."}
	x.router = chi.NewRouter()
	// No injected handler, store or plan: the real default runtime with every default owner.
	if err := enterprisesuite.Register(x.router, enterprisesuite.Options{Enabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{53}, 32),
		Pool: x.f.Runtime, Accounts: &accounts, IntakeBudget: &budget, DocumentStorage: documents}); err != nil {
		t.Fatal("default resilience runtime composition failed", err)
	}
	x.server = httptest.NewUnstartedServer(http.HandlerFunc(x.serve))
	x.server.StartTLS()
	t.Cleanup(x.server.Close)
	x.server.Client().Timeout = 30 * time.Second
	x.path = "/v1/orgs/" + x.scope.OrgID().String() + "/sync/operations"
	return x
}

// serve forwards to the composed router; an armed lost answer is computed and
// committed by the real intake, then its connection is closed before a byte
// of the answer leaves (the device never sees the acknowledgements).
func (x *resilienceState) serve(w http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(r.URL.Path, "/sync/operations") {
		x.router.ServeHTTP(w, r)
		return
	}
	answer := httptest.NewRecorder()
	x.router.ServeHTTP(answer, r)
	x.mu.Lock()
	lose := x.lose && answer.Code == http.StatusOK
	if lose {
		x.lose = false
		x.lost = append(x.lost, bytes.Clone(answer.Body.Bytes()))
	}
	x.mu.Unlock()
	if lose {
		if connection, _, err := w.(http.Hijacker).Hijack(); err == nil {
			_ = connection.Close()
		}
		return
	}
	for name, values := range answer.Header() {
		w.Header()[name] = values
	}
	w.WriteHeader(answer.Code)
	_, _ = w.Write(answer.Body.Bytes())
}

// signed is one signed intake request of fx's device for path, ready to send.
func (x *resilienceState) signed(t *testing.T, fx deviceFixtureState, path string, ops ...protocol.Operation) *http.Request {
	t.Helper()
	items := make([]json.RawMessage, len(ops))
	for i, op := range ops {
		items[i] = op.CanonicalBytes()
	}
	r := deviceSignedRequest(t, fx.registrar, fx.person.signer, path, syncHTTPBody(fx, items...), time.Time{})
	endpoint, err := url.Parse(x.server.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	r.URL, r.RequestURI, r.Host = endpoint, "", deviceTestHost
	return r
}

// send performs a prepared request and returns its status and, for 200, the
// canonical intent-bound acknowledgement bytes in operation order. A lost
// answer returns status 0.
func (x *resilienceState) send(t *testing.T, r *http.Request, ops ...protocol.Operation) (int, [][]byte) {
	t.Helper()
	response, err := x.server.Client().Do(r)
	if err != nil {
		return 0, nil
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 65537))
	if err != nil || len(body) > 65536 || response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("ETag") != "" {
		t.Fatal("signed intake answer unreadable or cacheable", response.StatusCode)
	}
	for _, private := range []string{x.person.auth.Session.AccessToken, x.person.auth.Session.FamilyID, x.person.signer.keyID, x.person.signer.fingerprint} {
		if strings.Contains(string(body), private) {
			t.Fatal("signed intake answer disclosed proof material")
		}
	}
	if response.StatusCode != http.StatusOK {
		if strings.Contains(string(body), "acknowledgements") {
			t.Fatal("a refused request exposed acknowledgements")
		}
		return response.StatusCode, nil
	}
	return http.StatusOK, x.acks(t, body, ops)
}

// acks decodes a complete answer and binds every acknowledgement to its operation.
func (x *resilienceState) acks(t *testing.T, body []byte, ops []protocol.Operation) [][]byte {
	t.Helper()
	var value struct {
		Version int               `json:"schema_version"`
		Items   []json.RawMessage `json:"acknowledgements"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil || value.Version != 1 || len(value.Items) != len(ops) {
		t.Fatal("signed intake answer is not one acknowledgement per operation", err)
	}
	out := make([][]byte, len(ops))
	for i, raw := range value.Items {
		ack, err := protocol.ParseAcknowledgement(raw, ops[i])
		if err != nil || !bytes.Equal(raw, ack.CanonicalBytes()) {
			t.Fatal("answer lost the canonical intent-bound acknowledgement", err)
		}
		out[i] = bytes.Clone(raw)
	}
	return out
}

// deliver signs and sends one batch of the member's own organization.
func (x *resilienceState) deliver(t *testing.T, ops ...protocol.Operation) (int, [][]byte) {
	t.Helper()
	return x.send(t, x.signed(t, x.deviceFixtureState, x.path, ops...), ops...)
}

// ok delivers and requires a complete 200 answer.
func (x *resilienceState) ok(t *testing.T, ops ...protocol.Operation) [][]byte {
	t.Helper()
	status, acks := x.deliver(t, ops...)
	if status != http.StatusOK {
		t.Fatal("signed intake refused the batch", status)
	}
	return acks
}

// loseNext delivers a batch whose answer is lost after the intake committed;
// it returns the lost acknowledgements as the server computed them.
func (x *resilienceState) loseNext(t *testing.T, ops ...protocol.Operation) [][]byte {
	t.Helper()
	x.mu.Lock()
	x.lose = true
	before := len(x.lost)
	x.mu.Unlock()
	if status, _ := x.deliver(t, ops...); status != 0 {
		t.Fatal("the armed answer reached the device", status)
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	if len(x.lost) != before+1 {
		t.Fatal("the lost answer was not committed by the intake")
	}
	return x.acks(t, x.lost[before], ops)
}

func ackOf(t *testing.T, raw []byte, op protocol.Operation) protocol.Acknowledgement {
	t.Helper()
	ack, err := protocol.ParseAcknowledgement(raw, op)
	if err != nil {
		t.Fatal(err)
	}
	return ack
}

// Positions of the field day's operations.
const (
	dayRespond = iota
	dayRun
	dayStart
	dayStop
	dayArrival
	dayReading
	dayFueling
	dayExpense
	dayFinish
	daySubmit
	daySize
)

var dayNames = [daySize]string{"respond", "run", "start", "stop", "arrival", "reading", "fueling", "expense", "finish", "submit"}

// fieldDay is BUC-EMP-277: the mission acceptance, the departure run, the
// start naming both, two logbook events, a reading, a fueling, a toll, the
// finish and the submission, each declared after the member's assignment.
func (x *resilienceState) fieldDay(t *testing.T) []protocol.Operation {
	t.Helper()
	d := x.syncFixtureState
	t0 := x.declaredNow(t)
	at := func(minutes int) time.Time { return t0.Add(time.Duration(minutes) * time.Minute) }
	ops := make([]protocol.Operation, daySize)
	ops[dayRespond] = x.m.respondOp(t, d, x.mission, 2, "ACCEPT", nil, nil)
	answers := []any{answer("OK", nil), answer("OK", nil), answer("OK", nil)}
	ops[dayRun] = syncHTTPOperation(t, d.deviceFixtureState, d.source.EnrollmentID, func(raw map[string]any) {
		raw["type"] = maintapp.RunOperationType
		raw["entity"] = map[string]any{"type": maintapp.RunEntityType, "id": onboardingID(t), "expected_revision": nil}
		raw["occurred_at"], raw["entry_zone"], raw["device_offset_seconds"] = orgtime.FormatInstant(at(1)), "America/Cuiaba", -14400
		raw["payload"] = map[string]any{"asset_id": x.vehicle.AssetID, "context": "DEPARTURE", "template_id": x.template.TemplateID, "template_version": x.template.Version,
			"policy_revision": x.template.PolicyRevision, "started_at": orgtime.FormatInstant(at(-4).Truncate(time.Microsecond)), "answers": answers,
			"device_outcome": "COMPLETED_OK", "acknowledged": false}
	})
	ops[dayStart] = x.startOp(t, d, at(10), func(e, _ map[string]any) {
		e["soft_references"] = []any{map[string]any{"type": tripapp.MissionReferenceType, "id": x.mission},
			map[string]any{"type": tripapp.ChecklistRunReferenceType, "id": ops[dayRun].Snapshot().Entity.ID}}
	})
	trip := tripOf(ops[dayStart])
	event := func(kind string, minutes int) protocol.Operation {
		return x.operation(t, d, tripapp.EventOperationType, trip, at(minutes), map[string]any{"trip_id": trip, "type": kind, "text": nil}, func(e, _ map[string]any) {
			e["entity"] = map[string]any{"type": tripapp.EventEntityType, "id": onboardingID(t), "expected_revision": nil}
			e["depends_on"] = []string{ops[dayStart].ID()}
		})
	}
	ops[dayStop], ops[dayArrival] = event("STOP", 40), event("ARRIVAL", 60)
	ops[dayReading] = syncHTTPOperation(t, d.deviceFixtureState, d.source.EnrollmentID, func(raw map[string]any) {
		raw["type"] = fleetpg.MeterReadingOperationType
		raw["entity"] = map[string]any{"type": "meter_reading", "id": onboardingID(t), "expected_revision": nil}
		raw["occurred_at"], raw["entry_zone"], raw["device_offset_seconds"] = orgtime.FormatInstant(at(90)), "America/Cuiaba", -14400
		raw["payload"] = map[string]any{"asset_id": x.vehicle.AssetID, "meter_id": x.vehicle.MeterID, "segment_id": x.vehicle.SegmentID, "kind": "ODOMETER", "value": "184400000", "method": "TYPED"}
	})
	ops[dayFueling] = syncHTTPOperation(t, d.deviceFixtureState, d.source.EnrollmentID, func(raw map[string]any) {
		raw["type"] = fuelapp.OperationType
		raw["entity"] = map[string]any{"type": fuelapp.EntityType, "id": onboardingID(t), "expected_revision": nil}
		raw["occurred_at"], raw["entry_zone"], raw["device_offset_seconds"] = orgtime.FormatInstant(at(120)), "America/Cuiaba", -14400
		raw["payload"] = map[string]any{"asset_id": x.vehicle.AssetID, "responsible_employee_id": d.person.employee, "product": "DIESEL_S10", "unit": "L",
			"quantity": "50000", "paid_total": "32450", "discount": nil, "unit_price": "6490", "unit_price_text": nil, "fill": "PARTIAL", "payer": "COMPANY", "payment_method": "CASH",
			"reading":                map[string]any{"meter_id": x.vehicle.MeterID, "segment_id": x.vehicle.SegmentID, "reading_id": onboardingID(t), "kind": "ODOMETER", "value": "184532000"},
			"missing_reading_reason": nil, "place": nil, "trip_id": nil, "contracted_supplier_id": nil}
	})
	ops[dayExpense] = syncHTTPOperation(t, d.deviceFixtureState, d.source.EnrollmentID, func(raw map[string]any) {
		raw["type"] = expenseapp.OperationType
		raw["entity"] = map[string]any{"type": expenseapp.EntityType, "id": onboardingID(t), "expected_revision": nil}
		raw["occurred_at"], raw["entry_zone"], raw["device_offset_seconds"] = orgtime.FormatInstant(at(135)), "America/Cuiaba", -14400
		raw["payload"] = map[string]any{"asset_id": x.vehicle.AssetID, "category": "TOLL", "paid_total": "1250", "company_amount": "1250", "payer": "EMPLOYEE", "trip_id": nil}
	})
	ops[dayFinish] = x.finishOp(t, d, trip, at(180), func(e, _ map[string]any) { e["depends_on"] = []string{ops[dayStart].ID()} })
	ops[daySubmit] = x.operation(t, d, tripapp.SubmitOperationType, trip, at(210), map[string]any{}, func(e, _ map[string]any) {
		e["entity"] = map[string]any{"type": tripapp.ReviewEntityType, "id": trip, "expected_revision": 1}
		e["depends_on"] = []string{ops[dayFinish].ID()}
	})
	return ops
}

// pick returns the operations at the given positions, in that order.
func pick(ops []protocol.Operation, positions ...int) []protocol.Operation {
	out := make([]protocol.Operation, len(positions))
	for i, p := range positions {
		out[i] = ops[p]
	}
	return out
}

// deviceOrder is the queue order of a device; disruptedOrder reverses every
// declared dependency while each soft-reference target still precedes the
// operation naming it (EDR-352: the device holds such an operation until its
// target's final receipt).
var deviceOrder = []int{dayRespond, dayRun, dayStart, dayStop, dayArrival, dayReading, dayFueling, dayExpense, dayFinish, daySubmit}
var disruptedOrder = []int{daySubmit, dayFinish, dayArrival, dayStop, dayExpense, dayFueling, dayReading, dayRespond, dayRun, dayStart}

// opCounts are the claims, final receipts, effects and audit events of one operation.
type opCounts struct{ claims, receipts, effects, audits int }

func (x *resilienceState) counts(t *testing.T, scopeOrg string, op protocol.Operation) opCounts {
	t.Helper()
	var c opCounts
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM org_sync_operations WHERE org_id=$1 AND operation_id=$2),
 (SELECT count(*) FROM org_sync_acknowledgements WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_sync_effects WHERE org_id=$1 AND operation_id=$2),
 (SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND operation_id=$2)`, scopeOrg, op.ID()).Scan(&c.claims, &c.receipts, &c.effects, &c.audits); err != nil {
		t.Fatal(err)
	}
	return c
}

// signature is the EDR-353 effect signature: per position the final result,
// code, revision and review presence with the operation's claims, receipts,
// effects and audit events, then every owner's stored effects, without server
// times or generated identifiers.
func (x *resilienceState) signature(t *testing.T, ops []protocol.Operation, final [][]byte) string {
	t.Helper()
	org := x.scope.OrgID().String()
	var b strings.Builder
	for i, op := range ops {
		ack := ackOf(t, final[i], op)
		revision := "-"
		if r := ack.Snapshot().EntityRevision; r != nil {
			revision = fmt.Sprint(*r)
		}
		fmt.Fprintf(&b, "%s=%s/%s/%s/review:%t/%+v\n", dayNames[i], ack.Result(), ack.Code(), revision, ack.Snapshot().ReviewItemID != "", x.counts(t, org, op))
	}
	b.WriteString(x.effects(t))
	return b.String()
}

// effects is the organization's stored state of every owner (EDR-353).
func (x *resilienceState) effects(t *testing.T) string {
	t.Helper()
	org := x.scope.OrgID().String()
	var effects string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT concat_ws(E'\n',
 'trips:'||COALESCE((SELECT string_agg(state,',' ORDER BY state) FROM org_trips WHERE org_id=$1),''),
 'events:'||(SELECT count(*) FROM org_trip_events WHERE org_id=$1),
 'finishes:'||(SELECT count(*) FROM org_trip_finishes WHERE org_id=$1),
 'reviews:'||COALESCE((SELECT string_agg(kind||':'||state||':'||revision,',' ORDER BY sequence) FROM org_trip_review_entries WHERE org_id=$1),''),
 'readings:'||COALESCE((SELECT string_agg(origin||':'||value||':'||state,',' ORDER BY value,origin) FROM org_meter_readings WHERE org_id=$1),''),
 'fuelings:'||COALESCE((SELECT string_agg(paid_centavos::text,',') FROM org_fuelings WHERE org_id=$1),''),
 'expenses:'||COALESCE((SELECT string_agg(origin_kind||':'||category||':'||paid_centavos||':'||company_centavos,',' ORDER BY origin_kind,category) FROM org_expenses WHERE org_id=$1),''),
 'runs:'||COALESCE((SELECT string_agg(outcome,',') FROM org_checklist_runs WHERE org_id=$1),''),
 'checklist links:'||COALESCE((SELECT string_agg(context,',' ORDER BY context) FROM org_trip_checklist_links WHERE org_id=$1),''),
 'mission links:'||(SELECT count(*) FROM org_trip_mission_links WHERE org_id=$1),
 'missions:'||COALESCE((SELECT string_agg(state,',') FROM org_trip_missions WHERE org_id=$1),''),
 'review items:'||(SELECT count(*) FROM org_sync_review_items WHERE org_id=$1),
 'held:'||(SELECT count(*) FROM org_trip_held_starts WHERE org_id=$1)||'/'||(SELECT count(*) FROM org_trip_held_finishes WHERE org_id=$1)||'/'||
   (SELECT count(*) FROM org_trip_held_events WHERE org_id=$1)||'/'||(SELECT count(*) FROM org_trip_held_submissions WHERE org_id=$1)||'/'||
   (SELECT count(*) FROM org_meter_held_readings WHERE org_id=$1),
 'ledger:'||(SELECT count(*) FROM org_sync_operations WHERE org_id=$1)||'/'||(SELECT count(*) FROM org_sync_acknowledgements WHERE org_id=$1)||'/'||
   (SELECT count(*) FROM org_sync_effects WHERE org_id=$1)||'/'||(SELECT count(*) FROM org_audit_events WHERE org_id=$1))`, org).Scan(&effects); err != nil {
		t.Fatal(err)
	}
	return effects
}

// inOrder delivers the field day once in device order and returns the final
// acknowledgements by position (every one ACCEPTED: the reference state).
func (x *resilienceState) inOrder(t *testing.T, ops []protocol.Operation) [][]byte {
	t.Helper()
	acks := x.ok(t, pick(ops, deviceOrder...)...)
	final := make([][]byte, daySize)
	for i, p := range deviceOrder {
		ack := ackOf(t, acks[i], ops[p])
		if ack.Result() != protocol.Accepted {
			t.Fatalf("in-order %s was %s %s", dayNames[p], ack.Result(), ack.Code())
		}
		final[p] = acks[i]
	}
	return final
}

// byPosition maps an answer for ops at positions back to field-day positions.
func byPosition(positions []int, acks [][]byte) map[int][]byte {
	out := map[int][]byte{}
	for i, p := range positions {
		out[p] = acks[i]
	}
	return out
}

// BUC-EMP-277, B-BR-EMP-540/541, EDR-353: the field day delivered in device
// order is the reference state; delivered with a lost answer, the device's
// restart and whole-queue resend, every dependency reversed and duplicate
// sequential and concurrent resends, it converges on the same signature.
func TestSyncResilienceFieldDayConvergesOnTheInOrderState(t *testing.T) {
	var reference string
	t.Run("in order", func(t *testing.T) {
		x := resilienceFixture(t, nil)
		ops := x.fieldDay(t)
		final := x.inOrder(t, ops)
		reference = x.signature(t, ops, final)
		t.Logf("in-order reference state:\n%s", reference)
		for _, want := range []string{"trips:COMPLETED\n", "events:2\n", "finishes:1\n", "reviews:SUBMIT:SUBMITTED:1\n", "TRIP_START:184320000:", "STANDALONE:184400000:",
			"FUELING:184532000:", "TRIP_END:184610000:", "fuelings:32450\n", "expenses:DIRECT:TOLL:1250:1250,FUELING:FUEL:32450:32450\n",
			"checklist links:DEPARTURE\n", "mission links:1\n", "missions:COMPLETED\n", "review items:0\n", "held:0/0/0/0/0\n"} {
			if !strings.Contains(reference, want) {
				t.Fatalf("in-order state lacks %q:\n%s", want, reference)
			}
		}
	})
	if reference == "" {
		t.Fatal("no in-order reference state")
	}
	t.Run("lost response, killed process and wrong dependency order", func(t *testing.T) {
		x := resilienceFixture(t, nil)
		ops := x.fieldDay(t)
		queue := pick(ops, disruptedOrder...)
		// Lost response: the server commits what it can decide; the device never reads the answer.
		lost := byPosition(disruptedOrder, x.loseNext(t, queue...))
		for _, p := range []int{daySubmit, dayFinish, dayArrival, dayStop} {
			if ack := ackOf(t, lost[p], ops[p]); ack.Result() != protocol.Retry || ack.Code() != "DEPENDENCY_PENDING" {
				t.Fatalf("wrong dependency order: %s first was %s %s", dayNames[p], ack.Result(), ack.Code())
			}
			if c := x.counts(t, x.scope.OrgID().String(), ops[p]); c.receipts != 0 || c.effects != 0 || c.audits != 0 {
				t.Fatalf("a RETRY %s left a receipt, effect or audit: %+v", dayNames[p], c)
			}
		}
		for _, p := range []int{dayExpense, dayFueling, dayReading, dayRespond, dayRun, dayStart} {
			if ack := ackOf(t, lost[p], ops[p]); ack.Result() != protocol.Accepted {
				t.Fatalf("independent %s was %s %s", dayNames[p], ack.Result(), ack.Code())
			}
		}
		// Killed process: the restarted device resends its whole durable queue; committed work replays byte for byte.
		resent := byPosition(disruptedOrder, x.ok(t, queue...))
		for p, raw := range lost {
			if ackOf(t, raw, ops[p]).IsFinal() && !bytes.Equal(raw, resent[p]) {
				t.Fatalf("the resend after a lost answer changed the stored %s acknowledgement", dayNames[p])
			}
		}
		if ack := ackOf(t, resent[daySubmit], ops[daySubmit]); ack.Result() != protocol.Retry || ack.Code() != "DEPENDENCY_PENDING" {
			t.Fatal("the submission ahead of its finish in the same resend was not retried", ack.Result(), ack.Code())
		}
		// The device keeps only the submission pending and converges.
		final := map[int][]byte{}
		for p, raw := range resent {
			final[p] = raw
		}
		final[daySubmit] = x.ok(t, ops[daySubmit])[0]
		// Duplicate effects: the whole queue three more times in both orders, every answer byte-identical.
		for _, order := range [][]int{disruptedOrder, deviceOrder, disruptedOrder} {
			again := byPosition(order, x.ok(t, pick(ops, order...)...))
			for p := range final {
				if !bytes.Equal(final[p], again[p]) {
					t.Fatalf("duplicate delivery changed the %s acknowledgement", dayNames[p])
				}
			}
		}
		positional := make([][]byte, daySize)
		for p := range positional {
			positional[p] = final[p]
		}
		if got := x.signature(t, ops, positional); got != reference {
			t.Fatalf("disrupted delivery diverged from the in-order state:\n%s\nwant\n%s", got, reference)
		}
		// Same identifier, other digest: refused, nothing stored, the original untouched.
		for p, op := range ops {
			changed := syncHTTPOperation(t, x.deviceFixtureState, x.source.EnrollmentID, func(raw map[string]any) {
				decoder := json.NewDecoder(bytes.NewReader(op.CanonicalBytes()))
				decoder.UseNumber()
				if err := decoder.Decode(&raw); err != nil {
					t.Fatal(err)
				}
				// The stored canonical intent carries its digest; a new seal computes the other one.
				delete(raw, "payload_digest")
				raw["occurred_at"] = orgtime.FormatInstant(x.declaredNow(t).Add(5 * time.Hour))
			})
			if ack := ackOf(t, x.ok(t, changed)[0], changed); ack.Result() != protocol.Rejected || ack.Code() != "OPERATION_ID_REUSED" {
				t.Fatalf("reused %s identifier was %s %s", dayNames[p], ack.Result(), ack.Code())
			}
		}
		if got := x.signature(t, ops, positional); got != reference {
			t.Fatalf("a reused identifier changed the stored state:\n%s", got)
		}
		for p, op := range ops {
			if replay := x.ok(t, op)[0]; !bytes.Equal(replay, positional[p]) {
				t.Fatalf("the original %s acknowledgement changed after a reuse attempt", dayNames[p])
			}
		}
	})
	t.Run("concurrent duplicates in both orders", func(t *testing.T) {
		x := resilienceFixture(t, nil)
		ops := x.fieldDay(t)
		// Six copies, three in device order and three with every dependency reversed, signed first and released
		// together behind the held organization root that every intake transaction locks.
		type prepared struct {
			order []int
			r     *http.Request
		}
		copies := make([]prepared, 6)
		for i := range copies {
			order := deviceOrder
			if i%2 == 1 {
				order = disruptedOrder
			}
			copies[i] = prepared{order, x.signed(t, x.deviceFixtureState, x.path, pick(ops, order...)...)}
		}
		root, err := x.f.Admin.Begin(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := root.Exec(t.Context(), `SELECT 1 FROM organizations WHERE org_id=$1 FOR UPDATE`, x.scope.OrgID().String()); err != nil {
			t.Fatal(err)
		}
		answers := make([]map[int][]byte, len(copies))
		statuses := make([]int, len(copies))
		start := make(chan struct{})
		var done sync.WaitGroup
		for i, c := range copies {
			done.Add(1)
			go func() {
				defer done.Done()
				<-start
				response, err := x.server.Client().Do(c.r)
				if err != nil {
					return
				}
				defer response.Body.Close()
				body, _ := io.ReadAll(io.LimitReader(response.Body, 65537))
				statuses[i] = response.StatusCode
				if response.StatusCode == http.StatusOK {
					var value struct {
						Items []json.RawMessage `json:"acknowledgements"`
					}
					if json.Unmarshal(body, &value) == nil && len(value.Items) == len(c.order) {
						answers[i] = map[int][]byte{}
						for j, p := range c.order {
							answers[i][p] = bytes.Clone(value.Items[j])
						}
					}
				}
			}()
		}
		close(start)
		waitRuntimeBlocked(t, x.legalFixtureState, len(copies))
		_ = root.Rollback(context.Background())
		done.Wait()
		final := byPosition(deviceOrder, x.ok(t, pick(ops, deviceOrder...)...))
		final[daySubmit] = x.ok(t, ops[daySubmit])[0]
		for i, answer := range answers {
			if statuses[i] != http.StatusOK || answer == nil {
				t.Fatalf("concurrent copy %d was not answered completely: %d", i, statuses[i])
			}
			for p, raw := range answer {
				ack := ackOf(t, raw, ops[p])
				if ack.IsFinal() && !bytes.Equal(raw, final[p]) {
					t.Fatalf("concurrent copy %d returned another final %s acknowledgement", i, dayNames[p])
				}
				if !ack.IsFinal() && (ack.Result() != protocol.Retry || ack.Code() != "DEPENDENCY_PENDING" && ack.Code() != "UNAVAILABLE") {
					t.Fatalf("concurrent copy %d answered %s %s %s", i, dayNames[p], ack.Result(), ack.Code())
				}
			}
		}
		positional := make([][]byte, daySize)
		for p := range positional {
			positional[p] = final[p]
		}
		if got := x.signature(t, ops, positional); got != reference {
			t.Fatalf("concurrent duplicates diverged from the in-order state:\n%s\nwant\n%s", got, reference)
		}
	})
}

// BR-OFFL-007, EDR-352/353: a soft reference delivered before its target is
// the named divergence the device prevents: the server holds the start
// ORPHAN_REFERENCE for review with nothing linked, and a later run or
// mission acceptance never links it.
func TestSyncResilienceSoftReferenceBeforeItsTargetIsHeldNotLinked(t *testing.T) {
	x := resilienceFixture(t, nil)
	ops := x.fieldDay(t)
	acks := byPosition([]int{dayStart, dayRun, dayRespond}, x.ok(t, ops[dayStart], ops[dayRun], ops[dayRespond]))
	start := ackOf(t, acks[dayStart], ops[dayStart])
	if start.Result() != protocol.AcceptedForReview || start.Snapshot().ReviewItemID == "" {
		t.Fatal("a start naming targets not stored yet was not held for review", start.Result(), start.Code())
	}
	// EDR-332/333: a named run that does not count holds ORPHAN_REFERENCE (the stored reason says the checklist is required).
	if code := start.Code(); code != "ORPHAN_REFERENCE" {
		t.Fatal("unexpected held code for the early start", code)
	}
	for _, p := range []int{dayRun, dayRespond} {
		if ack := ackOf(t, acks[p], ops[p]); ack.Result() != protocol.Accepted {
			t.Fatal("the late target was refused", dayNames[p], ack.Result(), ack.Code())
		}
	}
	var trips, links, missionLinks, held int
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM org_trips WHERE org_id=$1),(SELECT count(*) FROM org_trip_checklist_links WHERE org_id=$1),
 (SELECT count(*) FROM org_trip_mission_links WHERE org_id=$1),(SELECT count(*) FROM org_trip_held_starts WHERE org_id=$1 AND operation_id=$2)`,
		x.scope.OrgID().String(), ops[dayStart].ID()).Scan(&trips, &links, &missionLinks, &held); err != nil {
		t.Fatal(err)
	}
	if trips != 0 || links != 0 || missionLinks != 0 || held != 1 {
		t.Fatal("an early soft reference was applied or linked", trips, links, missionLinks, held)
	}
	if replay := x.ok(t, ops[dayStart])[0]; !bytes.Equal(replay, acks[dayStart]) {
		t.Fatal("a held start was re-decided after its targets arrived")
	}
}

// B-BR-EMP-540/542: a RETRY answer stores no acknowledgement and a later
// delivery decides afresh; a whole-request refusal stores nothing and the
// device's resend applies once.
func TestSyncResilienceMissingAcknowledgementStoresNothingAndRedecides(t *testing.T) {
	x := resilienceFixture(t, nil)
	ops := x.fieldDay(t)
	org := x.scope.OrgID().String()
	early := x.ok(t, ops[dayStop], ops[dayFinish])
	for i, p := range []int{dayStop, dayFinish} {
		if ack := ackOf(t, early[i], ops[p]); ack.Result() != protocol.Retry || ack.Code() != "DEPENDENCY_PENDING" || ack.IsFinal() {
			t.Fatal("a dependent operation before its start was not retried", dayNames[p], ack.Result(), ack.Code())
		}
		if c := x.counts(t, org, ops[p]); c.receipts != 0 || c.effects != 0 || c.audits != 0 {
			t.Fatalf("a RETRY answer stored a receipt, effect or audit: %+v", c)
		}
	}
	x.ok(t, ops[dayRespond], ops[dayRun], ops[dayStart])
	later := x.ok(t, ops[dayStop], ops[dayFinish])
	for i, p := range []int{dayStop, dayFinish} {
		if ack := ackOf(t, later[i], ops[p]); ack.Result() != protocol.Accepted {
			t.Fatal("the redelivered dependent operation was not decided afresh", dayNames[p], ack.Result(), ack.Code())
		}
		if c := x.counts(t, org, ops[p]); c != (opCounts{1, 1, 1, 1}) {
			t.Fatalf("the redelivered %s was not stored exactly once: %+v", dayNames[p], c)
		}
	}
	// A whole-request refusal: a foreign enrollment inside the batch, an expired signature and another
	// organization's route answer no acknowledgement and store nothing; the resend applies once.
	before := x.effects(t)
	foreign := syncHTTPOperation(t, x.deviceFixtureState, onboardingID(t), nil)
	if status, acks := x.deliver(t, ops[dayFueling], foreign); status < 400 || acks != nil {
		t.Fatal("a batch with a foreign binding was not refused as a whole", status)
	}
	expired := deviceSignedRequest(t, x.registrar, x.person.signer, x.path, syncHTTPBody(x.deviceFixtureState, ops[dayFueling].CanonicalBytes()), time.Now().Add(-time.Second))
	endpoint, err := url.Parse(x.server.URL + x.path)
	if err != nil {
		t.Fatal(err)
	}
	expired.URL, expired.RequestURI, expired.Host = endpoint, "", deviceTestHost
	if status, acks := x.send(t, expired, ops[dayFueling]); status < 400 || acks != nil {
		t.Fatal("an expired signed request was answered", status)
	}
	if after := x.effects(t); after != before {
		t.Fatalf("a refused request changed the stored state:\n%s\nwant\n%s", after, before)
	}
	first := x.ok(t, ops[dayFueling])[0]
	if ack := ackOf(t, first, ops[dayFueling]); ack.Result() != protocol.Accepted {
		t.Fatal("the fueling resent after the refusals was not accepted", ack.Result(), ack.Code())
	}
	if replay := x.ok(t, ops[dayFueling])[0]; !bytes.Equal(replay, first) || x.counts(t, org, ops[dayFueling]) != (opCounts{1, 1, 1, 1}) {
		t.Fatal("the fueling was not stored exactly once")
	}
}

// B-BR-EMP-543: a truncated or altered receipt transfer is refused and keeps
// the document UPLOADING at its revision; only a retry with exactly the
// declared size and SHA-256 stores it; a repeat after STORED stores nothing.
// Both C1 receipt owners (FUELING and EXPENSE) through the real ingress.
func TestSyncResilienceTruncatedReceiptUploadsRecoverOnlyWithTheDeclaredBytes(t *testing.T) {
	required := func(name string) string {
		t.Helper()
		value := os.Getenv(name)
		if value == "" {
			t.Fatalf("explicit disposable %s required", name)
		}
		return value
	}
	documents := mediapg.StorageConfig{Endpoint: required("ANPFUEL_TEST_STORAGE_ENDPOINT"), Bucket: required("ANPFUEL_TEST_STORAGE_BUCKET"), Region: required("ANPFUEL_TEST_STORAGE_REGION"),
		Credentials: storage.Credentials{AccessKeyID: required("ANPFUEL_TEST_STORAGE_ACCESS_KEY_ID"), SecretAccessKey: required("ANPFUEL_TEST_STORAGE_SECRET_ACCESS_KEY")}, AllowLoopback: true}
	x := resilienceFixture(t, &documents)
	raw, err := os.ReadFile("../../evidence/adapters/media/testdata/valid-4x4.jpg")
	if err != nil {
		t.Fatal(err)
	}
	marker := []byte("SYNTHETIC-RESILIENCE-RECEIPT-ORIGINAL")
	raw = append(append([]byte{0xff, 0xd8, 0xff, 0xe1, 0, byte(len(marker) + 2)}, marker...), raw[2:]...)
	org := x.scope.OrgID().String()
	fx := attachmentHTTPFixture{meterIntakeHTTPFixture: meterIntakeHTTPFixture{syncFixtureState: x.syncFixtureState, server: x.server, path: x.path}, router: x.router, raw: raw, org: org}
	ops := x.fieldDay(t)
	for i, ack := range x.ok(t, ops[dayFueling], ops[dayExpense]) {
		if ackOf(t, ack, []protocol.Operation{ops[dayFueling], ops[dayExpense]}[i]).Result() != protocol.Accepted {
			t.Fatal("receipt owner not accepted")
		}
	}
	events := `SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND target_id=$2 AND action=$3`
	for _, target := range []struct{ kind, id string }{{"FUELING", ops[dayFueling].Snapshot().Entity.ID}, {"EXPENSE", ops[dayExpense].Snapshot().Entity.ID}} {
		body := fx.declaration(t, raw, target.kind, target.id)
		document := body["document_id"].(string)
		status, opened, code := fx.session(t, "", body)
		if status != 200 || opened.Storage != "UPLOADING" || opened.Upload == nil {
			t.Fatal("receipt session", target.kind, status, code)
		}
		capability := opened.Upload.Capability
		altered := append([]byte(nil), raw...)
		altered[len(altered)/2] ^= 0xff
		for name, attempt := range map[string]func() int{
			"truncated": func() int {
				s, _, _ := fx.upload(t, org, document, capability, bytes.NewReader(raw[:len(raw)/2]), int64(len(raw)))
				return s
			},
			"altered": func() int {
				s, _, _ := fx.upload(t, org, document, capability, bytes.NewReader(altered), int64(len(altered)))
				return s
			},
			"interrupted": func() int {
				s, _, _ := fx.upload(t, org, document, capability, &failingReader{data: raw}, int64(len(raw)))
				return s
			},
		} {
			if s := attempt(); s < 400 {
				t.Fatal("a partial receipt transfer was accepted", target.kind, name, s)
			}
			if state, revision := fx.storage(t, document); state != "UPLOADING" || revision != 1 {
				t.Fatal("a refused transfer changed the document", target.kind, name, state, revision)
			}
		}
		// The device retries with a fresh session (its capability expired with the process) and the declared bytes.
		status, renewed, _ := fx.session(t, "", again(t, body))
		if status != 200 || renewed.Upload == nil || fx.count(t, events, org, document, "document.upload.created") != 1 {
			t.Fatal("the retry session wrote again or offered no capability", target.kind, status)
		}
		if s, _, c := fx.upload(t, org, document, renewed.Upload.Capability, bytes.NewReader(raw), int64(len(raw))); s != 200 {
			t.Fatal("the declared bytes were refused on retry", target.kind, s, c)
		}
		var digest string
		var size int64
		if err := x.f.Admin.QueryRow(t.Context(), `SELECT original_sha256,original_bytes FROM org_documents WHERE org_id=$1 AND id=$2 AND storage_state='STORED'`, org, document).Scan(&digest, &size); err != nil ||
			digest != body["sha256"] || size != int64(len(raw)) {
			t.Fatal("the stored receipt differs from its declaration", target.kind, err)
		}
		if s, replay, _ := fx.upload(t, org, document, renewed.Upload.Capability, bytes.NewReader(raw), int64(len(raw))); s != 200 || replay["replayed"] != true {
			t.Fatal("a repeat after STORED was not a replay", target.kind, s)
		}
		if fx.count(t, events, org, document, "document.upload.received") != 1 || fx.count(t, events, org, document, "document.upload.stored") != 1 ||
			fx.count(t, `SELECT count(*) FROM org_document_links WHERE org_id=$1 AND document_id=$2 AND target_kind=$3 AND target_id=$4`, org, document, target.kind, target.id) != 1 {
			t.Fatal("the receipt was received, stored or linked more than once", target.kind)
		}
	}
	if fx.count(t, `SELECT count(*) FROM org_documents WHERE org_id=$1`, org) != 2 {
		t.Fatal("partial transfers created extra documents")
	}
}

// B-BR-EMP-543: organization B of the same account never receives, applies
// or reveals A's state, whatever reaches its route.
func TestSyncResilienceCrossScopeStateNeverApplies(t *testing.T) {
	x := resilienceFixture(t, nil)
	ops := x.fieldDay(t)
	finalA := x.inOrder(t, ops)
	other := legalFixtureFor(t, x.f, x.onboarding, x.actor, x.flags)
	employee, member := teamDriver(t, other, x.person.auth.Account.ID)
	if _, _, err := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	b := x.syncFixtureState
	b.deviceFixtureState.legalFixtureState = other
	b.deviceFixtureState.person.employee, b.deviceFixtureState.person.member = employee, member
	matrixAssign(t, b.deviceFixtureState, matrixPerson{member: member}, "ADMIN", "ORG", "ACTIVE")
	actor := deviceVerifyActor(t, b.deviceFixtureState, b.person, b.person.signer, time.Time{})
	enrolled, err := x.devices.EnrollDevice(t.Context(), other.scope, actor, deviceEnrollCommand(t, "Synthetic second organization phone"))
	if err != nil {
		t.Fatal(err)
	}
	registration, err := enrolled.Device.Enrollment.Snapshot(other.scope)
	if err != nil {
		t.Fatal(err)
	}
	b.source = syncapp.Source{Scope: other.scope, Origin: x.source.Origin, Actor: actor, MembershipID: member, EnrollmentID: registration.ID}
	var unitB string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1`, other.scope.OrgID().String()).Scan(&unitB); err != nil {
		t.Fatal(err)
	}
	assetB, _ := lifecycleAssetIn(t, other, assignmentStore(t, other), unitB, fleet.Active)
	orgA, orgB := x.scope.OrgID().String(), other.scope.OrgID().String()
	pathB := "/v1/orgs/" + orgB + "/sync/operations"
	before := x.signature(t, ops, finalA)
	foreignState := func() string {
		t.Helper()
		var state string
		if err := x.f.Admin.QueryRow(t.Context(), `SELECT concat_ws('/',(SELECT count(*) FROM org_sync_operations WHERE org_id=$1),(SELECT count(*) FROM org_sync_acknowledgements WHERE org_id=$1),
 (SELECT count(*) FROM org_trips WHERE org_id=$1),(SELECT count(*) FROM org_fuelings WHERE org_id=$1),(SELECT count(*) FROM org_expenses WHERE org_id=$1),
 (SELECT count(*) FROM org_checklist_runs WHERE org_id=$1),(SELECT count(*) FROM org_meter_readings WHERE org_id=$1),(SELECT count(*) FROM org_sync_review_items WHERE org_id=$1))`, orgB).Scan(&state); err != nil {
			t.Fatal(err)
		}
		return state
	}
	empty := foreignState()
	// A's whole field day posted to B's route is refused as a whole and stores nothing in either organization.
	if status, acks := x.send(t, x.signed(t, x.deviceFixtureState, pathB, ops...), ops...); status < 400 || acks != nil {
		t.Fatal("A's operations were answered on B's route", status)
	}
	// B's own operation carrying A's start identifier gets B's own receipt.
	colliding := syncHTTPOperation(t, b.deviceFixtureState, registration.ID, func(raw map[string]any) {
		raw["operation_id"] = ops[dayStart].ID()
		raw["type"] = expenseapp.OperationType
		raw["entity"] = map[string]any{"type": expenseapp.EntityType, "id": ops[dayExpense].Snapshot().Entity.ID, "expected_revision": nil}
		raw["occurred_at"] = orgtime.FormatInstant(x.declaredNow(t))
		raw["payload"] = map[string]any{"asset_id": assetB, "category": "PARKING", "paid_total": "800", "company_amount": "640", "payer": "COMPANY", "trip_id": nil}
	})
	// B's operation posted to A's route is refused as a whole too.
	if status, acks := x.send(t, x.signed(t, b.deviceFixtureState, x.path, colliding), colliding); status < 400 || acks != nil {
		t.Fatal("B's operation was answered on A's route", status)
	}
	if foreignState() != empty || x.signature(t, ops, finalA) != before {
		t.Fatal("a refused cross-organization request stored something")
	}
	status, acks := x.send(t, x.signed(t, b.deviceFixtureState, pathB, colliding), colliding)
	if status != http.StatusOK {
		t.Fatal("B's own operation was refused", status)
	}
	ackB := ackOf(t, acks[0], colliding)
	if ackB.Result() != protocol.Accepted || bytes.Equal(acks[0], finalA[dayStart]) {
		t.Fatal("B's colliding identifier was not decided as B's own", ackB.Result(), ackB.Code())
	}
	if state := foreignState(); state != "1/1/0/0/1/0/0/0" {
		t.Fatal("B holds more than its own expense", state)
	}
	if x.signature(t, ops, finalA) != before {
		t.Fatal("B's delivery changed A's state")
	}
	if replay := x.ok(t, ops[dayStart])[0]; !bytes.Equal(replay, finalA[dayStart]) {
		t.Fatal("A's acknowledgement changed after B used its identifier")
	}
	// Each organization reads only its own rows under the ordinary forced-RLS runtime.
	for _, pair := range [][2]string{{orgA, orgB}, {orgB, orgA}} {
		var own, foreign int
		if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FILTER(WHERE org_id=$1),count(*) FILTER(WHERE org_id=$2) FROM org_sync_acknowledgements WHERE operation_id=$3`,
			pair[0], pair[1], ops[dayStart].ID()).Scan(&own, &foreign); err != nil || own != 1 || foreign != 1 {
			t.Fatal("one acknowledgement per (organization, operation identifier) was not kept", own, foreign, err)
		}
	}
	var unscoped int
	if err := x.f.Runtime.QueryRow(t.Context(), `SELECT count(*) FROM org_sync_acknowledgements`).Scan(&unscoped); err != nil || unscoped != 0 {
		t.Fatal("a bare runtime connection read acknowledgements", unscoped, err)
	}
}
