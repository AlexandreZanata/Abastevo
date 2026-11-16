//go:build integration

package adapters_test

import (
	"context"
	"io/fs"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

const expenseLinkMigration = "000257_organization_expense_document_links.sql"

// EMP-EXP-004 (EDR-338): a populated head before 000257 (an accepted direct
// expense and an accepted fueling with its expense, a document with an ASSET
// link and one with a FUELING link) upgrades through an ordinary
// non-superuser migrator: an injected failure changes no schema, constraint
// or data, recovery applies once, replay is a no-op, the previous embedded
// image stays compatible, the old links keep their bindings, and the new
// EXPENSE kind is carried only by its typed same-organization reference.
func TestExpenseLinkPopulatedUpgradeFailureRecoveryAndPreviousImage(t *testing.T) {
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() != "schema_min_supported" && (entry.Name() >= expenseLinkMigration || !strings.HasSuffix(entry.Name(), ".sql")) {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	if _, ok := previous["000245_organization_fueling_document_links.sql"]; !ok {
		t.Fatal("previous image does not reach the 000245 head")
	}
	f := orgtest.NewWithMigrations(t, previous)
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	x := expenseFixtureFrom(t, fuelingFixtureOn(t, selfIntakeFixtureOn(t, syncFixtureOn(t, devicesFixtureOn(t, legalFixtureFor(t, f, onboarding, actor, flags)))), nil))
	op := x.op(t, x.syncFixtureState, x.declaredNow(t), nil)
	if ack, err := x.process(t, x.syncFixtureState, op); err != nil || ack.Result() != protocol.Accepted {
		t.Fatal("populated fueling not accepted", err)
	}
	fueling := op.Snapshot().Entity.ID
	direct := x.directOp(t, x.syncFixtureState, x.declaredNow(t), nil)
	ack, err := x.processDirect(t, x.syncFixtureState, direct)
	x.directAccepted(t, direct, ack, err)
	expense := direct.Snapshot().Entity.ID
	org := x.scope.OrgID().String()
	document, linkOp, fuelingDocument := onboardingID(t), onboardingID(t), onboardingID(t)
	if err := pgx.BeginFunc(t.Context(), f.Admin, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `SET LOCAL session_replication_role=replica`); err != nil {
			return err
		}
		if _, err := tx.Exec(t.Context(), `INSERT INTO org_documents(org_id,id,kind,data_mode,operation_id,created_by,original_sha256,original_bytes,original_type,storage_state,storage_revision,extraction_state,extraction_revision,
 structure_state,structure_revision,conference_state,conference_revision,reconciliation_state,fiscal_state,fiscal_revision)
VALUES($1,$2,'RECEIPT','SYNTHETIC',$3,$4,repeat('ab',32),1024,'image/jpeg','UPLOADING',1,'NONE',1,'NOT_APPLICABLE',1,'UNCONFERRED',1,'UNALLOCATED','NOT_CONSULTED',1)`, org, document, onboardingID(t), x.fuelingState.member); err != nil {
			return err
		}
		if _, err := tx.Exec(t.Context(), `INSERT INTO org_documents(org_id,id,kind,data_mode,operation_id,created_by,original_sha256,original_bytes,original_type,storage_state,storage_revision,extraction_state,extraction_revision,
 structure_state,structure_revision,conference_state,conference_revision,reconciliation_state,fiscal_state,fiscal_revision)
VALUES($1,$2,'RECEIPT','SYNTHETIC',$3,$4,repeat('cd',32),2048,'image/jpeg','UPLOADING',1,'NONE',1,'NOT_APPLICABLE',1,'UNCONFERRED',1,'UNALLOCATED','NOT_CONSULTED',1)`, org, fuelingDocument, onboardingID(t), x.fuelingState.member); err != nil {
			return err
		}
		if _, err := tx.Exec(t.Context(), `INSERT INTO org_document_links(org_id,document_id,target_kind,target_id,asset_id,operation_id,created_by) VALUES($1,$2,'ASSET',$3,$3,$4,$5)`, org, document, x.vehicle.AssetID, linkOp, x.fuelingState.member); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), `INSERT INTO org_document_links(org_id,document_id,target_kind,target_id,fueling_id,operation_id,created_by) VALUES($1,$2,'FUELING',$3,$3,$4,$5)`, org, fuelingDocument, fueling, onboardingID(t), x.fuelingState.member)
		return err
	}); err != nil {
		t.Fatal("populated document link", err)
	}
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_fuelings x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_expenses x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_meter_readings x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_acknowledgements x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_audit_events x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_documents x),
(SELECT jsonb_agg(to_jsonb(x)-'expense_id' ORDER BY org_id,document_id)FROM org_document_links x))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	constraints := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT string_agg(conname||'='||pg_get_constraintdef(oid),';' ORDER BY conname) FROM pg_constraint WHERE conrelid='org_document_links'::regclass`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before, beforeConstraints := snapshot(), constraints()
	migrator := f.RuntimeRole + "_expense_link_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	// The production migration owner owns the link table and holds REFERENCES on the expense parent.
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+
		";GRANT REFERENCES ON org_expenses TO "+role+";ALTER TABLE org_document_links OWNER TO "+role); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := f.Admin.Exec(ctx, "REASSIGN OWNED BY "+role+" TO CURRENT_USER;DROP OWNED BY "+role+";DROP ROLE "+role); err != nil {
			t.Error(err)
		}
	})
	location, err := url.Parse(f.AdminDSN)
	if err != nil {
		t.Fatal(err)
	}
	query := location.Query()
	query.Set("options", "-c role="+migrator)
	location.RawQuery = strings.ReplaceAll(query.Encode(), "+", "%20")
	raw, err := fs.ReadFile(migrations.Files, expenseLinkMigration)
	if err != nil {
		t.Fatal(err)
	}
	version := strings.SplitN(expenseLinkMigration, "_", 2)[0]
	broken := fstest.MapFS{expenseLinkMigration: &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("injected expense link migration failure not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='org_document_links' AND column_name='expense_id')
 AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&clean); err != nil || !clean || before != snapshot() || beforeConstraints != constraints() {
		t.Fatal("failed expense link migration changed the schema, a constraint or data", err)
	}
	current := fstest.MapFS{}
	for path, data := range previous {
		current[path] = data
	}
	current[expenseLinkMigration] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != version {
		t.Fatal("ordinary expense link migration recovery", applied, err)
	}
	if before != snapshot() {
		t.Fatal("expense link upgrade rewrote existing state")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("expense link migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	// The kind and binding checks name EXPENSE with its own column; every older kind keeps its binding;
	// the reference is a composite same-organization foreign key to the expense.
	var widened bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT b LIKE '%''EXPENSE''%expense_id%' AND b LIKE '%''FUELING''%fueling_id%' AND b LIKE '%''INCIDENT''%incident_report_id%'
 AND b LIKE '%''ASSET''%asset_id%' AND b LIKE '%''METER_READING''%held_reading_id%' AND k LIKE '%EXPENSE%' AND k LIKE '%FUELING%' AND k LIKE '%INCIDENT%'
 AND EXISTS(SELECT 1 FROM pg_constraint c WHERE c.conname='org_document_links_expense_fkey' AND c.confrelid='org_expenses'::regclass AND c.contype='f'
  AND pg_get_constraintdef(c.oid) LIKE 'FOREIGN KEY (org_id, expense_id) REFERENCES org_expenses(org_id, id)')
 AND EXISTS(SELECT 1 FROM pg_constraint c WHERE c.conname='org_document_links_fueling_fkey' AND c.confrelid='org_fuelings'::regclass AND c.contype='f')
 AND(SELECT relrowsecurity AND relforcerowsecurity FROM pg_class WHERE oid='org_document_links'::regclass)
 FROM(SELECT pg_get_constraintdef((SELECT oid FROM pg_constraint WHERE conname='org_document_links_binding')) b,
 pg_get_constraintdef((SELECT oid FROM pg_constraint WHERE conname='org_document_links_target_kind_check')) k) s`).Scan(&widened); err != nil || !widened {
		t.Fatal("upgraded link checks or foreign key", err)
	}
	// An EXPENSE row must be carried by exactly its typed reference (checks hold even with triggers off).
	for name, insert := range map[string]string{
		"expense-without-reference": `INSERT INTO org_document_links(org_id,document_id,target_kind,target_id,operation_id,created_by) VALUES($1,$2,'EXPENSE',$3,$4,$5)`,
		"expense-with-fueling":      `INSERT INTO org_document_links(org_id,document_id,target_kind,target_id,expense_id,fueling_id,operation_id,created_by) VALUES($1,$2,'EXPENSE',$3,$3,$3,$4,$5)`,
		"fueling-with-expense":      `INSERT INTO org_document_links(org_id,document_id,target_kind,target_id,fueling_id,expense_id,operation_id,created_by) VALUES($1,$2,'FUELING',$3,$3,$3,$4,$5)`,
		"asset-with-expense":        `INSERT INTO org_document_links(org_id,document_id,target_kind,target_id,asset_id,expense_id,operation_id,created_by) VALUES($1,$2,'ASSET',$3,$3,$3,$4,$5)`,
	} {
		err := pgx.BeginFunc(t.Context(), f.Admin, func(tx pgx.Tx) error {
			if _, err := tx.Exec(t.Context(), `SET LOCAL session_replication_role=replica`); err != nil {
				return err
			}
			_, err := tx.Exec(t.Context(), insert, org, document, expense, onboardingID(t), x.fuelingState.member)
			return err
		})
		if err == nil || !strings.Contains(err.Error(), "23514") {
			t.Fatal("binding check accepted", name, err)
		}
	}
	if _, err := migrate.Apply(t.Context(), f.AdminDSN, migrations.Files); err != nil {
		t.Fatal("remaining embedded chain after the 257 rehearsal", err)
	}
	var kept bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT count(*)=2 AND bool_and(expense_id IS NULL AND CASE target_kind WHEN 'ASSET' THEN asset_id=target_id AND fueling_id IS NULL
 WHEN 'FUELING' THEN fueling_id=target_id AND fueling_id=$2 AND asset_id IS NULL ELSE false END) FROM org_document_links WHERE org_id=$1`, org, fueling).Scan(&kept); err != nil || !kept {
		t.Fatal("the populated links lost their bindings", err)
	}
}
