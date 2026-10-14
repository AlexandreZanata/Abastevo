package application

import (
	"errors"
	"strings"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
)

// Resource binds one raw artifact of a source edition.
type Resource struct {
	URL    string
	SHA256 string
	Bytes  uint64
}

// AssayInput is one staged determination. Identity and grouping follow the
// domain contracts; the publisher persists them verbatim.
type AssayInput struct {
	SampleRef   string
	AssayRef    string
	SourceCNPJ  string
	ObservedOn  string
	Fuel        string
	ResultText  string
	Unit        string
	Conformity  string
	EvidenceRef string
	Fingerprint string
}

// LinkInput is one staged station association. StationID is empty for
// unmatched/review records; no input creates a station.
type LinkInput struct {
	RecordKey string
	StationID string
	Automatic bool
	Reason    string
	Active    bool
}

// Input is one atomic publication request.
type Input struct {
	Source       string
	Stream       string
	Edition      string
	Predecessor  string
	Resources    []Resource
	Counts       domain.Counts
	ManifestHash string
	Assays       []AssayInput
	Links        []LinkInput
}

var (
	ErrEmptyResources = errors.New("quality: publication binds no resources")
	ErrCountsMismatch = errors.New("quality: counts do not reconcile")
	ErrHashShape      = errors.New("quality: manifest hash must be 64 hex characters")
)

// Validate enforces the publication contract before any write: resources
// are bound, the manifest reconciles through the domain rule, and every
// staged assay carries the identity the ledger keys on. Fuel and
// conformity enums stay with the database CHECKs so storage refuses what
// preparation let through.
func (in Input) Validate() error {
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
	for _, a := range in.Assays {
		if strings.TrimSpace(a.SampleRef) == "" || strings.TrimSpace(a.AssayRef) == "" {
			return domain.ErrMissingIdentifier
		}
		if len(a.Fingerprint) != 64 || !isHex(a.Fingerprint) {
			return ErrHashShape
		}
	}
	return nil
}

func isHex(s string) bool {
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

// Publisher persists one validated Input atomically. Implementations run
// staging, reconciliation, publication and the current-pointer switch in a
// single transaction; replaying an identical manifest returns the existing
// published revision without new rows.
type Publisher interface {
	Publish(input Input, newID func() string) (revisionID string, err error)
}
