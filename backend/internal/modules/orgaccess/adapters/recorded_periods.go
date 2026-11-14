package adapters

import (
	"context"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgaccess"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	"github.com/jackc/pgx/v5/pgtype"
)

var _ app.RecordedRolePeriods = (*Store)(nil)

func (s *Store) RecordedPeriodsDBTX(ctx context.Context, db app.AuthorityQueries, scope kernel.OrgScope, member string) ([]app.RecordedRolePeriod, error) {
	if s == nil || db == nil {
		return nil, app.ErrUnavailable
	}
	if err := validateID(scope, member); err != nil {
		return nil, err
	}
	if err := check(ctx, db, scope); err != nil {
		return nil, err
	}
	rows, err := q.New(db).RecordedAssignmentPeriods(ctx, q.RecordedAssignmentPeriodsParams{OrgID: scope.OrgID().String(), MembershipID: member})
	if err != nil {
		return nil, safe(ctx, err)
	}
	periods := make([]app.RecordedRolePeriod, 0, len(rows))
	for _, row := range rows {
		if !row.CreatedAt.Valid || row.CreatedAt.InfinityModifier != pgtype.Finite || row.CreatedAt.Time.IsZero() || row.CreatedAt.Time.UTC().Year() < 1 || row.CreatedAt.Time.UTC().Year() > 9999 || row.MembershipID != member {
			return nil, app.ErrUnavailable
		}
		assignment, err := value(scope, q.GetAssignmentRow{ID: row.ID, MembershipID: row.MembershipID, Role: row.Role, ScopeType: row.ScopeType, ScopeTarget: row.ScopeTarget, ValidFrom: row.ValidFrom, ValidTo: row.ValidTo, AssignedBy: row.AssignedBy, Reason: row.Reason, Revision: row.Revision, EndedAt: row.EndedAt})
		if err != nil {
			return nil, err
		}
		periods = append(periods, app.RecordedRolePeriod{Assignment: assignment, CreatedAt: row.CreatedAt.Time.UTC()})
	}
	return periods, nil
}
