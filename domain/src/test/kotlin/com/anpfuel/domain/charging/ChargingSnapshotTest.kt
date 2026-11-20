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
    fun `merged place keeps exact source attribution and refuses invented owners`() {
        val osm = ChargingSourceAttribution("OSM", "node-1", "r1", null, "ODbL", "OpenStreetMap")
        val ocm = ChargingSourceAttribution("OCM", "42", "r1", 1, "CC-BY", "Open Charge Map")
        val owned = group().copy(sourceKey = osm.sourceKey, rawConnectorCode = "")
        val merged = place().copy(sources = listOf(osm, ocm), groups = listOf(owned),
            catalogConfidence = ChargingCatalogConfidence.TWO_SOURCE, linkedStationId = groupId,
            operator = "Synthetic", addressLine = "Avenida Central")
        assertEquals(true, merged.confirmedByTwoSources)
        assertEquals("ocm:poi:42", ocm.sourceKey)
        assertEquals("osm:node-1", merged.groups.single().sourceKey)
        assertThrows(IllegalArgumentException::class.java) { osm.copy(source = "other") }
        assertThrows(IllegalArgumentException::class.java) { osm.copy(providerId = 1) }
        assertThrows(IllegalArgumentException::class.java) { ocm.copy(providerId = null) }
        assertThrows(IllegalArgumentException::class.java) { ocm.copy(providerId = 0) }
        assertThrows(IllegalArgumentException::class.java) { osm.copy(externalId = "bad\u0000") }
        assertThrows(IllegalArgumentException::class.java) { osm.copy(revision = " ") }
        assertThrows(IllegalArgumentException::class.java) { osm.copy(license = "") }
        assertThrows(IllegalArgumentException::class.java) { osm.copy(attribution = " ") }
        assertThrows(IllegalArgumentException::class.java) { owned.copy(rawConnectorCode = "x\n") }
        assertThrows(IllegalArgumentException::class.java) { merged.copy(sources = listOf(osm, osm)) }
        assertThrows(IllegalArgumentException::class.java) { merged.copy(sources = listOf(osm)) }
        assertThrows(IllegalArgumentException::class.java) { merged.copy(groups = listOf(owned.copy(sourceKey = "osm:foreign"))) }
        assertThrows(IllegalArgumentException::class.java) { merged.copy(addressLine = "1.23, 2.34") }
        assertThrows(IllegalArgumentException::class.java) { merged.copy(operator = " ") }
        assertThrows(IllegalArgumentException::class.java) { place().copy(linkedStationId = groupId) }
        assertThrows(IllegalArgumentException::class.java) { place().copy(groups = listOf(owned)) }

        val page = ChargingPlacePage(listOf(merged), "next", "2026-10-22T00:00:00Z")
        assertEquals(merged, page.items.single())
        assertThrows(IllegalArgumentException::class.java) { page.copy(items = listOf(merged, merged)) }
        assertThrows(IllegalArgumentException::class.java) { page.copy(nextCursor = "") }
        assertThrows(IllegalArgumentException::class.java) { page.copy(generatedAt = "tomorrow") }
        val offer = ChargingCatalogOffer.forRevision(key)
        assertEquals("/v1/charging/catalog/$key/offer/derived-database.ndjson", offer?.dataPath)
        assertEquals(null, ChargingCatalogOffer.forRevision("../x"))
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
