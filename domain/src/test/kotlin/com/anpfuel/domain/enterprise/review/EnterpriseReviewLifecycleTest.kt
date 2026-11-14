package com.anpfuel.domain.enterprise.review

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotSame
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test

class EnterpriseReviewLifecycleTest {
    private fun id(n: Int) = "9e000000-0000-4000-8000-" + n.toString().padStart(12, '0')
    private val author = ReviewIdentity(listOf(id(1)), listOf(id(11)), listOf(id(21)))
    private val reviewer = ReviewIdentity(listOf(id(2)), listOf(id(12)), listOf(id(22)))
    private fun facts(kind: ReviewCommandKind) = ReviewFacts(if (kind.role == ReviewRole.AUTHOR) author else reviewer, author, permitted = true)
    private fun ReviewCase.run(kind: ReviewCommandKind, reason: String = "") = apply(ReviewCommand(kind, revision, reason), facts(kind))
    private fun inReview() = ReviewCase.draft().run(ReviewCommandKind.SUBMIT).run(ReviewCommandKind.OPEN)
    private fun failure(block: () -> Unit) = assertThrows(ReviewDomainException::class.java) { block() }.failure

    @Test fun `draft starts at revision one with no history`() {
        val draft = ReviewCase.draft()
        assertEquals(ReviewState.DRAFT, draft.state)
        assertEquals(1L, draft.revision)
        assertEquals(emptyList<ReviewEntry>(), draft.history)
        assertEquals(280, ReviewCase.MAX_REASON_SCALARS)
    }

    @Test fun `history is an append-only copy`() {
        val base = inReview()
        val exposed = base.history
        assertNotSame(exposed, base.history)
        @Suppress("UNCHECKED_CAST")
        (exposed as? MutableList<ReviewEntry>)?.let { runCatching { it.clear() } }
        assertEquals(2, base.history.size)
        val stored = base.history.toMutableList()
        val replayed = ReviewCase.replay(stored)
        stored[1] = stored[1].copy(state = ReviewState.APPROVED)
        assertEquals(ReviewState.IN_REVIEW, replayed.history[1].state)
        val approved = base.run(ReviewCommandKind.APPROVE)
        val returned = base.run(ReviewCommandKind.RETURN, "Missing receipt")
        assertEquals(ReviewCommandKind.APPROVE, approved.history[2].kind)
        assertEquals(ReviewCommandKind.RETURN, returned.history[2].kind)
        assertEquals(2, base.history.size)
    }

    @Test fun `reason rule rejects lone surrogates and counts scalars`() {
        val case = inReview()
        assertEquals(ReviewFailure.INVALID, failure { case.run(ReviewCommandKind.RETURN, "bad\uD800reason") })
        assertEquals(ReviewFailure.INVALID, failure { case.run(ReviewCommandKind.RETURN, "\uDC00") })
        assertEquals(ReviewFailure.INVALID, failure { case.run(ReviewCommandKind.RETURN, "\u000B") })
        assertEquals(ReviewFailure.REASON_REQUIRED, failure { case.run(ReviewCommandKind.RETURN, "\u2003") })
        assertEquals(ReviewState.RETURNED, case.run(ReviewCommandKind.RETURN, "ç".repeat(280)).state)
        assertEquals(ReviewFailure.INVALID, failure { case.run(ReviewCommandKind.RETURN, "ç".repeat(281)) })
        assertEquals(ReviewState.RETURNED, case.run(ReviewCommandKind.RETURN, "zero\u200Bwidth").state)
        assertEquals(ReviewState.RETURNED, case.run(ReviewCommandKind.RETURN, "\u200B").state)
    }

    @Test fun `identity needs at least one canonical identifier`() {
        val case = inReview()
        val ok = ReviewFacts(ReviewIdentity(accounts = listOf(id(2), id(2))), author, ReviewIdentity(employees = listOf(id(23))), permitted = true)
        assertEquals(ReviewState.APPROVED, case.apply(ReviewCommand(ReviewCommandKind.APPROVE, 1), ok).state)
        val empty = ok.copy(actor = ReviewIdentity(accounts = listOf(id(2)), memberships = listOf("")))
        assertEquals(ReviewFailure.INVALID, failure { case.apply(ReviewCommand(ReviewCommandKind.APPROVE, 1), empty) })
        val defaults = ReviewFacts(reviewer, author)
        assertEquals(ReviewFailure.FORBIDDEN, failure { case.apply(ReviewCommand(ReviewCommandKind.APPROVE, 1), defaults) })
    }

    @Test fun `tokens are exact and closed`() {
        listOf("READY_TO_SEND", "approved", "VOIDED", "").forEach { assertEquals(ReviewFailure.INVALID, failure { ReviewState.parse(it) }) }
        listOf("DECIDE", "approve", "APPROVED", "").forEach { assertEquals(ReviewFailure.INVALID, failure { ReviewCommandKind.parse(it) }) }
        assertEquals(ReviewState.entries.toSet(), ReviewState.entries.map { ReviewState.parse(it.name) }.toSet())
    }

    @Test fun `presentation never turns local work into a server state`() {
        assertEquals(ReviewPresentation(ReviewState.DRAFT, serverConfirmed = false, readyToSend = true), ReviewPresenter.present(null, ReviewCommandKind.SUBMIT))
        for (kind in listOf(ReviewCommandKind.OPEN, ReviewCommandKind.APPROVE, ReviewCommandKind.RETURN, ReviewCommandKind.REJECT)) {
            for (server in listOf(null) + ReviewState.entries) {
                assertEquals(ReviewFailure.INVALID, failure { ReviewPresenter.present(server, kind) })
            }
        }
    }
}
