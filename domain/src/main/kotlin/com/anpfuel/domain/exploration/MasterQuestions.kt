package com.anpfuel.domain.exploration

import java.time.DayOfWeek
import java.time.Instant
import java.time.ZoneOffset
import java.time.temporal.TemporalAdjusters

/**
 * MEX-31 master questions: deterministic on-device Portuguese
 * interpretation, a faithful port of backend
 * `internal/modules/exploration` (same vocabularies, same precedence,
 * same reasons). No network, database, clock reads (beyond the caller
 * supplied instant) or model calls. Adversarial text is matched
 * literally, never run.
 *
 * One deliberate determinism fix over the Go original: UF-name matching
 * tries longest names first instead of map order, so "paraíba" cannot
 * randomly resolve to Pará.
 */

const val POLICY_VERSION = "mex-exploration-v1"

enum class InterpretationState { RESOLVED, NEEDS_CHOICE, UNSUPPORTED }

object Reason {
    const val METRIC_REQUIRED = "METRIC_REQUIRED"
    const val SOURCE_REQUIRED = "SOURCE_REQUIRED"
    const val FUEL_REQUIRED = "FUEL_REQUIRED"
    const val CONDITION_REQUIRED = "CONDITION_REQUIRED"
    const val PERIOD_REQUIRED = "PERIOD_REQUIRED"
    const val CONNECTOR_REQUIRED = "CONNECTOR_REQUIRED"
    const val COMPONENT_REQUIRED = "COMPONENT_REQUIRED"
    const val UNSUPPORTED_INTENT = "UNSUPPORTED_INTENT"
    const val UNKNOWN_FUEL = "UNKNOWN_FUEL"
    const val UNSUPPORTED_FILTER = "UNSUPPORTED_FILTER"
    const val UNKNOWN_SCHEMA = "UNKNOWN_SCHEMA"
}

object Kind {
    const val PRICE_RANKING = "PRICE_RANKING"
    const val OFFICIAL_RECORDS = "OFFICIAL_RECORDS"
    const val CHARGING_COMPATIBLE_PLACES = "CHARGING_COMPATIBLE_PLACES"
    const val REPORTED_ENERGY_PRICE = "REPORTED_ENERGY_PRICE"
    const val COMPLETE_SCENARIO_COST = "COMPLETE_SCENARIO_COST"
}

data class Geography(val level: String, val country: String, val uf: String = "", val municipality: String = "")
data class Page(val limit: Int)
data class PriceMetric(val kind: String, val groupBy: String, val sort: String, val weighting: String = "")
data class PriceSource(val kind: String, val surveyWeekStart: String = "", val eligibility: String = "")
data class ProductScope(val fuel: String, val unit: String)
data class Condition(val kind: String, val qualifierId: String = "")

data class PriceQuery(
    val version: Int = 1, val kind: String = Kind.PRICE_RANKING,
    val source: PriceSource, val product: ProductScope, val condition: Condition,
    val geography: Geography, val metric: PriceMetric, val page: Page = Page(20),
    val policy: String = POLICY_VERSION,
)

data class RecordScope(val kind: String, val stationId: String = "", val geographyBasis: String)
data class DateFilter(val basis: String, val precision: String, val from: String, val toExclusive: String)
data class StatusFilter(val kind: String, val reportedValue: String = "")

data class OfficialQuery(
    val version: Int = 1, val kind: String = Kind.OFFICIAL_RECORDS,
    val sources: List<String>, val recordKinds: List<String>, val scope: RecordScope,
    val geography: Geography, val date: DateFilter, val status: StatusFilter,
    val page: Page = Page(20), val policy: String = POLICY_VERSION,
)

data class ChargingPlaceQuery(
    val version: Int = 1, val kind: String = Kind.CHARGING_COMPATIBLE_PLACES,
    val profile: String = "ocm-br-20261021-v1", val connector: String = "",
    val current: String = "", val access: String = "", val geography: Geography,
    val page: Page = Page(20), val policy: String = POLICY_VERSION,
)

data class EnergyPriceQuery(
    val version: Int = 1, val kind: String = Kind.REPORTED_ENERGY_PRICE,
    val profile: String = "ocm-br-20261021-v1", val condition: Condition,
    val geography: Geography, val sort: String, val page: Page = Page(20),
    val policy: String = POLICY_VERSION,
)

