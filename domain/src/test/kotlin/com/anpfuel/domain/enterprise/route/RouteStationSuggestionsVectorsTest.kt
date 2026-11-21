package com.anpfuel.domain.enterprise.route

import java.io.File
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class RouteStationSuggestionsVectorsTest {
    private fun loadVectors(): List<String> {
        val candidates = listOf(
            File("../contracts/testdata/enterprise/route/suggestions-v1.psv"),
            File("contracts/testdata/enterprise/route/suggestions-v1.psv"),
        )
        val file = candidates.firstOrNull(File::isFile)
            ?: error("Missing suggestions-v1.psv fixture")
        return file.readLines().drop(1).filter { it.isNotBlank() }
    }

    private fun parseMap(raw: String): Map<String, String> =
        raw.split(';').associate { part ->
            val (k, v) = part.split('=', limit = 2)
            k to v
        }

    @Test
    fun `matches canonical section 10-7 and neutrality vectors`() {
        val rows = loadVectors()
        assertEquals(7, rows.size)

        for (line in rows) {
            val parts = line.split('|')
            val id = parts[0]
            val op = parts[1]
            val args = parseMap(parts[2])
            val expected = parseMap(parts[3])

            val product = args.getValue("product")
            val volumeMl = args.getValue("volume_ml").toLong()
            val baseM = args.getValue("base_m").toLong()
            val baseS = args.getValue("base_s").toLong()
            val cons = if (args["cons_m"] == "NONE") null else AssetConsumptionBasis(
                distanceMetres = args.getValue("cons_m").toLong(),
                volumeMilliLitres = args.getValue("cons_ml").toLong(),
            )
            val freshLimit = args.getValue("fresh_limit_s").toLong()

            if (op == "EVALUATE") {
                val ref = SuggestionPriceReference(
                    sourceKind = SuggestionPriceSource.parse(args.getValue("ref_source")),
                    product = args.getValue("ref_product"),
                    unit = args.getValue("ref_unit"),
                    paymentCondition = args.getValue("ref_pay"),
                    priceMilliBrl = args.getValue("ref_price").toLong(),
                    ageSeconds = args.getValue("ref_age_s").toLong(),
                )
                val candPrice = SuggestionPriceReference(
                    sourceKind = SuggestionPriceSource.parse(args.getValue("cand_source")),
                    product = args.getValue("cand_product"),
                    unit = args.getValue("cand_unit"),
                    paymentCondition = args.getValue("cand_pay"),
                    priceMilliBrl = args.getValue("cand_price").toLong(),
                    ageSeconds = args.getValue("cand_age_s").toLong(),
                )
                val result = RouteStationSuggestions.evaluate(
                    product = product,
                    plannedVolumeMl = volumeMl,
                    baseMetres = baseM,
                    baseSeconds = baseS,
                    consumption = cons,
                    freshnessLimitSeconds = freshLimit,
                    reference = ref,
                    candidate = RouteStationCandidate(
                        stationId = "cand-$id",
                        stationName = "Station $id",
                        price = candPrice,
                        routeTotalMetres = args.getValue("cand_m").toLong(),
                        routeTotalSeconds = args.getValue("cand_s").toLong(),
                        hasPaidPlan = args.getValue("paid_plan").toBooleanStrict(),
                        hasActiveAgreement = args.getValue("agreement").toBooleanStrict(),
                    ),
                )
                assertEquals(expected.getValue("comparable").toBooleanStrict(), result.comparable, "$id comparable")
                assertEquals(expected.getValue("detour_m").toLong(), result.detourMetres, "$id detour_m")
                assertEquals(expected.getValue("detour_s").toLong(), result.detourSeconds, "$id detour_s")
                assertEquals(expected.getValue("detour_exceeds").toBooleanStrict(), result.detourExceedsDiff, "$id detour_exceeds")
                assertEquals(expected.getValue("ref_cost_centavos").toLong(), result.referenceCostCentavos, "$id ref_cost")
                assertEquals(expected.getValue("cand_cost_centavos").toLong(), result.candidateCostCentavos, "$id cand_cost")
                assertOptLong("$id detour_fuel_ml", expected.getValue("detour_fuel_ml"), result.detourFuelMilliLitres)
                assertOptLong("$id detour_cost", expected.getValue("detour_cost_centavos"), result.detourCostCentavos)
                assertOptLong("$id gross_diff", expected.getValue("gross_diff_centavos"), result.grossDiffCentavos)
                assertOptLong("$id net_diff", expected.getValue("net_diff_centavos"), result.netDiffCentavos)
            } else if (op == "RANK") {
                val ref = SuggestionPriceReference(SuggestionPriceSource.COMMUNITY, product, "LITRE", "CASH", 6600L, 1000L)
                val a = RouteStationSuggestions.evaluate(
                    product, volumeMl, baseM, baseS, cons, freshLimit, ref,
                    RouteStationCandidate(
                        stationId = args.getValue("a_id"),
                        stationName = "Paid",
                        price = SuggestionPriceReference(SuggestionPriceSource.COMMUNITY, product, "LITRE", "CASH", args.getValue("a_price").toLong(), 1000L),
                        routeTotalMetres = args.getValue("a_m").toLong(),
                        routeTotalSeconds = baseS,
                        hasPaidPlan = args.getValue("a_paid").toBooleanStrict(),
                        hasActiveAgreement = args.getValue("a_agree").toBooleanStrict(),
                    ),
                )
                val b = RouteStationSuggestions.evaluate(
                    product, volumeMl, baseM, baseS, cons, freshLimit, ref,
                    RouteStationCandidate(
                        stationId = args.getValue("b_id"),
                        stationName = "Free",
                        price = SuggestionPriceReference(SuggestionPriceSource.COMMUNITY, product, "LITRE", "CASH", args.getValue("b_price").toLong(), 1000L),
                        routeTotalMetres = args.getValue("b_m").toLong(),
                        routeTotalSeconds = baseS,
                        hasPaidPlan = args.getValue("b_paid").toBooleanStrict(),
                        hasActiveAgreement = args.getValue("b_agree").toBooleanStrict(),
                    ),
                )
                val ranked = RouteStationSuggestions.rank(listOf(a, b))
                assertEquals(expected.getValue("first"), ranked[0].stationId)
                assertEquals(expected.getValue("second"), ranked[1].stationId)
                assertTrue(ranked[0].comparable)
                assertFalse(ranked[0].detourExceedsDiff)
            } else {
                error("Unknown op $op")
            }
        }
    }

    private fun assertOptLong(msg: String, raw: String, actual: Long?) {
        if (raw == "NONE") assertNull(actual, msg) else assertEquals(raw.toLong(), actual, msg)
    }
}
