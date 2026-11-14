//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	eventapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/application"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/jackc/pgx/v5/pgtype"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	accesspg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	eventpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/adapters"
	event "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/domain"
	syncpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/jackc/pgx/v5"
)

const syncTestType = "validation.setting"
const syncTestConsumer = "validation.sync"

type syncFixtureState struct {
	deviceFixtureState
	source  syncapp.Source
	handler *syncSQLHandler
	store   *syncpg.Store
	config  syncpg.Config
	target  string
}
type syncSQLHandler struct {
	roles      *accesspg.Store
	available  atomic.Bool
	afterApply func(context.Context, syncapp.DBTX) error
	descriptor *syncapp.Descriptor
}

func (h *syncSQLHandler) Descriptor() syncapp.Descriptor {
	if h.descriptor != nil {
		return *h.descriptor
	}
	return syncapp.Descriptor{Type: syncTestType, Schema: 1, WritePolicy: access.Policy{Permission: "org.settings.manage", Capability: "organizations", Effect: access.ConfigurationWrite}, ReadPolicy: access.Policy{Permission: "org.settings.read", Capability: "organizations", Effect: access.Read}}
}
func (h *syncSQLHandler) check(ctx context.Context, tx syncapp.DBTX, s syncapp.Source, op protocol.Operation, mode syncapp.Mode) error {
	if !h.available.Load() {
		return syncapp.ErrUnavailable
	}
	current, e := h.roles.LoadAuthorityTx(ctx, tx, s.Scope, s.Actor.AccountID)
	if e != nil {
		return syncapp.SourceDenied(orgapi.Forbidden)
	}
	var owner string
	var at time.Time
	if e := tx.QueryRow(ctx, `SELECT owner_member::text,clock_timestamp() FROM org_sync_test_targets WHERE org_id=$1 AND id=$2 FOR UPDATE`, s.Scope.OrgID().String(), op.Snapshot().Entity.ID).Scan(&owner, &at); e != nil {
		return syncapp.SourceDenied(orgapi.NotFound)
	}
	policy := h.Descriptor().WritePolicy
	if mode == syncapp.ReadReceipt {
		policy = h.Descriptor().ReadPolicy
	}
	if _, e := permissions.CoveringRules(s.Scope, s.MembershipID, current.Assignments, policy.Permission, permissions.ResourceFacts{OrgID: s.Scope.OrgID(), ID: op.Snapshot().Entity.ID, AuthoredBy: owner, AssignedTo: owner}, at); e != nil {
		return syncapp.SourceDenied(orgapi.Forbidden)
	}
	var enabled bool
	if e := tx.QueryRow(ctx, `SELECT enabled FROM org_capabilities WHERE org_id=$1 AND capability='organizations'`, s.Scope.OrgID().String()).Scan(&enabled); e != nil || !enabled {
		return syncapp.SourceDenied(orgapi.Unavailable)
	}
	if mode == syncapp.Write {
		for _, dependency := range op.Snapshot().DependsOn {
			var ready bool
			if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM org_sync_effects WHERE org_id=$1 AND operation_id=$2)`, s.Scope.OrgID().String(), dependency).Scan(&ready); e != nil {
				return syncapp.ErrUnavailable
			}
			if ready {
				covered, e := h.parentCovered(ctx, tx, s, dependency)
				if e != nil {
					return e
				}
				if !covered {
					return syncapp.ErrDependencyPending
				}
			}
		}
	}
	return nil
}

// Resolve the actual source-owned record and its current read scope, never its ACK.
func (h *syncSQLHandler) parentCovered(ctx context.Context, tx syncapp.DBTX, s syncapp.Source, id string) (bool, error) {
	var typ, entity, parent, author, assigned string
	var at time.Time
	e := tx.QueryRow(ctx, `SELECT c.operation_type,c.entity_type,t.id::text,r.author_member::text,t.owner_member::text,clock_timestamp() FROM org_sync_test_records r JOIN org_sync_operations c USING(org_id,operation_id) JOIN org_sync_test_targets t ON(t.org_id,t.id)=(r.org_id,r.target_id) WHERE r.org_id=$1 AND r.operation_id=$2 AND NOT r.held`, s.Scope.OrgID().String(), id).Scan(&typ, &entity, &parent, &author, &assigned, &at)
	if errors.Is(e, pgx.ErrNoRows) {
		return false, nil
	}
	if e != nil {
		return false, syncapp.ErrUnavailable
	}
	if typ != syncTestType || entity != "setting" {
		return false, nil
	}
	current, e := h.roles.LoadAuthorityTx(ctx, tx, s.Scope, s.Actor.AccountID)
	if e != nil {
		return false, syncapp.SourceDenied(orgapi.Forbidden)
	}
	_, e = permissions.CoveringRules(s.Scope, s.MembershipID, current.Assignments, h.Descriptor().ReadPolicy.Permission, permissions.ResourceFacts{OrgID: s.Scope.OrgID(), ID: parent, AuthoredBy: author, AssignedTo: assigned}, at)
	return e == nil, nil
}
func syncUUIDArray(ids []string) []pgtype.UUID {
	out := make([]pgtype.UUID, len(ids))
	for i, id := range ids {
		_ = out[i].Scan(id)
	}
	return out
}
func (h *syncSQLHandler) Admit(ctx context.Context, tx syncapp.DBTX, s syncapp.Source, op protocol.Operation, mode syncapp.Mode) error {
	return h.check(ctx, tx, s, op, mode)
}
func (h *syncSQLHandler) Confirm(ctx context.Context, tx syncapp.DBTX, s syncapp.Source, op protocol.Operation, mode syncapp.Mode, _ syncapp.Outcome) error {
	return h.check(ctx, tx, s, op, mode)
}
func (h *syncSQLHandler) Apply(ctx context.Context, tx syncapp.DBTX, s syncapp.Source, op protocol.Operation) (syncapp.Outcome, error) {
	var payload struct {
		Value string `json:"value"`
	}
	if e := json.Unmarshal(op.PayloadBytes(), &payload); e != nil || payload.Value == "" || len(payload.Value) > 280 {
		return syncapp.Outcome{Result: protocol.Rejected, Code: "MALFORMED"}, nil
	}
	var revision int64
	if e := tx.QueryRow(ctx, `SELECT revision FROM org_sync_test_targets WHERE org_id=$1 AND id=$2 FOR UPDATE`, s.Scope.OrgID().String(), op.Snapshot().Entity.ID).Scan(&revision); e != nil {
		return syncapp.Outcome{}, e
	}
	outcome := syncapp.Outcome{Result: protocol.Accepted}
	links, proposed := []string{}, []string{}
	for _, ref := range op.Snapshot().SoftReferences {
		proposed = append(proposed, ref.ID)
		covered, e := h.parentCovered(ctx, tx, s, ref.ID)
		if e != nil {
			return syncapp.Outcome{}, e
		}
		if ref.Type != "setting" || !covered {
			outcome.Result = protocol.AcceptedForReview
			outcome.Code = "ORPHAN_REFERENCE"
		} else {
			links = append(links, ref.ID)
		}
	}
	expected := op.Snapshot().Entity.ExpectedRevision
	if expected != nil && *expected != revision {
		outcome.Result = protocol.AcceptedForReview
		outcome.Code = "STALE_REVISION"
	}
	if outcome.Result == protocol.Accepted {
		if e := tx.QueryRow(ctx, `UPDATE org_sync_test_targets SET revision=revision+1 WHERE org_id=$1 AND id=$2 RETURNING revision`, s.Scope.OrgID().String(), op.Snapshot().Entity.ID).Scan(&revision); e != nil {
			return syncapp.Outcome{}, e
		}
		outcome.EntityRevision = &revision
	} else {
		outcome.ReviewItemID = op.ID()
	}
	if _, e := tx.Exec(ctx, `INSERT INTO org_sync_test_records(org_id,operation_id,target_id,value,held,review_id,author_member,links,proposed_refs) VALUES($1,$2,$3,$4,$5,NULLIF($6,'')::uuid,$7,$8::uuid[],$9::uuid[])`, s.Scope.OrgID().String(), op.ID(), op.Snapshot().Entity.ID, payload.Value, outcome.Result == protocol.AcceptedForReview, outcome.ReviewItemID, s.MembershipID, syncUUIDArray(links), syncUUIDArray(proposed)); e != nil {
		return syncapp.Outcome{}, e
	}
	if err := syncRecordReview(ctx, tx, s, op, outcome); err != nil {
		return syncapp.Outcome{}, err
	}
	if h.afterApply != nil {
		if e := h.afterApply(ctx, tx); e != nil {
			return syncapp.Outcome{}, e
		}
	}
	return outcome, nil
}
func syncFixture(t *testing.T) syncFixtureState {
	t.Helper()
	fx := devicesFixture(t)
	matrixAssign(t, fx, matrixPerson{member: fx.person.member}, "ADMIN", "ORG", "ACTIVE")
	enrolled, e := fx.devices.EnrollDevice(t.Context(), fx.scope, fx.person.actor, deviceEnrollCommand(t, "Synthetic SQL intake"))
	if e != nil {
		t.Fatal(e)
	}
	enrollment, e := enrolled.Device.Enrollment.Snapshot(fx.scope)
	if e != nil {
		t.Fatal(e)
	}
	r := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	_, e = fx.f.Admin.Exec(t.Context(), `CREATE TABLE org_sync_test_targets(org_id uuid NOT NULL,id uuid NOT NULL,owner_member uuid NOT NULL,revision bigint NOT NULL,PRIMARY KEY(org_id,id));CREATE TABLE org_sync_test_records(org_id uuid NOT NULL,operation_id uuid NOT NULL,target_id uuid NOT NULL,value text NOT NULL,held boolean NOT NULL,review_id uuid,author_member uuid NOT NULL,links uuid[] NOT NULL,proposed_refs uuid[] NOT NULL,PRIMARY KEY(org_id,operation_id));CREATE TABLE org_sync_test_followups(org_id uuid NOT NULL,operation_id uuid NOT NULL,event_name text NOT NULL,PRIMARY KEY(org_id,operation_id));`)
	if e != nil {
		t.Fatal(e)
	}
	for _, table := range []string{"org_sync_test_targets", "org_sync_test_records", "org_sync_test_followups"} {
		if _, e := fx.f.Admin.Exec(t.Context(), "ALTER TABLE "+table+" ENABLE ROW LEVEL SECURITY;ALTER TABLE "+table+" FORCE ROW LEVEL SECURITY;CREATE POLICY scoped ON "+table+" USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);GRANT SELECT,INSERT,UPDATE ON "+table+" TO "+r); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_sync_operations TO "+r+";GRANT UPDATE(operation_id) ON org_sync_operations TO "+r+";GRANT SELECT,INSERT ON org_sync_acknowledgements,org_sync_effects,org_audit_events,org_event_intents,org_event_receipts TO "+r+";GRANT SELECT,INSERT,UPDATE ON job_queue TO "+r); e != nil {
		t.Fatal(e)
	}
	syncInstallReviewSource(t, fx)
	// Migration199's shared effect guard plans its reviewed-Meter branch for
	// every effect, so the generic intake role needs exactly the reading columns
	// that guard compares. It still cannot read any Meter value or observation.
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_sync_review_items TO "+r+";GRANT SELECT ON org_sync_review_resolutions,org_meter_held_readings TO "+r+";GRANT SELECT(org_id,id,operation_id,review_resolution_id) ON org_meter_readings TO "+r); err != nil {
		t.Fatal(err)
	}
	target := onboardingID(t)
	if e := unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_sync_test_targets VALUES($1,$2,$3,1)`, fx.scope.OrgID().String(), target, fx.person.member); e != nil {
		t.Fatal(e)
	}
	ac, e := audit.NewCatalog(syncpg.AuditDefinitions())
	if e != nil {
		t.Fatal(e)
	}
	ec, e := event.NewCatalog(syncpg.EventDefinitions([]string{syncTestConsumer}))
	if e != nil {
		t.Fatal(e)
	}
	source := syncapp.Source{Scope: fx.scope, Origin: deviceTestOrigin, Actor: fx.person.actor, MembershipID: fx.person.member, EnrollmentID: enrollment.ID}
	handler := &syncSQLHandler{roles: accesspg.NewStore(fx.f.Runtime)}
	handler.available.Store(true)
	config := syncpg.Config{Pool: fx.f.Runtime, ConfirmSource: func(ctx context.Context, tx pgx.Tx, s syncapp.Source) (time.Time, error) {
		_, at, e := fx.devices.CheckCurrentDeviceTx(ctx, tx, s.Scope, s.Actor, s.MembershipID, s.EnrollmentID)
		if e != nil {
			return time.Time{}, syncapp.SourceDenied(e)
		}
		return at, nil
	}, Audit: auditpg.NewStore(fx.f.Runtime, ac), AuditCatalog: ac, EventCatalog: ec, EventWriter: eventpg.NewStore(fx.f.Runtime), Plans: map[string]map[string][]string{syncTestType: {protocol.Accepted: {syncTestConsumer}, protocol.AcceptedForReview: {syncTestConsumer}, protocol.Rejected: {syncTestConsumer}}}}
	return syncFixtureState{deviceFixtureState: fx, source: source, handler: handler, store: syncpg.NewStore(config), config: config, target: target}
}
func syncOperation(t *testing.T, fx syncFixtureState, id, value string, expected *int64, deps []string, soft []protocol.SoftReference) protocol.Operation {
	t.Helper()
	raw, e := json.Marshal(map[string]any{"schema_version": 1, "operation_id": id, "owner_account_id": fx.source.Actor.AccountID, "api_origin": fx.source.Origin, "org_id": fx.scope.OrgID().String(), "membership_id": fx.source.MembershipID, "enrollment_id": fx.source.EnrollmentID, "type": syncTestType, "operation_schema": 1, "entity": map[string]any{"type": "setting", "id": fx.target, "expected_revision": expected}, "occurred_at": time.Now().UTC().Format(time.RFC3339Nano), "entry_zone": "UTC", "device_offset_seconds": 0, "depends_on": append([]string{}, deps...), "soft_references": func() []map[string]string {
		out := []map[string]string{}
		for _, ref := range soft {
			out = append(out, map[string]string{"type": ref.Type, "id": ref.ID})
		}
		return out
	}(), "attachments": []any{}, "payload": map[string]any{"value": value}})
	if e != nil {
		t.Fatal(e)
	}
	binding, e := protocol.NewBinding(fx.source.Actor.AccountID, fx.source.Origin, fx.scope.OrgID().String())
	if e != nil {
		t.Fatal(e)
	}
	op, e := protocol.Seal(raw, binding)
	if e != nil {
		t.Fatal(e)
	}
	return op
}
func syncCount(t *testing.T, fx syncFixtureState, table string) int {
	t.Helper()
	allowed := map[string]bool{"org_sync_operations": true, "org_sync_acknowledgements": true, "org_sync_effects": true, "org_sync_test_records": true, "org_sync_test_followups": true}
	if !allowed[table] {
		t.Fatal("unknown test count")
	}
	var n int
	if e := fx.f.Admin.QueryRow(t.Context(), "SELECT count(*) FROM "+table).Scan(&n); e != nil {
		t.Fatal(e)
	}
	return n
}
func TestSyncIntakeAtomicAcceptanceAndExactReplay(t *testing.T) {
	fx := syncFixture(t)
	op := syncOperation(t, fx, onboardingID(t), "Synthetic private configuration", nil, nil, nil)
	ack, e := fx.store.Process(t.Context(), fx.source, op, protocol.DefaultLimits(), fx.handler)
	if e != nil || ack.Result() != protocol.Accepted {
		t.Fatal("normal intake did not apply", e)
	}
	replay, e := fx.store.Process(t.Context(), fx.source, op, protocol.DefaultLimits(), fx.handler)
	if e != nil || string(replay.CanonicalBytes()) != string(ack.CanonicalBytes()) {
		t.Fatal("original bytes did not replay", e)
	}
	for _, table := range []string{"org_sync_operations", "org_sync_acknowledgements", "org_sync_effects", "org_sync_test_records"} {
		if n := syncCount(t, fx, table); n != 1 {
			t.Fatal(table, n)
		}
	}
}

