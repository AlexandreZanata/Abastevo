//go:build integration

package adapters

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	communityjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/adapters/jobs"
	communityapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
	trustadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trust/adapters"
	trustdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trust/domain"
	jobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

func TestTrustAffectedJobRebuildsAuthoredAndConfirmedKeysAcrossPages(t *testing.T) {
	s, pool, station := freshStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	seed := func(index int, author, product string) domain.Observation {
		obs, _, err := domain.NewObservation(domain.Params{ID: fmt.Sprintf("d0000000-0000-4000-8000-%012d", index), ContributorRef: author,
			ClientSubmissionID: fmt.Sprintf("anchor-%d", index), StationID: station, Product: product, Unit: "L", AmountMilli: 5000,
			RawText: "5,000", ConditionKind: "STANDARD", ReceivedAt: now.Add(-time.Minute)})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := s.Submit(ctx, obs, func(context.Context, pgx.Tx, string, []byte, string) error { return nil }); err != nil {
			t.Fatal(err)
		}
		claim, err := domain.ClaimValidation(obs, domain.StateReceived, "synthetic-job", domain.ActorWorker, now, 1)
		if err != nil {
			t.Fatal(err)
		}
		admit, err := domain.Admit(obs, domain.StateValidating, domain.ActorWorker, now, 2)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range []domain.Decision{claim, admit} {
			if err := s.RecordDecision(ctx, d); err != nil {
				t.Fatal(err)
			}
		}
		return obs
	}
	authored := seed(1, "synthetic-target", "GASOLINE_REGULAR")
	confirmed := seed(2, "synthetic-author", "ETHANOL")
	unrelated := seed(3, "synthetic-unrelated", "DIESEL_S10")
	confirmation, _, err := domain.NewConfirmation(domain.ConfirmationParams{ID: "c0000000-0000-4000-8000-000000000041", ObservationID: confirmed.ID,
		ContributorRef: "synthetic-target", AuthorRef: confirmed.ContributorRef, ClientSubmissionID: "support", ReceivedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Confirm(ctx, confirmation, func(context.Context, pgx.Tx, string, []byte, string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	trustStore := trustadapters.NewStore(pool)
	ports := communityapp.RecomputePorts{Clock: func() time.Time { return now }, Store: s, Anchors: s.EligibleAnchors, Confirmations: s.ConfirmationsForAnchors,
		Tiers: func(ctx context.Context, refs []string) (map[string]string, error) {
			out := map[string]string{}
			for _, ref := range refs {
				tier, err := trustStore.Tier(ctx, ref)
				if err != nil {
					return nil, err
				}
				out[ref] = tier
			}
			return out, nil
		},
		Photo: func(_ context.Context, obs domain.Observation) (communityapp.PhotoInfo, error) {
			return communityapp.PhotoInfo{Found: true, Validated: true, MediaKey: obs.ID}, nil
		},
		Moderation: func(context.Context, domain.PriceKey) (bool, error) { return false, nil },
	}
	for _, obs := range []domain.Observation{authored, confirmed, unrelated} {
		if _, err := communityapp.Recompute(ctx, ports, obs.ID); err != nil {
			t.Fatal(err)
		}
	}
	assertProjection := func(product, availability, confidence string, version int) {
		t.Helper()
		var gotAvailability, gotConfidence string
		var gotVersion int
		if err := pool.QueryRow(ctx, `SELECT availability,confidence,projection_version FROM community_current_prices WHERE station_id=$1 AND fuel_product=$2`, station, product).Scan(&gotAvailability, &gotConfidence, &gotVersion); err != nil || gotAvailability != availability || gotConfidence != confidence || gotVersion != version {
			t.Fatalf("%s projection=%s/%s/%d error=%v", product, gotAvailability, gotConfidence, gotVersion, err)
		}
	}
	assertProjection("ETHANOL", "AVAILABLE", "MEDIUM", 1)
	enqueue := func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error {
		_, err := jobs.Enqueue(ctx, tx, kind, payload, dedupe, 5, time.Time{})
		return err
	}
	change := func(id, tier string) {
		d, _, err := trustdomain.NewDecision(trustdomain.DecisionParams{ID: id, ContributorRef: "synthetic-target", Tier: tier, Reason: "synthetic audited review", CaseRefs: []string{"synthetic-case"}, OccurredAt: now})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := trustStore.AppendDecision(ctx, d, enqueue); err != nil {
			t.Fatal(err)
		}
	}
	change("b0000000-0000-4000-8000-000000000001", trustdomain.TierBlocked)
	h := communityjobs.TrustAffected{Batch: 1, Affected: s.AffectedContributorKeys, Recompute: func(ctx context.Context, key domain.PriceKey) (domain.Result, error) {
		return communityapp.RecomputeKey(ctx, ports, key)
	}}
	// Standalone continuation enqueue uses the pool, not an absent transaction.
	h.Enqueue = func(ctx context.Context, kind string, payload []byte, dedupe string) error {
		_, err := jobs.Enqueue(ctx, pool, kind, payload, dedupe, 5, time.Time{})
		return err
	}
	queue := jobs.NewQueue(pool)
	first, err := queue.Claim(ctx, h.Kind(), "synthetic-worker", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	failing := h
	failing.Recompute = func(context.Context, domain.PriceKey) (domain.Result, error) {
		return domain.Result{}, errors.New("synthetic recompute unavailable")
	}
	if err := failing.Handle(ctx, first); err == nil {
		t.Fatal("failed recompute completed")
	}
	if _, err := queue.Fail(ctx, first.ID, first.LeaseToken, 0, "synthetic retry"); err != nil {
		t.Fatal(err)
	}
	drain := func() {
		count := 0
		for {
			job, err := queue.Claim(ctx, h.Kind(), "synthetic-worker", time.Minute)
			if errors.Is(err, jobs.ErrNoJob) {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := h.Handle(ctx, job); err != nil {
				t.Fatal(err)
			}
			if err := queue.Complete(ctx, job.ID, job.LeaseToken); err != nil {
				t.Fatal(err)
			}
			count++
			if count > 3 {
				t.Fatal("continuation failed to terminate")
			}
		}
		if count != 3 {
			t.Fatalf("bounded pages=%d, want2 keys plus terminal empty page", count)
		}
	}
	drain()
	assertProjection("GASOLINE_REGULAR", "UNKNOWN", "", 2)
	assertProjection("ETHANOL", "AVAILABLE", "LOW", 2)
	assertProjection("DIESEL_S10", "AVAILABLE", "LOW", 1)
	change("b0000000-0000-4000-8000-000000000002", trustdomain.TierEstablished)
	drain() // A later verdict must not converge on the first DONE job.
	assertProjection("GASOLINE_REGULAR", "AVAILABLE", "LOW", 3)
	assertProjection("ETHANOL", "AVAILABLE", "MEDIUM", 3)
	assertProjection("DIESEL_S10", "AVAILABLE", "LOW", 1)
	for _, limit := range []int{0, 101} {
		if _, err := s.AffectedContributorKeys(ctx, "synthetic-target", nil, limit); err == nil {
			t.Fatal("unbounded key page accepted")
		}
	}
}
