package portalgen

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

const (
	officialSchema     = "quality-official-export-v1"
	officialPageSize   = 200
	officialStationCap = 20
)

type officialCounts struct {
	Input       int `json:"input"`
	Accepted    int `json:"accepted"`
	Duplicates  int `json:"duplicates"`
	Quarantined int `json:"quarantined"`
}

type officialCensus struct {
	Total            int `json:"total"`
	Linked           int `json:"linked"`
	Unmatched        int `json:"unmatched"`
	Ambiguous        int `json:"ambiguous"`
	WrongBranch      int `json:"wrong_branch"`
	SuccessionReview int `json:"succession_review"`
}

type officialManifest struct {
	SchemaVersion          string         `json:"schema_version"`
	PublicFieldPolicy      string         `json:"public_field_policy"`
	Source                 string         `json:"source"`
	Stream                 string         `json:"stream"`
	Edition                string         `json:"edition"`
	RevisionID             string         `json:"revision_id"`
	LeaseID                string         `json:"lease_id"`
	PublishedAt            string         `json:"published_at"`
	Status                 string         `json:"status"`
	ProvenanceState        string         `json:"provenance_state"`
	SourceURL              string         `json:"source_url"`
	SourceChecksum         string         `json:"source_checksum"`
	SourceBytes            int64          `json:"source_bytes"`
	SourcePeriod           string         `json:"source_period"`
	SourceClockPrecision   string         `json:"source_clock_precision"`
	SourceClockValue       string         `json:"source_clock_value"`
	Counts                 officialCounts `json:"counts"`
	AssociationPolicy      string         `json:"association_policy"`
	AssociationScope       string         `json:"association_scope"`
	AssociationDecisionDay string         `json:"association_decision_day"`
	AssociationCensus      officialCensus `json:"association_census"`
	Stale                  bool           `json:"stale"`
	StaleReason            string         `json:"stale_reason"`
	Rows                   int            `json:"rows"`
	PayloadSHA256          string         `json:"payload_sha256"`
}

type officialPin struct {
	SchemaVersion          string         `json:"schema_version"`
	PublicFieldPolicy      string         `json:"public_field_policy"`
	Source                 string         `json:"source"`
	Stream                 string         `json:"stream"`
	Edition                string         `json:"edition"`
	RevisionID             string         `json:"revision_id"`
	LeaseID                string         `json:"lease_id"`
	PublishedAt            string         `json:"published_at"`
	Status                 string         `json:"status"`
	ProvenanceState        string         `json:"provenance_state"`
	SourceURL              string         `json:"source_url"`
	SourceChecksum         string         `json:"source_checksum"`
	SourcePeriod           string         `json:"source_period"`
	SourceClockPrecision   string         `json:"source_clock_precision"`
	SourceClockValue       string         `json:"source_clock_value"`
	Counts                 officialCounts `json:"counts"`
	AssociationPolicy      string         `json:"association_policy"`
	AssociationScope       string         `json:"association_scope"`
	AssociationDecisionDay string         `json:"association_decision_day"`
	AssociationCensus      officialCensus `json:"association_census"`
	Rows                   int            `json:"rows"`
	PayloadSHA256          string         `json:"payload_sha256"`
}

