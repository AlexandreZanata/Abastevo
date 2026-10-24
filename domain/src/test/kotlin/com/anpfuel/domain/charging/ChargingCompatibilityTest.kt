package com.anpfuel.domain.charging

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class ChargingCompatibilityTest {

    private var groupSeq = 0
    private fun group(connector: ChargingConnector, current: ChargingCurrent) = ChargingConnectionGroup(
        id = "22222222-2222-4222-8222-%012d".format(++groupSeq),
        entityKey = "b".repeat(64),
        externalId = "OCM-1",
        connector = connector,
        rawConnectorCode = connector.name,
        current = current,
        powerW = 150000,
        reportedQuantity = 2,
    )

    private fun place(groups: List<ChargingConnectionGroup>, access: ChargingAccess = ChargingAccess.PUBLIC) = ChargingPlace(
        id = "11111111-1111-4111-8111-111111111111",
        entityKey = "a".repeat(64),
        externalId = "OCM-1",
        name = "Eletroposto DF-1",
        country = "BR",
        latitude = null,
        longitude = null,
        publicAccess = access,
        reportedPoints = groups.size.toLong(),
        groups = groups,
    )

    @Test
    fun `matching connector is compatible unknown stays unknown`() {
        val groups = listOf(group(ChargingConnector.CCS2, ChargingCurrent.DC))
        assertEquals(Compatibility.COMPATIBLE, ChargingCompatibility.assess(ChargingConnector.CCS2, groups))
        assertEquals(Compatibility.UNKNOWN, ChargingCompatibility.assess(ChargingConnector.TYPE2, listOf(group(ChargingConnector.UNKNOWN, ChargingCurrent.UNKNOWN))))
        assertEquals(Compatibility.UNKNOWN, ChargingCompatibility.assess(null, groups))
        assertEquals(Compatibility.UNKNOWN, ChargingCompatibility.assess(ChargingConnector.CCS2, emptyList()))
        assertEquals(Compatibility.INCOMPATIBLE, ChargingCompatibility.assess(ChargingConnector.CHADEMO, groups))
    }

    @Test
    fun `mixed groups keep the best verdict and filters are exact`() {
        val mixed = listOf(group(ChargingConnector.TYPE2, ChargingCurrent.AC), group(ChargingConnector.CCS2, ChargingCurrent.DC))
        assertEquals(Compatibility.COMPATIBLE, ChargingCompatibility.assess(ChargingConnector.CCS2, mixed))
        val place = place(mixed)
        assertTrue(ChargingCompatibility.matches(place, ChargingConnector.CCS2, null, null))
        assertTrue(!ChargingCompatibility.matches(place, ChargingConnector.NACS, null, null))
        assertTrue(ChargingCompatibility.matches(place, null, ChargingCurrent.DC, null))
        assertTrue(!ChargingCompatibility.matches(place, null, null, ChargingAccess.UNKNOWN))
        assertTrue(!ChargingCompatibility.matches(place(place(mixed).groups, ChargingAccess.UNKNOWN), null, null, ChargingAccess.PUBLIC))
    }

    private fun assertTrue(value: Boolean) = assertEquals(true, value)
}
