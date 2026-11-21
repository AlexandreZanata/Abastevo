//go:build integration

package adapters_test

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	docpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/adapters"
	docapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	accesspg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
)

func TestReviewedFiscalSourceRecordsOneScopedObservation(t *testing.T) {
	fx, _, _ := lifecycleFixture(t)
	ctx := t.Context()
	org := fx.scope.OrgID().String()
	role, password := fx.f.ProvisionScriptRole(t)
	serving := fx.f.ScriptRuntimePool(t, role, password, 4)
	catalog, err := audit.NewCatalog(docpg.AuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	store := &docpg.Store{Pool: serving, Authority: docpg.CurrentAuthority{Source: accesspg.NewStore(serving), Now: time.Now}, Audit: auditpg.NewStore(serving, catalog)}
	owner := docapp.Actor{AccountID: fx.actor.AccountID, MembershipID: fx.member, Channel: "web"}
	storedDocument := func() string {
		t.Helper()
		command := docapp.Create{OperationID: onboardingID(t), DocumentID: onboardingID(t), Original: docapp.Declaration{
			Kind: docapp.KindReceipt, SHA256: strings.Repeat("ab", 32), Bytes: 1024, MediaType: docapp.MediaTypeJPEG,
		}}
		if _, _, err := store.Create(ctx, fx.scope, owner, command); err != nil {
			t.Fatal(err)
		}
		if err := database.WithOrgTx(ctx, serving, fx.scope, func(tx pgx.Tx) error {
			for index, state := range []string{"RECEIVED", "STORED"} {
				if _, err := tx.Exec(ctx, "UPDATE org_documents SET storage_state=$3,storage_revision=$4 WHERE org_id=$1 AND id=$2",
					org, command.DocumentID, state, index+2); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return command.DocumentID
	}
	doc, otherDoc := storedDocument(), storedDocument()
	connection, approval, credential := onboardingID(t), onboardingID(t), onboardingID(t)
	var observed time.Time
	if err := fx.f.Admin.QueryRow(ctx, `SELECT clock_timestamp()-interval '5 seconds'`).Scan(&observed); err != nil {
		t.Fatal(err)
	}
	ready := observed.Add(-time.Minute)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := fx.f.Admin.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO org_connector_approvals(org_id,id,revision,kind,provider_code,resource_id,credential_ref,maximum_bytes,
 maximum_items,decision,decision_ref,ready_at)
 VALUES($1,$2,1,'FISCAL_LOOKUP','synthetic-fiscal',$3,$4,4096,1,'APPROVED','synthetic-fiscal-review',$5)`,
		org, approval, doc, credential, ready)
	exec(`INSERT INTO org_connections(org_id,id,approval_id,approval_revision,label,state,revision,changed_at)
 VALUES($1,$2,$3,1,'Synthetic fiscal evidence','CONFIGURED',1,$4)`, org, connection, approval, ready)
	foreignScope := fx.f.Organizations[1].Scope(t)
	foreignOrg, foreignConnection := foreignScope.OrgID().String(), onboardingID(t)
	exec(`INSERT INTO org_connector_approvals(org_id,id,revision,kind,provider_code,resource_id,credential_ref,maximum_bytes,
 maximum_items,decision,decision_ref,ready_at)
 SELECT $1,id,revision,kind,provider_code,resource_id,credential_ref,maximum_bytes,maximum_items,decision,decision_ref,ready_at
 FROM org_connector_approvals WHERE (org_id,id,revision)=($2,$3,1)`, foreignOrg, org, approval)
	exec(`INSERT INTO org_connections(org_id,id,approval_id,approval_revision,label,state,revision,changed_at)
 VALUES($1,$2,$3,1,'Foreign synthetic connection','ACTIVE',2,$4)`, foreignOrg, foreignConnection, approval, ready)

	record := func(pool *pgxpool.Pool, operation, document, conn string, connRevision, fiscalRevision int, state, code string, at time.Time) (int64, error) {
		var rows int64
		err := database.WithOrgTx(ctx, pool, fx.scope, func(tx pgx.Tx) error {
			tag, err := tx.Exec(ctx, `INSERT INTO org_document_fiscal_consultations
 (org_id,operation_id,document_id,fiscal_revision,connection_id,connection_revision,status,source_result_code,response_sha256,consulted_at)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, org, operation, document, fiscalRevision, conn, connRevision,
				state, code, strings.Repeat("a", 64), at)
			rows = tag.RowsAffected()
			return err
		})
		return rows, err
	}
	op := onboardingID(t)
	if _, err := serving.Exec(ctx, `SELECT status FROM org_document_fiscal_consultations`); linkSQLState(err) != "42501" {
		t.Fatalf("serving role could read fiscal evidence: %v", err)
	}
	if _, err := record(serving, op, doc, connection, 1, 2, "AUTHORIZED_AS_OF", "TEST_AUTHORIZED", observed); linkSQLState(err) != "42501" {
		t.Fatalf("serving role could insert fiscal evidence: %v", err)
	}
	// Add the recorder grant to this disposable login only. No provider or
	// production recorder is provisioned by the migration.
	exec("GRANT SELECT, INSERT ON org_document_fiscal_consultations TO " + pgx.Identifier{role}.Sanitize())
	if err := database.WithOrgTx(ctx, serving, fx.scope, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE org_documents SET fiscal_state='AUTHORIZED_AS_OF',fiscal_revision=2,fiscal_consulted_at=$3
 WHERE org_id=$1 AND id=$2`, org, doc, observed)
		return err
	}); linkSQLState(err) != "23514" {
		t.Fatalf("recorder login advanced a fiscal status without evidence: %v", err)
	}
	if _, err := record(serving, op, doc, connection, 1, 2, "AUTHORIZED_AS_OF", "TEST_AUTHORIZED", observed); linkSQLState(err) != "23514" {
		t.Fatalf("configured connection could record a result: %v", err)
	}
	exec(`UPDATE org_connections SET state='ACTIVE',revision=2 WHERE org_id=$1 AND id=$2`, org, connection)
	for _, candidate := range []struct {
		label, document, conn string
		at                    time.Time
	}{
		{"wrong document", otherDoc, connection, observed},
		{"foreign connection", doc, foreignConnection, observed},
		{"before review and activation", doc, connection, ready.Add(-time.Second)},
	} {
		if _, err := record(serving, op, candidate.document, candidate.conn, 2, 2, "AUTHORIZED_AS_OF", "TEST_AUTHORIZED", candidate.at); linkSQLState(err) != "23514" {
			t.Fatalf("%s accepted: %v", candidate.label, err)
		}
	}
	if rows, err := record(serving, op, doc, connection, 2, 2, "AUTHORIZED_AS_OF", "TEST_AUTHORIZED", observed); err != nil || rows != 1 {
		t.Fatalf("exact confirmed fiscal result rejected: %d / %v", rows, err)
	}
	if err := database.WithOrgTx(ctx, serving, foreignScope, func(tx pgx.Tx) error {
		var visible int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM org_document_fiscal_consultations`).Scan(&visible); err != nil {
			return err
		}
		if visible != 0 {
			t.Errorf("foreign organization read %d receipts", visible)
		}
		_, err := tx.Exec(ctx, `INSERT INTO org_document_fiscal_consultations
 (org_id,operation_id,document_id,fiscal_revision,connection_id,connection_revision,status,source_result_code,response_sha256,consulted_at)
 VALUES($1,$2,$3,2,$4,2,'AUTHORIZED_AS_OF','TEST_AUTHORIZED',repeat('a',64),$5)`, org, op, doc, connection, observed)
		return err
	}); linkSQLState(err) != "42501" {
		t.Fatalf("foreign scope could target a receipt: %v", err)
	}
	view, err := store.Pending(ctx, fx.scope, owner, doc)
	if err != nil || view.Statuses.Fiscal != domain.AuthorizedAsOf || view.Revisions.Fiscal != 2 ||
		view.FiscalConsultedAt == nil || !observed.Equal(*view.FiscalConsultedAt) || view.Statuses.Storage != domain.Stored ||
		view.Statuses.Extraction != domain.ExtractionNone || view.Statuses.Structure != domain.NotApplicable ||
		view.Statuses.Conference != domain.Unconferred || view.Statuses.Reconciliation != domain.Unallocated {
		t.Fatalf("fiscal projection failed or changed another dimension: %+v / %v", view, err)
	}
	if rows, err := record(serving, op, doc, connection, 2, 2, "AUTHORIZED_AS_OF", "TEST_AUTHORIZED", observed); err != nil || rows != 0 {
		t.Fatalf("same consultation did not converge: %d / %v", rows, err)
	}
	if _, err := record(serving, op, doc, connection, 2, 2, "CANCELLED_AS_OF", "TEST_CANCELLED", observed); linkSQLState(err) != "23505" {
		t.Fatalf("operation identity admitted different provider evidence: %v", err)
	}
	secondOp, competingOp := onboardingID(t), onboardingID(t)
	if _, err := record(serving, secondOp, doc, connection, 2, 3, "CANCELLED_AS_OF", "TEST_CANCELLED", observed.Add(-time.Second)); linkSQLState(err) != "23514" {
		t.Fatalf("an older consultation advanced the fiscal state: %v", err)
	}
	results := make(chan error, 2)
	for _, operation := range []string{secondOp, competingOp} {
		go func() {
			_, err := record(serving, operation, doc, connection, 2, 3, "CANCELLED_AS_OF", "TEST_CANCELLED", observed.Add(time.Second))
			results <- err
		}()
	}
	a, b := <-results, <-results
	if a != nil {
		a, b = b, a
	}
	if a != nil || linkSQLState(b) != "23505" {
		t.Fatalf("competing fiscal commands did not commit exactly once: %v / %v", a, b)
	}
	exec(`INSERT INTO org_connector_approvals(org_id,id,revision,kind,provider_code,resource_id,credential_ref,maximum_bytes,
 maximum_items,decision,decision_ref) VALUES($1,$2,2,'FISCAL_LOOKUP','synthetic-fiscal',$3,$4,4096,1,'WITHDRAWN','synthetic-fiscal-withdrawal')`,
		org, approval, doc, credential)
	if _, err := record(serving, onboardingID(t), doc, connection, 2, 4, "AUTHORIZED_AS_OF", "TEST_AUTHORIZED", observed.Add(2*time.Second)); linkSQLState(err) != "23514" {
		t.Fatalf("withdrawn review authorized an additional consultation: %v", err)
	}
	if rows, err := record(serving, op, doc, connection, 2, 2, "AUTHORIZED_AS_OF", "TEST_AUTHORIZED", observed); err != nil || rows != 0 {
		t.Fatalf("withdrawal changed an existing receipt replay: %d / %v", rows, err)
	}
	view, err = store.Pending(ctx, fx.scope, owner, doc)
	if err != nil || view.Statuses.Fiscal != domain.CancelledAsOf || view.Revisions.Fiscal != 3 ||
		view.FiscalConsultedAt == nil || !observed.Add(time.Second).Equal(*view.FiscalConsultedAt) {
		t.Fatalf("withdrawal or replay altered the latest observation: %+v / %v", view, err)
	}
	var retained int
	if err := fx.f.Admin.QueryRow(ctx, `SELECT count(*) FROM org_document_fiscal_consultations WHERE org_id=$1`, org).Scan(&retained); err != nil || retained != 2 {
		t.Fatalf("consultation history diverged: %d / %v", retained, err)
	}
}
