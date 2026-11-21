package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	db "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgbilling"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const stationAdmissionAction = "subs.station.admitted"

var (
	ErrStationApprovalUnavailable = errors.New("organization billing: reviewed station association unavailable")
	ErrStationAdmissionExists     = errors.New("organization billing: station is already admitted")
)

func StationPlanAuditDefinitions() []audit.Definition {
	return []audit.Definition{{Action: stationAdmissionAction, TargetType: "station"}}
}

type stationAdmissionGate interface {
	GuardWriteDBTX(context.Context, database.OrgDBTX, kernel.OrgScope, string) error
}

type StationAdmission struct {
	StationID      string
	ReviewRevision int64
	AdmittedAt     time.Time
	Replayed       bool
}

type StationMeter struct {
	Pool  *pgxpool.Pool
	Plans EntitlementStore
	Flags stationAdmissionGate
	Audit auditing.TransactionRunner
}

func (s StationMeter) Admit(ctx context.Context, scope kernel.OrgScope, approval, operator, operation string) (StationAdmission, error) {
	if s.Pool == nil || s.Flags == nil || s.Audit == nil {
		return StationAdmission{}, app.ErrUnavailable
	}
	if scope.Validate() != nil || !audit.ValidID(approval) || !audit.ValidID(operator) || !audit.ValidID(operation) {
		return StationAdmission{}, ErrStationApprovalUnavailable
	}
	var out StationAdmission
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		var err error
		out, err = s.admitTx(ctx, tx, scope, approval, operator, operation)
		return err
	})
	if err != nil {
		return StationAdmission{}, err
	}
	return out, nil
}

func (s StationMeter) admitTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, approval, operator, operation string) (StationAdmission, error) {
	org := scope.OrgID().String()
	q := db.New(tx)
	review, err := q.ReadStationPlanReview(ctx, db.ReadStationPlanReviewParams{OrgID: org, ApprovalID: approval, OperatorID: operator})
	if errors.Is(err, pgx.ErrNoRows) {
		return StationAdmission{}, ErrStationApprovalUnavailable
	}
	if err != nil {
		return StationAdmission{}, err
	}
	if err := s.Flags.GuardWriteDBTX(ctx, tx, scope, "organizations"); err != nil {
		if errors.Is(err, capability.ErrEntitlementRequired) {
			return StationAdmission{}, access.ErrEntitlementRequired
		}
		return StationAdmission{}, err
	}
	state, err := q.LockStationPlanOrganization(ctx, org)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && state != string(organization.Active) {
		return StationAdmission{}, ErrStationApprovalUnavailable
	}
	if err != nil {
		return StationAdmission{}, err
	}
	encoded, err := json.Marshal([]any{1, org, approval, review.StationID, review.Revision})
	if err != nil {
		return StationAdmission{}, app.ErrUnavailable
	}
	digest := sha256.Sum256(encoded)
	event := audit.Event{OperationID: operation, ActorID: operator, Action: stationAdmissionAction, TargetType: "station",
		TargetID: review.StationID, RequestHash: hex.EncodeToString(digest[:])}
	replay, err := s.Audit.RecordChangeTx(ctx, tx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
		return s.admitStation(ctx, tx, scope, review, operation)
	})
	if err != nil {
		return StationAdmission{}, err
	}
	stored, err := q.ReadStationPlanAdmission(ctx, db.ReadStationPlanAdmissionParams{OrgID: org, OperationID: operation})
	if err != nil || !stored.AdmittedAt.Valid || stored.StationID != review.StationID || stored.ReviewRevision != review.Revision {
		return StationAdmission{}, app.ErrUnavailable
	}
	return StationAdmission{StationID: stored.StationID, ReviewRevision: stored.ReviewRevision, AdmittedAt: stored.AdmittedAt.Time.UTC(), Replayed: replay}, nil
}

func (s StationMeter) admitStation(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, review db.ReadStationPlanReviewRow, operation string) error {
	q := db.New(tx)
	org := scope.OrgID().String()
	now, err := q.OrganizationBillingClock(ctx)
	if err != nil || !now.Valid {
		return app.ErrUnavailable
	}
	ok, err := q.CheckStationPlanReview(ctx, db.CheckStationPlanReviewParams{OrgID: org, ObservedAt: now, StationID: review.StationID, ReviewRevision: review.Revision})
	if err != nil {
		return err
	}
	if !ok {
		return ErrStationApprovalUnavailable
	}
	admitted, err := q.StationAlreadyAdmitted(ctx, db.StationAlreadyAdmittedParams{OrgID: org, StationID: review.StationID, ReviewRevision: review.Revision})
	if err != nil {
		return err
	}
	if admitted {
		return ErrStationAdmissionExists
	}
	used, err := q.CountAdmittedStations(ctx, db.CountAdmittedStationsParams{OrgID: org, ObservedAt: now})
	if err != nil {
		return err
	}
	_, plans, err := s.Plans.entitledPlans(ctx, tx, scope, "organizations", now.Time.UTC())
	if err != nil {
		return err
	}
	if err := checkPlanIncrease(plans, domain.ActiveStations, used, 1); err != nil {
		return err
	}
	saved, err := q.RecordStationPlanAdmission(ctx, db.RecordStationPlanAdmissionParams{
		OrgID: org, OperationID: operation, StationID: review.StationID, ReviewRevision: review.Revision,
	})
	if err != nil {
		return err
	}
	if saved != 1 {
		return ErrStationAdmissionExists
	}
	return nil
}
