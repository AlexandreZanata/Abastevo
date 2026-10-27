//go:build integration

package adapters

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	adminadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/adapters"
	adminapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/application"
	admindomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/domain"
	staffapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/staff/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/staff/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
	name := fmt.Sprintf("staff_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		admin.Close(ctx)
		t.Fatal(err)
	}
	admin.Close(ctx)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		admin, err := pgx.Connect(ctx, dsn)
		if err != nil {
			t.Error("cleanup connection failed")
			return
		}
		defer admin.Close(ctx)
		if _, err = admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
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

func newID(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	raw := hex.EncodeToString(b[:])
	return raw[0:8] + "-" + raw[8:12] + "-" + raw[12:16] + "-" + raw[16:20] + "-" + raw[20:32]
}

func seedAccount(t *testing.T, pool *pgxpool.Pool, status string) string {
	t.Helper()
	id := newID(t)
	if _, err := pool.Exec(context.Background(), "INSERT INTO accounts(id,alias,status) VALUES($1,$2,$3)", id, "synthetic-"+id, status); err != nil {
		t.Fatal(err)
	}
	return id
}

func adminSession(t *testing.T, pool *pgxpool.Pool, now time.Time) (adminapp.Session, *adminadapters.Store) {
	t.Helper()
	store := adminadapters.NewStore(pool)
	key := "abv_admin_" + base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	presented := admindomain.HashKey(key)
	result, err := store.Login(context.Background(), adminapp.LoginInput{
		PresentedHash: presented, IPHash: strings.Repeat("a", 64), Now: now,
		Verify: func() (string, bool) { return admindomain.Fingerprint(presented), true },
	})
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.Authenticate(context.Background(), result.Token, now)
	if err != nil {
		t.Fatal(err)
	}
	return session, store
}

var mutationCounter int

func mutation(sessionID string) adminapp.Mutation {
	mutationCounter++
	return adminapp.Mutation{
		SessionID: sessionID, Action: "admin.staff.assign", TargetType: "staff_assignment",
		TargetID: newIDForMutation(), Reason: "synthetic staff assignment reason", Key: newIDForMutation(),
		RequestHash: strings.Repeat("a", 64), RequestID: "synthetic-request",
		ClientIPHash: strings.Repeat("b", 64), Now: time.Now().UTC(),
	}
}

func newIDForMutation() string {
	return fmt.Sprintf("11111111-1111-4111-8111-%012d", mutationCounter)
}

func staffService(t *testing.T, pool *pgxpool.Pool, now time.Time) (*staffapp.Service, *Store, adminapp.Session) {
	t.Helper()
	session, adminStore := adminSession(t, pool, now)
	store := NewStore(pool)
	return &staffapp.Service{Store: store, Runner: adminStore, Clock: func() time.Time { return now }, NewID: func() (string, error) { return newID(t), nil }}, store, session
}

