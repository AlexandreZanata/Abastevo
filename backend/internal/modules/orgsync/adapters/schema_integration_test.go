//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestIntakeSchemaRejectsInvalidClaimsAndEmptyMutation(t *testing.T) {
	f := orgtest.New(t)
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, e := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE,DELETE,TRUNCATE ON org_sync_operations,org_sync_acknowledgements,org_sync_effects TO "+role); e != nil {
		t.Fatal(e)
	}
	// TRUNCATE ... CASCADE reaches every referencing history table (for example
	// 199's review items, held facts and resolutions). Grant the whole closure so
	// the append-only triggers, not a missing privilege, must refuse it.
	var closure string
	if e := f.Admin.QueryRow(t.Context(), `WITH RECURSIVE r(oid) AS(SELECT 'org_sync_operations'::regclass::oid UNION SELECT c.conrelid FROM pg_constraint c JOIN r ON c.confrelid=r.oid WHERE c.contype='f') SELECT string_agg(format('%I',relname),',') FROM pg_class WHERE oid IN(SELECT oid FROM r)`).Scan(&closure); e != nil || closure == "" {
		t.Fatal("truncate closure unavailable", e)
	}
	if _, e := f.Admin.Exec(t.Context(), "GRANT TRUNCATE ON "+closure+" TO "+role); e != nil {
		t.Fatal(e)
	}
	for _, table := range []string{"org_sync_operations", "org_sync_acknowledgements", "org_sync_effects"} {
		var forced bool
		if e := f.Admin.QueryRow(t.Context(), `SELECT relrowsecurity AND relforcerowsecurity FROM pg_class WHERE oid=$1::regclass`, table).Scan(&forced); e != nil || !forced {
			t.Fatal("intake RLS missing", e)
		}
		for _, verb := range []string{"UPDATE " + table + " SET org_id=org_id WHERE false", "DELETE FROM " + table + " WHERE false", "TRUNCATE " + table + " CASCADE"} {
			e := database.WithOrgTx(t.Context(), f.Runtime, f.Organizations[0].Scope(t), func(tx pgx.Tx) error { _, e := tx.Exec(t.Context(), verb); return e })
			var p *pgconn.PgError
			if !errors.As(e, &p) || p.Code != "55000" {
				t.Fatal("intake append-only mutation admitted", table, e)
			}
		}
	}
	e := database.WithOrgTx(t.Context(), f.Runtime, f.Organizations[0].Scope(t), func(tx pgx.Tx) error {
		_, e := tx.Exec(context.Background(), `INSERT INTO org_sync_operations(org_id,operation_id,owner_account_id,api_origin,membership_id,enrollment_id,operation_type,operation_schema,entity_type,entity_id,payload_digest) VALUES($1,'7b0e2a54-0000-4000-8000-000000000001','00000000-0000-0000-0000-000000000000','https://api.example.invalid','7b0e2a54-0000-4000-8000-000000000002','7b0e2a54-0000-4000-8000-000000000003','example.record',1,'record','7b0e2a54-0000-4000-8000-000000000004','sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa')`, f.Organizations[0].ID)
		return e
	})
	var p *pgconn.PgError
	if !errors.As(e, &p) || p.Code != "23514" {
		t.Fatal("invalid claim was not constrained", e)
	}
}
