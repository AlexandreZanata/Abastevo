package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	db "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgdata"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	orgdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/domain"
	event "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/domain"
	notice "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgnotify/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
)

const WatchJobKind = "org_data_watch"

const watchChangeNotice = "data.watch.changed"

func WatchNoticeKind() notice.Kind {
	return notice.Kind{
		Name: watchChangeNotice, Templates: []string{watchChangeNotice}, Permission: "data.usage.read",
		Params: map[string]notice.Param{
			"source":     {Type: notice.ParamCode, Codes: []string{"ANP"}},
			"survey_end": {Type: notice.ParamDate},
			"change":     {Type: notice.ParamCode, Codes: []string{string(domain.WatchIncrease), string(domain.WatchDecrease)}},
		},
	}
}

type watchJob struct {
	Version int    `json:"version"`
	Org     string `json:"organization_id"`
	Watch   string `json:"watch_id"`
	Due     int64  `json:"due"`
}

func (j watchJob) dedupe() string {
	return WatchJobKind + ":" + j.Org + ":" + j.Watch + ":" + strconv.FormatInt(j.Due, 10)
}

func scheduleWatch(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, id string, due int64) error {
	job := watchJob{Version: 1, Org: scope.OrgID().String(), Watch: id, Due: due}
	payload, err := json.Marshal(job)
	if err != nil {
		return app.ErrUnavailable
	}
	_, err = jobs.Enqueue(ctx, tx, WatchJobKind, payload, job.dedupe(), 5, time.Unix(due, 0).UTC())
	return err
}

type watchFlags interface {
	GuardWriteDBTX(context.Context, database.OrgDBTX, kernel.OrgScope, string) error
}

type watchNotices interface {
	RequestTx(context.Context, pgx.Tx, kernel.OrgScope, event.Event, []notice.Request) ([]notice.Registered, error)
	Dispatch(context.Context, kernel.OrgScope, pgx.Tx, event.Event) error
}

type WatchWorker struct {
	Store   WatchStore
	Flags   watchFlags
	Notices watchNotices
	NewID   func() (string, error)
}

func (WatchWorker) Kind() string { return WatchJobKind }
func (WatchWorker) Version() int { return 1 }

func (w WatchWorker) Handle(ctx context.Context, job jobs.Job) error {
	var input watchJob
	if job.Kind != WatchJobKind || httpapi.DecodeJSON(job.Payload, &input) != nil ||
		input.Version != 1 || input.Due <= 0 || !audit.ValidID(input.Org) || !audit.ValidID(input.Watch) || job.DedupeKey != input.dedupe() {
		return domain.ErrInvalidDataset
	}
	org, _ := kernel.ParseOrgID(input.Org)
	scope, err := kernel.NewOrgScope(org)
	if err != nil {
		return domain.ErrInvalidDataset
	}
	if !w.Store.Enabled || w.Store.Pool == nil || w.Store.Plans == nil || w.Store.Now == nil ||
		w.Store.Audit == nil || w.Flags == nil || w.Notices == nil || w.NewID == nil {
		return app.ErrUnavailable
	}
	if err := w.process(ctx, scope, input); err != nil {
		return app.ErrUnavailable
	}
	return nil
}

func (w WatchWorker) process(ctx context.Context, scope kernel.OrgScope, job watchJob) error {
	return database.WithOrgTx(ctx, w.Store.Pool, scope, func(tx pgx.Tx) error {
		flag := w.Flags.GuardWriteDBTX(ctx, tx, scope, "data")
		if flag != nil && !errors.Is(flag, capability.ErrEntitlementRequired) {
			return flag
		}
		q := db.New(tx)
		state, err := q.LockDataOrganization(ctx, job.Org)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		row, err := q.ReadWatch(ctx, db.ReadWatchParams{OrgID: job.Org, WatchID: job.Watch})
		if errors.Is(err, pgx.ErrNoRows) || err == nil && (!row.Active || row.NextCheck != job.Due) {
			return nil
		}
		if err != nil {
			return err
		}
		now := w.Store.Now().UTC()
		if now.Unix() < job.Due {
			return app.ErrUnavailable
		}
		active, err := q.WatchOwnerCurrent(ctx, db.WatchOwnerCurrentParams{OrgID: job.Org, Member: row.MemberID, Account: row.AccountID})
		if err != nil {
			return err
		}
		if !active || state == string(orgdomain.Archived) {
			return w.retire(ctx, tx, scope, row)
		}
		last := row.LastRevision
		if flag == nil && state == string(orgdomain.Active) {
			last, err = w.observe(ctx, tx, scope, row, now)
			if err != nil {
				return err
			}
		}
		due := now.Add(watchCadence).Unix()
		advanced, err := q.AdvanceWatch(ctx, db.AdvanceWatchParams{
			OrgID: job.Org, WatchID: job.Watch, ExpectedCheck: job.Due, LastRevision: last, NextCheck: due,
		})
		if err != nil || advanced != 1 {
			return app.ErrUnavailable
		}
		return scheduleWatch(ctx, tx, scope, row.ID, due)
	})
}

