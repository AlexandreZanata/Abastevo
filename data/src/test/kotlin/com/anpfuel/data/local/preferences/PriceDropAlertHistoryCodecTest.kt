package com.anpfuel.data.local.preferences

import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.SurveyWeek
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

/**
 * P23-T01 RED: alert-history codec round-trips weeks and isolates
 * vehicles by key.
 */
class PriceDropAlertHistoryCodecTest {

    private val week = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")

    @Test
    fun `week round-trips through iso bounds`() {
        val (start, end) = PriceDropAlertHistoryCodec.encode(week)

        assertEquals(week, PriceDropAlertHistoryCodec.decode(start, end))
    }

    @Test
    fun `blank bounds decode to no history`() {
        assertNull(PriceDropAlertHistoryCodec.decode(null, null))
        assertNull(PriceDropAlertHistoryCodec.decode("", ""))
        assertNull(PriceDropAlertHistoryCodec.decode("2026-06-07", ""))
    }

    @Test
    fun `keys isolate vehicles`() {
        val first = PriceDropAlertHistoryCodec.key(DomainId.generate())
        val second = PriceDropAlertHistoryCodec.key(DomainId.generate())

        if (first == second) {
            throw AssertionError("distinct vehicles must map to distinct keys")
        }
    }

    @Test
    fun `same vehicle maps to a stable key`() {
        val id = DomainId.forSurveyWeek(week)

        assertEquals(
            PriceDropAlertHistoryCodec.key(id),
            PriceDropAlertHistoryCodec.key(DomainId.forSurveyWeek(week)),
        )
    }
}
