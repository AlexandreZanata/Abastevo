//go:build integration

package adapters

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"testing"
	"time"

	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/directory"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/registry"
	"github.com/jackc/pgx/v5/pgxpool"
)

func factVersion(t *testing.T, pool *pgxpool.Pool, store *registry.PGStore, canon RegistryCanonicalizer, version int, trade, source string, complete bool) (string, string) {
	t.Helper()
	ctx := context.Background()
	run, _ := NewUUIDv4()
	snapshot := fmt.Sprintf("sna-synthetic-%d", version)
	if source == registry.SourcePrep {
		snapshot = "station-prep:" + run + ":registry-13col"
	}
	if _, _, err := store.CreateRun(ctx, run, source, snapshot, "synthetic"); err != nil {
		t.Fatal(err)
	}
	checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprint(version))))
	a := registry.Assertion{Source: source, SourceKey: "91000000000066", Checksum: checksum, DisplayName: fmt.Sprintf("[P25-TEST] Source label %d", version), SourceDisplayName: "[P25-TEST] Own source", LegalName: fmt.Sprintf(" [P25-TEST] Legal %d LTDA ", version), TradeName: trade, Address: map[string]string{"street": fmt.Sprintf("[P25-TEST] STREET %d", version), "number": "100", "municipio": "TEST CITY", "uf": "SP"}, MunicipalityCode: "3550308", State: "SP", AuthState: "unknown", Eligibility: "pending", LocationQuality: "unknown"}
	if _, err := store.StageAssertion(ctx, a.WithRun(run)); err != nil {
		t.Fatal(err)
	}
	rows, err := store.ListAssertions(ctx, run)
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	id, err := canon.ResolveStation(ctx, a.SourceKey, a.DisplayName, a.Address)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetAssertionStation(ctx, rows[0].ID, id); err != nil {
		t.Fatal(err)
	}
	if complete {
		if err := store.FinishRun(ctx, run, "complete", 1, 0, 0, ""); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, "UPDATE registry_source_runs SET finished_at=$2 WHERE id=$1::uuid", run, time.Date(2026, 1, 1, 0, 0, version, 0, time.UTC)); err != nil {
			t.Fatal(err)
		}
	}
	return id, rows[0].ID
}

func TestStationFactProjectionNamesAuditedCorrectionAndReplay(t *testing.T) {
	pool, store, canon := freshRegistryDB(t)
	ctx := context.Background()
	id, first := factVersion(t, pool, store, canon, 1, "[P25-TEST] Trade", registry.SourceCSV, true)
	if err := canon.ProjectSourceFacts(ctx, id, first); err != nil {
		t.Fatal(err)
	}
	check := func(display, legal string) {
		t.Helper()
		var got, raw string
		if err := pool.QueryRow(ctx, "SELECT display_name,legal_name FROM directory_stations WHERE id=$1::uuid", id).Scan(&got, &raw); err != nil || got != display || raw != legal {
			t.Fatal(got, raw, err)
		}
	}
	check("[P25-TEST] Trade", " [P25-TEST] Legal 1 LTDA ")
	_, second := factVersion(t, pool, store, canon, 2, "NÃO POSSUI", registry.SourceCSV, true)
	if err := canon.ProjectSourceFacts(ctx, id, second); err != nil {
		t.Fatal(err)
	}
	check("[P25-TEST] Legal 2 LTDA", " [P25-TEST] Legal 2 LTDA ")
	// Pending/rejected names are private proposals, not canonical facts.
	correctionID, _ := NewUUIDv4()
	correctionUID, _ := mustUUID(correctionID)
	stationUID, _ := mustUUID(id)
	accountID, _ := NewUUIDv4()
	accountUID, _ := mustUUID(accountID)
	q := directory.New(pool)
	if _, err := q.CreateCorrection(ctx, directory.CreateCorrectionParams{ID: correctionUID, StationID: stationUID, AccountID: accountUID, Field: "display_name", ProposedValue: []byte(`{"value":"[P25-TEST] Approved"}`), ClientSubmissionID: "sna-synthetic"}); err != nil {
		t.Fatal(err)
	}
	if err := canon.ProjectSourceFacts(ctx, id, second); err != nil {
		t.Fatal(err)
	}
	check("[P25-TEST] Legal 2 LTDA", " [P25-TEST] Legal 2 LTDA ")
	corrections := CorrectionStore{Pool: pool, Repo: canon.Repo}
	if _, err := corrections.Decide(ctx, correctionID, "accepted", []string{}, "accepted", "synthetic evidence", "invalid", "[P25-TEST] reviewer", []byte(`{}`)); err == nil {
		t.Fatal("invalid audit committed")
	}
	check("[P25-TEST] Legal 2 LTDA", " [P25-TEST] Legal 2 LTDA ")
	if n, err := corrections.Decide(ctx, correctionID, "accepted", []string{}, "accepted", "synthetic evidence", "staff", "[P25-TEST] reviewer", []byte(`{}`)); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	check("[P25-TEST] Approved", " [P25-TEST] Legal 2 LTDA ")
	_, third := factVersion(t, pool, store, canon, 3, "[P25-TEST] New Trade", registry.SourceCSV, true)
	if err := canon.ProjectSourceFacts(ctx, id, third); err != nil {
		t.Fatal(err)
	}
	if err := canon.ProjectSourceFacts(ctx, id, first); err != nil {
		t.Fatal(err)
	}
	check("[P25-TEST] Approved", " [P25-TEST] Legal 3 LTDA ")
	evidence, _, err := store.GetAssertion(ctx, third)
	if err != nil || evidence.TradeName != "[P25-TEST] New Trade" || evidence.SourceDisplayName != "[P25-TEST] Own source" {
		t.Fatal(evidence, err)
	}
}

