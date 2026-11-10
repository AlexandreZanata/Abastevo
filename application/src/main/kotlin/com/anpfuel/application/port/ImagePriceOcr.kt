package com.anpfuel.application.port

import com.anpfuel.domain.portable.FuelBoardOcr

/**
 * Pixels are processed locally. OCR text and token coordinates never cross
 * this port; the only geometry that does is the coarse board rectangle used
 * to centre the review photo.
 */
interface ImagePriceOcr {
    /** Board rectangle as fractions (0..1) of the upright photo's width and height. */
    data class BoardFrame(val left: Float, val top: Float, val right: Float, val bottom: Float) {
        init {
            require(left in 0f..1f && top in 0f..1f && right in 0f..1f && bottom in 0f..1f && left < right && top < bottom) { "ocr.frame" }
        }
    }

    data class Recognition(val result: FuelBoardOcr.Result, val frame: BoardFrame?)

    suspend fun recognize(bytes: ByteArray): FuelBoardOcr.Result

    /** Same suggestions as [recognize], plus where the board was found (null when it was not). */
    suspend fun recognizeFramed(bytes: ByteArray): Recognition = Recognition(recognize(bytes), null)
}
