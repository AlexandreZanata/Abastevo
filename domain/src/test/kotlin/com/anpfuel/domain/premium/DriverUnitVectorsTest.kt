package com.anpfuel.domain.premium

import java.io.File
import java.time.Instant
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.TestFactory

/** DRV-02-T03: same rows as backend/internal/modules/metrics/driver_unit_vectors_test.go. */
class DriverUnitVectorsTest {
    private fun kv(s: String) = s.split(';').associate { it.substringBefore('=') to it.substringAfter('=', "") }
    private fun fill(s: String) = s.split(':').let {
        PremiumFill(it[0].toLong(), it[1].toLong(), it[2] == "F", PremiumFuel.GASOLINE_REGULAR, PremiumUnit.LITRE)
    }

    @TestFactory
    fun sharedUnitVectors(): List<DynamicTest> {
        val lines = File("../contracts/testdata/driver-suite/driver-unit-v1.psv").readLines()
        assertEquals("id|operation|args|expected", lines.first())
        val rows = lines.drop(1).map { it.split('|') }
        require(rows.all { it.size == 4 && it[0].isNotEmpty() })
        assertEquals(19, rows.size)
        assertEquals(rows.size, rows.map { it[0] }.toSet().size)
        return rows.map { (id, op, rawArgs, rawWant) ->
            DynamicTest.dynamicTest(id) {
                val args = kv(rawArgs)
                val want = kv(rawWant)
                val got: Map<String, String> = when (op) {
                    "KM_INPUT" -> mapOf(
                        "metres" to (LedgerDistance.parseKilometres(args.getValue("input"))?.toString() ?: "REFUSED"),
                    ) + (want["display"]?.let {
                        mapOf("display" to LedgerDistance.kilometresInput(want.getValue("metres").toLong(), ','))
                    } ?: emptyMap())
                    "INTERVAL" -> PremiumConsumption.compute(
                        PremiumInterval(fill(args.getValue("anchor")), args.getValue("fills").split(',').map(::fill),
                            args.getValue("distance").toLong()),
                    ).let {
                        if (it.unknown.isNotEmpty()) mapOf("unknown" to it.unknown) else mapOf(
                            "unknown" to "", "quantity" to "${it.eligibleQuantityMilli}", "cost" to "${it.eligibleCostCentavos}",
                            "distance" to "${it.distanceMetres}", "km_per_unit" to it.kmPerUnit, "cost_per_km" to it.centavosPerKm,
                        )
                    }
                    "COST" -> PremiumConsumption.fuelPurchasePerKm(args.getValue("total").toLong(), args.getValue("distance").toLong())
                        .let { mapOf("unknown" to it.unknown, "cost_per_km" to it.centavosPerKm, "method" to it.method) }
                    "SUM" -> PremiumConsumption.sumIntervals(args.getValue("windows").split(',').map {
                        val (d, q) = it.split(':')
                        PremiumConsumptionResult(eligibleQuantityMilli = q.toLong(), distanceMetres = d.toLong())
                    }).let { mapOf("km_per_unit" to it.kmPerUnit) }
                    "LEGACY_REVIEW" -> {
                        val draft = RefuelDraft("11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222",
                            Instant.parse("2026-03-01T12:00:00Z"), "UTC", PremiumFuel.ETHANOL, 40000, 26000, null,
                            args.getValue("original").toLong(), LedgerFill.FULL, null, null)
                        mapOf("corrected" to LegacyOdometerReview.preview(draft, OdometerReviewChoice.valueOf(args.getValue("choice"))).correctedMetres.toString())
                    }
                    else -> error("$id: unknown operation $op")
                }
                for ((key, expected) in want) {
                    if (key.startsWith("display_")) continue // BRL display is asserted by the app formatter test
                    assertEquals(expected, got[key], "$id $key")
                }
            }
        }
    }
}
