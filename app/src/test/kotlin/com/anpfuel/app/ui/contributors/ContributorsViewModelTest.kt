package com.anpfuel.app.ui.contributors

import com.anpfuel.application.usecase.contributor.GetContributorsUseCase
import com.anpfuel.domain.contributor.Contributor
import com.anpfuel.domain.contributor.ContributorException
import com.anpfuel.domain.contributor.ContributorPage
import io.mockk.coEvery
import io.mockk.mockk
import java.io.IOException
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
class ContributorsViewModelTest {
    private val dispatcher = StandardTestDispatcher()
    private val useCase = mockk<GetContributorsUseCase>()
    private val ana = Contributor("Ana", emptyList(), "octocat", "2026-03", listOf("Suporte"))
    private val bia = Contributor("Bia", emptyList(), null, "", emptyList())

    @BeforeEach fun setup() { Dispatchers.setMain(dispatcher) }
    @AfterEach fun teardown() { Dispatchers.resetMain() }

    private fun model() = ContributorsViewModel(useCase)

    @Test fun `first page renders items`() = runTest(dispatcher) {
        coEvery { useCase(null) } returns ContributorPage(listOf(ana), "")
        val vm = model()
        vm.refresh()
        advanceUntilIdle()
        assertEquals(listOf(ana), vm.uiState.value.items)
        assertFalse(vm.uiState.value.loading)
        assertNull(vm.uiState.value.notice)
        assertFalse(vm.uiState.value.hasMore)
    }

    @Test fun `empty page renders the empty state`() = runTest(dispatcher) {
        coEvery { useCase(null) } returns ContributorPage(emptyList(), "")
        val vm = model()
        vm.refresh()
        advanceUntilIdle()
        assertTrue(vm.uiState.value.items.isEmpty())
        assertNull(vm.uiState.value.notice)
    }

    @Test fun `offline surfaces retryable notice`() = runTest(dispatcher) {
        coEvery { useCase(null) } throws ContributorException.Offline("contributors.offline")
        val vm = model()
        vm.refresh()
        advanceUntilIdle()
        assertEquals(ContributorsNotice.OFFLINE, vm.uiState.value.notice)
        coEvery { useCase(null) } returns ContributorPage(listOf(ana), "")
        vm.refresh()
        advanceUntilIdle()
        assertNull(vm.uiState.value.notice)
        assertEquals(1, vm.uiState.value.items.size)
    }

    @Test fun `denied and errors map distinctly`() = runTest(dispatcher) {
        coEvery { useCase(null) } throws ContributorException.Denied("contributors.denied")
        val denied = model()
        denied.refresh()
        advanceUntilIdle()
        assertEquals(ContributorsNotice.DENIED, denied.uiState.value.notice)
        coEvery { useCase(null) } throws ContributorException.Unavailable("contributors.unavailable")
        val failed = model()
        failed.refresh()
        advanceUntilIdle()
        assertEquals(ContributorsNotice.ERROR, failed.uiState.value.notice)
    }

    @Test fun `cursor pages append and stop at empty cursor`() = runTest(dispatcher) {
        coEvery { useCase(null) } returns ContributorPage(listOf(ana), "c2")
        coEvery { useCase("c2") } returns ContributorPage(listOf(bia), "")
        val vm = model()
        vm.refresh()
        advanceUntilIdle()
        assertTrue(vm.uiState.value.hasMore)
        vm.loadMore()
        advanceUntilIdle()
        assertEquals(listOf(ana, bia), vm.uiState.value.items)
        assertFalse(vm.uiState.value.hasMore)
        assertFalse(vm.uiState.value.loadingMore)
    }

    @Test fun `late responses never overwrite a newer refresh`() = runTest(dispatcher) {
        coEvery { useCase(null) } returns ContributorPage(listOf(ana), "")
        val vm = model()
        vm.refresh()
        vm.refresh()
        advanceUntilIdle()
        assertEquals(1, vm.uiState.value.items.size)
    }
}
