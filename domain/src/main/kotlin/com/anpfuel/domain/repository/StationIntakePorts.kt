package com.anpfuel.domain.repository

/**
 * P27-T04 private intake transport port.
 *
 * Network-only: transport failures and non-2xx/empty bodies throw;
 * idempotency replay converges server-side. The data adapter owns
 * HTTPS/session-body routing and payload parsing; callers own retry
 * and mapping.
 */
data class IntakeReceipt(val id: String)

data class IntakeSummary(val id: String, val state: String)

data class IntakeStatus(val id: String, val state: String)

interface StationIntakeGateway {
    fun submit(
        familyId: String,
        accessToken: String,
        clientSubmissionId: String,
        proposalJson: String,
    ): IntakeReceipt

    fun mine(familyId: String, accessToken: String): List<IntakeSummary>

    fun status(
        familyId: String,
        accessToken: String,
        id: String,
    ): IntakeStatus

    fun cancel(
        familyId: String,
        accessToken: String,
        id: String,
    )
}
