package com.anpfuel.app.ui.community

import com.anpfuel.application.usecase.community.GetCityCommunityFeedUseCase
import com.anpfuel.application.usecase.network.ObserveNetworkConnectivityUseCase
import com.anpfuel.domain.community.*
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.FuelProduct
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import java.time.Instant
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.test.*
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.Assertions.*

@OptIn(ExperimentalCoroutinesApi::class)
class CommunityFeedViewModelTest {
    private val dispatcher = StandardTestDispatcher()
    private val useCase = mockk<GetCityCommunityFeedUseCase>()
    private val snapshots = mockk<CommunityFeedLocalSnapshot>()
    private val connectivity = mockk<ObserveNetworkConnectivityUseCase>()
    private val connected = MutableStateFlow(true)
    private val city = FeedCity("5107925", BrazilianState.MATO_GROSSO, "Sorriso")
    private val at = Instant.parse("2026-10-06T12:00:00Z")
    private fun item(price: Long = 5999) = CommunityFeedItem("00000000-0000-0000-0000-000000000001", "Synthetic station", FuelProduct.GASOLINE_REGULAR,
        price, at.minusSeconds(60), at.plusSeconds(3600), 1, 0, "LOW", 1)
    @BeforeEach fun setup() {
        Dispatchers.setMain(dispatcher); coEvery { useCase.city() } returns city
        coEvery { snapshots.snapshotFeed(any(), any(), any()) } returns null
        every { connectivity() } returns connected
    }
    @AfterEach fun cleanup() { Dispatchers.resetMain() }
    private fun model() = CommunityFeedViewModel(useCase, snapshots, connectivity).also { it.now = { at } }

