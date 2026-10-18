//go:build integration

package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/native"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const annual2024 = "https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/pmqc/pmqc_2024.zip"
const annual2025 = "https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/pmqc/pmqc_2025.zip"
const monthly202603 = "https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/pmqc/pmqc_2026_03.zip"

func member(archive, label, hashChar string) application.InventoryEntry {
	return application.InventoryEntry{
		Label: label, URL: archive, SHA256: strings.Repeat(hashChar, 64),
		Member: "PMQC_" + strings.ReplaceAll(label, "-", "_") + ".csv", Bytes: 10,
	}
}

func liveFixtures(t *testing.T) ([]native.ArchiveRef, func(native.ArchiveRef) []application.InventoryEntry) {
	t.Helper()
	refs := []native.ArchiveRef{
		{URL: annual2024, Year: "2024"},
		{URL: annual2025, Year: "2025"},
		{URL: monthly202603, Year: "2026", Month: "03"},
	}
	inspect := func(ref native.ArchiveRef) []application.InventoryEntry {
		switch ref.URL {
		case annual2024:
			return []application.InventoryEntry{member(annual2024, "2024-01", "1"), member(annual2024, "2024-02", "2"), member(annual2024, "2024-03", "3")}
		case annual2025:
			return []application.InventoryEntry{member(annual2025, "2025-01", "4")}
		case monthly202603:
			return []application.InventoryEntry{member(monthly202603, "2026-03", "5")}
		}
		t.Fatalf("unexpected inspection %s", ref.URL)
		return nil
	}
	return refs, inspect
}

func liveDiscovery(pool *pgxpool.Pool, refs []native.ArchiveRef, inspect func(native.ArchiveRef) []application.InventoryEntry, counts map[string]int) LiveDiscovery {
	return LiveDiscovery{
		Pool: pool, Source: "pmqc", Stream: "pmqc-assay-v1",
		List: func(context.Context) ([]native.ArchiveRef, error) { return refs, nil },
		Inspect: func(_ context.Context, ref native.ArchiveRef) ([]application.InventoryEntry, error) {
			if counts != nil {
				counts[ref.URL]++
			}
			return inspect(ref), nil
		},
		Enqueue: func(ctx context.Context, payload ImportPayload, dedupe string) (platformjobs.EnqueueResult, error) {
			raw, err := json.Marshal(payload)
			if err != nil {
				return platformjobs.EnqueueResult{}, err
			}
			return platformjobs.Enqueue(ctx, pool, "quality-import", raw, dedupe, 3, time.Now())
		},
		MaxEnqueues: 2,
	}
}

func importEditions(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), "SELECT DISTINCT payload->>'edition' FROM job_queue WHERE kind='quality-import' ORDER BY 1")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var edition string
		if err := rows.Scan(&edition); err != nil {
			t.Fatal(err)
		}
		out = append(out, edition)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func discoveryCursor(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	cursor, err := quality.New(pool).GetDiscoveryCursor(context.Background(), "quality-discovery")
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ""
		}
		t.Fatal(err)
	}
	return cursor.CursorUrl
}

