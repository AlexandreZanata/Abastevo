//go:build integration

package adapters

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/domain"
)

type finishOutcome struct {
	result FinishResult
	err    error
}

func installPublishDelay(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
CREATE OR REPLACE FUNCTION esp_test_publish_delay() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
	IF NEW.status = 'published' AND OLD.status = 'staging' THEN
		PERFORM pg_sleep(0.15);
	END IF;
	RETURN NEW;
END $$;
CREATE TRIGGER esp_test_publish_delay BEFORE UPDATE ON official_revisions
FOR EACH ROW EXECUTE FUNCTION esp_test_publish_delay()`)
	if err != nil {
		t.Fatal(err)
	}
}

func concurrentFinish(t *testing.T, im *Importer, begins []BeginResult) []finishOutcome {
	t.Helper()
	start := make(chan struct{})
	results := make(chan finishOutcome, len(begins))
	for _, begin := range begins {
		go func(begin BeginResult) {
			<-start
			var tally domain.QuarantineTally
			res, err := im.FinishRun(context.Background(), begin.RunID, begin.RevisionID, &tally)
			results <- finishOutcome{result: res, err: err}
		}(begin)
	}
	close(start)
	out := make([]finishOutcome, 0, len(begins))
	for range begins {
		out = append(out, <-results)
	}
	return out
}

func beginAndStage(t *testing.T, im *Importer, checksum string, station string, sourceRow int, amount int64) BeginResult {
	t.Helper()
	begin, err := im.BeginRun(context.Background(), domain.ImportKey{
		SourceURL: "https://example.test/review.xlsx", SourceChecksum: checksum, ParserVersion: "review-guard",
	}, weekStart, weekEnd)
	if err != nil || begin.NoOp {
		t.Fatalf("begin %s = %+v, %v", checksum, begin, err)
	}
	var tally domain.QuarantineTally
	if _, err := im.StageBatch(context.Background(), begin.RevisionID, []domain.PriceRow{priceRow(station, sourceRow, amount)}, &tally); err != nil {
		t.Fatalf("stage %s: %v", checksum, err)
	}
	return begin
}

func revisionStatus(t *testing.T, pool *pgxpool.Pool, revisionID string) string {
	t.Helper()
	var status string
	if err := pool.QueryRow(context.Background(), "SELECT status FROM official_revisions WHERE id=$1", mustUUIDOrFail(t, revisionID)).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func TestConcurrentFinishesFirstPublicationPublishOnce(t *testing.T) {
	im, pool := freshImporter(t)
	station := makeStation(t, pool, "Review station")
	installPublishDelay(t, pool)
	begins := []BeginResult{
		beginAndStage(t, im, "sha256:first-a", station, 8, 5999),
		beginAndStage(t, im, "sha256:first-b", station, 9, 6099),
	}
	outcomes := concurrentFinish(t, im, begins)
	published := 0
	for _, outcome := range outcomes {
		if outcome.err != nil {
			t.Fatalf("concurrent finish: %v", outcome.err)
		}
		if outcome.result.Published {
			published++
		}
	}
	if published != 1 {
		t.Fatalf("published finishes = %d, want exactly one", published)
	}
	current, err := im.CurrentRevision(context.Background(), weekStart, weekEnd)
	if err != nil {
		t.Fatal(err)
	}
	var publishedRows, superseded int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM official_revisions WHERE status='published'").Scan(&publishedRows); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM official_revisions WHERE status='superseded'").Scan(&superseded); err != nil {
		t.Fatal(err)
	}
	if publishedRows != 1 || superseded != 1 {
		t.Fatalf("revision states published=%d superseded=%d", publishedRows, superseded)
	}
	if status := revisionStatus(t, pool, current); status != domain.RevisionPublished {
		t.Fatalf("pointer revision status = %q", status)
	}
}

func TestConcurrentCorrectionsFromSamePredecessorPublishOnce(t *testing.T) {
	im, pool := freshImporter(t)
	station := makeStation(t, pool, "Review station")
	base := beginAndStage(t, im, "sha256:base", station, 8, 5899)
	var baseTally domain.QuarantineTally
	if res, err := im.FinishRun(context.Background(), base.RunID, base.RevisionID, &baseTally); err != nil || !res.Published {
		t.Fatalf("base publish = %+v, %v", res, err)
	}
	installPublishDelay(t, pool)
	begins := []BeginResult{
		beginAndStage(t, im, "sha256:correction-a", station, 8, 5999),
		beginAndStage(t, im, "sha256:correction-b", station, 8, 6099),
	}
	outcomes := concurrentFinish(t, im, begins)
	published := 0
	var winner BeginResult
	for _, outcome := range outcomes {
		if outcome.err != nil {
			t.Fatalf("concurrent correction finish: %v", outcome.err)
		}
		if outcome.result.Published {
			published++
			for _, begin := range begins {
				if begin.RevisionID == outcome.result.RevisionID {
					winner = begin
				}
			}
		}
	}
	if published != 1 {
		t.Fatalf("published corrections = %d, want exactly one", published)
	}
	current, err := im.CurrentRevision(context.Background(), weekStart, weekEnd)
	if err != nil || current != winner.RevisionID {
		t.Fatalf("pointer = %q, %v; want winner %q", current, err, winner.RevisionID)
	}
	loser := begins[0]
	if loser.RevisionID == winner.RevisionID {
		loser = begins[1]
	}
	if status := revisionStatus(t, pool, loser.RevisionID); status != domain.RevisionSuperseded {
		t.Fatalf("losing correction status = %q", status)
	}
	if status := revisionStatus(t, pool, base.RevisionID); status != domain.RevisionPublished {
		t.Fatalf("predecessor status changed to %q", status)
	}
	var amount int64
	if err := pool.QueryRow(context.Background(), "SELECT amount_milli_brl FROM official_station_prices WHERE revision_id=$1", mustUUIDOrFail(t, winner.RevisionID)).Scan(&amount); err != nil {
		t.Fatal(err)
	}
	if amount != 5999 && amount != 6099 {
		t.Fatalf("winner facts = %d", amount)
	}
}

func TestConcurrentSameRevisionFinishIsIdempotent(t *testing.T) {
	im, pool := freshImporter(t)
	station := makeStation(t, pool, "Review station")
	installPublishDelay(t, pool)
	begin := beginAndStage(t, im, "sha256:same", station, 8, 5999)
	outcomes := concurrentFinish(t, im, []BeginResult{begin, begin})
	for _, outcome := range outcomes {
		if outcome.err != nil {
			t.Fatalf("duplicate finish: %v", outcome.err)
		}
		if !outcome.result.Published {
			t.Fatalf("duplicate finish did not converge on the published revision: %+v", outcome.result)
		}
	}
	current, err := im.CurrentRevision(context.Background(), weekStart, weekEnd)
	if err != nil || current != begin.RevisionID {
		t.Fatalf("pointer = %q, %v", current, err)
	}
	var rows int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM official_station_prices WHERE revision_id=$1", mustUUIDOrFail(t, begin.RevisionID)).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("published rows = %d, want one", rows)
	}
}

func TestPublishedRevisionRejectsLateStage(t *testing.T) {
	im, pool := freshImporter(t)
	first := makeStation(t, pool, "First")
	second := makeStation(t, pool, "Second")
	begin := beginAndStage(t, im, "sha256:frozen", first, 8, 5999)
	var tally domain.QuarantineTally
	if res, err := im.FinishRun(context.Background(), begin.RunID, begin.RevisionID, &tally); err != nil || !res.Published {
		t.Fatalf("publish = %+v, %v", res, err)
	}
	_, stageErr := im.StageBatch(context.Background(), begin.RevisionID, []domain.PriceRow{priceRow(second, 9, 6999)}, &tally)
	if !errors.Is(stageErr, domain.ErrRevisionFrozen) {
		t.Fatalf("late stage error = %v, want ErrRevisionFrozen", stageErr)
	}
	_, restageErr := im.StageBatch(context.Background(), begin.RevisionID, []domain.PriceRow{priceRow(first, 8, 5999)}, &tally)
	if !errors.Is(restageErr, domain.ErrRevisionFrozen) {
		t.Fatalf("identical late restage error = %v, want ErrRevisionFrozen", restageErr)
	}
	var rows int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM official_station_prices WHERE revision_id=$1", mustUUIDOrFail(t, begin.RevisionID)).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("published revision rows = %d, want one", rows)
	}
}

func TestTerminalRevisionsRejectStage(t *testing.T) {
	im, pool := freshImporter(t)
	station := makeStation(t, pool, "Review station")
	setup := []struct {
		name string
		make func() BeginResult
	}{
		{name: "published", make: func() BeginResult {
			begin := beginAndStage(t, im, "sha256:terminal-published", station, 8, 5999)
			var tally domain.QuarantineTally
			if res, err := im.FinishRun(context.Background(), begin.RunID, begin.RevisionID, &tally); err != nil || !res.Published {
				t.Fatalf("publish: %+v %v", res, err)
			}
			return begin
		}},
		{name: "failed", make: func() BeginResult {
			begin, err := im.BeginRun(context.Background(), domain.ImportKey{SourceURL: "https://example.test/review.xlsx", SourceChecksum: "sha256:terminal-failed", ParserVersion: "review-guard"}, weekStart, weekEnd)
			if err != nil {
				t.Fatal(err)
			}
			var tally domain.QuarantineTally
			if res, err := im.FinishRun(context.Background(), begin.RunID, begin.RevisionID, &tally); err != nil || res.Published {
				t.Fatalf("failed finish: %+v %v", res, err)
			}
			return begin
		}},
		{name: "needs-review", make: func() BeginResult {
			begin := beginAndStage(t, im, "sha256:terminal-review", station, 40, 5999)
			var tally domain.QuarantineTally
			tally.Add("unknown-fuel-label", "synthetic")
			if res, err := im.FinishRun(context.Background(), begin.RunID, begin.RevisionID, &tally); err != nil || res.Published {
				t.Fatalf("needs-review finish: %+v %v", res, err)
			}
			return begin
		}},
		{name: "superseded", make: func() BeginResult {
			begin := beginAndStage(t, im, "sha256:terminal-superseded", station, 60, 5999)
			if _, err := pool.Exec(context.Background(), "UPDATE official_revisions SET status='superseded' WHERE id=$1", mustUUIDOrFail(t, begin.RevisionID)); err != nil {
				t.Fatal(err)
			}
			return begin
		}},
	}
	for _, tc := range setup {
		t.Run(tc.name, func(t *testing.T) {
			begin := tc.make()
			var before int
			if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM official_station_prices WHERE revision_id=$1", mustUUIDOrFail(t, begin.RevisionID)).Scan(&before); err != nil {
				t.Fatal(err)
			}
			var tally domain.QuarantineTally
			_, err := im.StageBatch(context.Background(), begin.RevisionID, []domain.PriceRow{priceRow(station, 99, 6999)}, &tally)
			if !errors.Is(err, domain.ErrRevisionFrozen) {
				t.Fatalf("stage on %s revision error = %v", tc.name, err)
			}
			var after int
			if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM official_station_prices WHERE revision_id=$1", mustUUIDOrFail(t, begin.RevisionID)).Scan(&after); err != nil {
				t.Fatal(err)
			}
			if before != after {
				t.Fatalf("rows changed after terminal stage: %d -> %d", before, after)
			}
		})
	}
}

func TestIdenticalRestageIsNoOpAndDivergentBatchRejected(t *testing.T) {
	im, pool := freshImporter(t)
	station := makeStation(t, pool, "Review station")
	begin, err := im.BeginRun(context.Background(), domain.ImportKey{SourceURL: "https://example.test/review.xlsx", SourceChecksum: "sha256:restage", ParserVersion: "review-guard"}, weekStart, weekEnd)
	if err != nil {
		t.Fatal(err)
	}
	row := priceRow(station, 8, 5999)
	var tally domain.QuarantineTally
	if staged, err := im.StageBatch(context.Background(), begin.RevisionID, []domain.PriceRow{row}, &tally); err != nil || staged != 1 {
		t.Fatalf("first stage = %d, %v", staged, err)
	}
	if staged, err := im.StageBatch(context.Background(), begin.RevisionID, []domain.PriceRow{row}, &tally); err != nil || staged != 1 {
		t.Fatalf("identical restage = %d, %v", staged, err)
	}
	divergent := row
	divergent.AmountMilli = 6099
	divergent.RawText = "6,099"
	if _, err := im.StageBatch(context.Background(), begin.RevisionID, []domain.PriceRow{divergent}, &tally); !errors.Is(err, domain.ErrDivergentRestage) {
		t.Fatalf("divergent restage error = %v", err)
	}
	var rows int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM official_station_prices WHERE revision_id=$1", mustUUIDOrFail(t, begin.RevisionID)).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("rows after divergent restage = %d", rows)
	}
	var amount int64
	if err := pool.QueryRow(context.Background(), "SELECT amount_milli_brl FROM official_station_prices WHERE revision_id=$1", mustUUIDOrFail(t, begin.RevisionID)).Scan(&amount); err != nil {
		t.Fatal(err)
	}
	if amount != 5999 {
		t.Fatalf("divergent restage changed facts to %d", amount)
	}
}

func TestStageConcurrentWithFinishRespectsTerminalState(t *testing.T) {
	im, pool := freshImporter(t)
	station := makeStation(t, pool, "Review station")
	installPublishDelay(t, pool)
	for i := 0; i < 4; i++ {
		start := weekStart.AddDate(0, 0, 7*i)
		end := weekEnd.AddDate(0, 0, 7*i)
		key := domain.ImportKey{SourceURL: "https://example.test/race.xlsx", SourceChecksum: "sha256:race-" + string(rune('a'+i)), ParserVersion: "review-guard"}
		begin, err := im.BeginRun(context.Background(), key, start, end)
		if err != nil {
			t.Fatal(err)
		}
		var tally domain.QuarantineTally
		initial := []domain.PriceRow{priceRow(station, 8, 5999), priceRow(station, 9, 5999), priceRow(station, 10, 5999)}
		if _, err := im.StageBatch(context.Background(), begin.RevisionID, initial, &tally); err != nil {
			t.Fatal(err)
		}
		startCh := make(chan struct{})
		finishCh := make(chan finishOutcome, 1)
		stageCh := make(chan error, 1)
		go func() {
			<-startCh
			var q domain.QuarantineTally
			res, err := im.FinishRun(context.Background(), begin.RunID, begin.RevisionID, &q)
			finishCh <- finishOutcome{result: res, err: err}
		}()
		go func() {
			<-startCh
			var q domain.QuarantineTally
			_, err := im.StageBatch(context.Background(), begin.RevisionID, []domain.PriceRow{priceRow(station, 11, 5999)}, &q)
			stageCh <- err
		}()
		close(startCh)
		finish := <-finishCh
		stageErr := <-stageCh
		if finish.err != nil {
			t.Fatalf("iteration %d finish: %v", i, finish.err)
		}
		if !finish.result.Published {
			t.Fatalf("iteration %d revision did not publish", i)
		}
		var rows int
		if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM official_station_prices WHERE revision_id=$1", mustUUIDOrFail(t, begin.RevisionID)).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		if stageErr == nil {
			if rows != 4 {
				t.Fatalf("iteration %d stage committed before finish but rows=%d", i, rows)
			}
		} else if !errors.Is(stageErr, domain.ErrRevisionFrozen) {
			t.Fatalf("iteration %d late stage error = %v", i, stageErr)
		} else if rows != 3 {
			t.Fatalf("iteration %d late stage changed rows to %d", i, rows)
		}
		status := revisionStatus(t, pool, begin.RevisionID)
		if status != domain.RevisionPublished {
			t.Fatalf("iteration %d pointer revision status = %q", i, status)
		}
	}
}

func TestFinishTransactionRollbackLeavesStaging(t *testing.T) {
	im, pool := freshImporter(t)
	station := makeStation(t, pool, "Review station")
	begin := beginAndStage(t, im, "sha256:rollback", station, 8, 5999)
	if _, err := pool.Exec(context.Background(), `
CREATE FUNCTION esp_test_pointer_fail() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
	RAISE EXCEPTION 'synthetic pointer failure';
END $$;
CREATE TRIGGER esp_test_pointer_fail BEFORE INSERT OR UPDATE ON official_current_revisions
FOR EACH ROW EXECUTE FUNCTION esp_test_pointer_fail()`); err != nil {
		t.Fatal(err)
	}
	var tally domain.QuarantineTally
	if _, err := im.FinishRun(context.Background(), begin.RunID, begin.RevisionID, &tally); err == nil || !strings.Contains(err.Error(), "synthetic pointer failure") {
		t.Fatalf("pointer failure not propagated: %v", err)
	}
	if status := revisionStatus(t, pool, begin.RevisionID); status != domain.RevisionStaging {
		t.Fatalf("revision after rollback = %q, want staging", status)
	}
	var runStatus string
	if err := pool.QueryRow(context.Background(), "SELECT status FROM official_import_runs WHERE id=$1", mustUUIDOrFail(t, begin.RunID)).Scan(&runStatus); err != nil {
		t.Fatal(err)
	}
	if runStatus != domain.RunRunning {
		t.Fatalf("run after rollback = %q", runStatus)
	}
	var pointers int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM official_current_revisions").Scan(&pointers); err != nil {
		t.Fatal(err)
	}
	if pointers != 0 {
		t.Fatalf("rollback left %d pointers", pointers)
	}
	if _, err := pool.Exec(context.Background(), "DROP TRIGGER esp_test_pointer_fail ON official_current_revisions"); err != nil {
		t.Fatal(err)
	}
	if res, err := im.FinishRun(context.Background(), begin.RunID, begin.RevisionID, &tally); err != nil || !res.Published {
		t.Fatalf("retry after rollback = %+v, %v", res, err)
	}
}

func TestBeginResumeUsesPersistedSurveyWindow(t *testing.T) {
	im, _ := freshImporter(t)
	key := domain.ImportKey{SourceURL: "https://example.test/window.xlsx", SourceChecksum: "sha256:window", ParserVersion: "review-guard"}
	first, err := im.BeginRun(context.Background(), key, weekStart, weekEnd)
	if err != nil {
		t.Fatal(err)
	}
	nextStart := weekStart.AddDate(0, 0, 7)
	nextEnd := weekEnd.AddDate(0, 0, 7)
	if _, err := im.BeginRun(context.Background(), key, nextStart, nextEnd); err == nil {
		t.Fatal("resume accepted a different survey window")
	}
	resumed, err := im.BeginRun(context.Background(), key, weekStart, weekEnd)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.RevisionID != first.RevisionID || !resumed.SurveyStart.Equal(first.SurveyStart) || !resumed.SurveyEnd.Equal(first.SurveyEnd) {
		t.Fatalf("resume diverged: first=%+v resumed=%+v", first, resumed)
	}
}

func TestFinishRejectsRevisionRunMismatch(t *testing.T) {
	im, pool := freshImporter(t)
	station := makeStation(t, pool, "Review station")
	first := beginAndStage(t, im, "sha256:link-a", station, 8, 5999)
	second := beginAndStage(t, im, "sha256:link-b", station, 9, 6099)
	var tally domain.QuarantineTally
	if _, err := im.FinishRun(context.Background(), second.RunID, first.RevisionID, &tally); err == nil {
		t.Fatal("finish accepted a revision from another run")
	}
	if status := revisionStatus(t, pool, first.RevisionID); status != domain.RevisionStaging {
		t.Fatalf("mismatched finish changed status to %q", status)
	}
	if _, err := im.FinishRun(context.Background(), first.RunID, first.RevisionID, &tally); err != nil {
		t.Fatalf("valid finish after mismatch: %v", err)
	}
}

func TestOfficialSupersededStatusMigrationApplied(t *testing.T) {
	_, pool := freshImporter(t)
	var definition string
	if err := pool.QueryRow(context.Background(), "SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname='official_revisions_status_check'").Scan(&definition); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(definition, "superseded") {
		t.Fatalf("official revision status constraint lacks superseded: %s", definition)
	}
}
