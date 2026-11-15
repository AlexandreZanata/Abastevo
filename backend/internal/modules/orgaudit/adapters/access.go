package adapters

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	qorgaudit "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgaudit"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
)

// AccessLog records sensitive access through the audit runner: the generic
// event and its scoped detail commit together or not at all. The runner's
// catalog must register domain.AccessDefinitions.
type AccessLog struct{ Runner application.Runner }

var _ application.AccessRecorder = AccessLog{}

func (log AccessLog) RecordAccess(ctx context.Context, scope kernel.OrgScope, access domain.SensitiveAccess) error {
	event, err := access.Event(scope)
	if err != nil {
		return domain.ErrInvalid
	}
	if log.Runner == nil {
		return domain.ErrUnavailable
	}
	replayed, err := log.Runner.RecordChange(ctx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
		if err := qorgaudit.New(tx).InsertAccessDetail(ctx, qorgaudit.InsertAccessDetailParams{
			OrgID: scope.OrgID().String(), OperationID: access.RequestID, Category: string(access.Category),
			Stage: string(access.Stage), MembershipID: access.MembershipID, AccessID: access.AccessID,
		}); err != nil {
			return domain.ErrUnavailable
		}
		return nil
	})
	switch {
	case errors.Is(err, domain.ErrReused):
		return domain.ErrReused
	case errors.Is(err, domain.ErrInvalid):
		return domain.ErrInvalid
	case err != nil:
		return domain.ErrUnavailable
	case replayed:
		return domain.ErrReplayed
	}
	return nil
}
