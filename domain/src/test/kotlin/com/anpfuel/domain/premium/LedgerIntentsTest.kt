package com.anpfuel.domain.premium

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test

class LedgerIntentsTest {

    private val after = "11111111-1111-4111-8111-111111111111"
    private val before = "22222222-2222-4222-8222-222222222222"

    @Test
    fun `gap and reset intents carry exact bodies`() {
        val gap = LedgerGapIntent(afterRecordId = after, beforeRecordId = before, note = "pneu furado")
        assertEquals(
            """{"after_id":"$after","before_id":"$before","note":"pneu furado"}""",
            gap.bodyJson(),
        )
        val open = LedgerGapIntent(afterRecordId = after, beforeRecordId = null, note = null)
        assertEquals("""{"after_id":"$after"}""", open.bodyJson())
        assertEquals("""{"reason":"reset"}""", LedgerSegmentReset("reset").bodyJson())
        assertEquals("""{"reason":"replacement"}""", LedgerSegmentReset("replacement").bodyJson())
    }

    @Test
    fun `intents refuse malformed anchors notes and reasons`() {
        assertThrows(PremiumValidationException::class.java) {
            LedgerGapIntent(afterRecordId = "nope", beforeRecordId = null, note = null)
        }
        assertThrows(PremiumValidationException::class.java) {
            LedgerGapIntent(afterRecordId = after, beforeRecordId = "nope", note = null)
        }
        assertThrows(PremiumValidationException::class.java) {
            LedgerGapIntent(afterRecordId = after, beforeRecordId = null, note = "bad\u0000note")
        }
        assertThrows(PremiumValidationException::class.java) {
            LedgerSegmentReset("explode")
        }
    }

    @Test
    fun `estimated efficiency is a positive bounded ratio`() {
        assertEquals("12.5", EstimatedEfficiency.of("12,5").toPlainString())
        assertThrows(PremiumValidationException::class.java) { EstimatedEfficiency.of("0") }
        assertThrows(PremiumValidationException::class.java) { EstimatedEfficiency.of("-3") }
        assertThrows(PremiumValidationException::class.java) { EstimatedEfficiency.of("1000.01") }
        assertThrows(PremiumValidationException::class.java) { EstimatedEfficiency.of("abc") }
        assertThrows(PremiumValidationException::class.java) { EstimatedEfficiency.of("12.345") }
    }
}