func (w WatchWorker) retire(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, row db.ReadWatchRow) error {
	id, err := w.NewID()
	if err != nil {
		return err
	}
	org := scope.OrgID().String()
	event := audit.Event{OperationID: id, ActorID: row.AccountID, TargetType: watchTarget, TargetID: row.ID, Action: watchDisable,
		RequestHash: watchDigest(org, row.ID, "retire")}
	_, err = w.Store.Audit.RecordChangeTx(ctx, tx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
		changed, err := db.New(tx).DisableWatch(ctx, db.DisableWatchParams{OrgID: org, WatchID: row.ID, Member: row.MemberID})
		if err != nil || changed != 1 {
			return app.ErrUnavailable
		}
		return nil
	})
	return err
}

func (w WatchWorker) observe(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, row db.ReadWatchRow, at time.Time) (int64, error) {
	plan, err := w.Store.Plans.DataPlanAt(ctx, tx, scope, at)
	if errors.Is(err, access.ErrEntitlementRequired) {
		return row.LastRevision, nil
	}
	if err != nil {
		return 0, err
	}
	next, err := db.New(tx).LatestWatchRevision(ctx, db.LatestWatchRevisionParams{
		ProductCode: plan.Product, PlanCode: plan.Plan, PlanRevision: plan.Revision, DatasetKey: row.DatasetKey, LastRevision: row.LastRevision,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return row.LastRevision, nil
	}
	if err != nil {
		return 0, err
	}
	catalog, err := OpenCatalog(ctx, tx)
	if err != nil {
		return 0, err
	}
	fuel := kernel.Product(row.FuelProduct)
	after, err := catalog.watchObservation(ctx, plan, row.DatasetKey, next, fuel)
	if watchNotAdmitted(err) {
		return row.LastRevision, nil
	}
	if err != nil {
		return 0, err
	}
	before, err := catalog.watchObservation(ctx, plan, row.DatasetKey, row.LastRevision, fuel)
	if watchNotAdmitted(err) {
		// The stored basis lost admission (withdrawal, a later rights revision or
		// another plan). Restart from the admitted survey without a notice;
		// nothing from the unadmitted revision is compared or published.
		return next, nil
	}
	if err != nil {
		return 0, err
	}
	move := domain.CompareWatchObservations(before, after)
	if move == domain.WatchUnavailable {
		return row.LastRevision, nil
	}
	if move != domain.WatchUnchanged {
		if err := w.publish(ctx, tx, scope, row, after, move); err != nil {
			return 0, err
		}
	}
	return next, nil
}

func watchNotAdmitted(err error) bool {
	return errors.Is(err, ErrDatasetNotFound) || errors.Is(err, domain.ErrRightsPending) || errors.Is(err, ErrSeriesUnavailable)
}

func (w WatchWorker) publish(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, row db.ReadWatchRow,
	after domain.WatchObservation, move domain.WatchMove,
) error {
	id, err := w.NewID()
	if err != nil {
		return err
	}
	source := event.Event{ID: id, Name: watchChangeNotice, TargetType: watchTarget, TargetID: row.ID, Revision: after.Revision}
	bound := audit.Event{OperationID: id, ActorID: row.AccountID, TargetType: watchTarget, TargetID: row.ID, Action: watchObserve,
		RequestHash: watchDigest(scope.OrgID().String(), row.ID, row.LastRevision, after.Revision, after.SurveyEnd, move)}
	_, err = w.Store.Audit.RecordChangeTx(ctx, tx, scope, bound, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := w.Notices.RequestTx(ctx, tx, scope, source, []notice.Request{{
			Key: "watch:" + row.ID + ":" + strconv.FormatInt(after.Revision, 10), Kind: watchChangeNotice, Template: watchChangeNotice,
			Params: map[string]string{"source": "ANP", "survey_end": after.SurveyEnd, "change": string(move)}, Recipients: []string{row.MemberID},
		}}); err != nil {
			return err
		}
		return w.Notices.Dispatch(ctx, scope, tx, source)
	})
	return err
}

var _ jobs.Handler = WatchWorker{}
