package application

import (
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
)

// NativeFineClock freezes the source's precision even for an empty observation set.
type NativeFineClock struct {
	ReportYear      int
	ExtractionMonth string
}

// NativeFineInput is not convertible to legacy event input without an explicit
// semantic change. Source identity/provenance must be verified by the caller.
type NativeFineInput struct {
	Edition             string
	Predecessor         string
	Clock               NativeFineClock
	Resources           []Resource
	Counts              domain.Counts
	ManifestHash        string
	Periods             []PeriodInput
	CompanyAssociations *CompanyAssociationInput
	Observations        []FineObservation
}

func (in NativeFineInput) Validate() error {
	refuse := errors.New("quality: invalid native fine publication")
	year := in.Clock.ReportYear
	month, e := time.Parse("2006-01", in.Clock.ExtractionMonth)
	if year < 2016 || year > 9999 || in.Edition != strconv.Itoa(year) || e != nil || month.Format("2006-01") != in.Clock.ExtractionMonth || month.Year() < year {
		return refuse
	}
	if in.Counts.Accepted != len(in.Observations) || in.Counts.Input > 500000 || len(in.Resources) != 1 {
		return refuse
	}
	for _, n := range []int{in.Counts.Input, in.Counts.Accepted, in.Counts.Duplicates, in.Counts.Quarantined} {
		if n < 0 || n > 500000 {
			return refuse
		}
	}
	manifest := domain.Manifest{Source: "sanctions", Stream: domain.StreamSanction, Edition: in.Edition, Predecessor: in.Predecessor, Counts: in.Counts}
	for _, r := range in.Resources {
		if r.ValidateMember() != nil || r.Member != "" || r.URL == "" || len(r.URL) > 1024 || r.Bytes == 0 || r.Bytes > 100<<20 || len(r.SHA256) != 64 || !isHex(r.SHA256) || r.SHA256 != strings.ToLower(r.SHA256) {
			return refuse
		}
		manifest.Resources = append(manifest.Resources, domain.Resource{URL: r.URL, SHA256: r.SHA256, Bytes: r.Bytes})
	}
	if manifest.Reconcile() != nil || len(in.ManifestHash) != 64 || !isHex(in.ManifestHash) || ValidatePeriods(in.Periods) != nil {
		return refuse
	}
	text := func(s string, max int) bool {
		return len(s) > 0 && len(s) <= max && utf8.ValidString(s) && strings.TrimSpace(s) != "" && strings.IndexFunc(s, unicode.IsControl) < 0
	}
	seen := map[string]bool{}
	for _, row := range in.Observations {
		hash, ok := strings.CutPrefix(row.ObservationKey, "observation:")
		cnpj, err := kernel.ParseCNPJ(row.SourceCNPJ)
		if !ok || len(hash) != 64 || !isHex(hash) || hash != strings.ToLower(hash) || seen[row.ObservationKey] || err != nil || cnpj.Normalized() != row.SourceCNPJ || row.ReportYear != year || row.ExtractionMonth != in.Clock.ExtractionMonth || !domain.ValidDay(row.DueOn) {
			return refuse
		}
		seen[row.ObservationKey] = true
		for _, s := range []string{row.ProceedingRef, row.DUFRef, row.Department, row.Status, row.AmountText, row.PaidText} {
			if !text(s, 64<<10) {
				return refuse
			}
		}
		if !text(row.EvidenceRef, 512) {
			return refuse
		}
		amount, ok := domain.ParseFineAmount(row.AmountText)
		if !ok || amount != row.AmountMinor {
			return refuse
		}
		paid, ok := domain.ParseFineAmount(row.PaidText)
		if !ok || paid != row.PaidMinor || row.ObservationIdentity() != row.ObservationKey {
			return refuse
		}
	}
	if in.CompanyAssociations != nil {
		sources := make(map[string]string, len(in.Observations))
		for _, row := range in.Observations {
			sources[row.ObservationKey] = row.SourceCNPJ
		}
		if _, e := in.CompanyAssociations.Bindings(sources); e != nil {
			return e
		}
	}
	return nil
}
