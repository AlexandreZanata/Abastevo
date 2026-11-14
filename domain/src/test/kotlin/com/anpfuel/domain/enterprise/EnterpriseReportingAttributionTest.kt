package com.anpfuel.domain.enterprise

import java.time.Instant
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test

class EnterpriseReportingAttributionTest {
    private fun at(raw: String): Instant = EnterpriseTime.parseInstant(raw)

    @Test fun `attribution needs no receipt and matches capture`() {
        val cases = listOf(
            Triple("2026-11-01T02:30:00Z", ReportingZones("America/Sao_Paulo"), ReportingAttribution("2026-10-31", "America/Sao_Paulo", "organization")),
            Triple("2026-11-01T03:30:00Z", ReportingZones("America/Sao_Paulo"), ReportingAttribution("2026-11-01", "America/Sao_Paulo", "organization")),
            Triple("2026-11-01T03:30:00Z", ReportingZones("America/Sao_Paulo", "America/Cuiaba"), ReportingAttribution("2026-10-31", "America/Cuiaba", "unit")),
            Triple("2011-12-30T10:00:00Z", ReportingZones("UTC", "Pacific/Apia"), ReportingAttribution("2011-12-31", "Pacific/Apia", "unit")),
            Triple("0001-01-01T00:00:00Z", ReportingZones("UTC"), ReportingAttribution("0001-01-01", "UTC", "organization")),
            Triple("9999-12-31T23:59:59.999999999Z", ReportingZones("UTC"), ReportingAttribution("9999-12-31", "UTC", "organization")),
        )
        for ((raw, zones, expected) in cases) {
            val attribution = EnterpriseTime.attributeReportingDay(at(raw), zones)
            assertEquals(expected, attribution, raw)
            val record = EnterpriseTime.capture(TimeDeclaration(at(raw), "Asia/Tokyo", 32400), at("2026-11-14T00:00:00Z"), zones)
            assertEquals(attribution, ReportingAttribution(record.reportingDay, record.reportingZone, record.reportingSource), raw)
        }
    }

    @Test fun `attribution refuses without fallback`() {
        val noon = at("2026-11-14T12:00:00Z")
        val cases = listOf(
            noon to ReportingZones("America/Sao_Paulo", "Mars/Olympus"),
            noon to ReportingZones("America/Sao_Paulo", ""),
            noon to ReportingZones("Mars/Olympus", "America/Cuiaba"),
            noon to ReportingZones(""),
            noon to ReportingZones("Asia/Tokyo "),
            at("0001-01-01T00:00:00Z") to ReportingZones("America/Sao_Paulo"),
            Instant.parse("0000-12-31T00:00:00Z") to ReportingZones("UTC"),
            Instant.parse("+10000-01-01T00:00:00Z") to ReportingZones("UTC"),
            at("9999-12-31T23:00:00Z") to ReportingZones("Asia/Tokyo"),
        )
        for ((instant, zones) in cases) {
            assertThrows(EnterpriseTimeException::class.java) { EnterpriseTime.attributeReportingDay(instant, zones) }
        }
    }
}
