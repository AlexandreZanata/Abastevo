//go:build integration

package adapters

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("integration database unreachable at %s: %v (start it: docker compose -f infra/compose.dev.yml up -d db)", dsn, err)
	}
	defer conn.Close(ctx)
	return dsn
}

func freshStore(t *testing.T) (*Store, *pgxpool.Pool) {
	t.Helper()
	adminDSN := testDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("moderation_test_%d", time.Now().UnixNano())
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
	u, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	u.Path = "/" + name
	dsn := u.String()
	if _, err := migrate.Apply(ctx, dsn, dbmigrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return NewStore(pool), pool
}

func mustCase(t *testing.T, id, targetType, targetID, priority, reason string, at time.Time) domain.Case {
	t.Helper()
	c, _, err := domain.NewCase(domain.CaseParams{
		ID: id, TargetType: targetType, TargetID: targetID,
		Priority: priority, Reason: reason, OpenedAt: at,
	})
	if err != nil {
		t.Fatalf("new case: %v", err)
	}
	return c
}

func TestOpenCaseConvergesDuplicate(t *testing.T) {
	s, _ := freshStore(t)
	ctx := context.Background()
	base := time.Now().Truncate(time.Millisecond)
	first := mustCase(t, "c0000000-0000-4000-8000-000000000001",
		domain.TargetObservation, "b0000000-0000-4000-8000-000000000001",
		domain.PriorityP2, "first report", base)
	id, replayed, err := s.OpenCase(ctx, first)
	if err != nil || replayed || id != first.ID {
		t.Fatalf("first open = %q %v %v", id, replayed, err)
	}
	second := mustCase(t, "c0000000-0000-4000-8000-000000000002",
		domain.TargetObservation, "b0000000-0000-4000-8000-000000000001",
		domain.PriorityP1, "second report, same target", base.Add(time.Second))
	id2, replayed2, err := s.OpenCase(ctx, second)
	if err != nil {
		t.Fatalf("duplicate open: %v", err)
	}
	if !replayed2 || id2 != first.ID {
		t.Errorf("duplicate did not converge: id=%q replayed=%v want %q true", id2, replayed2, first.ID)
	}
	got, err := s.Get(ctx, first.ID)
	if err != nil || got.Reason != "first report" || got.Priority != domain.PriorityP2 {
		t.Errorf("converged case mutated: %+v %v", got, err)
	}
}

func TestConcurrentOpenConverges(t *testing.T) {
	s, pool := freshStore(t)
	ctx := context.Background()
	base := time.Now().Truncate(time.Millisecond)
	const workers = 8
	ids := make([]string, workers)
	replayed := make([]bool, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			c := mustCase(t, fmt.Sprintf("c0000000-0000-4000-8000-0000000000%02d", 10+i),
				domain.TargetDispute, "d0000000-0000-4000-8000-000000000001",
				domain.PriorityP2, "race report", base)
			id, rep, err := s.OpenCase(ctx, c)
			ids[i], replayed[i], errs[i] = id, rep, err
		}(i)
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("worker %d: %v", i, err)
		}
	}
	for i := 1; i < workers; i++ {
		if ids[i] != ids[0] {
			t.Fatalf("concurrent opens diverged: %v", ids)
		}
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM moderation_cases").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("concurrent opens created %d rows, want 1", count)
	}
}

