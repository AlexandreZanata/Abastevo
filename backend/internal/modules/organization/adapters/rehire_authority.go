package adapters

import (
	"context"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	"github.com/jackc/pgx/v5"
)

// Rehire uses the existing fresh membership configuration-write source policy.
type RehireAuthority struct{ MembershipAccessAuthority }

func (g *RehireAuthority) CheckRehireTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor members.EmployeeActor, member string) error {
	if g == nil {
		return members.ErrRegisterUnavailable
	}
	_, e := g.CheckMembershipAccessTx(ctx, tx, scope, actor, member, members.MembershipReactivate)
	return e
}
