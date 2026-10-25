package portalgen

import (
	"fmt"
	"sort"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/analytics"
)

type insightCard struct {
	Question   string
	Answer     string
	Caveat     string
	Formula    string
	SourceHref string
}

type curiosidadesPage struct {
	pageBase
	Cards         []insightCard
	DownloadsHref string
}

func previousRevision(revisions []enterpriseexport.Revision, latest *enterpriseexport.Revision) *enterpriseexport.Revision {
	if latest == nil || len(revisions) < 2 {
		return nil
	}
	sorted := append([]enterpriseexport.Revision(nil), revisions...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].SurveyStart != sorted[j].SurveyStart {
			return sorted[i].SurveyStart < sorted[j].SurveyStart
		}
		return sorted[i].RevisionID < sorted[j].RevisionID
	})
	for i, revision := range sorted {
		if revision.RevisionID == latest.RevisionID && i > 0 {
			previous := sorted[i-1]
			return &previous
		}
	}
	return nil
}

func nationalSummary(summaries []analytics.Summary, fuel, unit string) (analytics.Summary, bool) {
	for _, summary := range summaries {
		if summary.Geography.Level == analytics.LevelNational &&
			summary.Geography.Code == "BR" && summary.Fuel == fuel && summary.Unit == unit {
			return summary, true
		}
	}
	return analytics.Summary{}, false
}

func collectionRange(observations []observation) string {
	if len(observations) == 0 {
		return "sem coletas"
	}
	dates := make([]string, 0, len(observations))
	for _, item := range observations {
		dates = append(dates, item.price.CollectedOn)
	}
	sort.Strings(dates)
	return dateBR(dates[0]) + " a " + dateBR(dates[len(dates)-1])
}

func movement(value int64) string {
	switch {
	case value > 0:
		return "aumentou " + brl(value)
	case value < 0:
		return "diminuiu " + brl(-value)
	default:
		return "ficou igual"
	}
}

func exclusionLabel(code string) string {
	switch code {
	case "unknown_station":
		return "posto desconhecido"
	case "unknown_fuel":
		return "combustível não reconhecido"
	case "unit_mismatch":
		return "unidade incompatível"
	case "unsupported_currency":
		return "moeda não suportada"
	case "invalid_date":
		return "data inválida"
	case "period_mismatch":
		return "data fora da pesquisa"
	case "older_observation":
		return "observação mais antiga do mesmo posto/combustível"
	default:
		return code
	}
}

