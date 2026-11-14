//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	event "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/domain"
	syncpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/jackc/pgx/v5"
)

func syncNoAsync(t *testing.T, fx syncFixtureState) (syncpg.Config, syncapp.Handler) {
	t.Helper()
	descriptor := fx.handler.Descriptor()
	descriptor.TerminalResults = []string{protocol.Accepted, protocol.Rejected}
	fx.handler.descriptor = &descriptor
	registry, err := syncapp.NewRegistry(fx.handler)
	if err != nil {
		t.Fatal(err)
	}
	handler, ok := registry.Lookup(syncTestType)
	if !ok {
		t.Fatal("missing registered source")
	}
	config := fx.config
	config.EventCatalog = event.Catalog{}
	config.EventWriter = nil
	config.Plans = nil
	config.NoAsyncPlans = map[syncapp.OperationKey]map[string]syncapp.FollowupMode{
		{Type: syncTestType, Schema: 1}: {protocol.Accepted: syncapp.NoAsyncFollowup, protocol.Rejected: syncapp.NoAsyncFollowup},
	}
	return config, handler
}

func syncNoAsyncCounts(t *testing.T, fx syncFixtureState, op protocol.Operation) (facts, audit, asynchronous int) {
	t.Helper()
	err := fx.f.Admin.QueryRow(t.Context(), `SELECT
 (SELECT count(*) FROM org_sync_operations WHERE org_id=$1 AND operation_id=$2)+
 (SELECT count(*) FROM org_sync_acknowledgements WHERE org_id=$1 AND operation_id=$2)+
 (SELECT count(*) FROM org_sync_effects WHERE org_id=$1 AND operation_id=$2)+
 (SELECT count(*) FROM org_sync_test_records WHERE org_id=$1 AND operation_id=$2),
 (SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND operation_id=$2),
 (SELECT count(*) FROM org_event_intents WHERE org_id=$1 AND event_id=$2)+
 (SELECT count(*) FROM org_sync_test_followups WHERE org_id=$1 AND operation_id=$2)+
 (SELECT count(*) FROM job_queue WHERE dedupe_key LIKE '%'||$2||'%')`, fx.scope.OrgID().String(), op.ID()).Scan(&facts, &audit, &asynchronous)
	if err != nil {
		t.Fatal(err)
	}
	return
}

func TestSyncNoAsyncAtomicConcurrentOriginalReceipts(t *testing.T) {
	fx := syncFixture(t)
	config, handler := syncNoAsync(t, fx)
	config.Pool = fx.f.RuntimePool(t, 2)
	store := syncpg.NewStore(config)
	op := syncOperation(t, fx, onboardingID(t), "Synthetic atomic command without an asynchronous business effect", nil, nil, nil)
	type completion struct {
		ack protocol.Acknowledgement
		err error
	}
	done := make(chan completion, 2)
	start := make(chan struct{})
	ctx, cancel := context.WithTimeout(t.Context(), 6*time.Second)
	defer cancel()
	for range 2 {
		go func() {
			<-start
			ack, err := store.Process(ctx, fx.source, op, protocol.DefaultLimits(), handler)
			done <- completion{ack, err}
		}()
	}
	close(start)
	var first []byte
	for range 2 {
		select {
		case result := <-done:
			if result.err != nil || result.ack.Result() != protocol.Accepted {
				t.Fatal("atomic source without followup did not commit", result.err)
			}
			if first == nil {
				first = result.ack.CanonicalBytes()
			} else if !bytes.Equal(first, result.ack.CanonicalBytes()) {
				t.Fatal("duplicate did not return original immutable ACK")
			}
		case <-ctx.Done():
			t.Fatal("bounded concurrent intake did not finish")
		}
	}
	if facts, audit, asynchronous := syncNoAsyncCounts(t, fx, op); facts != 4 || audit != 1 || asynchronous != 0 {
		t.Fatal("atomic facts/audit missing or artificial followup created", facts, audit, asynchronous)
	}
	var revision int64
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT revision FROM org_sync_test_targets WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), fx.target).Scan(&revision); err != nil || revision != 2 {
		t.Fatal("duplicate reapplied actual owning effect", err)
	}
}

func TestSyncNoAsyncClosedPreflightUndeclaredReviewAndUnsupportedSchema(t *testing.T) {
	fx := syncFixture(t)
	config, handler := syncNoAsync(t, fx)
	op := syncOperation(t, fx, onboardingID(t), "Synthetic incomplete followup declaration", nil, nil, nil)
	missing := config
	missing.NoAsyncPlans = map[syncapp.OperationKey]map[string]syncapp.FollowupMode{{Type: syncTestType, Schema: 1}: {protocol.Accepted: syncapp.NoAsyncFollowup}}
	var sourceCalls atomic.Int64
	missing.ConfirmSource = func(context.Context, pgx.Tx, syncapp.Source) (time.Time, error) {
		sourceCalls.Add(1)
		return time.Time{}, syncapp.ErrUnavailable
	}
	if _, err := syncpg.NewStore(missing).Process(t.Context(), fx.source, op, protocol.DefaultLimits(), handler); !errors.Is(err, syncapp.ErrUnavailable) || sourceCalls.Load() != 0 {
		t.Fatal("partial result declaration reached source/claim", err)
	}
	if facts, audit, async := syncNoAsyncCounts(t, fx, op); facts+audit+async != 0 {
		t.Fatal("missing producer created a durable claim")
	}
	store := syncpg.NewStore(config)
	review := syncOperation(t, fx, onboardingID(t), "Synthetic orphan cannot manufacture an unavailable review producer", nil, nil, []protocol.SoftReference{{Type: "setting", ID: onboardingID(t)}})
	if _, err := store.Process(t.Context(), fx.source, review, protocol.DefaultLimits(), handler); !errors.Is(err, syncapp.ErrUnavailable) {
		t.Fatal("undeclared held result committed", err)
	}
	if facts, audit, async := syncNoAsyncCounts(t, fx, review); facts+audit+async != 0 {
		t.Fatal("undeclared review left provisional claim/record/audit")
	}
	var raw map[string]any
	if err := json.Unmarshal(op.CanonicalBytes(), &raw); err != nil {
		t.Fatal(err)
	}
	delete(raw, "payload_digest")
	raw["operation_schema"] = 2
	body, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := protocol.NewBinding(fx.source.Actor.AccountID, fx.source.Origin, fx.scope.OrgID().String())
	if err != nil {
		t.Fatal(err)
	}
	unsupported, err := protocol.Seal(body, binding)
	if err != nil {
		t.Fatal(err)
	}
	fx.handler.afterApply = func(context.Context, syncapp.DBTX) error {
		t.Error("unsupported schema reached owning Apply")
		return syncapp.ErrUnavailable
	}
	ack, err := store.Process(t.Context(), fx.source, unsupported, protocol.DefaultLimits(), handler)
	if err != nil || ack.Result() != protocol.Rejected || ack.Code() != "MALFORMED" {
		t.Fatal("registered v1 source could not atomically refuse unknown schema", err)
	}
	if facts, audit, async := syncNoAsyncCounts(t, fx, unsupported); facts != 2 || audit != 1 || async != 0 {
		t.Fatal("schema refusal fabricated an effect or lost immutable audit/ACK", facts, audit, async)
	}
}

