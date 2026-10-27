package com.anpfuel.application.usecase.ledger

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.portable.PortableAuth
import com.anpfuel.domain.premium.*
import com.anpfuel.domain.repository.*
import io.mockk.*
import java.time.Instant
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class ReviewLedgerOdometerUseCaseTest {
    private val sessions=mockk<AuthSessionStore>()
    private val drafts=mockk<LedgerDraftStore>()
    private val useCase=SaveLedgerDraftUseCase(drafts,sessions,"local-origin",nowMillis={10})
    private fun session(owner:String)=PortableAuth.Session("family",owner,"synthetic","synthetic",0,0)
    private val draft=ExpenseDraft("11111111-1111-4111-8111-111111111111","22222222-2222-4222-8222-222222222222",Instant.parse("2026-03-01T12:00:00Z"),"UTC",5000,ExpenseCategory.TOLLS,10000,null)
    @Test fun `review uses actual owner and refuses changed owner before exposing its result`() = runTest {
        every { sessions.load() } returnsMany listOf(session("a"),session("b"))
        coEvery { drafts.reviewOdometer("a","local-origin",draft.dataset,draft.recordId,"a".repeat(64),OdometerReviewChoice.AS_METRES,10) } returns StoredLedgerDraft(draft,0,10,odometerEvidence=OdometerEvidence.VERIFIED_METRES)
        try { useCase.reviewOdometer(draft.dataset,draft.recordId,"a".repeat(64),OdometerReviewChoice.AS_METRES); fail("changed owner accepted") }
        catch(e:PremiumValidationException) { assertEquals("owner-scope-changed",e.code) }
        coVerify(exactly=0) { drafts.reviewOdometer("b",any(),any(),any(),any(),any(),any()) }
    }
    @Test fun `invalid review fingerprint performs no storage io`() = runTest {
        try { useCase.reviewOdometer(draft.dataset,draft.recordId,"bad",OdometerReviewChoice.AS_METRES); fail("bad base accepted") }
        catch(e:PremiumValidationException) { assertEquals("invalid-review-base",e.code) }
        coVerify(exactly=0) { drafts.reviewOdometer(any(),any(),any(),any(),any(),any(),any()) }
    }
    @Test fun `fresh explicitly verified input retains owner and evidence in the store`() = runTest {
        every { sessions.load() } returns session("a")
        coEvery { drafts.save("a","local-origin",draft,0,10,null,OdometerEvidence.VERIFIED_METRES) } returns StoredLedgerDraft(draft,0,10,odometerEvidence=OdometerEvidence.VERIFIED_METRES)
        assertEquals(OdometerEvidence.VERIFIED_METRES,useCase(draft,0,null,OdometerEvidence.VERIFIED_METRES).odometerEvidence)
    }
}
