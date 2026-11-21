package adapters

import (
	"context"
	"errors"

	db "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgdata"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	orgdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	billing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/application"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UsageStore struct {
	Pool      *pgxpool.Pool
	Plans     DataAllowanceSource
	Flags     capability.TransactionStateReader
	Authority app.UsageViewAuthority
	Enabled   bool
}

func (s UsageStore) ReadUsage(ctx context.Context, scope kernel.OrgScope, actor organization.CompanyActor, member string, after app.DatasetAfter, limit int) (app.UsageView, error) {
	if s.Pool == nil || s.Plans == nil || s.Flags == nil || s.Authority == nil || actor.Validate() != nil || !audit.ValidID(member) {
		return app.UsageView{}, app.ErrUnavailable
	}
	if limit < 1 || limit > 100 || (after != (app.DatasetAfter{}) && (!domain.ValidDatasetKey(after.Key) || after.Revision < 1)) {
		return app.UsageView{}, domain.ErrInvalidDataset
	}
	view := app.UsageView{Datasets: make([]app.DatasetView, 0, limit)}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckUsageReadTx(ctx, tx, scope, actor, member); err != nil {
			return err
		}
		q := db.New(tx)
		at, err := q.KeyClock(ctx)
		if err != nil || !at.Valid {
			return app.ErrUnavailable
		}
		view.ObservedAt = at.Time.UTC()
		if !s.Enabled {
			return nil
		}
		allowance, err := s.Plans.DataAllowanceAt(ctx, tx, scope, view.ObservedAt)
		if errors.Is(err, billing.ErrLimitUndefined) || errors.Is(err, access.ErrEntitlementRequired) {
			return nil
		}
		if err != nil {
			return err
		}
		return s.fill(ctx, tx, scope, allowance, after, limit, &view)
	})
	if err != nil {
		return app.UsageView{}, err
	}
	return view, nil
}

func (s UsageStore) fill(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, allowance billing.DataAllowance,
	after app.DatasetAfter, limit int, view *app.UsageView,
) error {
	q := db.New(tx)
	org := scope.OrgID().String()
	used, err := q.ReadTermUsage(ctx, db.ReadTermUsageParams{OrgID: org, TermStart: usageTimestamp(allowance.Term.Start), TermEnd: usageTimestamp(allowance.Term.End)})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	view.Term = &app.TermUsage{Start: allowance.Term.Start.UTC(), End: allowance.Term.End.UTC(), Used: used, Ceiling: allowance.Calls}
	flag, err := s.Flags.StateTx(ctx, tx, scope, "data")
	if err != nil {
		return err
	}
	lifecycle, err := q.LockDataOrganization(ctx, org)
	if err != nil {
		return err
	}
	view.Enabled = lifecycle == string(orgdomain.Active) && (flag.Revision == 0 || flag.Enabled)
	plan := allowance.Plan
	rate, err := q.ReadRatePolicy(ctx, db.ReadRatePolicyParams{ProductCode: plan.Product, PlanCode: plan.Plan, PlanRevision: plan.Revision})
	if err == nil {
		view.Rate = &app.RateView{WindowSeconds: rate.WindowSeconds, Calls: rate.WindowCalls}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	rows, err := q.ListAdmittedPlanDatasets(ctx, db.ListAdmittedPlanDatasetsParams{
		ProductCode: plan.Product, PlanCode: plan.Plan, PlanRevision: plan.Revision,
		AfterKey: after.Key, AfterRevision: after.Revision, RowLimit: int32(limit + 1),
	})
	if err != nil {
		return err
	}
	if len(rows) > limit {
		rows = rows[:limit]
		view.After = app.DatasetAfter{Key: rows[limit-1].DatasetKey, Revision: rows[limit-1].Revision}
	}
	for _, row := range rows {
		if _, err := (Catalog{DB: tx}).ReadAdmitted(ctx, row.DatasetKey, row.Revision); err != nil {
			if errors.Is(err, domain.ErrRightsPending) {
				continue
			}
			return err
		}
		view.Datasets = append(view.Datasets, app.DatasetView{Key: row.DatasetKey, Revision: row.Revision, Name: row.DisplayName,
			Region: row.StateCode, Method: row.MethodVersion, MinimumStations: row.MinimumStations})
	}
	return nil
}