func TestSyncNoAsyncAuditAndDeferredCommitFailuresRollbackAllFacts(t *testing.T) {
	for _, stage := range []string{"audit", "commit"} {
		t.Run(stage, func(t *testing.T) {
			fx := syncFixture(t)
			config, handler := syncNoAsync(t, fx)
			store := syncpg.NewStore(config)
			op := syncOperation(t, fx, onboardingID(t), "Synthetic atomic command subject to real SQL failure", nil, nil, nil)
			query := `CREATE FUNCTION sync_no_async_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic transaction failure';END $$;`
			if stage == "audit" {
				query += `CREATE TRIGGER sync_no_async_failure BEFORE INSERT ON org_audit_events FOR EACH ROW EXECUTE FUNCTION sync_no_async_failure()`
			} else {
				query += `CREATE CONSTRAINT TRIGGER sync_no_async_failure AFTER INSERT ON org_sync_acknowledgements DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION sync_no_async_failure()`
			}
			if _, err := fx.f.Admin.Exec(t.Context(), query); err != nil {
				t.Fatal(err)
			}
			if ack, err := store.Process(t.Context(), fx.source, op, protocol.DefaultLimits(), handler); !errors.Is(err, syncapp.ErrUnavailable) || ack.IsFinal() {
				t.Fatal("failed transaction exposed final receipt", err)
			}
			if facts, audit, async := syncNoAsyncCounts(t, fx, op); facts+audit+async != 0 {
				t.Fatal("failed audit/commit left provisional fact", facts, audit, async)
			}
			var revision int64
			if err := fx.f.Admin.QueryRow(t.Context(), `SELECT revision FROM org_sync_test_targets WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), fx.target).Scan(&revision); err != nil || revision != 1 {
				t.Fatal("failed atomic transaction changed owning target", err)
			}
			table := "org_audit_events"
			if stage == "commit" {
				table = "org_sync_acknowledgements"
			}
			if _, err := fx.f.Admin.Exec(t.Context(), "DROP TRIGGER sync_no_async_failure ON "+table+";DROP FUNCTION sync_no_async_failure()"); err != nil {
				t.Fatal(err)
			}
			if ack, err := store.Process(t.Context(), fx.source, op, protocol.DefaultLimits(), handler); err != nil || ack.Result() != protocol.Accepted {
				t.Fatal("original unchanged intent did not recover", err)
			}
			if facts, audit, async := syncNoAsyncCounts(t, fx, op); facts != 4 || audit != 1 || async != 0 {
				t.Fatal("recovery duplicated or lost actual atomic work", facts, audit, async)
			}
		})
	}
}

func TestSyncNoAsyncCannotReplayUndeclaredRetainedReview(t *testing.T) {
	fx := syncFixture(t)
	op := syncOperation(t, fx, onboardingID(t), "Synthetic actual held review with registered required followup", nil, nil, []protocol.SoftReference{{Type: "setting", ID: onboardingID(t)}})
	original, err := fx.store.Process(t.Context(), fx.source, op, protocol.DefaultLimits(), fx.handler)
	if err != nil || original.Result() != protocol.AcceptedForReview {
		t.Fatal("real owning review fixture was not retained", err)
	}
	beforeFacts, beforeAudit, beforeAsync := syncNoAsyncCounts(t, fx, op)
	config, handler := syncNoAsync(t, fx)
	ack, err := syncpg.NewStore(config).Process(t.Context(), fx.source, op, protocol.DefaultLimits(), handler)
	if !errors.Is(err, syncapp.ErrUnavailable) || ack.IsFinal() {
		t.Fatal("accepted/rejected-only source exposed undeclared cached review without producer", err)
	}
	var persisted []byte
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT canonical_ack FROM org_sync_acknowledgements WHERE org_id=$1 AND operation_id=$2`, fx.scope.OrgID().String(), op.ID()).Scan(&persisted); err != nil || !bytes.Equal(persisted, original.CanonicalBytes()) {
		t.Fatal("refused cached outcome was rewritten", err)
	}
	if facts, audit, async := syncNoAsyncCounts(t, fx, op); facts != beforeFacts || audit != beforeAudit || async != beforeAsync {
		t.Fatal("missing current producer changed immutable retained work")
	}
}
