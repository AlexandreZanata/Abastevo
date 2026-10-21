package com.anpfuel.domain.premium

import java.io.File
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.TestFactory

/** Shared input/output proof with Go; no duplicated financial examples in the harness. */
class FinancialExecutionVectorsTest {
    @TestFactory
    fun sharedExecutionVectors(): List<DynamicTest> {
        val rows = File("../contracts/testdata/premium/financial-execution-v1.csv").readLines().map { it.split(',') }
        val header = "id,operation,anchor,fills,distance,fuel_cost,expenses,window_start_odo,window_end_odo,window_start_ms,window_end_ms,has_gap,crosses_segment,odometer_missing,expected_quantity,expected_cost,expected_distance,expected_efficiency,expected_cost_per_km,expected_change,expected_unknown".split(',')
        assertEquals(header, rows.first())
        assertEquals(37, rows.size)
        val inputs = rows.drop(1)
        require(inputs.all { it.size == header.size && it[0].isNotEmpty() })
        assertEquals(inputs.size, inputs.map { it[0] }.toSet().size)
        return inputs.map { row ->
            val v = header.zip(row).toMap()
            DynamicTest.dynamicTest(v.getValue("id")) {
                fun value(name: String) = v.getValue(name)
                fun number(name: String) = numberValue(value(name))
                var change = ""
                val got = when (value("operation")) {
                    "INTERVAL" -> PremiumConsumption.compute(
                        PremiumInterval(
                            startingFillExcluded = fill(value("anchor")),
                            fills = items(value("fills")).map(::fill),
                            distanceMetres = number("distance"),
                            hasGap = value("has_gap") == "1",
                            crossesSegment = value("crosses_segment") == "1",
                            odometerMissing = value("odometer_missing") == "1",
                        ),
                    )
                    "SUM", "COMPARE" -> {
                        val results = items(value("fills")).map {
                            val p = parts(it, 4)
                            PremiumConsumptionResult(
                                distanceMetres = numberValue(p[0]), eligibleQuantityMilli = numberValue(p[1]),
                                eligibleCostCentavos = numberValue(p[2]), unknown = p[3].replace("_", ""),
                            )
                        }
                        if (value("operation") == "SUM") {
                            val a = PremiumConsumption.sumIntervals(results)
                            PremiumConsumptionResult(a.eligibleQuantityMilli, a.eligibleCostCentavos, a.distanceMetres, a.kmPerUnit, a.centavosPerKm, a.unknown)
                        } else {
                            require(results.size == 2)
                            val a = PremiumConsumption.compare(results[0], results[1])
                            change = a.changePercent
                            PremiumConsumptionResult(unknown = a.unknown)
                        }
                    }
                    "MATCHED", "COST" -> {
                        val a = if (value("operation") == "COST") {
                            PremiumConsumption.fuelPurchasePerKm(number("fuel_cost"), number("distance"))
                        } else {
                            val expenses = items(value("expenses")).map {
                                val p = parts(it, 4)
                                PremiumExpenseEntry(p[0], numberValue(p[1]), numberValue(p[2]), if (p[3] == "_") null else numberValue(p[3]))
                            }
                            PremiumConsumption.matchedWindowCost(
                                number("fuel_cost"), number("distance"), expenses,
                                PremiumWindow(number("window_start_odo"), number("window_end_odo"), number("window_start_ms"), number("window_end_ms"), value("has_gap") == "1", value("crosses_segment") == "1"),
                            )
                        }
                        PremiumConsumptionResult(eligibleCostCentavos = a.totalCentavos, distanceMetres = a.distanceMetres, centavosPerKm = a.centavosPerKm, unknown = a.unknown)
                    }
                    else -> error("Unknown fixture operation: ${value("operation")}")
                }
                assertEquals(
                    PremiumConsumptionResult(number("expected_quantity"), number("expected_cost"), number("expected_distance"), value("expected_efficiency"), value("expected_cost_per_km"), value("expected_unknown")),
                    got,
                )
                assertEquals(value("expected_change"), change)
            }
        }
    }

    private fun numberValue(s: String): Long = if (s.isEmpty()) 0L else s.toLong()

    private fun parts(s: String, count: Int): List<String> = s.split(':').also { require(it.size == count) }

    private fun fill(s: String): PremiumFill {
        val p = parts(s, 5)
        require(p[2] == "0" || p[2] == "1")
        return PremiumFill(numberValue(p[0]), numberValue(p[1]), p[2] == "1", PremiumFuel.parse(p[3]), PremiumUnit.parse(p[4]))
    }

    private fun items(s: String): List<String> {
        if (s.isEmpty()) return emptyList()
        val result = mutableListOf<String>()
        s.split(';').forEach { item ->
            val p = item.split('*', limit = 2)
            val count = if (p.size == 2) p[0].toInt() else 1
            val body = p.last()
            require(count in 1..20000 && result.size + count <= 20000)
            repeat(count) { result.add(body.replace("#", it.toString())) }
        }
        return result
    }
}
