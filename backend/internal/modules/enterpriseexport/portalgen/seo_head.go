package portalgen

import (
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	descriptionMin = 120
	descriptionMax = 160
	// ogImagePath is served by the landing on the same origin.
	ogImagePath = "/assets/brand/og-image-v1.png"
)

// descriptionFillers are honest generic sentences appended, in order, only
// while a description is shorter than descriptionMin.
var descriptionFillers = []string{
	"Fonte: ANP; cálculo e exibição do abastevo.",
	"Dados datados; não é preço ao vivo.",
	"Sem afiliação à ANP.",
	"Veja fonte, período e metodologia.",
}

// composeDescription joins complete sentences greedily up to descriptionMax
// runes, skipping a sentence that does not fit, then pads with fillers up to
// descriptionMin. An oversized first sentence is cut at a word boundary.
func composeDescription(parts ...string) string {
	out := ""
	add := func(part string) bool {
		part = strings.TrimSpace(part)
		if part == "" {
			return true
		}
		candidate := part
		if out != "" {
			candidate = out + " " + part
		}
		if utf8.RuneCountInString(candidate) > descriptionMax {
			return false
		}
		out = candidate
		return true
	}
	for i, part := range parts {
		if !add(part) && i == 0 {
			out = truncateWords(strings.TrimSpace(part), descriptionMax-1) + "…"
		}
	}
	for _, filler := range descriptionFillers {
		if utf8.RuneCountInString(out) >= descriptionMin {
			break
		}
		add(filler)
	}
	return out
}

func truncateWords(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	cut := string(runes[:limit])
	if i := strings.LastIndex(cut, " "); i > 0 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,;:.")
}

type fuelWords struct {
	Article string // "da"/"do"
	Name    string // lower-case running-text name
	Slug    string
}

var fuelVocabulary = map[string]fuelWords{
	"GASOLINE_REGULAR":       {"da", "gasolina comum", "gasolina-comum"},
	"GASOLINE_ADDITIVED":     {"da", "gasolina aditivada", "gasolina-aditivada"},
	"GASOLINE_PREMIUM_GRADE": {"da", "gasolina premium", "gasolina-premium"},
	"ETHANOL":                {"do", "etanol hidratado", "etanol"},
	"DIESEL_S10":             {"do", "diesel S10", "diesel-s10"},
	"DIESEL_S500":            {"do", "diesel S500", "diesel-s500"},
	"CNG":                    {"do", "GNV", "gnv"},
	"LPG_P13":                {"do", "GLP", "glp"},
}

// fuelSlug is the /precos/{slug}/ hub slug for a product code, "" when the
// product has no hub.
func fuelSlug(code string) string {
	return fuelVocabulary[code].Slug
}

func fuelRoute(code string) string {
	if slug := fuelSlug(code); slug != "" {
		return "/precos/" + slug + "/"
	}
	return ""
}

func fuelName(code string) string {
	if words, ok := fuelVocabulary[code]; ok {
		return words.Name
	}
	return strings.ToLower(fuelLabel(code))
}

func capitalize(value string) string {
	r, size := utf8.DecodeRuneInString(value)
	if r == utf8.RuneError {
		return value
	}
	return strings.ToUpper(string(r)) + value[size:]
}

// fuelFamilies lists, in a fixed order, the gasolina/etanol/diesel families
// present in codes; other products (GNV, GLP) are not named in titles.
func fuelFamilies(codes []string) []string {
	present := map[string]bool{}
	for _, code := range codes {
		switch {
		case strings.HasPrefix(code, "GASOLINE_"):
			present["gasolina"] = true
		case code == "ETHANOL":
			present["etanol"] = true
		case strings.HasPrefix(code, "DIESEL_"):
			present["diesel"] = true
		}
	}
	out := []string{}
	for _, family := range []string{"gasolina", "etanol", "diesel"} {
		if present[family] {
			out = append(out, family)
		}
	}
	return out
}

func joinPT(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	default:
		return strings.Join(items[:len(items)-1], ", ") + " e " + items[len(items)-1]
	}
}

func placeLabel(city, uf string) string {
	return city + " (" + uf + ")"
}

func cityTitle(city, uf string, fuels []string, surveyDate string) string {
	families := fuelFamilies(fuels)
	subject := "Preço dos combustíveis"
	if len(families) > 0 {
		article := "do"
		if families[0] == "gasolina" {
			article = "da"
		}
		subject = "Preço " + article + " " + joinPT(families)
	}
	return subject + " em " + placeLabel(city, uf) + " – pesquisa ANP " + surveyDate
}

func stateTitle(uf, surveyDate string) string {
	return "Preço dos combustíveis em " + ufName(uf) + " por cidade – ANP " + surveyDate
}

func fuelHubTitle(code, surveyDate string) string {
	words := fuelVocabulary[code]
	return "Preço " + words.Article + " " + words.Name + " no Brasil por estado – ANP " + surveyDate
}

func stationTitle(name, city, uf string) string {
	switch {
	case city != "" && uf != "":
		return name + " – preços em " + placeLabel(city, uf)
	case uf != "":
		return name + " – preços em " + ufName(uf)
	default:
		return name + " – preços de combustíveis"
	}
}

// periodShort renders a survey week compactly ("11/10 a 17/10/2026").
func periodShort(start, end string) string {
	s, errStart := parseDay(start)
	e, errEnd := parseDay(end)
	if errStart != nil || errEnd != nil {
		return "período não informado"
	}
	if s.Year() == e.Year() {
		return s.Format("02/01") + " a " + e.Format("02/01/2006")
	}
	return s.Format("02/01/2006") + " a " + e.Format("02/01/2006")
}

// byFuelRank orders summary rows the way drivers look for fuels.
func byFuelRank(rows []summaryRow) []summaryRow {
	out := append([]summaryRow(nil), rows...)
	sort.SliceStable(out, func(i, j int) bool { return fuelRank(out[i].Code) < fuelRank(out[j].Code) })
	return out
}

// rangeSentence summarizes one fuel row with figures shown on the page.
func rangeSentence(row summaryRow) string {
	sentence := capitalize(row.FuelName) + ": " + brl(row.Min)
	if row.Max != row.Min {
		sentence += " a " + brl(row.Max)
	}
	return sentence + " em " + groupThousands(int64(row.Stations)) + " " + plural(row.Stations, "posto", "postos") + "."
}
