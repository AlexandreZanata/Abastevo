package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
)

const (
	OfficialExportSchemaVersion     = "quality-official-export-v1"
	OfficialExportSource            = "sanctions"
	OfficialExportStream            = domain.StreamSanction
	OfficialExportScope             = "sanction"
	OfficialExportPublicFieldPolicy = "fq-public-v1"
	MinOfficialExportLimit          = 1
	MaxOfficialExportLimit          = 1000
	defaultOfficialExportLimit      = 500
)

var (
	ErrOfficialExportUnavailable = errors.New("quality: official export unavailable")
	ErrOfficialExportCursor      = errors.New("quality: official export cursor")
	ErrOfficialExportIncomplete  = errors.New("quality: official export incomplete")
)

// AssociationState is the public snapshot vocabulary. Exact current-company
// bindings are linked; wrong-branch and succession-review evidence is exported
// as unmatched with its original reason rather than guessed or hidden.
type AssociationState string

const (
	AssociationLinked    AssociationState = "linked"
	AssociationUnmatched AssociationState = "unmatched"
	AssociationAmbiguous AssociationState = "ambiguous"
)

// OfficialRecord is one allowlisted event-level sanction observation in a
// frozen export. OccurredOn stays empty because the source supplies no incident
// date; decision and due dates are separate fields. CPF, payment notes,
// department, internal history IDs and unlabelled source fields never enter it.
type OfficialRecord struct {
	NativeKey         string `json:"native_key"`
	SubjectCNPJ       string `json:"subject_cnpj"`
	Scope             string `json:"scope"`
	Status            string `json:"status"`
	OccurredOn        string `json:"occurred_on,omitempty"`
	AsOf              string `json:"as_of,omitempty"`
	Evidence          string `json:"evidence"`
	StationID         string `json:"station_id,omitempty"`
	AssociationState  string `json:"association_state"`
	AssociationReason string `json:"association_reason"`
	RevisionID        string `json:"revision_id"`
	LeaseID           string `json:"lease_id"`
	SourceURL         string `json:"source_url"`
	SourceChecksum    string `json:"source_checksum"`
	ProceedingRef     string `json:"proceeding_ref"`
	AmountMinor       *int64 `json:"amount_minor,omitempty"`
	AmountText        string `json:"amount_text,omitempty"`
	DecidedOn         string `json:"decided_on,omitempty"`
	DueOn             string `json:"due_on,omitempty"`
	SourceContract    string `json:"source_contract"`
	SourcePeriod      string `json:"source_period"`
	ExtractionMonth   string `json:"extraction_month,omitempty"`
	ReportYear        *int32 `json:"report_year,omitempty"`
}

// AssociationCensus freezes every stored resolution kind, including the two
// review-only kinds that the public record vocabulary reports as unmatched.
type AssociationCensus struct {
	Total            int `json:"total"`
	Linked           int `json:"linked"`
	Unmatched        int `json:"unmatched"`
	Ambiguous        int `json:"ambiguous"`
	WrongBranch      int `json:"wrong_branch"`
	SuccessionReview int `json:"succession_review"`
}

type OfficialExportCounts struct {
	Input       int `json:"input"`
	Accepted    int `json:"accepted"`
	Duplicates  int `json:"duplicates"`
	Quarantined int `json:"quarantined"`
}

// OfficialExportPin is the immutable descriptor the static export builder
// pins before reading records. LeaseID is the revision publication key, not a
// mutable revision selector.
type OfficialExportPin struct {
	SchemaVersion          string               `json:"schema_version"`
	PublicFieldPolicy      string               `json:"public_field_policy"`
	Source                 string               `json:"source"`
	Stream                 string               `json:"stream"`
	Edition                string               `json:"edition"`
	RevisionID             string               `json:"revision_id"`
	LeaseID                string               `json:"lease_id"`
	PublishedAt            string               `json:"published_at"`
	Status                 string               `json:"status"`
	ProvenanceState        string               `json:"provenance_state"`
	SourceURL              string               `json:"source_url"`
	SourceChecksum         string               `json:"source_checksum"`
	SourceBytes            int64                `json:"source_bytes"`
	SourcePeriod           string               `json:"source_period,omitempty"`
	SourceClockPrecision   string               `json:"source_clock_precision,omitempty"`
	SourceClockValue       string               `json:"source_clock_value,omitempty"`
	Counts                 OfficialExportCounts `json:"counts"`
	AssociationPolicy      string               `json:"association_policy"`
	AssociationScope       string               `json:"association_scope"`
	AssociationDecisionDay string               `json:"association_decision_day"`
	AssociationCensus      AssociationCensus    `json:"association_census"`
	Stale                  bool                 `json:"stale"`
	StaleReason            string               `json:"stale_reason,omitempty"`
}

// OfficialExportManifest adds the completed payload identity to the pin.
type OfficialExportManifest struct {
	OfficialExportPin
	Rows          int    `json:"rows"`
	PayloadSHA256 string `json:"payload_sha256"`
}

// ExportReader is the read-only Quality port consumed by the static builder.
// ExportPin and ExportSanctions must resolve the same current published
// revision; a changed revision invalidates any previous cursor.
type ExportReader interface {
	ExportPin(ctx context.Context) (OfficialExportPin, error)
	ExportSanctions(ctx context.Context, after string, limit int) ([]OfficialRecord, string, error)
}

func ValidateOfficialExportLimit(limit int) error {
	if limit < MinOfficialExportLimit || limit > MaxOfficialExportLimit {
		return fmt.Errorf("%w: limit", ErrOfficialExportCursor)
	}
	return nil
}