data class ScenarioCostQuery(
    val version: Int = 1, val kind: String = Kind.COMPLETE_SCENARIO_COST,
    val profile: String = "ocm-br-20261021-v1", val dimensions: Set<String>,
    val missing: List<String>, val condition: Condition, val geography: Geography,
    val page: Page = Page(20), val policy: String = POLICY_VERSION,
)

data class Interpretation(
    val state: InterpretationState,
    val query: Any? = null,
    val reason: String = "",
    val alternatives: List<Any> = emptyList(),
)

object MasterQuestions {

    const val MAX_QUESTION_RUNES = 500

    private data class Fuel(val fuel: String, val unit: String, val fragments: List<String>)

    private val fuels = listOf(
        Fuel("GASOLINE_REGULAR", "L", listOf("gasolina comum", "gasolina sem aditivo", "gasolina")),
        Fuel("GASOLINE_ADDITIVED", "L", listOf("gasolina aditivada", "gasolina com aditivo", "aditivada")),
        Fuel("GASOLINE_PREMIUM_GRADE", "L", listOf("gasolina premium", "gasolina podium", "podium", "premium")),
        Fuel("ETHANOL", "L", listOf("etanol", "alcool", "alcool hidratado")),
        Fuel("DIESEL_S10", "L", listOf("diesel s10", "s10")),
        Fuel("DIESEL_S500", "L", listOf("diesel s500", "s500", "diesel comum")),
        Fuel("DIESEL", "L", listOf("diesel", "oleo diesel")),
        Fuel("CNG", "M3", listOf("gnv", "gas natural veicular", "gas natural")),
        Fuel("LPG_P13", "KG_13", listOf("glp", "gas de cozinha", "gas de botijao", "botijao", "p13")),
    )

    private val unknownFuels = listOf(
        "eletricidade", "eletrico", "querosene", "arla", "ureia", "biodiesel",
        "querosene de aviacao", "gasolina de aviacao", "oleo lubrificante", "graxa",
    )

    private val ufs = mapOf(
        "acre" to "AC", "alagoas" to "AL", "amapa" to "AP", "amazonas" to "AM", "bahia" to "BA",
        "ceara" to "CE", "distrito federal" to "DF", "brasilia" to "DF",
        "espirito santo" to "ES", "goias" to "GO", "maranhao" to "MA",
        "mato grosso" to "MT", "mato grosso do sul" to "MS", "minas gerais" to "MG",
        "para" to "PA", "paraiba" to "PB", "parana" to "PR", "pernambuco" to "PE",
        "piaui" to "PI", "rio de janeiro" to "RJ", "rio grande do norte" to "RN",
        "rio grande do sul" to "RS", "rondonia" to "RO", "roraima" to "RR",
        "santa catarina" to "SC", "sao paulo" to "SP", "sergipe" to "SE", "tocantins" to "TO",
    )

    private val ufCodes: Set<String> by lazy {
        (ufs.values + listOf("AC", "AL", "AM", "AP", "BA", "CE", "DF", "ES", "GO", "MA", "MG", "MS", "MT", "PA", "PB", "PE", "PI", "PR", "RJ", "RN", "RO", "RR", "RS", "SC", "SE", "SP", "TO")).toSet()
    }

    private val accents = mapOf(
        'á' to 'a', 'à' to 'a', 'â' to 'a', 'ã' to 'a', 'ä' to 'a',
        'é' to 'e', 'è' to 'e', 'ê' to 'e', 'ë' to 'e',
        'í' to 'i', 'ì' to 'i', 'î' to 'i', 'ï' to 'i',
        'ó' to 'o', 'ò' to 'o', 'ô' to 'o', 'õ' to 'o', 'ö' to 'o',
        'ú' to 'u', 'ù' to 'u', 'û' to 'u', 'ü' to 'u',
        'ç' to 'c', 'ñ' to 'n',
    )

    private val connectors = listOf(
        "CCS2" to listOf("ccs2", "ccs 2", "combo 2", "combo2"),
        "TYPE2" to listOf("tipo 2", "tipo2", "type 2", "mennekes"),
        "CHADEMO" to listOf("chademo", "cha de mo"),
        "NACS" to listOf("nacs", "tesla", "padrao tesla"),
    )

    private val scenarioDimensions = listOf(
        "ENERGY_WH" to listOf("kwh", "kw h", "quilowatt", "energia", "carga"),
        "CHARGING_SECOND" to listOf("tempo de recarga", "tempo carregando", "duracao da recarga", "minutos de recarga", "horas de recarga"),
        "PARKING_SECOND" to listOf("estacionamento", "estacionar", "tempo estacionado"),
        "SESSION" to listOf("sessao", "sessoes", "por recarga", "por carga"),
    )

