package adapters

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// ReservationOutcomeKind is the per-reservation time-driven outcome job
// (EMP-FLEET-007, EDR-193). It is safe to re-run: every settlement is a
// compare-and-set on the stored state and revision.
const (
	ReservationOutcomeKind = "org-asset-reservation-outcome"
	// ReservationOutcomeAttempts bounds the retries of one queued outcome
	// (a run before its instant charges one).
	ReservationOutcomeAttempts int32 = 10
)

var errOutcomePayload = errors.New("fleet: unsupported reservation outcome payload")

// outcomePurposes are the two §6.4 time rows, one job each.
var outcomePurposes = map[fleet.ReservationCommand]string{fleet.ExpireReservation: "expire", fleet.MissReservation: "miss"}

// ReservationOutcomePayload is the reference-only versioned envelope.
type ReservationOutcomePayload struct {
	Version       int    `json:"version"`
	OrgID         string `json:"org_id"`
	ReservationID string `json:"reservation_id"`
	Outcome       string `json:"outcome"`
}

// ReservationOutcomeDedupe returns the queue de-duplication key of one
// outcome of one reservation.
func ReservationOutcomeDedupe(command fleet.ReservationCommand, scope kernel.OrgScope, id string) string {
	purpose, ok := outcomePurposes[command]
	if !ok || scope.Validate() != nil || !validID(id) {
		return ""
	}
	return ReservationOutcomeKind + ":" + purpose + ":" + scope.OrgID().String() + ":" + id
}

// ParseReservationOutcome accepts only the exact version-1 envelope.
func ParseReservationOutcome(raw []byte) (kernel.OrgScope, string, fleet.ReservationCommand, error) {
	var p ReservationOutcomePayload
	if httpapi.DecodeJSON(raw, &p) != nil || p.Version != 1 || !validID(p.OrgID) || !validID(p.ReservationID) {
		return kernel.OrgScope{}, "", "", errOutcomePayload
	}
	command := fleet.ReservationCommand(p.Outcome)
	if _, ok := outcomePurposes[command]; !ok {
		return kernel.OrgScope{}, "", "", errOutcomePayload
	}
	id, err := kernel.ParseOrgID(p.OrgID)
	if err != nil {
		return kernel.OrgScope{}, "", "", errOutcomePayload
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		return kernel.OrgScope{}, "", "", errOutcomePayload
	}
	return scope, p.ReservationID, command, nil
}

// enqueueOutcome queues the time-driven outcome due for the stored state in
// the caller's transaction, not before its instant; a repeat converges on
// the stored job.
func enqueueOutcome(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, id string, r fleet.Reservation) error {
	due, command, ok := r.DueAt()
	dedupe := ReservationOutcomeDedupe(command, scope, id)
	if !ok || dedupe == "" {
		return app.ErrUnavailable
	}
	payload, err := json.Marshal(ReservationOutcomePayload{Version: 1, OrgID: scope.OrgID().String(), ReservationID: id, Outcome: string(command)})
	if err != nil {
		return err
	}
	_, err = jobs.Enqueue(ctx, tx, ReservationOutcomeKind, payload, dedupe, ReservationOutcomeAttempts, due)
	return err
}

// ReservationOutcomeHandler settles one queued outcome.
type ReservationOutcomeHandler struct{ Outcomes app.ReservationOutcomes }

var _ jobs.Handler = ReservationOutcomeHandler{}

func (ReservationOutcomeHandler) Kind() string { return ReservationOutcomeKind }
func (ReservationOutcomeHandler) Version() int { return 1 }

// Handle reports only static module errors to the queue. A missing or
// another organization's reservation, or an outcome already reached or made
// moot by a later command, completes the job without a change.
func (h ReservationOutcomeHandler) Handle(ctx context.Context, job jobs.Job) error {
	if meterIntakeAbsent(h.Outcomes) {
		return app.ErrUnavailable
	}
	if job.Kind != ReservationOutcomeKind {
		return errOutcomePayload
	}
	scope, id, command, err := ParseReservationOutcome(job.Payload)
	if err != nil || job.DedupeKey == "" || job.DedupeKey != ReservationOutcomeDedupe(command, scope, id) {
		return errOutcomePayload
	}
	_, err = h.Outcomes.SettleReservation(ctx, scope, id, command)
	switch {
	case err == nil, errors.Is(err, app.ErrNotFound):
		return nil
	case errors.Is(err, app.ErrNotDue):
		return app.ErrNotDue
	}
	return app.ErrUnavailable
}
