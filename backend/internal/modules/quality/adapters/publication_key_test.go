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
