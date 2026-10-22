package domain

import (
	"strings"
	"time"
)

// Policy version frozen by the exploration contract.
const PolicyVersion = "mex-exploration-v1"

// Interpretation states. RESOLVED carries one typed query; NEEDS_CHOICE
// carries labelled alternatives for an explicitly ambiguous question;
// UNSUPPORTED carries a reason and never a fabricated answer.
const (
	StateResolved    = "RESOLVED"
	StateNeedsChoice = "NEEDS_CHOICE"
	StateUnsupported = "UNSUPPORTED"
)

// Needs-choice reasons: the question names an intent but leaves one
// dimension open. Alternatives resolve it without guessing.
const (
	ReasonMetricRequired    = "METRIC_REQUIRED"
	ReasonSourceRequired    = "SOURCE_REQUIRED"
	ReasonFuelRequired      = "FUEL_REQUIRED"
	ReasonConditionRequired = "CONDITION_REQUIRED"
	ReasonPeriodRequired    = "PERIOD_REQUIRED"
)

// Unsupported reasons: no supported intent, unknown fuel, unsupported
// filter combination, or unknown shape. Adversarial text lands here.
const (
	ReasonUnsupportedIntent = "UNSUPPORTED_INTENT"
	ReasonUnknownFuel       = "UNKNOWN_FUEL"
	ReasonUnsupportedFilter = "UNSUPPORTED_FILTER"
	ReasonUnknownSchema     = "UNKNOWN_SCHEMA"
)

// Query kinds.
const (
	KindPriceRanking   = "PRICE_RANKING"
	KindOfficialRecord = "OFFICIAL_RECORDS"
)

// PriceQuery is the frozen typed price question. Every field is exact;
// nothing is defaulted silently beyond the documented national scope and
// the caller-supplied reference week. JSON tags match the frozen
// exploration schema exactly; optional fields omit when empty.
type PriceQuery struct {
	Version int          `json:"version"`
	Kind    string       `json:"kind"`
	Source  PriceSource  `json:"source"`
	Product ProductScope `json:"product"`
	Cond    Condition    `json:"condition"`
	Geog    Geography    `json:"geography"`
	Metric  PriceMetric  `json:"metric"`
	Page    Page         `json:"page"`
	Policy  string       `json:"policy_version"`
}

// PriceSource selects ANP weekly (with survey-week start) or community
// current eligible prices. The two are never mixed.
type PriceSource struct {
	Kind            string `json:"kind"`
	SurveyWeekStart string `json:"survey_week_start,omitempty"`
	Eligibility     string `json:"eligibility,omitempty"`
}

// ProductScope is an exact fuel/unit pair. Liquid fuels use L, CNG uses
// M3, LPG_P13 uses KG_13.
type ProductScope struct {
	Fuel string `json:"fuel"`
	Unit string `json:"unit"`
}

// Condition is an exact price condition. STANDARD has no qualifier.
type Condition struct {
	Kind        string `json:"kind"`
	QualifierID string `json:"qualifier_id,omitempty"`
}

// Geography is COUNTRY/BR, STATE with explicit UF and country, or
// MUNICIPALITY with a caller-verified name. The interpreter never
// invents IBGE codes. Country is always BR.
type Geography struct {
	Level        string `json:"level"`
	Country      string `json:"country"`
	UF           string `json:"uf,omitempty"`
	Municipality string `json:"municipality,omitempty"`
}

// PriceMetric selects observation prices grouped by station or area means
// grouped by state/municipality, sorted ascending or descending.
// Weighting appears only on area means.
type PriceMetric struct {
	Kind      string `json:"kind"`
	GroupBy   string `json:"group_by"`
	Sort      string `json:"sort"`
	Weighting string `json:"weighting,omitempty"`
}

// Page bounds the result window.
type Page struct {
	Limit int `json:"limit"`
}

// OfficialQuery selects source records by type, geography, date basis and
// page, matching the frozen schema. Its shape never claims a source
// supports a field; execution must resolve capabilities separately.
type OfficialQuery struct {
	Version     int          `json:"version"`
	Kind        string       `json:"kind"`
	Sources     []string     `json:"sources"`
	RecordKinds []string     `json:"record_kinds"`
	Scope       RecordScope  `json:"scope"`
	Geog        Geography    `json:"geography"`
	Date        DateFilter   `json:"date"`
	Status      StatusFilter `json:"status"`
	Page        Page         `json:"page"`
	Policy      string       `json:"policy_version"`
}

// RecordScope selects source-global or current-company-only records.
type RecordScope struct {
	Kind           string `json:"kind"`
	StationID      string `json:"station_id,omitempty"`
	GeographyBasis string `json:"geography_basis"`
}

// DateFilter declares an explicit basis, precision and inclusive/exclusive
// bounds. Unknown dates never match a dated filter.
type DateFilter struct {
	Basis       string `json:"basis"`
	Precision   string `json:"precision"`
	From        string `json:"from"`
	ToExclusive string `json:"to_exclusive"`
}

