package com.anpfuel.domain.legal

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class TermsAcceptanceTest {
    private val documents = LegalDocuments(termsVersion = "2026-10", privacyVersion = "1.0")
    private val now = 1_800_000_000_000L

    @Test fun `ticked acceptance records both versions and the instant`() {
        val record = TermsAcceptanceRule.accept(documents, ticked = true, nowMillis = now)
        assertEquals(TermsAcceptance("2026-10", "1.0", now), record)
        assertTrue(TermsAcceptanceRule.isCurrent(record, documents, now))
    }

    @Test fun `acceptance without a tick or without a real instant is refused`() {
        assertThrows(IllegalArgumentException::class.java) { TermsAcceptanceRule.accept(documents, ticked = false, nowMillis = now) }
        assertThrows(IllegalArgumentException::class.java) { TermsAcceptanceRule.accept(documents, ticked = true, nowMillis = 0) }
        assertThrows(IllegalArgumentException::class.java) { TermsAcceptanceRule.accept(documents, ticked = true, nowMillis = -1) }
    }

    @Test fun `no record or a record for another version is not current`() {
        assertFalse(TermsAcceptanceRule.isCurrent(null, documents, now))
        assertFalse(TermsAcceptanceRule.isCurrent(TermsAcceptance("2026-09", "1.0", now), documents, now))
        assertFalse(TermsAcceptanceRule.isCurrent(TermsAcceptance("2026-10", "0.9", now), documents, now))
    }

    @Test fun `a record from the future or without an instant is not current`() {
        assertFalse(TermsAcceptanceRule.isCurrent(TermsAcceptance("2026-10", "1.0", 0), documents, now))
        assertFalse(TermsAcceptanceRule.isCurrent(TermsAcceptance("2026-10", "1.0", now + TermsAcceptanceRule.CLOCK_SKEW_MILLIS + 1), documents, now))
        assertTrue(TermsAcceptanceRule.isCurrent(TermsAcceptance("2026-10", "1.0", now + TermsAcceptanceRule.CLOCK_SKEW_MILLIS), documents, now))
    }

    @Test fun `document versions are short printable identifiers`() {
        assertThrows(IllegalArgumentException::class.java) { LegalDocuments("", "1.0") }
        assertThrows(IllegalArgumentException::class.java) { LegalDocuments("2026-10", " ") }
        assertThrows(IllegalArgumentException::class.java) { LegalDocuments("2026-10\n", "1.0") }
        assertThrows(IllegalArgumentException::class.java) { LegalDocuments("x".repeat(33), "1.0") }
    }
}
