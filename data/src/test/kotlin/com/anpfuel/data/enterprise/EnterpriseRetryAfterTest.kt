package com.anpfuel.data.enterprise

import com.anpfuel.domain.enterprise.sync.QueueRetryPolicy
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

/** The shared signed-plane Retry-After profile never parks retained work beyond its documented bound. */
class EnterpriseRetryAfterTest {
    private val bound = 21_600_000L
    private val date = "Thu, 1 Jan 1970 00:00:00 GMT"

    @Test fun boundEqualsTheExistingQueueBackoffCap() {
        assertEquals(QueueRetryPolicy().capMillis, bound)
        assertEquals(bound, EnterpriseRetryAfter.MAX_MILLIS)
    }

    @Test fun usableHintsAreHonoredUpToTheBoundAndNeverShortenedBelowIt() {
        assertEquals(0L, EnterpriseRetryAfter.parse("0", null))
        assertEquals(120_000L, EnterpriseRetryAfter.parse(" 120 ", null))
        assertEquals(bound, EnterpriseRetryAfter.parse("21600", null))
        assertEquals(bound - 1000, EnterpriseRetryAfter.parse("21599", null))
        assertEquals(120_000L, EnterpriseRetryAfter.parse("Thu, 1 Jan 1970 00:02:00 GMT", date))
        assertEquals(0L, EnterpriseRetryAfter.parse(date, "Thu, 1 Jan 1970 00:02:00 GMT"))
    }

    @Test fun hugeOrOverflowingHintsAreClampedToTheBound() {
        for (raw in listOf("21601", "9223372036854775", "9223372036854775807", "99999999999999999999999999")) {
            assertEquals(bound, EnterpriseRetryAfter.parse(raw, null), raw)
        }
        assertEquals(bound, EnterpriseRetryAfter.parse("Fri, 31 Dec 9999 23:59:59 GMT", date))
        assertEquals(bound, EnterpriseRetryAfter.parse("Thu, 1 Jan 1970 06:00:01 GMT", date))
    }

    @Test fun negativeGarbageAndUnanchoredHintsKeepOrdinaryBackoff() {
        for (raw in listOf(null, "", "   ", "-1", "-9223372036854775808", "+5", "1.5", "120s", "0x10", "invalid", "１２０")) {
            assertNull(EnterpriseRetryAfter.parse(raw, date), raw.toString())
        }
        assertNull(EnterpriseRetryAfter.parse("Thu, 1 Jan 1970 00:02:00 GMT", null))
        assertNull(EnterpriseRetryAfter.parse("Thu, 1 Jan 1970 00:02:00 GMT", "invalid"))
    }
}
