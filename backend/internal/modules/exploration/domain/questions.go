package domain

import (
	"time"
)

// interpretPrice resolves price questions. Fuel, metric and period gaps
// become explicit choices; unknown fuels and conditions are unsupported,
// never guessed or dropped.
func interpretPrice(normalized string, now time.Time) Interpretation {
	fuel, unit, unknownFuel, missingFuel := extractFuel(normalized)
	if unknownFuel {
		return Interpretation{State: StateUnsupported, Reason: ReasonUnknownFuel}
	}
	if missingFuel {
		return Interpretation{State: StateNeedsChoice, Reason: ReasonFuelRequired, Alternatives: []any{
			priceQuery("GASOLINE_REGULAR", "L", observationMetric("DESC"), Geography{Level: "COUNTRY", Country: "BR"}, latestSunday(now)),
			priceQuery("ETHANOL", "L", observationMetric("DESC"), Geography{Level: "COUNTRY", Country: "BR"}, latestSunday(now)),
			priceQuery("DIESEL_S10", "L", observationMetric("DESC"), Geography{Level: "COUNTRY", Country: "BR"}, latestSunday(now)),
		}}
	}
	if fuel == "DIESEL" {
		geography := extractGeography(normalized)
		week := latestSunday(now)
		return Interpretation{State: StateNeedsChoice, Reason: ReasonFuelRequired, Alternatives: []any{
			priceQuery("DIESEL_S10", "L", observationMetric(sortDirection(normalized)), geography, week),
			priceQuery("DIESEL_S500", "L", observationMetric(sortDirection(normalized)), geography, week),
		}}
	}
	if unsupportedCondition(normalized) {
		return Interpretation{State: StateUnsupported, Reason: ReasonUnsupportedFilter}
	}
	condition := extractCondition(normalized)
	geography := extractGeography(normalized)
	week := latestSunday(now)
	metric, metricOK := extractMetric(normalized)
	if !metricOK {
		return Interpretation{State: StateNeedsChoice, Reason: ReasonMetricRequired, Alternatives: []any{
			priceQuery(fuel, unit, observationMetric(sortDirection(normalized)), geography, week),
			priceQuery(fuel, unit, meanByStateMetric(sortDirection(normalized)), geography, week),
		}}
	}
	return Interpretation{State: StateResolved, Query: priceQueryWithCondition(fuel, unit, metric, geography, week, condition)}
}

func priceQuery(fuel, unit string, metric PriceMetric, geography Geography, week string) PriceQuery {
	return priceQueryWithCondition(fuel, unit, metric, geography, week, Condition{Kind: "STANDARD"})
}

func priceQueryWithCondition(fuel, unit string, metric PriceMetric, geography Geography, week string, condition Condition) PriceQuery {
	return PriceQuery{
		Version: 1, Kind: KindPriceRanking,
		Source:  PriceSource{Kind: "ANP_WEEKLY", SurveyWeekStart: week},
		Product: ProductScope{Fuel: fuel, Unit: unit},
		Cond:    condition, Geog: geography, Metric: metric,
		Page:   Page{Limit: 20},
		Policy: PolicyVersion,
	}
}

func observationMetric(sort string) PriceMetric {
	return PriceMetric{Kind: "OBSERVATION_PRICE", GroupBy: "STATION", Sort: sort}
}

func meanByStateMetric(sort string) PriceMetric {
	return PriceMetric{Kind: "AREA_MEAN", GroupBy: "STATE", Sort: sort, Weighting: "ELIGIBLE_SOURCE_OBSERVATIONS"}
}

func sortDirection(normalized string) string {
	if contains(normalized, "barato", "barata", "menor", "mais barato", "mais barata", "cheapest", "lowest") {
		return "ASC"
	}
	return "DESC"
}

