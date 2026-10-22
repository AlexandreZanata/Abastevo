package com.anpfuel.domain.occurrence

import com.anpfuel.domain.repository.OccurrenceGateway
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class OccurrenceTest {
    private val id = "eeeeeeee-6666-4666-8666-000000000002"
    private fun record(kind: String = "company-shaped") = OccurrenceRecord(
        id = id, nativeKey = "enforcement-source:abc", subjectKind = kind,
        uf = "DF", municipality = "BRASILIA", segment = "Revenda de Combustíveis",
        documentDay = "2026-10-04", dateBasis = "document_day", documentNumber = "712508",
        procedure = "Inspection", result = "Resultado", evidenceRef = "synthetic.xlsx#row:2")

    @Test fun `valid record keeps verbatim wording and unknown kinds`() {
        assertEquals("company-shaped", record().subjectKind)
        assertEquals("FUTURE_KIND", record("FUTURE_KIND").subjectKind)
    }

    @Test fun `malformed identities are refused`() {
        assertThrows(IllegalArgumentException::class.java) { record().copy(id = "not-a-uuid") }
        assertThrows(IllegalArgumentException::class.java) { record().copy(uf = "SPO") }
        assertThrows(IllegalArgumentException::class.java) { record().copy(documentDay = "2026-13-40") }
        assertThrows(IllegalArgumentException::class.java) { record().copy(documentNumber = "") }
    }

    @Test fun `scopes are typed and revision vectors are exact`() {
        assertTrue(OccurrenceScope.SourceGlobal is OccurrenceScope.SourceGlobal)
        val station = "eeeeeeee-5555-4555-8555-000000000001"
        assertEquals(station, (OccurrenceScope.CurrentCompany(station) as OccurrenceScope.CurrentCompany).stationId)
        assertThrows(IllegalArgumentException::class.java) { OccurrenceScope.CurrentCompany("nope") }
        val revision = SourceRevision("enforcement", "enforcement-v1", "enforcement-source-v1", station)
        assertEquals("enforcement", revision.source)
        assertThrows(IllegalArgumentException::class.java) { SourceRevision("", "s", "e", station) }
    }

    @Test fun `stations require active status and positive counts`() {
        val station = OccurrenceStation("eeeeeeee-5555-4555-8555-000000000001", "Posto", null, "DF", "active", 2)
        assertEquals(2L, station.recordCount)
        assertThrows(IllegalArgumentException::class.java) { station.copy(status = "retired") }
        assertThrows(IllegalArgumentException::class.java) { station.copy(recordCount = 0) }
    }

    @Test fun `gateway exposes occurrence reads`() {
        assertTrue(OccurrenceGateway::class.java.methods.map { it.name }.containsAll(listOf("records", "recordCoverage", "stations", "stationCoverage")))
    }
}
