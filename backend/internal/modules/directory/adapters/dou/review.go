package dou

import (
	"errors"
	"time"
)

// MissingDates lists weekday edition dates in [from, to] not yet seen,
// for bounded catch-up after an outage. DOU publishes on weekdays:
// weekends are never listed (not failures). Bad ranges return empty,
// never an error masquerading as dates. Seen-dates come from edition
// checkpoints so replays never refetch.
func MissingDates(from, to string, seen func(date string) bool) []string {
	start, err := time.Parse("2006-01-02", from)
	if err != nil {
		return nil
	}
	end, err := time.Parse("2006-01-02", to)
	if err != nil || end.Before(start) {
		return nil
	}
	var out []string
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			continue
		}
		date := day.Format("2006-01-02")
		if !seen(date) {
			out = append(out, date)
		}
	}
	return out
}

// CorrectionLink is one reviewed correction-chain edge: the new
// assertion supersedes the prior act's assertion. Review authority
// stays with the existing moderation ports; this type only carries the
// audited link (both sides identified, no self-links).
type CorrectionLink struct {
	AssertionID   string
	ActID         string
	CorrectsActID string
	SupersededBy  string
}

// LinkCorrection validates and builds the supersession edge. Unrelated
// acts and self-links are refused; ambiguous chains stay quarantined
// for human review instead of guessing.
func LinkCorrection(prior, next CorrectionLink) (CorrectionLink, error) {
	if next.CorrectsActID == "" || next.CorrectsActID != prior.ActID {
		return CorrectionLink{}, errors.New("dou: correction does not reference the prior act")
	}
	if next.AssertionID == "" || prior.AssertionID == "" {
		return CorrectionLink{}, errors.New("dou: correction needs both assertions")
	}
	if next.AssertionID == prior.AssertionID {
		return CorrectionLink{}, errors.New("dou: correction cannot supersede itself")
	}
	next.SupersededBy = prior.AssertionID
	return next, nil
}
