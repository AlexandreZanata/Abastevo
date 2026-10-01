package com.anpfuel.app.community

import com.anpfuel.domain.model.BackendOfficialSection
import com.anpfuel.domain.model.BackendPriceGroup
import com.anpfuel.domain.model.BackendPriceGroups
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * P10-T06: UNKNOWN/DISPUTED/STALE, differing sources and APP qualifier
 * display. Community stays UNKNOWN until P04; confidence never renders
 * as a guarantee; money stays exact milli with units always.
 */
class CommunityPriceDisplayTest {

    private val stationId = "d6c74c23-63db-4c24-a2e5-408cb23bad26"

    private fun official() = BackendOfficialSection.create(
        source = "ANP",
        amountMilliBrl = 5890L,
        currency = "BRL",
        collectedOn = "2026-09-28",
        surveyWeekStart = "2026-09-27",
        surveyWeekEnd = "2026-10-03",
        revisionId = "rev-1",
    )

    private fun groups(withOfficial: Boolean) = BackendPriceGroups.create(
        stationId = stationId,
        fuelFilterWire = null,
        groups = listOf(
            BackendPriceGroup.create(
                stationId = stationId,
                fuelProductWire = "GASOLINE_REGULAR",
                unit = "L",
                conditionKind = "STANDARD",
                official = if (withOfficial) official() else null,
                community = null,
            ),
        ),
        fetchedAtMillis = 1_000_000L,
        expiresAtMillis = 1_060_000L,
    )

    @Test
    fun `missing official renders UNKNOWN never substituted`() {
        val rows = CommunityPriceDisplay.fromGroups(groups(withOfficial = false))

        assertEquals(1, rows.size)
        assertNull(rows[0].officialAmountMilli)
        assertEquals(CommunityAvailability.UNKNOWN, rows[0].availability)
        assertEquals(CommunityFreshness.UNKNOWN, rows[0].freshness)
        assertEquals("L", rows[0].unit)
    }

    @Test
    fun `stale cache labels STALE never fresh`() {
        val fresh = CommunityPriceDisplay.fromGroups(groups(true))
        val stale = CommunityPriceDisplay.fromGroups(groups(true), staleCache = true)

        assertEquals(CommunityFreshness.UNKNOWN, fresh[0].freshness)
        assertEquals(CommunityFreshness.STALE, stale[0].freshness)
        assertTrue(stale[0].staleCache)
        assertEquals("STALE", CommunityPriceDisplay.freshnessLabel(CommunityFreshness.STALE))
    }

    @Test
    fun `disputed availability labels honestly without backend data`() {
        assertEquals("DISPUTED", CommunityPriceDisplay.availabilityLabel(CommunityAvailability.DISPUTED))
        assertEquals("UNKNOWN", CommunityPriceDisplay.availabilityLabel(CommunityAvailability.UNKNOWN))
    }

    @Test
    fun `differing sources stay separate`() {
        val outcome = groups(true)
        val rows = CommunityPriceDisplay.fromGroups(outcome)

        assertEquals("backend", outcome.source)
        assertEquals("ANP", rows[0].officialSource)
    }

    @Test
    fun `app qualifier display requires qualifier`() {
        assertEquals("STANDARD", CommunityPriceDisplay.formatCondition("STANDARD", null))
        assertTrue(
            CommunityPriceDisplay.formatCondition("APP", null).contains("qualifier required"),
        )
        assertTrue(
            CommunityPriceDisplay.formatCondition("APP", "SHELL_BOX").contains("SHELL_BOX"),
        )
        assertTrue(
            CommunityPriceDisplay.formatCondition("OTHER", null).contains("excluded from ranking"),
        )
    }

    @Test
    fun `milli money stays exact with units always`() {
        assertEquals("R$ 5,890", CommunityPriceDisplay.formatMilliBrl(5890L))
        assertEquals("R$ 0,001", CommunityPriceDisplay.formatMilliBrl(1L))
        val rows = CommunityPriceDisplay.fromGroups(groups(true))
        assertEquals("L", rows[0].unit)
    }

    @Test
    fun `confidence renders as support never guarantee`() {
        assertTrue(CommunityPriceDisplay.confidenceNote().contains("not a guarantee"))
    }
}
