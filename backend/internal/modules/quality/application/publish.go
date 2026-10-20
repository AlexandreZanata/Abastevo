package application

import (
	"errors"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
)

// ErrSupersededPublication refuses late retries that would resurrect a corrected
// or withdrawn source publication. Consumers acknowledge without watermark drift.
var ErrSupersededPublication = errors.New("quality: publication already superseded")

// Resource binds one raw artifact of a source edition.
type Resource struct {
	Member       string `json:"Member,omitempty"`
	MemberSHA256 string `json:"MemberSHA256,omitempty"`
	MemberBytes  uint64 `json:"MemberBytes,omitempty"`
	URL          string
	SHA256       string
	Bytes        uint64
}

// AssayInput is one staged determination. Identity and grouping follow the
// domain contracts; the publisher persists them verbatim.
type AssayInput struct {
	SampleRef        string
	AssayRef         string
	SourceCNPJ       string
	ObservedOn       string
	SourceObservedAt string
	Fuel             string
	ResultText       string
	Unit             string
	Conformity       string
	EvidenceRef      string
	Fingerprint      string
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
	Periods      []PeriodInput
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
		if err := r.ValidateMember(); err != nil {
			return err
		}
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
	if err := ValidatePeriods(in.Periods); err != nil {
		return err
	}
	for _, a := range in.Assays {
		if a.SourceObservedAt != "" {
			day, ok := domain.SourceCollectionDay(a.SourceObservedAt)
			if !ok || len(a.SourceObservedAt) != 19 || day != a.ObservedOn {
				return errors.New("quality: invalid source collection clock")
			}
		}
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

// ValidateMember binds an optional public archive member without changing old
// memberless publication-key serialization. It never accepts filesystem paths.
func (r Resource) ValidateMember() error {
	if r.Member == "" {
		if r.MemberSHA256 != "" || r.MemberBytes != 0 {
			return errors.New("quality: incomplete member identity")
		}
		return nil
	}
	if !utf8.ValidString(r.Member) || len(r.Member) > 256 || path.IsAbs(r.Member) || path.Clean(r.Member) != r.Member || strings.ContainsAny(r.Member, "\\\x00") || r.Member == "." || strings.IndexFunc(r.Member, unicode.IsControl) >= 0 || r.Member == ".." || strings.HasPrefix(r.Member, "../") || len(r.MemberSHA256) != 64 || !isHex(r.MemberSHA256) || r.MemberSHA256 != strings.ToLower(r.MemberSHA256) || r.MemberBytes == 0 || r.MemberBytes > 100<<20 {
		return errors.New("quality: invalid member identity")
	}
	return nil
}
