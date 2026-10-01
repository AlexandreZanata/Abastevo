package com.anpfuel.domain.model

import com.anpfuel.domain.exception.DomainException
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test

class BackendPriceGroupsTest {

    private val stationId = "d6c74c23-63db-4c24-a2e5-408cb23bad26"

    private fun official() = BackendOfficialSection.create(
        source = "ANP",
        amountMilliBrl = 5999,
        currency = "BRL",
        collectedOn = "2026-09-28",
        surveyWeekStart = "2026-09-21",
        surveyWeekEnd = "2026-09-27",
        revisionId = "123e4567-e89b-12d3-a456-426614174000",
    )

    @Test
    fun `accepts wire additived and keeps community null`() {
        val group = BackendPriceGroup.create(
            stationId = stationId,
            fuelProductWire = "GASOLINE_ADDITIVED",
            unit = "L",
            conditionKind = "STANDARD",
            official = official(),
            community = null,
        )

        assertEquals("GASOLINE_ADDITIVED", group.fuelProductWire)
        assertNull(group.community)
    }

    @Test
    fun `refuses legacy premium name on the wire`() {
        assertThrows(DomainException::class.java) {
            BackendPriceGroup.create(
                stationId = stationId,
                fuelProductWire = "GASOLINE_PREMIUM",
                unit = "L",
                conditionKind = "STANDARD",
                official = null,
                community = null,
            )
        }
    }

    @Test
    fun `refuses unknown fuel and non-null community`() {
        assertThrows(DomainException::class.java) {
            BackendPriceGroup.create(
                stationId = stationId,
                fuelProductWire = "JET",
                unit = "L",
                conditionKind = "STANDARD",
                official = null,
                community = null,
            )
        }
        assertThrows(DomainException::class.java) {
            BackendPriceGroup.create(
                stationId = stationId,
                fuelProductWire = "ETHANOL",
                unit = "L",
                conditionKind = "STANDARD",
                official = null,
                community = "COMMUNITY",
            )
        }
    }

    @Test
    fun `refuses malformed station id and bad money`() {
        assertThrows(DomainException::class.java) {
            BackendPriceGroup.create(
                stationId = "not-a-uuid",
                fuelProductWire = "ETHANOL",
                unit = "L",
                conditionKind = "STANDARD",
                official = null,
                community = null,
            )
        }
        assertThrows(IllegalArgumentException::class.java) {
            BackendOfficialSection.create(
                source = "ANP",
                amountMilliBrl = 0,
                currency = "BRL",
                collectedOn = "2026-09-28",
                surveyWeekStart = "2026-09-21",
                surveyWeekEnd = "2026-09-27",
                revisionId = "123e4567-e89b-12d3-a456-426614174000",
            )
        }
    }

    @Test
    fun `groups carry source version and expiry`() {
        val groups = BackendPriceGroups.create(
            stationId = stationId,
            fuelFilterWire = null,
            groups = emptyList(),
            fetchedAtMillis = 1_000_000L,
            expiresAtMillis = 1_060_000L,
        )

        assertEquals("backend", groups.source)
        assertEquals("v1", groups.version)
        assertEquals(1_000_000L, groups.fetchedAtMillis)
        assertEquals(1_060_000L, groups.expiresAtMillis)
    }
}
