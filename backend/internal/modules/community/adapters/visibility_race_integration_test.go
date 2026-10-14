//go:build integration

package adapters

import (
	"context"
	"errors"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
	mod "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/adapters"
	modapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/application"
	md "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/domain"
	queue "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
	"testing"
	"time"
)

func TestVisibilityRaceRefusesNewSocialWrites(t *testing.T) {
	for _, operation := range []string{"confirm", "report", "helpfulness"} {
		t.Run(operation, func(t *testing.T) {
			s, pool, station := freshStore(t)
			ctx := context.Background()
			obs := validatedFixture(t, s, station, "visibility")
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if _, err := tx.Exec(ctx, `UPDATE community_observations SET hidden_at=now() WHERE id=$1`, obs.ID); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			started := make(chan struct{})
			jobs := 0
			enqueue := func(context.Context, pgx.Tx, string, []byte, string) error { jobs++; return nil }
			go func() {
				close(started)
				var err error
				switch operation {
				case "confirm":
					_, _, err = s.Confirm(ctx, domain.Confirmation{ID: helpfulVoteID(), ObservationID: obs.ID, ContributorRef: "other", ClientSubmissionID: "race", ReceivedAt: time.Now(), PolicyVersion: domain.PolicyV1}, enqueue)
				case "report":
					d, _, _ := domain.NewDispute(domain.DisputeParams{ID: helpfulVoteID(), TargetObservationID: obs.ID, ContributorRef: "other", ClientSubmissionID: "race", Reason: domain.ReasonWrongProduct, ReceivedAt: time.Now()})
					_, _, err = s.Report(ctx, d, enqueue)
				case "helpfulness":
					_, _, err = NewHelpfulnessStore(pool).CastVote(ctx, domain.StoredVote{ID: helpfulVoteID(), ObservationID: obs.ID, ContributorRef: "other", DecisionSequence: 2, ClientSubmissionID: "race", Choice: domain.ChoiceHelpful, CreatedAt: time.Now()})
				}
				done <- err
			}()
			<-started
			select {
			case err := <-done:
				t.Fatalf("write bypassed visibility lock: %v", err)
			case <-time.After(40 * time.Millisecond):
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if !errors.Is(err, application.ErrTargetNotFound) {
					t.Fatalf("hidden target accepted: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("write stuck")
			}
			if jobs != 0 {
				t.Fatalf("hidden write enqueued %d jobs", jobs)
			}
			var n int
			if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM community_confirmations)+(SELECT count(*) FROM community_disputes)+(SELECT count(*) FROM community_helpfulness_votes)`).Scan(&n); err != nil || n != 0 {
				t.Fatalf("writes=%d %v", n, err)
			}
		})
	}
}

func TestModerationInvalidationAuditDecisionJobAreAtomic(t *testing.T) {
	s, pool, station := freshStore(t)
	ctx := context.Background()
	obs := validatedFixture(t, s, station, "moderation-atomic")
	ms := mod.NewStore(pool)
	c, _, err := md.NewCase(md.CaseParams{ID: helpfulVoteID(), TargetType: md.TargetObservation, TargetID: obs.ID, Priority: md.PriorityP2, Reason: "wrong price", OpenedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ms.OpenCase(ctx, c); err != nil {
		t.Fatal(err)
	}
	fail := true
	ports := modapp.ActPorts{Clock: time.Now, NewID: func() (string, error) { return helpfulVoteID(), nil }, Store: ms, InvalidateObservation: s.InvalidateTx, EnqueueJob: func(context.Context, pgx.Tx, string, []byte, string) error {
		if fail {
			return errors.New("queue unavailable")
		}
		return nil
	}}
	dto := modapp.ActDTO{CaseID: c.ID, Action: md.ActionInvalidate, Reason: "reviewed wrong price"}
	if _, err := modapp.Act(ctx, ports, modapp.Caller{OperatorID: "synthetic-operator"}, dto); err == nil {
		t.Fatal("failed job accepted")
	}
	stored, err := ms.Get(ctx, c.ID)
	if err != nil || stored.Status != md.StatusOpen {
		t.Fatalf("failed action closed case: %+v %v", stored, err)
	}
	actions, err := ms.ListActions(ctx, c.ID)
	if err != nil || len(actions) != 0 {
		t.Fatalf("failed action committed audit: %+v %v", actions, err)
	}
	decisions, err := s.Decisions(ctx, obs.ID)
	if err != nil || len(decisions) != 2 {
		t.Fatalf("failed action committed decision: %+v %v", decisions, err)
	}
	fail = false
	if _, err := modapp.Act(ctx, ports, modapp.Caller{OperatorID: "synthetic-operator"}, dto); err != nil {
		t.Fatal(err)
	}
	decisions, err = s.Decisions(ctx, obs.ID)
	if err != nil || len(decisions) != 3 || decisions[2].ToState != domain.StateRejected {
		t.Fatalf("missing eligibility decision: %+v %v", decisions, err)
	}
	anchors, err := s.EligibleAnchors(ctx, domain.PriceKey{StationID: station, Product: obs.Product, Unit: obs.Unit, ConditionKind: obs.ConditionKind, Qualifier: obs.QualifierKey}, time.Now().Add(-domain.ConsensusWindow))
	if err != nil || len(anchors) != 0 {
		t.Fatalf("invalidated remains eligible: %+v %v", anchors, err)
	}
}

func TestReportAndInvalidationShareObservationFirstLockOrder(t *testing.T) {
	s, pool, station := freshStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	obs := validatedFixture(t, s, station, "lock-order")
	ms := mod.NewStore(pool)
	c, _, err := md.NewCase(md.CaseParams{ID: helpfulVoteID(), TargetType: md.TargetObservation, TargetID: obs.ID, Priority: md.PriorityP2, Reason: "wrong price", OpenedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ms.OpenCase(ctx, c); err != nil {
		t.Fatal(err)
	}
	held, release := make(chan struct{}), make(chan struct{})
	reported, acted := make(chan error, 1), make(chan error, 1)
	go func() {
		d, _, _ := domain.NewDispute(domain.DisputeParams{ID: helpfulVoteID(), TargetObservationID: obs.ID, ContributorRef: "other", ClientSubmissionID: "concurrent-report", Reason: domain.ReasonWrongProduct, ReceivedAt: time.Now()})
		_, _, err := s.Report(ctx, d, func(ctx context.Context, tx pgx.Tx, _ string, _ []byte, _ string) error {
			close(held)
			<-release
			again := c
			again.ID = helpfulVoteID()
			_, _, err := ms.OpenCaseTx(ctx, tx, again)
			return err
		})
		reported <- err
	}()
	<-held
	go func() {
		_, err := modapp.Act(ctx, modapp.ActPorts{Clock: time.Now, NewID: func() (string, error) { return helpfulVoteID(), nil }, Store: ms, InvalidateObservation: s.InvalidateTx, EnqueueJob: func(context.Context, pgx.Tx, string, []byte, string) error { return nil }}, modapp.Caller{OperatorID: "synthetic-operator"}, modapp.ActDTO{CaseID: c.ID, Action: md.ActionInvalidate, Reason: "reviewed wrong price"})
		acted <- err
	}()
	time.Sleep(50 * time.Millisecond)
	close(release)
	if err := <-reported; err != nil {
		t.Fatalf("report deadlock/failure: %v", err)
	}
	if err := <-acted; err != nil {
		t.Fatalf("invalidation deadlock/failure: %v", err)
	}
}

func TestInvalidatedProjectionStillAdvancesInternalVersion(t *testing.T) {
	s, pool, station := freshStore(t)
	ctx := context.Background()
	obs := validatedFixture(t, s, station, "projection-invalidate")
	key := projectionKey()
	key.StationID = station
	now := time.Now()
	result := priceResult()
	result.ComputedAt = now
	result.ExpiresAt = now.Add(time.Hour)
	if err := s.SaveProjection(ctx, key, result, obs.ID, now, now.Add(-domain.ConsensusWindow), nil, domain.ConsensusCommunity); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err := s.InvalidateTx(ctx, tx, obs.ID, helpfulVoteID(), now); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	next, err := application.RecomputeKey(ctx, application.RecomputePorts{Clock: func() time.Time { return now }, Store: s, Anchors: s.EligibleAnchors, Confirmations: s.ConfirmationsForAnchors, Tiers: func(context.Context, []string) (map[string]string, error) { return map[string]string{}, nil }, Photo: func(context.Context, domain.Observation) (application.PhotoInfo, error) {
		return application.PhotoInfo{}, nil
	}, Moderation: func(context.Context, domain.PriceKey) (bool, error) { return false, nil }}, key)
	if err != nil || next.Verdict != domain.VerdictUnknown {
		t.Fatalf("recompute after invalidate: %+v %v", next, err)
	}
	var version int64
	var availability string
	if err := pool.QueryRow(ctx, `SELECT projection_version,availability FROM community_current_prices WHERE station_id=$1`, station).Scan(&version, &availability); err != nil || version != 2 || availability != "UNKNOWN" {
		t.Fatalf("projection stalled: version=%d availability=%s %v", version, availability, err)
	}
}

func TestNewConfirmationAndReportDoNotReuseCompletedValidationJob(t *testing.T) {
	s, pool, station := freshStore(t)
	ctx := context.Background()
	obs := validatedFixture(t, s, station, "event-dedupe")
	if _, err := queue.Enqueue(ctx, pool, "community-consensus", consensusPayload(obs.ID), "consensus:"+obs.ID, 5, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE job_queue SET status='done' WHERE dedupe_key=$1`, "consensus:"+obs.ID); err != nil {
		t.Fatal(err)
	}
	enqueue := func(ctx context.Context, tx pgx.Tx, kind string, raw []byte, dedupe string) error {
		_, err := queue.Enqueue(ctx, tx, kind, raw, dedupe, 5, time.Time{})
		return err
	}
	vote := domain.Confirmation{ID: helpfulVoteID(), ObservationID: obs.ID, ContributorRef: "other", ClientSubmissionID: "new-confirm", ReceivedAt: time.Now(), PolicyVersion: domain.PolicyV1}
	for range 2 {
		if _, _, err := s.Confirm(ctx, vote, enqueue); err != nil {
			t.Fatal(err)
		}
	}
	report, _, _ := domain.NewDispute(domain.DisputeParams{ID: helpfulVoteID(), TargetObservationID: obs.ID, ContributorRef: "other", ClientSubmissionID: "new-report", Reason: domain.ReasonWrongProduct, ReceivedAt: time.Now()})
	for range 2 {
		if _, _, err := s.Report(ctx, report, enqueue); err != nil {
			t.Fatal(err)
		}
	}
	var pending int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM job_queue WHERE kind='community-consensus' AND status='queued'`).Scan(&pending); err != nil || pending != 2 {
		t.Fatalf("new event intents lost or duplicated: %d %v", pending, err)
	}
}
