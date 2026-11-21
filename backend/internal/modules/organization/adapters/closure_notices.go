package adapters

import (
	"context"
	"slices"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/organization"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	event "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/domain"
	notice "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgnotify/domain"
	"github.com/jackc/pgx/v5"
)

const closureNoticeKind = "org.lifecycle.closure_requested"

func ClosureNoticeKind() notice.Kind {
	return notice.Kind{Name: closureNoticeKind, Templates: []string{closureNoticeKind}, Permission: "org.lifecycle.manage", Mandatory: true,
		Params: map[string]notice.Param{"closes_on": {Type: notice.ParamDate}}}
}

type closureNotices interface {
	RequestTx(context.Context, pgx.Tx, kernel.OrgScope, event.Event, []notice.Request) ([]notice.Registered, error)
	Dispatch(context.Context, kernel.OrgScope, pgx.Tx, event.Event) error
}

func (s LifecycleStore) notifyClosureOwners(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, operation string, revision int64, closesAt time.Time) error {
	if s.Notices == nil {
		return app.ErrUnavailable
	}
	org := scope.OrgID().String()
	owners, err := q.New(tx).OrganizationClosureOwners(ctx, org)
	if err != nil {
		return err
	}
	source := event.Event{ID: operation, Name: closureNoticeKind, TargetType: lifecycleTarget, TargetID: org, Revision: revision}
	for recipients := range slices.Chunk(owners, notice.MaxRecipients) {
		if _, err := s.Notices.RequestTx(ctx, tx, scope, source, []notice.Request{{
			Key: "closure:" + operation + ":" + recipients[0], Kind: closureNoticeKind, Template: closureNoticeKind,
			Params: map[string]string{"closes_on": closesAt.UTC().Format(time.DateOnly)}, Recipients: recipients,
		}}); err != nil {
			return err
		}
	}
	return s.Notices.Dispatch(ctx, scope, tx, source)
}
