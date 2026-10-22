package com.anpfuel.domain.quality

import java.time.LocalDate
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows

class QualityBoundaryAuditTest {
    private val id = "11111111-1111-4111-8111-111111111111"
    private val today = LocalDate.of(2026, 10, 21)
    private fun command() = QualityReportCommand(id, id, "SUBMIT", 0, "EXPERIENCE", id, "ETHANOL", "2026-10-20", "OTHER", "Synthetic experience")

    @Test
    fun stationFuelRevisionSelectionRejectsUnsupportedScope() {
        assertThrows<IllegalArgumentException> { QualitySelection("bad", "ETHANOL") }
        assertThrows<IllegalArgumentException> { QualitySelection(id, "ELECTRICITY") }
        assertThrows<IllegalArgumentException> { QualitySelection(id, "ETHANOL", "bad") }
        for (fuel in QUALITY_FUELS) assertEquals(fuel, QualitySelection(id, fuel, id).fuel)
        assertTrue(QualityRevision(id, "edition", "published", "VERIFIED", true).selectable)
        assertTrue(QualityRevision(id, "edition", "superseded", "VERIFIED", false).selectable)
        assertFalse(QualityRevision(id, "edition", "withdrawn", "VERIFIED", false).selectable)
        assertFalse(QualityRevision(id, "edition", "published", "LEGACY", true).selectable)
    }

    @Test
    fun malformedPrivateClaimsNeverBecomeSignedIntents() {
        val invalid = listOf(
            command().copy(commandId = "bad"), command().copy(expectedRevision = Int.MAX_VALUE),
            command().copy(action = "UPSERT"), command().copy(kind = "OFFICIAL_PROOF"),
            command().copy(stationId = "bad"), command().copy(fuel = "UNKNOWN"),
            command().copy(occurredOn = "2026-02-31"), command().copy(occurredOn = "2026-10-22"),
            command().copy(symptom = "CONFIRMED_FAULT"), command().copy(text = "x".repeat(1001)),
            command().copy(text = "\uD800"), command().copy(text = "\uDC00"), command().copy(text = "a\u0000b"),
            command().copy(officialRevisionId = id), command().copy(sampleRef = "sample"),
            command().copy(officialRevisionId = id, sampleRef = "é".repeat(65)),
            command().copy(officialRevisionId = "bad", sampleRef = "sample"),
        )
        invalid.forEach { assertThrows<IllegalArgumentException> { it.validate(today) } }
        command().copy(text = "😀".repeat(1000)).validate(today)
        command().copy(officialRevisionId = id, sampleRef = "é".repeat(64)).validate(today)
        command().copy(action = "WITHDRAW", expectedRevision = 1).let { full ->
            assertThrows<IllegalArgumentException> { full.validate(today) }
        }
    }
}
