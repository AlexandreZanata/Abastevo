package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	db "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgdata"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	billing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/domain"
	export "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/application"
	file "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const MarketComparisonKind = "data.market_comparison.json"

type reportRequesters interface {
	CheckRequesterTx(context.Context, pgx.Tx, kernel.OrgScope, export.Requester, string, permissions.Permission) error
}

type MarketComparisonExport struct {
	Pool       *pgxpool.Pool
	Requesters reportRequesters
	Plans      DataPlanSource
	Flags      capability.TransactionStateReader
}

var _ export.Format = MarketComparisonExport{}
var _ export.FormatAdmission = MarketComparisonExport{}

func (MarketComparisonExport) Kind() string                           { return MarketComparisonKind }
func (MarketComparisonExport) ReadPermission() permissions.Permission { return "data.usage.read" }
func (MarketComparisonExport) MediaType() string                      { return "application/json" }
func (MarketComparisonExport) Limits() file.Limits {
	return file.Limits{Rows: 1, Bytes: 48 * 1024, Duration: 10 * time.Second}
}

type marketSelection struct {
	Dataset  string `json:"dataset"`
	Revision int64  `json:"revision"`
}

func (s marketSelection) valid() bool {
	return domain.ValidDatasetKey(s.Dataset) && s.Revision > 0
}

type marketParameters struct {
	Baseline  marketSelection `json:"baseline"`
	Reference marketSelection `json:"reference"`
	Fuel      kernel.Product  `json:"fuel"`
}

func parseMarketParameters(raw []byte) (marketParameters, error) {
	var p marketParameters
	if err := httpapi.DecodeJSON(raw, &p); err != nil || !p.Baseline.valid() || !p.Reference.valid() || p.Baseline == p.Reference {
		return marketParameters{}, file.ErrInvalid
	}
	if _, err := p.Fuel.Unit(); err != nil {
		return marketParameters{}, file.ErrInvalid
	}
	return p, nil
}

func (MarketComparisonExport) Canonical(raw []byte) ([]byte, error) {
	canonical, err := file.CanonicalParameters(raw)
	if err != nil {
		return nil, err
	}
	if _, err := parseMarketParameters(canonical); err != nil {
		return nil, err
	}
	return canonical, nil
}

type reportObservation struct {
	Dataset string `json:"dataset"`
	Region  string `json:"region"`
	Method  string `json:"method"`
	SeriesPeriod
}

type marketReport struct {
	SchemaVersion int               `json:"schema_version"`
	Fuel          kernel.Product    `json:"fuel"`
	Unit          kernel.Unit       `json:"unit"`
	Condition     string            `json:"condition"`
	Baseline      reportObservation `json:"baseline"`
	Reference     reportObservation `json:"reference"`
	domain.MarketDifference
}

func (f MarketComparisonExport) CheckInputsTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, raw []byte) error {
	_, err := f.read(ctx, tx, scope, raw)
	return err
}

func (f MarketComparisonExport) Produce(ctx context.Context, in export.ProduceInput, sink *file.Sink) error {
	if f.Pool == nil || f.Requesters == nil || sink == nil {
		return export.ErrUnavailable
	}
	if in.Scope.Validate() != nil || in.Requester.Validate() != nil || !file.ValidID(in.ExportID) || in.AsOf.IsZero() {
		return file.ErrInvalid
	}
	return database.WithOrgTx(ctx, f.Pool, in.Scope, func(tx pgx.Tx) error {
		if err := f.Requesters.CheckRequesterTx(ctx, tx, in.Scope, in.Requester, f.Kind(), f.ReadPermission()); err != nil {
			return err
		}
		result, err := f.read(ctx, tx, in.Scope, in.Parameters)
		if errors.Is(err, export.ErrNotFound) {
			return export.ErrRightsLost
		}
		if err != nil {
			return err
		}
		payload, err := json.Marshal(result)
		if err != nil {
			return export.ErrUnavailable
		}
		return sink.Row(append(payload, '\n'))
	})
}

func (f MarketComparisonExport) read(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, raw []byte) (marketReport, error) {
	if tx == nil || f.Plans == nil || f.Flags == nil {
		return marketReport{}, export.ErrUnavailable
	}
	params, err := parseMarketParameters(raw)
	if err != nil {
		return marketReport{}, err
	}
	plan, err := f.currentPlan(ctx, tx, scope)
	if err != nil {
		return marketReport{}, err
	}
	base, baseBasis, err := reportBasis(ctx, tx, plan, params.Baseline, params.Fuel)
	if err != nil {
		return marketReport{}, err
	}
	ref, referenceBasis, err := reportBasis(ctx, tx, plan, params.Reference, params.Fuel)
	if err != nil {
		return marketReport{}, err
	}
	unit, _ := params.Fuel.Unit()
	return marketReport{SchemaVersion: 1, Fuel: params.Fuel, Unit: unit, Condition: "STANDARD", Baseline: base, Reference: ref,
		MarketDifference: domain.CompareMarket(baseBasis, referenceBasis)}, nil
}

func (f MarketComparisonExport) currentPlan(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope) (billing.PlanKey, error) {
	var state string
	err := tx.QueryRow(ctx, `SELECT state FROM organizations WHERE org_id=$1::text::uuid`, scope.OrgID().String()).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && state != string(org.Active) {
		return billing.PlanKey{}, export.ErrNotFound
	}
	if err != nil {
		return billing.PlanKey{}, err
	}
	flag, err := f.Flags.StateTx(ctx, tx, scope, "data")
	if err != nil {
		return billing.PlanKey{}, err
	}
	if flag.Revision > 0 && !flag.Enabled {
		return billing.PlanKey{}, export.ErrNotFound
	}
	at, err := db.New(tx).KeyClock(ctx)
	if err != nil || !at.Valid {
		return billing.PlanKey{}, export.ErrUnavailable
	}
	plan, err := f.Plans.DataPlanAt(ctx, tx, scope, at.Time.UTC())
	if errors.Is(err, access.ErrEntitlementRequired) {
		return billing.PlanKey{}, export.ErrNotFound
	}
	return plan, err
}

func reportBasis(ctx context.Context, tx pgx.Tx, plan billing.PlanKey, selection marketSelection, fuel kernel.Product) (reportObservation, domain.MarketBasis, error) {
	admitted, err := readScopedDataset(ctx, tx, plan, selection.Dataset, selection.Revision)
	if errors.Is(err, ErrDatasetNotFound) || errors.Is(err, domain.ErrRightsPending) {
		return reportObservation{}, domain.MarketBasis{}, export.ErrNotFound
	}
	if err != nil {
		return reportObservation{}, domain.MarketBasis{}, err
	}
	period, err := readPeriod(ctx, tx, admitted)
	if err != nil {
		return reportObservation{}, domain.MarketBasis{}, err
	}
	basis := domain.MarketBasis{Region: admitted.Definition.MarketState, Method: admitted.Definition.Method,
		Start: period.SurveyStart, End: period.SurveyEnd, MinimumStations: period.MinimumStations}
	for _, sample := range period.Samples {
		if sample.Fuel == fuel {
			basis.Sample = sample
			break
		}
	}
	period.Samples = []domain.Sample{}
	period.Readiness = "INSUFFICIENT_DATA"
	if basis.Sample.Stations > 0 {
		period.Samples = append(period.Samples, basis.Sample)
		period.Readiness = "AVAILABLE"
	}
	return reportObservation{Dataset: selection.Dataset, Region: basis.Region, Method: basis.Method, SeriesPeriod: period}, basis, nil
}
