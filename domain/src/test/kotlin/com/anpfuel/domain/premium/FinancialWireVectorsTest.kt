package com.anpfuel.domain.premium

import java.io.File
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.TestFactory

class FinancialWireVectorsTest {
    @TestFactory
    fun sharedWireValues(): List<DynamicTest> {
        val rows = File("../contracts/testdata/premium/financial-wire-v1.csv").readLines().map { it.split(',') }
        assertEquals(listOf("id", "fuel", "unit", "quantity_milli", "total_centavos", "accepted"), rows.first())
        assertEquals(23, rows.size)
        val inputs = rows.drop(1)
        require(inputs.all { it.size == 6 && it[0].isNotEmpty() })
        assertEquals(inputs.size, inputs.map { it[0] }.toSet().size)
        return inputs.map { row ->
            DynamicTest.dynamicTest(row[0]) {
                val accepted = try {
                    val fuel = PremiumFuel.parse(row[1])
                    val unit = PremiumUnit.parse(row[2])
                    PremiumBounds.checkQuantityMilli(row[3].toLong())
                    PremiumBounds.checkTotalCentavos(row[4].toLong())
                    assertEquals(row[1], fuel.wire)
                    assertEquals(row[2], unit.wire)
                    fuel.unit == unit
                } catch (_: PremiumValidationException) {
                    false
                }
                assertEquals(row[5] == "1", accepted)
            }
        }
    }
}