type officialRecord struct {
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

type officialDataset struct {
	Manifest  officialManifest
	Records   []officialRecord
	ByStation map[string][]officialRecord
	Pages     int
}

type officialRecordView struct {
	RecordID          string
	NativeKey         string
	CNPJ              string
	Status            string
	Proceeding        string
	Amount            string
	DecidedOn         string
	DueOn             string
	AsOf              string
	OccurredOn        string
	ExtractionMonth   string
	ReportYear        string
	SourcePeriod      string
	AssociationState  string
	AssociationLabel  string
	AssociationReason string
	StationName       string
	StationHref       string
	SourceURL         string
	SourceChecksum    string
	Evidence          string
	RevisionID        string
	LeaseID           string
}

type fiscalizacaoPage struct {
	pageBase
	HasData    bool
	Manifest   officialPin
	Records    []officialRecordView
	Page       int
	Pages      int
	PrevHref   string
	NextHref   string
	Linked     int
	Unmatched  int
	Ambiguous  int
	Total      int
	SourceInfo string
}

type fiscalizacaoRecordPage struct {
	pageBase
	Record   officialRecordView
	Manifest officialPin
}

func (m officialManifest) pin() officialPin {
	return officialPin{
		SchemaVersion: m.SchemaVersion, PublicFieldPolicy: m.PublicFieldPolicy,
		Source: m.Source, Stream: m.Stream,
		Edition: m.Edition, RevisionID: m.RevisionID, LeaseID: m.LeaseID,
		PublishedAt: m.PublishedAt, Status: m.Status, ProvenanceState: m.ProvenanceState,
		SourceURL: m.SourceURL, SourceChecksum: m.SourceChecksum,
		SourcePeriod: m.SourcePeriod, SourceClockPrecision: m.SourceClockPrecision,
		SourceClockValue: m.SourceClockValue, Counts: m.Counts,
		AssociationPolicy: m.AssociationPolicy, AssociationScope: m.AssociationScope,
		AssociationDecisionDay: m.AssociationDecisionDay,
		AssociationCensus:      m.AssociationCensus, Rows: m.Rows, PayloadSHA256: m.PayloadSHA256,
	}
}

func combinedInputHash(exportHash string, official *officialDataset) string {
	if official == nil {
		return exportHash
	}
	sum := sha256.Sum256([]byte(exportHash + "|" + official.Manifest.RevisionID + "|" + official.Manifest.LeaseID + "|" + official.Manifest.PayloadSHA256))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func brlCents(minor int64) string {
	sign := ""
	if minor < 0 {
		sign = "-"
		minor = -minor
	}
	return fmt.Sprintf("R$ %s%s,%02d", sign, groupThousands(minor/100), minor%100)
}

func officialRecordID(nativeKey string) string {
	sum := sha256.Sum256([]byte("sanction|" + nativeKey))
	return hex.EncodeToString(sum[:])[:24]
}

func containsCPF(value string) bool {
	digits := strings.NewReplacer(".", "", "-", "", " ", "").Replace(value)
	if len(digits) != 11 {
		return false
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func loadOfficialDataset(dir string, stations map[string]enterpriseexport.Station, requireStations bool) (*officialDataset, error) {
	manifestPath := filepath.Join(dir, "official-records-manifest.json")
	payloadPath := filepath.Join(dir, "official-records.jsonl")
	rawManifest, err := os.ReadFile(manifestPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var manifest officialManifest
	if err := json.Unmarshal(rawManifest, &manifest); err != nil {
		return nil, err
	}
	if manifest.SchemaVersion != officialSchema || manifest.Source != "sanctions" || manifest.Stream != "sanction-v1" ||
		manifest.Status != "published" || manifest.ProvenanceState != "VERIFIED" ||
		manifest.Rows <= 0 || manifest.Counts.Accepted != manifest.Rows ||
		manifest.AssociationCensus.Total != manifest.Rows ||
		manifest.AssociationCensus.Total != manifest.AssociationCensus.Linked+manifest.AssociationCensus.Unmatched+manifest.AssociationCensus.Ambiguous+
			manifest.AssociationCensus.WrongBranch+manifest.AssociationCensus.SuccessionReview ||
		len(manifest.PayloadSHA256) != 64 || manifest.RevisionID == "" || manifest.LeaseID == "" {
		return nil, fmt.Errorf("portalgen: invalid official manifest")
	}
	payload, err := os.ReadFile(payloadPath)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(payload)
	if hex.EncodeToString(sum[:]) != manifest.PayloadSHA256 {
		return nil, fmt.Errorf("portalgen: official payload hash mismatch")
	}
	records := make([]officialRecord, 0, manifest.Rows)
	scanner := bufio.NewScanner(strings.NewReader(string(payload)))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	linked, unmatched, ambiguous := 0, 0, 0
	byStation := map[string][]officialRecord{}
	for scanner.Scan() {
		var record officialRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, err
		}
		if record.NativeKey == "" || record.Evidence == "" || record.Scope != "sanction" ||
			record.Status == "" || record.AssociationReason == "" ||
			record.SourceURL == "" || record.SourceChecksum == "" ||
			record.RevisionID != manifest.RevisionID || record.LeaseID != manifest.LeaseID ||
			record.SourcePeriod == "" || record.SourceContract == "" {
			return nil, fmt.Errorf("portalgen: invalid official record")
		}
		cnpj, err := kernel.ParseCNPJ(record.SubjectCNPJ)
		if err != nil || cnpj.Normalized() != record.SubjectCNPJ {
			return nil, fmt.Errorf("portalgen: invalid official CNPJ")
		}
		for _, field := range []string{record.NativeKey, record.Evidence, record.Status, record.ProceedingRef} {
			if containsCPF(field) {
				return nil, fmt.Errorf("portalgen: CPF-shaped official field")
			}
		}
		for _, day := range []string{record.OccurredOn, record.AsOf, record.DecidedOn, record.DueOn} {
			if day != "" {
				if _, err := time.Parse("2006-01-02", day); err != nil {
					return nil, fmt.Errorf("portalgen: invalid official date")
				}
			}
		}
		switch record.AssociationState {
		case "linked":
			if record.StationID == "" || !isUUID(record.StationID) {
				return nil, fmt.Errorf("portalgen: linked official record without station")
			}
			// Station presence is only required when station pages are
			// rendered. Official-only output delinks records, so the
			// snapshot may omit station rows.
			if _, ok := stations[record.StationID]; !ok && requireStations {
				return nil, fmt.Errorf("portalgen: linked official station absent from snapshot")
			}
			linked++
			byStation[record.StationID] = append(byStation[record.StationID], record)
		case "unmatched":
			if record.StationID != "" {
				return nil, fmt.Errorf("portalgen: unmatched official record has station")
			}
			unmatched++
		case "ambiguous":
			if record.StationID != "" {
				return nil, fmt.Errorf("portalgen: ambiguous official record has station")
			}
			ambiguous++
		default:
			return nil, fmt.Errorf("portalgen: invalid official association state")
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(records) != manifest.Rows || linked != manifest.AssociationCensus.Linked ||
		unmatched != manifest.AssociationCensus.Unmatched+manifest.AssociationCensus.WrongBranch+manifest.AssociationCensus.SuccessionReview ||
		ambiguous != manifest.AssociationCensus.Ambiguous {
		return nil, fmt.Errorf("portalgen: official census mismatch")
	}
	sort.Slice(records, func(i, j int) bool {
		left, right := officialSortDate(records[i]), officialSortDate(records[j])
		if left != right {
			return left > right
		}
		return records[i].NativeKey < records[j].NativeKey
	})
	for station := range byStation {
		sort.Slice(byStation[station], func(i, j int) bool {
			left, right := officialSortDate(byStation[station][i]), officialSortDate(byStation[station][j])
			if left != right {
				return left > right
			}
			return byStation[station][i].NativeKey < byStation[station][j].NativeKey
		})
	}
	pages := (len(records) + officialPageSize - 1) / officialPageSize
	return &officialDataset{Manifest: manifest, Records: records, ByStation: byStation, Pages: pages}, nil
}

func officialSortDate(record officialRecord) string {
	for _, day := range []string{record.DecidedOn, record.DueOn, record.AsOf, record.OccurredOn} {
		if day != "" {
			return day
		}
	}
	return record.SourcePeriod
}

func officialAssociation(state string) string {
	switch state {
	case "linked":
		return "Vinculado a posto (decisão atual)"
	case "unmatched":
		return "Sem vínculo de posto"
	case "ambiguous":
		return "Vínculo ambíguo"
	default:
		return state
	}
}

func officialView(record officialRecord, stations map[string]enterpriseexport.Station, linkStations bool) officialRecordView {
	view := officialRecordView{
		RecordID: officialRecordID(record.NativeKey), NativeKey: record.NativeKey,
		CNPJ: cnpjBR(record.SubjectCNPJ), Status: record.Status,
		Proceeding: record.ProceedingRef, DecidedOn: dateBR(record.DecidedOn),
		DueOn: dateBR(record.DueOn), AsOf: dateBR(record.AsOf),
		OccurredOn: dateBR(record.OccurredOn), ExtractionMonth: record.ExtractionMonth,
		SourcePeriod: record.SourcePeriod, AssociationState: record.AssociationState,
		AssociationLabel:  officialAssociation(record.AssociationState),
		AssociationReason: record.AssociationReason, SourceURL: record.SourceURL,
		SourceChecksum: record.SourceChecksum, Evidence: record.Evidence,
		RevisionID: record.RevisionID, LeaseID: record.LeaseID,
	}
	if text := strings.TrimSpace(record.AmountText); text != "" {
		if strings.HasPrefix(strings.ToUpper(text), "R$") {
			view.Amount = text
		} else {
			view.Amount = "R$ " + text
		}
	} else if record.AmountMinor != nil {
		view.Amount = brlCents(*record.AmountMinor)
	}
	if record.ReportYear != nil {
		view.ReportYear = strconv.Itoa(int(*record.ReportYear))
	}
	if record.AssociationState == "linked" {
		if station, ok := stations[record.StationID]; ok && linkStations {
			view.StationName = station.DisplayName
			view.StationHref = "/postos/" + record.StationID + "/"
		}
	}
	return view
}

func (g *generator) generateOfficial(official *officialDataset, stations map[string]enterpriseexport.Station, period, published, inputHash string) error {
	if official == nil {
		base := g.base("Fiscalização — ainda não disponível", "Recorte oficial de fiscalização ainda não presente no snapshot; nenhum registro é exibido ou inferido.", "fiscalizacao", period, published, inputHash, []crumb{{Label: "Fiscalização", Current: true}})
		base.Indexable = false
		return g.writePage("fiscalizacao/index.html", "fiscalizacao", fiscalizacaoPage{pageBase: base})
	}
	pin := official.Manifest.pin()
	views := make([]officialRecordView, 0, len(official.Records))
	for _, record := range official.Records {
		views = append(views, officialView(record, stations, !g.opts.OfficialOnly))
	}
	for page := 1; page <= official.Pages; page++ {
		start := (page - 1) * officialPageSize
		end := start + officialPageSize
		if end > len(views) {
			end = len(views)
		}
		prev, next := "", ""
		if page > 2 {
			prev = fmt.Sprintf("/fiscalizacao/pagina/%d/", page-1)
		} else if page == 2 {
			prev = "/fiscalizacao/"
		}
		if page < official.Pages {
			next = fmt.Sprintf("/fiscalizacao/pagina/%d/", page+1)
		}
		crumbs := []crumb{{Label: "Fiscalização", Current: page == 1}}
		if page > 1 {
			crumbs[0].Href = "/fiscalizacao/"
		}
		data := fiscalizacaoPage{
			pageBase: g.base(fiscalizacaoTitle(page), "Registros oficiais de multas aplicadas, com vínculo apenas quando a associação foi decidida com segurança.", "fiscalizacao", period, published, inputHash, crumbs),
			HasData:  true, Manifest: pin, Records: views[start:end],
			Page: page, Pages: official.Pages, PrevHref: prev, NextHref: next,
			Linked: pin.AssociationCensus.Linked, Unmatched: pin.AssociationCensus.Unmatched + pin.AssociationCensus.WrongBranch + pin.AssociationCensus.SuccessionReview,
			Ambiguous: pin.AssociationCensus.Ambiguous, Total: pin.Rows,
			SourceInfo: valueOr(pin.SourcePeriod, pin.Edition),
		}
		path := "fiscalizacao/index.html"
		if page > 1 {
			path = fmt.Sprintf("fiscalizacao/pagina/%d/index.html", page)
		}
		if err := g.writePage(path, "fiscalizacao", data); err != nil {
			return err
		}
		if page == 1 {
			g.addSitemapRoute("/fiscalizacao/")
		} else {
			g.addSitemapRoute(fmt.Sprintf("/fiscalizacao/pagina/%d/", page))
		}
	}
	recordPageBase := func(view officialRecordView) pageBase {
		return g.base("Registro oficial — "+view.Proceeding, "Registro oficial de multa aplicada com status, datas, fonte e vínculo apenas quando decidido com segurança.", "fiscalizacao", period, published, inputHash, []crumb{
			{Label: "Fiscalização", Href: "/fiscalizacao/"},
			{Label: view.Proceeding, Current: true},
		})
	}
	for _, view := range views {
		if err := g.writePage("fiscalizacao/registro/"+view.RecordID+"/index.html", "fiscalizacaoRecord", fiscalizacaoRecordPage{
			pageBase: recordPageBase(view), Record: view, Manifest: pin,
		}); err != nil {
			return err
		}
		g.addSitemapRoute("/fiscalizacao/registro/" + view.RecordID + "/")
	}
	return nil
}

func fiscalizacaoTitle(page int) string {
	if page <= 1 {
		return "Fiscalização — multas aplicadas"
	}
	return fmt.Sprintf("Fiscalização — multas aplicadas (página %d)", page)
}

func officialStationViews(dataset *officialDataset, stationID string, stations map[string]enterpriseexport.Station) ([]officialRecordView, int) {
	if dataset == nil {
		return nil, 0
	}
	records := dataset.ByStation[stationID]
	views := make([]officialRecordView, 0, min(len(records), officialStationCap))
	for i, record := range records {
		if i >= officialStationCap {
			break
		}
		views = append(views, officialView(record, stations, true))
	}
	return views, len(records)
}
