//go:build integration

package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	accountadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
	identityadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	notificationsadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/notifications/adapters"
	notificationsapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/notifications/application"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// decisionNotices composes the real chain exactly like the worker and ops
// composition roots (contributionProducers + contributionNotices):
// attribution token -> identity contributor -> bound account -> inbox.
func decisionNotices(pool *pgxpool.Pool) application.DecisionNotifier {
	identity := identityadapters.NewRegistrar(pool)
	accounts := accountadapters.NewPGStore(pool)
	producers := notificationsapp.Producers{
		Notifier: notificationsadapters.NewStore(pool),
		Clock:    time.Now,
		AccountOfContributor: func(ctx context.Context, tx pgx.Tx, ref string) (string, bool, error) {
			contributor, found, err := identity.ContributorForAttributionTx(ctx, tx, ref)
			if err != nil || !found {
				return "", false, err
			}
			return accounts.FindBindingOwnerTx(ctx, tx, contributor)
		},
	}
	return func(ctx context.Context, tx pgx.Tx, n application.DecisionNotice) error {
		return producers.ContributionDecided(ctx, tx, notificationsapp.ContributionFacts{
			ContributorRef: n.ContributorRef, ObservationID: n.ObservationID, StationID: n.StationID,
			StationName: n.StationName, Fuel: n.Product, Validated: n.Validated, Reason: n.Reason,
		})
	}
}

// boundContributor creates a contributor with the attribution token ref
// and, when bound, an active account owning it.
func boundContributor(t *testing.T, pool *pgxpool.Pool, ref string, bound bool) string {
	t.Helper()
	ctx := context.Background()
	contributor := fmt.Sprintf("c0000000-0000-4000-8000-%012d", fixtureSeq.Add(1))
	if _, err := pool.Exec(ctx, `INSERT INTO identity_contributors (id, attribution_token) VALUES ($1, $2)`, contributor, ref); err != nil {
		t.Fatal(err)
	}
	if !bound {
		return ""
	}
	account := fmt.Sprintf("a0000000-0000-4000-8000-%012d", fixtureSeq.Add(1))
	if _, err := pool.Exec(ctx, `INSERT INTO accounts (id, alias, status) VALUES ($1, $2, 'active')`, account, "synthetic-"+account); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO account_contributor_bindings (account_id, contributor_id, key_fingerprint) VALUES ($1, $2, 'fp-synthetic')`, account, contributor); err != nil {
		t.Fatal(err)
	}
	return account
}

func submitFor(t *testing.T, s *Store, stationID, ref string) domain.Observation {
	t.Helper()
	obs := testObs(stationID, fmt.Sprintf("sub-%d", fixtureSeq.Add(1)))
	obs.ID = fmt.Sprintf("d6c74c23-63db-4c24-a2e5-%012d", fixtureSeq.Add(1))
	obs.ContributorRef = ref
	var jobs [][]byte
	if _, _, err := s.Submit(context.Background(), obs, enqueueStub(&jobs)); err != nil {
		t.Fatal(err)
	}
	claim, err := domain.ClaimValidation(obs, domain.StateReceived, "job-1", domain.ActorWorker, time.Now(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordDecision(context.Background(), claim); err != nil {
		t.Fatal(err)
	}
	return obs
}

func admitWithJob(s *Store, obs domain.Observation, enqueue func(context.Context, pgx.Tx, string, []byte, string) error) error {
	admit, err := domain.Admit(obs, domain.StateValidating, domain.ActorWorker, time.Now(), 2)
	if err != nil {
		return err
	}
	return s.RecordDecisionWithJob(context.Background(), admit, application.ConsensusJobKind, []byte(`{}`), "consensus:"+obs.ID, enqueue)
}

func realEnqueue(ctx context.Context, tx pgx.Tx, kind string, p []byte, dedupe string) error {
	_, err := platformjobs.Enqueue(ctx, tx, kind, p, dedupe, 5, time.Time{})
	return err
}

type storedNotice struct {
	Kind    string
	Key     string
	Payload map[string]any
}

func inbox(t *testing.T, pool *pgxpool.Pool, account string) []storedNotice {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT kind, event_key, payload FROM account_notifications WHERE account_id=$1 ORDER BY created_at, kind`, account)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []storedNotice
	for rows.Next() {
		var n storedNotice
		var raw []byte
		if err := rows.Scan(&n.Kind, &n.Key, &raw); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &n.Payload); err != nil {
			t.Fatal(err)
		}
		out = append(out, n)
	}
	return out
}

