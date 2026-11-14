//go:build integration

package adapters_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
)

var dimensionStates = map[domain.Dimension][]domain.State{
	domain.Storage:    {domain.Uploading, domain.Received, domain.Stored, domain.Quarantined, domain.StorageFailed, domain.Replaced, domain.Deleted},
	domain.Extraction: {domain.ExtractionNone, domain.ExtractionPending, domain.ExtractionDone, domain.ExtractionFailed},
	domain.Structure:  {domain.NotApplicable, domain.Unchecked, domain.StructureOK, domain.StructureInvalid},
	domain.Conference: {domain.Unconferred, domain.ConferredOK, domain.Divergent, domain.Illegible},
	domain.Fiscal:     {domain.NotConsulted, domain.AuthorizedAsOf, domain.CancelledAsOf, domain.Unavailable},
}

func sqlState(err error) string {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

// scoped runs one statement as the ordinary runtime login in a scoped
// transaction that always rolls back, returning the database outcome.
func (f *docFixture) scoped(t *testing.T, scope kernel.OrgScope, query string, args ...any) error {
	t.Helper()
	probe := errors.New("probe rollback")
	var outcome error
	err := database.WithOrgTx(t.Context(), f.db.Runtime, scope, func(tx pgx.Tx) error {
		_, outcome = tx.Exec(t.Context(), query, args...)
		if outcome != nil {
			return outcome
		}
		// Deferred foreign keys run at commit; check them before rolling back.
		if _, outcome = tx.Exec(t.Context(), "SET CONSTRAINTS ALL IMMEDIATE"); outcome != nil {
			return outcome
		}
		return probe
	})
	if outcome == nil && !errors.Is(err, probe) {
		return err
	}
	return outcome
}

func TestDocumentGraphInDatabaseEqualsTheDomainGraph(t *testing.T) {
	f := newDocFixture(t)
	pairs, allowed := 0, 0
	for dimension, states := range dimensionStates {
		for _, from := range states {
			targets, err := domain.Allowed(dimension, from)
			if err != nil {
				t.Fatal(err)
			}
			for _, to := range states {
				var got bool
				if err := f.db.Runtime.QueryRow(t.Context(), `SELECT org_document_transition_allowed($1,$2,$3)`, string(dimension), string(from), string(to)).Scan(&got); err != nil {
					t.Fatal(err)
				}
				pairs++
				if got {
					allowed++
				}
				if got != slices.Contains(targets, to) {
					t.Fatalf("database graph differs from EMP-DOC-001 for %s %s -> %s", dimension, from, to)
				}
			}
		}
	}
	if pairs != 49+16+16+16+16 || allowed != 8+4+4+12+12 {
		t.Fatal("graph coverage drift", pairs, allowed)
	}
	for _, derived := range []string{"RECONCILIATION", "storage", " STORAGE"} {
		var got bool
		if err := f.db.Runtime.QueryRow(t.Context(), `SELECT org_document_transition_allowed($1,'UNALLOCATED','RECONCILED')`, derived).Scan(&got); err != nil || got {
			t.Fatal("derived or non-canonical dimension commanded", derived)
		}
	}
}

func TestDocumentSchemaGuardsScopeIdentityRevisionsAndRetention(t *testing.T) {
	f := newDocFixture(t)
	raw := syntheticJPEG(t)
	command := declare(t, raw)
	if _, _, err := f.store.Create(t.Context(), f.scopeA, f.driver, command); err != nil {
		t.Fatal(err)
	}
	id, org := command.DocumentID, f.scopeA.OrgID().String()

	// Forced RLS: the ordinary login without the organization setting sees and
	// writes nothing, and the other organization's setting sees nothing.
	var visible int
	if err := f.db.Runtime.QueryRow(t.Context(), `SELECT count(*) FROM org_documents`).Scan(&visible); err != nil || visible != 0 {
		t.Fatal("unscoped session saw private documents", visible, err)
	}
	if tag, err := f.db.Runtime.Exec(t.Context(), `UPDATE org_documents SET storage_state='RECEIVED',storage_revision=2`); err != nil || tag.RowsAffected() != 0 {
		t.Fatal("unscoped session changed private documents", err)
	}
	if n := f.count(t, f.scopeB, `SELECT count(*) FROM org_documents`); n != 0 {
		t.Fatal("other organization saw the document")
	}
	if _, err := f.db.Runtime.Exec(t.Context(), `INSERT INTO org_documents(org_id,id,kind,data_mode,operation_id,created_by,original_sha256,original_bytes,original_type,storage_state,storage_revision,extraction_state,extraction_revision,structure_state,structure_revision,conference_state,conference_revision,reconciliation_state,fiscal_state,fiscal_revision)
		VALUES($1,$2,'RECEIPT','SYNTHETIC',$3,$4,$5,1,'image/jpeg','UPLOADING',1,'NONE',1,'NOT_APPLICABLE',1,'UNCONFERRED',1,'UNALLOCATED','NOT_CONSULTED',1)`,
		org, newID(t), newID(t), f.driver.MembershipID, command.Original.SHA256); sqlState(err) != "42501" {
		t.Fatal("unscoped insert not refused by scope", err)
	}
	insert := `INSERT INTO org_documents(org_id,id,kind,data_mode,operation_id,created_by,original_sha256,original_bytes,original_type,storage_state,storage_revision,extraction_state,extraction_revision,structure_state,structure_revision,conference_state,conference_revision,reconciliation_state,fiscal_state,fiscal_revision)
		VALUES($1,$2,'RECEIPT','SYNTHETIC',$3,$4,$5,1,'image/jpeg',$6,1,'NONE',1,'NOT_APPLICABLE',1,'UNCONFERRED',1,'UNALLOCATED','NOT_CONSULTED',1)`
	if err := f.scoped(t, f.scopeA, insert, f.scopeB.OrgID().String(), newID(t), newID(t), f.driver.MembershipID, command.Original.SHA256, "UPLOADING"); sqlState(err) != "42501" {
		t.Fatal("foreign organization row accepted under scope A", err)
	}
	if err := f.scoped(t, f.scopeA, insert, org, newID(t), newID(t), f.driver.MembershipID, command.Original.SHA256, "UPLOADING"); sqlState(err) != "23503" {
		t.Fatal("document without its creation audit event accepted", err)
	}
	// An existing event of another document cannot create this one: the guard
	// matches action and target before the unique operation key is reached.
	if err := f.scoped(t, f.scopeA, insert, org, newID(t), command.OperationID, f.driver.MembershipID, command.Original.SHA256, "UPLOADING"); sqlState(err) != "23503" {
		t.Fatal("document created under another document's creation event", err)
	}
	if err := f.scoped(t, f.scopeA, insert, org, newID(t), command.OperationID, f.driver.MembershipID, command.Original.SHA256, "STORED"); sqlState(err) != "23514" {
		t.Fatal("document created past its initial state", err)
	}
	for name, probe := range map[string]string{
		"png":            `UPDATE org_documents SET original_type='image/png'`,
		"bytes":          `UPDATE org_documents SET original_bytes=original_bytes+1`,
		"digest":         `UPDATE org_documents SET original_sha256=repeat('0',64)`,
		"creator":        `UPDATE org_documents SET created_by=gen_random_uuid()`,
		"operation":      `UPDATE org_documents SET operation_id=gen_random_uuid()`,
		"created-at":     `UPDATE org_documents SET created_at=created_at-interval '1 day'`,
		"skip-received":  `UPDATE org_documents SET storage_state='STORED',storage_revision=2`,
		"double-step":    `UPDATE org_documents SET storage_state='RECEIVED',storage_revision=3`,
		"same-revision":  `UPDATE org_documents SET storage_state='RECEIVED'`,
		"silent-bump":    `UPDATE org_documents SET extraction_revision=2`,
		"reconciliation": `UPDATE org_documents SET reconciliation_state='RECONCILED'`,
		"replaced":       `UPDATE org_documents SET storage_state='REPLACED',storage_revision=2`,
		"fiscal-no-time": `UPDATE org_documents SET fiscal_state='UNAVAILABLE',fiscal_revision=2`,
		"conference-bad": `UPDATE org_documents SET conference_state='UNCONFERRED',conference_revision=2`,
	} {
		if err := f.scoped(t, f.scopeA, probe+` WHERE org_id=$1 AND id=$2`, org, id); sqlState(err) != "23514" {
			t.Fatal("database guard admitted", name, err)
		}
	}
	// Removal is refused by the retention trigger even for the table owner, and
	// the ordinary login holds no DELETE or TRUNCATE privilege at all. Since
	// EMP-DOC-003 links reference documents, a plain TRUNCATE is refused even
	// earlier (0A000), so the owner probe truncates with CASCADE: the strongest
	// removal still reaches, and is refused by, the retention triggers.
	if _, err := f.db.Admin.Exec(t.Context(), `TRUNCATE org_documents`); sqlState(err) != "0A000" {
		t.Fatal("plain truncate of referenced documents not refused", err)
	}
	for name, probe := range map[string]string{"delete": `DELETE FROM org_documents WHERE org_id=$1`, "truncate": `TRUNCATE org_documents CASCADE`} {
		args := []any{org}
		if name == "truncate" {
			args = nil
		}
		if err := f.scoped(t, f.scopeA, probe, args...); sqlState(err) != "42501" {
			t.Fatal("ordinary login may remove documents", name, err)
		}
		err := func() error {
			tx, err := f.db.Admin.Begin(t.Context())
			if err != nil {
				return err
			}
			defer func() { _ = tx.Rollback(t.Context()) }()
			if _, err := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, org); err != nil {
				return err
			}
			_, err = tx.Exec(t.Context(), probe, args...)
			return err
		}()
		if sqlState(err) != "55000" {
			t.Fatal("retention trigger admitted removal", name, err)
		}
	}
	// Valid independent steps change exactly one dimension and its revision.
	at := time.Now().UTC().Truncate(time.Microsecond)
	for _, step := range []struct {
		query string
		args  []any
	}{
		{`UPDATE org_documents SET storage_state='RECEIVED',storage_revision=2 WHERE org_id=$1 AND id=$2`, nil},
		{`UPDATE org_documents SET storage_state='STORED',storage_revision=3 WHERE org_id=$1 AND id=$2`, nil},
		{`UPDATE org_documents SET conference_state='CONFERRED_OK',conference_revision=2 WHERE org_id=$1 AND id=$2`, nil},
		{`UPDATE org_documents SET fiscal_state='AUTHORIZED_AS_OF',fiscal_revision=2,fiscal_consulted_at=$3 WHERE org_id=$1 AND id=$2`, []any{at}},
		{`UPDATE org_documents SET fiscal_revision=3,fiscal_consulted_at=$3 WHERE org_id=$1 AND id=$2`, []any{at.Add(time.Second)}},
	} {
		if err := database.WithOrgTx(t.Context(), f.db.Runtime, f.scopeA, func(tx pgx.Tx) error {
			_, err := tx.Exec(t.Context(), step.query, append([]any{org, id}, step.args...)...)
			return err
		}); err != nil {
			t.Fatal("valid independent transition refused", step.query, err)
		}
	}
	if err := f.scoped(t, f.scopeA, `UPDATE org_documents SET fiscal_revision=4,fiscal_consulted_at=$3 WHERE org_id=$1 AND id=$2`, org, id, at); sqlState(err) != "23514" {
		t.Fatal("older fiscal consultation accepted", err)
	}
	var storage, conference, fiscal, extraction string
	var revisions [5]int64
	if err := database.WithOrgTx(t.Context(), f.db.Runtime, f.scopeA, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT storage_state,conference_state,fiscal_state,extraction_state,storage_revision,extraction_revision,structure_revision,conference_revision,fiscal_revision FROM org_documents WHERE org_id=$1 AND id=$2`, org, id).
			Scan(&storage, &conference, &fiscal, &extraction, &revisions[0], &revisions[1], &revisions[2], &revisions[3], &revisions[4])
	}); err != nil {
		t.Fatal(err)
	}
	if storage != "STORED" || conference != "CONFERRED_OK" || fiscal != "AUTHORIZED_AS_OF" || extraction != "NONE" || revisions != [5]int64{3, 1, 1, 2, 3} {
		t.Fatal("independent dimensions or revisions drifted", storage, conference, fiscal, extraction, revisions)
	}
	var policies int
	if err := f.db.Admin.QueryRow(t.Context(), `SELECT count(*) FROM pg_class c WHERE c.relname='org_documents' AND c.relrowsecurity AND c.relforcerowsecurity`).Scan(&policies); err != nil || policies != 1 {
		t.Fatal("forced row-level security absent", err)
	}
	var columns []string
	rows, err := f.db.Admin.Query(t.Context(), `SELECT column_name FROM information_schema.columns WHERE table_name='org_documents' ORDER BY ordinal_position`)
	if err != nil {
		t.Fatal(err)
	}
	columns, err = pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	for _, column := range columns {
		for _, forbidden := range []string{"key", "url", "name", "token", "content", "bytes_data", "path"} {
			if strings.Contains(column, forbidden) {
				t.Fatal("document schema stores a storage or content locator", column)
			}
		}
	}
}
