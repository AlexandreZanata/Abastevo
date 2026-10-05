package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

// VerifyPorts isolates review automation: exact-match resolution,
// unknown-quality pin recording (never projected) and audited
// decision persistence.
type VerifyPorts struct {
	Store     VerifyStore
	Resolve   func(ctx context.Context, cnpj, display string, address map[string]string) (stationID, municipality, state string, err error)
	RecordPin func(ctx context.Context, stationID string, lat, lon float64, ref string) error
	NewID     func() (string, error)
	Reviewer  string
}

// VerifyStore persists review decisions and pending transitions.
type VerifyStore interface {
	GetSuggestion(ctx context.Context, id string) (SuggestionRow, error)
	CreateDecision(ctx context.Context, id, suggestionID, decision, reason, reviewer, stationID string) error
	SetSuggestionState(ctx context.Context, id, state string) (int64, error)
	ListPending(ctx context.Context, limit int) ([]SuggestionRow, error)
}

var (
	ErrVerifyClosed   = errors.New("directory: suggestion is no longer pending")
	ErrVerifyReviewer = errors.New("directory: reviewer is required")
	ErrVerifyReason   = errors.New("directory: decision reason is required")
	ErrStationUnknown = errors.New("directory: station unknown")
)

const (
	DecisionApproved = "approved"
	DecisionRejected = "rejected"
	AutoReviewer     = "auto-verifier"
)

// VerifyResult reports one automation pass: approved links a station,
// deferred leaves the record for human review (never auto-rejection).
type VerifyResult struct {
	Approved  bool
	StationID string
}

// AutoVerify approves only exact official matches: the proposal CNPJ
// resolves to a canonical station in the same municipality/state.
// Conflicting addresses, unknown stations, missing CNPJs and any
// doubt stay pending for authorized review. Replays of decided
// records are harmless no-ops.
func AutoVerify(ctx context.Context, ports VerifyPorts, suggestionID string) (VerifyResult, error) {
	row, err := ports.Store.GetSuggestion(ctx, suggestionID)
	if err != nil {
		return VerifyResult{}, err
	}
	if row.State != SuggestionPending {
		return VerifyResult{}, nil
	}
	proposal, err := decodeProposal(row.Proposal)
	if err != nil || proposal.CNPJ == "" {
		return VerifyResult{}, nil
	}
	stationID, municipality, state, err := ports.Resolve(ctx, proposal.CNPJ, proposal.DisplayName, proposalAddress(proposal))
	if err != nil {
		// Unknown stations defer to human review; transport failures
		// surface for retry.
		if errors.Is(err, ErrStationUnknown) {
			return VerifyResult{}, nil
		}
		return VerifyResult{}, err
	}
	if stationID == "" {
		return VerifyResult{}, nil
	}
	if !strings.EqualFold(municipality, proposal.MunicipalityCode) && municipality != "" {
		return VerifyResult{}, nil
	}
	if state != "" && !strings.EqualFold(state, proposal.State) {
		return VerifyResult{}, nil
	}
	if err := decide(ctx, ports, row.ID, AutoReviewer, DecisionApproved, "exact official match", stationID); err != nil {
		return VerifyResult{}, err
	}
	if proposal.HasCoords {
		if err := ports.RecordPin(ctx, stationID, proposal.Latitude, proposal.Longitude, "suggestion:"+row.ID); err != nil {
			return VerifyResult{Approved: true, StationID: stationID}, err
		}
	}
	return VerifyResult{Approved: true, StationID: stationID}, nil
}

// Decide records one authorized human review: approval links a station
// (resolved beforehand by the reviewer), rejection needs a reason.
// Only pending records transition; concurrent reviewers converge on
// the SQL guard. Cancellation and appeal stay owner-side (a new
// suggestion); reviewers never edit proposals.
func Decide(ctx context.Context, ports VerifyPorts, suggestionID, reviewer string, approve bool, reason, stationID string) error {
	if strings.TrimSpace(reviewer) == "" {
		return ErrVerifyReviewer
	}
	if strings.TrimSpace(reason) == "" {
		return ErrVerifyReason
	}
	row, err := ports.Store.GetSuggestion(ctx, suggestionID)
	if err != nil {
		return err
	}
	if row.State != SuggestionPending {
		return ErrVerifyClosed
	}
	decision := DecisionRejected
	if approve {
		decision = DecisionApproved
	}
	return decide(ctx, ports, row.ID, reviewer, decision, strings.TrimSpace(reason), stationID)
}

func decide(ctx context.Context, ports VerifyPorts, id, reviewer, decision, reason, stationID string) error {
	newID, err := ports.NewID()
	if err != nil {
		return err
	}
	if err := ports.Store.CreateDecision(ctx, newID, id, decision, reason, reviewer, stationID); err != nil {
		return err
	}
	affected, err := ports.Store.SetSuggestionState(ctx, id, decisionToState(decision))
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrVerifyClosed
	}
	return nil
}

func decisionToState(decision string) string {
	if decision == DecisionApproved {
		return "approved"
	}
	return "rejected"
}

func decodeProposal(raw []byte) (Proposal, error) {
	var proposal Proposal
	if err := json.Unmarshal(raw, &proposal); err != nil {
		return Proposal{}, err
	}
	return proposal, nil
}

func proposalAddress(proposal Proposal) map[string]string {
	return map[string]string{
		"municipio_ibge": proposal.MunicipalityCode,
		"uf":             proposal.State,
	}
}