// TestLiveDiscoveryWalksArchivesAndNeverStarvesMembers proves the bounded
// scan: one archive inspected per run, created-only budget accounting, a
// held cursor when a defensive cap leaves members unplanned, forward
// progress through later members and a wrap after the whole listing.
func TestLiveDiscoveryWalksArchivesAndNeverStarvesMembers(t *testing.T) {
	pool := freshJobsDB(t)
	ctx := context.Background()
	refs, inspect := liveFixtures(t)
	counts := map[string]int{}
	d := liveDiscovery(pool, refs, inspect, counts)

	// Run 1: three members planned, cap two → two created, cursor held.
	if err := d.Handle(ctx, platformjobs.Job{}); err != nil {
		t.Fatal(err)
	}
	if got := importEditions(t, pool); len(got) != 2 || got[0] != "2024-01" || got[1] != "2024-02" {
		t.Fatal("bounded first chunk", got)
	}
	if cursor := discoveryCursor(t, pool); cursor != "" {
		t.Fatal("cursor advanced past unplanned members", cursor)
	}
	// Run 2: the same archive re-inspected; the two converged members no
	// longer charge the budget, so the third member flows and the cursor
	// advances.
	if err := d.Handle(ctx, platformjobs.Job{}); err != nil {
		t.Fatal(err)
	}
	if got := importEditions(t, pool); len(got) != 3 || got[2] != "2024-03" {
		t.Fatal("starving member never imported", got)
	}
	if cursor := discoveryCursor(t, pool); cursor != annual2024 {
		t.Fatal("cursor did not advance after full planning", cursor)
	}
	// Run 3 and 4 visit the following archives exactly once each.
	if err := d.Handle(ctx, platformjobs.Job{}); err != nil {
		t.Fatal(err)
	}
	if cursor := discoveryCursor(t, pool); cursor != annual2025 {
		t.Fatal("second archive not reached", cursor)
	}
	if err := d.Handle(ctx, platformjobs.Job{}); err != nil {
		t.Fatal(err)
	}
	if cursor := discoveryCursor(t, pool); cursor != monthly202603 {
		t.Fatal("monthly archive not reached", cursor)
	}
	// Run 5 wraps to the first archive; everything converges, nothing forks.
	if err := d.Handle(ctx, platformjobs.Job{}); err != nil {
		t.Fatal(err)
	}
	if got := importEditions(t, pool); len(got) != 5 {
		t.Fatal("wrap duplicated work", got)
	}
	if cursor := discoveryCursor(t, pool); cursor != annual2024 {
		t.Fatal("wrap did not restart the listing", cursor)
	}
	if counts[annual2024] != 3 || counts[annual2025] != 1 || counts[monthly202603] != 1 {
		t.Fatal("inspection cadence diverged", counts)
	}
}

func seedMemberEdition(t *testing.T, pool *pgxpool.Pool, label, archive, hash, member string) {
	t.Helper()
	in := application.Input{
		Source: "pmqc", Stream: "pmqc-assay-v1", Edition: label,
		Resources: []application.Resource{{
			URL: archive, SHA256: hash, Bytes: 10,
			Member: member, MemberSHA256: strings.Repeat("e", 64), MemberBytes: 20,
		}},
		Counts:       domain.Counts{Input: 1, Accepted: 1},
		ManifestHash: strings.Repeat("f", 64),
		Assays: []application.AssayInput{
			{SampleRef: "S-1", AssayRef: "A", SourceCNPJ: "11222333000181", ObservedOn: "2024-01-04", Fuel: "ETHANOL", ResultText: "92,6", Unit: "%", Conformity: "CONFORMING", EvidenceRef: "f#S-1#A", Fingerprint: strings.Repeat("c", 64)},
		},
	}
	maker := &jobsIDMaker{}
	if _, err := adapters.NewPublisher(pool).Publish(context.Background(), in, maker.next); err != nil {
		t.Fatal(err)
	}
}

