package com.anpfuel.data.local.ocr

import com.anpfuel.application.port.OcrPort
import com.anpfuel.domain.portable.PortablePriceOcr
import javax.inject.Inject
import javax.inject.Singleton

/** Legacy deterministic text parser; real pixels use MlKitImagePriceOcr behind ImagePriceOcr. */
@Singleton
class LocalRegexPriceOcr @Inject constructor() : OcrPort {
    override fun candidatesFromText(ocrText: String): List<PortablePriceOcr.OcrCandidate> =
        PortablePriceOcr.parseCandidates(ocrText)
}
