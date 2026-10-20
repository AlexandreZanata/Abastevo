package application

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
)

// FineHistoryProfile is the frozen independently inspected source shape, not
// an interpretation of payment status, legal finality or historical ownership.
type FineHistoryProfile struct {
	Contract, Period, SourceFile, ReferenceLabel, ClockPrecision string
	HeaderRecord                                                 int
	Transit, UnknownPaid, Unlabelled                             bool
}

func HistoricalFineProfile(period string) (FineHistoryProfile, bool) {
	p := FineHistoryProfile{Period: period, ReferenceLabel: "Número do DUF", ClockPrecision: "MONTH", HeaderRecord: 7}
	switch period {
	case "2016-2019":
		p.Contract = "anp-sicom-utf8bom-group-2016-2019-v1"
		p.SourceFile = "multas-aplicadas-2016a2019.csv"
		p.ReferenceLabel = "Auto de Infração"
		p.ClockPrecision = "DAY"
		p.HeaderRecord = 5
		p.Transit = true
	case "2020":
		p.Contract = "anp-sicom-utf8bom-2020-v1"
		p.ReferenceLabel = "Auto de Infração"
		p.Transit = true
	case "2021":
		p.Contract = "anp-sicom-utf8bom-csv-named-xlsx-2021-v1"
		p.SourceFile = "multas-2021-junho22.xlsx"
		p.ReferenceLabel = "Auto de Infração"
		p.Transit = true
	case "2022":
		p.Contract = "anp-sicom-utf8bom-2022-v1"
		p.Transit = true
	case "2023":
		p.Contract = "anp-sicom-cp850-2023-v1"
		p.HeaderRecord = 6
		p.UnknownPaid = true
	case "2024":
		p.Contract = "anp-sicom-utf8bom-2024-v1"
		p.Unlabelled = true
	default:
		return FineHistoryProfile{}, false
	}
	if p.SourceFile == "" {
		p.SourceFile = "multas-aplicadas-" + period + ".csv"
	}
	return p, true
}

type FineHistoryClock struct {
	Precision string `json:"precision"`
	Value     string `json:"value"`
}

func (c FineHistoryClock) Valid(period string) bool {
	p, ok := HistoricalFineProfile(period)
	if !ok || c.Precision != p.ClockPrecision {
		return false
	}
	if p.ClockPrecision == "DAY" {
		return domain.ValidDay(c.Value) && c.Value >= "2020-01-01"
	}
	value, e := time.Parse("2006-01", c.Value)
	year, _ := strconv.Atoi(period)
	return e == nil && value.Format("2006-01") == c.Value && value.Year() >= year
}

// HistoricalFineObservation is deliberately separate from2025 FineObservation
// and legacy SanctionInput. Fields/order form the native canonical contract.
type HistoricalFineObservation struct {
	Schema          string           `json:"schema_version"`
	Source          string           `json:"source"`
	ObservationKey  string           `json:"observation_key"`
	SourceCNPJ      string           `json:"source_key"`
	ProceedingRef   string           `json:"proceeding_ref"`
	ReferenceLabel  string           `json:"reference_label"`
	ReferenceRef    string           `json:"reference_ref"`
	Department      string           `json:"department"`
	Status          string           `json:"status"`
	SourceTransitOn *string          `json:"source_transit_on"`
	DueOn           string           `json:"due_on"`
	AmountText      string           `json:"amount_text"`
	AmountMinor     int64            `json:"amount_minor"`
	PaidText        string           `json:"paid_text"`
	PaidMinor       *int64           `json:"paid_minor"`
	UnlabelledField string           `json:"unlabelled_field"`
	SourcePeriod    string           `json:"source_period"`
	Clock           FineHistoryClock `json:"clock"`
	InputContract   string           `json:"input_contract"`
	EvidenceRef     string           `json:"evidence_ref"`
}

