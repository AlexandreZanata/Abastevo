package com.anpfuel.application.port

import com.anpfuel.domain.portable.FuelBoardOcr

/** Pixels are processed locally; neither OCR text nor coordinates cross this port. */
interface ImagePriceOcr {
    suspend fun recognize(bytes: ByteArray): FuelBoardOcr.Result
}
