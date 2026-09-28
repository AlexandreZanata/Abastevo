package com.anpfuel.application.usecase.station

import com.anpfuel.domain.event.StationNavigationRequested
import com.anpfuel.domain.model.RetailStation
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.repository.DomainEventPublisher
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.Cnpj
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import io.mockk.slot
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

class BuildStationNavigationQueryUseCaseTest {

    private val userPreferencesRepository = mockk<UserPreferencesRepository>()
    private val eventPublisher = mockk<DomainEventPublisher>()

    private lateinit var useCase: BuildStationNavigationQueryUseCase

    private val station = RetailStation.create(
        cnpj = Cnpj.parse("61602199002409"),
        legalName = "COMPANHIA ULTRAGAZ S A",
        tradeName = "ULTRAGAZ",
        address = "RUA AMARO CASTRO LIMA",
        municipality = "CAMPO GRANDE",
        state = BrazilianState.MATO_GROSSO_DO_SUL,
        brand = "ULTRAGAZ",
    )

    @BeforeEach
    fun setUp() {
        useCase = BuildStationNavigationQueryUseCase(
            userPreferencesRepository = userPreferencesRepository,
            eventPublisher = eventPublisher,
        )
        coEvery { eventPublisher.publish(any()) } returns Unit
    }

    @Test
    fun buildQueryUsesPreferredLocationAsFallbackMetadata() = runTest {
        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences(
            preferredState = BrazilianState.PARANA,
            preferredMunicipality = "CURITIBA",
        )

        val query = useCase.buildQuery(station)

        assertTrue(query.contains("CAMPO GRANDE - MS"))
        assertEquals(
            "ULTRAGAZ, RUA AMARO CASTRO LIMA, CAMPO GRANDE - MS, Brazil",
            query,
        )
    }

    @Test
    fun invokePublishesStationNavigationRequestedAndReturnsQuery() = runTest {
        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences()
        val eventSlot = slot<StationNavigationRequested>()

        val result = useCase.invoke(station)

        assertEquals(
            "ULTRAGAZ, RUA AMARO CASTRO LIMA, CAMPO GRANDE - MS, Brazil",
            result.navigationQuery,
        )
        coVerify(exactly = 1) { eventPublisher.publish(capture(eventSlot)) }
        assertEquals(station.cnpj, eventSlot.captured.payload.cnpj)
        assertEquals(result.navigationQuery, eventSlot.captured.payload.navigationQuery)
    }
}
