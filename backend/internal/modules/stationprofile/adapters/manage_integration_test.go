//go:build integration

package adapters

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	stationprofile "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/stationprofile"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func freshManageDB(t *testing.T) (*pgxpool.Pool, Store, ReviewDecisions, string) {
	t.Helper()
	adminDSN := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if adminDSN == "" {
		adminDSN = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("manage_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		admin, err := pgx.Connect(ctx, adminDSN)
		if err != nil {
			t.Errorf("admin connect for drop: %v", err)
			return
		}
		defer admin.Close(ctx)
		if _, err := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
			t.Errorf("drop database: %v", err)
		}
	})
	parsed, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	parsed.Path = "/" + name
	dsn := parsed.String()
	if _, err := migrate.Apply(ctx, dsn, dbmigrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	queries := stationprofile.New(pool)
	var stationID string
	if err := pool.QueryRow(ctx, "INSERT INTO directory_stations (id, display_name) VALUES (gen_random_uuid(), 'Posto Gestao') RETURNING id::text").Scan(&stationID); err != nil {
		t.Fatalf("seed station: %v", err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO station_operator_revisions (id, station_id, cnpj, source) VALUES (gen_random_uuid(), $1, '04218406000104', 'registry')", stationID); err != nil {
		t.Fatalf("seed operator: %v", err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO station_profiles (station_id, policy_version) VALUES ($1, 'profile-v1') ON CONFLICT DO NOTHING", stationID); err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	return pool, Store{Q: queries}, ReviewDecisions{Pool: pool}, stationID
}

func seedGrant(t *testing.T, pool *pgxpool.Pool, accountID, stationID string) string {
	t.Helper()
	grantID := "00000000-0000-4000-8000-000000000001"
	claimID := "00000000-0000-4000-8000-000000000002"
	decisionID := "00000000-0000-4000-8000-000000000003"
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "INSERT INTO profile_claims (id, account_id, station_id, role, policy_version, state) VALUES ($1, $2, $3, 'administrator', 'profile-v1', 'approved')",
		claimID, accountID, stationID); err != nil {
		t.Fatalf("seed claim: %v", err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO claim_decisions (id, claim_id, reviewer, decision, reason, policy_version, operator_cnpj, scopes) VALUES ($1, $2, 'op-1', 'approved', 'seed', 'profile-v1', '04218406000104', 'profile.edit,reply.official')",
		decisionID, claimID); err != nil {
		t.Fatalf("seed decision: %v", err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO representation_grants (id, account_id, station_id, operator_cnpj, role, scopes, claim_id, decision_id) VALUES ($1, $2, $3, '04218406000104', 'administrator', 'profile.edit,reply.official', $4, $5)",
		grantID, accountID, stationID, claimID, decisionID); err != nil {
		t.Fatalf("seed grant: %v", err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO station_profiles (station_id, policy_version) VALUES ($1, 'profile-v1') ON CONFLICT DO NOTHING", stationID); err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	return grantID
}

func managePortsForTest(store Store, decisions ReviewDecisions) application.ManagePorts {
	return application.ManagePorts{
		Grants:   decisions,
		Profiles: store,
		OperatorOf: func(ctx context.Context, stationID string) (string, bool, error) {
			op, found, err := store.CurrentOperator(ctx, stationID)
			if err != nil || !found {
				return "", false, err
			}
			return op.CNPJ, true, nil
		},
		AccountLive: func(_ context.Context, _ string) (bool, error) { return true, nil },
		SubmitReply: func(context.Context, string, string, string, string) (string, error) {
			return "comment-1", nil
		},
		Attribute: func(context.Context, string, string, string, string) error { return nil },
	}
}

func TestManageIntegrationEditAndReply(t *testing.T) {
	pool, store, decisions, stationID := freshManageDB(t)
	ctx := context.Background()
	acc := "11111111-1111-4111-8111-111111111111"
	seedGrant(t, pool, acc, stationID)
	ports := managePortsForTest(store, decisions)

	updated, err := application.EditBusinessFields(ctx, ports, acc, stationID, 1, map[string]string{
		"phone": "+55-11-99999-0000", "services": "fuel,convenience",
	})
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if updated.Revision != 2 || updated.Business["services"] != "fuel,convenience" {
		t.Fatalf("updated = %+v", updated)
	}
	commentID, err := application.PostOfficialReply(ctx, ports, acc, stationID, "ETHANOL", "Obrigado pela visita!")
	if err != nil || commentID != "comment-1" {
		t.Fatalf("reply = %q, err = %v", commentID, err)
	}
}

func TestManageIntegrationStaleAndConcurrent(t *testing.T) {
	pool, store, decisions, stationID := freshManageDB(t)
	ctx := context.Background()
	acc := "11111111-1111-4111-8111-111111111111"
	seedGrant(t, pool, acc, stationID)
	ports := managePortsForTest(store, decisions)

	// Operator succession denies even with a live grant.
	if _, err := pool.Exec(ctx, "UPDATE station_operator_revisions SET valid_to = now() WHERE station_id = $1 AND valid_to IS NULL", stationID); err != nil {
		t.Fatalf("close operator: %v", err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO station_operator_revisions (id, station_id, cnpj, source) VALUES (gen_random_uuid(), $1, '00428184000195', 'dou')", stationID); err != nil {
		t.Fatalf("rotate operator: %v", err)
	}
	if _, err := application.EditBusinessFields(ctx, ports, acc, stationID, 1, map[string]string{"phone": "x"}); err == nil {
		t.Fatal("stale operator must deny")
	}
	// Restore the operator, establish revision 2, then race two
	// concurrent edits on it: exactly one wins.
	if _, err := pool.Exec(ctx, "UPDATE station_operator_revisions SET valid_to = now() WHERE station_id = $1 AND valid_to IS NULL", stationID); err != nil {
		t.Fatalf("close operator: %v", err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO station_operator_revisions (id, station_id, cnpj, source) VALUES (gen_random_uuid(), $1, '04218406000104', 'registry')", stationID); err != nil {
		t.Fatalf("restore operator: %v", err)
	}
	if _, err := application.EditBusinessFields(ctx, ports, acc, stationID, 1, map[string]string{"phone": "a"}); err != nil {
		t.Fatalf("first edit: %v", err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = application.EditBusinessFields(ctx, ports, acc, stationID, 2, map[string]string{"phone": "race"})
		}(i)
	}
	wg.Wait()
	wins := 0
	for _, err := range errs {
		if err == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("wins = %d, errs = %v", wins, errs)
	}
}

func TestGrantValidityWindowBlocksManagementIntegration(t *testing.T) {
	pool, store, decisions, stationID := freshManageDB(t)
	ctx := context.Background()
	accountID := "11111111-1111-4111-8111-111111111111"
	grantID := seedGrant(t, pool, accountID, stationID)
	ports := managePortsForTest(store, decisions)
	for _, update := range []string{
		"UPDATE representation_grants SET valid_to=now()-interval '1 second' WHERE id=$1::uuid",
		"UPDATE representation_grants SET valid_to=NULL, valid_from=now()+interval '1 hour' WHERE id=$1::uuid",
	} {
		if _, err := pool.Exec(ctx, update, grantID); err != nil {
			t.Fatal(err)
		}
		if _, err := application.EditBusinessFields(ctx, ports, accountID, stationID, 1, map[string]string{"phone": "public"}); err == nil {
			t.Fatal("expired/future grant edited profile")
		}
	}
}

func TestPublicBadgeRequiresLiveAccountOperatorAndGrantIntegration(t *testing.T) {
	pool, _, decisions, stationID := freshManageDB(t)
	ctx := context.Background()
	accountID := "11111111-1111-4111-8111-111111111111"
	grantID := seedGrant(t, pool, accountID, stationID)
	check := func(cnpj string, live bool, want bool) {
		t.Helper()
		badge, err := decisions.HasRepresentation(ctx, stationID, cnpj, func(context.Context, string) (bool, error) { return live, nil })
		if err != nil || badge != want {
			t.Fatalf("badge=%v want=%v error=%v", badge, want, err)
		}
	}
	check("04218406000104", true, true)
	check("04218406000104", false, false)
	check("other-operator", true, false)
	for _, status := range []string{"suspended", "revoked", "expired"} {
		if _, err := pool.Exec(ctx, "UPDATE representation_grants SET status=$2 WHERE id=$1::uuid", grantID, status); err != nil {
			t.Fatal(err)
		}
		check("04218406000104", true, false)
	}
	if _, err := pool.Exec(ctx, "UPDATE representation_grants SET status='active', valid_to=now()-interval '1 second' WHERE id=$1::uuid", grantID); err != nil {
		t.Fatal(err)
	}
	check("04218406000104", true, false)
}