    fun interpret(question: String, now: Instant): Interpretation {
        val normalized = normalize(question) ?: return Interpretation(InterpretationState.UNSUPPORTED, reason = Reason.UNSUPPORTED_INTENT)
        if (normalized.isEmpty()) return Interpretation(InterpretationState.UNSUPPORTED, reason = Reason.UNSUPPORTED_INTENT)
        val price = priceScore(normalized)
        val official = officialScore(normalized)
        val electric = evScore(normalized)
        if (price == 0 && official == 0 && electric == 0) {
            return Interpretation(InterpretationState.UNSUPPORTED, reason = Reason.UNSUPPORTED_INTENT)
        }
        if (electric > price && electric > official) return interpretEv(normalized)
        if (official > price) return interpretOfficial(normalized)
        return interpretPrice(normalized, now)
    }

    internal fun normalize(question: String): String? {
        if (question.codePointCount(0, question.length) > MAX_QUESTION_RUNES) return null
        val lowered = question.lowercase(java.util.Locale.ROOT)
        val b = StringBuilder()
        var previousSpace = true
        var i = 0
        while (i < lowered.length) {
            val code = lowered.codePointAt(i)
            val r: Char = if (code < 0x80) {
                val plain = code.toChar()
                if ((plain in 'a'..'z') || (plain in '0'..'9')) plain else ' '
            } else {
                accents[code.toChar()] ?: ' '
            }
            if (r == ' ') {
                if (!previousSpace) b.append(' ')
                previousSpace = true
            } else {
                b.append(r)
                previousSpace = false
            }
            i += Character.charCount(code)
        }
        val out = b.toString().trim()
        return out.ifEmpty { null }
    }

    private fun contains(normalized: String, vararg fragments: String): Boolean =
        fragments.any { normalized.contains(it) }

    private fun containsWord(normalized: String, word: String): Boolean =
        normalized.split(' ').any { it == word }

    private fun latestSunday(now: Instant): String {
        val date = now.atZone(ZoneOffset.UTC).toLocalDate()
        val sunday = date.with(TemporalAdjusters.previousOrSame(DayOfWeek.SUNDAY))
        return sunday.toString()
    }

    private fun sortDirection(normalized: String): String =
        if (contains(normalized, "barato", "barata", "menor", "mais barato", "mais barata", "cheapest", "lowest")) "ASC" else "DESC"

    private fun extractGeography(normalized: String): Geography {
        for (code in ufCodes.sorted()) {
            if (containsWord(normalized, code.lowercase())) return Geography("STATE", "BR", code)
        }
        for (name in ufs.keys.sortedByDescending { it.length }) {
            if (normalized.contains(name)) return Geography("STATE", "BR", ufs.getValue(name))
        }
        val fields = normalized.split(' ')
        for (i in fields.indices) {
            if (fields[i] == "em" && i + 1 < fields.size) {
                val candidate = fields.drop(i + 1).joinToString(" ").trimEnd(' ', '?', '!', '.')
                if (candidate.isNotEmpty() && candidate.length <= 80) {
                    return Geography("MUNICIPALITY", "BR", municipality = candidate)
                }
            }
        }
        return Geography("COUNTRY", "BR")
    }

    private fun extractFuel(normalized: String): Triple<String, String, String> {
        var bestFuel = ""
        var bestUnit = ""
        var bestLen = 0
        for (f in fuels) {
            for (fragment in f.fragments) {
                if (normalized.contains(fragment) && fragment.length > bestLen) {
                    bestFuel = f.fuel
                    bestUnit = f.unit
                    bestLen = fragment.length
                }
            }
        }
        if (bestFuel.isNotEmpty()) return Triple(bestFuel, bestUnit, "")
        if (contains(normalized, *unknownFuels.toTypedArray())) return Triple("", "", "unknown")
        return Triple("", "", "missing")
    }

    private fun priceScore(normalized: String): Int {
        var score = 0
        for (f in fuels) {
            for (fragment in f.fragments) {
                if (normalized.contains(fragment)) {
                    score += 3
                    break
                }
            }
        }
        if (contains(normalized, "preco", "precos", "caro", "cara", "barato", "barata", "media", "ranking", "valor", "valores", "combustivel", "combustiveis")) score++
        if (contains(normalized, "maior", "menor", "mais caro", "mais barata", "mais barato")) score++
        return score
    }

