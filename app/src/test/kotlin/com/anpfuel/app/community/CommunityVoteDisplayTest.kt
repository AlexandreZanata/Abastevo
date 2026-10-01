package com.anpfuel.app.community

import com.anpfuel.domain.repository.CommunityDisputeReasons
import com.anpfuel.domain.repository.CommunityVoteRejectKind
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * P10-T07: conditions stay visible before either action, the six
 * dispute reasons label honestly, only a changed price asks for a
 * replacement, and rejection kinds stay explicit.
 */
class CommunityVoteDisplayTest {

    @Test
    fun `summary carries amount unit and full condition`() {
        val summary = CommunityVoteDisplay.confirmSummary(
            productWire = "GASOLINE_REGULAR",
            amountMilliBrl = 5890L,
            unit = "BRL/L",
            conditionLabel = "STANDARD",
        )

        assertTrue(summary.contains("GASOLINE_REGULAR"))
        assertTrue(summary.contains("R$ 5,890"))
        assertTrue(summary.contains("BRL/L"))
        assertTrue(summary.contains("STANDARD"))
    }

    @Test
    fun `all six reasons label and only price-changed needs replacement`() {
        assertEquals(6, CommunityVoteDisplay.disputeReasons().size)
        CommunityDisputeReasons.ALL.forEach { reason ->
            val label = CommunityVoteDisplay.disputeReasonLabel(reason)
            assertTrue(label.isNotBlank())
        }
        assertTrue(CommunityVoteDisplay.needsReplacement(CommunityDisputeReasons.PRICE_CHANGED))
        assertFalse(CommunityVoteDisplay.needsReplacement(CommunityDisputeReasons.WRONG_PRODUCT))
        assertFalse(CommunityVoteDisplay.needsReplacement(CommunityDisputeReasons.OTHER))
    }

    @Test
    fun `unknown reason refuses`() {
        assertThrows(IllegalArgumentException::class.java) {
            CommunityVoteDisplay.disputeReasonLabel("MADE_UP")
        }
    }

    @Test
    fun `reject kinds label explicitly`() {
        assertEquals("SELF", CommunityVoteDisplay.rejectKindLabel(CommunityVoteRejectKind.SELF_CONFIRMATION))
        assertEquals(
            "ALREADY_RECORDED",
            CommunityVoteDisplay.rejectKindLabel(CommunityVoteRejectKind.ALREADY_RECORDED),
        )
        assertEquals(
            "INELIGIBLE",
            CommunityVoteDisplay.rejectKindLabel(CommunityVoteRejectKind.INELIGIBLE_TARGET),
        )
    }
}
