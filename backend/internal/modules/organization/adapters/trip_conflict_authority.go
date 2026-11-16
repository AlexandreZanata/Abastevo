package adapters

import (
	"context"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	trippg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/adapters"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	"github.com/jackc/pgx/v5"
)

// TripConflictAuthority proves `trip.trip.review` over a held trip item with
// the EMP-TRIP-004 company proof and scoped roles (EMP-TRIP-007, EDR-263).
// Structural grants resolve through the HeldTripStructure kind; the owning
// store compares the returned reviewer with the item's author and a held
// start's declared driver (BR-REV-003).
type TripConflictAuthority TripReviewAuthority

var _ trippg.ConflictAuthority = (*TripConflictAuthority)(nil)

func (g *TripConflictAuthority) CheckTripConflictTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor tripapp.ReviewActor, member, target string, write bool) (tripapp.Reviewer, error) {
	if g == nil {
		return tripapp.Reviewer{}, tripapp.ErrUnavailable
	}
	return (*TripReviewAuthority)(g).check(ctx, tx, scope, actor, member, target, tripapp.ConflictPolicy(write), write, HeldTripStructure)
}