func TestSyncIntakeDependencyClaimsReviewAndMissingProducer(t *testing.T) {
	fx := syncFixture(t)
	parent := syncOperation(t, fx, onboardingID(t), "Synthetic parent", nil, nil, nil)
	child := syncOperation(t, fx, onboardingID(t), "Synthetic dependent", nil, []string{parent.ID()}, nil)
	ack, e := fx.store.Process(t.Context(), fx.source, child, protocol.DefaultLimits(), fx.handler)
	if e != nil || ack.Result() != protocol.Retry || ack.Code() != "DEPENDENCY_PENDING" || ack.IsFinal() {
		t.Fatal("unresolved dependency finalized", e)
	}
	if syncCount(t, fx, "org_sync_operations") != 1 || syncCount(t, fx, "org_sync_acknowledgements") != 0 || syncCount(t, fx, "org_sync_test_records") != 0 {
		t.Fatal("pending dependency acquired final projections")
	}
	if a, e := fx.store.Process(t.Context(), fx.source, parent, protocol.DefaultLimits(), fx.handler); e != nil || a.Result() != protocol.Accepted {
		t.Fatal(e)
	}
	if a, e := fx.store.Process(t.Context(), fx.source, child, protocol.DefaultLimits(), fx.handler); e != nil || a.Result() != protocol.Accepted {
		t.Fatal("later actual dependency did not unblock", e)
	}
	review := syncOperation(t, fx, onboardingID(t), "Synthetic held orphan", nil, nil, []protocol.SoftReference{{Type: "setting", ID: onboardingID(t)}})
	if a, e := fx.store.Process(t.Context(), fx.source, review, protocol.DefaultLimits(), fx.handler); e != nil || a.Result() != protocol.AcceptedForReview || a.Code() != "ORPHAN_REFERENCE" {
		t.Fatal("orphan facts not held", e)
	}
	heldChild := syncOperation(t, fx, onboardingID(t), "Synthetic waits for review", nil, []string{review.ID()}, nil)
	if a, e := fx.store.Process(t.Context(), fx.source, heldChild, protocol.DefaultLimits(), fx.handler); e != nil || a.Code() != "DEPENDENCY_PENDING" {
		t.Fatal("review ACK became readiness", e)
	}
	// The actual effect guard must refuse it, not a missing fixture privilege.
	unitSchemaCode(t, unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_sync_effects(org_id,operation_id,entity_revision)VALUES($1,$2,1)`, fx.scope.OrgID().String(), review.ID()), "23514")
	unavailable := syncOperation(t, fx, onboardingID(t), "Synthetic temporarily unavailable", nil, nil, nil)
	before := syncCount(t, fx, "org_sync_operations")
	fx.handler.available.Store(false)
	if _, e := fx.store.Process(t.Context(), fx.source, unavailable, protocol.DefaultLimits(), fx.handler); !errors.Is(e, syncapp.ErrUnavailable) {
		t.Fatal(e)
	}
	if syncCount(t, fx, "org_sync_operations") != before {
		t.Fatal("missing history producer claimed an intent")
	}
	fx.handler.available.Store(true)
	if a, e := fx.store.Process(t.Context(), fx.source, unavailable, protocol.DefaultLimits(), fx.handler); e != nil || a.Result() != protocol.Accepted {
		t.Fatal(e)
	}
}

func TestSyncIntakeTwelveWayDuplicatesAndTargetContention(t *testing.T) {
	for _, same := range []bool{true, false} {
		t.Run(fmt.Sprint(same), func(t *testing.T) {
			fx := syncFixture(t)
			pool := fx.f.RuntimePool(t, 12)
			config := fx.config
			config.Pool = pool
			store := syncpg.NewStore(config)
			firstRevision := int64(1)
			ops := make([]protocol.Operation, 12)
			ops[0] = syncOperation(t, fx, onboardingID(t), "Synthetic concurrent value", &firstRevision, nil, nil)
			for i := 1; i < len(ops); i++ {
				ops[i] = ops[0]
				if !same {
					ops[i] = syncOperation(t, fx, onboardingID(t), "Synthetic target competitor", &firstRevision, nil, nil)
				}
			}
			acks := make([]protocol.Acknowledgement, 12)
			errs := make([]error, 12)
			start := make(chan struct{})
			var wg sync.WaitGroup
			for i := range ops {
				wg.Go(func() {
					<-start
					acks[i], errs[i] = store.Process(t.Context(), fx.source, ops[i], protocol.DefaultLimits(), fx.handler)
				})
			}
			close(start)
			wg.Wait()
			accepted, review := 0, 0
			for i, a := range acks {
				if errs[i] != nil {
					t.Fatal("concurrent intake", errs[i])
				}
				if a.Result() == protocol.Accepted {
					accepted++
				}
				if a.Result() == protocol.AcceptedForReview {
					review++
				}
				if same && !bytes.Equal(a.CanonicalBytes(), acks[0].CanonicalBytes()) {
					t.Fatal("duplicate ACK bytes diverged")
				}
			}
			want := 12
			if same {
				want = 1
				if accepted != 12 || review != 0 {
					t.Fatal(accepted, review)
				}
			} else if accepted != 1 || review != 11 {
				t.Fatal(accepted, review)
			}
			if syncCount(t, fx, "org_sync_acknowledgements") != want || syncCount(t, fx, "org_sync_test_records") != want || syncCount(t, fx, "org_sync_effects") != 1 {
				t.Fatal("duplicate/target effects diverged")
			}
		})
	}
}

func TestSyncIntakeLateProjectionFailuresRollBackEveryProjectionAndRecover(t *testing.T) {
	for _, stage := range []string{"effect", "org_audit_events", "org_event_intents", "org_sync_acknowledgements", "org_sync_effects", "commit"} {
		t.Run(stage, func(t *testing.T) {
			fx := syncFixture(t)
			op := syncOperation(t, fx, onboardingID(t), "Synthetic failpoint value", nil, nil, nil)
			var commitWitness atomic.Bool
			store := fx.store
			if stage == "commit" {
				cfg := fx.config
				var calls atomic.Int64
				cfg.ConfirmSource = func(ctx context.Context, tx pgx.Tx, s syncapp.Source) (time.Time, error) {
					at, e := fx.config.ConfirmSource(ctx, tx, s)
					if e != nil {
						return at, e
					}
					if calls.Add(1) == 3 {
						var projections int
						e = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM org_sync_operations WHERE org_id=$1 AND operation_id=$2)+(SELECT count(*) FROM org_sync_acknowledgements WHERE org_id=$1 AND operation_id=$2)+(SELECT count(*) FROM org_sync_effects WHERE org_id=$1 AND operation_id=$2)+(SELECT count(*) FROM org_sync_test_records WHERE org_id=$1 AND operation_id=$2)+(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND operation_id=$2)+(SELECT count(*) FROM org_event_intents WHERE org_id=$1 AND event_id=$2)`, s.Scope.OrgID().String(), op.ID()).Scan(&projections)
						if e != nil {
							return time.Time{}, e
						}
						if projections != 6 {
							return time.Time{}, errors.New("synthetic incomplete commit projection")
						}
						commitWitness.Store(true)
					}
					return at, nil
				}
				store = syncpg.NewStore(cfg)
			}
			if stage == "effect" {
				fx.handler.afterApply = func(context.Context, syncapp.DBTX) error { return errors.New("synthetic post-effect failure") }
			} else if stage == "commit" {
				if _, e := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION org_sync_test_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic deferred commit failure';END $$;CREATE CONSTRAINT TRIGGER org_sync_test_failure AFTER INSERT ON org_sync_acknowledgements DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_sync_test_failure()`); e != nil {
					t.Fatal(e)
				}
			} else {
				if _, e := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION org_sync_test_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic projection failure';END $$;CREATE TRIGGER org_sync_test_failure BEFORE INSERT ON `+stage+` FOR EACH ROW EXECUTE FUNCTION org_sync_test_failure()`); e != nil {
					t.Fatal(e)
				}
			}
			if a, e := store.Process(t.Context(), fx.source, op, protocol.DefaultLimits(), fx.handler); !errors.Is(e, syncapp.ErrUnavailable) || a.IsFinal() {
				t.Fatal("provisional success escaped", e)
			}
			if stage == "commit" && !commitWitness.Load() {
				t.Fatal("deferred failure did not occur after all provisional projections and final source proof")
			}
			for _, table := range []string{"org_sync_operations", "org_sync_acknowledgements", "org_sync_effects", "org_sync_test_records"} {
				if syncCount(t, fx, table) != 0 {
					t.Fatal("partial projection survived", stage, table)
				}
			}
			var auditCount, intents, revision int
			if e := fx.f.Admin.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_event_intents WHERE org_id=$1 AND event_id=$2),(SELECT revision FROM org_sync_test_targets WHERE org_id=$1 AND id=$3)`, fx.scope.OrgID().String(), op.ID(), fx.target).Scan(&auditCount, &intents, &revision); e != nil || auditCount != 0 || intents != 0 || revision != 1 {
				t.Fatal("audit/outbox/target survived rollback", e)
			}
			var jobs int
			dedupe := (event.Envelope{OrgID: fx.scope.OrgID().String(), Consumer: syncTestConsumer, Event: event.Event{ID: op.ID()}}).DedupeKey()
			if e := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM job_queue WHERE dedupe_key=$1`, dedupe).Scan(&jobs); e != nil || jobs != 0 {
				t.Fatal("queue staging escaped caller rollback", e)
			}
			fx.handler.afterApply = nil
			if stage != "effect" {
				table := stage
				if table == "commit" {
					table = "org_sync_acknowledgements"
				}
				if _, e := fx.f.Admin.Exec(t.Context(), `DROP TRIGGER org_sync_test_failure ON `+table); e != nil {
					t.Fatal(e)
				}
			}
			if a, e := fx.store.Process(t.Context(), fx.source, op, protocol.DefaultLimits(), fx.handler); e != nil || a.Result() != protocol.Accepted {
				t.Fatal("same immutable intent did not recover", e)
			}
		})
	}
}

