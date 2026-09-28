package com.anpfuel.application.usecase.location

import com.anpfuel.domain.event.CitySelected
import com.anpfuel.domain.event.DeviceLocationResolved
import com.anpfuel.domain.model.ReverseGeocodeResult
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.repository.DomainEventPublisher
import com.anpfuel.domain.repository.ReverseGeocodeOutcome
import com.anpfuel.domain.repository.ReverseGeocodeRepository
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DeviceLocation
import com.anpfuel.domain.valueobject.DomainId
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import io.mockk.slot
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

class ResolveDeviceLocationUseCaseTest {

    private val reverseGeocodeRepository = mockk<ReverseGeocodeRepository>()
    private val selectLocationUseCase = mockk<SelectLocationUseCase>()
    private val eventPublisher = mockk<DomainEventPublisher>()

    private lateinit var useCase: ResolveDeviceLocationUseCase

    private val location = DeviceLocation.of(-25.4284, -49.2733)

    @BeforeEach
    fun setUp() {
        useCase = ResolveDeviceLocationUseCase(
            reverseGeocodeRepository = reverseGeocodeRepository,
            selectLocationUseCase = selectLocationUseCase,
            eventPublisher = eventPublisher,
        )
        coEvery { eventPublisher.publish(any()) } returns Unit
    }

    @Test
    fun persistsLocationWhenReverseGeocodeMatchesCatalog() = runTest {
        val reverseGeocodeResult = ReverseGeocodeResult(
            state = BrazilianState.PARANA,
            municipality = "CURITIBA",
        )
        coEvery { reverseGeocodeRepository.reverseGeocode(location) } returns
            ReverseGeocodeOutcome.Success(reverseGeocodeResult)
        coEvery {
            selectLocationUseCase.selectLocation(BrazilianState.PARANA, "CURITIBA")
        } returns SelectLocationResult(
            event = CitySelected.create(
                payload = CitySelected.Payload(
                    municipality = "CURITIBA",
                    state = BrazilianState.PARANA,
                    surveyWeekId = DomainId.generate(),
                ),
            ),
            preferences = UserPreferences(
                preferredState = BrazilianState.PARANA,
                preferredMunicipality = "CURITIBA",
            ),
        )

        val outcome = useCase.invoke(location)

        assertTrue(outcome is ResolveDeviceLocationOutcome.Success)
        val eventSlot = slot<DeviceLocationResolved>()
        coVerify(exactly = 1) { eventPublisher.publish(capture(eventSlot)) }
        assertEquals(BrazilianState.PARANA, eventSlot.captured.payload.state)
        assertEquals("CURITIBA", eventSlot.captured.payload.municipality)
    }

    @Test
    fun returnsMunicipalityNotInCatalogWithoutPersistingLocation() = runTest {
        coEvery { reverseGeocodeRepository.reverseGeocode(location) } returns
            ReverseGeocodeOutcome.MunicipalityNotInCatalog

        val outcome = useCase.invoke(location)

        assertEquals(ResolveDeviceLocationOutcome.MunicipalityNotInCatalog, outcome)
        coVerify(exactly = 0) { selectLocationUseCase.selectLocation(any(), any()) }
        coVerify(exactly = 0) { eventPublisher.publish(any()) }
    }

    @Test
    fun mapsRateLimitedOutcome() = runTest {
        coEvery { reverseGeocodeRepository.reverseGeocode(location) } returns
            ReverseGeocodeOutcome.RateLimited

        val outcome = useCase.invoke(location)

        assertEquals(ResolveDeviceLocationOutcome.RateLimited, outcome)
    }
}