// TestLiveDiscoveryReplayCorrectionDeadAndRecovery covers the ledger
// interactions: a recorded member replays stable, republished bytes create
// a new correction job, a dead converged job never forks, and the audited
// operator replay recovers it.
func TestLiveDiscoveryReplayCorrectionDeadAndRecovery(t *testing.T) {
	pool := freshJobsDB(t)
	ctx := context.Background()
	original := strings.Repeat("1", 64)
	seedMemberEdition(t, pool, "2024-01", annual2024, original, "PMQC_2024_01.csv")
	refs := []native.ArchiveRef{{URL: annual2024, Year: "2024"}}
	current := member(annual2024, "2024-01", "1")
	d := liveDiscovery(pool, refs, func(native.ArchiveRef) []application.InventoryEntry { return []application.InventoryEntry{current} }, nil)

	// Stable replay behind the ledger enqueues nothing.
	if err := d.Handle(ctx, platformjobs.Job{}); err != nil {
		t.Fatal(err)
	}
	if got := importEditions(t, pool); len(got) != 0 {
		t.Fatal("stable replay enqueued", got)
	}
	// Republished bytes under the same label create one correction job.
	current = member(annual2024, "2024-01", "9")
	if err := d.Handle(ctx, platformjobs.Job{}); err != nil {
		t.Fatal(err)
	}
	rows, err := pool.Query(ctx, "SELECT payload->>'sha256' FROM job_queue WHERE kind='quality-import'")
	if err != nil {
		t.Fatal(err)
	}
	var hashes []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			t.Fatal(err)
		}
		hashes = append(hashes, h)
	}
	rows.Close()
	if len(hashes) != 1 || hashes[0] != current.SHA256 {
		t.Fatal("correction hash not pinned", hashes)
	}
	// Exhaust the job: a dead converged refresh must not fork on later runs.
	queue := platformjobs.NewQueue(pool)
	for i := 0; i < 3; i++ {
		claimed, cerr := queue.Claim(ctx, "quality-import", "discovery-test", time.Minute)
		if cerr != nil {
			t.Fatal(cerr)
		}
		if _, ferr := queue.Fail(ctx, claimed.ID, claimed.LeaseToken, 0, "synthetic outage"); ferr != nil {
			t.Fatal(ferr)
		}
	}
	if err := d.Handle(ctx, platformjobs.Job{}); err != nil {
		t.Fatal(err)
	}
	var count int
	var status string
	if err := pool.QueryRow(ctx, "SELECT count(*), min(status) FROM job_queue WHERE kind='quality-import'").Scan(&count, &status); err != nil {
		t.Fatal(err)
	}
	if count != 1 || status != "dead" {
		t.Fatal("dead converged job forked or vanished", count, status)
	}
	// The audited operator path recovers it without a new row.
	jobID := ""
	if err := pool.QueryRow(ctx, "SELECT id FROM job_queue WHERE kind='quality-import'").Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	if err := queue.ReplayDead(ctx, jobID, "discovery-test: source recovered"); err != nil {
		t.Fatal(err)
	}
	var attempts int
	if err := pool.QueryRow(ctx, "SELECT status,attempts FROM job_queue WHERE id=$1", jobID).Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != "queued" || attempts != 0 {
		t.Fatal("operator replay did not recover", status, attempts)
	}
}

// TestDiscoveryConvergedEnqueuesDoNotChargeTheBudget freezes the
// created-only budget rule on the injected-lister handler: with a budget of
// two, three bounded runs cover six editions instead of re-charging the
// same already-enqueued first members forever.
func TestDiscoveryConvergedEnqueuesDoNotChargeTheBudget(t *testing.T) {
	pool := freshJobsDB(t)
	ctx := context.Background()
	entries := []application.InventoryEntry{}
	for i, label := range []string{"2026-01", "2026-02", "2026-03", "2026-04", "2026-05", "2026-06"} {
		entries = append(entries, application.InventoryEntry{Label: label, URL: "u/" + label + ".zip", SHA256: strings.Repeat(string("123456789abcdef"[i]), 64)})
	}
	d := Discovery{
		Source: "pmqc", Stream: "pmqc-assay-v1", Pool: pool,
		List: func(context.Context) ([]application.InventoryEntry, error) { return entries, nil },
		Enqueue: func(ctx context.Context, payload ImportPayload, dedupe string) (platformjobs.EnqueueResult, error) {
			raw, err := json.Marshal(payload)
			if err != nil {
				return platformjobs.EnqueueResult{}, err
			}
			return platformjobs.Enqueue(ctx, pool, "quality-import", raw, dedupe, 3, time.Now())
		},
		MaxEditions: 2,
	}
	for run := 0; run < 3; run++ {
		if err := d.Handle(ctx, platformjobs.Job{}); err != nil {
			t.Fatal(err)
		}
	}
	got := importEditions(t, pool)
	if len(got) != len(entries) {
		t.Fatal("deduped enqueues starved later editions", got)
	}
	for i := range entries {
		if got[i] != entries[i].Label {
			t.Fatal("edition order diverged", got)
		}
	}
}
