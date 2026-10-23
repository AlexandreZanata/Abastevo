package com.anpfuel.domain.repository

import com.anpfuel.domain.premium.PremiumCommand
import com.anpfuel.domain.premium.PremiumOwnerScope

/** Durable server acknowledgement for one ledger command. */
data class LedgerSyncReceipt(
    val operationId: String,
    val revision: Long,
    val replayed: Boolean,
)

class LedgerSyncConflict(message: String) : Exception(message)
class LedgerSyncDenied(message: String) : Exception(message)

/**
 * MEX-14 private sync transport port. Implementations POST one command
 * with live owner proof; they never invent revisions or replay locally.
 */
interface LedgerSyncGateway {

    suspend fun submit(
        scope: PremiumOwnerScope,
        command: PremiumCommand,
        attemptNonce: String,
        familyId: String,
        accessToken: String,
    ): LedgerSyncReceipt
}