func TestSyncIntakeActualProofAndRoleExpiryAfterEffectsNeverCommit(t *testing.T) {
	for _, kind := range []string{"proof", "role"} {
		t.Run(kind, func(t *testing.T) {
			fx := syncFixture(t)
			var until time.Time
			if kind == "proof" {
				fx.source.Actor = deviceVerifyActor(t, fx.deviceFixtureState, fx.person, fx.person.signer, time.Now().UTC().Truncate(time.Second).Add(3*time.Second))
				until = fx.source.Actor.ProofExpires
			} else {
				until = time.Now().UTC().Add(time.Second)
				if e := unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_role_assignments SET ended_at=clock_timestamp(),revision=revision+1 WHERE org_id=$1 AND membership_id=$2 AND ended_at IS NULL`, fx.scope.OrgID().String(), fx.source.MembershipID); e != nil {
					t.Fatal(e)
				}
				if e := unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,valid_to,assigned_by,reason)VALUES($1,$2,$3,'ADMIN','ORG',$1,clock_timestamp(),$4,$5,'Synthetic finite sync authority')`, fx.scope.OrgID().String(), onboardingID(t), fx.source.MembershipID, until, fx.member); e != nil {
					t.Fatal(e)
				}
			}
			var applied atomic.Bool
			fx.handler.afterApply = func(ctx context.Context, tx syncapp.DBTX) error {
				applied.Store(true)
				_, e := tx.Exec(ctx, `SELECT pg_sleep(GREATEST(0,EXTRACT(EPOCH FROM($1::timestamptz-clock_timestamp())))+0.01)`, until)
				return e
			}
			op := syncOperation(t, fx, onboardingID(t), "Synthetic expiring authority", nil, nil, nil)
			if _, e := fx.store.Process(t.Context(), fx.source, op, protocol.DefaultLimits(), fx.handler); !errors.Is(e, syncapp.ErrSourceDenied) {
				t.Fatal("stale authority committed", e)
			}
			if !applied.Load() {
				t.Fatal("expiry did not occur after actual provisional effect")
			}
			for _, table := range []string{"org_sync_operations", "org_sync_acknowledgements", "org_sync_effects", "org_sync_test_records"} {
				if syncCount(t, fx, table) != 0 {
					t.Fatal("expiry left partial private data", table)
				}
			}
			fx.handler.afterApply = nil
			if kind == "proof" {
				fx.source.Actor = deviceVerifyActor(t, fx.deviceFixtureState, fx.person, fx.person.signer, time.Time{})
			} else {
				matrixAssign(t, fx.deviceFixtureState, matrixPerson{member: fx.source.MembershipID}, "ADMIN", "ORG", "ACTIVE")
			}
			if a, e := fx.store.Process(t.Context(), fx.source, op, protocol.DefaultLimits(), fx.handler); e != nil || a.Result() != protocol.Accepted {
				t.Fatal("fresh authority could not retry original", e)
			}
		})
	}
}