    private fun officialScore(normalized: String): Int {
        var score = 0
        if (contains(normalized, "fiscalizacao", "fiscalizacoes", "acao fiscal", "acoes fiscais", "documento de fiscalizacao")) score += 3
        if (contains(normalized, "auto de infracao", "autos de infracao", "infracao", "infracoes", "multa", "multas", "autuado", "autuacao")) score += 3
        if (contains(normalized, "analise de qualidade", "analises", "ensaio", "ensaios", "qualidade do combustivel", "amostra")) score += 3
        if (contains(normalized, "registros da anp", "registros oficiais", "acoes da anp")) score++
        return score
    }

    private fun evScore(normalized: String): Int {
        for (fragment in listOf("carregar", "carregando", "recarga", "recarregar", "eletroposto", "eletropostos", "carregador", "carregadores", "ponto de recarga", "pontos de recarga", "kwh", "conector", "conectores", "chademo", "ccs", "tipo 2", "nacs", "tomada tipo 2")) {
            if (normalized.contains(fragment)) return 3
        }
        if (contains(normalized, "bateria", "autonomia", "eletrico", "eletrica", "eletromobilidade")) return 1
        return 0
    }

    private fun extractConnector(normalized: String): Pair<String, Boolean> {
        var best = ""
        var bestLen = 0
        for ((code, fragments) in connectors) {
            for (fragment in fragments) {
                if (normalized.contains(fragment) && fragment.length > bestLen) {
                    best = code
                    bestLen = fragment.length
                }
            }
        }
        return best to (best == "")
    }

    private fun unsupportedCondition(normalized: String): Boolean =
        contains(normalized, "pix", "boleto", "transferencia", "criptomoeda", "bitcoin", "vale", "cupom")

    private fun extractCondition(normalized: String): Condition = when {
        contains(normalized, "aplicativo", "app de desconto", "fidelidade", "programa de fidelidade") -> Condition("APP")
        contains(normalized, "dinheiro", "a vista", "pagamento em dinheiro") -> Condition("CASH")
        contains(normalized, "debito", "cartao de debito") -> Condition("DEBIT")
        contains(normalized, "credito", "cartao de credito") -> Condition("CREDIT")
        else -> Condition("STANDARD")
    }

    private fun interpretPrice(normalized: String, now: Instant): Interpretation {
        val (fuel, unit, fuelState) = extractFuel(normalized)
        if (fuelState == "unknown") return Interpretation(InterpretationState.UNSUPPORTED, reason = Reason.UNKNOWN_FUEL)
        if (fuelState == "missing") {
            val week = latestSunday(now)
            return Interpretation(
                InterpretationState.NEEDS_CHOICE, reason = Reason.FUEL_REQUIRED,
                alternatives = listOf(
                    priceQuery("GASOLINE_REGULAR", "L", observationMetric(sortDirection(normalized)), extractGeography(normalized), week),
                    priceQuery("ETHANOL", "L", observationMetric(sortDirection(normalized)), extractGeography(normalized), week),
                    priceQuery("DIESEL_S10", "L", observationMetric(sortDirection(normalized)), extractGeography(normalized), week),
                ),
            )
        }
        if (fuel == "DIESEL") {
            val geography = extractGeography(normalized)
            val week = latestSunday(now)
            return Interpretation(
                InterpretationState.NEEDS_CHOICE, reason = Reason.FUEL_REQUIRED,
                alternatives = listOf(
                    priceQuery("DIESEL_S10", "L", observationMetric(sortDirection(normalized)), geography, week),
                    priceQuery("DIESEL_S500", "L", observationMetric(sortDirection(normalized)), geography, week),
                ),
            )
        }
        if (unsupportedCondition(normalized)) {
            return Interpretation(InterpretationState.UNSUPPORTED, reason = Reason.UNSUPPORTED_FILTER)
        }
        val condition = extractCondition(normalized)
        val geography = extractGeography(normalized)
        val week = latestSunday(now)
        val metric = extractMetric(normalized)
        if (metric == null) {
            return Interpretation(
                InterpretationState.NEEDS_CHOICE, reason = Reason.METRIC_REQUIRED,
                alternatives = listOf(
                    priceQuery(fuel, unit, observationMetric(sortDirection(normalized)), geography, week),
                    priceQuery(fuel, unit, meanByStateMetric(sortDirection(normalized)), geography, week),
                ),
            )
        }
        return Interpretation(InterpretationState.RESOLVED, query = priceQueryWithCondition(fuel, unit, metric, geography, week, condition))
    }

