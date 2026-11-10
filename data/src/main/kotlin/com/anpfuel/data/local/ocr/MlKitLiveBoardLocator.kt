package com.anpfuel.data.local.ocr

import android.media.Image
import com.anpfuel.domain.portable.FuelBoardFraming
import com.anpfuel.domain.portable.LiveBoardZoom
import com.google.mlkit.vision.common.InputImage
import com.google.mlkit.vision.text.TextRecognition
import com.google.mlkit.vision.text.TextRecognizer
import com.google.mlkit.vision.text.latin.TextRecognizerOptions

/**
 * Finds the price board in camera preview frames (B-BR-PC02 live framing) with the same
 * on-device recognizer and [FuelBoardFraming] rule as the photo reading. A frame is read in
 * memory only; no frame, token or result is stored, logged or sent. Without an installed
 * model every frame simply shows no board.
 */
object MlKitLiveBoardLocator {
    fun open(): Session = Session(TextRecognition.getClient(TextRecognizerOptions.DEFAULT_OPTIONS))

    /** One recognizer for one camera session; close it with the camera. */
    class Session internal constructor(private val recognizer: TextRecognizer) : AutoCloseable {
        @Volatile private var closed = false

        /**
         * Reads [image] (rotated by [rotationDegrees] to upright) and reports the board as
         * fractions of the upright frame, or null. [onResult] runs exactly once; the caller
         * keeps [image] open until then.
         */
        fun locate(image: Image, rotationDegrees: Int, onResult: (LiveBoardZoom.Board?) -> Unit) {
            if (closed) { onResult(null); return }
            val quarter = rotationDegrees % 180 != 0
            val width = if (quarter) image.height else image.width
            val height = if (quarter) image.width else image.height
            try {
                recognizer.process(InputImage.fromMediaImage(image, rotationDegrees)).addOnCompleteListener { task ->
                    val frame = if (task.isSuccessful && !closed) FuelBoardFraming.locate(boardTokens(task.result), width, height) else null
                    onResult(frame?.let {
                        LiveBoardZoom.Board(
                            it.left.toDouble() / width,
                            it.top.toDouble() / height,
                            it.right.toDouble() / width,
                            it.bottom.toDouble() / height,
                        )
                    })
                }
            } catch (_: Exception) {
                onResult(null)
            }
        }

        override fun close() {
            closed = true
            recognizer.close()
        }
    }
}
