package application

import (
	"context"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
)

// EMP-TRIP-019 checklist gate at trip start and finish (B-BR-EMP-511..516,
// EDR-331..334). The device names its departure run on trip.start and may
// name its return run on trip.finish through one `checklist_run` soft
// reference; the server decides the gate at receipt under the start's root
// and asset locks. The acknowledgement keeps the closed held codes; the
// specific gate reason is a stored server fact beside the held start.

const (
	// ChecklistRunReferenceType is the soft-reference type naming a checklist
	// run (EDR-331).
	ChecklistRunReferenceType = "checklist_run"
	// Run contexts a trip boundary links (the CHECK-001 vocabulary).
	ChecklistDeparture = "DEPARTURE"
	ChecklistReturn    = "RETURN"
	// Gate reasons stored beside a held start (EDR-333); never ACK codes.
	ChecklistRequired = "CHECKLIST_REQUIRED"
	ChecklistBlocking = "CHECKLIST_BLOCKING"
	ChecklistExpired  = "CHECKLIST_EXPIRED"
)

// ChecklistRun is one stored checklist run as a trip boundary reads it at an
// instant: its identity facts, whether its server outcome was BLOCKING at
// that instant (recheck BLOCKING, not released before it, EDR-285) and the
// validity of its template's policy revision in force at that instant by the
// server's history. Linked is filled by the trip store (one run, one trip).
type ChecklistRun struct {
	private
	ID, AssetID, Context, AuthorMembershipID string
	CompletedAt                              time.Time
	BlockingAt                               bool
	ValiditySeconds                          int64
	Linked                                   bool
}

// ChecklistGate is the maintenance port the trip owners read on the borrowed
// intake transaction, under the caller's organization root lock (EDR-334).
type ChecklistGate interface {
	// DepartureRequiredDBTX applies the CHECK-001 applicable-template rule
	// (BR-CHECK-002) to the asset's current DEPARTURE templates.
	DepartureRequiredDBTX(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, asset string) (bool, error)
	// ChecklistRunDBTX reads a run of the organization at the instant;
	// ErrNotFound when no such run is stored in the organization.
	ChecklistRunDBTX(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, run string, at time.Time) (ChecklistRun, error)
}

// GateDecision is the departure gate of one start: Reason is the stored gate
// reason ("" when it passes), Code the held acknowledgement code it implies
// ("" when it passes), Link whether the named run is linked to the trip.
type GateDecision struct {
	Reason, Code string
	Link         bool
}

// DepartureGate decides the departure gate (B-BR-EMP-512/513). named is the
// run the start names ("" none); run its stored facts at the declared start
// instant (nil when it is not stored in the organization).
func DepartureGate(required bool, named string, run *ChecklistRun, author, asset string, at time.Time) GateDecision {
	if named == "" {
		if required {
			return GateDecision{Reason: ChecklistRequired, Code: AssetUnavailable}
		}
		return GateDecision{}
	}
	// BR-OFFL-007: a named run that does not count is an orphan reference,
	// the requirement unmet when a checklist is required.
	if !counts(named, run, ChecklistDeparture, author, asset) {
		if required {
			return GateDecision{Reason: ChecklistRequired, Code: OrphanReference}
		}
		return GateDecision{Reason: OrphanReference, Code: OrphanReference}
	}
	if run.BlockingAt {
		return GateDecision{Reason: ChecklistBlocking, Code: AssetUnavailable}
	}
	// BR-CHECK-012: valid in [completion, completion + validity).
	if run.ValiditySeconds <= 0 || at.Before(run.CompletedAt) || !at.Before(run.CompletedAt.Add(time.Duration(run.ValiditySeconds)*time.Second)) {
		return GateDecision{Reason: ChecklistExpired, Code: AssetUnavailable}
	}
	return GateDecision{Link: true}
}

// counts reports whether a named run is the author's own stored run of the
// context on the asset that no trip has linked yet.
func counts(named string, run *ChecklistRun, context, author, asset string) bool {
	return run != nil && named != "" && run.ID == named && run.Context == context && run.AssetID == asset && author != "" &&
		run.AuthorMembershipID == author && !run.Linked && !run.CompletedAt.IsZero()
}

// ReturnLinkable reports whether a finish links the return run it names
// (B-BR-EMP-515): never a reason to hold the finish.
func ReturnLinkable(named string, run *ChecklistRun, author, asset string, started, finished time.Time) bool {
	return counts(named, run, ChecklistReturn, author, asset) && !run.CompletedAt.Before(started) && !run.CompletedAt.After(finished)
}
