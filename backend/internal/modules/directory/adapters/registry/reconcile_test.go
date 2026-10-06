package registry

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

// fakeCanon converges one CNPJ to one station like the partial unique
// index does; different CNPJs never merge; revocation sticks against
// later active snapshots (reactivation is audited-only).
type fakeCanon struct {
	mu       sync.Mutex
	byCNPJ   map[string]string
	status   map[string]string
	points   map[string][2]float64
	touched  []string
	failCNPJ map[string]bool
}

func newFakeCanon() *fakeCanon {
	return &fakeCanon{byCNPJ: map[string]string{}, status: map[string]string{}, points: map[string][2]float64{}}
}

func (f *fakeCanon) ResolveStation(_ context.Context, cnpj, _ string, _ map[string]string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failCNPJ[cnpj] {
		return "", errors.New("canon boom")
	}
	if id, ok := f.byCNPJ[cnpj]; ok {
		return id, nil
	}
	id := "station-for-" + cnpj
	f.byCNPJ[cnpj] = id
	f.status[id] = "active"
	f.touched = append(f.touched, cnpj)
	return id, nil
}

func (f *fakeCanon) RecordReviewedPoint(_ context.Context, stationID string, lat, lon float64, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.points[stationID] = [2]float64{lat, lon}
	return nil
}

func (f *fakeCanon) SetStatus(_ context.Context, stationID, status string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	current := f.status[stationID]
	if current != status && !((current == "suspended" || current == "revoked") && status == "active") {
		f.status[stationID] = status
	}
	return nil
}

func stageSample(t *testing.T, store Store, snapshot, csv string) Report {
	t.Helper()
	report, err := StageCSV(context.Background(), store, snapshot, strings.NewReader(csv), Limits{MaxBytes: 1 << 20, MaxRows: 100, BatchSize: 10})
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	if report.State != "complete" {
		t.Fatalf("state = %q", report.State)
	}
	return report
}

const reconCSV = `CNPJ;RAZAO_SOCIAL;COD_IBGE;UF;SITUACAO;ATO_AUTORIZACAO
04218406000104;[P25-TEST] ALFA;3550308;SP;ATIVA;PRC-1
12ABC34501DE35;[P25-TEST] GAMA;3550308;SP;DESCONHECIDA;
`

func TestReconcilePublishesOneIdentityPerCNPJ(t *testing.T) {
	store := newFakeStore()
	canon := newFakeCanon()
	staged := stageSample(t, store, "recon-1", reconCSV)

	report, err := ReconcileRun(context.Background(), store, canon, SourceCSV, "recon-1")
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if report.Reconciled != 2 || report.Skipped != 0 {
		t.Fatalf("report = %+v", report)
	}
	// Same CNPJ across CSV and API converges on one station; nearby
	// distinct CNPJs stay distinct (no address-only merge).
	apiAssertion := Assertion{
		Source: "registry-api", SourceKey: "04218406000104", Checksum: "api-sum-1",
		DisplayName: "[P25-TEST] ALFA", Address: map[string]string{"municipio_ibge": "3550308", "uf": "SP"},
		MunicipalityCode: "3550308", State: "SP", AuthState: "authorized",
		Eligibility: "eligible", LocationQuality: "reviewed",
		Latitude: -23.561, Longitude: -46.656, HasCoords: true, CRS: "WGS84",
		runID: staged.RunID,
	}
	if _, err := store.StageAssertion(context.Background(), apiAssertion); err != nil {
		t.Fatalf("stage api: %v", err)
	}
	second, err := ReconcileRun(context.Background(), store, canon, SourceCSV, "recon-1")
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if second.Reconciled != 3 {
		t.Fatalf("report = %+v", second)
	}
	if len(canon.byCNPJ) != 2 {
		t.Fatalf("stations = %v, want exactly 2", canon.byCNPJ)
	}
	alfa := canon.byCNPJ["04218406000104"]
	if pt := canon.points[alfa]; pt != [2]float64{-23.561, -46.656} {
		t.Fatalf("reviewed point = %v", pt)
	}
	if canon.status[alfa] != "active" {
		t.Fatalf("status = %q", canon.status[alfa])
	}
}

func TestReconcileRevocationSticks(t *testing.T) {
	store := newFakeStore()
	canon := newFakeCanon()
	stageSample(t, store, "recon-rev", "CNPJ;RAZAO_SOCIAL;COD_IBGE;UF;SITUACAO\n55881177000136;[P25-TEST] EPS;3550308;SP;CANCELADA\n")

	if _, err := ReconcileRun(context.Background(), store, canon, SourceCSV, "recon-rev"); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	id := canon.byCNPJ["55881177000136"]
	if canon.status[id] != "revoked" {
		t.Fatalf("status = %q, want revoked", canon.status[id])
	}
	// A later active snapshot must not clear the revocation.
	stageSample(t, store, "recon-rev-2", "CNPJ;RAZAO_SOCIAL;COD_IBGE;UF;SITUACAO\n55881177000136;[P25-TEST] EPS;3550308;SP;ATIVA\n")
	if _, err := ReconcileRun(context.Background(), store, canon, SourceCSV, "recon-rev-2"); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if canon.status[id] != "revoked" {
		t.Fatalf("revocation cleared by later snapshot: %q", canon.status[id])
	}
	// Only asserted CNPJs were touched: no sweep, no closure.
	if len(canon.touched) != 1 {
		t.Fatalf("touched = %v", canon.touched)
	}
}

func TestReconcileRefusesIncompleteRun(t *testing.T) {
	store := newFakeStore()
	if _, err := StageCSV(context.Background(), store, "recon-bad", strings.NewReader("CNPJ;UF\n"), Limits{MaxBytes: 1 << 20, MaxRows: 10, BatchSize: 10}); err != nil {
		t.Fatalf("stage: %v", err)
	}
	if _, err := ReconcileRun(context.Background(), store, newFakeCanon(), SourceCSV, "recon-bad"); err == nil {
		t.Fatal("incomplete run must be refused")
	}
}

func TestReconcileSkipsFailedRowsHonestly(t *testing.T) {
	store := newFakeStore()
	canon := newFakeCanon()
	canon.failCNPJ = map[string]bool{"12ABC34501DE35": true}
	stageSample(t, store, "recon-part", reconCSV)

	report, err := ReconcileRun(context.Background(), store, canon, SourceCSV, "recon-part")
	if err == nil {
		t.Fatal("row failure must surface")
	}
	if report.Reconciled != 1 || report.Skipped != 1 {
		t.Fatalf("report = %+v", report)
	}
}
