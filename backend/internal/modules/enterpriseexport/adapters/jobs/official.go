package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	qualityapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
)

// OfficialPin is the frozen FQ-07/FQ-08 provenance used by the durable export
// state. PayloadSHA256 is filled only by Build; Pin intentionally does not read
// the payload during the no-op check.
type OfficialPin struct {
	RevisionID           string `json:"revision_id"`
	LeaseID              string `json:"lease_id"`
	Edition              string `json:"edition"`
	SourceURL            string `json:"source_url"`
	SourceChecksum       string `json:"source_checksum"`
	SourcePeriod         string `json:"source_period"`
	SourceClockPrecision string `json:"source_clock_precision"`
	SourceClockValue     string `json:"source_clock_value"`
	Input                int    `json:"input"`
	Accepted             int    `json:"accepted"`
	Quarantined          int    `json:"quarantined"`
	Rows                 int    `json:"rows"`
	Linked               int    `json:"linked"`
	Unmatched            int    `json:"unmatched"`
	Ambiguous            int    `json:"ambiguous"`
	WrongBranch          int    `json:"wrong_branch"`
	SuccessionReview     int    `json:"succession_review"`
	PayloadSHA256        string `json:"payload_sha256,omitempty"`
}

type OfficialBuild struct {
	Manifest []byte
	Pin      OfficialPin
}

type OfficialProvider interface {
	Pin(ctx context.Context) (*OfficialPin, error)
	Build(ctx context.Context, out io.Writer) (OfficialBuild, error)
}

type QualityOfficialProvider struct {
	reader qualityapp.ExportReader
}

func NewQualityOfficialProvider(reader qualityapp.ExportReader) *QualityOfficialProvider {
	return &QualityOfficialProvider{reader: reader}
}

func (p *QualityOfficialProvider) Pin(ctx context.Context) (*OfficialPin, error) {
	pin, err := p.reader.ExportPin(ctx)
	if errors.Is(err, qualityapp.ErrNoPublishedOfficialRecords) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &OfficialPin{
		RevisionID: pin.RevisionID, LeaseID: pin.LeaseID, Edition: pin.Edition,
		SourceURL: pin.SourceURL, SourceChecksum: pin.SourceChecksum,
		SourcePeriod: pin.SourcePeriod, SourceClockPrecision: pin.SourceClockPrecision,
		SourceClockValue: pin.SourceClockValue,
		Input:            pin.Counts.Input, Accepted: pin.Counts.Accepted, Quarantined: pin.Counts.Quarantined,
		Rows:      pin.Counts.Accepted,
		Linked:    pin.AssociationCensus.Linked,
		Unmatched: pin.AssociationCensus.Unmatched,
		Ambiguous: pin.AssociationCensus.Ambiguous, WrongBranch: pin.AssociationCensus.WrongBranch,
		SuccessionReview: pin.AssociationCensus.SuccessionReview,
	}, nil
}

func (p *QualityOfficialProvider) Build(ctx context.Context, out io.Writer) (OfficialBuild, error) {
	if out == nil {
		return OfficialBuild{}, fmt.Errorf("enterprise export: official writer required")
	}
	manifest, err := qualityapp.BuildOfficialExport(ctx, p.reader, out, 500)
	if err != nil {
		return OfficialBuild{}, err
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		return OfficialBuild{}, err
	}
	return OfficialBuild{
		Manifest: raw,
		Pin: OfficialPin{
			RevisionID: manifest.RevisionID, LeaseID: manifest.LeaseID, Edition: manifest.Edition,
			SourceURL: manifest.SourceURL, SourceChecksum: manifest.SourceChecksum,
			SourcePeriod: manifest.SourcePeriod, SourceClockPrecision: manifest.SourceClockPrecision,
			SourceClockValue: manifest.SourceClockValue,
			Input:            manifest.Counts.Input, Accepted: manifest.Counts.Accepted,
			Quarantined: manifest.Counts.Quarantined, Rows: manifest.Rows,
			Linked: manifest.AssociationCensus.Linked, Unmatched: manifest.AssociationCensus.Unmatched,
			Ambiguous: manifest.AssociationCensus.Ambiguous, WrongBranch: manifest.AssociationCensus.WrongBranch,
			SuccessionReview: manifest.AssociationCensus.SuccessionReview,
			PayloadSHA256:    manifest.PayloadSHA256,
		},
	}, nil
}

func sameOfficial(left, right *OfficialPin) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.RevisionID == right.RevisionID && left.LeaseID == right.LeaseID &&
		left.Edition == right.Edition && left.SourceChecksum == right.SourceChecksum &&
		left.Rows == right.Rows && left.Accepted == right.Accepted &&
		left.Linked == right.Linked && left.Unmatched == right.Unmatched &&
		left.Ambiguous == right.Ambiguous && left.WrongBranch == right.WrongBranch &&
		left.SuccessionReview == right.SuccessionReview
}
