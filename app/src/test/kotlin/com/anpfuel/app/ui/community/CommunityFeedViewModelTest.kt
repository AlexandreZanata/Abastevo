package com.anpfuel.app.ui.community

import com.anpfuel.application.usecase.community.GetCityCommunityFeedUseCase
import com.anpfuel.domain.community.*
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.FuelProduct
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import java.time.Instant
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.test.*
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.Assertions.*

@OptIn(ExperimentalCoroutinesApi::class)
class CommunityFeedViewModelTest {
    private val dispatcher = StandardTestDispatcher()
    private val useCase = mockk<GetCityCommunityFeedUseCase>()
    private val city = FeedCity("5107925", BrazilianState.MATO_GROSSO, "Sorriso")
    private val at = Instant.parse("2026-10-06T12:00:00Z")
    private fun item(price: Long = 5999) = CommunityFeedItem("00000000-0000-0000-0000-000000000001", "Synthetic station", FuelProduct.GASOLINE_REGULAR,
        price, at.minusSeconds(60), at.plusSeconds(3600), 1, 0, "LOW", 1)
    @BeforeEach fun setup() { Dispatchers.setMain(dispatcher); coEvery { useCase.city() } returns city }
    @AfterEach fun cleanup() { Dispatchers.resetMain() }
    private fun model() = CommunityFeedViewModel(useCase).also { it.now = { at } }

    @Test fun `foreground refresh sees updated prices and stops on tab exit`() = runTest(dispatcher) {
        var price = 5999L
        coEvery { useCase(any(), any()) } answers { CommunityFeedPage(listOf(item(price)), null, at) }
        val vm = model(); vm.setForeground(true); runCurrent()
        assertEquals(5999L, vm.state.value.items.single().amountMilliBrl)
        price = 5980; advanceTimeBy(15000); runCurrent()
        assertEquals(5980L, vm.state.value.items.single().amountMilliBrl)
        vm.setForeground(false); advanceTimeBy(60000); runCurrent()
        coVerify(exactly = 2) { useCase(any(), any()) }
    }
    @Test fun `failed manual refresh retains same city data but expired prices disappear`() = runTest(dispatcher) {
        coEvery { useCase(any(), any()) } returns CommunityFeedPage(listOf(item()), null, at)
        val vm = model(); vm.setForeground(true); runCurrent()
        coEvery { useCase(any(), any()) } throws java.io.IOException("unavailable")
        vm.refresh(); runCurrent()
        assertTrue(vm.state.value.failed); assertEquals(1, vm.state.value.items.size)
        vm.now = { at.plusSeconds(3601) }; advanceTimeBy(15000); runCurrent()
        assertTrue(vm.state.value.items.isEmpty()); vm.setForeground(false)
    }
    @Test fun `city and fuel changes cannot retain previous scope`() = runTest(dispatcher) {
        coEvery { useCase(any(), any()) } returns CommunityFeedPage(listOf(item()), null, at)
        val vm = model(); vm.setForeground(true); runCurrent()
        val next = FeedCity("5103403", BrazilianState.MATO_GROSSO, "Cuiaba")
        coEvery { useCase.city() } returns next
        coEvery { useCase(any(), any()) } returns CommunityFeedPage(emptyList(), null, at)
        advanceTimeBy(15000); runCurrent()
        assertEquals(next, vm.state.value.city); assertTrue(vm.state.value.items.isEmpty())
        vm.selectFuel(FuelProduct.ETHANOL); runCurrent()
        coVerify { useCase(match { it.city == next && it.fuel == FuelProduct.ETHANOL }, null) }
        vm.setForeground(false)
    }
    @Test fun `obsolete network requests are cancelled when filter changes`() = runTest(dispatcher) {
        var cancelled = false
        coEvery { useCase(any(), any()) } coAnswers { try { awaitCancellation() } finally { cancelled = true } }
        val vm = model(); vm.setForeground(true); runCurrent()
        vm.selectSort(FeedSort.CHEAPEST); runCurrent()
        assertTrue(cancelled); assertEquals(FeedSort.CHEAPEST, vm.state.value.sort)
        vm.setForeground(false); runCurrent()
    }
    @Test fun `missing city makes no network requests`() = runTest(dispatcher) {
        coEvery { useCase.city() } returns null
        val vm = model(); vm.setForeground(true); runCurrent()
        assertTrue(vm.state.value.noCity); coVerify(exactly = 0) { useCase(any(), any()) }
        vm.setForeground(false)
    }
    @Test fun `pagination deduplicates stations and failed page keeps existing cards`() = runTest(dispatcher) {
        coEvery { useCase(any(), null) } returns CommunityFeedPage(listOf(item()), "page-two", at)
        coEvery { useCase(any(), "page-two") } returns CommunityFeedPage(listOf(item()), "page-three", at)
        val vm = model(); vm.setForeground(true); runCurrent()
        vm.loadMore(); runCurrent()
        assertEquals(1, vm.state.value.items.size)
        coEvery { useCase(any(), "page-three") } throws java.io.IOException("unavailable")
        vm.loadMore(); runCurrent()
        assertTrue(vm.state.value.failed); assertEquals(1, vm.state.value.items.size)
        vm.setForeground(false)
    }

    @Test fun `selecting a city after initial absence shows connection error rather than missing city`() = runTest(dispatcher) {
        coEvery { useCase.city() } returns null
        val vm = model(); vm.setForeground(true); runCurrent()
        assertTrue(vm.state.value.noCity)
        coEvery { useCase.city() } returns city
        coEvery { useCase(any(), any()) } throws java.io.IOException("unavailable")
        advanceTimeBy(15000); runCurrent()
        assertEquals(city, vm.state.value.city)
        assertFalse(vm.state.value.noCity); assertTrue(vm.state.value.failed)
        vm.setForeground(false)
    }

    @Test fun `automatic updates preserve browsing older pages until explicit refresh`() = runTest(dispatcher) {
        val first = (1..20).map { item().copy(stationId = "00000000-0000-0000-0000-" + it.toString().padStart(12, '0')) }
        coEvery { useCase(any(), null) } returns CommunityFeedPage(first, "more", at)
        coEvery { useCase(any(), "more") } returns CommunityFeedPage(listOf(item().copy(stationId = "00000000-0000-0000-0000-000000000021")), null, at)
        val vm = model(); vm.setForeground(true); runCurrent(); vm.loadMore(); runCurrent()
        assertEquals(21, vm.state.value.items.size)
        val updated = first.map { it.copy(amountMilliBrl = 5980) }
        coEvery { useCase(any(), null) } returns CommunityFeedPage(updated, "more", at)
        advanceTimeBy(15000); runCurrent()
        assertTrue(vm.state.value.newUpdates); assertEquals(21, vm.state.value.items.size)
        vm.refresh(); runCurrent()
        assertEquals(20, vm.state.value.items.size); assertEquals(5980L, vm.state.value.items.first().amountMilliBrl)
        assertFalse(vm.state.value.newUpdates); vm.setForeground(false)
    }

}
