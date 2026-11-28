package application

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// DecisionNotice is one terminal admissibility decision (VALIDATED or
// REJECTED) of a live observation, reported to the notification port
// (B-BR-NOTIF-02). Every decision, worker or moderator, is appended to the
// decision log the public projection reads, so this is the canonical
// point where an observation "reaches" a decision. Only public facts
// travel: the attribution token stays server-side for account lookup.
type DecisionNotice struct {
	ObservationID  string
	ContributorRef string
	StationID      string
	StationName    string
	// Product is the existing wire product string (for example ETHANOL).
	Product   string
	Validated bool
	// Reason is the first stable reason code of a rejection, empty when
	// validated.
	Reason string
}

// DecisionNotifier receives a DecisionNotice inside the transaction that
// appends the decision; an error rolls the decision back with it.
type DecisionNotifier func(ctx context.Context, tx pgx.Tx, n DecisionNotice) error
