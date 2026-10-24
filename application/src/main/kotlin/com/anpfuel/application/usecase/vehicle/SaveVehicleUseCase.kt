package com.anpfuel.application.usecase.vehicle

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.event.DomainEvent
import com.anpfuel.domain.event.PriceDropAlertConfigured
import com.anpfuel.domain.event.VehicleRegistered
import com.anpfuel.domain.event.VehicleRemoved
import com.anpfuel.domain.event.VehicleUpdated
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.premium.AdmissionAuthority
import com.anpfuel.domain.premium.AdmissionOutcome
import com.anpfuel.domain.premium.AdmissionScope
import com.anpfuel.domain.premium.VehicleAdmissionRefused
import com.anpfuel.domain.premium.VehicleAdmissionRule
import com.anpfuel.domain.premium.VehicleServerProof
import com.anpfuel.domain.repository.DomainEventPublisher
import com.anpfuel.domain.repository.VehicleAdmissionRepository
import com.anpfuel.domain.repository.VehicleRepository
import java.time.Instant

data class SaveVehicleResult(
    val vehicle: Vehicle,
    val events: List<DomainEvent>,
)

/**
 * MEX-08 admission-gated vehicle save. The first local vehicle is free;
 * extra creation needs a valid capability lease or fresh server proof,
 * decided by [VehicleAdmissionRule] and enforced atomically by the
 * repository (the recount lives inside the write transaction). Edits of
 * held IDs replay without consuming a slot; legacy over-ceiling rows
 * stay readable and editable. Hosted scope is never used here: a local
 * offline lease cannot authorize a server write.
 */
class SaveVehicleUseCase(
    private val vehicleRepository: VehicleRepository,
    private val eventPublisher: DomainEventPublisher,
    private val admissionRepository: VehicleAdmissionRepository,
    private val sessions: AuthSessionStore,
    private val origin: String,
    private val serverProof: VehicleServerProof = VehicleServerProof.Stale,
    private val now: () -> Instant = Instant::now,
) {

    suspend operator fun invoke(vehicle: Vehicle): SaveVehicleResult {
        val existing = vehicleRepository.findById(vehicle.id)
        val accountId = sessions.load()?.accountId ?: LOCAL_OWNER
        val authority = resolveAuthority(accountId)
        val outcome = VehicleAdmissionRule.decide(
            AdmissionScope.LOCAL,
            authority,
            admissionRepository.countVehicles(),
            if (existing == null) 1 else 0,
        )
        if (outcome != AdmissionOutcome.ACCEPT) {
            throw VehicleAdmissionRefused(outcome)
        }
        admissionRepository.admit(
            vehicle,
            hasPaidCapability = authority == AdmissionAuthority.SERVER_VERIFIED ||
                authority == AdmissionAuthority.LOCAL_VALID_LEASE,
        )

        val events = buildList {
            if (existing == null) {
                add(
                    VehicleRegistered.create(
                        payload = VehicleRegistered.Payload(
                            vehicleId = vehicle.id,
                            displayName = vehicle.displayName,
                            fuelProduct = vehicle.fuelProduct,
                        ),
                    ),
                )
            } else {
                add(
                    VehicleUpdated.create(
                        payload = VehicleUpdated.Payload(
                            vehicleId = vehicle.id,
                            displayName = vehicle.displayName,
                            fuelProduct = vehicle.fuelProduct,
                        ),
                    ),
                )
            }

            if (shouldPublishPriceDropAlertConfigured(existing, vehicle)) {
                add(
                    PriceDropAlertConfigured.create(
                        payload = PriceDropAlertConfigured.Payload(
                            vehicleId = vehicle.id,
                            enabled = vehicle.priceDropAlertEnabled,
                            alertPriceSource = vehicle.priceSource,
                        ),
                    ),
                )
            }
        }

        events.forEach { event -> eventPublisher.publish(event) }

        return SaveVehicleResult(
            vehicle = vehicle,
            events = events,
        )
    }

    internal suspend fun resolveAuthority(accountId: String): AdmissionAuthority {
        val server = VehicleAdmissionRule.resolveServerAuthority(serverProof, now())
        if (server == AdmissionAuthority.SERVER_VERIFIED) return server
        val lease = admissionRepository.loadLease(accountId, origin)
        return VehicleAdmissionRule.resolveLocalAuthority(lease, accountId, origin, now())
    }

    internal fun shouldPublishPriceDropAlertConfigured(
        existing: Vehicle?,
        updated: Vehicle,
    ): Boolean {
        if (existing == null) {
            return updated.priceDropAlertEnabled
        }
        return existing.priceDropAlertEnabled != updated.priceDropAlertEnabled ||
            existing.priceSource != updated.priceSource
    }

    companion object {
        const val LOCAL_OWNER = "local"
    }
}
