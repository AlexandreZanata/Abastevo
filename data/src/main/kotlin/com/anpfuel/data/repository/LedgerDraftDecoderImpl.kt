package com.anpfuel.data.repository

import com.anpfuel.data.mapper.LedgerDraftCodec
import com.anpfuel.domain.premium.LedgerDataset
import com.anpfuel.domain.premium.LedgerDraft
import com.anpfuel.domain.repository.LedgerDraftDecoder
import javax.inject.Inject
import javax.inject.Singleton

@Singleton
class LedgerDraftDecoderImpl @Inject constructor() : LedgerDraftDecoder {

    override fun decode(dataset: LedgerDataset, bodyJson: String): LedgerDraft =
        LedgerDraftCodec.decode(dataset, bodyJson)
}
