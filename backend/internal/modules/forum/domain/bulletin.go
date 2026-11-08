package domain

import (
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

// Deterministic Forum bulletin template. Inputs come only from the Official
// FOR-25 aggregate; this package never reads prices or writes official facts.
const (
	BulletinTemplateVersion    = "forum-bulletin-v1"
	BulletinMethodologyVersion = "forum-official-mean-v1"
)

var (
	ErrInvalidBulletinInput      = errors.New("forum: invalid bulletin input")
	ErrBulletinContentTooLong    = errors.New("forum: bulletin content exceeds field limits")
	ErrBulletinArithmetic        = errors.New("forum: bulletin arithmetic exceeds supported range")
	ErrMissingBulletinProvenance = errors.New("forum: bulletin requires provenance and identity")
	ErrBulletinImmutable         = errors.New("forum: deterministic bulletin content is immutable")
)

var bulletinFuelUnits = map[string]string{
	"ETHANOL":                "L",
	"GASOLINE_REGULAR":       "L",
	"GASOLINE_ADDITIVED":     "L",
	"GASOLINE_PREMIUM_GRADE": "L",
	"DIESEL_S500":            "L",
	"DIESEL_S10":             "L",
	"CNG":                    "M3",
	"LPG_P13":                "KG_13",
}

var bulletinFuelTitles = map[string]string{
	"ETHANOL":                "Etanol",
	"GASOLINE_REGULAR":       "Gasolina comum",
	"GASOLINE_ADDITIVED":     "Gasolina aditivada",
	"GASOLINE_PREMIUM_GRADE": "Gasolina premium",
	"DIESEL_S500":            "Diesel S500",
	"DIESEL_S10":             "Diesel S10",
	"CNG":                    "GNV",
	"LPG_P13":                "GLP 13 kg",
}

var bulletinFuelBodyLabels = map[string]string{
	"ETHANOL":                "etanol",
	"GASOLINE_REGULAR":       "gasolina comum",
	"GASOLINE_ADDITIVED":     "gasolina aditivada",
	"GASOLINE_PREMIUM_GRADE": "gasolina premium",
	"DIESEL_S500":            "diesel S500",
	"DIESEL_S10":             "diesel S10",
	"CNG":                    "GNV",
	"LPG_P13":                "GLP 13 kg",
}

var bulletinUnitLabels = map[string]string{
	"L":     "L",
	"M3":    "m³",
	"KG_13": "kg",
}

// BulletinInput is the complete provenance accepted from the Official
// aggregate port. Counts are already deduplicated per station.
type BulletinInput struct {
	UF   string
	Fuel string
	Unit string

	CurrentRevisionID         string
	CurrentSurveyStart        string
	CurrentSurveyEnd          string
	CurrentPublishedAt        string
	CurrentSourceURL          string
	CurrentSourceChecksum     string
	CurrentParserVersion      string
	CurrentSumMilliBrl        int64
	CurrentObservationCount   int64
	CurrentSampledStations    int64
	CurrentSourceStations     int64
	CurrentSourceObservations int64

	PreviousRevisionID         string
	PreviousSurveyStart        string
	PreviousSurveyEnd          string
	PreviousPublishedAt        string
	PreviousSourceURL          string
	PreviousSourceChecksum     string
	PreviousParserVersion      string
	PreviousSumMilliBrl        int64
	PreviousObservationCount   int64
	PreviousSampledStations    int64
	PreviousSourceStations     int64
	PreviousSourceObservations int64
}

// BulletinIdentity is the deterministic no-op/correction identity.
type BulletinIdentity struct {
	Key                 string
	GroupKey            string
	TemplateVersion     string
	CurrentRevisionID   string
	PreviousRevisionID  string
	CurrentSurveyStart  string
	CurrentSurveyEnd    string
	CurrentPublishedAt  string
	PreviousSurveyStart string
	PreviousSurveyEnd   string
	PreviousPublishedAt string
	UF                  string
	Fuel                string
	Unit                string
}

// IsCorrectionOf reports whether next replaces a bulletin with the same two
// survey windows but a different accepted revision pair. Equal keys are a
// no-op, never a correction.
func (i BulletinIdentity) IsCorrectionOf(previous BulletinIdentity) bool {
	if i.Key == previous.Key {
		return false
	}
	if i.TemplateVersion != previous.TemplateVersion ||
		i.UF != previous.UF ||
		i.Fuel != previous.Fuel ||
		i.Unit != previous.Unit ||
		i.CurrentSurveyStart != previous.CurrentSurveyStart ||
		i.CurrentSurveyEnd != previous.CurrentSurveyEnd ||
		i.PreviousSurveyStart != previous.PreviousSurveyStart ||
		i.PreviousSurveyEnd != previous.PreviousSurveyEnd {
		return false
	}
	revisionChanged := i.CurrentRevisionID != previous.CurrentRevisionID ||
		i.PreviousRevisionID != previous.PreviousRevisionID
	if !revisionChanged {
		return false
	}
	return bulletinTimeAfter(i.CurrentPublishedAt, previous.CurrentPublishedAt) ||
		bulletinTimeAfter(i.PreviousPublishedAt, previous.PreviousPublishedAt)
}

// BulletinDraft is the complete original plain-text output for review.
type BulletinDraft struct {
	Key                string
	Title              string
	Excerpt            string
	BodyText           string
	Identity           BulletinIdentity
	MethodologyVersion string
	TemplateVersion    string
}

// BuildBulletinDraft renders one deterministic pt-BR draft from exact inputs.
func BuildBulletinDraft(in BulletinInput) (*BulletinDraft, error) {
	if err := validateBulletinInput(in); err != nil {
		return nil, err
	}

	currentMeanMilli := roundHalfAway(in.CurrentSumMilliBrl, in.CurrentObservationCount)
	previousMeanMilli := roundHalfAway(in.PreviousSumMilliBrl, in.PreviousObservationCount)
	deltaMilli, err := bulletinDeltaMilli(in)
	if err != nil {
		return nil, err
	}
	percentTenths, err := bulletinPercentTenths(in)
	if err != nil {
		return nil, err
	}

	currentMean := formatMilliBRL(currentMeanMilli)
	previousMean := formatMilliBRL(previousMeanMilli)
	delta := formatSignedMilliBRL(deltaMilli)
	percent := formatSignedPercentBig(percentTenths)
	unit := bulletinUnitLabels[in.Unit]
	fuelTitle := bulletinFuelTitles[in.Fuel]
	fuelBody := bulletinFuelBodyLabels[in.Fuel]

	title := fmt.Sprintf("%s em %s: R$ %s/%s", fuelTitle, in.UF, currentMean, unit)
	excerpt := fmt.Sprintf(
		"Média amostral de %d postos: R$ %s/%s, contra R$ %s/%s na semana anterior (%s/%s, %s%%).",
		in.CurrentObservationCount, currentMean, unit, previousMean, unit, delta, unit, percent,
	)
	body := fmt.Sprintf(
		"A média amostral do preço da %s (%s) no estado de %s foi de R$ %s/%s na semana de %s a %s. "+
			"Na semana anterior (%s a %s), a média foi de R$ %s/%s, com variação de %s/%s (%s%%).\n\n"+
			"Esta é uma média transversal calculada pelo Abastevo a partir de uma observação mais recente por posto nas duas revisões oficiais aceitas. "+
			"Não é a média publicada pela ANP, não mede a variação do preço no mesmo posto e não representa a frota nacional.\n\n"+
			"Revisão atual: %s, período %s a %s, publicada em %s, checksum %s, URL %s, parser %s.\n"+
			"Revisão anterior: %s, período %s a %s, publicada em %s, checksum %s, URL %s, parser %s.\n\n"+
			"Cobertura: %d postos amostrados na revisão atual e %d na anterior; %d postos com qualquer preço oficial na UF na revisão atual; %d na anterior. "+
			"Metodologia %s. Template %s.",
		fuelBody, unit, in.UF, currentMean, unit, in.CurrentSurveyStart, in.CurrentSurveyEnd,
		in.PreviousSurveyStart, in.PreviousSurveyEnd, previousMean, unit, delta, unit, percent,
		in.CurrentRevisionID, in.CurrentSurveyStart, in.CurrentSurveyEnd, in.CurrentPublishedAt,
		in.CurrentSourceChecksum, in.CurrentSourceURL, in.CurrentParserVersion,
		in.PreviousRevisionID, in.PreviousSurveyStart, in.PreviousSurveyEnd, in.PreviousPublishedAt,
		in.PreviousSourceChecksum, in.PreviousSourceURL, in.PreviousParserVersion,
		in.CurrentSampledStations, in.PreviousSampledStations, in.CurrentSourceStations, in.PreviousSourceStations,
		BulletinMethodologyVersion, BulletinTemplateVersion,
	)

	key := fmt.Sprintf("%s:%s:%s:%s:%s:%s", BulletinTemplateVersion, in.CurrentRevisionID, in.PreviousRevisionID, in.UF, in.Fuel, in.Unit)
	groupKey := fmt.Sprintf("%s:%s:%s:%s:%s:%s:%s:%s", BulletinTemplateVersion,
		in.CurrentSurveyStart, in.CurrentSurveyEnd, in.PreviousSurveyStart, in.PreviousSurveyEnd,
		in.UF, in.Fuel, in.Unit)
	if utf8.RuneCountInString(title) > MaxTopicTitleRunes ||
		utf8.RuneCountInString(excerpt) > 400 ||
		utf8.RuneCountInString(body) > MaxTopicBodyRunes {
		return nil, ErrBulletinContentTooLong
	}

	return &BulletinDraft{
		Key:      key,
		Title:    title,
		Excerpt:  excerpt,
		BodyText: body,
		Identity: BulletinIdentity{
			Key:                 key,
			GroupKey:            groupKey,
			TemplateVersion:     BulletinTemplateVersion,
			CurrentRevisionID:   in.CurrentRevisionID,
			PreviousRevisionID:  in.PreviousRevisionID,
			CurrentSurveyStart:  in.CurrentSurveyStart,
			CurrentSurveyEnd:    in.CurrentSurveyEnd,
			CurrentPublishedAt:  in.CurrentPublishedAt,
			PreviousSurveyStart: in.PreviousSurveyStart,
			PreviousSurveyEnd:   in.PreviousSurveyEnd,
			PreviousPublishedAt: in.PreviousPublishedAt,
			UF:                  in.UF,
			Fuel:                in.Fuel,
			Unit:                in.Unit,
		},
		MethodologyVersion: BulletinMethodologyVersion,
		TemplateVersion:    BulletinTemplateVersion,
	}, nil
}

func validateBulletinInput(in BulletinInput) error {
	uf := strings.TrimSpace(in.UF)
	if _, ok := validUFs[uf]; !ok {
		return fmt.Errorf("%w: uf", ErrInvalidBulletinInput)
	}
	unit, ok := bulletinFuelUnits[in.Fuel]
	if !ok || unit != in.Unit {
		return fmt.Errorf("%w: fuel/unit", ErrInvalidBulletinInput)
	}
	if !uuidRegex.MatchString(in.CurrentRevisionID) || !uuidRegex.MatchString(in.PreviousRevisionID) ||
		strings.EqualFold(in.CurrentRevisionID, in.PreviousRevisionID) {
		return fmt.Errorf("%w: revision pair", ErrInvalidBulletinInput)
	}
	currentStart, err := time.Parse("2006-01-02", in.CurrentSurveyStart)
	if err != nil {
		return fmt.Errorf("%w: current survey start", ErrInvalidBulletinInput)
	}
	currentEnd, err := time.Parse("2006-01-02", in.CurrentSurveyEnd)
	if err != nil || currentEnd.Before(currentStart) {
		return fmt.Errorf("%w: current survey period", ErrInvalidBulletinInput)
	}
	previousStart, err := time.Parse("2006-01-02", in.PreviousSurveyStart)
	if err != nil {
		return fmt.Errorf("%w: previous survey start", ErrInvalidBulletinInput)
	}
	previousEnd, err := time.Parse("2006-01-02", in.PreviousSurveyEnd)
	if err != nil || previousEnd.Before(previousStart) {
		return fmt.Errorf("%w: previous survey period", ErrInvalidBulletinInput)
	}
	if !previousEnd.Before(currentStart) {
		return fmt.Errorf("%w: overlapping survey windows", ErrInvalidBulletinInput)
	}
	if _, err := time.Parse(time.RFC3339, in.CurrentPublishedAt); err != nil {
		return fmt.Errorf("%w: current published_at", ErrInvalidBulletinInput)
	}
	if _, err := time.Parse(time.RFC3339, in.PreviousPublishedAt); err != nil {
		return fmt.Errorf("%w: previous published_at", ErrInvalidBulletinInput)
	}
	if err := validateBulletinProvenance(in.CurrentSourceURL, in.CurrentSourceChecksum, in.CurrentParserVersion); err != nil {
		return err
	}
	if err := validateBulletinProvenance(in.PreviousSourceURL, in.PreviousSourceChecksum, in.PreviousParserVersion); err != nil {
		return err
	}
	if in.CurrentSumMilliBrl <= 0 || in.PreviousSumMilliBrl <= 0 ||
		in.CurrentObservationCount <= 0 || in.PreviousObservationCount <= 0 {
		return fmt.Errorf("%w: non-positive sample", ErrInvalidBulletinInput)
	}
	if in.CurrentSampledStations < BulletinMinimumSampledStations ||
		in.PreviousSampledStations < BulletinMinimumSampledStations ||
		in.CurrentSampledStations != in.CurrentObservationCount ||
		in.PreviousSampledStations != in.PreviousObservationCount {
		return fmt.Errorf("%w: sampled stations", ErrInvalidBulletinInput)
	}
	if in.CurrentSourceStations < in.CurrentSampledStations ||
		in.PreviousSourceStations < in.PreviousSampledStations ||
		in.CurrentSourceObservations < in.CurrentObservationCount ||
		in.PreviousSourceObservations < in.PreviousObservationCount {
		return fmt.Errorf("%w: coverage", ErrInvalidBulletinInput)
	}
	return nil
}

// BulletinMinimumSampledStations mirrors the frozen FOR-25 floor.
const BulletinMinimumSampledStations = 30

func bulletinDeltaMilli(in BulletinInput) (int64, error) {
	diffNum := new(big.Int).Sub(
		new(big.Int).Mul(big.NewInt(in.CurrentSumMilliBrl), big.NewInt(in.PreviousObservationCount)),
		new(big.Int).Mul(big.NewInt(in.PreviousSumMilliBrl), big.NewInt(in.CurrentObservationCount)),
	)
	diffDen := new(big.Int).Mul(big.NewInt(in.CurrentObservationCount), big.NewInt(in.PreviousObservationCount))
	rounded := roundHalfAwayBig(diffNum, diffDen)
	if !rounded.IsInt64() {
		return 0, ErrBulletinArithmetic
	}
	return rounded.Int64(), nil
}

func bulletinPercentTenths(in BulletinInput) (*big.Int, error) {
	diffNum := new(big.Int).Sub(
		new(big.Int).Mul(big.NewInt(in.CurrentSumMilliBrl), big.NewInt(in.PreviousObservationCount)),
		new(big.Int).Mul(big.NewInt(in.PreviousSumMilliBrl), big.NewInt(in.CurrentObservationCount)),
	)
	percentDen := new(big.Int).Mul(big.NewInt(in.PreviousSumMilliBrl), big.NewInt(in.CurrentObservationCount))
	percentNum := new(big.Int).Mul(diffNum, big.NewInt(1000))
	rounded := roundHalfAwayBig(percentNum, percentDen)
	if !rounded.IsInt64() {
		return nil, ErrBulletinArithmetic
	}
	return rounded, nil
}

func roundHalfAway(num, den int64) int64 {
	if den <= 0 {
		return 0
	}
	return roundHalfAwayBig(big.NewInt(num), big.NewInt(den)).Int64()
}

func roundHalfAwayBig(num, den *big.Int) *big.Int {
	if den.Sign() <= 0 {
		return big.NewInt(0)
	}
	sign := num.Sign()
	absNum := new(big.Int).Abs(num)
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(absNum, den, remainder)
	remainder.Lsh(remainder, 1)
	if remainder.Cmp(den) >= 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	if sign < 0 {
		quotient.Neg(quotient)
	}
	return quotient
}

func formatMilliBRL(milli int64) string {
	abs := milli
	if abs < 0 {
		abs = -abs
	}
	return fmt.Sprintf("%d,%03d", abs/1000, abs%1000)
}

func formatSignedMilliBRL(milli int64) string {
	switch {
	case milli > 0:
		return "+" + formatMilliBRL(milli)
	case milli < 0:
		return "-" + formatMilliBRL(-milli)
	default:
		return formatMilliBRL(0)
	}
}

func formatSignedPercentBig(tenths *big.Int) string {
	sign := ""
	value := new(big.Int).Set(tenths)
	if value.Sign() > 0 {
		sign = "+"
	} else if value.Sign() < 0 {
		sign = "-"
		value.Abs(value)
	}
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(value, big.NewInt(10), remainder)
	return fmt.Sprintf("%s%s,%s", sign, quotient.String(), remainder.String())
}

func bulletinTimeAfter(candidate, previous string) bool {
	candidateTime, candidateErr := time.Parse(time.RFC3339, candidate)
	previousTime, previousErr := time.Parse(time.RFC3339, previous)
	if candidateErr != nil || previousErr != nil {
		return candidate > previous
	}
	return candidateTime.After(previousTime)
}

func validateBulletinProvenance(sourceURL, checksum, parserVersion string) error {
	parsed, err := url.Parse(sourceURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return fmt.Errorf("%w: source url", ErrInvalidBulletinInput)
	}
	for _, field := range []string{sourceURL, checksum, parserVersion} {
		if strings.TrimSpace(field) == "" {
			return fmt.Errorf("%w: provenance", ErrInvalidBulletinInput)
		}
		for _, r := range field {
			if r < 0x20 || r == 0x7f {
				return fmt.Errorf("%w: provenance control character", ErrInvalidBulletinInput)
			}
		}
	}
	return nil
}

// BulletinRevisionProvenance is the persisted exact provenance for one side of
// a generated comparison.
type BulletinRevisionProvenance struct {
	RevisionID         string `json:"revision_id"`
	SurveyStart        string `json:"survey_start"`
	SurveyEnd          string `json:"survey_end"`
	PublishedAt        string `json:"published_at"`
	SourceURL          string `json:"source_url"`
	SourceChecksum     string `json:"source_checksum"`
	ParserVersion      string `json:"parser_version"`
	SumMilliBrl        int64  `json:"sum_milli_brl"`
	ObservationCount   int64  `json:"observation_count"`
	SampledStations    int64  `json:"sampled_stations"`
	SourceObservations int64  `json:"source_observations"`
	SourceStations     int64  `json:"source_stations"`
	MeanNumerator      int64  `json:"mean_numerator"`
	MeanDenominator    int64  `json:"mean_denominator"`
	Mean               string `json:"mean"`
}

// BulletinProvenance is the structured public provenance/methodology view.
type BulletinProvenance struct {
	MethodologyVersion string                     `json:"methodology_version"`
	TemplateVersion    string                     `json:"template_version"`
	Fuel               string                     `json:"fuel"`
	Unit               string                     `json:"unit"`
	Current            BulletinRevisionProvenance `json:"current"`
	Previous           BulletinRevisionProvenance `json:"previous"`
}
