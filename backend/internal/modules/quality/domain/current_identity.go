package domain

import "strings"

// IdentityEvidence carries identifier continuity, never business validity dates.
// Operational insertion/retirement timestamps cannot establish business windows.
type IdentityEvidence struct {
	CNPJ, StationID        string
	Current, StationActive bool
}

// ResolveCurrentIdentity is the conservative real-Directory policy when business
// validity windows are unavailable. Multiple canonical identities always require
// review, including a retired predecessor plus one current identity.
func ResolveCurrentIdentity(record SourceRecord, decidedAt string, evidence []IdentityEvidence, otherBranch bool) Decision {
	observed := strings.TrimSpace(record.ObservedAt)
	decidedAt = strings.TrimSpace(decidedAt)
	d := Decision{Kind: ResolutionUnmatched, Reason: ReasonUnmatchedID, ObservedAt: observed, DecidedAt: decidedAt, Link: StationLink{RecordKey: record.RecordKey, Reason: ReasonUnmatchedID}}
	if !ValidDay(observed) || !ValidDay(decidedAt) {
		d.Reason = ReasonMissingSample + ": malformed observation or decision date"
		d.Link.Reason = d.Reason
		return d
	}
	cnpj, e := normalizeShape(record.CNPJ)
	if e != nil {
		d.Reason = ReasonInvalidCNPJ
		d.Link.Reason = d.Reason
		return d
	}
	ids := map[string]bool{}
	for _, row := range evidence {
		if row.CNPJ == cnpj && row.StationID != "" {
			ids[row.StationID] = ids[row.StationID] || (row.Current && row.StationActive)
		}
	}
	if len(ids) > 1 {
		d.Kind = ResolutionAmbiguous
		d.Reason = ReasonAmbiguous
	} else if len(ids) == 1 {
		for id, current := range ids {
			if current {
				d.Kind = ResolutionBound
				d.Reason = "exact current cnpj; business window unavailable"
				d.Link = StationLink{RecordKey: record.RecordKey, StationID: id, Automatic: true, Active: true, Reason: d.Reason}
				return d
			}
		}
		d.Kind = ResolutionSuccession
		d.Reason = ReasonSuccession
	} else if otherBranch {
		d.Kind = ResolutionWrongBranch
		d.Reason = ReasonWrongBranch
	}
	d.Link.Reason = d.Reason
	return d
}
