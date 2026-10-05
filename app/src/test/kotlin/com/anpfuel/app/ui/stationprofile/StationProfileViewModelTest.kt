package com.anpfuel.app.ui.stationprofile

import androidx.lifecycle.SavedStateHandle
import com.anpfuel.application.usecase.profile.GetStationProfileUseCase
import com.anpfuel.application.usecase.profile.StationProfileOutcome
import com.anpfuel.domain.profile.StationProfile
import io.mockk.coEvery
import io.mockk.mockk
import java.io.IOException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.*
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class StationProfileViewModelTest {
    private val dispatcher = StandardTestDispatcher()
    private val read = mockk<GetStationProfileUseCase>()
    @BeforeEach fun before() { Dispatchers.setMain(dispatcher) }
    @AfterEach fun after() { Dispatchers.resetMain() }

    @Test fun `failed refresh preserves public data but removes current authority`() = runTest(dispatcher) {
        coEvery { read("station") } returns StationProfileOutcome.Found(StationProfile("station", "Posto", hasBadge = true))
        val vm = StationProfileViewModel(SavedStateHandle(mapOf("stationId" to "station")), read)
        advanceUntilIdle()
        assertFalse(vm.state.value.stale)
        coEvery { read("station") } throws IOException()
        vm.refresh(); advanceUntilIdle()
        assertEquals("Posto", vm.state.value.profile?.displayName)
        assertTrue(vm.state.value.stale)
        coEvery { read("station") } returns StationProfileOutcome.Unclaimed
        vm.refresh(); advanceUntilIdle()
        assertNull(vm.state.value.profile)
        assertTrue(vm.state.value.unavailable)
    }
}
