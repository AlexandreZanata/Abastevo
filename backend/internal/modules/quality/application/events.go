package application

import (
	"errors"
	"strings"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
)

// PeriodInput claims one covered period of an edition. Labels stay
// visible even when coverage is missing or duplicated; FQ-05 discovery
// resolves gaps against these claims.
type PeriodInput struct {
	Label string
	Start string
	End   string
}

// EnforcementInput is one staged fiscal-action record. Scope and status
// stay source-verbatim: FQ-06 owns the classification vocabulary, so this
// contract never invents procedural meaning.
type EnforcementInput struct {
	NativeKey   string
	Scope       string
	Status      string
	SubjectCNPJ string
	OccurredOn  string
	AsOf        string
	EvidenceRef string
}

// SanctionInput is one staged fine/proceeding snapshot. Status is
// extraction-time only; finality and appeals belong to FQ-07.
type SanctionInput struct {
	NativeKey     string
	ProceedingRef string
	PartyCNPJ     string
	AmountMinor   *int64
	Status        string
	DecidedOn     string
	AsOf          string
	EvidenceRef   string
}

// EventsInput is one atomic enforcement/sanction publication request. It
// shares editions, manifests, revisions and the read lease with assay
// publications, scoped per source/stream.
type EventsInput struct {
	Source       string
	Stream       string
	Edition      string
	Predecessor  string
	Resources    []Resource
	Counts       domain.Counts
	ManifestHash string
	Periods      []PeriodInput
	Enforcement  []EnforcementInput
	Sanctions    []SanctionInput
}

var ErrUnknownEventSource = errors.New("quality: event source must be enforcement or sanctions")

// Validate enforces the event publication contract before any write. Row
// identity and period order are checked here; procedural scope/status
// enums stay with the database CHECKs and their owning tasks.
func (in EventsInput) Validate() error {
	if in.Source != "enforcement" && in.Source != "sanctions" {
		return ErrUnknownEventSource
	}
	if len(in.Resources) == 0 {
		return ErrEmptyResources
	}
	manifest := domain.Manifest{
		Source: in.Source, Stream: in.Stream, Edition: in.Edition,
		Counts: in.Counts, Predecessor: in.Predecessor,
	}
	for _, r := range in.Resources {
		manifest.Resources = append(manifest.Resources, domain.Resource{
			URL: r.URL, SHA256: r.SHA256, Bytes: r.Bytes,
		})
	}
	if err := manifest.Reconcile(); err != nil {
		return ErrCountsMismatch
	}
	if len(in.ManifestHash) != 64 || !isHex(in.ManifestHash) {
		return ErrHashShape
	}
	for _, p := range in.Periods {
		if strings.TrimSpace(p.Label) == "" || !domain.ValidDay(p.Start) || !domain.ValidDay(p.End) || p.End < p.Start {
			return errors.New("quality: malformed edition period")
		}
	}
	for _, e := range in.Enforcement {
		if strings.TrimSpace(e.NativeKey) == "" {
			return domain.ErrMissingIdentifier
		}
	}
	for _, s := range in.Sanctions {
		if strings.TrimSpace(s.NativeKey) == "" {
			return domain.ErrMissingIdentifier
		}
	}
	return nil
}