func buildInsights(latest, previous *enterpriseexport.Revision, latestSummaries, previousSummaries []analytics.Summary, coverage analytics.Coverage, observations []observation, municipalityStations map[string]int, published string) []insightCard {
	period := "pesquisa não publicada"
	if latest != nil {
		period = periodLabel(latest.SurveyStart, latest.SurveyEnd)
	}
	cards := make([]insightCard, 0, 6)

	coverageAnswer := "Sem pesquisa publicada neste candidato."
	if latest != nil {
		coverageAnswer = fmt.Sprintf(
			"A pesquisa de %s tem %d %s com pelo menos um preço incluído. Observações de origem: %d; incluídas: %d. Postos do diretório sem preço nesta pesquisa: %d; observações sem localização: %d.",
			period, coverage.StationCount, plural(coverage.StationCount, "posto", "postos"),
			coverage.SourceObservations, coverage.IncludedObservations,
			coverage.UnpricedStations, coverage.UnlocatedObservations,
		)
	}
	cards = append(cards, insightCard{
		Question:   "Quantos postos tinham preço nesta pesquisa?",
		Answer:     coverageAnswer,
		Caveat:     "A amostra não cobre todos os postos; ausência não é preço zero.",
		Formula:    "Contagem de postos e observações por revisão; fonte ANP; cálculo Abastevo.",
		SourceHref: "/metodologia/",
	})

	comparisonAnswer := "Sem resumo nacional de gasolina comum nesta pesquisa."
	comparisonCaveat := "Variação de resumo derivado; não é inflação oficial, previsão ou garantia de preço."
	if latest != nil {
		if current, ok := nationalSummary(latestSummaries, "GASOLINE_REGULAR", "L"); ok {
			if previous == nil {
				comparisonAnswer = fmt.Sprintf(
					"Há apenas uma pesquisa publicada. Nesta amostra de %d postos, a gasolina comum tem mínimo %s, máximo %s e dispersão %s.",
					current.StationCount, brl(current.MinMilliBRL), brl(current.MaxMilliBRL), brl(current.SpreadMilliBRL),
				)
			} else if before, ok := nationalSummary(previousSummaries, "GASOLINE_REGULAR", "L"); ok {
				change, err := analytics.CompareSummaries(before, current)
				switch {
				case err != nil:
					comparisonAnswer = "Não foi possível comparar as duas pesquisas."
				case change.SampleChanged:
					comparisonAnswer = fmt.Sprintf(
						"A amostra de postos mudou entre %s e %s; não calculamos variação percentual. Na pesquisa mais recente, o mínimo é %s, o máximo é %s e a dispersão é %s.",
						periodLabel(before.SurveyStart, before.SurveyEnd), period,
						brl(current.MinMilliBRL), brl(current.MaxMilliBRL), brl(current.SpreadMilliBRL),
					)
				case change.Comparable:
					comparisonAnswer = fmt.Sprintf(
						"Entre %s e %s, o mínimo %s, o máximo %s e a dispersão %s. A amostra de postos foi a mesma (%d postos).",
						periodLabel(before.SurveyStart, before.SurveyEnd), period,
						movementValue(change.ChangeMinMilliBRL), movementValue(change.ChangeMaxMilliBRL),
						movementValue(change.ChangeSpreadMilliBRL), current.StationCount,
					)
				default:
					comparisonAnswer = "Sem dados suficientes para comparar as duas pesquisas."
				}
			}
		}
	}
	cards = append(cards, insightCard{
		Question:   "Como o mínimo e o máximo da gasolina comum mudaram?",
		Answer:     comparisonAnswer,
		Caveat:     comparisonCaveat,
		Formula:    "Diferença entre resumos nacionais de gasolina comum (L) de duas revisões; só com a mesma amostra de postos; fonte ANP; cálculo Abastevo.",
		SourceHref: "/metodologia/",
	})

	fuelAnswer := "Sem combustíveis com observação nesta pesquisa."
	if len(coverage.FuelUnits) > 0 {
		parts := make([]string, 0, len(coverage.FuelUnits))
		for _, fuel := range coverage.FuelUnits {
			parts = append(parts, fmt.Sprintf("%s (%s): %d %s", fuelLabel(fuel.Fuel), fuel.Unit, fuel.Stations, plural(fuel.Stations, "posto", "postos")))
		}
		fuelAnswer = strings.Join(parts, "; ") + "."
	}
	cards = append(cards, insightCard{
		Question:   "Quais combustíveis aparecem na amostra?",
		Answer:     fuelAnswer,
		Caveat:     "Postos podem vender combustíveis que não foram amostrados.",
		Formula:    "Contagem de postos por combustível/unidade; uma observação mais recente por posto; fonte ANP; cálculo Abastevo.",
		SourceHref: "/metodologia/",
	})

	municipalities := map[string]analytics.Summary{}
	for _, summary := range latestSummaries {
		if summary.Geography.Level == analytics.LevelMunicipality {
			if _, exists := municipalities[summary.Geography.Code]; !exists {
				municipalities[summary.Geography.Code] = summary
			}
		}
	}
	sufficient, insufficient := 0, 0
	for code, summary := range municipalities {
		stations := summary.StationCount
		if total := municipalityStations[code]; total > stations {
			stations = total
		}
		if stations >= minMunicipality {
			sufficient++
		} else {
			insufficient++
		}
	}
	municipalityAnswer := "Sem resumos municipais nesta pesquisa."
	if len(municipalities) > 0 {
		municipalityAnswer = fmt.Sprintf(
			"%d de %d municípios com resumo têm pelo menos %d postos; abaixo do mínimo: %d (sem ranking).",
			sufficient, len(municipalities), minMunicipality, insufficient,
		)
	}
	cards = append(cards, insightCard{
		Question:   "Quantos municípios têm amostra suficiente?",
		Answer:     municipalityAnswer,
		Caveat:     "Amostra insuficiente não é um julgamento sobre os postos; é um limite de leitura do resumo.",
		Formula:    "Contagem de resumos municipais; mínimo de 5 postos; fonte ANP; cálculo Abastevo.",
		SourceHref: "/metodologia/",
	})

	datesAnswer := "Sem pesquisa publicada neste candidato."
	if latest != nil {
		datesAnswer = fmt.Sprintf(
			"Pesquisa de %s; coletas de %s; publicação Abastevo em %s. Fonte: ANP.",
			period, collectionRange(observations), valueOr(published, "não publicada"),
		)
	}
	cards = append(cards, insightCard{
		Question:   "De onde vêm e de quando são os dados?",
		Answer:     datesAnswer,
		Caveat:     "Dados datados; não é preço ao vivo.",
		Formula:    "Datas da revisão e das observações; a publicação Abastevo é operacional; fonte ANP.",
		SourceHref: "/metodologia/",
	})

	excluded := 0
	for _, count := range coverage.Excluded {
		excluded += count
	}
	exclusionAnswer := "Sem pesquisa publicada neste candidato."
	if latest != nil {
		if excluded == 0 {
			exclusionAnswer = "Nenhuma observação da revisão foi descartada."
		} else {
			reasons := make([]string, 0, len(coverage.Excluded))
			for code, count := range coverage.Excluded {
				reasons = append(reasons, fmt.Sprintf("%s: %d", exclusionLabel(code), count))
			}
			sort.Strings(reasons)
			exclusionAnswer = fmt.Sprintf(
				"Observações descartadas: %d. %s. Cada linha da revisão foi incluída ou contabilizada em uma categoria.",
				excluded, strings.Join(reasons, "; "),
			)
		}
	}
	cards = append(cards, insightCard{
		Question:   "O que ficou fora da amostra?",
		Answer:     exclusionAnswer,
		Caveat:     "Os descartes preservam a integridade da amostra; consulte a metodologia.",
		Formula:    "Censo das linhas da revisão; cada linha incluída ou em uma categoria de descarte; fonte ANP.",
		SourceHref: "/metodologia/",
	})
	return cards
}

