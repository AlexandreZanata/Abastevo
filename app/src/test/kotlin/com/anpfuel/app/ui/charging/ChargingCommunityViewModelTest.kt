package com.anpfuel.app.ui.charging

import androidx.lifecycle.SavedStateHandle
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.usecase.charging.ChargingCommunitySnapshot
import com.anpfuel.application.usecase.charging.ChargingCommunityUseCase
import com.anpfuel.application.usecase.charging.ChargingSaveOutcome
import com.anpfuel.application.usecase.network.ObserveNetworkConnectivityUseCase
import com.anpfuel.domain.charging.ChargingCondition
import com.anpfuel.domain.charging.ChargingConnector
import com.anpfuel.domain.charging.ChargingObservationException
import com.anpfuel.domain.charging.PendingChargingKind
import com.anpfuel.domain.charging.ChargingReportReason
import com.anpfuel.domain.charging.ChargingTally
import com.anpfuel.domain.charging.ChargingVote
import com.anpfuel.domain.charging.CommunityChargingObservation
import com.anpfuel.domain.charging.OwnedChargingObservation
import com.anpfuel.domain.charging.PendingChargingOperation
import com.anpfuel.domain.portable.PortableAuth
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class ChargingCommunityViewModelTest {
    private val dispatcher = StandardTestDispatcher()
    private val place = "11111111-1111-4111-8111-111111111111"
    private val useCase = mockk<ChargingCommunityUseCase>(relaxed = true)
    private val connectivity = mockk<ObserveNetworkConnectivityUseCase>()
    private val sessions = mockk<AuthSessionStore>()
    private val session = PortableAuth.Session("fam", "acct", "token", "refresh", Long.MAX_VALUE, Long.MAX_VALUE)
    private val observation = CommunityChargingObservation(
        "22222222-2222-4222-8222-222222222222", ChargingConnector.TYPE2, 1800,
        ChargingCondition.STANDARD, "2026-10-22", "n", 1, 0,
    )
    private val owned = OwnedChargingObservation(
        "22222222-2222-4222-8222-222222222222", place, ChargingConnector.TYPE2, 1800,
        ChargingCondition.STANDARD, "2026-10-22", "n", 2,
    )

    @BeforeEach fun setup() {
        Dispatchers.setMain(dispatcher)
        every { connectivity.invoke() } returns flowOf(true)
        every { sessions.load() } returns session
        every { useCase.pendingForCurrentOwner() } returns emptyList()
        coEvery { useCase.snapshot(place, 20) } returns ChargingCommunitySnapshot(listOf(observation), listOf(owned), emptyList(), false)
    }
    @AfterEach fun teardown() { Dispatchers.resetMain() }

    private fun model(): ChargingCommunityViewModel {
        val handle = SavedStateHandle(mapOf("placeId" to place))
        return ChargingCommunityViewModel(handle, useCase, connectivity, sessions)
    }

    @Test fun `offline save keeps a visible pending operation`() = runTest(dispatcher) {
        val pending = PendingChargingOperation(
            "33333333-3333-4333-8333-333333333333", PendingChargingKind.SUBMIT,
            null, null, com.anpfuel.domain.charging.ChargingObservationDraft(
                placeId = place, connector = ChargingConnector.TYPE2, priceMilliBrlPerKwh = 1800,
                condition = ChargingCondition.STANDARD, observedDay = "2026-10-22", note = "n",
            ), 1L,
        )
        coEvery { useCase.save(any(), any()) } returns ChargingSaveOutcome.Offline(listOf(pending))
        every { useCase.pendingForCurrentOwner() } returns listOf(pending)
        val vm = model(); advanceUntilIdle()
        vm.save(); advanceUntilIdle()
        assertEquals("offline", vm.uiState.value.notice)
        assertEquals(1, vm.uiState.value.pending.size)
        assertTrue(vm.uiState.value.offline)
    }

    @Test fun `invalid price and day never reach the use case`() = runTest(dispatcher) {
        val vm = model(); advanceUntilIdle()
        vm.setPrice("abc")
        vm.save(); advanceUntilIdle()
        assertEquals("price", vm.uiState.value.formError)
        vm.setPrice("1.80"); vm.setDay("22/10/2026")
        vm.save(); advanceUntilIdle()
        assertEquals("form", vm.uiState.value.formError)
        coVerify(exactly = 0) { useCase.save(any(), any()) }
    }

    @Test fun `edit prefills fields and withdraw stays explicit`() = runTest(dispatcher) {
        coEvery { useCase.withdraw(owned) } returns ChargingSaveOutcome.Saved(emptyList())
        val vm = model(); advanceUntilIdle()
        vm.edit(owned)
        assertEquals("1.8", vm.uiState.value.form.price)
        assertEquals(owned.id, vm.uiState.value.form.editingId)
        vm.withdraw(owned); advanceUntilIdle()
        assertEquals("withdrawn", vm.uiState.value.notice)
        coVerify(exactly = 1) { useCase.withdraw(owned) }
    }

    @Test fun `self vote refusal is explicit and never claims a tally`() = runTest(dispatcher) {
        coEvery { useCase.vote(observation.id, ChargingVote.HELPFUL) } throws ChargingObservationException.Denied("self")
        val vm = model(); advanceUntilIdle()
        vm.vote(observation, ChargingVote.HELPFUL); advanceUntilIdle()
        assertEquals("self", vm.uiState.value.notice)
        assertEquals(1L, vm.uiState.value.history.single().helpful)
    }

    @Test fun `signed out state disables writes and keeps public reads`() = runTest(dispatcher) {
        every { sessions.load() } returns null
        coEvery { useCase.snapshot(place, 20) } returns ChargingCommunitySnapshot(listOf(observation), emptyList(), emptyList(), false)
        val vm = model(); advanceUntilIdle()
        assertTrue(vm.uiState.value.signedOut)
        assertEquals(1, vm.uiState.value.history.size)
        vm.save(); advanceUntilIdle()
        coVerify(exactly = 0) { useCase.save(any(), any()) }
    }

    @Test fun `vote updates exact tallies from the server response`() = runTest(dispatcher) {
        coEvery { useCase.vote(observation.id, ChargingVote.NOT_HELPFUL) } returns ChargingTally(1, 2)
        val vm = model(); advanceUntilIdle()
        vm.vote(observation, ChargingVote.NOT_HELPFUL); advanceUntilIdle()
        assertEquals(2L, vm.uiState.value.history.single().notHelpful)
        assertEquals("voted", vm.uiState.value.notice)
    }
}
