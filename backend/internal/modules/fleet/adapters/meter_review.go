package adapters

import (
	"context"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MeterReviewStore struct {
	Pool          *pgxpool.Pool
	Authority     app.MeterReviewAuthority
	Audit         auditing.TransactionRunner
	NewID         func() (string, error)
	ConfirmSource func(context.Context, pgx.Tx, syncapp.Source) (time.Time, error)
}

func MeterReviewAuditDefinitions() []audit.Definition {
	return []audit.Definition{{Action: "offl.review.resolved", TargetType: "offl.review"}}
}

func (s *MeterStore) HoldMeterReadingTx(ctx context.Context, db syncapp.DBTX, source syncapp.Source, operation protocol.Operation, command app.MeterReadingCommand, reason string) (app.ReviewReference, error) {
	if s == nil || s.Reviews == nil {
		return app.ReviewReference{}, app.ErrUnavailable
	}
	return s.Reviews.HoldMeterReadingTx(ctx, db, source, operation, command, reason)
}
func (s *MeterStore) CheckHeldTx(ctx context.Context, db syncapp.DBTX, source syncapp.Source, operation protocol.Operation, outcome syncapp.Outcome) error {
	if s == nil || s.Reviews == nil {
		return app.ErrUnavailable
	}
	return s.Reviews.CheckHeldTx(ctx, db, source, operation, outcome)
}
func (s *MeterStore) CheckOwnReceiptTx(ctx context.Context, db syncapp.DBTX, source syncapp.Source, operation protocol.Operation) (bool, error) {
	if s == nil || s.Reviews == nil {
		return false, app.ErrUnavailable
	}
	return s.Reviews.CheckOwnReceiptTx(ctx, db, source, operation)
}

var _ app.MeterHeldRepository = (*MeterReviewStore)(nil)
var _ syncapp.ReviewProof = (*MeterReviewStore)(nil)
var _ app.MeterReviewRepository = (*MeterReviewStore)(nil)
var _ app.MeterOwnReviewRepository = (*MeterReviewStore)(nil)