func TestListOpenOrdersByPriorityThenAge(t *testing.T) {
	s, _ := freshStore(t)
	ctx := context.Background()
	base := time.Now().Truncate(time.Millisecond)
	// P3 oldest still sorts last: priority dominates age.
	for _, tc := range []struct {
		id, target, priority string
		at                   time.Time
	}{
		{"c0000000-0000-4000-8000-000000000001", "b0000000-0000-4000-8000-000000000001", domain.PriorityP3, base},
		{"c0000000-0000-4000-8000-000000000002", "b0000000-0000-4000-8000-000000000002", domain.PriorityP1, base.Add(time.Hour)},
		{"c0000000-0000-4000-8000-000000000003", "b0000000-0000-4000-8000-000000000003", domain.PriorityP2, base.Add(-time.Hour)},
	} {
		if _, _, err := s.OpenCase(ctx, mustCase(t, tc.id, domain.TargetObservation, tc.target, tc.priority, "r", tc.at)); err != nil {
			t.Fatal(err)
		}
	}
	page, err := s.ListOpen(ctx, -1, time.Time{}, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 3 {
		t.Fatalf("page = %d cases", len(page))
	}
	if page[0].Priority != domain.PriorityP1 || page[1].Priority != domain.PriorityP2 || page[2].Priority != domain.PriorityP3 {
		t.Errorf("queue order wrong: %q %q %q", page[0].Priority, page[1].Priority, page[2].Priority)
	}
	// Cursor walk: one row per page in queue order.
	first, err := s.ListOpen(ctx, -1, time.Time{}, "", 1)
	if err != nil || len(first) != 1 || first[0].ID != page[0].ID {
		t.Fatalf("first cursor page = %+v %v", first, err)
	}
	second, err := s.ListOpen(ctx, int32(domain.PriorityRank(first[0].Priority)), first[0].OpenedAt, first[0].ID, 1)
	if err != nil || len(second) != 1 || second[0].ID != page[1].ID {
		t.Fatalf("second cursor page = %+v %v", second, err)
	}
	third, err := s.ListOpen(ctx, int32(domain.PriorityRank(second[0].Priority)), second[0].OpenedAt, second[0].ID, 10)
	if err != nil || len(third) != 1 || third[0].ID != page[2].ID {
		t.Fatalf("third cursor page = %+v %v", third, err)
	}
}

func TestEvidenceReferenceStaysIdentifierOnly(t *testing.T) {
	s, _ := freshStore(t)
	ctx := context.Background()
	base := time.Now().Truncate(time.Millisecond)
	c, _, err := domain.NewCase(domain.CaseParams{
		ID:         "c0000000-0000-4000-8000-000000000001",
		TargetType: domain.TargetObservation,
		TargetID:   "b0000000-0000-4000-8000-000000000001",
		Priority:   domain.PriorityP2, Reason: "evidence mismatch",
		EvidenceID: "e0000000-0000-4000-8000-000000000001",
		OpenedAt:   base,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.OpenCase(ctx, c); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.EvidenceID != "e0000000-0000-4000-8000-000000000001" {
		t.Errorf("evidence ref = %q", got.EvidenceID)
	}
}

func TestSchemaCarriesNoSensitiveColumns(t *testing.T) {
	s, pool := freshStore(t)
	_ = s
	ctx := context.Background()
	rows, err := pool.Query(ctx, `
		SELECT table_name, column_name, data_type FROM information_schema.columns
		WHERE table_name IN ('moderation_cases', 'moderation_actions')`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	forbidden := []string{
		"geography", "geometry", "point", "coordinates", "latitude",
		"longitude", "accuracy", "ip_address", "ip_digest", "media",
		"photo", "image", "bytes", "bytea", "url", "exif",
	}
	found := map[string]string{}
	for rows.Next() {
		var table, name, typ string
		if err := rows.Scan(&table, &name, &typ); err != nil {
			t.Fatal(err)
		}
		found[table+"."+strings.ToLower(name)] = strings.ToLower(typ)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for col, typ := range found {
		for _, f := range forbidden {
			if strings.Contains(col, f) || strings.Contains(typ, f) {
				t.Errorf("sensitive column/type leaked: %q %q", col, typ)
			}
		}
		if typ == "bytea" {
			t.Errorf("bytea payload column: %q", col)
		}
	}
	for _, want := range []string{"moderation_cases.id", "moderation_cases.target_type", "moderation_cases.target_id", "moderation_cases.status", "moderation_cases.priority", "moderation_cases.reason", "moderation_cases.opened_at", "moderation_cases.policy_version", "moderation_actions.id", "moderation_actions.case_id", "moderation_actions.actor_id", "moderation_actions.action", "moderation_actions.reason"} {
		if _, ok := found[want]; !ok {
			t.Errorf("missing ledger column %q (got %v)", want, found)
		}
	}
}

func TestNoDestructiveSQLPaths(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	sqlPath := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "db", "queries", "moderation", "moderation.sql")
	raw, err := os.ReadFile(sqlPath)
	if err != nil {
		t.Fatal(err)
	}
	upper := strings.ToUpper(string(raw))
	// The single allowed mutating path is the guarded case-status
	// transition (actionable states only, never target/reason edits):
	// exactly one UPDATE carrying the terminal-state guard, zero DELETE.
	if got := strings.Count(upper, "UPDATE MODERATION_CASES"); got != 1 {
		t.Errorf("moderation.sql has %d case UPDATEs, want exactly the guarded transition", got)
	}
	if !strings.Contains(upper, "STATUS IN ('OPEN', 'IN_REVIEW')") {
		t.Error("guarded transition lost its terminal-state guard")
	}
	for _, verb := range []string{"\nDELETE ", "DELETE FROM MODERATION_"} {
		if strings.Contains(upper, verb) {
			t.Errorf("destructive path in moderation.sql: %q", strings.TrimSpace(verb))
		}
	}
}
