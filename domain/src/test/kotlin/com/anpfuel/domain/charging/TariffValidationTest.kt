package com.anpfuel.domain.charging

import java.time.Instant
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows

/** EE-11 structural refusals outside the shared scenario vectors. */
class TariffValidationTest {

    @Test
    fun frozenUuidAndUnicodeBounds() {
        refused("invalid-tariff", tariff().copy(id = "not-a-uuid"))
        refused("invalid-tariff", tariff().copy(targetId = "not-a-uuid"))
        val result = TariffCalculator.calculate(tariff().copy(revision = "😀".repeat(128)), scenario())
        assertEquals("KNOWN", result.state)
    }

    private val start = Instant.parse("2026-10-01T00:00:00Z")
    private val energy = TariffComponent(
        dimension = "ENERGY_WH",
        rateCentavos = RationalValue("1", "5"),
        billingStep = 1,
        graceSeconds = 0,
    )

    private fun tariff(
        components: List<TariffComponent> = listOf(energy),
    ) = Tariff(
        version = 1,
        id = "11111111-1111-4111-8111-111111111111",
        revision = "synthetic-tariff-v1",
        targetKind = "CONNECTOR",
        targetId = "22222222-2222-4222-8222-222222222222",
        currency = "BRL",
        condition = "STANDARD",
        qualifierId = null,
        timezone = "America/Sao_Paulo",
        validFrom = start,
        validUntil = null,
        taxBasis = "INCLUDED",
        mandatoryComponentsComplete = true,
        components = components,
        minimumCentavos = null,
        maximumCentavos = null,
        billingRounding = "FINAL_HALF_UP_CENTAVO",
        provenance = "DATED_COMMUNITY_REPORT",
    )

    private fun scenario() = TariffScenario(
        condition = "STANDARD",
        qualifierId = null,
        windowStart = Instant.parse("2026-10-21T10:00:00Z"),
        windowEnd = Instant.parse("2026-10-21T10:40:00Z"),
        energyWh = 1000,
        chargingSeconds = 0,
        idleSeconds = 0,
        sessionCount = 1,
        parkingSeconds = 0,
    )

    private fun refused(code: String, tariff: Tariff = tariff(), scenario: TariffScenario = scenario()) {
        val error = assertThrows<TariffValidationException> {
            TariffCalculator.calculate(tariff, scenario)
        }
        assertEquals(code, error.code)
    }

    @Test
    fun refusesInvalidTariffIdentityAndEnvelope() {
        refused("invalid-tariff", tariff().copy(version = 2))
        refused("invalid-tariff", tariff().copy(id = " "))
        refused("invalid-tariff", tariff().copy(revision = "r".repeat(129)))
        refused("invalid-tariff", tariff().copy(targetId = ""))
        refused("invalid-tariff", tariff().copy(timezone = ""))
        refused("invalid-tariff", tariff().copy(timezone = "z".repeat(129)))
        refused("invalid-tariff", tariff().copy(targetKind = "STATION"))
        refused("invalid-tariff", tariff().copy(currency = "brl"))
        refused(
            "invalid-tariff",
            tariff().copy(validUntil = start.minusSeconds(1)),
        )
        refused("invalid-tariff", tariff().copy(taxBasis = "MAYBE"))
        refused("invalid-tariff", tariff().copy(provenance = "PROVIDER"))
        refused("invalid-tariff", tariff().copy(billingRounding = "BANKERS"))
    }

    @Test
    fun refusesUnsupportedComponentBounds() {
        refused("invalid-tariff", tariff(listOf(energy.copy(billingStep = 0))))
        refused(
            "invalid-tariff",
            tariff(listOf(energy.copy(billingStep = TariffCalculator.MAX_BILLING_STEP + 1))),
        )
        refused("invalid-tariff", tariff(listOf(energy.copy(graceSeconds = -1))))
        refused(
            "invalid-tariff",
            tariff(listOf(energy.copy(graceSeconds = TariffCalculator.MAX_SECONDS + 1))),
        )
        refused(
            "invalid-tariff",
            tariff(
                listOf(
                    energy,
                    energy.copy(dimension = "SESSION", billingStep = 1, rateCentavos = RationalValue("2", "1")),
                    energy.copy(dimension = "CHARGING_SECOND"),
                    energy.copy(dimension = "IDLE_SECOND"),
                    energy.copy(dimension = "PARKING_SECOND"),
                    energy.copy(dimension = "ENERGY_WH"),
                ),
            ),
        )
    }

    @Test
    fun refusesInvalidMoneyAndRateBounds() {
        refused("invalid-tariff", tariff().copy(minimumCentavos = -1))
        refused(
            "invalid-tariff",
            tariff().copy(maximumCentavos = TariffCalculator.MAX_CENTAVOS + 1),
        )
        refused(
            "invalid-tariff",
            tariff(listOf(energy.copy(rateCentavos = RationalValue("1", "01")))),
        )
        refused(
            "invalid-tariff",
            tariff(listOf(energy.copy(rateCentavos = RationalValue("1", "0")))),
        )
        refused(
            "invalid-tariff",
            tariff(listOf(energy.copy(rateCentavos = RationalValue("01", "1")))),
        )
    }

    @Test
    fun refusesUnknownOrBogusConditions() {
        refused("invalid-tariff", tariff().copy(condition = "UNKNOWN", qualifierId = "club"))
        refused("invalid-tariff", tariff().copy(condition = "BOGUS"))
        refused(
            "invalid-scenario",
            scenario = scenario().copy(condition = "BOGUS"),
        )
        refused(
            "invalid-scenario",
            scenario = scenario().copy(condition = "MEMBER"),
        )
        refused(
            "invalid-scenario",
            scenario = scenario().copy(qualifierId = "unexpected"),
        )
    }

    @Test
    fun unknownEstimateNeverCarriesANumber() {
        val estimate = TariffEstimate(state = "UNKNOWN", reason = "UNKNOWN_FEES")
        assertNull(estimate.value)
        assertNull(estimate.method)
        assertNull(estimate.unit)
    }
}
