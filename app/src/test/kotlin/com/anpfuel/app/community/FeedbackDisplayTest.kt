package com.anpfuel.app.community

import com.anpfuel.domain.repository.FeedbackRejectKind
import com.anpfuel.domain.repository.VoteTallySnapshot
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * P17-T02 RED: feedback presentation helpers.
 *
 * Agreement renders from exact counts with floor truncation and one
 * decimal (2 valid / 1 invalid → 66.6%); zero votes render "No
 * votes", never 0% certainty. The 280-scalar counter shares the
 * portable rule. Rejection labels are fixed short codes — private
 * report reasons and dispute detail never appear in copy.
 */
class FeedbackDisplayTest {

    @Test
    fun `agreement truncates to one decimal with counts`() {
        val line = FeedbackDisplay.agreementLine(VoteTallySnapshot("c-1", 1, 2, 1))
        assertTrue(line.contains("66.6%"))
        assertTrue(line.contains("2"))
        assertTrue(line.contains("1"))
    }

    @Test
    fun `zero votes renders no votes`() {
        assertEquals("No votes", FeedbackDisplay.agreementLine(VoteTallySnapshot("c-1", 1, 0, 0)))
    }

    @Test
    fun `chars remaining follows the 280 rule`() {
        assertEquals(0, FeedbackDisplay.charsRemaining("x".repeat(280)))
        assertEquals(-1, FeedbackDisplay.charsRemaining("x".repeat(281)))
        assertEquals(279, FeedbackDisplay.charsRemaining("é"))
    }

    @Test
    fun `reject labels are fixed codes`() {
        assertEquals("STALE", FeedbackDisplay.rejectKindLabel(FeedbackRejectKind.STALE_REVISION))
        assertEquals("SELF_VOTE", FeedbackDisplay.rejectKindLabel(FeedbackRejectKind.SELF_VOTE))
        assertEquals("QUOTA", FeedbackDisplay.rejectKindLabel(FeedbackRejectKind.QUOTA_EXCEEDED))
        assertEquals("TRANSPORT", FeedbackDisplay.rejectKindLabel(FeedbackRejectKind.TRANSPORT))
        assertEquals("NOT_FOUND", FeedbackDisplay.rejectKindLabel(FeedbackRejectKind.COMMENT_NOT_FOUND))
    }
}
