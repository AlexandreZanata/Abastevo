package domain

import (
	"regexp"
	"time"
)

var payoutHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type ReconciledRecord struct {
	ID       string `json:"id"`
	Revision int64  `json:"revision"`
}

type ReconciliationDecision struct {
	ID               string             `json:"id"`
	Version          int64              `json:"version"`
	Kind             string             `json:"kind"`
	Platform         string             `json:"platform"`
	From             time.Time          `json:"from"`
	To               time.Time          `json:"to"`
	PrimaryID        string             `json:"primary_id"`
	PrimaryRevision  int64              `json:"primary_revision"`
	Counterparts     []ReconciledRecord `json:"counterparts"`
	PayoutSourceHash string             `json:"payout_source_hash,omitempty"`
}

func (d ReconciliationDecision) Normalize() (ReconciliationDecision, error) {
	out := d
	out.Counterparts = append([]ReconciledRecord(nil), d.Counterparts...)
	if e := out.validate(); e != nil {
		return out, e
	}
	out.From = out.From.UTC()
	out.To = out.To.UTC()
	if out.From.Year() < 1 || out.From.Year() > 9999 || out.To.Year() < 1 || out.To.Year() > 9999 || !out.To.After(out.From) || out.From.Nanosecond()%1000 != 0 || out.To.Nanosecond()%1000 != 0 {
		return out, ErrInvalid
	}
	return out, nil
}

func (d ReconciliationDecision) validate() error {
	if !ValidID(d.ID) || d.Version < 1 || !ValidID(d.PrimaryID) || d.PrimaryRevision < 1 || len(d.Counterparts) > MaxItems || !d.To.After(d.From) {
		return ErrInvalid
	}
	if d.Platform != "UBER" && d.Platform != "99" && d.Platform != "INDRIVE" && d.Platform != "OTHER" {
		return ErrInvalid
	}
	switch d.Kind {
	case "SUMMARY_DETAIL":
		if d.PayoutSourceHash != "" || len(d.Counterparts) == 0 {
			return ErrInvalid
		}
	case "PAYOUT_EARNINGS":
		if !payoutHashPattern.MatchString(d.PayoutSourceHash) {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	seen := map[string]bool{d.PrimaryID: true}
	for _, c := range d.Counterparts {
		if !ValidID(c.ID) || c.Revision < 1 || seen[c.ID] {
			return ErrInvalid
		}
		seen[c.ID] = true
	}
	return nil
}

func (d ReconciliationDecision) ContentHash() (string, error) {
	normalized, e := d.Normalize()
	if e != nil {
		return "", e
	}
	return Digest(normalized)
}