// StatusFilter selects any status or one exact reported value.
type StatusFilter struct {
	Kind          string `json:"kind"`
	ReportedValue string `json:"reported_value,omitempty"`
}

// Interpretation is one RESOLVED query, one NEEDS_CHOICE alternative set,
// or one UNSUPPORTED reason.
type Interpretation struct {
	State        string
	Query        any
	Reason       string
	Alternatives []any
}

// MaxQuestionRunes bounds adversarial input. Longer questions are
// unsupported, never truncated into a different meaning.
const MaxQuestionRunes = 500

// Fuel vocabulary: normalized name fragments to exact fuel/unit pairs.
// The wire spelling stays canonical; legacy spellings map here, never on
// the wire.
var fuels = []struct {
	fuel      string
	unit      string
	fragments []string
}{
	{"GASOLINE_REGULAR", "L", []string{"gasolina comum", "gasolina sem aditivo", "gasolina"}},
	{"GASOLINE_ADDITIVED", "L", []string{"gasolina aditivada", "gasolina com aditivo", "aditivada"}},
	{"GASOLINE_PREMIUM_GRADE", "L", []string{"gasolina premium", "gasolina podium", "podium", "premium"}},
	{"ETHANOL", "L", []string{"etanol", "alcool", "alcool hidratado"}},
	{"DIESEL_S10", "L", []string{"diesel s10", "s10"}},
	{"DIESEL_S500", "L", []string{"diesel s500", "s500", "diesel comum"}},
	{"DIESEL", "L", []string{"diesel", "oleo diesel"}},
	{"CNG", "M3", []string{"gnv", "gas natural veicular", "gas natural"}},
	{"LPG_P13", "KG_13", []string{"glp", "gas de cozinha", "gas de botijao", "botijao", "p13"}},
}

// Unsupported fuel-like words: fuel intent with no supported mapping.
var unknownFuels = []string{"eletricidade", "eletrico", "querosene", "arla", "ureia", "biodiesel", "querosene de aviacao", "gasolina de aviacao", "oleo lubrificante", "graxa"}

// UF vocabulary: normalized full names to codes, plus the codes themselves.
var ufs = map[string]string{
	"acre": "AC", "alagoas": "AL", "amapa": "AM", "amazonas": "AM", "bahia": "BA",
	"ceara": "CE", "distrito federal": "DF", "brasilia": "DF",
	"espirito santo": "ES", "goias": "GO", "maranhao": "MA",
	"mato grosso": "MT", "mato grosso do sul": "MS", "minas gerais": "MG",
	"para": "PA", "paraiba": "PB", "parana": "PR", "pernambuco": "PE",
	"piaui": "PI", "rio de janeiro": "RJ", "rio grande do norte": "RN",
	"rio grande do sul": "RS", "rondonia": "RO", "roraima": "RR",
	"santa catarina": "SC", "sao paulo": "SP", "sergipe": "SE", "tocantins": "TO",
}

// normalize lowercases, strips accents, collapses whitespace and bounds
// length. It never interprets, only canonicalizes for allowlisted matching.
// Kept: a-z, 0-9 (for dates), and separators. Everything else is a space.
func normalize(question string) (string, bool) {
	if len([]rune(question)) > MaxQuestionRunes {
		return "", false
	}
	lowered := strings.ToLower(question)
	var b strings.Builder
	b.Grow(len(lowered))
	previousSpace := true
	for _, r := range lowered {
		if accent, ok := accents[r]; ok {
			r = accent
		}
		keep := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if !keep {
			r = ' '
		}
		if r == ' ' {
			if !previousSpace {
				b.WriteRune(' ')
			}
			previousSpace = true
			continue
		}
		b.WriteRune(r)
		previousSpace = false
	}
	return strings.TrimSpace(b.String()), true
}

var accents = map[rune]rune{
	'á': 'a', 'à': 'a', 'â': 'a', 'ã': 'a', 'ä': 'a',
	'é': 'e', 'è': 'e', 'ê': 'e', 'ë': 'e',
	'í': 'i', 'ì': 'i', 'î': 'i', 'ï': 'i',
	'ó': 'o', 'ò': 'o', 'ô': 'o', 'õ': 'o', 'ö': 'o',
	'ú': 'u', 'ù': 'u', 'û': 'u', 'ü': 'u',
	'ç': 'c', 'ñ': 'n',
}

// contains reports whether any fragment appears as a substring.
func contains(normalized string, fragments ...string) bool {
	for _, fragment := range fragments {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}

// latestSunday returns the most recent Sunday on or before the reference
// date, formatted as a survey-week start. The caller supplies now;
// the interpreter never reads a clock.
func latestSunday(now time.Time) string {
	weekday := int(now.Weekday())
	sunday := now.AddDate(0, 0, -weekday)
	return sunday.Format("2006-01-02")
}
