//go:build integration

package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
)

const correctionStationID = "00000000-0000-4000-8000-000000000001"

func correctionSetup(t *testing.T) (*pgxpool.Pool, application.CorrectionService) {
	t.Helper()
	pool, _, _ := freshRegistryDB(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO directory_stations
		(id, display_name, address, municipality_code, state)
		VALUES ($1, 'Posto Teste', '{}'::jsonb, '3550308', 'SP')`, correctionStationID); err != nil {
		t.Fatalf("station: %v", err)
	}
	svc := application.CorrectionService{
		Store: CorrectionStore{Pool: pool, Repo: NewRepository(pool)},
		Clock: time.Now,
		NewID: NewUUIDv4,
	}
	return pool, svc
}

func correctionValue(t *testing.T, lat, lon float64) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]float64{"latitude": lat, "longitude": lon})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestCorrectionIntegrationIdempotencyAndOwnership(t *testing.T) {
	pool, svc := correctionSetup(t)
	ctx := context.Background()
	owner := "11111111-1111-4111-8111-111111111111"
	foreign := "22222222-2222-4222-8222-222222222222"
	in := application.CorrectionInput{
		StationID: correctionStationID, Field: application.CorrectionFieldLocation,
		ProposedValue: correctionValue(t, -23.55, -46.63), ClientSubmissionID: "k1",
	}
	created, isNew, err := svc.Submit(ctx, owner, "k1", in)
	if err != nil || !isNew || created.Status != application.CorrectionPending {
		t.Fatalf("submit = %+v, new=%v, err=%v", created, isNew, err)
	}
	replay, isNew, err := svc.Submit(ctx, owner, "k1", in)
	if err != nil || isNew || replay.ID != created.ID {
		t.Fatalf("replay = %+v, new=%v, err=%v", replay, isNew, err)
	}
	changed := in
	changed.ProposedValue = correctionValue(t, -23.56, -46.64)
	if _, _, err := svc.Submit(ctx, owner, "k1", changed); !errors.Is(err, application.ErrCorrectionConflict) {
		t.Fatalf("changed body err = %v", err)
	}
	if _, _, err := svc.Submit(ctx, owner, "k2", in); !errors.Is(err, application.ErrCorrectionOpen) {
		t.Fatalf("second open err = %v", err)
	}
	if _, err := svc.Owned(ctx, foreign, created.ID); !errors.Is(err, application.ErrCorrectionNotFound) {
		t.Fatalf("foreign read err = %v", err)
	}
	if err := svc.Cancel(ctx, foreign, created.ID); !errors.Is(err, application.ErrCorrectionNotFound) {
		t.Fatalf("foreign cancel err = %v", err)
	}
	if err := svc.Cancel(ctx, owner, created.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	owned, err := svc.Owned(ctx, owner, created.ID)
	if err != nil || owned.Status != application.CorrectionCancelled {
		t.Fatalf("cancelled = %+v, err=%v", owned, err)
	}
	// A cancelled row no longer blocks a fresh correction for the field.
	if _, _, err := svc.Submit(ctx, owner, "k3", in); err != nil {
		t.Fatalf("resubmit: %v", err)
	}
	var decisions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM station_correction_decisions`).Scan(&decisions); err != nil {
		t.Fatal(err)
	}
	if decisions != 0 {
		t.Fatalf("cancel wrote %d decisions", decisions)
	}
}