func TestDecisionNotices_ValidatedThenInvalidatedReachTheBoundAccount(t *testing.T) {
	s, pool, station := freshStore(t)
	s.Decided = decisionNotices(pool)
	account := boundContributor(t, pool, "tok-bound-1", true)
	obs := submitFor(t, s, station, "tok-bound-1")
	if got := inbox(t, pool, account); len(got) != 0 {
		t.Fatalf("a claim is not a decision: %v", got)
	}
	if err := admitWithJob(s, obs, realEnqueue); err != nil {
		t.Fatal(err)
	}
	got := inbox(t, pool, account)
	if len(got) != 1 || got[0].Kind != "contribution.validated" || got[0].Key != "contribution.validated:"+obs.ID {
		t.Fatalf("validated notices = %+v", got)
	}
	want := map[string]any{"observation_id": obs.ID, "station_id": station, "station_name": "Posto T", "fuel": "GASOLINE_REGULAR"}
	for k, v := range want {
		if got[0].Payload[k] != v {
			t.Fatalf("payload %s = %v, want %v", k, got[0].Payload[k], v)
		}
	}
	if len(got[0].Payload) != len(want) {
		t.Fatalf("validated payload carries extra fields: %v", got[0].Payload)
	}
	// A replayed admit is refused by the machine and adds nothing.
	if err := admitWithJob(s, obs, realEnqueue); err == nil {
		t.Fatal("replayed admit recorded")
	}
	// Moderator invalidation is a rejection with its stable reason.
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.InvalidateTx(ctx, tx, obs.ID, "f0000000-0000-4000-8000-000000000001", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	got = inbox(t, pool, account)
	if len(got) != 2 || got[1].Kind != "contribution.rejected" || got[1].Payload["reason"] != "moderation-invalid" || got[1].Payload["fuel"] != "GASOLINE_REGULAR" {
		t.Fatalf("invalidation notices = %+v", got)
	}
}

func TestDecisionNotices_WorkerRejectionCarriesItsFirstReason(t *testing.T) {
	s, pool, station := freshStore(t)
	s.Decided = decisionNotices(pool)
	account := boundContributor(t, pool, "tok-bound-2", true)
	obs := submitFor(t, s, station, "tok-bound-2")
	reject, err := domain.Reject(obs, domain.StateValidating, domain.ActorWorker, []string{application.ReasonEvidenceTimeout, application.ReasonEvidenceReused}, time.Now(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordDecision(context.Background(), reject); err != nil {
		t.Fatal(err)
	}
	got := inbox(t, pool, account)
	if len(got) != 1 || got[0].Kind != "contribution.rejected" || got[0].Payload["reason"] != "evidence-timeout" {
		t.Fatalf("rejection notices = %+v", got)
	}
}

func TestDecisionNotices_ContributorsWithoutAnAccountGetNothing(t *testing.T) {
	s, pool, station := freshStore(t)
	s.Decided = decisionNotices(pool)
	boundContributor(t, pool, "tok-device-only", false)
	for _, ref := range []string{"tok-device-only", "tok-no-identity"} {
		obs := submitFor(t, s, station, ref)
		if err := admitWithJob(s, obs, realEnqueue); err != nil {
			t.Fatalf("%s: the decision must still commit: %v", ref, err)
		}
		history, err := s.Decisions(context.Background(), obs.ID)
		if err != nil || len(history) != 2 || history[1].ToState != domain.StateValidated {
			t.Fatalf("%s: decisions %+v %v", ref, history, err)
		}
	}
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM account_notifications").Scan(&n); err != nil || n != 0 {
		t.Fatalf("unbound contributors produced %d notices (%v)", n, err)
	}
}

func TestDecisionNotices_ErasedObservationsStaySilent(t *testing.T) {
	s, pool, station := freshStore(t)
	s.Decided = decisionNotices(pool)
	account := boundContributor(t, pool, "tok-bound-3", true)
	obs := submitFor(t, s, station, "tok-bound-3")
	if _, err := pool.Exec(context.Background(), "UPDATE community_observations SET deleted_at = now() WHERE id = $1", obs.ID); err != nil {
		t.Fatal(err)
	}
	if err := admitWithJob(s, obs, realEnqueue); err != nil {
		t.Fatal(err)
	}
	if got := inbox(t, pool, account); len(got) != 0 {
		t.Fatalf("an erased observation notified: %+v", got)
	}
}

func TestDecisionNotices_ShareTheDecisionTransaction(t *testing.T) {
	s, pool, station := freshStore(t)
	s.Decided = decisionNotices(pool)
	account := boundContributor(t, pool, "tok-bound-4", true)
	obs := submitFor(t, s, station, "tok-bound-4")
	// A failed consensus enqueue rolls back the decision and its notice.
	if err := admitWithJob(s, obs, func(context.Context, pgx.Tx, string, []byte, string) error { return errors.New("queue down") }); err == nil {
		t.Fatal("enqueue failure accepted")
	}
	if got := inbox(t, pool, account); len(got) != 0 {
		t.Fatalf("a rolled-back decision left a notice: %+v", got)
	}
	// A failed notice rolls back the decision.
	boom := errors.New("inbox unavailable")
	s.Decided = func(context.Context, pgx.Tx, application.DecisionNotice) error { return boom }
	if err := admitWithJob(s, obs, realEnqueue); !errors.Is(err, boom) {
		t.Fatalf("notice failure = %v", err)
	}
	history, err := s.Decisions(context.Background(), obs.ID)
	if err != nil || len(history) != 1 {
		t.Fatalf("decision must roll back with its notice: %+v %v", history, err)
	}
	s.Decided = decisionNotices(pool)
	if err := admitWithJob(s, obs, realEnqueue); err != nil {
		t.Fatal(err)
	}
	if got := inbox(t, pool, account); len(got) != 1 {
		t.Fatalf("retry notices = %+v", got)
	}
}