type syncPausedWriter struct {
	eventapp.Writer
	entered, release chan struct{}
}

func (w *syncPausedWriter) Enqueue(ctx context.Context, scope kernel.OrgScope, tx pgx.Tx, e event.Envelope, hash string) error {
	close(w.entered)
	select {
	case <-w.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	return w.Writer.Enqueue(ctx, scope, tx, e, hash)
}
func TestSyncIntakeReceiptTimeFollowsRealOutboxWait(t *testing.T) {
	fx := syncFixture(t)
	writer := &syncPausedWriter{Writer: fx.config.EventWriter, entered: make(chan struct{}), release: make(chan struct{})}
	config := fx.config
	config.EventWriter = writer
	store := syncpg.NewStore(config)
	op := syncOperation(t, fx, onboardingID(t), "Synthetic blocked followup", nil, nil, nil)
	type completion struct {
		ack protocol.Acknowledgement
		e   error
	}
	done := make(chan completion, 1)
	go func() {
		a, e := store.Process(t.Context(), fx.source, op, protocol.DefaultLimits(), fx.handler)
		done <- completion{a, e}
	}()
	defer func() {
		select {
		case <-writer.release:
		default:
			close(writer.release)
		}
	}()
	select {
	case <-writer.entered:
	case result := <-done:
		t.Fatal("outbox was not reached", result.e)
	case <-time.After(5 * time.Second):
		t.Fatal("outbox wait did not begin")
	}
	var releasedAt time.Time
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT clock_timestamp()`).Scan(&releasedAt); e != nil {
		t.Fatal(e)
	}
	close(writer.release)
	result := <-done
	if result.e != nil {
		t.Fatal(result.e)
	}
	received, e := orgtime.ParseInstant(result.ack.Snapshot().ReceivedAt)
	if e != nil || !received.After(releasedAt) {
		t.Fatal("final receipt used time captured before outbox release", e)
	}
}

type syncGuardFault struct {
	*syncSQLHandler
	admit, confirm error
	readOnly       bool
}

func (h *syncGuardFault) Admit(ctx context.Context, tx syncapp.DBTX, s syncapp.Source, op protocol.Operation, mode syncapp.Mode) error {
	if e := h.syncSQLHandler.Admit(ctx, tx, s, op, mode); e != nil {
		return e
	}
	if !h.readOnly || mode == syncapp.ReadReceipt {
		return h.admit
	}
	return nil
}
func (h *syncGuardFault) Confirm(ctx context.Context, tx syncapp.DBTX, s syncapp.Source, op protocol.Operation, mode syncapp.Mode, o syncapp.Outcome) error {
	if e := h.syncSQLHandler.Confirm(ctx, tx, s, op, mode, o); e != nil {
		return e
	}
	if !h.readOnly || mode == syncapp.ReadReceipt {
		return h.confirm
	}
	return nil
}
func TestSyncIntakeSourceDenialAlwaysOutranksPendingAndRollsBack(t *testing.T) {
	cases := []struct {
		name           string
		admit, confirm error
	}{
		{"wrapped_admit", syncapp.SourceDenied(syncapp.ErrDependencyPending), nil},
		{"joined_admit", errors.Join(syncapp.ErrDependencyPending, syncapp.SourceDenied(orgapi.Unauthenticated)), nil},
		{"pending_then_wrapped_confirm", syncapp.ErrDependencyPending, syncapp.SourceDenied(syncapp.ErrDependencyPending)},
		{"final_joined_confirm", nil, errors.Join(syncapp.ErrDependencyPending, syncapp.SourceDenied(orgapi.Unauthenticated))},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fx := syncFixture(t)
			op := syncOperation(t, fx, onboardingID(t), "Synthetic denial precedence", nil, nil, nil)
			h := &syncGuardFault{syncSQLHandler: fx.handler, admit: c.admit, confirm: c.confirm}
			if a, e := fx.store.Process(t.Context(), fx.source, op, protocol.DefaultLimits(), h); !errors.Is(e, syncapp.ErrSourceDenied) || a.IsFinal() {
				t.Fatal("source denial was downgraded", e)
			}
			for _, table := range []string{"org_sync_operations", "org_sync_acknowledgements", "org_sync_effects", "org_sync_test_records"} {
				if syncCount(t, fx, table) != 0 {
					t.Fatal("mixed denial committed projections", table)
				}
			}
		})
	}
	fx := syncFixture(t)
	op := syncOperation(t, fx, onboardingID(t), "Synthetic unavailable parent coverage", nil, nil, nil)
	h := &syncGuardFault{syncSQLHandler: fx.handler, admit: syncapp.ErrDependencyPending, confirm: syncapp.ErrDependencyPending}
	if a, e := fx.store.Process(t.Context(), fx.source, op, protocol.DefaultLimits(), h); e != nil || a.Code() != "DEPENDENCY_PENDING" || a.IsFinal() {
		t.Fatal("coverage pending classification", e)
	}
	if syncCount(t, fx, "org_sync_operations") != 0 {
		t.Fatal("partially admitted intent was committed")
	}
	if _, e := fx.store.Process(t.Context(), fx.source, op, protocol.DefaultLimits(), fx.handler); e != nil {
		t.Fatal(e)
	}
	h.readOnly = true
	if _, e := fx.store.Process(t.Context(), fx.source, op, protocol.DefaultLimits(), h); !errors.Is(e, syncapp.ErrUnavailable) {
		t.Fatal("read receipt silently adopted hard-dependency policy", e)
	}
}

func TestSyncIntakeTypedNilProvidersCannotReachSourceOrCreateClaims(t *testing.T) {
	fx := syncFixture(t)
	op := syncOperation(t, fx, onboardingID(t), "Synthetic missing provider", nil, nil, nil)
	var nilHandler *syncSQLHandler
	var nilAudit *auditpg.Store
	var nilWriter *eventpg.Store
	for _, kind := range []string{"handler", "audit", "writer"} {
		t.Run(kind, func(t *testing.T) {
			config := fx.config
			var sourceCalls atomic.Int64
			config.ConfirmSource = func(ctx context.Context, tx pgx.Tx, s syncapp.Source) (time.Time, error) {
				sourceCalls.Add(1)
				return fx.config.ConfirmSource(ctx, tx, s)
			}
			var h syncapp.Handler = fx.handler
			switch kind {
			case "handler":
				h = nilHandler
			case "audit":
				config.Audit = nilAudit
			case "writer":
				config.EventWriter = nilWriter
			}
			if _, e := syncpg.NewStore(config).Process(t.Context(), fx.source, op, protocol.DefaultLimits(), h); !errors.Is(e, syncapp.ErrUnavailable) {
				t.Fatal(e)
			}
			if sourceCalls.Load() != 0 || syncCount(t, fx, "org_sync_operations") != 0 {
				t.Fatal("missing producer reached intake")
			}
		})
	}
}

func TestSyncIntakeSchemaValidCanonicalAckCorruptionIsNeverRepaired(t *testing.T) {
	fx := syncFixture(t)
	op := syncOperation(t, fx, onboardingID(t), "Synthetic immutable receipt", nil, nil, nil)
	original, e := fx.store.Process(t.Context(), fx.source, op, protocol.DefaultLimits(), fx.handler)
	if e != nil {
		t.Fatal(e)
	}
	variants := []protocol.AcknowledgementSnapshot{original.Snapshot(), original.Snapshot()}
	wrongRevision := int64(999)
	variants[0].EntityRevision = &wrongRevision
	variants[1].Result = protocol.Rejected
	variants[1].Code = "MALFORMED"
	variants[1].EntityRevision = nil
	for _, value := range variants {
		corrupt, e := protocol.NewAcknowledgement(op, value)
		if e != nil {
			t.Fatal(e)
		}
		if e := pgx.BeginFunc(t.Context(), fx.f.Admin, func(tx pgx.Tx) error {
			if _, e := tx.Exec(t.Context(), `ALTER TABLE org_sync_acknowledgements DISABLE TRIGGER org_sync_acknowledgements_immutable`); e != nil {
				return e
			}
			if _, e := tx.Exec(t.Context(), `UPDATE org_sync_acknowledgements SET canonical_ack=$1 WHERE org_id=$2 AND operation_id=$3`, corrupt.CanonicalBytes(), fx.scope.OrgID().String(), op.ID()); e != nil {
				return e
			}
			_, e := tx.Exec(t.Context(), `ALTER TABLE org_sync_acknowledgements ENABLE TRIGGER org_sync_acknowledgements_immutable`)
			return e
		}); e != nil {
			t.Fatal(e)
		}
		if a, e := fx.store.Process(t.Context(), fx.source, op, protocol.DefaultLimits(), fx.handler); !errors.Is(e, syncapp.ErrUnavailable) || a.IsFinal() {
			t.Fatal("canonical corrupt receipt escaped", e)
		}
	}
	if syncCount(t, fx, "org_sync_effects") != 1 || syncCount(t, fx, "org_sync_test_records") != 1 {
		t.Fatal("receipt corruption repeated effects")
	}
}

func TestSyncIntakeParentCoverageMatchesMissingWithoutDeviceRestriction(t *testing.T) {
	fx := syncFixture(t)
	other := deviceNewPerson(t, fx.deviceFixtureState)
	matrixAssign(t, fx.deviceFixtureState, matrixPerson{member: other.member}, "ADMIN", "ORG", "ACTIVE")
	enrolled, e := fx.devices.EnrollDevice(t.Context(), fx.scope, other.actor, deviceEnrollCommand(t, "Synthetic covered parent source"))
	if e != nil {
		t.Fatal(e)
	}
	registration, e := enrolled.Device.Enrollment.Snapshot(fx.scope)
	if e != nil {
		t.Fatal(e)
	}
	otherFX := fx
	otherFX.source = syncapp.Source{Scope: fx.scope, Origin: fx.source.Origin, Actor: other.actor, MembershipID: other.member, EnrollmentID: registration.ID}
	otherFX.target = onboardingID(t)
	if e := unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_sync_test_targets VALUES($1,$2,$3,1)`, fx.scope.OrgID().String(), otherFX.target, other.member); e != nil {
		t.Fatal(e)
	}
	parent := syncOperation(t, otherFX, onboardingID(t), "Synthetic other device parent", nil, nil, nil)
	if a, e := fx.store.Process(t.Context(), otherFX.source, parent, protocol.DefaultLimits(), fx.handler); e != nil || a.Result() != protocol.Accepted {
		t.Fatal("real other-device effect did not exist", e)
	}
	matrixAssign(t, fx.deviceFixtureState, matrixPerson{member: fx.source.MembershipID}, "ADMIN", "SELF", "ACTIVE")
	hidden := syncOperation(t, fx, onboardingID(t), "Synthetic inaccessible dependency", nil, []string{parent.ID()}, nil)
	missing := syncOperation(t, fx, onboardingID(t), "Synthetic missing dependency", nil, []string{onboardingID(t)}, nil)
	hiddenAck, e := fx.store.Process(t.Context(), fx.source, hidden, protocol.DefaultLimits(), fx.handler)
	if e != nil {
		t.Fatal(e)
	}
	missingAck, e := fx.store.Process(t.Context(), fx.source, missing, protocol.DefaultLimits(), fx.handler)
	if e != nil {
		t.Fatal(e)
	}
	if hiddenAck.Result() != protocol.Retry || hiddenAck.Code() != "DEPENDENCY_PENDING" || missingAck.Result() != hiddenAck.Result() || missingAck.Code() != hiddenAck.Code() || hiddenAck.IsFinal() || missingAck.IsFinal() {
		t.Fatal("parent visibility distinguished private existence")
	}
	var hiddenClaims, missingClaims int
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*)FILTER(WHERE operation_id=$2),count(*)FILTER(WHERE operation_id=$3) FROM org_sync_operations WHERE org_id=$1`, fx.scope.OrgID().String(), hidden.ID(), missing.ID()).Scan(&hiddenClaims, &missingClaims); e != nil || hiddenClaims != 0 || missingClaims != 1 {
		t.Fatal("reference admission persisted an unauthorized new claim", e)
	}
	soft := syncOperation(t, fx, onboardingID(t), "Synthetic private proposed link", nil, nil, []protocol.SoftReference{{Type: "setting", ID: parent.ID()}})
	originalSoftBytes, originalSoftDigest := soft.CanonicalBytes(), soft.PayloadDigest()
	if a, e := fx.store.Process(t.Context(), fx.source, soft, protocol.DefaultLimits(), fx.handler); e != nil || a.Result() != protocol.AcceptedForReview || a.Code() != "ORPHAN_REFERENCE" {
		t.Fatal("unreadable soft reference was linked", e)
	}
	var held bool
	var links, proposed int
	var value string
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT held,cardinality(links),cardinality(proposed_refs),value FROM org_sync_test_records WHERE org_id=$1 AND operation_id=$2`, fx.scope.OrgID().String(), soft.ID()).Scan(&held, &links, &proposed, &value); e != nil || !held || links != 0 || proposed != 1 || value != "Synthetic private proposed link" {
		t.Fatal("orphan facts/context were not actually held", e)
	}
	matrixAssign(t, fx.deviceFixtureState, matrixPerson{member: fx.source.MembershipID}, "ADMIN", "ORG", "ACTIVE")
	if a, e := fx.store.Process(t.Context(), fx.source, hidden, protocol.DefaultLimits(), fx.handler); e != nil || a.Result() != protocol.Accepted {
		t.Fatal("covered parent from another member/device was prohibited", e)
	}
	if !bytes.Equal(soft.CanonicalBytes(), originalSoftBytes) || soft.PayloadDigest() != originalSoftDigest {
		t.Fatal("immutable operation changed")
	}
}