func TestCorrectionIntegrationAcceptedLocationProjectsReviewed(t *testing.T) {
	pool, svc := correctionSetup(t)
	ctx := context.Background()
	owner := "11111111-1111-4111-8111-111111111111"
	in := application.CorrectionInput{
		StationID: correctionStationID, Field: application.CorrectionFieldLocation,
		ProposedValue: correctionValue(t, -23.55, -46.63), ClientSubmissionID: "loc-1",
	}
	created, _, err := svc.Submit(ctx, owner, "loc-1", in)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if err := svc.Decide(ctx, created.ID, application.CorrectionAccepted, "verified against the photo", "staff", "op-1", []byte(`{"source":"test"}`)); err != nil {
		t.Fatalf("decide: %v", err)
	}
	var qual, point string
	if err := pool.QueryRow(ctx, `SELECT current_quality, ST_AsText(current_point) FROM directory_stations WHERE id=$1`, correctionStationID).Scan(&qual, &point); err != nil {
		t.Fatalf("station projection: %v", err)
	}
	if qual != "reviewed" || point == "" {
		t.Fatalf("projection = %q %q", qual, point)
	}
	var revQuality string
	if err := pool.QueryRow(ctx, `SELECT quality FROM directory_location_revisions WHERE station_id=$1 ORDER BY obtained_at DESC LIMIT 1`, correctionStationID).Scan(&revQuality); err != nil {
		t.Fatalf("revision: %v", err)
	}
	if revQuality != "reviewed" {
		t.Fatalf("revision quality = %q", revQuality)
	}
	var decision, reason, kind string
	if err := pool.QueryRow(ctx, `SELECT decision, reason, decider_kind FROM station_correction_decisions WHERE correction_id=$1`, created.ID).Scan(&decision, &reason, &kind); err != nil {
		t.Fatalf("decision audit: %v", err)
	}
	if decision != "accepted" || reason == "" || kind != "staff" {
		t.Fatalf("decision = %q %q %q", decision, reason, kind)
	}
	if err := svc.Decide(ctx, created.ID, application.CorrectionAccepted, "again", "staff", "op-1", nil); !errors.Is(err, application.ErrCorrectionClosed) {
		t.Fatalf("second decision err = %v", err)
	}
}

func TestCorrectionIntegrationConcurrentDecisionsConverge(t *testing.T) {
	_, svc := correctionSetup(t)
	ctx := context.Background()
	owner := "11111111-1111-4111-8111-111111111111"
	created, _, err := svc.Submit(ctx, owner, "race-1", application.CorrectionInput{
		StationID: correctionStationID, Field: application.CorrectionFieldDisplayName,
		ProposedValue: json.RawMessage(`{"value":"Posto Alfa"}`), ClientSubmissionID: "race-1",
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results <- svc.Decide(ctx, created.ID, application.CorrectionAccepted, "reviewed", "staff", fmt.Sprintf("op-%d", i), nil)
		}(i)
	}
	wg.Wait()
	close(results)
	var ok, closed int
	for err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, application.ErrCorrectionClosed):
			closed++
		default:
			t.Fatalf("unexpected decision err: %v", err)
		}
	}
	if ok != 1 || closed != 1 {
		t.Fatalf("concurrent decisions ok=%d closed=%d", ok, closed)
	}
}

func TestCorrectionIntegrationConcurrentOpenFieldConverges(t *testing.T) {
	_, svc := correctionSetup(t)
	ctx := context.Background()
	owner := "11111111-1111-4111-8111-111111111111"
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, _, err := svc.Submit(ctx, owner, fmt.Sprintf("open-%d", i), application.CorrectionInput{
				StationID: correctionStationID, Field: application.CorrectionFieldBrand,
				ProposedValue: json.RawMessage(`{"value":"Ipiranga"}`), ClientSubmissionID: fmt.Sprintf("open-%d", i),
			})
			results <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	var ok, open int
	for err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, application.ErrCorrectionOpen):
			open++
		default:
			t.Fatalf("unexpected submit err: %v", err)
		}
	}
	if ok != 1 || open != 1 {
		t.Fatalf("concurrent open ok=%d open=%d", ok, open)
	}
}

func TestCorrectionIntegrationQuota(t *testing.T) {
	pool, svc := correctionSetup(t)
	ctx := context.Background()
	owner := "11111111-1111-4111-8111-111111111111"
	for i := 0; i < application.MaxCorrectionsPerDay; i++ {
		if _, err := pool.Exec(ctx, `INSERT INTO station_corrections
			(id, station_id, account_id, field, proposed_value, client_submission_id, status, decided_at)
			VALUES ($1,$2,$3,'brand','{"value":"X"}'::jsonb,$4,'accepted',now())`,
			fmt.Sprintf("00000000-0000-4000-8000-%012d", i+10), correctionStationID, owner,
			fmt.Sprintf("quota-%d", i)); err != nil {
			t.Fatalf("seed quota: %v", err)
		}
	}
	_, _, err := svc.Submit(ctx, owner, "quota-over", application.CorrectionInput{
		StationID: correctionStationID, Field: application.CorrectionFieldDisplayName,
		ProposedValue: json.RawMessage(`{"value":"Posto Alfa"}`), ClientSubmissionID: "quota-over",
	})
	if !errors.Is(err, application.ErrCorrectionQuota) {
		t.Fatalf("quota err = %v", err)
	}
}
