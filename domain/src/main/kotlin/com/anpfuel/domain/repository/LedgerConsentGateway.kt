package com.anpfuel.domain.repository

import com.anpfuel.domain.premium.LedgerDataset

/** Account-wide server consent mask for the private ledger after a change. */
data class LedgerConsentStatus(
    val revision: Long,
    val datasets: List<LedgerDataset>,
)

/**
 * Server consent for private ledger uploads (`POST /v1/ledger/consent`).
 *
 * The backend refuses every ledger upsert of a dataset outside the account's
 * consent mask, so participation starts with this signed write. Enabling adds
 * [datasets] to the mask and disabling removes them; other datasets keep their
 * state. Implementations sign the exact body with the device key bound to
 * [accountId] and never retry a refusal.
 */
interface LedgerConsentGateway {

    suspend fun setConsent(
        accountId: String,
        familyId: String,
        accessToken: String,
        datasets: List<LedgerDataset>,
        enabled: Boolean,
    ): LedgerConsentStatus
}
