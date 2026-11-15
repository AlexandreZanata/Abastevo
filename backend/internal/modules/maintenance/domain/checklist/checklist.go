// Package checklist is the pure checklist template rule of EMP-CHECK-001
// (B-BR-EMP-330..334): bounded immutable definitions, the per-template
// critical-item policy and its history, the applicable-template resolution
// and the answer/outcome rule that EMP-CHECK-002 evaluates offline and on the
// server. It is standard-library only and mirrored in Kotlin
// (com.anpfuel.domain.enterprise.maintenance.checklist) through the shared
// checklist-v1 oracle.
package checklist

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	// ErrInvalid is a malformed definition, policy, answer set, candidate set
	// or history.
	ErrInvalid = errors.New("checklist: invalid")
	// ErrAnswerNotAllowed is NOT_APPLICABLE on an item that is not optional.
	ErrAnswerNotAllowed = errors.New("checklist: answer not allowed")
	// ErrPhotoRequired is NOT_OK without a photo on an item that requires one.
	ErrPhotoRequired = errors.New("checklist: photo required")
	// ErrIncomplete is an item left unanswered.
	ErrIncomplete = errors.New("checklist: incomplete")
)

// Bounds of B-BR-EMP-330/333/334.
const (
	MaxItems               = 40
	MaxTargets             = 100
	MaxNameScalars         = 80
	MaxLabelScalars        = 120
	MaxNoteScalars         = 140
	MinValiditySeconds     = 3600
	MaxValiditySeconds     = 259200
	DefaultValiditySeconds = 43200
)

// Context is the fixed moment a template serves.
type Context string

const (
	Departure Context = "DEPARTURE"
	Return    Context = "RETURN"
	AdHoc     Context = "AD_HOC"
)

func ParseContext(raw string) (Context, error) {
	switch c := Context(raw); c {
	case Departure, Return, AdHoc:
		return c, nil
	}
	return "", ErrInvalid
}

// TargetKind names what a version applies to.
type TargetKind string

const (
	TargetOrganization TargetKind = "ORGANIZATION"
	TargetCategories   TargetKind = "CATEGORIES"
	TargetAssets       TargetKind = "ASSETS"
)

func ParseTargetKind(raw string) (TargetKind, error) {
	switch k := TargetKind(raw); k {
	case TargetOrganization, TargetCategories, TargetAssets:
		return k, nil
	}
	return "", ErrInvalid
}

// Target is the organization default (no identifiers) or 1..MaxTargets
// distinct canonical category or asset identifiers.
type Target struct {
	Kind TargetKind
	IDs  []string
}

// Item is one ordered question; its identity is its 1-based position.
type Item struct {
	Label                            string
	Critical, Optional, PhotoOnNotOK bool
}

// Definition is the content of one template version.
type Definition struct {
	Name    string
	Context Context
	Target  Target
	Items   []Item
}

// NewDefinition validates a definition and returns a canonical copy whose
// target identifiers are sorted.
func NewDefinition(in Definition) (Definition, error) {
	if !validText(in.Name, MaxNameScalars, true) {
		return Definition{}, ErrInvalid
	}
	if _, err := ParseContext(string(in.Context)); err != nil {
		return Definition{}, err
	}
	target, err := canonicalTarget(in.Target)
	if err != nil {
		return Definition{}, err
	}
	items, err := validItems(in.Items)
	if err != nil {
		return Definition{}, err
	}
	return Definition{Name: in.Name, Context: in.Context, Target: target, Items: items}, nil
}

// canonicalTarget copies a valid target with sorted identifiers.
func canonicalTarget(t Target) (Target, error) {
	if _, err := ParseTargetKind(string(t.Kind)); err != nil {
		return Target{}, err
	}
	if t.Kind == TargetOrganization {
		if len(t.IDs) != 0 {
			return Target{}, ErrInvalid
		}
		return Target{Kind: t.Kind}, nil
	}
	if len(t.IDs) < 1 || len(t.IDs) > MaxTargets {
		return Target{}, ErrInvalid
	}
	ids := slices.Clone(t.IDs)
	slices.Sort(ids)
	for i, id := range ids {
		if !CanonicalID(id) || i > 0 && ids[i-1] == id {
			return Target{}, ErrInvalid
		}
	}
	return Target{Kind: t.Kind, IDs: ids}, nil
}

