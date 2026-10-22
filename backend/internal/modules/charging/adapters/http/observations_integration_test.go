//go:build integration

package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/charging/adapters/prepared"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func observationTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	adminDSN := "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Skip("database unavailable")
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("charging_obs_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		admin, err := pgx.Connect(ctx, adminDSN)
		if err == nil {
			admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize())
			admin.Close(ctx)
		}
	})
	u, _ := url.Parse(adminDSN)
	u.Path = "/" + name
	if _, err := migrate.Apply(ctx, u.String(), dbmigrations.Files); err != nil {
		t.Fatal("migrate", err)
	}
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	// Seed account for auth.
	if _, err := pool.Exec(ctx, `INSERT INTO accounts(id,alias,status) VALUES('aaaaaaaa-1111-4111-8111-000000000001','test-owner','active')`); err != nil {
		t.Fatal(err)
	}
	return pool
}

func TestObservationCommandsAuthQuotaReplay(t *testing.T) {
	pool := observationTestPool(t)
	router := chi.NewRouter()
	sessions := func(ctx context.Context, familyID, token string) (string, error) {
		if token == "valid-token" {
			return "aaaaaaaa-1111-4111-8111-000000000001", nil
		}
		return "", fmt.Errorf("forged")
	}
	CommandsHandler{Store: prepared.ObservationStore{Pool: pool}, Sessions: sessions}.RegisterRoutes(router)
	post := func(body map[string]any) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		request := httptest.NewRequest("POST", "/v1/charging/observations", bytes.NewReader(raw))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, request)
		var response map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &response)
		return w.Code, response
	}
	session := map[string]any{"family_id": "bbbbbbbb-1111-4111-8111-000000000001", "access_token": "valid-token"}
	observation := map[string]any{"session": session, "id": "cccccccc-1111-4111-8111-000000000001",
		"connector_code": "TYPE2", "condition": "STANDARD", "observed_day": "2026-10-22", "note": "test"}
	code, _ := post(observation)
	if code != 201 {
		t.Fatal("valid submit refused", code)
	}
	// Replay same ID is idempotent (no duplicate, same ID back).
	code, replay := post(observation)
	if code != 201 || replay["id"] != "cccccccc-1111-4111-8111-000000000001" {
		t.Fatal("replay diverged", code, replay)
	}
	// Forged session refused.
	forged := map[string]any{"session": map[string]any{"family_id": "bbbbbbbb-1111-4111-8111-000000000001", "access_token": "forged"},
		"id": "dddddddd-1111-4111-8111-000000000001", "connector_code": "TYPE2", "condition": "STANDARD", "observed_day": "2026-10-22"}
	if code, _ := post(forged); code != 401 {
		t.Fatal("forged session accepted", code)
	}
	// Invalid shape refused.
	bad := map[string]any{"session": session, "id": "not-a-uuid", "connector_code": "TYPE2", "condition": "STANDARD", "observed_day": "2026-10-22"}
	if code, _ := post(bad); code != 400 {
		t.Fatal("malformed command accepted", code)
	}
	var count int64
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM charging_observations WHERE NOT deleted`).Scan(&count); err != nil || count != 1 {
		t.Fatal("storage diverged", count, err)
	}
}
