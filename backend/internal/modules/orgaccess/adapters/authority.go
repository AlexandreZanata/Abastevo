package adapters

import (
	"context"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgaccess"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	m "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	o "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/jackc/pgx/v5"
)

var _ app.AuthoritySource = (*Store)(nil)

// LoadAuthority takes one current statement snapshot; it neither caches nor
// authorizes. The active account, deletion fence and every composite join are
// selected within the same forced-RLS scope and ordinary runtime transaction.
func (s *Store) LoadAuthority(ctx context.Context, scope kernel.OrgScope, account string) (app.Authority, error) {
	if err := validateID(scope, account); err != nil {
		return app.Authority{}, err
	}
	return run(ctx, s, scope, func(tx pgx.Tx) (app.Authority, error) {
		rows, err := q.New(tx).CurrentAuthority(ctx, q.CurrentAuthorityParams{OrgID: scope.OrgID().String(), AccountID: account})
		if err != nil {
			return app.Authority{}, safe(ctx, err)
		}
		if len(rows) == 0 {
			return app.Authority{}, app.ErrNotFound
		}
		first := rows[0]
		if !first.OrganizationState.Valid {
			return app.Authority{}, app.ErrUnavailable
		}
		current := app.Authority{OrgID: scope.OrgID(), AccountID: first.AccountID, MembershipID: first.MembershipID, EmployeeID: first.EmployeeID, MembershipState: m.MembershipState(first.MembershipState), EmployeeState: m.EmployeeState(first.EmployeeState), Model: m.AccessModel(first.AccessModel), OrganizationState: o.State(first.OrganizationState.String), Assignments: make([]d.Assignment, 0, len(rows))}
		for _, r := range rows {
			if r.AccountID != first.AccountID || r.MembershipID != first.MembershipID || r.EmployeeID != first.EmployeeID || r.MembershipState != first.MembershipState || r.AccessModel != first.AccessModel || r.EmployeeState != first.EmployeeState || r.OrganizationState != first.OrganizationState {
				return app.Authority{}, app.ErrUnavailable
			}
			if r.AssignmentID == "" {
				continue
			}
			assignment, err := value(scope, q.GetAssignmentRow{ID: r.AssignmentID, MembershipID: r.MembershipID, Role: r.Role, ScopeType: r.ScopeType, ScopeTarget: r.ScopeTarget, ValidFrom: r.ValidFrom, ValidTo: r.ValidTo, AssignedBy: r.AssignedBy, Reason: r.Reason, Revision: r.Revision, EndedAt: r.EndedAt})
			if err != nil {
				return app.Authority{}, err
			}
			current.Assignments = append(current.Assignments, assignment)
		}
		return current, nil
	})
}
