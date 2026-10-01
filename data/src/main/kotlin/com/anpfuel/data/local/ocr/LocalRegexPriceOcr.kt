package com.anpfuel.data.local.ocr

import com.anpfuel.application.port.OcrPort
import com.anpfuel.domain.portable.PortablePriceOcr
import javax.inject.Inject
import javax.inject.Singleton

/**
 * P10-T04 default on-device OCR adapter.
 *
 * Deterministic text parser over [PortablePriceOcr]: it turns platform
 * OCR text (ML Kit on real devices, fakes in JVM tests) into ordered
 * price candidates. No network, no storage and no product/condition
 * inference. The ML Kit `TextRecognition` engine plugs in behind the same
 * [OcrPort] by converting `InputImage` text to [candidatesFromText];
 * bundling that engine stays deferred (see `CaptureModule` rationale) so
 * this adapter keeps the release APK inside its 15 MB budget and stays
 * fully unit-testable on the JVM.
 */
@Singleton
class LocalRegexPriceOcr @Inject constructor() : OcrPort {
    override fun candidatesFromText(ocrText: String): List<PortablePriceOcr.OcrCandidate> =
        PortablePriceOcr.parseCandidates(ocrText)
}
