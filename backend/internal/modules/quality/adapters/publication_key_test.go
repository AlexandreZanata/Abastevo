package adapters

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
)

func TestMemberlessPublicationKeyRemainsByteCompatible(t *testing.T) {
	type priorResource struct {
		URL, SHA256 string
		Bytes       uint64
	}
	req := editionRequest{Source: "pmqc", Stream: "pmqc-assay-v1", Edition: "2026-09", ManifestHash: "test", Counts: domain.Counts{Input: 1, Accepted: 1}, Resources: []application.Resource{{URL: "https://example.invalid/p.zip", SHA256: "hash", Bytes: 10}}}
	old, _ := json.Marshal(struct {
		Source, Stream, Edition, Manifest string
		Counts                            domain.Counts
		Resources                         []priorResource
		Periods                           []application.PeriodInput
		Links                             []application.LinkInput
	}{req.Source, req.Stream, req.Edition, req.ManifestHash, req.Counts, []priorResource{{req.Resources[0].URL, req.Resources[0].SHA256, req.Resources[0].Bytes}}, nil, nil})
	sum := sha256.Sum256(old)
	if got := req.publicationKey(); got != hex.EncodeToString(sum[:]) {
		t.Fatal("memberless retry key changed", got)
	}
	req.Resources[0].Member = "2026/PMQC_2026_09.csv"
	if req.publicationKey() == hex.EncodeToString(sum[:]) {
		t.Fatal("member excluded from key")
	}
}

func TestNativeClockPublicationKeyRemainsCompatibleBeforeHistory(t *testing.T) {
	clock := &application.NativeFineClock{ReportYear: 2025, ExtractionMonth: "2026-01"}
	req := editionRequest{Source: "sanctions", Stream: "sanction-v1", Edition: "2025", ManifestHash: "source-manifest", Counts: domain.Counts{Input: 1, Accepted: 1}, Resources: []application.Resource{{URL: "https://example.invalid/f.csv", SHA256: "hash", Bytes: 100}}, NativeFineClock: clock}
	old, _ := json.Marshal(struct {
		Source, Stream, Edition, Manifest string
		Counts                            domain.Counts
		Resources                         []application.Resource
		Periods                           []application.PeriodInput
		Links                             []application.LinkInput
		NativeFineClock                   *application.NativeFineClock `json:",omitempty"`
	}{req.Source, req.Stream, req.Edition, req.ManifestHash, req.Counts, req.Resources, nil, nil, clock})
	sum := sha256.Sum256(old)
	if req.publicationKey() != hex.EncodeToString(sum[:]) {
		t.Fatal("native2025 publication key changed")
	}
	req.NativeFineClock = nil
	req.HistoricalFineContext = &application.FineHistoryContext{SourcePeriod: "2023", Clock: application.FineHistoryClock{Precision: "MONTH", Value: "2024-06"}}
	before := req.publicationKey()
	req.HistoricalFineContext.Clock.Value = "2024-07"
	if req.publicationKey() == before {
		t.Fatal("historical source clock omitted from frozen identity")
	}
}

func TestCompanyPublicationKeyPreservesOldHistoryAndFreezesOnlyIdentityFacts(t *testing.T) {
	c := &application.FineHistoryContext{SourcePeriod: "2024", Clock: application.FineHistoryClock{Precision: "MONTH", Value: "2025-03"}}
	req := editionRequest{Source: "sanctions", Stream: "sanction-v1", Edition: "2024", ManifestHash: "same-source", HistoricalFineContext: c}
	prior, _ := json.Marshal(struct {
		Source, Stream, Edition, Manifest string
		Counts                            domain.Counts
		Resources                         []application.Resource
		Periods                           []application.PeriodInput
		Links                             []application.LinkInput
		NativeFineClock                   *application.NativeFineClock    `json:",omitempty"`
		HistoricalFineContext             *application.FineHistoryContext `json:",omitempty"`
	}{req.Source, req.Stream, req.Edition, req.ManifestHash, req.Counts, nil, nil, nil, nil, c})
	sum := sha256.Sum256(prior)
	old := hex.EncodeToString(sum[:])
	if req.publicationKey() != old {
		t.Fatal("history key changed without company evidence")
	}
	req.CompanyAssociations = &application.CompanyAssociationInput{PolicyVersion: domain.CompanyObservationPolicyVersion, DecisionDay: "2026-10-20", Decisions: []domain.CompanyObservationDecision{{RecordKey: "source-a", Kind: domain.ResolutionUnmatched, Reason: domain.ReasonUnmatchedID}}}
	declared := req.publicationKey()
	if declared == old {
		t.Fatal("explicit company policy omitted")
	}
	req.CompanyAssociations.DecisionDay = "2026-10-21"
	req.CompanyAssociations.Decisions[0].DecidedAt = "2026-10-21"
	if req.publicationKey() != declared {
		t.Fatal("operational day entered frozen identity")
	}
	req.CompanyAssociations.Decisions[0].Kind = domain.ResolutionAmbiguous
	req.CompanyAssociations.Decisions[0].Reason = domain.ReasonAmbiguous
	if req.publicationKey() == declared {
		t.Fatal("company verdict excluded from frozen identity")
	}
}
