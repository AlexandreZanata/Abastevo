package application

import (
	"errors"
	"sort"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
)

var ErrCompanyAssociationInput = errors.New("quality: invalid company association publication")

// CompanyAssociationInput is an optional complete census of accepted fine
// observations. Nil retains the pre-association contract; empty is explicit.
type CompanyAssociationInput struct {
	PolicyVersion, DecisionDay string
	Decisions                  []domain.CompanyObservationDecision
}

// CompanyBinding is derived from source CNPJs and validated company decisions,
// then passed to the Directory transaction guard. Jobs never supply this list.
type CompanyBinding struct{ CNPJ, StationID string }

type CompanyDecisionIdentity struct{ RecordKey, StationID, Kind, Scope, Reason string }
type CompanyAssociationIdentity struct {
	PolicyVersion, Scope          string
	HistoricalBusinessWindowKnown bool
	Decisions                     []CompanyDecisionIdentity
}

// PublicationIdentity excludes operational decision day and canonicalizes order.
// It must only be used after Bindings has validated the complete source census.
func (in *CompanyAssociationInput) PublicationIdentity() *CompanyAssociationIdentity {
	if in == nil {
		return nil
	}
	out := &CompanyAssociationIdentity{PolicyVersion: in.PolicyVersion, Scope: domain.CurrentCompanyOnly, Decisions: make([]CompanyDecisionIdentity, 0, len(in.Decisions))}
	for _, d := range in.Decisions {
		out.Decisions = append(out.Decisions, CompanyDecisionIdentity{RecordKey: d.RecordKey, StationID: d.StationID, Kind: string(d.Kind), Scope: d.Scope, Reason: d.Reason})
	}
	sort.Slice(out.Decisions, func(i, j int) bool { return out.Decisions[i].RecordKey < out.Decisions[j].RecordKey })
	return out
}

// Bindings validates one decision for every source observation and one
// consistent verdict per full company identifier before database I/O.
func (in *CompanyAssociationInput) Bindings(sources map[string]string) ([]CompanyBinding, error) {
	if in == nil {
		return nil, nil
	}
	if in.PolicyVersion != domain.CompanyObservationPolicyVersion || !domain.ValidDay(in.DecisionDay) || len(sources) > MaxCompanyObservations || len(in.Decisions) != len(sources) {
		return nil, ErrCompanyAssociationInput
	}
	for key, cnpj := range sources {
		id, e := kernel.ParseCNPJ(cnpj)
		if key == "" || e != nil || id.Normalized() != cnpj {
			return nil, ErrCompanyAssociationInput
		}
	}
	seen := make(map[string]bool, len(in.Decisions))
	verdicts := map[string]CompanyDecisionIdentity{}
	bound := map[string]string{}
	for _, d := range in.Decisions {
		cnpj, known := sources[d.RecordKey]
		if !known || seen[d.RecordKey] || d.PolicyVersion != in.PolicyVersion || d.DecidedAt != in.DecisionDay || d.HistoricalBusinessWindowKnown {
			return nil, ErrCompanyAssociationInput
		}
		seen[d.RecordKey] = true
		reason := ""
		switch d.Kind {
		case domain.ResolutionBound:
			station, e := ValidateStationID(d.StationID)
			if e != nil || station != d.StationID || d.Scope != domain.CurrentCompanyOnly {
				return nil, ErrCompanyAssociationInput
			}
			reason = "exact current cnpj; historical business window unknown"
			bound[cnpj] = station
		case domain.ResolutionUnmatched:
			reason = domain.ReasonUnmatchedID
		case domain.ResolutionAmbiguous:
			reason = domain.ReasonAmbiguous
		case domain.ResolutionWrongBranch:
			reason = domain.ReasonWrongBranch
		case domain.ResolutionSuccession:
			reason = domain.ReasonSuccession
		default:
			return nil, ErrCompanyAssociationInput
		}
		if d.Reason != reason || (d.Kind != domain.ResolutionBound && (d.StationID != "" || d.Scope != "")) {
			return nil, ErrCompanyAssociationInput
		}
		verdict := CompanyDecisionIdentity{StationID: d.StationID, Kind: string(d.Kind), Scope: d.Scope, Reason: d.Reason}
		if previous, ok := verdicts[cnpj]; ok && previous != verdict {
			return nil, ErrCompanyAssociationInput
		}
		verdicts[cnpj] = verdict
	}
	keys := make([]string, 0, len(bound))
	for cnpj := range bound {
		keys = append(keys, cnpj)
	}
	sort.Strings(keys)
	out := make([]CompanyBinding, 0, len(keys))
	for _, cnpj := range keys {
		out = append(out, CompanyBinding{CNPJ: cnpj, StationID: bound[cnpj]})
	}
	return out, nil
}
