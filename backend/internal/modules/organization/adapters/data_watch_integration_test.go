//go:build integration

package adapters_test

import (
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	accesspg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	datapg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/adapters/preparation"
	data "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/application"
	notifypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgnotify/adapters"
	notice "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgnotify/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func commitDataWithdrawalDuring[T any](t *testing.T, admin *pgxpool.Pool, priceDigest string, read func() T) T {
	t.Helper()
	ctx := t.Context()
	withdrawal, err := admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer withdrawal.Rollback(ctx)
	if _, err := withdrawal.Exec(ctx, `INSERT INTO enterprise_data_source_reviews
(source_code,source_digest,revision,status,decision_ref)
VALUES ('S-ESP-02',$1,2,'WITHDRAWN','synthetic-concurrent-withdrawal')`, priceDigest); err != nil {
		t.Fatal(err)
	}
	result := make(chan T, 1)
	go func() { result <- read() }()
	pid := int(withdrawal.Conn().PgConn().PID())
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	for {
		var waiting bool
		if err := admin.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity
 WHERE datname=current_database() AND wait_event_type='Lock' AND $1=ANY(pg_blocking_pids(pid))
 AND query LIKE '%name: LockDataSourceRights%')`, pid).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case <-result:
			t.Fatal("Data operation finished while the withdrawal was uncommitted")
		case <-deadline.C:
			t.Fatal("Data operation did not reach the source rights lock")
		case <-ticker.C:
		}
	}
	if err := withdrawal.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case value := <-result:
		return value
	case <-deadline.C:
		t.Fatal("Data operation did not settle after the withdrawal committed")
		var zero T
		return zero
	}
}

func TestScopedDataWatchNotifiesOnceAndStopsAtWithdrawnSource(t *testing.T) {
	env := dataUsageFixtureFor(t, 20, 7*24*time.Hour)
	ctx, fx := t.Context(), env.fx
	org := fx.scope.OrgID().String()
	var runtimeRole string
	if err := env.pool.QueryRow(ctx, "SELECT current_user").Scan(&runtimeRole); err != nil {
		t.Fatal(err)
	}
	role := pgx.Identifier{runtimeRole}.Sanitize()
	if os.Getenv("ANPFUEL_TEST_RUNTIME_GRANTS") != "script" {
		for _, grant := range []string{
			"GRANT SELECT ON enterprise_data_source_reviews,enterprise_regional_datasets,enterprise_data_plan_datasets,enterprise_regional_series,enterprise_regional_series_samples,org_notification_preferences TO ",
			"GRANT SELECT,INSERT ON job_queue,org_notification_recipients,org_notification_notices,org_notification_deliveries TO ",
			"GRANT SELECT,INSERT,UPDATE ON org_data_watchlists,org_notification_requests,org_notification_budgets TO ",
			"GRANT USAGE ON SEQUENCE org_notification_notices_position_seq TO ",
		} {
			if _, err := fx.f.Admin.Exec(ctx, grant+role); err != nil {
				t.Fatal(err)
			}
		}
	}
	const dataset = "watch-mt"
	prepare := func(month string, stations int, revision int64) string {
		t.Helper()
		path, manifest := buildRegionalExport(t, month, stations)
		registry, price := manifest.Files[0].SHA256, manifest.Sources.Official[0].SourceChecksum
		if _, err := fx.f.Admin.Exec(ctx, `INSERT INTO enterprise_data_source_reviews
(source_code,source_digest,revision,status,decision_ref,license_ref,attribution)
VALUES ('S-ESP-01',$1,1,'APPROVED','synthetic-watch-admission','Synthetic registry license','Synthetic registry'),
       ('S-ESP-02',$2,1,'APPROVED','synthetic-watch-admission','Synthetic survey license','Synthetic survey')`, registry, price); err != nil {
			t.Fatal(err)
		}
		if _, err := fx.f.Admin.Exec(ctx, `INSERT INTO enterprise_regional_datasets
(dataset_key,revision,display_name,state_code,minimum_stations,method_version,snapshot_digest,official_revision,
 registry_digest,registry_review,price_digest,price_review)
VALUES($1,$2,'Synthetic official watch','MT',5,'abastevo-metrics-v1',$3,$4,$5,1,$6,1)`,
			dataset, revision, manifest.ContentSHA256, manifest.Sources.Official[0].RevisionID, registry, price); err != nil {
			t.Fatal(err)
		}
		const tables = " ON enterprise_regional_series,enterprise_regional_series_samples "
		if _, err := fx.f.Admin.Exec(ctx, "GRANT INSERT"+tables+"TO "+role); err != nil {
			t.Fatal(err)
		}
		_, prepareErr := preparation.Prepare(ctx, env.pool, dataset, revision, path)
		if _, err := fx.f.Admin.Exec(ctx, "REVOKE INSERT"+tables+"FROM "+role); err != nil {
			t.Fatal(err)
		}
		if prepareErr != nil {
			t.Fatal(prepareErr)
		}
		if _, err := fx.f.Admin.Exec(ctx, `INSERT INTO enterprise_data_plan_datasets
(product_code,plan_code,plan_revision,dataset_key,dataset_revision,approval_ref)
VALUES ('data','synthetic',1,$1,$2,'synthetic-watch-plan')`, dataset, revision); err != nil {
			t.Fatal(err)
		}
		return price
	}
	prepare("01", 5, 1)
	catalog, err := audit.NewCatalog(datapg.WatchAuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	notices, err := notice.NewCatalog([]notice.Kind{datapg.WatchNoticeKind()})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	store := datapg.WatchStore{Pool: env.pool, Authority: env.auth, Plans: env.flags.PlanWrites.(datapg.DataPlanSource),
		Audit: auditpg.NewStore(env.pool, catalog), Now: func() time.Time { return now }, Enabled: true}
	worker := datapg.WatchWorker{Store: store, Flags: env.flags, NewID: jobs.NewUUIDv4,
		Notices: &notifypg.Store{Pool: env.pool, Authority: accesspg.NewStore(env.pool), Catalog: notices, NewID: jobs.NewUUIDv4}}
	input := data.WatchInput{OperationID: onboardingID(t), Dataset: dataset, Fuel: kernel.GasolineRegular, Revision: 1}
	w, replayed, err := store.CreateWatch(ctx, fx.scope, fx.actor, fx.member, input)
	if err != nil || replayed || !w.Active || w.LastObserved != 1 {
		t.Fatalf("watch creation did not pin its approved basis: %+v %v", w, err)
	}
	if again, repeated, err := store.CreateWatch(ctx, fx.scope, fx.actor, fx.member, input); err != nil || !repeated || again != w {
		t.Fatalf("watch retry did not converge: %+v %v", again, err)
	}
	wrong := input
	wrong.OperationID = onboardingID(t)
	if _, _, err := store.CreateWatch(ctx, fx.scope, fx.actor, fx.member, wrong); !errors.Is(err, datapg.ErrWatchConflict) {
		t.Fatalf("duplicate active watch returned %v", err)
	}
	foreign := fx.f.Organizations[1].Scope(t)
	if _, _, err := store.DisableWatch(ctx, foreign, fx.actor, fx.member, w.ID, onboardingID(t)); err == nil {
		t.Fatal("foreign organization disabled the watch")
	}
	observedPrice := prepare("02", 6, 2)
	nextJob := func() jobs.Job {
		t.Helper()
		var j jobs.Job
		var due int64
		if err := fx.f.Admin.QueryRow(ctx, `SELECT next_check FROM org_data_watchlists WHERE org_id=$1 AND id=$2`, org, w.ID).Scan(&due); err != nil {
			t.Fatal(err)
		}
		key := datapg.WatchJobKind + ":" + org + ":" + w.ID + ":" + strconv.FormatInt(due, 10)
		if err := fx.f.Admin.QueryRow(ctx, `SELECT id::text,kind,payload,dedupe_key FROM job_queue WHERE dedupe_key=$1`, key).
			Scan(&j.ID, &j.Kind, &j.Payload, &j.DedupeKey); err != nil {
			t.Fatal(err)
		}
		now = time.Unix(due, 0).UTC()
		return j
	}
	first := nextJob()
	if err := worker.Handle(ctx, first); err != nil {
		t.Fatalf("scheduled watch delivery failed: %v", err)
	}
	if err := worker.Handle(ctx, first); err != nil {
		t.Fatalf("watch delivery replay failed: %v", err)
	}
	count := func() {
		t.Helper()
		var sent, used int
		var exact bool
		if err := fx.f.Admin.QueryRow(ctx, `SELECT count(*),COALESCE(bool_and(r.params->>'source'='ANP' AND
  r.params->>'survey_end'='2026-02-01' AND r.params->>'change'='INCREASE'),false),
  (SELECT count(*) FROM org_data_usage_terms WHERE org_id=$1)
  FROM org_notification_notices n JOIN org_notification_requests r ON (r.org_id,r.id)=(n.org_id,n.request_id) WHERE n.org_id=$1`,
			org).Scan(&sent, &exact, &used); err != nil || sent != 1 || !exact || used != 0 {
			t.Fatalf("notice or operation meter diverged: sent=%d exact=%v used=%d err=%v", sent, exact, used, err)
		}
	}
	count()
	withdrawn := prepare("03", 7, 3)
	job := nextJob()
	if err := commitDataWithdrawalDuring(t, fx.f.Admin, withdrawn, func() error { return worker.Handle(ctx, job) }); err != nil {
		t.Fatalf("withdrawn source did not settle without publication: %v", err)
	}
	count()
	page, err := store.ListWatches(ctx, fx.scope, fx.actor, fx.member, "", 10)
	if err != nil || len(page.Items) != 1 || page.Items[0].LastObserved != 2 {
		t.Fatalf("withdrawn survey changed the stored watch: %+v %v", page, err)
	}
	// The last observed survey can lose admission (a withdrawal, a later rights
	// revision or a plan change). The watch then restarts from the next
	// admitted survey without a notice instead of waiting forever.
	if _, err := fx.f.Admin.Exec(ctx, `INSERT INTO enterprise_data_source_reviews
(source_code,source_digest,revision,status,decision_ref) VALUES ('S-ESP-02',$1,2,'WITHDRAWN','synthetic-observed-withdrawal')`,
		observedPrice); err != nil {
		t.Fatal(err)
	}
	prepare("04", 8, 4)
	if err := worker.Handle(ctx, nextJob()); err != nil {
		t.Fatalf("watch did not restart after its basis lost admission: %v", err)
	}
	count()
	if page, err := store.ListWatches(ctx, fx.scope, fx.actor, fx.member, "", 10); err != nil || len(page.Items) != 1 || page.Items[0].LastObserved != 4 {
		t.Fatalf("watch stayed on a basis that lost admission: %+v %v", page, err)
	}
	// A correction of an earlier week can receive a higher dataset revision;
	// the next comparison still uses the latest admitted survey.
	prepare("06", 9, 5)
	prepare("05", 10, 6)
	if err := worker.Handle(ctx, nextJob()); err != nil {
		t.Fatalf("watch with a backfilled revision failed: %v", err)
	}
	var latest string
	if err := fx.f.Admin.QueryRow(ctx, `SELECT r.params->>'survey_end' FROM org_notification_notices n
JOIN org_notification_requests r ON (r.org_id,r.id)=(n.org_id,n.request_id) WHERE n.org_id=$1 ORDER BY n.position DESC LIMIT 1`,
		org).Scan(&latest); err != nil || latest != "2026-06-01" {
		t.Fatalf("watch compared a backfilled survey instead of the latest one: %q %v", latest, err)
	}
	if page, err := store.ListWatches(ctx, fx.scope, fx.actor, fx.member, "", 10); err != nil || len(page.Items) != 1 || page.Items[0].LastObserved != 5 {
		t.Fatalf("watch did not keep the latest observed survey: %+v %v", page, err)
	}
	pending := nextJob()
	if row, _, err := store.DisableWatch(ctx, fx.scope, fx.actor, fx.member, w.ID, onboardingID(t)); err != nil || row.Active {
		t.Fatalf("cannot close watch: %+v %v", row, err)
	}
	if err := worker.Handle(ctx, pending); err != nil {
		t.Fatal("disabled watch was not a terminal no-op", err)
	}
	var delivered int
	if err := fx.f.Admin.QueryRow(ctx, `SELECT count(*) FROM org_notification_notices WHERE org_id=$1`, org).Scan(&delivered); err != nil || delivered != 2 {
		t.Fatalf("disabled watch changed its notices: %d %v", delivered, err)
	}
}
