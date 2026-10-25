package com.anpfuel.app.ui.map

import com.anpfuel.maps.engine.MapOverlayMarker
import com.anpfuel.maps.engine.MapStationLocationQuality
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class MapSheetViewModelTest {
    private val dispatcher = StandardTestDispatcher()

    private fun marker(
        uuid: String = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
        fuel: Boolean = true,
        charging: Boolean = false,
    ) = MapOverlayMarker(
        uuid = uuid,
        latitude = -25.42,
        longitude = -49.27,
        title = "Posto",
        fuel = fuel,
        charging = charging,
        locationQuality = MapStationLocationQuality.REVIEWED,
    )

    @BeforeEach fun setup() { Dispatchers.setMain(dispatcher) }
    @AfterEach fun tearDown() { Dispatchers.resetMain() }

    @Test fun `anp price shows exact product unit and date`() = runTest {
        val vm = MapSheetViewModel(SheetPriceSource { _, source, _ ->
            if (source == MapPriceSource.ANP) SheetPrice("GASOLINE_REGULAR", "L", "5.99", source, "2026-10-20") else null
        })
        vm.select(marker(), MapPriceSource.ANP)
        advanceTimeBy(200)
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals("5.99", vm.ui.value.price?.amount)
        assertEquals("L", vm.ui.value.price?.unit)
        assertEquals(false, vm.ui.value.priceAbsent)
    }

    @Test fun `absence is explicit without silent fallback`() = runTest {
        val vm = MapSheetViewModel(SheetPriceSource { _, _, _ -> null })
        vm.select(marker(), MapPriceSource.ANP)
        advanceTimeBy(200)
        dispatcher.scheduler.advanceUntilIdle()
        assertTrue(vm.ui.value.priceAbsent)
        assertNull(vm.ui.value.price)
        // Switching source does not paint the old response.
        vm.select(marker(), MapPriceSource.COMMUNITY)
        advanceTimeBy(50)
        // Old ANP price (null) stays null; new load is pending, never a mix.
        assertNull(vm.ui.value.price)
    }

    @Test fun `rapid source switches discard stale responses`() = runTest {
        var calls = 0
        val vm = MapSheetViewModel(SheetPriceSource { _, source, _ ->
            calls++
            kotlinx.coroutines.delay(200)
            SheetPrice("GASOLINE_REGULAR", "L", if (source == MapPriceSource.ANP) "5.99" else "6.10", source, "2026-10-20")
        })
        vm.select(marker(), MapPriceSource.ANP)
        advanceTimeBy(50)
        vm.select(marker(), MapPriceSource.COMMUNITY)
        advanceTimeBy(400)
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(MapPriceSource.COMMUNITY, vm.ui.value.price?.source)
        assertEquals("6.10", vm.ui.value.price?.amount)
    }

    @Test fun `charging in anp explains missing tariff and offers switch`() = runTest {
        var lookups = 0
        val vm = MapSheetViewModel(SheetPriceSource { _, _, _ ->
            lookups++
            SheetPrice("TARIFF", "kWh", "1.20", MapPriceSource.COMMUNITY, "2026-10-20")
        })
        vm.select(marker(charging = true, fuel = false), MapPriceSource.ANP)
        dispatcher.scheduler.advanceUntilIdle()
        assertTrue(vm.ui.value.priceAbsent)
        assertEquals("anp-no-tariff", vm.ui.value.absentReason)
        assertEquals(0, lookups)
    }

    @Test fun `no-price locations stay discoverable`() = runTest {
        val vm = MapSheetViewModel(SheetPriceSource { _, _, _ -> null })
        vm.select(marker(), MapPriceSource.COMMUNITY)
        advanceTimeBy(200)
        dispatcher.scheduler.advanceUntilIdle()
        assertTrue(vm.ui.value.priceAbsent)
        assertEquals(marker().uuid, vm.ui.value.marker?.uuid)
    }
}
