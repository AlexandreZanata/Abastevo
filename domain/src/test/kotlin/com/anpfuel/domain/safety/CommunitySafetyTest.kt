package com.anpfuel.domain.safety

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNotEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class CommunitySafetyTest {

    private val id = "9B1DEB4D-3B7D-4BAD-9BDD-2B0D7B3DCB6D"

    @Test
    fun `content kinds carry the exact wire values and refuse unknown ones`() {
        assertEquals(listOf("FORUM_POST", "FORUM_COMMENT", "STATION_COMMENT"), SafetyContentKind.entries.map { it.wire })
        assertEquals(SafetyContentKind.STATION_COMMENT, SafetyContentKind.fromWire(" STATION_COMMENT "))
        assertNull(SafetyContentKind.fromWire("FORUM_TOPIC"))
        assertNull(SafetyContentKind.fromWire("forum_post"))
        assertNull(SafetyContentKind.fromWire(null))
    }

    @Test
    fun `content references compare ids case-insensitively per kind`() {
        val upper = SafetyContentRef(SafetyContentKind.FORUM_COMMENT, " $id ")
        val lower = SafetyContentRef(SafetyContentKind.FORUM_COMMENT, id.lowercase())
        assertEquals(id.lowercase(), upper.id)
        assertEquals(lower, upper)
        assertEquals(lower.hashCode(), upper.hashCode())
        assertTrue(upper.isWellFormed)
        assertNotEquals(lower, SafetyContentRef(SafetyContentKind.FORUM_POST, id))
        assertFalse(lower.equals(id as Any))
        assertEquals(1, setOf(upper, lower).size)
        assertTrue(lower.toString().contains("FORUM_COMMENT"))
    }

    @Test
    fun `malformed ids are flagged and never canonical`() {
        assertFalse(SafetyContentRef(SafetyContentKind.FORUM_POST, "").isWellFormed)
        assertFalse(SafetyContentRef(SafetyContentKind.FORUM_POST, "1-1-1-1-1").isWellFormed)
        assertFalse(SafetyContentRef(SafetyContentKind.FORUM_POST, "$id/report").isWellFormed)
    }

    @Test
    fun `report reasons are the six contract codes and unknown codes read as other`() {
        assertEquals(
            listOf("SPAM", "HARASSMENT", "HATE", "IMPERSONATION", "ILLEGAL", "OTHER"),
            UserReportReason.entries.map { it.wire },
        )
        assertEquals(UserReportReason.HATE, UserReportReason.fromWire("HATE"))
        assertEquals(UserReportReason.OTHER, UserReportReason.fromWire("DEFAMATION"))
        assertEquals(UserReportReason.OTHER, UserReportReason.fromWire(null))
    }

    @Test
    fun `report detail is trimmed and blank detail is omitted`() {
        assertNull(UserReportDetail.normalize(null))
        assertNull(UserReportDetail.normalize("   \n "))
        assertEquals("line one\nline two", UserReportDetail.normalize("  line one\r\nline two  "))
        assertTrue(UserReportDetail.isValid(null))
        assertTrue(UserReportDetail.isValid("   "))
        assertEquals(UserReportDetail.MAX_SCALARS, UserReportDetail.remaining(null))
    }

    @Test
    fun `report detail counts unicode scalars up to two hundred`() {
        val emoji = "😀" // one scalar, two UTF-16 units
        val atLimit = emoji.repeat(UserReportDetail.MAX_SCALARS)
        assertTrue(UserReportDetail.isValid(atLimit))
        assertEquals(0, UserReportDetail.remaining(atLimit))
        assertFalse(UserReportDetail.isValid(atLimit + "a"))
        assertEquals(-1, UserReportDetail.remaining(atLimit + "a"))
        assertTrue(UserReportDetail.isValid("  " + "a".repeat(200) + "  "))
    }

    @Test
    fun `receipts and blocks keep the server fields`() {
        val user = BlockedUser("b1", "", null, "2026-11-15T10:00:00Z")
        val receipt = BlockReceipt(user, replayed = true)
        assertEquals("", receipt.blockedUser.label)
        assertNull(receipt.blockedUser.sourceKind)
        assertTrue(receipt.replayed)
        val report = UserReportReceipt("r1", UserReportReason.SPAM, "2026-11-15T10:00:00Z", replayed = false)
        assertEquals(report, report.copy())
        assertFalse(report.replayed)
    }

    @Test
    fun `refusals expose only stable codes`() {
        val refusals = listOf(
            CommunitySafetyException.BadRequest(),
            CommunitySafetyException.BadRequest("safety.detail-too-long"),
            CommunitySafetyException.SelfTarget(),
            CommunitySafetyException.AuthRequired(),
            CommunitySafetyException.NotFound(),
            CommunitySafetyException.BlockLimit(),
            CommunitySafetyException.RateLimited(42),
            CommunitySafetyException.Unavailable(),
            CommunitySafetyException.Offline(),
        )
        assertEquals(
            listOf(
                "safety.bad-request", "safety.detail-too-long", "safety.self-target", "safety.auth-required",
                "safety.not-found", "safety.block-limit", "safety.rate-limited", "safety.unavailable", "safety.offline",
            ),
            refusals.map { it.code },
        )
        refusals.forEach { assertEquals(it.code, it.message) }
        assertEquals(42L, (refusals[6] as CommunitySafetyException.RateLimited).retryAfterSeconds)
    }
}
