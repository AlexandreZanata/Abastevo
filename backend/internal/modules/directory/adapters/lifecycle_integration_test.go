//go:build integration

package adapters

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/directory"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/read"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/registry"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// P29-T02 source-to-app lifecycle on the new candidate: registry-only
// station → suggestion → exact-match review → searchable/detail-readable
// zero-price station with stable UUID targets for price/comment/report.
// Closed sources, offline replays and unknown states stay honest.
// Device rows (denied GPS, photo expiry, low-end resources, cold start)
// join the end manual batch per directive — no emulator runs here.

const lifecycleCSV = `CNPJ;RAZAO_SOCIAL;COD_IBGE;UF;SITUACAO;ATO_AUTORIZACAO
04218406000104;[P29-TEST] ALFA;3550308;SP;ATIVA;PRC-1
12ABC34501DE35;[P29-TEST] GAMA;3550308;SP;DESCONHECIDA;
`

func TestLifecycleSourceToAppCatalog(t *testing.T) {
	pool, registryStore, canon := freshRegistryDB(t)
	ctx := context.Background()
	limits := registry.Limits{MaxBytes: 1 << 20, MaxRows: 1000, BatchSize: 100}

	staged, err := registry.StageCSV(ctx, registryStore, "lifecycle-1", strings.NewReader(lifecycleCSV), limits)
	if err != nil || staged.State != "complete" || staged.Accepted != 2 {
		t.Fatalf("stage = %+v, err = %v", staged, err)
	}
	recon, err := registry.ReconcileRun(ctx, registryStore, canon, registry.SourceCSV, "lifecycle-1")
	if err != nil || recon.Reconciled != 2 {
		t.Fatalf("reconcile = %+v, err = %v", recon, err)
	}

	intakeStore := &IntakeStore{Q: directory.New(pool)}
	svc := application.IntakeService{
		Store: intakeStore,
		Clock: func() time.Time { return time.Now() },
		NewID: func() (string, error) { return "00000000-0000-4000-8000-000000000001", nil },
	}
	acc := "55555555-5555-4555-8555-555555555555"
	suggestion, created, err := svc.Submit(ctx, acc, "lifecycle-key", application.ProposalInput{
		DisplayName: "Posto Alfa", MunicipalityCode: "3550308", State: "SP", CNPJ: "04218406000104",
	})
	if err != nil || !created {
		t.Fatalf("submit = %+v, %v, %v", suggestion, created, err)
	}
	sweep := jobs.VerifySweep{
		Store: intakeStore,
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
		RecordPin: func(context.Context, string, float64, float64, string) error { return nil },
		NewID:     func() (string, error) { return "00000000-0000-4000-8000-000000000002", nil },
		Batch:     25,
	}
	payload, _ := json.Marshal(map[string]any{"version": 1, "batch": 25})
	if err := sweep.Handle(ctx, platformjobs.Job{Kind: sweep.Kind(), Payload: payload}); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	owned, err := svc.Owned(ctx, acc, suggestion.ID)
	if err != nil || owned.State != "approved" {
		t.Fatalf("owned = %+v, err = %v", owned, err)
	}

	reader := read.NewReader(pool)
	alfa, _, err := reader.Search(ctx, application.SearchFilter{Q: "ALFA", Limit: 10})
	if err != nil || len(alfa) != 1 {
		t.Fatalf("search = %+v, err = %v", alfa, err)
	}
	uuid := alfa[0].ID
	if len(uuid) != 36 {
		t.Fatalf("target is not a canonical UUID: %q", uuid)
	}
	detail, err := reader.Detail(ctx, uuid)
	if err != nil || detail.DisplayName == "" {
		t.Fatalf("detail = %+v, err = %v", detail, err)
	}
	// No price rows exist anywhere: zero-price honesty for the new
	// price/comment/report UUID targets.
	var prices int64
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM community_observations").Scan(&prices); err != nil {
		t.Logf("observations table not present (ok): %v", err)
	} else if prices != 0 {
		t.Fatalf("prices = %d, want 0", prices)
	}
	// Unknown-location station stays searchable with honest nulls.
	gama, _, err := reader.Search(ctx, application.SearchFilter{Q: "GAMA", Limit: 10})
	if err != nil || len(gama) != 1 {
		t.Fatalf("search gama = %+v, err = %v", gama, err)
	}
	if gama[0].Coordinates != nil || gama[0].LocationQuality != "unknown" {
		t.Fatalf("gama not honest: %+v", gama[0])
	}
}

func TestLifecycleClosedSourceAndOfflineReplay(t *testing.T) {
	pool, registryStore, canon := freshRegistryDB(t)
	ctx := context.Background()
	limits := registry.Limits{MaxBytes: 1 << 20, MaxRows: 1000, BatchSize: 100}

	closed := "CNPJ;RAZAO_SOCIAL;COD_IBGE;UF;SITUACAO\n55881177000136;[P29-TEST] EPS;3550308;SP;CANCELADA\n"
	if _, err := registry.StageCSV(ctx, registryStore, "closed-1", strings.NewReader(closed), limits); err != nil {
		t.Fatalf("stage: %v", err)
	}
	report, err := registry.ReconcileRun(ctx, registryStore, canon, registry.SourceCSV, "closed-1")
	if err != nil || report.Reconciled != 1 {
		t.Fatalf("reconcile = %+v, err = %v", report, err)
	}
	// Closed stations stay listed with their sourced status (no silent
	// disappearance, no closure by missing rows).
	reader := read.NewReader(pool)
	found, _, err := reader.Search(ctx, application.SearchFilter{Q: "EPS", Limit: 10})
	if err != nil || len(found) != 1 {
		t.Fatalf("search = %+v, err = %v", found, err)
	}
	// Offline replay of the identical snapshot converges (no dupes).
	replay, err := registry.StageCSV(ctx, registryStore, "closed-1", strings.NewReader(closed), limits)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Accepted != 1 || replay.Duplicates != 0 {
		t.Fatalf("replay = %+v (same run no-op expected)", replay)
	}
	var stations int64
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM directory_stations").Scan(&stations); err != nil {
		t.Fatalf("count: %v", err)
	}
	if stations != 1 {
		t.Fatalf("stations = %d, want 1", stations)
	}
}
