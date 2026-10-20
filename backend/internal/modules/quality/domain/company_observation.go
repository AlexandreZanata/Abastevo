package domain

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// CurrentCompanyOnly relates a source company's observation to its one current
// canonical identity. It establishes no historical site/operator or incident.
const CurrentCompanyOnly = "CURRENT_COMPANY_ONLY"
const CompanyObservationPolicyVersion = "current-company-v1"

// CompanyObservationRecord deliberately has no event/due/extraction date. Fine
// due dates cannot substitute for missing incident or business-validity dates.
// The application validates the CNPJ checksum before this pure shape policy.
type CompanyObservationRecord struct{ RecordKey, CNPJ string }

func (r CompanyObservationRecord) ValidShape() bool {
	_, e := normalizeShape(r.CNPJ)
	return e == nil && r.RecordKey != "" && len(r.RecordKey) <= 512 && strings.TrimSpace(r.RecordKey) == r.RecordKey && utf8.ValidString(r.RecordKey) && strings.IndexFunc(r.RecordKey, unicode.IsControl) < 0
}

// CompanyObservationDecision is separate from the dated PMQC Decision/StationLink
// type so callers cannot silently treat a company observation as a site finding.
// HistoricalBusinessWindowKnown is always false under this current-only policy.
type CompanyObservationDecision struct {
	Kind                                                          ResolutionKind
	RecordKey, StationID, Scope, Reason, DecidedAt, PolicyVersion string
	HistoricalBusinessWindowKnown                                 bool
}

func ResolveCompanyObservation(record CompanyObservationRecord, decidedAt string, evidence []IdentityEvidence, otherBranch bool) CompanyObservationDecision {
	d := CompanyObservationDecision{Kind: ResolutionUnmatched, RecordKey: record.RecordKey, Reason: ReasonUnmatchedID, DecidedAt: decidedAt, PolicyVersion: CompanyObservationPolicyVersion}
	if !record.ValidShape() || !ValidDay(decidedAt) {
		d.Reason = "invalid company observation or decision day"
		return d
	}
	cnpj, _ := normalizeShape(record.CNPJ)
	ids := map[string]bool{}
	for _, row := range evidence {
		if row.CNPJ == cnpj && strings.TrimSpace(row.StationID) != "" {
			ids[row.StationID] = ids[row.StationID] || (row.Current && row.StationActive)
		}
	}
	switch len(ids) {
	case 0:
		if otherBranch {
			d.Kind = ResolutionWrongBranch
			d.Reason = ReasonWrongBranch
		}
	case 1:
		for id, current := range ids {
			if current {
				d.Kind = ResolutionBound
				d.StationID = id
				d.Scope = CurrentCompanyOnly
				d.Reason = "exact current cnpj; historical business window unknown"
				return d
			}
		}
		d.Kind = ResolutionSuccession
		d.Reason = ReasonSuccession
	default:
		d.Kind = ResolutionAmbiguous
		d.Reason = ReasonAmbiguous
	}
	return d
}
