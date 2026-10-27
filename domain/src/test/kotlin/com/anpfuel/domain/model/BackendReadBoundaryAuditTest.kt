package com.anpfuel.domain.model

import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.domain.discovery.StationLocationQuality
import com.anpfuel.domain.exception.DomainException
import java.time.Instant
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows

class BackendReadBoundaryAuditTest {
    private val station = "11111111-1111-4111-8111-111111111111"
    private fun official(source: String = "ANP", amount: Long = 5999, currency: String = "BRL", collected: String = "2026-10-06", start: String = "2026-10-05", end: String = "2026-10-11", revision: String = "revision") =
        BackendOfficialSection.create(source, amount, currency, collected, start, end, revision)

    @Test
    fun incompleteOfficialEvidenceCannotBecomeAQuotedPrice() {
        val invalid = listOf<() -> BackendOfficialSection>(
            { official(source = " ") }, { official(amount = 0) }, { official(amount = 1000001) },
            { official(currency = "USD") }, { official(collected = "") }, { official(start = "") },
            { official(end = "") }, { official(revision = "") },
        )
        invalid.forEach { assertThrows<IllegalArgumentException> { it() } }
        val group = BackendPriceGroup.create(station, "ETHANOL", "L", "STANDARD", official(), null)
        assertNotNull(group.official)
        assertNull(group.community)
        val mutable = mutableListOf(group)
        val cached = BackendPriceGroups.create(station, null, mutable, 1000, 2000)
        mutable.clear()
        assertEquals(1, cached.groups.size)
        assertEquals(5999L, cached.groups.single().official?.amountMilliBrl)
    }

    @Test
    fun invalidIdentityFilterAndExpiryCannotEnterTheCache() {
        assertThrows<DomainException> { BackendPriceGroup.create(station.replace('1', 'z'), "ETHANOL", "L", "STANDARD", null, null) }
        assertThrows<DomainException> { BackendPriceGroup.create(station, "BATTERY", "L", "STANDARD", null, null) }
        assertThrows<IllegalArgumentException> { BackendPriceGroup.create(station, "ETHANOL", "", "STANDARD", null, null) }
        assertThrows<IllegalArgumentException> { BackendPriceGroup.create(station, "ETHANOL", "L", "", null, null) }
        assertThrows<DomainException> { BackendPriceGroups.create("bad", null, emptyList(), 0, 1) }
        assertThrows<DomainException> { BackendPriceGroups.create(station, "BATTERY", emptyList(), 0, 1) }
        assertThrows<IllegalArgumentException> { BackendPriceGroups.create(station, null, emptyList(), 1, 1) }
    }

    @Test
    fun communityRequiresAnHonestAvailabilityAndOrderedClock() {
        val at = Instant.parse("2026-10-21T12:00:00Z")
        val unknown = BackendCommunitySection("COMMUNITY", "UNKNOWN", null, "BRL", "", "UNKNOWN", 0, 0, null, null, null, "v1", 1)
        assertThrows<IllegalArgumentException> { unknown.copy(availability = "AVAILABLE") }
        assertThrows<IllegalArgumentException> { unknown.copy(lastReceivedAt = at, expiresAt = at) }
        val available = unknown.copy(availability = "AVAILABLE", amountMilliBrl = 5999, confidence = "LOW", freshness = "FRESH", lastReceivedAt = at, expiresAt = at.plusSeconds(60))
        assertEquals(5999L, available.amountMilliBrl)
        assertNull(unknown.amountMilliBrl)
    }

    @Test
    fun stationReadsPreserveUnknownLocationAndRefuseMalformedCoordinates() {
        fun value(lat: Double? = null, lon: Double? = null, cnpj: String? = null, revision: String? = null) =
            ServerStation.create(station, " Station ", StationLocationQuality.UNKNOWN, lat, lon, cnpj, null, null, revision)
        assertNull(value().latitude)
        assertNull(value().longitude)
        for (wire in listOf("unknown", "city-centroid", "reviewed", "osm-mapped", "anp-reported", "address-point")) {
            assertEquals(wire, StationLocationQuality.fromWire(wire).wire)
        }
        // Forward compatible: an unrecognised tier degrades to UNKNOWN and never becomes a pin.
        assertEquals(StationLocationQuality.UNKNOWN, StationLocationQuality.fromWire("gps-guessed"))
        assertEquals(false, StationLocationQuality.fromWire("gps-guessed").isPin)
        assertThrows<DomainException> { value(lon = 10.0) }
        assertThrows<DomainException> { value(lat = 0.0, lon = Double.NaN) }
        assertThrows<DomainException> { value(cnpj = "x".repeat(19)) }
        assertThrows<DomainException> { value(revision = "bad") }
    }
}
