//go:build integration

package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/native"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
)

func TestFineDiscoveryCursorRecoveryCorrectionDeadBudgetAndConcurrency(t *testing.T) {
	p := freshJobsDB(t)
	ctx := context.Background()
	ref := func(year string) native.FineRef {
		return native.FineRef{URL: "https://www.gov.br" + native.FineResourcePrefix + "multas-aplicadas-" + year + ".csv", Year: year}
	}
	refs := []native.FineRef{ref("2025"), ref("2020"), ref("2024")}
	var inspected []string
	hashes := map[string]string{"2020": strings.Repeat("a", 64), "2024": strings.Repeat("b", 64), "2025": strings.Repeat("c", 64)}
	d := FineDiscovery{Pool: p, Serial: adapters.NativeSourceSerial(p), List: func(context.Context) ([]native.FineRef, error) { return refs, nil }, Inspect: func(_ context.Context, r native.FineRef) (application.InventoryEntry, error) {
		inspected = append(inspected, r.Year)
		return application.InventoryEntry{URL: r.URL, Label: r.Year, SHA256: hashes[r.Year], Bytes: 100}, nil
	}, Enqueue: func(ctx context.Context, payload ImportPayload, key string) (platformjobs.EnqueueResult, error) {
		b, e := json.Marshal(payload)
		if e != nil {
			return platformjobs.EnqueueResult{}, e
		}
		return platformjobs.Enqueue(ctx, p, NativeFineImportKind, b, key, 3, time.Time{})
	}}
	cursor := func() string {
		row, e := quality.New(p).GetDiscoveryCursor(ctx, d.Kind())
		if errors.Is(e, pgx.ErrNoRows) {
			return ""
		}
		if e != nil {
			t.Fatal(e)
		}
		return row.CursorUrl
	}
	count := func(want int) {
		var n int
		if e := p.QueryRow(ctx, "SELECT count(*) FROM job_queue WHERE kind=$1", NativeFineImportKind).Scan(&n); e != nil || n != want {
			t.Fatal("unexpected import jobs", n, want, e)
		}
	}
	// Lost cursor acknowledgment after enqueue must converge the next retry.
	if _, e := p.Exec(ctx, `CREATE FUNCTION synthetic_fine_cursor_failure() RETURNS trigger LANGUAGE plpgsql AS $test$ BEGIN RAISE EXCEPTION 'private failure'; END $test$;
CREATE TRIGGER synthetic_fine_cursor_failure BEFORE INSERT OR UPDATE ON quality_discovery_cursors FOR EACH ROW EXECUTE FUNCTION synthetic_fine_cursor_failure()`); e != nil {
		t.Fatal(e)
	}
	if e := d.Handle(ctx, platformjobs.Job{}); e == nil || strings.Contains(e.Error(), "private") {
		t.Fatal("lost cursor acknowledgment accepted/leaked", e)
	}
	count(1)
	if cursor() != "" {
		t.Fatal("failed cursor advanced")
	}
	if _, e := p.Exec(ctx, "DROP TRIGGER synthetic_fine_cursor_failure ON quality_discovery_cursors; DROP FUNCTION synthetic_fine_cursor_failure()"); e != nil {
		t.Fatal(e)
	}
	for range 3 {
		if e := d.Handle(ctx, platformjobs.Job{}); e != nil {
			t.Fatal(e)
		}
	}
	count(3)
	if strings.Join(inspected, ",") != "2020,2020,2024,2025" || cursor() != ref("2025").URL {
		t.Fatal("starvation/unbounded inspection", inspected)
	}
	if _, e := p.Exec(ctx, "UPDATE job_queue SET status='dead',attempts=3 WHERE kind=$1", NativeFineImportKind); e != nil {
		t.Fatal(e)
	}
	for range 3 {
		if e := d.Handle(ctx, platformjobs.Job{}); e != nil {
			t.Fatal(e)
		}
	}
	count(3)
	var untouched int
	if e := p.QueryRow(ctx, "SELECT count(*) FROM job_queue WHERE kind=$1 AND status='dead' AND attempts=3", NativeFineImportKind).Scan(&untouched); e != nil || untouched != 3 {
		t.Fatal("dead attempt budget reset", untouched, e)
	}
	hashes["2024"] = strings.Repeat("d", 64)
	for range 2 {
		if e := d.Handle(ctx, platformjobs.Job{}); e != nil {
			t.Fatal(e)
		}
	}
	count(4)
	if cursor() != ref("2024").URL {
		t.Fatal("correction cursor not advanced")
	}
	// Three simultaneous runs serialize the shared cursor and each inspect one
	// successive resource, instead of regressing/duplicating scan progress.
	before := len(inspected)
	var wg sync.WaitGroup
	errs := make(chan error, 3)
	for range 3 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- d.Handle(ctx, platformjobs.Job{}) }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	if strings.Join(inspected[before:], ",") != "2025,2020,2024" || cursor() != ref("2024").URL {
		t.Fatal("cursor serialization failed", inspected[before:])
	}
	count(4)
	var foreign int
	if e := p.QueryRow(ctx, "SELECT count(*) FROM quality_discovery_cursors WHERE kind='quality-discovery'").Scan(&foreign); e != nil || foreign != 0 {
		t.Fatal("PMQC cursor touched", foreign, e)
	}
}

