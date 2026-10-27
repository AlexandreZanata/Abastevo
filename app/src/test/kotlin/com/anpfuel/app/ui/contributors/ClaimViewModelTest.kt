package com.anpfuel.app.ui.contributors

import com.anpfuel.application.usecase.contributor.OpenClaimUseCase
import com.anpfuel.application.usecase.contributor.ReadOwnClaimsUseCase
import com.anpfuel.application.usecase.contributor.SubmitClaimProofUseCase
import com.anpfuel.application.usecase.contributor.WithdrawClaimUseCase
import com.anpfuel.domain.contributor.Claim
import com.anpfuel.domain.contributor.ClaimProof
import com.anpfuel.domain.contributor.ClaimState
import com.anpfuel.domain.contributor.ContributorException
import com.anpfuel.domain.contributor.IssuedClaim
import io.mockk.coEvery
import io.mockk.mockk
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class ClaimViewModelTest {
    private val dispatcher = StandardTestDispatcher()
    private val openClaim = mockk<OpenClaimUseCase>()
    private val readClaims = mockk<ReadOwnClaimsUseCase>()
    private val submitProof = mockk<SubmitClaimProofUseCase>()
    private val withdrawClaim = mockk<WithdrawClaimUseCase>()
    private val claim = Claim(
        id = "c1", githubLogin = "octocat", state = ClaimState.PENDING, proof = ClaimProof.UNVERIFIED,
        gistId = null, override = false, createdAt = "", expiresAt = "",
    )

    @BeforeEach fun setup() { Dispatchers.setMain(dispatcher) }
    @AfterEach fun teardown() { Dispatchers.resetMain() }

    private fun model() = ClaimViewModel(openClaim, readClaims, submitProof, withdrawClaim)

    @Test fun `refresh lists claims`() = runTest(dispatcher) {
        coEvery { readClaims() } returns listOf(claim)
        val vm = model()
        vm.refresh()
        advanceUntilIdle()
        assertEquals(listOf(claim), vm.uiState.value.claims)
        assertNull(vm.uiState.value.notice)
    }

    @Test fun `open issues the code and reloads`() = runTest(dispatcher) {
        val issued = IssuedClaim("c1", "abastevo-contributor-AAAAAAAAAAAAAAAA")
        coEvery { openClaim("octocat") } returns issued
        coEvery { readClaims() } returns listOf(claim)
        val vm = model()
        vm.open("octocat")
        advanceUntilIdle()
        assertEquals(issued, vm.uiState.value.issued)
        assertEquals(1, vm.uiState.value.claims.size)
        assertFalse(vm.uiState.value.loading)
        assertFalse(vm.uiState.value.working)
        vm.dismissIssued()
        assertNull(vm.uiState.value.issued)
    }

    @Test fun `quota conflict and offline map distinctly`() = runTest(dispatcher) {
        coEvery { readClaims() } returns emptyList()
        coEvery { openClaim(any()) } throws ContributorException.Quota("contributor.quota") andThen
            IssuedClaim("c1", "code")
        val vm = model()
        vm.open("a")
        advanceUntilIdle()
        assertEquals(ClaimNotice.QUOTA, vm.uiState.value.notice)
        vm.open("b")
        advanceUntilIdle()
        assertNull(vm.uiState.value.notice)
    }

    @Test fun `proof and withdraw reload the list`() = runTest(dispatcher) {
        coEvery { readClaims() } returns listOf(claim)
        coEvery { submitProof("c1", "gist1") } returns claim
        coEvery { withdrawClaim("c1") } returns claim
        val vm = model()
        vm.prove("c1", "gist1")
        advanceUntilIdle()
        vm.withdraw("c1")
        advanceUntilIdle()
        assertEquals(1, vm.uiState.value.claims.size)
        assertNull(vm.uiState.value.notice)
    }

    @Test fun `denied maps to the signed-out notice`() = runTest(dispatcher) {
        coEvery { readClaims() } throws ContributorException.Denied("contributors.signed-out")
        val vm = model()
        vm.refresh()
        advanceUntilIdle()
        assertEquals(ClaimNotice.DENIED, vm.uiState.value.notice)
        assertTrue(vm.uiState.value.claims.isEmpty())
    }

    @Test fun `invalid input maps to its own notice`() = runTest(dispatcher) {
        coEvery { openClaim(any()) } throws ContributorException.Invalid("contributors.invalid-login")
        val vm = model()
        vm.open("-bad-")
        advanceUntilIdle()
        assertEquals(ClaimNotice.INVALID, vm.uiState.value.notice)
    }
}
