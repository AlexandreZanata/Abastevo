package com.anpfuel.domain.charging

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test

class ChargingSnapshotTest {

    private val placeId = "11111111-1111-4111-8111-111111111111"
    private val groupId = "22222222-2222-4222-8222-222222222222"
    private val key = "a".repeat(64)

    private fun group(
        connector: ChargingConnector = ChargingConnector.CCS2,
        raw: String = "CCS2",
        current: ChargingCurrent = ChargingCurrent.DC,
    ) = ChargingConnectionGroup(
        id = groupId,
        entityKey = key,
        externalId = "OCM-1",
        connector = connector,
        rawConnectorCode = raw,
        current = current,
        powerW = 150000,
        reportedQuantity = 2,
    )

    private fun place() = ChargingPlace(
        id = placeId,
        entityKey = key,
        externalId = "OCM-1",
        name = "Eletroposto DF-1",
        country = "BR",
        latitude = "-15.7939",
        longitude = "-47.8828",
        publicAccess = ChargingAccess.PUBLIC,
        reportedPoints = 2,
        groups = listOf(group()),
    )

    @Test
    fun `valid place and future enums are retained never dropped`() {
        val kept = place()
        assertEquals("BR", kept.country)
        val future = group(connector = ChargingConnector.UNKNOWN, raw = "MEGAWATT_X", current = ChargingCurrent.UNKNOWN)
        assertEquals("MEGAWATT_X", future.rawConnectorCode)
        assertEquals(ChargingConnector.UNKNOWN, future.connector)
    }

    @Test
    fun `corrupt identities coordinates and counts are refused`() {
        assertThrows(IllegalArgumentException::class.java) { place().copy(id = "not-a-uuid") }
        assertThrows(IllegalArgumentException::class.java) { place().copy(entityKey = "short") }
        assertThrows(IllegalArgumentException::class.java) { place().copy(country = "US") }
        assertThrows(IllegalArgumentException::class.java) { place().copy(latitude = "91.0") }
        assertThrows(IllegalArgumentException::class.java) { place().copy(longitude = "nan") }
        assertThrows(IllegalArgumentException::class.java) { place().copy(reportedPoints = -1) }
        assertThrows(IllegalArgumentException::class.java) { place().copy(publicAccess = ChargingAccess.UNKNOWN, latitude = null, longitude = null).copy(name = "") }
        assertThrows(IllegalArgumentException::class.java) { group().copy(powerW = 0) }
        assertThrows(IllegalArgumentException::class.java) { group().copy(reportedQuantity = -2) }
    }

    @Test
    fun `coverage keeps counts only and pack meta binds source`() {
        val coverage = ChargingCoverage(
            profile = "ocm-br-20261021-v1",
            sourceRevision = "ocm-export-8e3bedca",
            places = 1298,
            groups = 3387,
            vehicles = 181,
            generatedAt = "2026-10-22T00:00:00Z",
        )
        assertEquals(1298L, coverage.places)
        assertThrows(IllegalArgumentException::class.java) { coverage.copy(places = -1) }
        assertThrows(IllegalArgumentException::class.java) { coverage.copy(generatedAt = "yesterday") }
        val meta = ChargingPackMeta(
            profile = "ocm-br-20261021-v1",
            sourceRevision = "ocm-export-8e3bedca",
            origin = "https://api.example.test",
            license = "CC-BY-4.0 (provider metadata, attribution required)",
            schemaVersion = 1,
            baseGeneration = 0,
            generatedAt = "2026-10-22T00:00:00Z",
        )
        assertEquals(1, meta.schemaVersion)
        assertThrows(IllegalArgumentException::class.java) { meta.copy(schemaVersion = 2) }
        assertThrows(IllegalArgumentException::class.java) { meta.copy(license = "  ") }
        assertThrows(IllegalArgumentException::class.java) { meta.copy(origin = "http://insecure.example/") }
    }
}
