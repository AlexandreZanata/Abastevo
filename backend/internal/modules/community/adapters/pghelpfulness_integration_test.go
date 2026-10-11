//go:build integration

package adapters

import (
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

var helpfulIDSeq atomic.Int64

func helpfulVoteID() string {
	return fmt.Sprintf("e6c74c23-63db-4c24-a2e5-408cb23d%04d", helpfulIDSeq.Add(1)%10000)
}

func helpfulPorts(hs *HelpfulnessStore) application.HelpfulnessPorts {
	return application.HelpfulnessPorts{
		Clock:        time.Now,
		NewID:        func() (string, error) { return helpfulVoteID(), nil },
		CheckAccount: func(_ context.Context, _ string) error { return nil },
		Store:        hs,
	}
}

func helpfulCaller(ref string) application.Caller {
	return application.Caller{ContributorID: ref, Fingerprint: "fp", KeyID: "k", Token: "tok-" + ref}
}

// helpfulAuthorCaller impersonates the key that authored the seeded
// observation (observations carry caller.Token as contributor_ref).
func helpfulAuthorCaller() application.Caller {
	return application.Caller{ContributorID: "author-acct", Fingerprint: "fp", KeyID: "k", Token: "ref-1"}
}

// validatedHelpfulTarget seeds one VALIDATED observation (latest decision
// sequence 2) and returns its ID.
func validatedHelpfulTarget(t *testing.T, s *Store, stationID, submission string) string {
	t.Helper()
	obs := validatedFixture(t, s, stationID, submission)
	return obs.ID
}

func liveHelpfulnessRows(t *testing.T, pool poolQueryer, obsID string, seq int64) (helpful, notHelpful, rows int64) {
	t.Helper()
	err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FILTER (WHERE choice='HELPFUL'), COUNT(*) FILTER (WHERE choice='NOT_HELPFUL'), COUNT(*)
		 FROM community_helpfulness_votes WHERE observation_id=$1 AND decision_sequence=$2 AND deleted_at IS NULL`,
		obsID, seq).Scan(&helpful, &notHelpful, &rows)
	if err != nil {
		t.Fatal(err)
	}
	return helpful, notHelpful, rows
}

type poolQueryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func TestHelpfulnessConcurrentCastsAgree(t *testing.T) {
	s, pool, stationID := freshStore(t)
	hs := NewHelpfulnessStore(pool)
	ctx := context.Background()
	obsID := validatedHelpfulTarget(t, s, stationID, "help-sub-1")

	const voters = 20
	var wg sync.WaitGroup
	errs := make(chan error, voters)
	for i := 0; i < voters; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ref := fmt.Sprintf("voter-%02d", i)
			_, err := application.Cast(ctx, helpfulPorts(hs), helpfulCaller(ref), application.CastDTO{
				ObservationID: obsID, DecisionSequence: 2,
				Choice: domain.ChoiceHelpful, ClientSubmissionID: fmt.Sprintf("sub-%02d", i),
			})
			if err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent cast: %v", err)
	}
	h, n, rows := liveHelpfulnessRows(t, pool, obsID, 2)
	if h != voters || n != 0 || rows != voters {
		t.Fatalf("live rows = %d/%d over %d rows, want %d/0 over %d", h, n, rows, voters, voters)
	}
	tally, found, err := hs.Tally(ctx, obsID, 2)
	if err != nil || !found {
		t.Fatalf("tally: %v found=%v", err, found)
	}
	if tally.Helpful != voters || tally.NotHelpful != 0 {
		t.Fatalf("stored tally = %+v, want %d/0", tally, voters)
	}
	rebuilt, err := hs.RebuildTally(ctx, obsID, 2)
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if rebuilt != tally {
		t.Fatalf("rebuilt %+v != stored %+v", rebuilt, tally)
	}
}

func TestHelpfulnessConcurrentFlipsKeepOneRow(t *testing.T) {
	s, pool, stationID := freshStore(t)
	hs := NewHelpfulnessStore(pool)
	ctx := context.Background()
	obsID := validatedHelpfulTarget(t, s, stationID, "help-sub-2")
	ports := helpfulPorts(hs)
	caller := helpfulCaller("voter-solo")
	dto := application.CastDTO{ObservationID: obsID, DecisionSequence: 2, Choice: domain.ChoiceHelpful, ClientSubmissionID: "sub-solo"}
	if _, err := application.Cast(ctx, ports, caller, dto); err != nil {
		t.Fatal(err)
	}
	const flips = 10
	var wg sync.WaitGroup
	errs := make(chan error, flips)
	for i := 0; i < flips; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			choice := domain.ChoiceNotHelpful
			if i%2 == 0 {
				choice = domain.ChoiceHelpful
			}
			_, err := application.Cast(ctx, ports, caller, application.CastDTO{
				ObservationID: obsID, DecisionSequence: 2,
				Choice: choice, ClientSubmissionID: fmt.Sprintf("flip-%d", i),
			})
			if err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent flip: %v", err)
	}
	h, n, rows := liveHelpfulnessRows(t, pool, obsID, 2)
	if rows != 1 || h+n != 1 {
		t.Fatalf("flips must keep one live row with exact totals, got %d/%d over %d rows", h, n, rows)
	}
	tally, _, err := hs.Tally(ctx, obsID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if tally.Helpful != h || tally.NotHelpful != n {
		t.Fatalf("stored tally %+v disagrees with live rows %d/%d", tally, h, n)
	}
}

func TestHelpfulnessConcurrentRemovalsIdempotent(t *testing.T) {
	s, pool, stationID := freshStore(t)
	hs := NewHelpfulnessStore(pool)
	ctx := context.Background()
	obsID := validatedHelpfulTarget(t, s, stationID, "help-sub-3")
	ports := helpfulPorts(hs)
	for i := 0; i < 5; i++ {
		ref := fmt.Sprintf("voter-rm-%d", i)
		if _, err := application.Cast(ctx, ports, helpfulCaller(ref), application.CastDTO{
			ObservationID: obsID, DecisionSequence: 2,
			Choice: domain.ChoiceHelpful, ClientSubmissionID: fmt.Sprintf("rm-%d", i),
		}); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ref := fmt.Sprintf("voter-rm-%d", i%5)
			if err := application.Remove(ctx, ports, helpfulCaller(ref), obsID, 2); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent remove: %v", err)
	}
	h, n, rows := liveHelpfulnessRows(t, pool, obsID, 2)
	if h != 0 || n != 0 || rows != 0 {
		t.Fatalf("removals must leave zero totals, got %d/%d over %d rows", h, n, rows)
	}
	if err := application.Remove(ctx, ports, helpfulCaller("voter-absent"), obsID, 2); err != nil {
		t.Fatalf("absent remove must converge as no-op success: %v", err)
	}
}

func TestHelpfulnessRetryAfterSuccessReplays(t *testing.T) {
	s, pool, stationID := freshStore(t)
	hs := NewHelpfulnessStore(pool)
	ctx := context.Background()
	obsID := validatedHelpfulTarget(t, s, stationID, "help-sub-4")
	ports := helpfulPorts(hs)
	caller := helpfulCaller("voter-retry")
	dto := application.CastDTO{ObservationID: obsID, DecisionSequence: 2, Choice: domain.ChoiceHelpful, ClientSubmissionID: "sub-retry"}
	first, err := application.Cast(ctx, ports, caller, dto)
	if err != nil {
		t.Fatal(err)
	}
	// Crash after commit: the client never saw the ack and retries with
	// the same submission identity.
	second, err := application.Cast(ctx, ports, caller, dto)
	if err != nil {
		t.Fatal(err)
	}
	if second.Created || !second.Replayed || second.VoteID != first.VoteID {
		t.Fatalf("retry must replay the committed vote: %+v", second)
	}
	if second.Tally.Helpful != 1 || second.Tally.NotHelpful != 0 {
		t.Fatalf("retry must not inflate totals: %+v", second.Tally)
	}
}

func TestHelpfulnessSubmissionScopedPerContributor(t *testing.T) {
	s, pool, stationID := freshStore(t)
	hs := NewHelpfulnessStore(pool)
	ctx := context.Background()
	obsID := validatedHelpfulTarget(t, s, stationID, "help-sub-5")
	ports := helpfulPorts(hs)
	// Voter B replays voter A's submission identity: it must open a
	// separate vote, never converge onto (or read) A's row.
	if _, err := application.Cast(ctx, ports, helpfulCaller("voter-a"), application.CastDTO{
		ObservationID: obsID, DecisionSequence: 2,
		Choice: domain.ChoiceHelpful, ClientSubmissionID: "sub-shared",
	}); err != nil {
		t.Fatal(err)
	}
	res, err := application.Cast(ctx, ports, helpfulCaller("voter-b"), application.CastDTO{
		ObservationID: obsID, DecisionSequence: 2,
		Choice: domain.ChoiceNotHelpful, ClientSubmissionID: "sub-shared",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Created || res.MyChoice != domain.ChoiceNotHelpful {
		t.Fatalf("foreign submission must not converge: %+v", res)
	}
	if res.Tally.Helpful != 1 || res.Tally.NotHelpful != 1 {
		t.Fatalf("tally must count both owners: %+v", res.Tally)
	}
}

func TestHelpfulnessRebuildRepairsCorruptTally(t *testing.T) {
	s, pool, stationID := freshStore(t)
	hs := NewHelpfulnessStore(pool)
	ctx := context.Background()
	obsID := validatedHelpfulTarget(t, s, stationID, "help-sub-6")
	ports := helpfulPorts(hs)
	for i := 0; i < 3; i++ {
		if _, err := application.Cast(ctx, ports, helpfulCaller(fmt.Sprintf("voter-c-%d", i)), application.CastDTO{
			ObservationID: obsID, DecisionSequence: 2,
			Choice: domain.ChoiceHelpful, ClientSubmissionID: fmt.Sprintf("c-%d", i),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE community_helpfulness_tallies SET helpful=999, not_helpful=999 WHERE observation_id=$1`, obsID); err != nil {
		t.Fatal(err)
	}
	rebuilt, err := hs.RebuildTally(ctx, obsID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt.Helpful != 3 || rebuilt.NotHelpful != 0 {
		t.Fatalf("rebuild must restore exact counts, got %+v", rebuilt)
	}
	stored, found, err := hs.Tally(ctx, obsID, 2)
	if err != nil || !found || stored != rebuilt {
		t.Fatalf("stored tally must equal rebuild: %+v found=%v err=%v", stored, found, err)
	}
}

func TestHelpfulnessTargetOwnershipAndEligibility(t *testing.T) {
	s, pool, stationID := freshStore(t)
	hs := NewHelpfulnessStore(pool)
	ctx := context.Background()
	obsID := validatedHelpfulTarget(t, s, stationID, "help-sub-7")

	if _, err := hs.Target(ctx, "00000000-0000-0000-0000-000000000000"); err == nil {
		t.Fatal("unknown target must fail")
	}
	got, err := hs.Target(ctx, obsID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AuthorRef != "ref-1" || !got.Eligible() || got.LatestSequence != 2 {
		t.Fatalf("target must resolve author, eligibility and sequence: %+v", got)
	}
	// A forged caller cannot become the author: the stored author stays
	// ref-1, so application self-vote logic keeps refusing the owner.
	if _, err := application.Cast(ctx, helpfulPorts(hs), helpfulAuthorCaller(), application.CastDTO{
		ObservationID: obsID, DecisionSequence: 2,
		Choice: domain.ChoiceHelpful, ClientSubmissionID: "sub-forge",
	}); err != domain.ErrSelfVote {
		t.Fatalf("owner vote must refuse with ErrSelfVote, got %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE community_observations SET hidden_at=now() WHERE id=$1`, obsID); err != nil {
		t.Fatal(err)
	}
	hidden, err := hs.Target(ctx, obsID)
	if err != nil {
		t.Fatal(err)
	}
	if hidden.Eligible() {
		t.Fatal("hidden target must resolve ineligible")
	}
}

func TestHelpfulnessEraseTombstonesAndAllowsRecast(t *testing.T) {
	s, pool, stationID := freshStore(t)
	hs := NewHelpfulnessStore(pool)
	ctx := context.Background()
	obsID := validatedHelpfulTarget(t, s, stationID, "help-sub-8")
	ports := helpfulPorts(hs)
	for i := 0; i < 2; i++ {
		if _, err := application.Cast(ctx, ports, helpfulCaller(fmt.Sprintf("voter-e-%d", i)), application.CastDTO{
			ObservationID: obsID, DecisionSequence: 2,
			Choice: domain.ChoiceHelpful, ClientSubmissionID: fmt.Sprintf("e-%d", i),
		}); err != nil {
			t.Fatal(err)
		}
	}
	n, err := hs.EraseContributorVotes(ctx, "tok-voter-e-0", time.Now())
	if err != nil || n != 1 {
		t.Fatalf("erase must tombstone one live vote, got %d (%v)", n, err)
	}
	rebuilt, err := hs.RebuildTally(ctx, obsID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt.Helpful != 1 {
		t.Fatalf("tally after erase must be 1, got %+v", rebuilt)
	}
	// Restore path: the erased contributor may vote again afterwards.
	res, err := application.Cast(ctx, ports, helpfulCaller("voter-e-0"), application.CastDTO{
		ObservationID: obsID, DecisionSequence: 2,
		Choice: domain.ChoiceNotHelpful, ClientSubmissionID: "e-recast",
	})
	if err != nil || !res.Created {
		t.Fatalf("recast after erase must insert: %+v (%v)", res, err)
	}
	if res.Tally.Helpful != 1 || res.Tally.NotHelpful != 1 {
		t.Fatalf("tally after recast must be 1/1: %+v", res.Tally)
	}
}

func freshPre054DB(t *testing.T, adminDSN string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("helpful_upgrade_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		admin, err := pgx.Connect(ctx, adminDSN)
		if err != nil {
			t.Errorf("admin connect for drop: %v", err)
			return
		}
		defer admin.Close(ctx)
		if _, err := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
			t.Errorf("drop database: %v", err)
		}
	})
	u, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	u.Path = "/" + name
	return u.String()
}

func TestHelpfulnessUpgradePreservesFacts(t *testing.T) {
	adminDSN := testDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	previous := fstest.MapFS{}
	files, err := fs.Glob(dbmigrations.Files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if name == "000054_community_helpfulness.sql" {
			continue
		}
		data, err := fs.ReadFile(dbmigrations.Files, name)
		if err != nil {
			t.Fatal(err)
		}
		previous[name] = &fstest.MapFile{Data: data}
	}
	dsn := freshPre054DB(t, adminDSN)
	if _, err := migrate.Apply(ctx, dsn, previous); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, `INSERT INTO directory_stations(id,display_name,state,municipality_code) VALUES ('00000000-0000-0000-0000-000000000002','Upgrade station','MT','5107925')`); err != nil {
		t.Fatal(err)
	}
	applied, err := migrate.Apply(ctx, dsn, dbmigrations.Files)
	if err != nil || len(applied) != 1 || applied[0] != "000054" {
		t.Fatalf("upgrade %v %v", applied, err)
	}
	var tables int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM pg_tables WHERE schemaname='public' AND tablename IN ('community_helpfulness_votes','community_helpfulness_tallies')`).Scan(&tables); err != nil || tables != 2 {
		t.Fatalf("tables %d %v", tables, err)
	}
	var cols int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_name='community_observations' AND column_name IN ('hidden_at','deleted_at')`).Scan(&cols); err != nil || cols != 2 {
		t.Fatalf("visibility columns %d %v", cols, err)
	}
	again, err := migrate.Apply(ctx, dsn, dbmigrations.Files)
	if err != nil || len(again) != 0 {
		t.Fatalf("replay %v %v", again, err)
	}
}

func TestHelpfulnessQueriesStayNonDestructive(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "db", "queries", "community", "helpfulness.sql"))
	if err != nil {
		t.Fatalf("read owned queries: %v", err)
	}
	upper := strings.ToUpper(string(raw))
	for _, verb := range []string{"DELETE FROM", "TRUNCATE", "DROP ", "ALTER "} {
		if strings.Contains(upper, verb) {
			t.Errorf("destructive statement present: %s", verb)
		}
	}
}
