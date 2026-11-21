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

func TestFuelingReviewUpgradePreservesPopulatedHistoryAndRecovers(t *testing.T) {
	const migration = "000279_organization_fueling_review_decisions.sql"
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || e.Name() != "schema_min_supported" && (e.Name() >= migration || !strings.HasSuffix(e.Name(), ".sql")) {
			continue
		}
		data, err := fs.ReadFile(migrations.Files, e.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[e.Name()] = &fstest.MapFile{Data: data}
	}
	f := orgtest.NewWithMigrations(t, previous)
	ctx := t.Context()
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	x := fuelingFixtureOn(t, selfIntakeFixtureOn(t, syncFixtureOn(t, devicesFixtureOn(t, legalFixtureFor(t, f, onboarding, actor, flags)))), nil)
	op := x.op(t, x.syncFixtureState, x.declaredNow(t), nil)
	accepted, err := x.process(t, x.syncFixtureState, op)
	if err != nil || accepted.Result() != protocol.Accepted {
		t.Fatalf("prior image did not create the signed fueling: %v", err)
	}
	migrator := f.RuntimeRole + "_fueling_review_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, err := f.Admin.Exec(ctx, "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS; GRANT USAGE,CREATE ON SCHEMA public TO "+role+
		"; GRANT SELECT,INSERT ON schema_migrations TO "+role+"; GRANT REFERENCES ON org_audit_events TO "+role+
		"; ALTER TABLE org_fueling_review_entries OWNER TO "+role); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := f.Admin.Exec(ctx, "REASSIGN OWNED BY "+role+" TO CURRENT_USER; DROP OWNED BY "+role+"; DROP ROLE "+role); err != nil {
			t.Error(err)
		}
	})
	location, err := url.Parse(f.AdminDSN)
	if err != nil {
		t.Fatal(err)
	}
	options := location.Query()
	options.Set("options", "-c role="+migrator)
	location.RawQuery = strings.ReplaceAll(options.Encode(), "+", "%20")
	raw, err := fs.ReadFile(migrations.Files, migration)
	if err != nil {
		t.Fatal(err)
	}
	broken := fstest.MapFS{migration: &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(ctx, location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatalf("forced failure did not occur after the migration: %v", err)
	}
	var untouched bool
	if err := f.Admin.QueryRow(ctx, `SELECT NOT EXISTS (SELECT 1 FROM schema_migrations WHERE version='000279')
 AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='org_fueling_review_entries' AND column_name='warning_notes')`).Scan(&untouched); err != nil || !untouched {
		t.Fatalf("failed migration changed the prior image: %v", err)
	}
	previous[migration] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(ctx, location.String(), previous); err != nil || len(applied) != 1 || applied[0] != "000279" {
		t.Fatalf("populated migration recovery failed: %v %v", applied, err)
	}
	replay, err := x.process(t, x.syncFixtureState, op)
	if err != nil || string(replay.CanonicalBytes()) != string(accepted.CanonicalBytes()) {
		t.Fatalf("upgrade changed the retained receipt: %v", err)
	}
	x.accepted(t, op, replay, nil, 0, 1)
}
