package com.anpfuel.application.usecase.profile

import com.anpfuel.domain.profile.StationProfile
import com.anpfuel.domain.repository.StationProfileGateway
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * P32-T01 — Profile read use-case RED→GREEN.
 */
class GetStationProfileUseCaseTest {

    private class FakeGateway(val profile: StationProfile?) : StationProfileGateway {
        var calls = 0
        override suspend fun getProfile(stationId: String): StationProfile? {
            calls++
            return profile
        }
    }

    @Test
    fun `blank id is invalid without touching gateway`() = runTest {
        val gateway = FakeGateway(null)
        val outcome = GetStationProfileUseCase(gateway).invoke("  ")
        assertEquals(StationProfileOutcome.Invalid, outcome)
        assertEquals(0, gateway.calls)
    }

    @Test
    fun `unknown profile maps to unclaimed`() = runTest {
        val outcome = GetStationProfileUseCase(FakeGateway(null)).invoke("9f6d8a2e-3b4c-4d5e-8f90-1234567890ab")
        assertEquals(StationProfileOutcome.Unclaimed, outcome)
    }

    @Test
    fun `known profile returns public projection`() = runTest {
        val profile = StationProfile(
            stationId = "9f6d8a2e-3b4c-4d5e-8f90-1234567890ab",
            displayName = "Posto Exemplo",
            business = mapOf("phone" to "+55 41 3333-3333"),
        )
        val outcome = GetStationProfileUseCase(FakeGateway(profile)).invoke(profile.stationId)
        assertTrue(outcome is StationProfileOutcome.Found)
        assertEquals(profile, (outcome as StationProfileOutcome.Found).profile)
    }
}
