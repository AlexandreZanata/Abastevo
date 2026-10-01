package com.anpfuel.application.port

import com.anpfuel.domain.portable.PortablePriceOcr

/**
 * P10-T04 on-device OCR boundary.
 *
 * The data layer turns platform OCR text into [PortablePriceOcr]
 * candidates; the application flow only orchestrates permission, cancel
 * and human confirmation. No network, no upload and no product/condition
 * inference cross this port. ML Kit (or any engine) plugs in behind this
 * same interface; the default JVM-testable adapter only parses text.
 */
interface OcrPort {
    /** Parses OCR text into ordered price candidates (never sorted). */
    fun candidatesFromText(ocrText: String): List<PortablePriceOcr.OcrCandidate>
}
