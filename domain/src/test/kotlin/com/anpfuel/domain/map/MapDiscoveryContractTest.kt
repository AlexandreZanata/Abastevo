package com.anpfuel.domain.map

import java.time.Instant
import java.time.LocalDate
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows

class MapDiscoveryContractTest {
    private val id = "00000000-0000-4000-8000-000000000001"
    private val other = "00000000-0000-4000-8000-000000000002"
    private val day = LocalDate.parse("2026-01-02")
    private val instant = Instant.parse("2026-01-02T00:00:00Z")
    private val unknown = MapLocation(null, null, MapLocationQuality.UNKNOWN, null, null, null)
    private val reviewed = MapLocation("-15", "-56", MapLocationQuality.REVIEWED, MapDisplayPoint.STATION, null, null)
    private val rights = MapRights(MapSource.ANP, "ADMITTED", "Synthetic source", "a".repeat(64))
    private val charging = MapCharging(id, id, id, "CCS2", 50_000L, "public", "unknown", null, null, instant)

    private fun failWith(code: String, block: () -> Unit) {
        assertEquals(code, assertThrows<MapContractException>(block).code)
    }

    @Test
    fun `wire vocabulary preserves case and rejects unknown values`() {
        MapSource.entries.forEach { assertEquals(it, MapSource.fromWire(it.wire)) }
        MapService.entries.forEach { assertEquals(it, MapService.fromWire(it.wire)) }
        MapLocationQuality.entries.forEach { assertEquals(it, MapLocationQuality.fromWire(it.wire)) }
        MapDisplayPoint.entries.forEach { assertEquals(it, MapDisplayPoint.fromWire(it.wire)) }
        MapProduct.entries.forEach { assertEquals(it, MapProduct.fromWire(it.wire)) }
        failWith("unknown-source") { MapSource.fromWire("anp") }
        failWith("unknown-service") { MapService.fromWire("energy") }
        failWith("unknown-quality") { MapLocationQuality.fromWire("precise") }
        failWith("unknown-display-point") { MapDisplayPoint.fromWire("road") }
        failWith("unknown-product") { MapProduct.fromWire("diesel") }
    }

    @Test
    fun `identity and coordinate provenance is enforced before pin display`() {
        val identities = MapIdentities(id, "11AA2222333344", null, null, null, "way", 1)
        failWith("invalid-identity") { identities.copy(placeId = "invalid") }
        failWith("invalid-cnpj") { identities.copy(cnpj = "invalid") }
        failWith("invalid-osm-identity") { identities.copy(osmId = null) }
        failWith("invalid-osm-type") { identities.copy(osmType = "polygon") }
        failWith("invalid-osm-id") { identities.copy(osmId = 0) }
        assertTrue(reviewed.pinEligible)
        assertFalse(unknown.pinEligible)
        assertTrue(MapLocation("0", "0", MapLocationQuality.OSM_MAPPED, MapDisplayPoint.OSM_NODE, null, null).pinEligible)
        assertFalse(MapLocation("0", "0", MapLocationQuality.CITY_CENTROID, MapDisplayPoint.CITY_CENTROID, null, null).pinEligible)
        assertTrue(reviewed.copy(entranceLatitudeDeg = "-15.0001", entranceLongitudeDeg = "-56").entranceKnown)
        assertFalse(reviewed.entranceKnown)
        failWith("incomplete-coordinate") { reviewed.copy(longitudeDeg = null) }
        failWith("invalid-coordinate") { reviewed.copy(latitudeDeg = "bad") }
        failWith("invalid-coordinate") { reviewed.copy(longitudeDeg = "181") }
        failWith("invalid-coordinate") { reviewed.copy(longitudeDeg = "NaN") }
        failWith("invalid-reviewed-location") { reviewed.copy(displayPoint = MapDisplayPoint.OSM_NODE) }
        failWith("invalid-osm-location") { reviewed.copy(quality = MapLocationQuality.OSM_MAPPED) }
        failWith("fabricated-location") { reviewed.copy(quality = MapLocationQuality.UNKNOWN) }
        failWith("centroid-as-pin") { reviewed.copy(quality = MapLocationQuality.CITY_CENTROID) }
        failWith("incomplete-entrance") { reviewed.copy(entranceLatitudeDeg = "0") }
        failWith("entrance-without-review") { unknown.copy(entranceLatitudeDeg = "0", entranceLongitudeDeg = "0") }
        failWith("invalid-entrance") { reviewed.copy(entranceLatitudeDeg = "0", entranceLongitudeDeg = "181") }
        failWith("invalid-entrance") { reviewed.copy(entranceLatitudeDeg = "bad", entranceLongitudeDeg = "0") }
        failWith("invalid-entrance") { reviewed.copy(entranceLatitudeDeg = "0", entranceLongitudeDeg = "bad") }
    }

