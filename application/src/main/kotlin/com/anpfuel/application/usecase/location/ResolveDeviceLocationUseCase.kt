package com.anpfuel.application.usecase.location

import com.anpfuel.domain.event.DeviceLocationResolved
import com.anpfuel.domain.model.ReverseGeocodeResult
import com.anpfuel.domain.repository.DomainEventPublisher
import com.anpfuel.domain.repository.ReverseGeocodeOutcome
import com.anpfuel.domain.repository.ReverseGeocodeRepository
import com.anpfuel.domain.valueobject.DeviceLocation

/**
 * UC-012 — Resolves municipality from device coordinates via reverse geocoding.
 */
sealed interface ResolveDeviceLocationOutcome {
    data class Success(
        val reverseGeocodeResult: ReverseGeocodeResult,
        val selectLocationResult: SelectLocationResult,
    ) : ResolveDeviceLocationOutcome

    data object MunicipalityNotInCatalog : ResolveDeviceLocationOutcome
    data object RateLimited : ResolveDeviceLocationOutcome
    data object NetworkError : ResolveDeviceLocationOutcome
    data object InvalidGeocodeResponse : ResolveDeviceLocationOutcome
}

class ResolveDeviceLocationUseCase(
    private val reverseGeocodeRepository: ReverseGeocodeRepository,
    private val selectLocationUseCase: SelectLocationUseCase,
    private val eventPublisher: DomainEventPublisher,
) {

    suspend operator fun invoke(location: DeviceLocation): ResolveDeviceLocationOutcome {
        return when (val outcome = reverseGeocodeRepository.reverseGeocode(location)) {
            is ReverseGeocodeOutcome.Success -> {
                val selectLocationResult = selectLocationUseCase.selectLocation(
                    state = outcome.result.state,
                    municipality = outcome.result.municipality,
                )
                eventPublisher.publish(
                    DeviceLocationResolved.create(
                        payload = DeviceLocationResolved.Payload(
                            state = outcome.result.state,
                            municipality = outcome.result.municipality,
                        ),
                    ),
                )
                ResolveDeviceLocationOutcome.Success(
                    reverseGeocodeResult = outcome.result,
                    selectLocationResult = selectLocationResult,
                )
            }
            ReverseGeocodeOutcome.MunicipalityNotInCatalog ->
                ResolveDeviceLocationOutcome.MunicipalityNotInCatalog
            ReverseGeocodeOutcome.RateLimited ->
                ResolveDeviceLocationOutcome.RateLimited
            ReverseGeocodeOutcome.NetworkError ->
                ResolveDeviceLocationOutcome.NetworkError
            ReverseGeocodeOutcome.InvalidResponse ->
                ResolveDeviceLocationOutcome.InvalidGeocodeResponse
        }
    }
}
