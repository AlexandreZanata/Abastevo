package application

import (
	"errors"
	"strconv"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
)

type FineHistoryContext struct {
	InputContract     string
	SourcePeriod      string
	SourceFile        string
	HeaderRecord      int
	Clock             FineHistoryClock
	SourceRecordCount int
	MetadataRecords   int
}

func (c FineHistoryContext) Valid() bool {
	p, ok := HistoricalFineProfile(c.SourcePeriod)
	return ok && c.InputContract == p.Contract && c.SourceFile == p.SourceFile && c.HeaderRecord == p.HeaderRecord && c.Clock.Valid(c.SourcePeriod) && c.SourceRecordCount >= 0 && c.SourceRecordCount <= 500000 && c.MetadataRecords >= 0 && c.MetadataRecords <= c.SourceRecordCount && ((c.SourcePeriod == "2020" || c.SourcePeriod == "2021") || c.MetadataRecords == 0)
}
func (c FineHistoryContext) Record(locator string) (int, bool) {
	suffix, ok := strings.CutPrefix(locator, c.SourceFile+":")
	n, e := strconv.Atoi(suffix)
	return n, ok && e == nil && strconv.Itoa(n) == suffix && n > c.HeaderRecord && n <= c.HeaderRecord+c.SourceRecordCount
}

type HistoricalFineInput struct {
	Context      FineHistoryContext
	Predecessor  string
	Resources    []Resource
	Counts       domain.Counts
	ManifestHash string
	Observations []HistoricalFineObservation
	Notes        []FineHistoryNote
}

func (in HistoricalFineInput) Validate() error {
	refused := errors.New("quality: invalid historical fine publication")
	if !in.Context.Valid() || len(in.Resources) != 1 || in.Counts.Accepted != len(in.Observations) || in.Counts.Input != in.Context.SourceRecordCount || len(in.Notes) != in.Context.MetadataRecords || in.Context.MetadataRecords > in.Counts.Quarantined {
		return refused
	}
	for _, n := range []int{in.Counts.Input, in.Counts.Accepted, in.Counts.Duplicates, in.Counts.Quarantined} {
		if n < 0 || n > 500000 {
			return refused
		}
	}
	resource := in.Resources[0]
	if resource.ValidateMember() != nil || resource.Member != "" || resource.URL != "https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/mav/"+in.Context.SourceFile || resource.Bytes == 0 || resource.Bytes > 100<<20 || len(resource.SHA256) != 64 || !isHex(resource.SHA256) || resource.SHA256 != strings.ToLower(resource.SHA256) || len(in.ManifestHash) != 64 || !isHex(in.ManifestHash) || in.ManifestHash != strings.ToLower(in.ManifestHash) {
		return refused
	}
	manifest := domain.Manifest{Source: "sanctions", Stream: domain.StreamSanction, Edition: in.Context.SourcePeriod, Counts: in.Counts, Resources: []domain.Resource{{URL: resource.URL, SHA256: resource.SHA256, Bytes: resource.Bytes}}}
	if manifest.Reconcile() != nil {
		return refused
	}
	keys, locators := map[string]bool{}, map[int]bool{}
	lastObservation := 0
	for _, row := range in.Observations {
		n, ok := in.Context.Record(row.EvidenceRef)
		if !ok || locators[n] || !row.Valid() || row.SourcePeriod != in.Context.SourcePeriod || row.InputContract != in.Context.InputContract || row.Clock != in.Context.Clock || keys[row.ObservationKey] {
			return refused
		}
		keys[row.ObservationKey] = true
		locators[n] = true
		lastObservation = max(lastObservation, n)
	}
	for _, note := range in.Notes {
		n, ok := in.Context.Record(note.EvidenceRef)
		if !ok || locators[n] || n <= lastObservation || !note.Valid(in.Context.SourcePeriod) {
			return refused
		}
		locators[n] = true
	}
	return nil
}