    @Test fun `foreground refresh sees updated prices and stops on tab exit`() = runTest(dispatcher) {
        var price = 5999L
        var generated = at
        coEvery { useCase(any(), any()) } answers { CommunityFeedPage(listOf(item(price)), null, generated) }
        val vm = model(); vm.setForeground(true); runCurrent()
        assertEquals(5999L, vm.state.value.items.single().amountMilliBrl)
        price = 5980; generated = at.plusSeconds(61); vm.now = { at.plusSeconds(61) }; advanceTimeBy(60000); runCurrent()
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
        vm.now = { at.plusSeconds(61) }
        coEvery { useCase(any(), any()) } returns CommunityFeedPage(emptyList(), null, at)
        advanceTimeBy(30000); runCurrent()
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
    @Test fun `best and worst sorts refresh with the rating order`() = runTest(dispatcher) {
        coEvery { useCase(any(), any()) } answers { CommunityFeedPage(listOf(item()), null, at) }
        val vm = model(); vm.setForeground(true); runCurrent()
        vm.selectSort(FeedSort.BEST); runCurrent()
        assertEquals(FeedSort.BEST, vm.state.value.sort)
        coVerify { useCase(match { it.sort == FeedSort.BEST }, null) }
        vm.selectSort(FeedSort.WORST); runCurrent()
        assertEquals(FeedSort.WORST, vm.state.value.sort)
        coVerify { useCase(match { it.sort == FeedSort.WORST }, null) }
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
        advanceTimeBy(30000); runCurrent()
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
        vm.now = { at.plusSeconds(61) }
        advanceTimeBy(30000); runCurrent()
        assertTrue(vm.state.value.newUpdates); assertEquals(21, vm.state.value.items.size)
        vm.refresh(); runCurrent()
        assertEquals(20, vm.state.value.items.size); assertEquals(5980L, vm.state.value.items.first().amountMilliBrl)
        assertFalse(vm.state.value.newUpdates); vm.setForeground(false)
    }

    @Test fun `continuation reaches beyond two hundred rows with bounded memory and explicit restart`() = runTest(dispatcher) {
        coEvery { useCase(any(), any()) } answers {
            val index = (secondArg<String?>()?.removePrefix("p")?.toInt() ?: 0)
            val rows = (index * 20 until index * 20 + 20).map { number -> item().copy(
                stationId = "00000000-0000-4000-8000-%012d".format(number)) }
            CommunityFeedPage(rows, if (index < 11) "p${index + 1}" else null, at)
        }
        val vm = model(); vm.setForeground(true); runCurrent()
        repeat(11) { vm.loadMore(); runCurrent() }
        assertEquals(200, vm.state.value.items.size)
        assertTrue(vm.state.value.earlierItemsTrimmed)
        assertEquals("00000000-0000-4000-8000-000000000239", vm.state.value.items.last().stationId)
        assertNull(vm.state.value.nextCursor)
        vm.refresh(); runCurrent()
        assertEquals(20, vm.state.value.items.size)
        assertFalse(vm.state.value.earlierItemsTrimmed)
        vm.setForeground(false)
    }

    @Test fun `fresh snapshot paints immediately without a network call`() = runTest(dispatcher) {
        val deferred = kotlinx.coroutines.CompletableDeferred<CommunityFeedPage>()
        coEvery { snapshots.snapshotFeed(city, FuelProduct.GASOLINE_REGULAR, FeedSort.RECENT) } returns CommunityFeedPage(listOf(item()), null, at)
        coEvery { useCase(any(), any()) } coAnswers { deferred.await() }
        val vm = model(); vm.setForeground(true); runCurrent()
        assertEquals(5999L, vm.state.value.items.single().amountMilliBrl)
        assertFalse(vm.state.value.loading)
        assertFalse(vm.state.value.stale)
        coVerify(exactly = 0) { useCase(any(), any()) }
        // Once the snapshot ages out, the scheduled evaluation refreshes from the network.
        vm.now = { at.plusSeconds(61) }
        deferred.complete(CommunityFeedPage(listOf(item(5980)), null, at.plusSeconds(61)))
        advanceTimeBy(30000); runCurrent()
        assertEquals(5980L, vm.state.value.items.single().amountMilliBrl)
        vm.setForeground(false); runCurrent()
    }

    @Test fun `aged snapshot paints with a stale mark`() = runTest(dispatcher) {
        val deferred = kotlinx.coroutines.CompletableDeferred<CommunityFeedPage>()
        coEvery { snapshots.snapshotFeed(city, FuelProduct.GASOLINE_REGULAR, FeedSort.RECENT) } returns
            CommunityFeedPage(listOf(item()), null, at.minusSeconds(120))
        coEvery { useCase(any(), any()) } coAnswers { deferred.await() }
        val vm = model(); vm.setForeground(true); runCurrent()
        assertEquals(1, vm.state.value.items.size)
        assertTrue(vm.state.value.stale)
        assertFalse(vm.state.value.failed)
        assertFalse(vm.state.value.loading)
        deferred.complete(CommunityFeedPage(listOf(item()), null, at))
        advanceTimeBy(30000); runCurrent()
        assertFalse(vm.state.value.stale)
        vm.setForeground(false); runCurrent()
    }

    @Test fun `offline entry paints the snapshot with an error mark and no query`() = runTest(dispatcher) {
        connected.value = false
        coEvery { snapshots.snapshotFeed(city, FuelProduct.GASOLINE_REGULAR, FeedSort.RECENT) } returns CommunityFeedPage(listOf(item()), null, at)
        val vm = model(); vm.setForeground(true); runCurrent()
        assertEquals(1, vm.state.value.items.size)
        assertTrue(vm.state.value.failed)
        coVerify(exactly = 0) { useCase(any(), any()) }
        vm.setForeground(false); runCurrent()
    }

    @Test fun `reconnect refreshes a stale foreground feed`() = runTest(dispatcher) {
        connected.value = false
        coEvery { snapshots.snapshotFeed(city, FuelProduct.GASOLINE_REGULAR, FeedSort.RECENT) } returns CommunityFeedPage(listOf(item()), null, at)
        coEvery { useCase(any(), any()) } returns CommunityFeedPage(listOf(item(5980)), null, at.plusSeconds(61))
        val vm = model(); vm.setForeground(true); runCurrent()
        assertTrue(vm.state.value.failed)
        vm.now = { at.plusSeconds(61) }
        connected.value = true; runCurrent()
        assertEquals(5980L, vm.state.value.items.single().amountMilliBrl)
        assertFalse(vm.state.value.failed)
        vm.setForeground(false); runCurrent()
    }

    @Test fun `returning to a fresh screen issues no query but stale data refreshes`() = runTest(dispatcher) {
        coEvery { useCase(any(), any()) } returns CommunityFeedPage(listOf(item()), null, at)
        val vm = model(); vm.setForeground(true); runCurrent()
        vm.setForeground(false); runCurrent()
        vm.setForeground(true); runCurrent()
        coVerify(exactly = 1) { useCase(any(), any()) }
        vm.now = { at.plusSeconds(61) }
        vm.setForeground(false); runCurrent()
        vm.setForeground(true); runCurrent()
        coVerify(exactly = 2) { useCase(any(), any()) }
        vm.setForeground(false); runCurrent()
    }

    @Test fun `retries back off and stop after the budget until manual refresh`() = runTest(dispatcher) {
        coEvery { useCase(any(), any()) } throws java.io.IOException("unavailable")
        var now = at
        val vm = model().also { it.now = { now } }
        vm.setForeground(true); runCurrent()
        assertTrue(vm.state.value.failed)
        coVerify(exactly = 1) { useCase(any(), any()) }
        // Spaced retries double (15 s, 30 s, 60 s); the budget allows five attempts total.
        fun tick(seconds: Long) {
            now = now.plusSeconds(seconds)
            advanceTimeBy(seconds * 1000); runCurrent()
        }
        tick(15)
        coVerify(exactly = 2) { useCase(any(), any()) }
        tick(30)
        coVerify(exactly = 3) { useCase(any(), any()) }
        tick(60)
        coVerify(exactly = 4) { useCase(any(), any()) }
        tick(300)
        coVerify(exactly = 5) { useCase(any(), any()) }
        // Budget exhausted: silence until an explicit event.
        tick(600)
        coVerify(exactly = 5) { useCase(any(), any()) }
        coEvery { useCase(any(), any()) } returns CommunityFeedPage(listOf(item()), null, at)
        vm.refresh(); runCurrent()
        coVerify(exactly = 6) { useCase(any(), any()) }
        assertFalse(vm.state.value.failed)
        vm.setForeground(false); runCurrent()
    }

    @Test fun `local sweep drops expired cards without extra network calls`() = runTest(dispatcher) {
        var generated = at
        coEvery { useCase(any(), any()) } answers { CommunityFeedPage(listOf(item()), null, generated) }
        val vm = model(); vm.setForeground(true); runCurrent()
        assertEquals(1, vm.state.value.items.size)
        vm.now = { at.plusSeconds(20) }
        advanceTimeBy(30000); runCurrent()
        assertEquals(1, vm.state.value.items.size)
        // The sweep drops expired cards; the aged update is refreshed once, then stays quiet.
        generated = at.plusSeconds(3700)
        vm.now = { at.plusSeconds(3700) }
        advanceTimeBy(30000); runCurrent()
        assertTrue(vm.state.value.items.isEmpty())
        coVerify(exactly = 2) { useCase(any(), any()) }
        vm.setForeground(false); runCurrent()
    }

    @Test fun `return after exhausted retries reopens the feed budget`() = runTest(dispatcher) {
        var clock = at
        val vm = model().also { it.now = { clock } }
        coEvery { useCase(any(), any()) } throws java.io.IOException("unavailable")
        vm.setForeground(true); runCurrent()
        repeat(4) { clock = clock.plusSeconds(300); advanceTimeBy(300_000); runCurrent() }
        coVerify(exactly = 5) { useCase(any(), any()) }
        vm.setForeground(false); runCurrent()
        clock = clock.plusSeconds(300)
        coEvery { useCase(any(), any()) } returns CommunityFeedPage(listOf(item()), null, clock)
        vm.setForeground(true); runCurrent()
        vm.setForeground(false)
        coVerify(exactly = 6) { useCase(any(), any()) }
        assertFalse(vm.state.value.failed)
    }

    @Test fun `offline cursor request retains cards without a query`() = runTest(dispatcher) {
        coEvery { useCase(any(), null) } returns CommunityFeedPage(listOf(item()), "more", at)
        coEvery { useCase(any(), "more") } returns CommunityFeedPage(emptyList(), null, at)
        val vm = model(); vm.setForeground(true); runCurrent()
        connected.value = false; runCurrent()
        vm.loadMore(); runCurrent()
        vm.setForeground(false)
        coVerify(exactly = 0) { useCase(any(), "more") }
        assertEquals(1, vm.state.value.items.size)
        assertTrue(vm.state.value.failed)
    }

    @Test fun `full fresh snapshot acquires a live cursor while retaining its cards`() = runTest(dispatcher) {
        val first = (1..20).map { item().copy(stationId = "00000000-0000-0000-0000-%012d".format(it)) }
        coEvery { snapshots.snapshotFeed(any(), any(), any()) } returns CommunityFeedPage(first, null, at)
        val head = kotlinx.coroutines.CompletableDeferred<CommunityFeedPage>()
        coEvery { useCase(any(), null) } coAnswers { head.await() }
        coEvery { useCase(any(), "live-more") } returns CommunityFeedPage(listOf(item().copy(stationId = "00000000-0000-0000-0000-000000000021")), null, at)
        val vm = model(); vm.setForeground(true); runCurrent()
        assertEquals(first, vm.state.value.items)
        head.complete(CommunityFeedPage(first, "live-more", at)); runCurrent()
        vm.loadMore(); runCurrent()
        vm.setForeground(false)
        coVerify(exactly = 1) { useCase(any(), null) }
        assertEquals(21, vm.state.value.items.size)
    }

    @Test fun `cancelled snapshot cannot paint into a newer fuel scope`() = runTest(dispatcher) {
        val old = kotlinx.coroutines.CompletableDeferred<CommunityFeedPage>()
        coEvery { snapshots.snapshotFeed(city, FuelProduct.GASOLINE_REGULAR, FeedSort.RECENT) } coAnswers {
            kotlinx.coroutines.withContext(kotlinx.coroutines.NonCancellable) { old.await() }
        }
        coEvery { useCase(any(), any()) } coAnswers { kotlinx.coroutines.awaitCancellation() }
        val vm = model(); vm.setForeground(true); runCurrent()
        vm.selectFuel(FuelProduct.ETHANOL); runCurrent()
        old.complete(CommunityFeedPage(listOf(item()), null, at)); runCurrent()
        vm.setForeground(false); runCurrent()
        assertEquals(FuelProduct.ETHANOL, vm.state.value.fuel)
        assertTrue(vm.state.value.items.isEmpty())
    }

    @Test fun `five foreground minutes refresh fresh data exactly six times`() = runTest(dispatcher) {
        val vm = model().also { it.now = { at.plusMillis(testScheduler.currentTime) } }
        coEvery { useCase(any(), null) } answers { CommunityFeedPage(listOf(item()), null, at.plusMillis(testScheduler.currentTime)) }
        try {
            vm.setForeground(true); runCurrent()
            advanceTimeBy(300_000); runCurrent()
            coVerify(exactly = 6) { useCase(any(), null) }
        } finally { vm.setForeground(false); runCurrent() }
    }

    @Test fun `five offline foreground minutes keep snapshot with zero calls`() = runTest(dispatcher) {
        connected.value = false
        coEvery { snapshots.snapshotFeed(any(), any(), any()) } returns CommunityFeedPage(listOf(item()), null, at)
        val vm = model().also { it.now = { at.plusMillis(testScheduler.currentTime) } }
        try {
            vm.setForeground(true); runCurrent()
            advanceTimeBy(300_000); runCurrent()
            coVerify(exactly = 0) { useCase(any(), any()) }
            assertEquals(1, vm.state.value.items.size)
            assertTrue(vm.state.value.failed)
        } finally { vm.setForeground(false); runCurrent() }
    }

    @Test fun `five outage foreground minutes make exactly five attempts`() = runTest(dispatcher) {
        coEvery { useCase(any(), null) } throws java.io.IOException("unavailable")
        val vm = model().also { it.now = { at.plusMillis(testScheduler.currentTime) } }
        try {
            vm.setForeground(true); runCurrent()
            advanceTimeBy(300_000); runCurrent()
            coVerify(exactly = 5) { useCase(any(), null) }
            advanceTimeBy(300_000); runCurrent()
            coVerify(exactly = 5) { useCase(any(), null) }
        } finally { vm.setForeground(false); runCurrent() }
    }

    @Test fun `first page and cursor jobs do not overlap`() = runTest(dispatcher) {
        coEvery { useCase(any(), null) } returns CommunityFeedPage(listOf(item()), "more", at)
        val vm = model()
        val head = kotlinx.coroutines.CompletableDeferred<CommunityFeedPage>()
        val continuation = kotlinx.coroutines.CompletableDeferred<CommunityFeedPage>()
        coEvery { useCase(any(), "more") } coAnswers { continuation.await() }
        try {
            vm.setForeground(true); runCurrent()
            coEvery { useCase(any(), null) } coAnswers { head.await() }
            vm.now = { at.plusSeconds(61) }; advanceTimeBy(30_000); runCurrent()
            vm.loadMore(); runCurrent()
            coVerify(exactly = 0) { useCase(any(), "more") }
            head.complete(CommunityFeedPage(listOf(item()), "more", at.plusSeconds(61))); runCurrent()
            vm.loadMore(); runCurrent()
            vm.now = { at.plusSeconds(122) }; advanceTimeBy(60_000); runCurrent()
            coVerify(exactly = 2) { useCase(any(), null) }
            assertTrue(vm.state.value.loadingMore)
            continuation.complete(CommunityFeedPage(emptyList(), null, at.plusSeconds(122))); runCurrent()
            assertFalse(vm.state.value.loadingMore)
        } finally { vm.setForeground(false); runCurrent() }
    }

}