func (r HistoricalFineObservation) ObservationIdentity() string {
	r.ObservationKey = ""
	r.EvidenceRef = ""
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	_ = e.Encode(r)
	sum := sha256.Sum256(bytes.TrimSuffix(b.Bytes(), []byte("\n")))
	return "observation:" + hex.EncodeToString(sum[:])
}
func historyText(s string, max int, empty bool) bool {
	return len(s) <= max && (empty || strings.TrimSpace(s) != "") && utf8.ValidString(s) && strings.IndexFunc(s, func(r rune) bool { return unicode.IsControl(r) || r == '\u2028' || r == '\u2029' }) < 0
}
func HistoricalFineAmount(raw string) (int64, bool) {
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(s, "R$") {
		s = strings.TrimSpace(strings.TrimPrefix(s, "R$"))
	}
	return domain.ParseFineAmount(s)
}
func (r HistoricalFineObservation) Valid() bool {
	p, ok := HistoricalFineProfile(r.SourcePeriod)
	if !ok || r.Schema != "anp-fine-history-observation-v1" || r.Source != "sanctions" || r.InputContract != p.Contract || r.ReferenceLabel != p.ReferenceLabel || !r.Clock.Valid(r.SourcePeriod) || !domain.ValidDay(r.DueOn) {
		return false
	}
	company, e := kernel.ParseCNPJ(r.SourceCNPJ)
	if e != nil || company.Normalized() != r.SourceCNPJ {
		return false
	}
	if r.SourceTransitOn != nil && (!p.Transit || !domain.ValidDay(*r.SourceTransitOn)) {
		return false
	}
	if !p.Unlabelled && r.UnlabelledField != "" {
		return false
	}
	for _, s := range []string{r.ProceedingRef, r.ReferenceRef, r.Status, r.AmountText, r.PaidText} {
		if !historyText(s, 64<<10, false) {
			return false
		}
	}
	if !historyText(r.Department, 64<<10, true) || !historyText(r.UnlabelledField, 64<<10, true) || !historyText(r.EvidenceRef, 512, false) {
		return false
	}
	amount, ok := HistoricalFineAmount(r.AmountText)
	if !ok || amount != r.AmountMinor {
		return false
	}
	if r.PaidMinor == nil {
		text := strings.TrimSpace(r.PaidText)
		if !p.UnknownPaid || !strings.HasPrefix(text, "R$") || strings.TrimSpace(strings.TrimPrefix(text, "R$")) != "-" {
			return false
		}
	} else {
		paid, ok := HistoricalFineAmount(r.PaidText)
		if !ok || paid != *r.PaidMinor {
			return false
		}
	}
	return r.ObservationKey == r.ObservationIdentity()
}

type FineHistoryNote struct {
	Schema      string `json:"schema_version"`
	Kind        string `json:"kind"`
	Text        string `json:"text"`
	EvidenceRef string `json:"evidence_ref"`
}

func (n FineHistoryNote) Valid(period string) bool {
	if n.Schema != "anp-fine-source-note-v1" || (period != "2020" && period != "2021") || !historyText(n.EvidenceRef, 512, false) {
		return false
	}
	if n.Kind == "BLANK_SEPARATOR" {
		return n.Text == ""
	}
	if n.Kind != "PUBLIC_PAYMENT_NOTE" {
		return false
	}
	if n.Text == "(*) Para verificação do valor total pago dos processos com status parcelamento em andamento é preciso acessar o link: https://app.anp.gov.br/anp-gpc-consulta-web/public/emissao-gru/list.xhtml" {
		return true
	}
	if period == "2020" {
		return n.Text == "(**) Parcelamentos feitos via SAPIENS são controlados pela Procuradoria Federal. Para obter os valores já pagos entrar em contato com a AGU."
	}
	return n.Text == "(**) Parcelamentos SAPIENS são controlados pela Procuradoria Federal. Para obter o s valores já pagos entrar em contato com a AGU."
}
