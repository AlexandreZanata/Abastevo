package domain

import (
	"errors"
	"strings"
)

// Resolution kinds. Only Bound proposes an active link; every other kind
// quarantines the record for FQ-03 review with its reason. No kind creates
// a station or approves coordinates.
type ResolutionKind string

const (
	ResolutionBound       ResolutionKind = "bound"
	ResolutionUnmatched   ResolutionKind = "unmatched"
	ResolutionAmbiguous   ResolutionKind = "ambiguous"
	ResolutionWrongBranch ResolutionKind = "wrong_branch"
	ResolutionSuccession  ResolutionKind = "succession_review"
)

var (
	ErrInvalidCNPJShape = errors.New("quality: malformed CNPJ shape")
	ErrInvalidDate      = errors.New("quality: malformed date-only value")
	ErrWindowInverted   = errors.New("quality: effective window inverted")
	ErrWindowOutside    = errors.New("quality: effective window outside binding validity")
)

// Additional quarantine reasons for the review queue.
const (
	ReasonWrongBranch = "wrong_branch"
	ReasonSuccession  = "succession_review"
	ReasonAmbiguous   = "ambiguous_binding"
	ReasonUnmatchedID = "unmatched_cnpj"
)

// Binding is Directory's read-only view of one CNPJ validity window. Dates
// are date-only source values (YYYY-MM-DD); empty bounds stay open. Names
// are informational: a renamed station keeps its CNPJ binding.
type Binding struct {
	StationID string
	CNPJ      string
	Name      string
	ValidFrom string
	ValidTo   string
}

// SourceRecord is the minimal identity surface of one imported record.
type SourceRecord struct {
	RecordKey  string
	CNPJ       string
	ObservedAt string
}

// Decision is one resolved record. ObservedAt is the source collection
// date; DecidedAt is the reviewer-injected decision date. The clocks are
// independent: review never rewrites observation, and no ordering is
// imposed between them — both are recorded explicitly.
type Decision struct {
	Kind       ResolutionKind
	Reason     string
	Link       StationLink
	ObservedAt string
	DecidedAt  string
}

// normalizeShape strips CNPJ formatting and uppercases, preserving leading
// zeroes as text. Checksum validation stays with kernel/Directory: this
// only gates shape, so a well-formed but unchecked identifier resolves
// opaquely instead of being refused or validated here.
func normalizeShape(raw string) (string, error) {
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= '0' && r <= '9', r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= 'a' && r <= 'z':
			b.WriteRune(r - ('a' - 'A'))
		case r == '.' || r == '/' || r == '-' || r == ' ':
		default:
			return "", ErrInvalidCNPJShape
		}
	}
	if s := b.String(); len(s) == 14 {
		return s, nil
	}
	return "", ErrInvalidCNPJShape
}

func validDate(s string) bool {
	return ValidDay(s)
}

// inWindow reports whether observed falls inside the binding validity.
// Empty bounds stay open; an inverted binding window matches nothing.
func inWindow(observed string, b Binding) bool {
	if !validDate(observed) {
		return false
	}
	if b.ValidFrom != "" && (!validDate(b.ValidFrom) || observed < b.ValidFrom) {
		return false
	}
	if b.ValidTo != "" && (!validDate(b.ValidTo) || observed > b.ValidTo) {
		return false
	}
	if b.ValidFrom != "" && b.ValidTo != "" && b.ValidTo < b.ValidFrom {
		return false
	}
	return true
}

// DirectoryView is the read-only Directory port FQ-03 needs: exact
// bindings by full CNPJ plus base-index evidence for wrong-branch review.
// It performs no writes; FQ-04 adapts the real store to it.
type DirectoryView interface {
	ByCNPJ(cnpj string) []Binding
	ByBase(base string) []Binding
}

