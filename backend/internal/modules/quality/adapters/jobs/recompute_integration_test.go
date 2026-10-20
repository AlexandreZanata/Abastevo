//go:build integration

package jobs

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

func TestNewMethodologyDoesNotReuseCompletedLegacyRecompute(t *testing.T) {
	pool := freshJobsDB(t)
	ctx := context.Background()
	const revision = "aaaaaaaa-1111-4111-8111-999999999999"
	if _, err := platformjobs.Enqueue(ctx, pool, RecomputeKind, []byte(`{"version":1}`), "quality-recompute:"+revision, 1, time.Time{}); err != nil {
		t.Fatal(err)
	}
	queue := platformjobs.NewQueue(pool)
	claimed, err := queue.Claim(ctx, RecomputeKind, "legacy-policy", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err = queue.Complete(ctx, claimed.ID, claimed.LeaseToken); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = EnqueueRecompute(ctx, pool, "pmqc", "pmqc-assay-v1", revision); err != nil {
			t.Fatal(err)
		}
	}
	var queued, done int
	if err = pool.QueryRow(ctx, "SELECT count(*) FILTER(WHERE status='queued'),count(*) FILTER(WHERE status='done') FROM job_queue WHERE kind='quality-recompute'").Scan(&queued, &done); err != nil || queued != 1 || done != 1 {
		t.Fatal("new policy reused completed work or forked intents", queued, done, err)
	}
}

// TestImportEnqueuesRecomputeWithPublishedRevision proves the publication →
// recomputation handoff: the revision identity is queued with a converging
// dedupe key and the handler applies it; stale or malformed envelopes never
// touch derived rows.
func TestImportEnqueuesRecomputeWithPublishedRevision(t *testing.T) {
	pool := freshJobsDB(t)
	ctx := context.Background()
	raw := []byte("S-1;11222333000181;2026-09-04;ETANOL;A;92,6;%;CONFORME\n")
	hash := strings.Repeat("e", 64)
	im := Import{
		Pool: pool, Publish: adapters.NewPublisher(pool),
		Fetch:   func(context.Context, string) ([]byte, error) { return raw, nil },
		Prepare: fakePrepare(hash),
	}
	job := importJob(t, raw)
	for i := 0; i < 2; i++ {
		if err := im.Handle(ctx, job); err != nil {
			t.Fatalf("import %d: %v", i, err)
		}
	}
	rows, err := pool.Query(ctx, "SELECT payload::text FROM job_queue WHERE kind='quality-recompute'")
	if err != nil {
		t.Fatal(err)
	}
	var payloads [][]byte
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			t.Fatal(err)
		}
		payloads = append(payloads, payload)
	}
	rows.Close()
	if len(payloads) != 1 {
		t.Fatal("replay forked recompute intents", len(payloads))
	}
	var envelope RecomputePayload
	if err := json.Unmarshal(payloads[0], &envelope); err != nil {
		t.Fatal(err)
	}
	current, err := quality.New(pool).GetCurrentRevision(ctx, quality.GetCurrentRevisionParams{Source: "pmqc", Stream: "pmqc-assay-v1"})
	if err != nil {
		t.Fatal(err)
	}
	if envelope.Version != 1 || envelope.Source != "pmqc" || envelope.Stream != "pmqc-assay-v1" || envelope.RevisionID != uuidText(current) {
		t.Fatal("recompute envelope wrong", envelope, current)
	}
	handler := Recompute{Store: adapters.NewProjectionStore(pool)}
	if err := handler.Handle(ctx, platformjobs.Job{Payload: payloads[0]}); err != nil {
		t.Fatal(err)
	}
	if err := handler.Handle(ctx, platformjobs.Job{Payload: payloads[0]}); err != nil {
		t.Fatal("replay failed", err)
	}
	stale, err := json.Marshal(RecomputePayload{Version: 1, Source: "pmqc", Stream: "pmqc-assay-v1", RevisionID: "aaaaaaaa-1111-4111-8111-999999999999"})
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.Handle(ctx, platformjobs.Job{Payload: stale}); err != nil {
		t.Fatal("stale basis did not complete as no-op", err)
	}
	if err := handler.Handle(ctx, platformjobs.Job{Payload: []byte(`{"version":1}`)}); err == nil {
		t.Fatal("malformed envelope accepted")
	}
}
