package com.anpfuel.data.repository

import com.anpfuel.domain.charging.ChargingAccess
import com.anpfuel.domain.charging.ChargingConnectionGroup
import com.anpfuel.domain.charging.ChargingConnector
import com.anpfuel.domain.charging.ChargingCurrent
import com.anpfuel.domain.charging.ChargingPlace
import com.anpfuel.domain.ev.EvAccess
import com.anpfuel.domain.ev.EvHours
import com.anpfuel.domain.ev.EvProvenance
import com.anpfuel.domain.ev.EvTariff
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class EvCandidateMapperTest {
    private val placeId = "11111111-1111-4111-8111-111111111111"
    private val ccs2 = "22222222-2222-4222-8222-222222222222"
    private val chademo = "33333333-3333-4333-8333-333333333333"
    private val entityKey = "a".repeat(64)

    private fun place(
        latitude: String? = "-25.4295",
        longitude: String? = "-49.2719",
        access: ChargingAccess = ChargingAccess.PUBLIC,
    ) = ChargingPlace(
        id = placeId,
        entityKey = entityKey,
        externalId = "OCM-1",
        name = "Synthetic charger",
        country = "BR",
        latitude = latitude,
        longitude = longitude,
        publicAccess = access,
        reportedPoints = 2,
        groups = listOf(
            ChargingConnectionGroup(
                id = ccs2, entityKey = entityKey, externalId = "OCM-1",
                connector = ChargingConnector.CCS2, rawConnectorCode = "CCS2",
                current = ChargingCurrent.DC, powerW = 150_000, reportedQuantity = 2,
            ),
            ChargingConnectionGroup(
                id = chademo, entityKey = entityKey, externalId = "OCM-1",
                connector = ChargingConnector.CHADEMO, rawConnectorCode = "CHADEMO",
                current = ChargingCurrent.DC, powerW = 50_000, reportedQuantity = 1,
            ),
        ),
    )

    @Test
    fun `compatible group keeps exact identity power and typed unknown metadata`() {
        val candidates = EvCandidateMapper.from(place(), ChargingConnector.CCS2, "rev-7")
        assertEquals(1, candidates.size)
        val candidate = candidates.single()
        assertEquals(ccs2, candidate.candidateId)
        assertEquals(placeId, candidate.siteId)
        assertEquals(entityKey, candidate.equipmentId)
        assertEquals(ccs2, candidate.connectorId)
        assertEquals(ChargingConnector.CCS2, candidate.connector)
        assertEquals(150_000L, candidate.powerW)
        assertEquals(EvAccess.PUBLIC, candidate.access)
        assertEquals(EvHours.Unknown, candidate.hours)
        assertEquals(EvTariff.Unknown, candidate.tariff)
        assertEquals(EvProvenance.STATIC_REFERENCE, candidate.provenance)
        assertEquals("rev-7", candidate.sourceRevision)
        assertNull(candidate.observedAt)
        assertEquals(-25.4295, candidate.point.latitude, 1e-9)
        assertEquals(-49.2719, candidate.point.longitude, 1e-9)
    }

    @Test
    fun `unknown access stays unknown and malformed coordinates are skipped`() {
        assertEquals(
            EvAccess.UNKNOWN,
            EvCandidateMapper.from(place(access = ChargingAccess.UNKNOWN), ChargingConnector.CCS2, "rev").single().access,
        )
        assertTrue(EvCandidateMapper.from(place(latitude = null, longitude = null), ChargingConnector.CCS2, "rev").isEmpty())
    }

    @Test
    fun `incompatible connector is not fabricated from another group`() {
        assertTrue(EvCandidateMapper.from(place(), ChargingConnector.NACS, "rev").isEmpty())
    }
}