func TestStationFactProjectionRejectsIncompleteForeignAndUnboundPrepared(t *testing.T) {
	pool, store, canon := freshRegistryDB(t)
	ctx := context.Background()
	id, incomplete := factVersion(t, pool, store, canon, 1, "[P25-TEST] Trade", registry.SourceCSV, false)
	if err := canon.ProjectSourceFacts(ctx, id, incomplete); err == nil {
		t.Fatal("incomplete source projected")
	}
	other, err := canon.ResolveStation(ctx, "12ABC34501DE35", "[P25-TEST] Other", map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	_, complete := factVersion(t, pool, store, canon, 2, "[P25-TEST] Trade", registry.SourceCSV, true)
	if err := canon.ProjectSourceFacts(ctx, other, complete); err == nil {
		t.Fatal("foreign CNPJ source projected")
	}
	_, unbound := factVersion(t, pool, store, canon, 3, "[P25-TEST] Unbound", registry.SourcePrep, true)
	if err := canon.ProjectSourceFacts(ctx, id, unbound); err == nil {
		t.Fatal("unbound prepared source projected")
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM directory_stations WHERE legal_name IS NOT NULL").Scan(&count); err != nil || count != 0 {
		t.Fatal("partial source publication", count, err)
	}
}

func TestStationFactProjectionCuratedFieldsAndConcurrentOrdering(t *testing.T) {
	pool, store, canon := freshRegistryDB(t)
	ctx := context.Background()
	id, first := factVersion(t, pool, store, canon, 1, "[P25-TEST] Old", registry.SourceCSV, true)
	if err := canon.ProjectSourceFacts(ctx, id, first); err != nil {
		t.Fatal(err)
	}
	_, second := factVersion(t, pool, store, canon, 2, "[P25-TEST] Latest", registry.SourceCSV, true)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assertion := first
			if i%2 == 0 {
				assertion = second
			}
			if err := canon.ProjectSourceFacts(ctx, id, assertion); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	var name, legal string
	if err := pool.QueryRow(ctx, "SELECT display_name,legal_name FROM directory_stations WHERE id=$1::uuid", id).Scan(&name, &legal); err != nil || name != "[P25-TEST] Latest" || legal != " [P25-TEST] Legal 2 LTDA " {
		t.Fatal(name, legal, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE directory_stations SET display_name='[P25-TEST] Curated',address='{"street":"[P25-TEST] Curated street"}' WHERE id=$1::uuid`, id); err != nil {
		t.Fatal(err)
	}
	_, third := factVersion(t, pool, store, canon, 3, "[P25-TEST] Incoming", registry.SourceCSV, true)
	if err := canon.ProjectSourceFacts(ctx, id, third); err != nil {
		t.Fatal(err)
	}
	var street string
	var unknownSource bool
	if err := pool.QueryRow(ctx, "SELECT display_name,address->>'street',address_source IS NULL,legal_name FROM directory_stations WHERE id=$1::uuid", id).Scan(&name, &street, &unknownSource, &legal); err != nil || name != "[P25-TEST] Curated" || street != "[P25-TEST] Curated street" || !unknownSource || legal != " [P25-TEST] Legal 3 LTDA " {
		t.Fatal(name, street, unknownSource, legal, err)
	}
}

func TestStationFactProjectionConcurrentApprovedNameAndImport(t *testing.T) {
	pool, store, canon := freshRegistryDB(t)
	ctx := context.Background()
	id, first := factVersion(t, pool, store, canon, 1, "[P25-TEST] First", registry.SourceCSV, true)
	if err := canon.ProjectSourceFacts(ctx, id, first); err != nil {
		t.Fatal(err)
	}
	_, next := factVersion(t, pool, store, canon, 2, "[P25-TEST] Next", registry.SourceCSV, true)
	correctionID, _ := NewUUIDv4()
	correctionUID, _ := mustUUID(correctionID)
	stationUID, _ := mustUUID(id)
	accountID, _ := NewUUIDv4()
	accountUID, _ := mustUUID(accountID)
	if _, err := directory.New(pool).CreateCorrection(ctx, directory.CreateCorrectionParams{ID: correctionUID, StationID: stationUID, AccountID: accountUID, Field: "display_name", ProposedValue: []byte(`{"value":"[P25-TEST] Concurrent approved"}`), ClientSubmissionID: "sna-concurrent-synthetic"}); err != nil {
		t.Fatal(err)
	}
	corrections := CorrectionStore{Pool: pool, Repo: canon.Repo}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if err := canon.ProjectSourceFacts(ctx, id, next); err != nil {
			t.Error(err)
		}
	}()
	go func() {
		defer wg.Done()
		if n, err := corrections.Decide(ctx, correctionID, "accepted", []string{}, "accepted", "synthetic evidence", "staff", "[P25-TEST] reviewer", []byte(`{}`)); err != nil || n != 1 {
			t.Error(n, err)
		}
	}()
	wg.Wait()
	var name, legal string
	if err := pool.QueryRow(ctx, "SELECT display_name,legal_name FROM directory_stations WHERE id=$1::uuid", id).Scan(&name, &legal); err != nil || name != "[P25-TEST] Concurrent approved" || legal != " [P25-TEST] Legal 2 LTDA " {
		t.Fatal(name, legal, err)
	}
}