func TestFineDiscoveryFailuresKeepCursorAndEnqueueNothing(t *testing.T) {
	p := freshJobsDB(t)
	ctx := context.Background()
	ref := native.FineRef{URL: "https://www.gov.br" + native.FineResourcePrefix + "multas-aplicadas-2025.csv", Year: "2025"}
	entry := application.InventoryEntry{URL: ref.URL, Label: ref.Year, SHA256: strings.Repeat("a", 64), Bytes: 10}
	d := FineDiscovery{Pool: p, Serial: adapters.NativeSourceSerial(p), List: func(context.Context) ([]native.FineRef, error) { return []native.FineRef{ref}, nil }, Inspect: func(context.Context, native.FineRef) (application.InventoryEntry, error) { return entry, nil }, Enqueue: func(context.Context, ImportPayload, string) (platformjobs.EnqueueResult, error) {
		return platformjobs.EnqueueResult{}, errors.New("private enqueue failure")
	}}
	for _, tc := range []struct {
		name   string
		change func(*FineDiscovery)
	}{
		{"list failure", func(d *FineDiscovery) {
			d.List = func(context.Context) ([]native.FineRef, error) { return nil, errors.New("private source failure") }
		}},
		{"duplicate refs", func(d *FineDiscovery) {
			d.List = func(context.Context) ([]native.FineRef, error) { return []native.FineRef{ref, ref}, nil }
		}},
		{"foreign period", func(d *FineDiscovery) {
			d.List = func(context.Context) ([]native.FineRef, error) {
				r := ref
				r.Year = "2024"
				return []native.FineRef{r}, nil
			}
		}},
		{"inspect failure", func(d *FineDiscovery) {
			d.Inspect = func(context.Context, native.FineRef) (application.InventoryEntry, error) {
				return application.InventoryEntry{}, errors.New("private body")
			}
		}},
		{"wrong identity", func(d *FineDiscovery) {
			d.Inspect = func(context.Context, native.FineRef) (application.InventoryEntry, error) {
				r := entry
				r.URL += "?x"
				return r, nil
			}
		}},
		{"invalid hash", func(d *FineDiscovery) {
			d.Inspect = func(context.Context, native.FineRef) (application.InventoryEntry, error) {
				r := entry
				r.SHA256 = "bad"
				return r, nil
			}
		}},
		{"oversize", func(d *FineDiscovery) {
			d.Inspect = func(context.Context, native.FineRef) (application.InventoryEntry, error) {
				r := entry
				r.Bytes = 101 << 20
				return r, nil
			}
		}},
		{"enqueue failure", func(*FineDiscovery) {}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := d
			tc.change(&c)
			if e := c.Handle(ctx, platformjobs.Job{}); e == nil || strings.Contains(e.Error(), "private") {
				t.Fatal("failure accepted/leaked", e)
			}
			var n int
			if e := p.QueryRow(ctx, "SELECT (SELECT count(*) FROM quality_discovery_cursors)+(SELECT count(*) FROM job_queue)").Scan(&n); e != nil || n != 0 {
				t.Fatal("failed planning advanced", n, e)
			}
		})
	}
}
