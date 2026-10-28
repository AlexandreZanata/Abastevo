//go:build integration

package seeds

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	contributorq "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/contributor"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func freshDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("explicit disposable PostGIS DSN required")
	}
	u, err := url.Parse(dsn)
	if err != nil || u.Hostname() != "127.0.0.1" {
		t.Fatal("literal loopback test DSN required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal("disposable database unavailable")
	}
	name := fmt.Sprintf("seeds_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		defer admin.Close(context.Background())
		if _, err := admin.Exec(context.Background(), "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	})
	u.Path = "/" + name
	if _, err = migrate.Apply(ctx, u.String(), migrations.Files); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// TestProjectCreatorSeed applies the seed twice on the real schema and reads
// it back through the public contributors query the app uses: the creator is
// listed first with the GitHub handle and the "Criador do projeto" badge.
func TestProjectCreatorSeed(t *testing.T) {
	pool := freshDB(t)
	ctx := context.Background()
	const other = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	for _, stmt := range []string{
		`INSERT INTO accounts(id,alias,status) VALUES('` + other + `','synthetic-later','active')`,
		`INSERT INTO contributor_profiles(account_id,display_name,visibility,public_consent) VALUES('` + other + `','Synthetic Later','PUBLIC',true)`,
		`INSERT INTO entitlement_grants(id,account_id,tier,policy_version,state,start_at,reason)
		 VALUES('bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb','` + other + `','CONTRIBUTOR',1,'ACTIVE','2026-10-01T00:00:00Z','synthetic later contributor')`,
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	for run := 0; run < 2; run++ {
		if _, err := pool.Exec(ctx, ProjectCreator); err != nil {
			t.Fatalf("seed run %d: %v", run+1, err)
		}
	}
	var grants, roles int
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM entitlement_grants WHERE account_id='c4ea7000-0000-4000-8000-000000000001'),
		(SELECT count(*) FROM staff_assignments WHERE account_id='c4ea7000-0000-4000-8000-000000000001')`).Scan(&grants, &roles); err != nil || grants != 1 || roles != 1 {
		t.Fatalf("not idempotent: grants=%d roles=%d err=%v", grants, roles, err)
	}
	rows, err := contributorq.New(pool).PublicContributors(ctx, contributorq.PublicContributorsParams{
		Now: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}, RowLimit: 10,
	})
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows=%d err=%v", len(rows), err)
	}
	first := rows[0]
	var badges []string
	if err := json.Unmarshal(first.Roles, &badges); err != nil {
		t.Fatal(err)
	}
	if first.DisplayName != "Alexandre Zanata" || first.GithubLogin.String != "AlexandreZanata" ||
		len(badges) != 1 || badges[0] != "project_creator" {
		t.Fatalf("first=%s github=%s roles=%v", first.DisplayName, first.GithubLogin.String, badges)
	}
}