func validItems(in []Item) ([]Item, error) {
	if len(in) < 1 || len(in) > MaxItems {
		return nil, ErrInvalid
	}
	for _, item := range in {
		if !validText(item.Label, MaxLabelScalars, true) || item.Critical && item.Optional {
			return nil, ErrInvalid
		}
	}
	return slices.Clone(in), nil
}

// CanonicalID reports a lowercase, dashed, non-nil UUID.
func CanonicalID(raw string) bool {
	if len(raw) != 36 || raw == "00000000-0000-0000-0000-000000000000" {
		return false
	}
	for i := 0; i < len(raw); i++ {
		b := raw[i]
		switch i {
		case 8, 13, 18, 23:
			if b != '-' {
				return false
			}
		default:
			if !(b >= '0' && b <= '9' || b >= 'a' && b <= 'f') {
				return false
			}
		}
	}
	return true
}

// validText: valid UTF-8, at most maximum scalars, no C0/C1 control; a
// required text, or any non-empty optional text, says something besides
// white space.
func validText(raw string, maximum int, required bool) bool {
	if !utf8.ValidString(raw) || utf8.RuneCountInString(raw) > maximum {
		return false
	}
	says := false
	for _, r := range raw {
		if unicode.IsControl(r) {
			return false
		}
		if !unicode.IsSpace(r) {
			says = true
		}
	}
	return says || !required && raw == ""
}

// Mode is the critical-item policy mode.
type Mode string

const (
	Block Mode = "BLOCK"
	Warn  Mode = "WARN"
)

func ParseMode(raw string) (Mode, error) {
	switch m := Mode(raw); m {
	case Block, Warn:
		return m, nil
	}
	return "", ErrInvalid
}

// Policy is one critical-item policy value.
type Policy struct {
	Mode            Mode
	ValiditySeconds int64
}

func NewPolicy(mode Mode, validitySeconds int64) (Policy, error) {
	p := Policy{Mode: mode, ValiditySeconds: validitySeconds}
	if err := p.Validate(); err != nil {
		return Policy{}, err
	}
	return p, nil
}

// DefaultPolicy is BLOCK with a 12-hour run validity.
func DefaultPolicy() Policy { return Policy{Mode: Block, ValiditySeconds: DefaultValiditySeconds} }

// Validate reports whether the policy is admissible.
func (p Policy) Validate() error {
	if _, err := ParseMode(string(p.Mode)); err != nil || p.ValiditySeconds < MinValiditySeconds || p.ValiditySeconds > MaxValiditySeconds {
		return ErrInvalid
	}
	return nil
}

// Answer is the value given to one item; Unanswered is the empty value.
type Answer string

const (
	Unanswered          Answer = ""
	AnswerOK            Answer = "OK"
	AnswerNotOK         Answer = "NOT_OK"
	AnswerNotApplicable Answer = "NOT_APPLICABLE"
)

// Response is the answer to one item with its optional note and photo
// declaration. The note is private field text and never formatted.
type Response struct {
	private
	Answer Answer
	Note   string
	Photo  bool
}

// Outcome is the computed result of a completed answer set (BR-CHECK-004).
type Outcome string

const (
	CompletedOK         Outcome = "COMPLETED_OK"
	CompletedWithIssues Outcome = "COMPLETED_WITH_ISSUES"
	Blocking            Outcome = "BLOCKING"
)

// Evaluation lists the 1-based positions answered NOT_OK (Issues) and the
// critical ones among them (Critical).
type Evaluation struct {
	Outcome                 Outcome
	Issues, Critical        []int
	AcknowledgementRequired bool
}

// Evaluate checks a complete answer set against the items and the policy and
// computes the outcome (B-BR-EMP-334).
func Evaluate(items []Item, policy Policy, responses []Response) (Evaluation, error) {
	if _, err := validItems(items); err != nil || policy.Validate() != nil || len(responses) != len(items) {
		return Evaluation{}, ErrInvalid
	}
	for _, r := range responses {
		switch r.Answer {
		case Unanswered, AnswerOK, AnswerNotOK, AnswerNotApplicable:
		default:
			return Evaluation{}, ErrInvalid
		}
		if !validText(r.Note, MaxNoteScalars, false) {
			return Evaluation{}, ErrInvalid
		}
	}
	notAllowed, photo, incomplete := false, false, false
	var out Evaluation
	for i, r := range responses {
		item := items[i]
		switch r.Answer {
		case AnswerNotApplicable:
			notAllowed = notAllowed || !item.Optional
		case AnswerNotOK:
			photo = photo || item.PhotoOnNotOK && !r.Photo
			out.Issues = append(out.Issues, i+1)
			if item.Critical {
				out.Critical = append(out.Critical, i+1)
			}
		case Unanswered:
			incomplete = true
		}
	}
	switch {
	case notAllowed:
		return Evaluation{}, ErrAnswerNotAllowed
	case photo:
		return Evaluation{}, ErrPhotoRequired
	case incomplete:
		return Evaluation{}, ErrIncomplete
	}
	switch {
	case len(out.Issues) == 0:
		out.Outcome = CompletedOK
	case len(out.Critical) > 0 && policy.Mode == Block:
		out.Outcome = Blocking
	default:
		out.Outcome = CompletedWithIssues
		out.AcknowledgementRequired = len(out.Critical) > 0
	}
	return out, nil
}

