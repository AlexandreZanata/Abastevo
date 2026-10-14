package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

func TestTrustAffectedBoundedContinuationAndRetryIdentity(t *testing.T) {
	key := boundaryKey("STANDARD")
	var after *domain.PriceKey
	var recomputes int
	var dedupes []string
	var continuations [][]byte
	h := TrustAffected{Batch: 1,
		Affected: func(_ context.Context, ref string, cursor *domain.PriceKey, limit int) ([]domain.PriceKey, error) {
			if ref != "synthetic-owner" || limit != 1 {
				t.Fatal("wrong owner/bound")
			}
			after = cursor
			if cursor != nil {
				return nil, nil
			}
			return []domain.PriceKey{key}, nil
		},
		Recompute: func(context.Context, domain.PriceKey) (domain.Result, error) {
			recomputes++
			return domain.Result{}, nil
		},
		Enqueue: func(_ context.Context, kind string, payload []byte, dedupe string) error {
			if kind != "trust-affected-recompute" {
				t.Fatal("wrong continuation kind")
			}
			dedupes = append(dedupes, dedupe)
			continuations = append(continuations, payload)
			return nil
		},
	}
	job := jobs.Job{ID: "original-job", Payload: []byte(`{"version":1,"contributor_ref":"synthetic-owner"}`)}
	for i := 0; i < 2; i++ {
		if err := h.Handle(context.Background(), job); err != nil {
			t.Fatal(err)
		}
	}
	if recomputes != 2 || dedupes[0] != dedupes[1] {
		t.Fatal("retry changed continuation identity")
	}
	job.ID = "later-trust-change"
	if err := h.Handle(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if dedupes[2] == dedupes[0] {
		t.Fatal("later trust change reused old completed continuation")
	}
	if err := h.Handle(context.Background(), jobs.Job{ID: "continuation-job", Payload: continuations[0]}); err != nil {
		t.Fatal(err)
	}
	if after == nil || *after != key || recomputes != 3 || len(dedupes) != 3 {
		t.Fatal("cursor did not drain exact page")
	}
}

func TestTrustAffectedRejectsMalformedMissingAndFailingPorts(t *testing.T) {
	valid := jobs.Job{ID: "job", Payload: []byte(`{"version":1,"contributor_ref":"synthetic-owner"}`)}
	reads, writes := 0, 0
	h := TrustAffected{Batch: 100,
		Affected: func(context.Context, string, *domain.PriceKey, int) ([]domain.PriceKey, error) {
			reads++
			return nil, nil
		},
		Recompute: func(context.Context, domain.PriceKey) (domain.Result, error) { writes++; return domain.Result{}, nil },
		Enqueue:   func(context.Context, string, []byte, string) error { writes++; return nil },
	}
	for _, raw := range []string{`{`, `{"version":2,"contributor_ref":"owner"}`, `{"version":1}`, `{"version":1,"contributor_ref":"owner","extra":true}`, `{"version":1,"contributor_ref":"owner"}{}`, `{"version":1,"contributor_ref":"owner","after":{"station_id":"s"}}`} {
		if err := h.Handle(context.Background(), jobs.Job{ID: "job", Payload: []byte(raw)}); err == nil {
			t.Fatalf("bad payload accepted: %s", raw)
		}
	}
	if reads != 0 || writes != 0 {
		t.Fatal("invalid job reached dependencies")
	}
	if err := (TrustAffected{}).Handle(context.Background(), valid); err == nil {
		t.Fatal("missing bounded ports accepted")
	}
	boom := errors.New("synthetic unavailable")
	h.Affected = func(context.Context, string, *domain.PriceKey, int) ([]domain.PriceKey, error) { return nil, boom }
	if err := h.Handle(context.Background(), valid); !errors.Is(err, boom) {
		t.Fatal("read failure swallowed", err)
	}
	h.Affected = func(context.Context, string, *domain.PriceKey, int) ([]domain.PriceKey, error) {
		return []domain.PriceKey{boundaryKey("STANDARD")}, nil
	}
	h.Recompute = func(context.Context, domain.PriceKey) (domain.Result, error) { return domain.Result{}, boom }
	if err := h.Handle(context.Background(), valid); !errors.Is(err, boom) || writes != 0 {
		t.Fatal("recompute failure completed or continued", err)
	}
	h.Batch = 1
	h.Recompute = func(context.Context, domain.PriceKey) (domain.Result, error) { return domain.Result{}, nil }
	h.Enqueue = func(context.Context, string, []byte, string) error { return boom }
	if err := h.Handle(context.Background(), valid); !errors.Is(err, boom) {
		t.Fatal("continuation failure swallowed", err)
	}
	h.Affected = func(context.Context, string, *domain.PriceKey, int) ([]domain.PriceKey, error) {
		return make([]domain.PriceKey, 2), nil
	}
	if err := h.Handle(context.Background(), valid); err == nil {
		t.Fatal("oversized page accepted")
	}
	var envelope trustAffectedPayload
	if err := json.Unmarshal(valid.Payload, &envelope); err != nil || envelope.ContributorRef != "synthetic-owner" {
		t.Fatal(err)
	}
}