// Resolve binds one source record through a read-only DirectoryView.
// lookup must not write: it returns current bindings for an exact CNPJ, or
// nil when unknown. Name, address, brand and proximity never participate:
// only the full establishment CNPJ plus chronology decide.
//
//   - exact single binding covering the observation date → Bound with an
//     active automatic link (a renamed station stays bound: the name is
//     informational);
//   - exact bindings observed outside every window → Succession review
//     (historical replacement lead, never an automatic rebind);
//   - several bindings for one CNPJ → Ambiguous via ReviewLinks;
//   - same 8-digit base under a different branch → WrongBranch review;
//   - no binding at all → Unmatched quarantine;
//   - malformed CNPJ shape or date → Unmatched with the reason.
func Resolve(record SourceRecord, decidedAt string, directory DirectoryView) Decision {
	observed := strings.TrimSpace(record.ObservedAt)
	if !validDate(observed) || !validDate(strings.TrimSpace(decidedAt)) {
		return Decision{
			Kind:       ResolutionUnmatched,
			Reason:     ReasonMissingSample + ": malformed observation or decision date",
			ObservedAt: observed,
			DecidedAt:  strings.TrimSpace(decidedAt),
		}
	}
	cnpj, err := normalizeShape(record.CNPJ)
	if err != nil {
		return Decision{
			Kind:       ResolutionUnmatched,
			Reason:     ReasonInvalidCNPJ,
			ObservedAt: observed,
			DecidedAt:  strings.TrimSpace(decidedAt),
		}
	}
	bindings := directory.ByCNPJ(cnpj)
	exact := make([]Binding, 0, len(bindings))
	for _, b := range bindings {
		if b.CNPJ == cnpj {
			exact = append(exact, b)
		}
	}
	if len(exact) > 1 {
		links := make([]StationLink, 0, len(exact))
		for _, b := range exact {
			links = append(links, StationLink{
				StationID: b.StationID, RecordKey: record.RecordKey,
				Automatic: true, Reason: "exact cnpj", Active: true,
			})
		}
		reviewed := ReviewLinks(links)
		return Decision{
			Kind: ResolutionAmbiguous, Reason: ReasonAmbiguous,
			Link:       reviewed[0],
			ObservedAt: observed, DecidedAt: strings.TrimSpace(decidedAt),
		}
	}
	if len(exact) == 1 {
		b := exact[0]
		link := StationLink{
			StationID: b.StationID, RecordKey: record.RecordKey,
			Automatic: true, Reason: "exact cnpj", Active: true,
		}
		if inWindow(observed, b) {
			return Decision{
				Kind: ResolutionBound, Reason: "exact cnpj",
				Link:       link,
				ObservedAt: observed, DecidedAt: strings.TrimSpace(decidedAt),
			}
		}
		link.Active = false
		link.Reason = ReasonSuccession + ": observation outside binding validity"
		return Decision{
			Kind: ResolutionSuccession, Reason: ReasonSuccession,
			Link:       link,
			ObservedAt: observed, DecidedAt: strings.TrimSpace(decidedAt),
		}
	}
	if base := cnpj[:8]; hasForeignBranch(base, cnpj, directory) {
		return Decision{
			Kind:   ResolutionWrongBranch,
			Reason: ReasonWrongBranch + ": same base " + base + ", different branch",
			Link: StationLink{
				RecordKey: record.RecordKey,
				Automatic: false, Reason: ReasonWrongBranch, Active: false,
			},
			ObservedAt: observed, DecidedAt: strings.TrimSpace(decidedAt),
		}
	}
	return Decision{
		Kind:   ResolutionUnmatched,
		Reason: ReasonUnmatchedID,
		Link: StationLink{
			RecordKey: record.RecordKey,
			Automatic: false, Reason: ReasonUnmatchedID, Active: false,
		},
		ObservedAt: observed, DecidedAt: strings.TrimSpace(decidedAt),
	}
}

// hasForeignBranch reports whether any known binding shares the 8-digit
// base under a different branch. The base alone never assigns a record to
// a station; a hit only routes to review.
func hasForeignBranch(base, cnpj string, directory DirectoryView) bool {
	for _, b := range directory.ByBase(base) {
		if len(b.CNPJ) == 14 && b.CNPJ[:8] == base && b.CNPJ != cnpj {
			return true
		}
	}
	return false
}

// LinkRevision is the explicit revision contract for one station link:
// the proposed link, the correction chain applied to it, the independent
// source/review clocks and the effective window. FQ-01 owns Revision and
// ApplyCorrection; this type binds them to a link.
type LinkRevision struct {
	Link          StationLink
	Corrections   []Revision
	ObservedAt    string
	DecidedAt     string
	EffectiveFrom string
	EffectiveTo   string
}

// Validate checks the revision contract against the owning binding window:
// clocks are well-formed dates, the effective window is ordered, and it
// overlaps the binding validity (open bounds allowed). It never mutates.
func (r LinkRevision) Validate(binding Binding) error {
	if !validDate(r.ObservedAt) || !validDate(r.DecidedAt) {
		return ErrInvalidDate
	}
	if r.EffectiveFrom != "" && !validDate(r.EffectiveFrom) {
		return ErrInvalidDate
	}
	if r.EffectiveTo != "" && !validDate(r.EffectiveTo) {
		return ErrInvalidDate
	}
	if r.EffectiveFrom != "" && r.EffectiveTo != "" && r.EffectiveTo < r.EffectiveFrom {
		return ErrWindowInverted
	}
	from, to := r.EffectiveFrom, r.EffectiveTo
	if from == "" {
		from = binding.ValidFrom
	}
	if to == "" {
		to = binding.ValidTo
	}
	if from != "" && to != "" && to < from {
		return ErrWindowOutside
	}
	if binding.ValidFrom != "" && to != "" && to < binding.ValidFrom {
		return ErrWindowOutside
	}
	if binding.ValidTo != "" && from != "" && from > binding.ValidTo {
		return ErrWindowOutside
	}
	return nil
}