    private fun observationMetric(sort: String) = PriceMetric("OBSERVATION_PRICE", "STATION", sort)
    private fun meanByStateMetric(sort: String) = PriceMetric("AREA_MEAN", "STATE", sort, "ELIGIBLE_SOURCE_OBSERVATIONS")

    private fun extractMetric(normalized: String): PriceMetric? {
        val sort = sortDirection(normalized)
        val hasObservation = contains(normalized, "observado", "observada", "observados", "posto", "postos", "bomba")
        val hasMean = contains(normalized, "media", "medias", "promedio")
        val hasState = contains(normalized, "estado", "estados", "uf", "ufs")
        val hasCity = contains(normalized, "municipio", "municipios", "cidade", "cidades")
        return when {
            hasObservation && !hasMean -> observationMetric(sort)
            hasMean && hasState && !hasCity -> meanByStateMetric(sort)
            hasMean && hasCity && !hasState -> PriceMetric("AREA_MEAN", "MUNICIPALITY", sort, "ELIGIBLE_SOURCE_OBSERVATIONS")
            else -> null
        }
    }

    private fun priceQuery(fuel: String, unit: String, metric: PriceMetric, geography: Geography, week: String): PriceQuery =
        priceQueryWithCondition(fuel, unit, metric, geography, week, Condition("STANDARD"))

    private fun priceQueryWithCondition(fuel: String, unit: String, metric: PriceMetric, geography: Geography, week: String, condition: Condition): PriceQuery =
        PriceQuery(source = PriceSource("ANP_WEEKLY", week), product = ProductScope(fuel, unit), condition = condition, geography = geography, metric = metric)

    private fun interpretOfficial(normalized: String): Interpretation {
        val geography = extractGeography(normalized)
        val enforcement = contains(normalized, "fiscalizacao", "fiscalizacoes", "acao fiscal", "acoes fiscais", "documento de fiscalizacao")
        val infraction = contains(normalized, "auto de infracao", "autos de infracao", "infracao", "infracoes", "multa", "multas", "autuado", "autuacao")
        val assay = contains(normalized, "analise de qualidade", "analises", "ensaio", "ensaios", "qualidade do combustivel", "amostra")
        val count = listOf(enforcement, infraction, assay).count { it }
        if (count == 0) {
            return Interpretation(
                InterpretationState.NEEDS_CHOICE, reason = Reason.SOURCE_REQUIRED,
                alternatives = listOf(officialQuery("enforcement", geography), officialQuery("pmqc", geography), officialQuery("sanctions", geography)),
            )
        }
        if (count > 1) return Interpretation(InterpretationState.UNSUPPORTED, reason = Reason.UNSUPPORTED_INTENT)
        val source = if (enforcement) "enforcement" else if (infraction) "sanctions" else "pmqc"
        return Interpretation(InterpretationState.RESOLVED, query = officialQuery(source, geography))
    }

    private fun officialQuery(source: String, geography: Geography): OfficialQuery {
        val kind = if (source == "pmqc") "ASSAY" else if (source == "sanctions") "FINE_RECORD" else "ENFORCEMENT_ACTION"
        return OfficialQuery(
            sources = listOf(source), recordKinds = listOf(kind),
            scope = RecordScope("SOURCE_GLOBAL", geographyBasis = "SOURCE_REPORTED"),
            geography = geography,
            date = DateFilter("EVENT_DAY", "DAY", "0001-01-01", "9999-12-31"),
            status = StatusFilter("ANY"),
        )
    }

    private fun evPlaceQuery(connector: String, current: String, access: String, geography: Geography) = ChargingPlaceQuery(
        connector = connector, current = current, access = access, geography = geography,
    )

    private fun evPriceQuery(condition: Condition, geography: Geography, sort: String): EnergyPriceQuery {
        val fixed = if (sort == "ASC") "ASC" else "DESC"
        return EnergyPriceQuery(condition = condition, geography = geography, sort = fixed)
    }

    private fun evScenarioQuery(dimensions: Map<String, Boolean>, condition: Condition, geography: Geography): ScenarioCostQuery {
        val known = setOf("ENERGY_WH", "CHARGING_SECOND", "PARKING_SECOND", "SESSION")
        val missing = known.filter { !dimensions.getOrDefault(it, false) }.sorted()
        return ScenarioCostQuery(dimensions = dimensions.keys.toSet(), missing = missing, condition = condition, geography = geography)
    }

