package com.anpfuel.domain.repository

import com.anpfuel.domain.premium.LedgerDataset
import com.anpfuel.domain.premium.LedgerDraft
import com.anpfuel.domain.premium.PremiumValidationException

/**
 * Strict stored-body decoder (throws [PremiumValidationException] on
 * corrupt or drifted bodies; never invents values). Implemented in
 * data over the canonical codec; callers fail the item, never the batch.
 */
interface LedgerDraftDecoder {

    @Throws(PremiumValidationException::class)
    fun decode(dataset: LedgerDataset, bodyJson: String): LedgerDraft
}
