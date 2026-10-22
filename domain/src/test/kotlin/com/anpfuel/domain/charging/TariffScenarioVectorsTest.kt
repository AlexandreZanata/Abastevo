package com.anpfuel.domain.charging

import java.io.File
import java.time.Instant
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows

/**
 * EE-11 shared tariff vectors. The same reviewed rows execute in Go and Rust;
 * they are synthetic contract examples, never provider billing terms.
 */
class TariffScenarioVectorsTest {

    private val columns = 22

    @Test
    fun reviewedTariffVectors() {
        val lines = File("../contracts/testdata/ev/tariff-v1.csv")
            .readLines()
            .filter { it.isNotBlank() }
        assertEquals(54, lines.size)
        val seen = mutableSetOf<String>()
        for (line in lines.drop(1)) {
            val row = line.split(',')
            assertEquals(columns, row.size, line.take(40))
            assertTrue(seen.add(row[0]), "duplicate vector ID ${row[0]}")
            val expected = row[21]
            if (expected.startsWith("ERROR:")) {
                val error = assertThrows<TariffValidationException>(row[0]) {
                    TariffCalculator.calculate(tariff(row), scenario(row))
                }
                assertEquals(expected.removePrefix("ERROR:"), error.code, row[0])
            } else {
                assertExpected(expected, TariffCalculator.calculate(tariff(row), scenario(row)))
            }
        }
    }

    private fun assertExpected(expected: String, estimate: TariffEstimate) {
        when {
            expected.startsWith("UNKNOWN:") -> {
                val reason = expected.removePrefix("UNKNOWN:")
                assertEquals("UNKNOWN", estimate.state)
                assertEquals(reason, estimate.reason)
                assertNull(estimate.value)
                assertNull(estimate.method)
                assertNull(estimate.unit)
            }
            expected.startsWith("KNOWN:") -> {
                val cents = expected.removePrefix("KNOWN:")
                assertEquals("KNOWN", estimate.state)
                assertEquals("TARIFF_SCENARIO", estimate.method)
                assertEquals("CENTAVOS", estimate.unit)
                assertNull(estimate.reason)
                assertEquals(RationalValue(cents, "1"), estimate.value)
            }
            else -> error("unknown expectation $expected")
        }
    }

    private fun tariff(row: List<String>): Tariff = Tariff(
        version = 1,
        id = "11111111-1111-4111-8111-111111111111",
        revision = "synthetic-tariff-v1",
        targetKind = "CONNECTOR",
        targetId = "22222222-2222-4222-8222-222222222222",
        currency = row[5],
        condition = row[1],
        qualifierId = nullableText(row[2]),
        timezone = "America/Sao_Paulo",
        validFrom = Instant.parse(row[11]),
        validUntil = nullableInstant(row[12]),
        taxBasis = row[6],
        mandatoryComponentsComplete = row[7] == "1",
        components = components(row[20]),
        minimumCentavos = nullableLong(row[9]),
        maximumCentavos = nullableLong(row[10]),
        billingRounding = row[8],
        provenance = "DATED_COMMUNITY_REPORT",
    )

    private fun scenario(row: List<String>): TariffScenario = TariffScenario(
        condition = row[3],
        qualifierId = nullableText(row[4]),
        windowStart = Instant.parse(row[13]),
        windowEnd = Instant.parse(row[14]),
        energyWh = nullableLong(row[15]),
        chargingSeconds = nullableLong(row[16]),
        idleSeconds = nullableLong(row[17]),
        sessionCount = nullableLong(row[18]),
        parkingSeconds = nullableLong(row[19]),
    )

    private fun components(value: String): List<TariffComponent> {
        if (value == "-" || value.isEmpty()) return emptyList()
        return value.split('|').map { part ->
            val fields = part.split(':')
            assertEquals(4, fields.size, part)
            val rate = fields[1].split('/')
            assertEquals(2, rate.size, fields[1])
            TariffComponent(
                dimension = fields[0],
                rateCentavos = RationalValue(rate[0], rate[1]),
                billingStep = fields[2].toLong(),
                graceSeconds = fields[3].toLong(),
            )
        }
    }

    private fun nullableText(value: String): String? =
        value.takeUnless { it == "-" || it.isEmpty() }

    private fun nullableLong(value: String): Long? =
        value.takeUnless { it == "-" || it.isEmpty() }?.toLong()

    private fun nullableInstant(value: String): Instant? =
        value.takeUnless { it == "-" || it.isEmpty() }?.let(Instant::parse)

    @Test
    fun refusalsUseStableCodes() {
        val error = assertThrows<TariffValidationException> {
            TariffCalculator.calculate(
                tariff(validRow()),
                scenario(validRow()).copy(windowEnd = Instant.parse("2026-10-21T10:00:00Z")),
            )
        }
        assertEquals("invalid-scenario", error.code)
    }

    private fun validRow(): List<String> =
        File("../contracts/testdata/ev/tariff-v1.csv").readLines()[1].split(',')
}
