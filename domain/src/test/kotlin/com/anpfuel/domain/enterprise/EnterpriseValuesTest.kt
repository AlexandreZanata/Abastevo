package com.anpfuel.domain.enterprise

import java.io.File
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.TestFactory

class EnterpriseValuesTest {
    private fun fields(raw: String): Map<String, String> {
        val pairs = raw.split(';').map {
            val pair = it.split('=', limit = 2)
            require(pair.size == 2 && pair[0].isNotEmpty())
            pair[0] to pair[1]
        }
        require(pairs.map { it.first }.toSet().size == pairs.size)
        return pairs.toMap()
    }

    private fun optional(raw: String): Long? = if (raw == "?") null else raw.toLong()
    private fun text(value: Long?): String = value?.toString() ?: "?"

    private fun execute(operation: String, args: Map<String, String>): Map<String, String> = when (operation) {
        "PARSE_CENTAVOS" -> mapOf("value" to EnterpriseValues.parseCentavos(args.getValue("text")).toString())
        "PARSE_QUANTITY" -> mapOf("value" to EnterpriseValues.parseQuantityMilli(args.getValue("text")).toString())
        "PARSE_PRICE" -> mapOf("value" to EnterpriseValues.parseUnitPriceMilliBRL(args.getValue("text")).toString())
        "RECONCILE" -> EnterpriseValues.reconcile(
            ExactRecord(
                optional(args.getValue("paid")), optional(args.getValue("quantity")),
                optional(args.getValue("price")), optional(args.getValue("discount")),
                args.getValue("unit"), args.getValue("source"),
            ),
        ).let {
            mapOf(
                "paid" to text(it.paidCentavos), "gross" to text(it.grossCentavos),
                "expected" to text(it.expectedCentavos), "difference" to text(it.differenceCentavos),
                "derived" to (it.derivedPriceMilliBRL ?: "?"), "warning" to it.warning,
                "unknown" to it.unknown, "unit" to it.unit, "source" to it.sourcePriceText,
            )
        }
        "ALLOCATE" -> {
            val lines = args.getValue("lines").takeIf { it.isNotEmpty() }?.split(',')?.map {
                val pair = it.split(':')
                require(pair.size == 2)
                AllocationLine(pair[0].toLong(), optional(pair[1]))
            } ?: emptyList()
            EnterpriseValues.allocate(optional(args.getValue("source")), lines).let {
                mapOf("parts" to (it.parts?.joinToString(",") { p -> "${p.order}:${p.centavos}" } ?: "?"), "unknown" to it.unknown)
            }
        }
        else -> error("Unhandled shared operation")
    }

    @TestFactory
    fun sharedEnterpriseExactValueVectors(): List<DynamicTest> {
        val lines = File("../contracts/testdata/enterprise/exact-values-v1.psv").readLines()
        assertEquals("id|operation|args|expected", lines.first())
        val rows = lines.drop(1).map { it.split('|') }
        assertEquals(112, rows.size)
        require(rows.all { it.size == 4 && it[0].isNotEmpty() })
        assertEquals(rows.size, rows.map { it[0] }.toSet().size)
        return rows.map { (id, operation, rawArgs, rawExpected) ->
            DynamicTest.dynamicTest(id) {
                val got = try {
                    execute(operation, fields(rawArgs))
                } catch (_: EnterpriseValueException) {
                    mapOf("error" to "invalid-value")
                }
                assertEquals(fields(rawExpected), got)
            }
        }
    }

    @Test
    fun allocationPreservesSourceAndExplicitOrderWithoutChangingInputs() {
        for (source in 0L..101L) {
            for (weights in listOf(listOf(1L, 1L, 1L), listOf(1L, 2L, 4L), listOf(0L, Long.MAX_VALUE, Long.MAX_VALUE))) {
                val lines = weights.mapIndexed { i, weight -> AllocationLine(i.toLong(), weight) }
                val before = lines.toList()
                val allocation = EnterpriseValues.allocate(source, lines)
                val parts = allocation.parts!!
                assertEquals(source, parts.sumOf { it.centavos })
                assertTrue(parts.all { it.centavos in 0..source })
                assertEquals(allocation, EnterpriseValues.allocate(source, lines.reversed()))
                assertEquals(before, lines)
                assertEquals(listOf(0L, 1L, 2L), parts.map { it.order })
            }
        }
    }

    @Test
    fun allocationHonorsProcessingBudgetAndDoesNotFillUnknownWithZero() {
        val lines = List(EnterpriseValues.MAX_ALLOCATION_LINES) { AllocationLine(it.toLong(), 1) }
        assertEquals(1000L, EnterpriseValues.allocate(1000, lines).parts!!.sumOf { it.centavos })
        assertThrows(EnterpriseValueException::class.java) { EnterpriseValues.allocate(1, lines + AllocationLine(1000, 1)) }
        assertNotEquals(EnterpriseValues.allocate(null, lines), EnterpriseValues.allocate(0, lines))
    }

    @Test
    fun consistencyWarningPreservesRecordedTotalAndSourceText() {
        val input = ExactRecord(97000, 150000, 6490, 0, "L", "6,490123000")
        val result = EnterpriseValues.reconcile(input)
        assertEquals(97000L, result.paidCentavos)
        assertEquals(97350L, result.expectedCentavos)
        assertEquals("ARITHMETIC_MISMATCH", result.warning)
        assertEquals("6,490123000", result.sourcePriceText)
        assertEquals(97000L, input.paidCentavos)
    }
}