    @Test
    fun `prices require comparable units and source specific periods`() {
        val price = MapPrice(MapSource.ANP, id, MapProduct.DIESEL_S10, MapPriceUnit.L, 5900, day, day, day.plusDays(6))
        assertEquals(5900L, price.amountMilliBrl)
        val community = price.copy(source = MapSource.COMMUNITY, periodStart = null, periodEnd = null)
        assertNull(community.periodStart)
        failWith("invalid-price-source") { price.copy(source = MapSource.OSM) }
        failWith("invalid-price-station") { price.copy(stationId = "bad") }
        failWith("product-unit-mismatch") { price.copy(product = MapProduct.CNG) }
        failWith("amount-out-of-range") { price.copy(amountMilliBrl = 0) }
        failWith("incomplete-period") { price.copy(periodEnd = null) }
        failWith("anp-period-required") { price.copy(periodStart = null, periodEnd = null) }
        failWith("community-period-forbidden") { price.copy(source = MapSource.COMMUNITY) }
        failWith("invalid-period") { price.copy(periodEnd = day.minusDays(1)) }
    }

    @Test
    fun `charging cannot invent status or unpriced tariff conditions`() {
        assertEquals(50_000L, charging.powerW)
        assertEquals("MEMBER", charging.copy(tariffMilliBrlPerKwh = 2000, tariffCondition = "MEMBER").tariffCondition)
        failWith("invalid-charging-place") { charging.copy(placeId = "bad") }
        failWith("invalid-charging-equipment") { charging.copy(equipmentId = "bad") }
        failWith("invalid-charging-connector") { charging.copy(connectorId = "bad") }
        failWith("invalid-connector") { charging.copy(connectorCode = "") }
        failWith("invalid-connector") { charging.copy(connectorCode = "c".repeat(65)) }
        failWith("invalid-power") { charging.copy(powerW = -1) }
        failWith("invalid-access") { charging.copy(access = "staff-only") }
        failWith("invalid-status") { charging.copy(status = "busy") }
        failWith("invalid-tariff") { charging.copy(tariffMilliBrlPerKwh = 0) }
        failWith("invalid-tariff-condition") { charging.copy(tariffMilliBrlPerKwh = 1, tariffCondition = "FREE") }
        failWith("condition-without-tariff") { charging.copy(tariffCondition = "STANDARD") }
    }

