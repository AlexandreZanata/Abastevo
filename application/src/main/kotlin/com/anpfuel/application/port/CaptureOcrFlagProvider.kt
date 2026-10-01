package com.anpfuel.application.port

/**
 * P10-T04 capture/OCR flag provider (data owns storage).
 */
interface CaptureOcrFlagProvider {
    fun isEnabled(): Boolean
}