    private fun connectorAlternatives(geography: Geography): List<Any> =
        listOf("CCS2", "TYPE2", "CHADEMO", "NACS").map { evPlaceQuery(it, "", "", geography) }

    fun interpretEv(normalized: String): Interpretation {
        val geography = extractGeography(normalized)
        val (connector, missingConnector) = extractConnector(normalized)
        val hasPlace = contains(normalized, "onde", "eletroposto", "eletropostos", "ponto de recarga", "pontos de recarga", "perto", "proximo", "proxima", "estacao de recarga", "endereco")
        val hasPrice = contains(normalized, "preco", "precos", "custa", "custam", "tarifa", "tarifas", "valor", "valores", "caro", "cara")
        val hasScenario = contains(normalized, "total", "completa", "completo", "custo total", "quanto custa carregar", "incluindo", "incluindo estacionamento", "com estacionamento")
        val hasFree = contains(normalized, "gratis", "gratuito", "gratuita", "de graca", "sem pagar", "sem custo")
        val hasCheapest = contains(normalized, "mais barato", "mais barata", "cheapest")
        val hasFastest = contains(normalized, "mais rapid", "mais veloz", "fastest")
        if (unsupportedCondition(normalized)) {
            return Interpretation(InterpretationState.UNSUPPORTED, reason = Reason.UNSUPPORTED_FILTER)
        }
        val condition = extractCondition(normalized)
        if (hasCheapest || hasFastest) {
            return Interpretation(
                InterpretationState.NEEDS_CHOICE, reason = Reason.METRIC_REQUIRED,
                alternatives = listOf(
                    evPriceQuery(condition, geography, sortDirection(normalized)),
                    evPlaceQuery("", "DC", "", geography),
                ),
            )
        }
        if (hasFree && (hasScenario || hasPlace)) {
            return Interpretation(
                InterpretationState.NEEDS_CHOICE, reason = Reason.COMPONENT_REQUIRED,
                alternatives = listOf(
                    evScenarioQuery(mapOf("ENERGY_WH" to true), condition, geography),
                    evScenarioQuery(mapOf("PARKING_SECOND" to true), condition, geography),
                ),
            )
        }
        var count = 0
        if (hasPlace) count++
        if (hasPrice && !hasScenario) count++
        if (hasScenario) count++
        if (count > 1) return Interpretation(InterpretationState.UNSUPPORTED, reason = Reason.UNSUPPORTED_INTENT)
        if (hasScenario) {
            val dimensions = mutableMapOf<String, Boolean>()
            if (contains(normalized, "kwh", "kw h", "quilowatt", "energia", "carga")) dimensions["ENERGY_WH"] = true
            if (contains(normalized, "tempo de recarga", "tempo carregando", "duracao da recarga", "minutos de recarga", "horas de recarga")) dimensions["CHARGING_SECOND"] = true
            if (contains(normalized, "estacionamento", "estacionar", "tempo estacionado")) dimensions["PARKING_SECOND"] = true
            if (contains(normalized, "sessao", "sessoes", "por recarga", "por carga")) dimensions["SESSION"] = true
            if (dimensions.isEmpty()) {
                return Interpretation(
                    InterpretationState.NEEDS_CHOICE, reason = Reason.COMPONENT_REQUIRED,
                    alternatives = listOf(
                        evScenarioQuery(mapOf("ENERGY_WH" to true), condition, geography),
                        evScenarioQuery(mapOf("CHARGING_SECOND" to true), condition, geography),
                        evScenarioQuery(mapOf("PARKING_SECOND" to true), condition, geography),
                    ),
                )
            }
            return Interpretation(InterpretationState.RESOLVED, query = evScenarioQuery(dimensions, condition, geography))
        }
        if (hasPrice) {
            return Interpretation(InterpretationState.RESOLVED, query = evPriceQuery(condition, geography, sortDirection(normalized)))
        }
        if (missingConnector) {
            return Interpretation(InterpretationState.NEEDS_CHOICE, reason = Reason.CONNECTOR_REQUIRED, alternatives = connectorAlternatives(geography))
        }
        var current = ""
        if (contains(normalized, "rapida", "rapido", "fast", "dc", "corrente continua")) current = "DC"
        else if (contains(normalized, "lenta", "lento", "ac", "corrente alternada")) current = "AC"
        return Interpretation(InterpretationState.RESOLVED, query = evPlaceQuery(connector, current, "", geography))
    }
}