// TestAssign_STF02 covers active target, duplicate, bounds and revoke.
func TestAssign_STF02(t *testing.T) {
	pool := freshDB(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	service, store, session := staffService(t, pool, now)
	accountID := seedAccount(t, pool, "active")
	m := mutation(session.ID)
	assignment, err := service.Assign(context.Background(), m, staffapp.AssignInput{AccountID: accountID, Role: string(domain.RoleSupportAgent)})
	if err != nil || assignment.Label != "Suporte" || assignment.State != "ACTIVE" {
		t.Fatalf("assignment=%+v err=%v", assignment, err)
	}
	m2 := mutation(session.ID)
	m2.TargetID = accountID
	if _, err := service.Assign(context.Background(), m2, staffapp.AssignInput{AccountID: accountID, Role: string(domain.RoleSupportAgent)}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate err=%v", err)
	}
	suspended := seedAccount(t, pool, "suspended")
	if _, err := service.Assign(context.Background(), mutation(session.ID), staffapp.AssignInput{AccountID: suspended, Role: string(domain.RoleStationReviewer)}); !errors.Is(err, domain.ErrIneligible) {
		t.Fatalf("suspended err=%v", err)
	}
	tooSoon := now.Add(30 * time.Minute)
	if _, err := service.Assign(context.Background(), mutation(session.ID), staffapp.AssignInput{AccountID: accountID, Role: string(domain.RoleStationReviewer), EndAt: &tooSoon}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("bounds err=%v", err)
	}
	m3 := mutation(session.ID)
	m3.Action = "admin.staff.revoke"
	m3.TargetID = assignment.ID
	revoked, err := service.Revoke(context.Background(), m3, assignment.ID)
	if err != nil || revoked.State != "REVOKED" {
		t.Fatalf("revoke=%+v err=%v", revoked, err)
	}
	m3.Key = "22222222-2222-4222-8222-222222222222"
	if _, err := service.Revoke(context.Background(), m3, assignment.ID); !errors.Is(err, domain.ErrAlreadyRevoked) {
		t.Fatalf("second revoke err=%v", err)
	}
	tx, err := store.Pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	labels, err := store.ActiveLabels(context.Background(), tx, accountID, now)
	if err != nil || len(labels) != 0 {
		t.Fatalf("labels after revoke=%v err=%v", labels, err)
	}
}

// TestErasure_STF06 revokes active assignments on account deletion.
func TestErasure_STF06(t *testing.T) {
	pool := freshDB(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	service, _, session := staffService(t, pool, now)
	accountID := seedAccount(t, pool, "active")
	assignment, err := service.Assign(context.Background(), mutation(session.ID), staffapp.AssignInput{AccountID: accountID, Role: string(domain.RoleContributorCurator)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), "UPDATE accounts SET status='deleted' WHERE id=$1", accountID); err != nil {
		t.Fatal(err)
	}
	var state, reason string
	if err := pool.QueryRow(context.Background(), "SELECT state,revoke_reason FROM staff_assignments WHERE id=$1", assignment.ID).Scan(&state, &reason); err != nil {
		t.Fatal(err)
	}
	if state != "REVOKED" || reason != "account-erased" {
		t.Fatalf("state=%s reason=%s", state, reason)
	}
}

// TestAudit_STF_BUCADM12 covers action/target filters and pagination.
func TestAudit_STF_BUCADM12(t *testing.T) {
	pool := freshDB(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	service, _, session := staffService(t, pool, now)
	accountID := seedAccount(t, pool, "active")
	first, err := service.Assign(context.Background(), mutation(session.ID), staffapp.AssignInput{AccountID: accountID, Role: string(domain.RoleCommunityModerator)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Assign(context.Background(), mutation(session.ID), staffapp.AssignInput{AccountID: accountID, Role: string(domain.RoleStationReviewer)}); err != nil {
		t.Fatal(err)
	}
	_, adminStore := adminSession(t, pool, now)
	events, err := adminStore.ListAudit(context.Background(), adminapp.AuditFilter{Action: "admin.staff.assign", TargetID: first.ID, Limit: 10})
	if err != nil || len(events) != 1 || events[0].TargetID != first.ID {
		t.Fatalf("filtered=%+v err=%v", events, err)
	}
	page1, err := adminStore.ListAudit(context.Background(), adminapp.AuditFilter{Action: "admin.staff.assign", Limit: 1})
	if err != nil || len(page1) != 1 {
		t.Fatalf("page1=%+v err=%v", page1, err)
	}
	page2, err := adminStore.ListAudit(context.Background(), adminapp.AuditFilter{
		Action: "admin.staff.assign", BeforeAt: page1[0].At, BeforeID: page1[0].ID, Limit: 1,
	})
	if err != nil || len(page2) != 1 || page2[0].ID == page1[0].ID {
		t.Fatalf("page2=%+v err=%v", page2, err)
	}
}
