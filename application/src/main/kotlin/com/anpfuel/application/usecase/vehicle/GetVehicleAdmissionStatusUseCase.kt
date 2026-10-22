package com.anpfuel.application.usecase.vehicle

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.premium.AdmissionAuthority
import com.anpfuel.domain.premium.AdmissionOutcome
import com.anpfuel.domain.premium.AdmissionScope
import com.anpfuel.domain.premium.VehicleAdmissionRule
import com.anpfuel.domain.premium.VehicleServerProof
import com.anpfuel.domain.repository.VehicleAdmissionRepository
import java.time.Instant

data class VehicleAdmissionStatus(
    val registered: Int,
    val outcomeForNew: AdmissionOutcome,
)

/**
 * MEX-08 admission pre-check for honest UI gating: reports what a new
 * registration would decide without writing anything. Enforcement still
 * happens atomically in [SaveVehicleUseCase].
 */
class GetVehicleAdmissionStatusUseCase(
    private val admissionRepository: VehicleAdmissionRepository,
    private val sessions: AuthSessionStore,
    private val origin: String,
    private val serverProof: VehicleServerProof = VehicleServerProof.Stale,
    private val now: () -> Instant = Instant::now,
) {

    suspend operator fun invoke(): VehicleAdmissionStatus {
        val accountId = sessions.load()?.accountId ?: SaveVehicleUseCase.LOCAL_OWNER
        val server = VehicleAdmissionRule.resolveServerAuthority(serverProof, now())
        val authority = if (server == AdmissionAuthority.SERVER_VERIFIED) {
            server
        } else {
            VehicleAdmissionRule.resolveLocalAuthority(
                admissionRepository.loadLease(accountId, origin),
                accountId,
                origin,
                now(),
            )
        }
        val count = admissionRepository.countVehicles()
        return VehicleAdmissionStatus(
            registered = count,
            outcomeForNew = VehicleAdmissionRule.decide(AdmissionScope.LOCAL, authority, count, 1),
        )
    }
}
