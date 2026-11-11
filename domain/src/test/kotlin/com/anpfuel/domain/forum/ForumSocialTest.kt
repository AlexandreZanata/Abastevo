package com.anpfuel.domain.forum

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class ForumSocialTest {

    @Test
    fun `tapping the active reaction clears it and the other one switches`() {
        assertEquals(ForumReaction.LIKE, ForumReaction.NONE.toggle(ForumReaction.LIKE))
        assertEquals(ForumReaction.NONE, ForumReaction.LIKE.toggle(ForumReaction.LIKE))
        assertEquals(ForumReaction.DISLIKE, ForumReaction.LIKE.toggle(ForumReaction.DISLIKE))
    }

    @Test
    fun `counts move one vote and never go below zero`() {
        var counts = ForumReactionCounts()
        counts = counts.applying(ForumReaction.NONE, ForumReaction.LIKE)
        assertEquals(ForumReactionCounts(1, 0), counts)
        counts = counts.applying(ForumReaction.LIKE, ForumReaction.DISLIKE)
        assertEquals(ForumReactionCounts(0, 1), counts)
        counts = counts.applying(ForumReaction.DISLIKE, ForumReaction.DISLIKE)
        assertEquals(ForumReactionCounts(0, 1), counts)
        assertEquals(ForumReactionCounts(0, 0), ForumReactionCounts().applying(ForumReaction.LIKE, ForumReaction.NONE))
    }

    @Test
    fun `unknown wire reaction reads as none`() {
        assertEquals(ForumReaction.LIKE, ForumReaction.fromWire("LIKE"))
        assertEquals(ForumReaction.NONE, ForumReaction.fromWire("love"))
        assertEquals(ForumReaction.NONE, ForumReaction.fromWire(null))
    }
}