// SealOfficialExportCursor binds the keyset position to one revision. Native
// keys may contain '|', so only the first separator is structural.
func SealOfficialExportCursor(revisionID, nativeKey string) string {
	return revisionID + "|" + nativeKey
}

func OpenOfficialExportCursor(cursor string) (string, string, error) {
	if cursor == "" {
		return "", "", nil
	}
	revision, key, ok := strings.Cut(cursor, "|")
	if !ok || !isUUID(revision) || key == "" || len(key) > 256 {
		return "", "", ErrOfficialExportCursor
	}
	return revision, key, nil
}

func AssociationStateFor(kind string) (AssociationState, error) {
	switch kind {
	case "bound":
		return AssociationLinked, nil
	case "unmatched", "wrong_branch", "succession_review":
		return AssociationUnmatched, nil
	case "ambiguous":
		return AssociationAmbiguous, nil
	default:
		return "", ErrOfficialExportUnavailable
	}
}

// ValidateOfficialRecord enforces the public allowlist and the association
// vocabulary at the export boundary: no CPF-shaped identifier, no station on a
// review outcome, no invented date.
func ValidateOfficialRecord(record OfficialRecord) error {
	refuse := func() error { return fmt.Errorf("%w: record", ErrOfficialExportIncomplete) }
	if record.NativeKey == "" || len(record.NativeKey) > 256 || record.Evidence == "" || len(record.Evidence) > 512 {
		return refuse()
	}
	id, err := kernel.ParseCNPJ(record.SubjectCNPJ)
	if err != nil || id.Normalized() != record.SubjectCNPJ {
		return refuse()
	}
	if record.Scope != OfficialExportScope || record.Status == "" || record.SourceContract == "" || record.SourcePeriod == "" || len(record.SourceChecksum) != 64 || !isHex(record.SourceChecksum) || record.SourceURL == "" || len(record.SourceURL) > 1024 || record.RevisionID == "" || record.LeaseID == "" {
		return refuse()
	}
	switch record.AssociationState {
	case string(AssociationLinked):
		if record.StationID == "" || !isUUID(record.StationID) {
			return refuse()
		}
	case string(AssociationUnmatched), string(AssociationAmbiguous):
		if record.StationID != "" {
			return refuse()
		}
	default:
		return refuse()
	}
	if record.AssociationReason == "" {
		return refuse()
	}
	for _, day := range []string{record.OccurredOn, record.AsOf, record.DecidedOn, record.DueOn} {
		if day != "" && !domain.ValidDay(day) {
			return refuse()
		}
	}
	if record.AmountMinor != nil && *record.AmountMinor < 0 {
		return refuse()
	}
	if record.ReportYear != nil && (*record.ReportYear < 2016 || *record.ReportYear > 9999) {
		return refuse()
	}
	return nil
}

// BuildOfficialExport traverses the current published revision exactly once,
// writes deterministic JSONL and returns the frozen manifest with the SHA-256
// of the exact bytes written. Rows, association census and accepted count must
// agree; partial or repeated traversal fails closed.
func BuildOfficialExport(ctx context.Context, reader ExportReader, out io.Writer, limit int) (OfficialExportManifest, error) {
	if reader == nil || out == nil {
		return OfficialExportManifest{}, fmt.Errorf("%w: ports", ErrOfficialExportUnavailable)
	}
	if limit == 0 {
		limit = defaultOfficialExportLimit
	}
	if err := ValidateOfficialExportLimit(limit); err != nil {
		return OfficialExportManifest{}, err
	}
	pin, err := reader.ExportPin(ctx)
	if err != nil {
		return OfficialExportManifest{}, err
	}
	if pin.Counts.Accepted < 0 || pin.Counts.Accepted > 500000 || pin.AssociationCensus.Total != pin.Counts.Accepted {
		return OfficialExportManifest{}, fmt.Errorf("%w: census", ErrOfficialExportIncomplete)
	}
	manifest := OfficialExportManifest{OfficialExportPin: pin}
	hash := sha256.New()
	encoder := json.NewEncoder(io.MultiWriter(out, hash))
	encoder.SetEscapeHTML(false)
	seen := make(map[string]bool, pin.Counts.Accepted)
	after := ""
	for {
		records, next, err := reader.ExportSanctions(ctx, after, limit)
		if err != nil {
			return OfficialExportManifest{}, err
		}
		for _, record := range records {
			if seen[record.NativeKey] || len(seen) >= pin.Counts.Accepted {
				return OfficialExportManifest{}, fmt.Errorf("%w: traversal", ErrOfficialExportIncomplete)
			}
			if err := ValidateOfficialRecord(record); err != nil {
				return OfficialExportManifest{}, err
			}
			if record.RevisionID != pin.RevisionID || record.LeaseID != pin.LeaseID {
				return OfficialExportManifest{}, fmt.Errorf("%w: revision", ErrOfficialExportIncomplete)
			}
			seen[record.NativeKey] = true
			if err := encoder.Encode(record); err != nil {
				return OfficialExportManifest{}, err
			}
		}
		if next == "" {
			break
		}
		if next == after {
			return OfficialExportManifest{}, fmt.Errorf("%w: cursor", ErrOfficialExportIncomplete)
		}
		after = next
	}
	if len(seen) != pin.Counts.Accepted {
		return OfficialExportManifest{}, fmt.Errorf("%w: rows", ErrOfficialExportIncomplete)
	}
	manifest.Rows = len(seen)
	manifest.PayloadSHA256 = hex.EncodeToString(hash.Sum(nil))
	return manifest, nil
}
