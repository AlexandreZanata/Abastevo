package portalgen

import (
	"fmt"
	"strings"
)

// Dated answers (B-BR-SEO-09): a short summary of the survey and a visible
// question block on the price pages. Every sentence is a fixed template over
// figures the page already shows, and a sentence whose condition fails is
// left out. A "today" question is answered only with the survey period and
// the statement that the site has no live price.

const (
	noLivePrice         = "Este site não mostra preço ao vivo."
	todayMayDiffer      = "O preço de hoje em cada posto pode ser diferente."
	methodologyMeanHref = "/metodologia/#media-da-amostra"
)

// faqItem is one visible question; the optional link ends the answer.
type faqItem struct {
	Question  string
	Answer    string
	LinkHref  string
	LinkLabel string
}

// fuelFacts are the figures of one fuel in one area that the summary and
// the questions read. Place is "" for Brazil.
type fuelFacts struct {
	Code       string
	Place      string
	States     int
	Row        summaryRow
	Comparison *comparisonView
}

func stationsPhrase(n int) string {
	return fmt.Sprintf("%s %s", groupThousands(int64(n)), plural(n, "posto", "postos"))
}

// priceRange is "de R$ 6,000 a R$ 6,500", or the single price of a flat sample.
func priceRange(row summaryRow) string {
	if row.Min == row.Max {
		return brl(row.Min)
	}
	return "de " + brl(row.Min) + " a " + brl(row.Max)
}

// weekSummary is the one-paragraph reading of a fuel in an area.
func (g *generator) weekSummary(facts fuelFacts) string {
	if g.period == "" || facts.Row.Stations == 0 {
		return ""
	}
	row, name := facts.Row, fuelName(facts.Code)
	had := plural(row.Stations, "tinha", "tinham")
	sentences := []string{}
	if facts.Place == "" {
		sentences = append(sentences, fmt.Sprintf("Na pesquisa da ANP de %s, %s de %d %s %s preço de %s.",
			g.period, stationsPhrase(row.Stations), facts.States, plural(facts.States, "estado", "estados"), had, name))
	} else {
		sentences = append(sentences, fmt.Sprintf("Na pesquisa da ANP de %s, %s de %s %s preço de %s.",
			g.period, stationsPhrase(row.Stations), facts.Place, had, name))
	}
	switch {
	case row.Stations == 1:
		sentences = append(sentences, "O preço encontrado foi "+brl(row.Min)+".")
	case row.Min == row.Max:
		sentences = append(sentences, "Todos os postos da amostra tinham o mesmo preço: "+brl(row.Min)+".")
	default:
		sentences = append(sentences, "O menor preço da amostra foi "+brl(row.Min)+" e o maior, "+brl(row.Max)+".")
	}
	if row.HasMean {
		sentences = append(sentences, "A média da amostra foi "+brl(row.Mean)+".")
	}
	if facts.Comparison != nil {
		for _, change := range facts.Comparison.Rows {
			if change.Code != facts.Code {
				continue
			}
			switch {
			case !change.Comparable:
				sentences = append(sentences, "A amostra de postos mudou em relação à pesquisa anterior; por isso não calculamos variação.")
			case change.HasMeanDelta && change.MeanDelta > 0:
				sentences = append(sentences, "Com os mesmos postos da pesquisa anterior, a média subiu "+brl(change.MeanDelta)+".")
			case change.HasMeanDelta && change.MeanDelta < 0:
				sentences = append(sentences, "Com os mesmos postos da pesquisa anterior, a média caiu "+brl(-change.MeanDelta)+".")
			case change.HasMeanDelta:
				sentences = append(sentences, "Com os mesmos postos da pesquisa anterior, a média não mudou.")
			}
		}
	}
	return strings.Join(sentences, " ")
}

func datedQuestion(period string) faqItem {
	return faqItem{
		Question: "De quando são estes preços?",
		Answer:   "Da pesquisa semanal da ANP de " + period + ". Cada linha mostra a data da coleta no posto. Os números mudam quando a ANP publica a pesquisa seguinte.",
	}
}

// fuelFAQ answers the questions people type about one fuel in one area.
func (g *generator) fuelFAQ(facts fuelFacts) []faqItem {
	if g.period == "" || facts.Row.Stations == 0 {
		return nil
	}
	row, words := facts.Row, fuelVocabulary[facts.Code]
	in, of := "em "+facts.Place, "de "+facts.Place
	if facts.Place == "" {
		in, of = "no Brasil", "do Brasil"
	}
	today := noLivePrice + " A referência mais recente é a pesquisa da ANP de " + g.period + ": " + priceRange(row) + " em " + stationsPhrase(row.Stations) + " " + of + "."
	if row.HasMean {
		today += " A média da amostra foi " + brl(row.Mean) + "."
	}
	items := []faqItem{{Question: "Qual o preço " + words.Article + " " + words.Name + " hoje " + in + "?", Answer: today + " " + todayMayDiffer}}
	if facts.Place != "" {
		// The project never names a cheapest station: the question and the
		// answer speak of the lowest price found in a dated sample.
		items = append(items, faqItem{
			Question: "Qual foi o menor preço " + words.Article + " " + words.Name + " " + in + "?",
			Answer: "Na amostra da ANP de " + g.period + ", o menor preço foi " + brl(row.Min) +
				". A amostra não cobre todos os postos, e o preço pode ter mudado desde a coleta.",
		})
	}
	items = append(items, datedQuestion(g.period))
	if row.HasMean {
		items = append(items, faqItem{
			Question: "Como a média é calculada?",
			Answer: "É a média simples do preço mais recente de cada posto da amostra (" + stationsPhrase(row.Stations) +
				"). É um cálculo do abastevo a partir dos dados da ANP, e não uma estatística oficial.",
			LinkHref: methodologyMeanHref, LinkLabel: "Ver a metodologia",
		})
	}
	return items
}

// cityFAQ answers the all-fuels question of a city page with the range of
// each fuel the page summarizes.
func (g *generator) cityFAQ(place string, rows []summaryRow) []faqItem {
	if g.period == "" || len(rows) == 0 {
		return nil
	}
	answer := noLivePrice + " A referência mais recente é a pesquisa da ANP de " + g.period + "."
	for _, row := range byFuelRank(rows) {
		answer += " " + rangeSentence(row)
	}
	return []faqItem{
		{Question: "Qual o preço dos combustíveis hoje em " + place + "?", Answer: answer + " " + todayMayDiffer},
		datedQuestion(g.period),
	}
}