// Candidate is one ACTIVE template's current version, as resolution input.
type Candidate struct {
	TemplateID string
	Context    Context
	Target     Target
}

// Match names how the applicable template was found.
type Match string

const (
	MatchAsset        Match = "ASSET"
	MatchCategory     Match = "CATEGORY"
	MatchOrganization Match = "ORGANIZATION"
	MatchNone         Match = "NONE"
)

// Resolution is the applicable template; TemplateID is empty for MatchNone.
type Resolution struct {
	TemplateID string
	Match      Match
}

// Resolve applies BR-CHECK-002 to the ACTIVE candidates.
func Resolve(candidates []Candidate, context Context, assetID, categoryID string) (Resolution, error) {
	if _, err := ParseContext(string(context)); err != nil || !CanonicalID(assetID) || !CanonicalID(categoryID) {
		return Resolution{}, ErrInvalid
	}
	templates := map[string]bool{}
	named := map[[3]string]string{}
	for _, c := range candidates {
		if !CanonicalID(c.TemplateID) || templates[c.TemplateID] {
			return Resolution{}, ErrInvalid
		}
		templates[c.TemplateID] = true
		if _, err := ParseContext(string(c.Context)); err != nil {
			return Resolution{}, err
		}
		target, err := canonicalTarget(c.Target)
		if err != nil {
			return Resolution{}, err
		}
		keys := [][3]string{{string(c.Context), string(TargetOrganization), ""}}
		if target.Kind != TargetOrganization {
			keys = keys[:0]
			for _, id := range target.IDs {
				keys = append(keys, [3]string{string(c.Context), string(target.Kind), id})
			}
		}
		for _, key := range keys {
			if _, taken := named[key]; taken {
				return Resolution{}, ErrInvalid
			}
			named[key] = c.TemplateID
		}
	}
	for _, step := range []struct {
		key   [3]string
		match Match
	}{
		{[3]string{string(context), string(TargetAssets), assetID}, MatchAsset},
		{[3]string{string(context), string(TargetCategories), categoryID}, MatchCategory},
		{[3]string{string(context), string(TargetOrganization), ""}, MatchOrganization},
	} {
		if id, ok := named[step.key]; ok {
			return Resolution{TemplateID: id, Match: step.match}, nil
		}
	}
	return Resolution{Match: MatchNone}, nil
}

// PolicyRevision is one stored policy revision.
type PolicyRevision struct {
	Revision    int64
	Policy      Policy
	EffectiveAt time.Time
}

// PolicyAt returns the revision in force at the instant; false when none is.
func PolicyAt(history []PolicyRevision, at time.Time) (PolicyRevision, bool, error) {
	if !calendarInstant(at) {
		return PolicyRevision{}, false, ErrInvalid
	}
	var found PolicyRevision
	ok := false
	for i, r := range history {
		if r.Revision != int64(i+1) || r.Policy.Validate() != nil || !calendarInstant(r.EffectiveAt) || i > 0 && r.EffectiveAt.Before(history[i-1].EffectiveAt) {
			return PolicyRevision{}, false, ErrInvalid
		}
		if !r.EffectiveAt.After(at) {
			found, ok = r, true
		}
	}
	return found, ok, nil
}

func calendarInstant(at time.Time) bool {
	year := at.UTC().Year()
	return year >= 1 && year <= 9999
}

// private redacts the note of a response on every formatting surface.
type private struct{}

func (private) String() string               { return "checklist.Response(redacted)" }
func (p private) Format(f fmt.State, _ rune) { _, _ = io.WriteString(f, p.String()) }
func (private) MarshalJSON() ([]byte, error) { return []byte(`"checklist.Response(redacted)"`), nil }
func (p private) LogValue() slog.Value       { return slog.StringValue(p.String()) }