    @Test
    fun `assertion rejects facts that conflict with its source identity and flags`() {
        val price = MapPrice(MapSource.ANP, id, MapProduct.DIESEL_S10, MapPriceUnit.L, 5900, day, day, day)
        val fuel = MapAssertion(id, MapSource.ANP, "FUEL_STATION", "Fixture", MapIdentities(id, null, null, null, null, null, null),
            reviewed, setOf(MapService.FUEL), listOf(price), null, MapClocks(instant, instant, instant), rights, MapCapabilities(true, true, false, false))
        val ev = fuel.copy(source = MapSource.OCM, kind = "CHARGING_SITE", services = setOf(MapService.CHARGING),
            prices = emptyList(), charging = charging, rights = rights.copy(source = MapSource.OCM),
            identities = MapIdentities(null, null, id, id, id, null, null), capabilities = fuel.capabilities.copy(priceKnown = false))
        failWith("invalid-id") { fuel.copy(id = "") }
        failWith("unknown-kind") { fuel.copy(kind = "UNKNOWN") }
        failWith("invalid-name") { fuel.copy(name = " ") }
        failWith("station-identity-required") { fuel.copy(identities = fuel.identities.copy(stationId = null)) }
        failWith("service-required") { fuel.copy(services = emptySet()) }
        failWith("price-identity-mismatch") { fuel.copy(prices = listOf(price.copy(stationId = other))) }
        failWith("price-identity-mismatch") { fuel.copy(prices = listOf(price.copy(source = MapSource.COMMUNITY, periodStart = null, periodEnd = null))) }
        failWith("duplicate-price") { fuel.copy(prices = listOf(price, price)) }
        failWith("charging-source-refused") { fuel.copy(charging = charging) }
        failWith("charging-facts-required") { ev.copy(charging = null) }
        failWith("charging-identity-mismatch") { ev.copy(identities = ev.identities.copy(placeId = other)) }
        failWith("charging-identity-mismatch") { ev.copy(identities = ev.identities.copy(equipmentId = other)) }
        failWith("charging-identity-mismatch") { ev.copy(identities = ev.identities.copy(connectorId = other)) }
        failWith("rights-source-mismatch") { fuel.copy(rights = ev.rights) }
        failWith("price-flag-mismatch") { fuel.copy(capabilities = fuel.capabilities.copy(priceKnown = false)) }
        failWith("pin-flag-mismatch") { fuel.copy(capabilities = fuel.capabilities.copy(pinEligible = false)) }
        failWith("entrance-flag-mismatch") { fuel.copy(capabilities = fuel.capabilities.copy(entranceKnown = true)) }
        failWith("live-status-not-admitted") { fuel.copy(capabilities = fuel.capabilities.copy(liveStatus = true)) }
        failWith("rights-not-admitted") { rights.copy(license = "PENDING") }
        failWith("invalid-attribution") { rights.copy(attribution = "") }
        failWith("invalid-source-hash") { rights.copy(sourceSha256 = "x".repeat(64)) }
        val corpus = MapAssertionCorpus("abastevo.map-assertions-v1", 1, "sample", listOf(fuel, ev.copy(id = other)))
        failWith("unsupported-format") { corpus.copy(format = "") }
        failWith("unsupported-version") { corpus.copy(version = 2) }
        failWith("invalid-generation") { corpus.copy(generation = "") }
        failWith("assertion-count") { corpus.copy(assertions = emptyList()) }
        failWith("duplicate-assertion-id") { corpus.copy(assertions = listOf(fuel, ev)) }
    }

    @Test
    fun `native packages require pinned engine and safe bounded paths`() {
        val pkg = NativeMapPackage("br-mt", "REGION", "country/Brazil_Mato Grosso.mwm", 8, "a".repeat(64))
        assertTrue(isSafeMapPackagePath(pkg.path))
        listOf("", "/root.map", "a/../b", "a /b", "a/ b", "a  b", "a\\b", "água.mwm", "x".repeat(161))
            .forEach { assertFalse(isSafeMapPackagePath(it), it) }
        failWith("invalid-package-id") { pkg.copy(id = "BR") }
        failWith("invalid-package-role") { pkg.copy(role = "ZIP") }
        failWith("unsafe-package-path") { pkg.copy(path = "../map") }
        failWith("invalid-package-bytes") { pkg.copy(bytes = 0) }
        failWith("invalid-package-hash") { pkg.copy(sha256 = "bad") }
        val manifest = NativeMapManifest("sample", "comaps", "a".repeat(40), "b".repeat(40), "1",
            "br-mt", listOf(pkg), listOf(pkg.id), "ODbL-1.0", "Synthetic source", instant)
        failWith("invalid-generation") { manifest.copy(generation = "") }
        failWith("unsupported-engine") { manifest.copy(engineName = "other") }
        failWith("unpinned-engine") { manifest.copy(upstreamSha = "HEAD") }
        failWith("unpinned-engine") { manifest.copy(generatorSha = "HEAD") }
        failWith("invalid-format-version") { manifest.copy(formatVersion = "") }
        failWith("invalid-region") { manifest.copy(regionId = "BR") }
        failWith("packages-required") { manifest.copy(packages = emptyList()) }
        failWith("duplicate-package") { manifest.copy(packages = listOf(pkg, pkg)) }
        failWith("missing-dependency") { manifest.copy(dependencies = listOf("missing")) }
        failWith("invalid-license") { manifest.copy(license = "unreviewed") }
        failWith("invalid-attribution") { manifest.copy(attribution = "") }
    }
}