func movementValue(value *int64) string {
	if value == nil {
		return "sem comparação"
	}
	return movement(*value)
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// buildOfficialInsights answers the approved official-records questions from
// the frozen export manifest: event counts with denominators, coverage
// (edition/period/clock), and the meaning of association states. It states
// no guilt, current quality, interdiction or station ranking: a fine is one
// historical event, and absence from the slice is not a clean certificate.
func buildOfficialInsights(official *officialDataset) []insightCard {
	if official == nil {
		return nil
	}
	manifest := official.Manifest
	census := manifest.AssociationCensus
	return []insightCard{
		{
			Question: "Quantos registros oficiais há nesta edição",
			Answer: fmt.Sprintf(
				"A edição %s publica %d %s: %d vinculados, %d sem vínculo e %d com vínculo ambíguo.",
				valueOr(manifest.SourcePeriod, manifest.Edition),
				manifest.Rows, plural(manifest.Rows, "registro", "registros"),
				census.Linked, census.Unmatched, census.Ambiguous),
			Caveat:     "Multa não prova adulteração, culpa ou definitividade; ausência neste recorte não é certidão negativa.",
			Formula:    "Contagem dos eventos publicados na revisão, por estado de associação; fonte ANP.",
			SourceHref: manifest.SourceURL,
		},
		{
			Question: "O que cobre a fiscalização publicada",
			Answer: fmt.Sprintf(
				"Retrato anual da edição %s, extração %s (precisão %s). Não é o histórico completo de cada empresa nem a situação atual.",
				valueOr(manifest.SourcePeriod, manifest.Edition),
				valueOr(manifest.SourceClockValue, "não informada"),
				valueOr(manifest.SourceClockPrecision, "não informada")),
			Caveat:     "Registros posteriores à extração não aparecem; uma fonte mais nova exige nova revisão.",
			Formula:    "Edição anual mais mês de extração da fonte; relógio da fonte, não da publicação Abastevo.",
			SourceHref: manifest.SourceURL,
		},
		{
			Question: "O que significa o vínculo de um registro",
			Answer: fmt.Sprintf(
				"Vinculado indica empresa atual na data da decisão (%s); sem vínculo ou ambíguo indicam que nenhuma empresa única pôde ser afirmada. Vínculo não prova operação histórica no local.",
				valueOr(manifest.AssociationDecisionDay, "data da decisão")),
			Caveat:     "A associação congela na revisão publicada; mudanças posteriores no cadastro exigem nova revisão.",
			Formula:    "Decisão de associação da revisão sobre o CNPJ integral e o cadastro vigente.",
			SourceHref: manifest.SourceURL,
		},
	}
}
