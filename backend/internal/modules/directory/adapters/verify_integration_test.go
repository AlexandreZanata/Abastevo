//go:build integration

package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/directory"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/registry"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/domain"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

const verifyCSV = `CNPJ;RAZAO_SOCIAL;COD_IBGE;UF;SITUACAO;ATO_AUTORIZACAO
04218406000104;[P27-TEST] ALFA;3550308;SP;ATIVA;PRC-1
`

func verifyTestSetup(t *testing.T) (*pgxpool.Pool, *IntakeStore, RegistryCanonicalizer, application.IntakeService) {
	t.Helper()
	pool, _, canon := freshRegistryDB(t)
	ctx := context.Background()
	registryStore := &registry.PGStore{Q: directory.New(pool)}
	if _, err := registry.StageCSV(ctx, registryStore, "verify-src", strings.NewReader(verifyCSV), registry.Limits{MaxBytes: 1 << 20, MaxRows: 100, BatchSize: 10}); err != nil {
		t.Fatalf("stage: %v", err)
	}
	if _, err := registry.ReconcileRun(ctx, registryStore, canon, registry.SourceCSV, "verify-src"); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	store := &IntakeStore{Q: directory.New(pool)}
	n := 0
	svc := application.IntakeService{
		Store: store,
		Clock: func() time.Time { return time.Now() },
		NewID: func() (string, error) {
			n++
			return fmt.Sprintf("00000000-0000-4000-8000-%012d", n), nil
		},
	}
	return pool, store, canon, svc
}

func verifyHandler(pool *pgxpool.Pool, store *IntakeStore, canon RegistryCanonicalizer) jobs.VerifySweep {
	n := 100
	return jobs.VerifySweep{
		Store: store,
		Resolve: func(ctx context.Context, cnpj, display string, address map[string]string) (string, string, string, error) {
			official, err := directory.New(pool).FindOfficialAssertion(ctx, cnpj)
			if err != nil {
				return "", "", "", application.ErrStationUnknown
			}
			station, err := canon.Repo.ResolveCNPJ(ctx, cnpj, display, address)
			if err != nil {
				return "", "", "", err
			}
			return station.ID, official.MunicipalityCode.String, official.State.String, nil
		},
		RecordPin: func(ctx context.Context, stationID string, lat, lon float64, ref string) error {
			_, err := canon.Repo.RecordLocation(ctx, domain.LocationRevision{
				StationID: stationID, PointWKT: fmt.Sprintf("POINT(%f %f)", lon, lat),
				Quality: "unknown", Provider: "suggestion", SourceReference: ref,
			})
			return err
		},
		NewID: func() (string, error) {
			n++
			return fmt.Sprintf("00000000-0000-4000-8000-%012d", n), nil
		},
		Batch: 25,
	}
}

func suggestionInput(cnpj, ibge string, lat, lon float64, hasCoords bool) application.ProposalInput {
	return application.ProposalInput{
		DisplayName: "Posto Alfa", MunicipalityCode: ibge, State: "SP",
		CNPJ: cnpj, Latitude: lat, Longitude: lon, HasCoords: hasCoords,
	}
}

func TestVerifyIntegrationApprovesExactMatchWithPin(t *testing.T) {
	pool, store, canon, svc := verifyTestSetup(t)
	ctx := context.Background()
	acc := "11111111-1111-4111-8111-111111111111"

	created, _, err := svc.Submit(ctx, acc, "key-exact", suggestionInput("04218406000104", "3550308", -23.55, -46.63, true))
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	handler := verifyHandler(pool, store, canon)
	payload, _ := json.Marshal(map[string]any{"version": 1, "batch": 25})
	if err := handler.Handle(ctx, platformjobs.Job{Kind: handler.Kind(), Payload: payload}); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	owned, err := svc.Owned(ctx, acc, created.ID)
	if err != nil || owned.State != "approved" {
		t.Fatalf("owned = %+v, err = %v", owned, err)
	}
	// The user pin recorded as an unknown-quality revision: preserved
	// for review, never projected as canonical geometry.
	var qualities []string
	rows, err := pool.Query(ctx, "SELECT quality FROM directory_location_revisions WHERE provider = 'suggestion'")
	if err != nil {
		t.Fatalf("revisions: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var quality string
		if err := rows.Scan(&quality); err != nil {
			t.Fatalf("scan: %v", err)
		}
		qualities = append(qualities, quality)
	}
	if len(qualities) != 1 || qualities[0] != "unknown" {
		t.Fatalf("pin qualities = %v", qualities)
	}
	var projected *string
	if err := pool.QueryRow(ctx, "SELECT current_quality FROM directory_stations WHERE id = (SELECT station_id FROM directory_identifiers WHERE kind = 'CNPJ' AND normalized_value = '04218406000104' AND valid_to IS NULL)").Scan(&projected); err != nil {
		t.Fatalf("projection: %v", err)
	}
	if projected != nil {
		t.Fatalf("user pin must not project: %v", *projected)
	}
}

func TestVerifyIntegrationConflictStaysPending(t *testing.T) {
	pool, store, canon, svc := verifyTestSetup(t)
	ctx := context.Background()
	acc := "11111111-1111-4111-8111-111111111111"

	// Same CNPJ as the official assertion but a conflicting
	// municipality: automation must defer to human review, never
	// auto-approve and never auto-reject.
	created, _, err := svc.Submit(ctx, acc, "key-conflict", suggestionInput("04218406000104", "3106200", 0, 0, false))
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	handler := verifyHandler(pool, store, canon)
	payload, _ := json.Marshal(map[string]any{"version": 1, "batch": 25})
	if err := handler.Handle(ctx, platformjobs.Job{Kind: handler.Kind(), Payload: payload}); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	owned, err := svc.Owned(ctx, acc, created.ID)
	if err != nil || owned.State != application.SuggestionPending {
		t.Fatalf("owned = %+v, err = %v (must stay pending)", owned, err)
	}
}

func TestDecideIntegrationConcurrentReviewers(t *testing.T) {
	_, store, _, svc := verifyTestSetup(t)
	ctx := context.Background()
	acc := "11111111-1111-4111-8111-111111111111"

	created, _, err := svc.Submit(ctx, acc, "key-race", suggestionInput("04218406000104", "3550308", 0, 0, false))
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	ports := application.VerifyPorts{
		Store:     store,
		RecordPin: func(context.Context, string, float64, float64, string) error { return nil },
		NewID: func() (string, error) {
			return fmt.Sprintf("00000000-0000-4000-8000-%012d", time.Now().UnixNano()%1000000000000), nil
		},
	}
	var wg sync.WaitGroup
	results := make([]error, 2)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = application.Decide(ctx, ports, created.ID, "op-1", true, "looks right", "")
		}(i)
	}
	wg.Wait()
	wins := 0
	for _, err := range results {
		if err == nil {
			wins++
		} else if err != application.ErrVerifyClosed {
			t.Fatalf("unexpected err: %v", err)
		}
	}
	if wins != 1 {
		t.Fatalf("wins = %d, want exactly 1", wins)
	}
}
