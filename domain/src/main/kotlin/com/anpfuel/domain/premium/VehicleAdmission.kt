package com.anpfuel.domain.premium

import com.anpfuel.domain.exception.DomainException
import java.time.Duration
import java.time.Instant

/**
 * MEX-08 local vehicle admission (VEHICLE_REGISTRATION_POLICY v2).
 *
 * One new local vehicle is free, including guest/offline operation. A
 * second or third needs the `vehicles.multiple` capability via a valid
 * offline lease or fresh server proof. Corrections and replays of held
 * IDs are always free; hosted creation never accepts a local lease; the
 * technical ceiling stays three. Unknown/missing capability versions
 * cannot grant extra registrations.
 */
enum class AdmissionScope { LOCAL, HOSTED }

enum class AdmissionAuthority {
    NONE,
    EXPIRED,
    SERVER_VERIFIED,
    LOCAL_VALID_LEASE,
    CLOCK_ROLLBACK,
    PROOF_STALE,
    UNKNOWN_POLICY,
}

enum class AdmissionOutcome {
    ACCEPT,
    PREMIUM_REQUIRED,
    CAPACITY_EXCEEDED,
    HOSTED_AUTHORITY_REQUIRED,
}

/**
 * Versioned owner/origin capability lease. Verified means a server proof
 * was observed at [verifiedAt]; offline use is bounded by
 * min(verifiedAt + 24h, paidThrough), end exclusive.
 */
data class VehicleCapabilityLease(
    val version: Int,
    val accountId: String,
    val origin: String,
    val capability: String,
    val verifiedAt: Instant,
    val paidThrough: Instant,
    val revoked: Boolean,
) {
    companion object {
        const val POLICY_VERSION = 2
        const val CAPABILITY = "vehicles.multiple"
        val OFFLINE_LEASE = Duration.ofHours(24)
    }
}

class VehicleAdmissionRefused(val outcome: AdmissionOutcome) :
    DomainException("Vehicle admission refused: $outcome")

/** Atomic admission receipt: a new slot or an edit replay. */
sealed interface VehicleAdmissionResult {
    data object AdmittedAsNew : VehicleAdmissionResult
    data object Replayed : VehicleAdmissionResult
}

/**
 * Server proof for hosted/capability verification. Fresh means observed
 * within [PROOF_FRESHNESS]; anything else is stale and never authorizes.
 */
sealed interface VehicleServerProof {
    data object Stale : VehicleServerProof
    data class Fresh(val verifiedAt: Instant) : VehicleServerProof
}

object VehicleAdmissionRule {

    const val FREE_LIMIT = 1
    const val TECHNICAL_CEILING = 3
    val PROOF_FRESHNESS: Duration = Duration.ofSeconds(900)

    /**
     * Server proof authority: fresh (observed within 900 s, never from
     * the future) verifies; everything else is stale and authorizes
     * nothing.
     */
    fun resolveServerAuthority(proof: VehicleServerProof, now: Instant): AdmissionAuthority =
        when (proof) {
            is VehicleServerProof.Stale -> AdmissionAuthority.PROOF_STALE
            is VehicleServerProof.Fresh -> {
                val age = Duration.between(proof.verifiedAt, now)
                if (!age.isNegative && age <= PROOF_FRESHNESS) {
                    AdmissionAuthority.SERVER_VERIFIED
                } else {
                    AdmissionAuthority.PROOF_STALE
                }
            }
        }

    fun decide(
        scope: AdmissionScope,
        authority: AdmissionAuthority,
        registeredBefore: Int,
        newDistinctIds: Int,
    ): AdmissionOutcome {        if (newDistinctIds < 0 || registeredBefore < 0) {
            throw DomainException("Admission counts must be non-negative")
        }
        if (scope == AdmissionScope.HOSTED &&
            authority != AdmissionAuthority.SERVER_VERIFIED
        ) {
            return AdmissionOutcome.HOSTED_AUTHORITY_REQUIRED
        }
        if (newDistinctIds == 0) {
            return AdmissionOutcome.ACCEPT
        }
        val after = registeredBefore + newDistinctIds
        if (after <= FREE_LIMIT) {
            return AdmissionOutcome.ACCEPT
        }
        if (after > TECHNICAL_CEILING) {
            return AdmissionOutcome.CAPACITY_EXCEEDED
        }
        return if (authority == AdmissionAuthority.SERVER_VERIFIED ||
            authority == AdmissionAuthority.LOCAL_VALID_LEASE
        ) {
            AdmissionOutcome.ACCEPT
        } else {
            AdmissionOutcome.PREMIUM_REQUIRED
        }
    }

    /**
     * Resolves a stored lease against the caller's owner/origin and clock.
     * Wrong-owner/origin leases are ignored (never merged); revoked,
     * rolled-back, elapsed and malformed grants refuse as expired or
     * unknown-policy. Nothing here performs I/O or grants silently.
     */
    fun resolveLocalAuthority(
        lease: VehicleCapabilityLease?,
        accountId: String,
        origin: String,
        now: Instant,
    ): AdmissionAuthority {
        if (lease == null) return AdmissionAuthority.NONE
        if (lease.version != VehicleCapabilityLease.POLICY_VERSION ||
            lease.capability != VehicleCapabilityLease.CAPABILITY
        ) {
            return AdmissionAuthority.UNKNOWN_POLICY
        }
        if (lease.accountId != accountId || lease.origin != origin) {
            return AdmissionAuthority.NONE
        }
        if (lease.revoked) return AdmissionAuthority.EXPIRED
        if (now.isBefore(lease.verifiedAt)) return AdmissionAuthority.CLOCK_ROLLBACK
        if (!lease.paidThrough.isAfter(lease.verifiedAt)) {
            return AdmissionAuthority.UNKNOWN_POLICY
        }
        val end = minOf(
            lease.verifiedAt.plus(VehicleCapabilityLease.OFFLINE_LEASE),
            lease.paidThrough,
        )
        if (!now.isBefore(end)) return AdmissionAuthority.EXPIRED
        return AdmissionAuthority.LOCAL_VALID_LEASE
    }
}