// extractMetric maps explicit metric wording to a typed metric. Bare
// "mais caro"/"mais barato"/"media" without a kind is ambiguous.
func extractMetric(normalized string) (PriceMetric, bool) {
	sort := sortDirection(normalized)
	hasObservation := contains(normalized, "observado", "observada", "observados", "posto", "postos", "bomba")
	hasMean := contains(normalized, "media", "medias", "promedio")
	hasState := contains(normalized, "estado", "estados", "uf", "ufs")
	hasCity := contains(normalized, "municipio", "municipios", "cidade", "cidades")
	switch {
	case hasObservation && !hasMean:
		return observationMetric(sort), true
	case hasMean && hasState && !hasCity:
		return meanByStateMetric(sort), true
	case hasMean && hasCity && !hasState:
		return PriceMetric{Kind: "AREA_MEAN", GroupBy: "MUNICIPALITY", Sort: sort, Weighting: "ELIGIBLE_SOURCE_OBSERVATIONS"}, true
	case hasMean && !hasState && !hasCity:
		return PriceMetric{}, false
	case !hasObservation && !hasMean:
		return PriceMetric{}, false
	default:
		return PriceMetric{}, false
	}
}

// extractCondition maps exact condition words. STANDARD is the default
// only when no payment qualifier appears; an unrecognized qualifier is
// refused upstream instead of dropped.
func extractCondition(normalized string) Condition {
	switch {
	case contains(normalized, "aplicativo", "app de desconto", "fidelidade", "programa de fidelidade"):
		return Condition{Kind: "APP"}
	case contains(normalized, "dinheiro", "a vista", "pagamento em dinheiro"):
		return Condition{Kind: "CASH"}
	case contains(normalized, "debito", "cartao de debito"):
		return Condition{Kind: "DEBIT"}
	case contains(normalized, "credito", "cartao de credito"):
		return Condition{Kind: "CREDIT"}
	default:
		return Condition{Kind: "STANDARD"}
	}
}

// unsupportedCondition reports payment qualifiers outside the allowlisted
// conditions. They must not silently fall back to STANDARD.
func unsupportedCondition(normalized string) bool {
	return contains(normalized, "pix", "boleto", "transferencia", "criptomoeda", "bitcoin", "vale", "cupom")
}

// interpretOfficial resolves official-record questions by source type.
// An unclear source becomes an explicit source choice; unsupported
// combinations are refused, never executed.
func interpretOfficial(normalized string) Interpretation {
	geography := extractGeography(normalized)
	enforcement := contains(normalized, "fiscalizacao", "fiscalizacoes", "acao fiscal", "acoes fiscais", "documento de fiscalizacao")
	infraction := contains(normalized, "auto de infracao", "autos de infracao", "infracao", "infracoes", "multa", "multas", "autuado", "autuacao")
	assay := contains(normalized, "analise de qualidade", "analises", "ensaio", "ensaios", "qualidade do combustivel", "amostra")
	count := 0
	for _, present := range []bool{enforcement, infraction, assay} {
		if present {
			count++
		}
	}
	if count == 0 {
		return Interpretation{State: StateNeedsChoice, Reason: ReasonSourceRequired, Alternatives: []any{
			officialQuery("enforcement", geography),
			officialQuery("pmqc", geography),
			officialQuery("sanctions", geography),
		}}
	}
	if count > 1 {
		return Interpretation{State: StateUnsupported, Reason: ReasonUnsupportedIntent}
	}
	switch {
	case enforcement:
		return Interpretation{State: StateResolved, Query: officialQuery("enforcement", geography)}
	case infraction:
		return Interpretation{State: StateResolved, Query: officialQuery("sanctions", geography)}
	default:
		return Interpretation{State: StateResolved, Query: officialQuery("pmqc", geography)}
	}
}

func officialQuery(source string, geography Geography) OfficialQuery {
	recordKind := "ENFORCEMENT_ACTION"
	if source == "pmqc" {
		recordKind = "ASSAY"
	} else if source == "sanctions" {
		recordKind = "FINE_RECORD"
	}
	return OfficialQuery{
		Version: 1, Kind: KindOfficialRecord,
		Sources: []string{source}, RecordKinds: []string{recordKind},
		Scope:  RecordScope{Kind: "SOURCE_GLOBAL", GeographyBasis: "SOURCE_REPORTED"},
		Geog:   geography,
		Date:   DateFilter{Basis: "EVENT_DAY", Precision: "DAY", From: "0001-01-01", ToExclusive: "9999-12-31"},
		Status: StatusFilter{Kind: "ANY"},
		Page:   Page{Limit: 20},
		Policy: PolicyVersion,
	}
}
