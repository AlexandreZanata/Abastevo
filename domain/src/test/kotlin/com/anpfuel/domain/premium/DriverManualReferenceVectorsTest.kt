package com.anpfuel.domain.premium

import java.io.File
import java.math.BigInteger
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.TestFactory

/**
 * Test-only worked reference, not an earnings/shift runtime implementation.
 * The future domain/application/UI consumers must run these same frozen rows.
 * Go merges spans; this independent time reference sweeps boundary events.
 */
class DriverManualReferenceVectorsTest {
    @TestFactory
    fun frozenManualExamples(): List<DynamicTest> {
        val lines = File("../contracts/testdata/driver-suite/driver-manual-reference-v1.psv").readLines()
        assertEquals("id|operation|args|expected", lines.first())
        val rows = lines.drop(1).map { it.split('|') }
        require(rows.all { it.size == 4 && it[0].isNotBlank() })
        assertEquals(15, rows.size)
        assertEquals(rows.size, rows.map { it[0] }.toSet().size)
        return rows.map { (id, operation, args, expected) ->
            DynamicTest.dynamicTest(id) {
                assertEquals(fields(expected), compute(operation, fields(args)), id)
            }
        }
    }

    private fun fields(raw: String): Map<String, String> {
        val parts = raw.split(';').map {
            require(it.contains('='))
            it.substringBefore('=') to it.substringAfter('=')
        }
        require(parts.all { it.first.isNotBlank() } && parts.map { it.first }.toSet().size == parts.size)
        return parts.toMap()
    }

    private fun compute(operation: String, args: Map<String, String>): Map<String, String> {
        fun n(key: String) = args.getValue(key).toBigInteger()
        fun sum(key: String) = args.getValue(key).split(',').fold(BigInteger.ZERO) { acc, s -> acc + s.toBigInteger() }
        fun ratio(amount: BigInteger, scale: Long, denominator: BigInteger): String {
            val numerator = amount * BigInteger.valueOf(scale)
            val divisor = numerator.gcd(denominator)
            val a = numerator / divisor
            val b = denominator / divisor
            return if (b == BigInteger.ONE) a.toString() else "$a/$b"
        }
        fun ratios(amount: BigInteger): MutableMap<String, String> {
            val out = mutableMapOf<String, String>()
            val reasons = mutableListOf<String>()
            for ((input, output, reason) in listOf(
                Triple("distance", "per_km", "distance"), Triple("seconds", "per_hour", "time"),
            )) {
                val raw = args.getValue(input)
                out[output] = when {
                    raw == "null" -> { reasons.add("$reason-missing"); "null" }
                    raw.toBigInteger() <= BigInteger.ZERO -> { reasons.add("$reason-nonpositive"); "null" }
                    else -> ratio(amount, if (input == "distance") 1000 else 3600, raw.toBigInteger())
                }
            }
            out["unknown"] = reasons.joinToString(",")
            return out
        }
        return when (operation) {
            "DETAILED" -> mapOf("earned" to (n("gross") - n("fees") + n("tips") + n("bonus") - n("refund")).toString())
            "NET" -> mapOf("earned" to n("inclusive").toString(), "gross" to "null", "fees" to "null", "unknown" to "gross-and-fee-components-unknown")
            "PAYOUT" -> mapOf("earned" to n("earned").toString(), "received" to n("payout").toString())
            "RESULT" -> if (args.getValue("complete") != "1" || args.getValue("costs") == "null") {
                mapOf("result" to "null", "per_km" to "null", "per_hour" to "null", "unknown" to "cost-coverage-incomplete")
            } else {
                val result = n("earned") - sum("costs")
                ratios(result).apply { put("result", result.toString()) }
            }
            "MODEL" -> {
                val modeled = n("recorded") - sum("estimates")
                ratios(modeled).apply { put("recorded", n("recorded").toString()); put("modeled", modeled.toString()) }
            }
            "RATIOS" -> ratios(n("amount"))
            "UNION" -> {
                val events = sortedMapOf<Long, Int>()
                var summed = 0L
                for (span in args.getValue("intervals").split(',')) {
                    val pair = span.split(':')
                    require(pair.size == 2)
                    val (start, end) = pair.map { it.toLong() }
                    require(start >= 0 && end > start)
                    events[start] = events.getOrDefault(start, 0) + 1
                    events[end] = events.getOrDefault(end, 0) - 1
                    summed += end - start
                }
                var active = 0
                var previous = 0L
                var union = 0L
                for ((time, delta) in events) {
                    if (active > 0) union += time - previous
                    active += delta
                    previous = time
                }
                require(active == 0)
                mapOf("union" to "$union", "summed" to "$summed")
            }
            else -> error("unknown manual reference operation $operation")
        }
    }
}