func TestSyncIntakeUnexpectedPendingFromApplyIsUnavailableAndSourceFailureIsFatal(t *testing.T) {
	fx := syncFixture(t)
	op := syncOperation(t, fx, onboardingID(t), "Synthetic misplaced pending", nil, nil, nil)
	fx.handler.afterApply = func(context.Context, syncapp.DBTX) error { return syncapp.ErrDependencyPending }
	if a, e := fx.store.Process(t.Context(), fx.source, op, protocol.DefaultLimits(), fx.handler); !errors.Is(e, syncapp.ErrUnavailable) || a.IsFinal() {
		t.Fatal("unguarded Apply sentinel gained pending authority", e)
	}
	if syncCount(t, fx, "org_sync_operations") != 0 || syncCount(t, fx, "org_sync_test_records") != 0 {
		t.Fatal("unexpected Apply sentinel retained private writes")
	}
	fx.handler.afterApply = nil
	config := fx.config
	config.ConfirmSource = func(context.Context, pgx.Tx, syncapp.Source) (time.Time, error) {
		return time.Time{}, syncapp.ErrDependencyPending
	}
	if _, e := syncpg.NewStore(config).Process(t.Context(), fx.source, op, protocol.DefaultLimits(), fx.handler); !errors.Is(e, syncapp.ErrSourceDenied) {
		t.Fatal("unverifiable source became a per-operation retry", e)
	}
}
