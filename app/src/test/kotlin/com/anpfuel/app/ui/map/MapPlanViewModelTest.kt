package com.anpfuel.app.ui.map

import com.anpfuel.application.usecase.mapsaved.GetSavedTripUseCase
import com.anpfuel.domain.map.saved.GeoPoint
import com.anpfuel.domain.map.saved.SavedTrip
import com.anpfuel.domain.map.saved.SavedTripKind
import com.anpfuel.domain.map.saved.TripPoint
import io.mockk.coEvery
import io.mockk.coVerify
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
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class MapPlanViewModelTest {
    private val dispatcher = StandardTestDispatcher()

    @BeforeEach fun setUp() { Dispatchers.setMain(dispatcher) }
    @AfterEach fun tearDown() { Dispatchers.resetMain() }

    private fun trip() = SavedTrip(
        id = "11111111-1111-4111-8111-111111111111",
        kind = SavedTripKind.ROUTE_PLAN,
        name = "Saved plan",
        points = listOf(
            TripPoint(GeoPoint.fromDouble(-25.4295, -49.2719)),
            TripPoint(GeoPoint.fromDouble(-25.4222, -49.2640)),
        ),
        avoid = setOf("TOLL"),
        createdAtMillis = 1,
    )

    @Test
    fun `load returns the owner-scoped saved trip and blank id does nothing`() = runTest {
        val useCase = mockk<GetSavedTripUseCase>()
        val saved = trip()
        coEvery { useCase("11111111-1111-4111-8111-111111111111") } returns saved
        val viewModel = MapPlanViewModel(useCase)
        viewModel.load("11111111-1111-4111-8111-111111111111")
        advanceUntilIdle()
        assertEquals(saved, viewModel.trip.value)
        viewModel.load("")
        advanceUntilIdle()
        coVerify(exactly = 1) { useCase(any()) }
        assertNull(MapPlanViewModel(useCase).trip.value)
    }
}
