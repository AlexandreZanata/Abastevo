//go:build integration

package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/registry"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

type reprojectionBarrier struct {
	inner    StationFactReprojection
	arrivals atomic.Int32
	both     chan struct{}
}

func (b *reprojectionBarrier) Kind() string { return b.inner.Kind() }
func (b *reprojectionBarrier) Version() int { return 1 }
func (b *reprojectionBarrier) Handle(ctx context.Context, job platformjobs.Job) error {
	var payload stationFactReprojectionPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return err
	}
	if payload.StationID != "" {
		arrival := b.arrivals.Add(1)
		if arrival == 2 {
			close(b.both)
		}
		if arrival <= 2 {
			select {
			case <-b.both:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return b.inner.Handle(ctx, job)
}

func TestStationFactReprojectionPageRollbackReplayAndTwoDispatchers(t *testing.T) {
	pool, store, canon := freshRegistryDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	run, _ := NewUUIDv4()
	if _, _, err := store.CreateRun(ctx, run, registry.SourceCSV, "sna-c-synthetic", "synthetic"); err != nil {
		t.Fatal(err)
	}
	for index, cnpj := range []string{"91000000000066", "12ABC34501DE35", "00428184000195", "04218406000104"} {
		a := registry.Assertion{Source: registry.SourceCSV, SourceKey: cnpj, Checksum: fmt.Sprintf("%x", sha256.Sum256([]byte(cnpj))), DisplayName: fmt.Sprintf("[P25-TEST] LEGAL %d", index), LegalName: fmt.Sprintf(" [P25-TEST] LEGAL %d ", index), TradeName: fmt.Sprintf("[P25-TEST] Trade %d", index), Address: map[string]string{"street": "[P25-TEST] Street", "municipio": "TEST CITY", "uf": "SP"}, AuthState: "unknown", Eligibility: "pending", LocationQuality: "unknown"}
		if _, err := store.StageAssertion(ctx, a.WithRun(run)); err != nil {
			t.Fatal(err)
		}
		id, err := canon.ResolveStation(ctx, cnpj, a.DisplayName, a.Address)
		if err != nil {
			t.Fatal(err)
		}
		rows, err := store.ListAssertions(ctx, run)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			if row.SourceKey == cnpj {
				if err := store.SetAssertionStation(ctx, row.ID, id); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	if err := store.FinishRun(ctx, run, "complete", 4, 0, 0, ""); err != nil {
		t.Fatal(err)
	}
	campaign, _ := NewUUIDv4()
	first, err := StartStationFactReprojection(ctx, pool, campaign, "[P25-TEST] operator", "SNA synthetic campaign")
	if err != nil || !first.Created {
		t.Fatal(first, err)
	}
	replay, err := StartStationFactReprojection(ctx, pool, campaign, "[P25-TEST] operator", "SNA synthetic campaign")
	if err != nil || replay.Created || replay.ID != first.ID {
		t.Fatal(replay, err)
	}
	if _, err := StartStationFactReprojection(ctx, pool, campaign, "[P25-TEST] other", "SNA synthetic campaign"); err == nil {
		t.Fatal("campaign audit changed")
	}
	handler := StationFactReprojection{Pool: pool, BatchSize: 2}
	raw, _ := json.Marshal(stationFactReprojectionPayload{Version: 1, CampaignID: campaign, AfterID: zeroStationFactCursor})
	// Fail the second enqueue of the page, after an earlier enqueue succeeded.
	if _, err := pool.Exec(ctx, `CREATE FUNCTION sna_fail_page() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.payload->>'station_id'=(SELECT id::text FROM directory_stations ORDER BY id OFFSET 1 LIMIT 1) THEN RAISE EXCEPTION 'synthetic page interruption'; END IF; RETURN NEW; END $$;
 CREATE TRIGGER sna_fail_page BEFORE INSERT ON job_queue FOR EACH ROW WHEN(NEW.kind='station-facts-reproject') EXECUTE FUNCTION sna_fail_page();`); err != nil {
		t.Fatal(err)
	}
	if err := handler.Handle(ctx, platformjobs.Job{Payload: raw}); err == nil {
		t.Fatal("interrupted page committed")
	}
	status, err := ReadStationFactReprojectionStatus(ctx, pool, campaign)
	if err != nil || len(status.Stations) != 0 || status.Scan["queued"] != 1 || status.Complete {
		t.Fatal("partial page", status, err)
	}
	if _, err := pool.Exec(ctx, "DROP TRIGGER sna_fail_page ON job_queue; DROP FUNCTION sna_fail_page()"); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := handler.Handle(ctx, platformjobs.Job{Payload: raw}); err != nil {
			t.Fatal(err)
		}
	}
	status, err = ReadStationFactReprojectionStatus(ctx, pool, campaign)
	if err != nil || status.Stations["queued"] != 2 || status.Scan["queued"] != 2 {
		t.Fatal("page cap/dedupe", status, err)
	}
	barrier := &reprojectionBarrier{inner: handler, both: make(chan struct{})}
	var wg sync.WaitGroup
	for worker := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dispatch := platformjobs.Dispatcher{Queue: platformjobs.NewQueue(pool), Handlers: map[string]platformjobs.Handler{StationFactReprojectionKind: barrier}, WorkerID: fmt.Sprint("sna-worker-", worker), LeaseTTL: time.Minute, RetryDelay: time.Millisecond}
			for range 32 {
				found, err := dispatch.RunOnce(ctx)
				if err != nil {
					t.Error(err)
					return
				}
				if !found {
					return
				}
			}
			t.Error("bounded campaign did not drain")
		}()
	}
	wg.Wait()
	status, err = ReadStationFactReprojectionStatus(ctx, pool, campaign)
	if err != nil || !status.Complete || status.Stations["done"] != 4 || status.Scan["done"] != 3 {
		t.Fatal("campaign incomplete", status, err)
	}
	if barrier.arrivals.Load() != 4 {
		t.Fatal("duplicate station work", barrier.arrivals.Load())
	}
	var projected, assertions int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM directory_stations WHERE legal_name IS NOT NULL AND display_name LIKE '[P25-TEST] Trade %' AND address_source='anp'").Scan(&projected); err != nil || projected != 4 {
		t.Fatal(projected, err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM registry_assertions").Scan(&assertions); err != nil || assertions != 4 {
		t.Fatal("immutable evidence changed", assertions, err)
	}
	if err := handler.Handle(ctx, platformjobs.Job{Payload: raw}); err != nil {
		t.Fatal(err)
	}
	after, err := ReadStationFactReprojectionStatus(ctx, pool, campaign)
	if err != nil || !after.Complete || after.Stations["done"] != 4 {
		t.Fatal("replay forked work", after, err)
	}
}

func TestStationFactReprojectionLatestSourceAndFailureRemainVisible(t *testing.T) {
	pool, store, canon := freshRegistryDB(t)
	ctx := context.Background()
	id, first := factVersion(t, pool, store, canon, 1, "[P25-TEST] Old", registry.SourceCSV, true)
	campaign, _ := NewUUIDv4()
	if _, err := StartStationFactReprojection(ctx, pool, campaign, "[P25-TEST] operator", "synthetic latest"); err != nil {
		t.Fatal(err)
	}
	_, latest := factVersion(t, pool, store, canon, 2, "[P25-TEST] Latest", registry.SourceCSV, true)
	if latest == first {
		t.Fatal("fixture did not change source")
	}
	raw, _ := json.Marshal(stationFactReprojectionPayload{Version: 1, CampaignID: campaign, StationID: id})
	handler := StationFactReprojection{Pool: pool}
	for range 2 {
		if err := handler.Handle(ctx, platformjobs.Job{Payload: raw}); err != nil {
			t.Fatal(err)
		}
	}
	var got string
	if err := pool.QueryRow(ctx, "SELECT display_name FROM directory_stations WHERE id=$1::uuid", id).Scan(&got); err != nil || got != "[P25-TEST] Latest" {
		t.Fatal(got, err)
	}
	// A real queue dead letter is never reported as a completed campaign.
	if _, err := pool.Exec(ctx, "UPDATE job_queue SET status='dead' WHERE id=(SELECT id FROM job_queue LIMIT 1)"); err != nil {
		t.Fatal(err)
	}
	status, err := ReadStationFactReprojectionStatus(ctx, pool, campaign)
	if err != nil || status.Complete || status.Scan["dead"] != 1 {
		t.Fatal(status, err)
	}
	noSource, err := canon.ResolveStation(ctx, "12ABC34501DE35", "[P25-TEST] No source", map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	missingRaw, _ := json.Marshal(stationFactReprojectionPayload{Version: 1, CampaignID: campaign, StationID: noSource})
	if _, err := platformjobs.Enqueue(ctx, pool, StationFactReprojectionKind, missingRaw, factCampaignPrefix(campaign, "station")+noSource, 1, time.Now()); err != nil {
		t.Fatal(err)
	}
	dispatch := platformjobs.Dispatcher{Queue: platformjobs.NewQueue(pool), Handlers: map[string]platformjobs.Handler{StationFactReprojectionKind: handler}, WorkerID: "sna-failure", LeaseTTL: time.Minute, RetryDelay: time.Millisecond}
	if err := handler.Handle(ctx, platformjobs.Job{Payload: missingRaw}); err == nil {
		t.Fatal("missing source handler succeeded")
	}
	if found, err := dispatch.RunOnce(ctx); !found || err != nil {
		t.Fatal("dispatcher failed to persist handler failure", found, err)
	}
	status, err = ReadStationFactReprojectionStatus(ctx, pool, campaign)
	if err != nil || status.Complete || status.Stations["dead"] != 1 {
		t.Fatal("projection failure hidden", status, err)
	}
	var untouched bool
	if err := pool.QueryRow(ctx, "SELECT legal_name IS NULL AND facts_source_clock IS NULL FROM directory_stations WHERE id=$1::uuid", noSource).Scan(&untouched); err != nil || !untouched {
		t.Fatal("missing source changed canonical metadata", untouched, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	other, _ := NewUUIDv4()
	if _, err := StartStationFactReprojection(cancelled, pool, other, "[P25-TEST] operator", "synthetic cancelled"); err == nil {
		t.Fatal("cancelled campaign created")
	}
	var campaigns int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM directory_station_fact_reprojection_campaigns").Scan(&campaigns); err != nil || campaigns != 1 {
		t.Fatal("cancelled audit leaked", campaigns, err)
	}
}
