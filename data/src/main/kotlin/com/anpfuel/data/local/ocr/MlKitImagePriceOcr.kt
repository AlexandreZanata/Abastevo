package com.anpfuel.data.local.ocr

import android.content.Context
import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.ColorMatrix
import android.graphics.ColorMatrixColorFilter
import android.graphics.Paint
import android.graphics.Rect
import kotlin.math.abs
import kotlin.math.roundToInt
import com.anpfuel.data.local.media.BoundedPhotoBitmap
import dagger.hilt.android.qualifiers.ApplicationContext
import com.google.android.gms.common.moduleinstall.ModuleInstall
import com.google.android.gms.common.moduleinstall.ModuleInstallRequest
import com.google.android.gms.common.moduleinstall.InstallStatusListener
import com.google.android.gms.common.moduleinstall.ModuleInstallStatusUpdate
import com.google.android.gms.tasks.Task
import kotlinx.coroutines.withTimeout
import com.anpfuel.application.port.ImagePriceOcr
import com.anpfuel.domain.portable.FuelBoardOcr
import com.google.mlkit.vision.common.InputImage
import com.google.mlkit.vision.text.TextRecognition
import com.google.mlkit.vision.text.latin.TextRecognizerOptions
import javax.inject.Inject
import javax.inject.Singleton
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException

/** Bounded on-device Latin recognition. Photos/output never leave the device through this adapter. */
@Singleton
class MlKitImagePriceOcr @Inject constructor(@ApplicationContext private val context: Context) : ImagePriceOcr {
    override suspend fun recognize(bytes: ByteArray): FuelBoardOcr.Result =
        FuelBoardOcr.reconcile(recognizeViews(bytes))

    /** Private instrumentation provenance; no views are logged or uploaded. */
    internal suspend fun recognizeViews(bytes: ByteArray): List<FuelBoardOcr.Result> = withContext(Dispatchers.Default) {
        val original = recognizeTokens(bytes)
        val views = mutableListOf(FuelBoardOcr.associate(original))
        val upright = BoundedPhotoBitmap.decode(bytes)
        try {
            val region = boardRegion(original, upright.width, upright.height)
            if (region != null) {
                for (grayscale in listOf(false, true)) {
                    val view = enlargedBoard(upright, region, grayscale)
                    // Cancellation belongs to the task's bitmap lifetime, not fallback.
                    views += FuelBoardOcr.associate(recognizeBitmap(view))
                }
            }
        } finally { upright.recycle() }
        views
    }

    private fun boardRegion(tokens: List<FuelBoardOcr.Token>, width: Int, height: Int): Rect? {
        val labels = tokens.filter { labelPattern.containsMatchIn(it.text.uppercase()) }
        if (labels.isEmpty()) return null
        val prices = tokens.filter { wholePrice.matches(it.text.trim()) }.filter { value ->
            labels.any { label -> abs(value.cy - label.cy) <= maxOf(label.height, value.height) * 3 &&
                abs(value.cx - label.cx) <= width * 0.65 }
        }
        if (prices.isEmpty()) return null
        val board = labels + prices
        val margin = maxOf(24, board.maxOf { it.height })
        val region = Rect((board.minOf { it.left } - margin).coerceAtLeast(0),
            (board.minOf { it.top } - margin).coerceAtLeast(0),
            (board.maxOf { it.right } + margin).coerceAtMost(width),
            (board.maxOf { it.bottom } + margin).coerceAtMost(height))
        return region.takeIf { it.width() > 0 && it.height() > 0 &&
            it.width().toLong() * it.height() < width.toLong() * height * 0.9 }
    }

    private fun enlargedBoard(source: Bitmap, region: Rect, grayscale: Boolean): Bitmap {
        val scale = minOf(if (grayscale) 3.0 else 2.0, 2048.0 / maxOf(region.width(), region.height()))
        val output = Bitmap.createBitmap((region.width() * scale).roundToInt().coerceAtLeast(1),
            (region.height() * scale).roundToInt().coerceAtLeast(1), Bitmap.Config.ARGB_8888)
        try {
            val paint = Paint(Paint.ANTI_ALIAS_FLAG or Paint.FILTER_BITMAP_FLAG)
            if (grayscale) paint.colorFilter = ColorMatrixColorFilter(ColorMatrix().apply { setSaturation(0f) })
            Canvas(output).drawBitmap(source, region, Rect(0, 0, output.width, output.height), paint)
            return output
        } catch (error: Exception) { output.recycle(); throw error }
    }

    private val labelPattern = Regex("ETANOL|ALCOOL|GASOLINA|DIESEL|GRID|ADITIV|\\bS[ -]?(10|500)\\b|\\bGNV\\b")
    private val wholePrice = Regex("[0-9]{1,2}[., ][0-9]{2,3}")

    // Transient geometry only; used by private device evaluation, never logs or uploads.
    suspend fun recognizeTokens(bytes: ByteArray): List<FuelBoardOcr.Token> = withContext(Dispatchers.Default) {
        require(bytes.isNotEmpty() && bytes.size <= 32 * 1024 * 1024) { "ocr.input-size" }
        recognizeBitmap(BoundedPhotoBitmap.decode(bytes))
    }

    /** Owns its bitmap until ML Kit completes, including cancellation. */
    private suspend fun recognizeBitmap(bitmap: Bitmap): List<FuelBoardOcr.Token> {
        val recognizer = TextRecognition.getClient(TextRecognizerOptions.DEFAULT_OPTIONS)
        try {
            val modules=ModuleInstall.getClient(context)
            if (!withTimeout(5000) { modules.areModulesAvailable(recognizer).await().areModulesAvailable() }) {
                // Request the model only; image bytes are never part of this request.
                modules.installModules(ModuleInstallRequest.newBuilder().addApi(recognizer).build())
                error("ocr.model-pending")
            }
        } catch (error: Exception) {
            bitmap.recycle();recognizer.close()
            throw IllegalStateException(if(error.message=="ocr.model-pending") "ocr.model-pending" else "ocr.unavailable")
        }
        return suspendCancellableCoroutine { continuation ->
            try {
                recognizer.process(InputImage.fromBitmap(bitmap, 0)).addOnCompleteListener { task ->
                    // Task processing owns the bitmap until completion, even after cancellation.
                    try {
                        if (continuation.isActive) {
                            if (!task.isSuccessful) {
                                continuation.resumeWithException(IllegalStateException("ocr.unavailable"))
                            } else {
                                val lines = task.result.textBlocks.flatMap { it.lines }.take(256)
                                val tokens = lines.flatMap { line ->
                                    val entries = listOf(line.text to line.boundingBox) + line.elements.map { it.text to it.boundingBox }
                                    entries.mapNotNull { (text, rect) -> rect?.let { FuelBoardOcr.Token(text.take(256), it.left, it.top, it.right, it.bottom) } }
                                }.distinct().take(512)
                                continuation.resume(tokens)
                            }
                        }
                    } finally {
                        bitmap.recycle()
                        recognizer.close()
                    }
                }
            } catch (_: Exception) {
                bitmap.recycle()
                recognizer.close()
                if (continuation.isActive) continuation.resumeWithException(IllegalStateException("ocr.unavailable"))
            }
        }
    }

    /** Device harness readiness: production capture offers retry/manual while the model downloads. */
    suspend fun prepareModel(): Boolean {
        val recognizer=TextRecognition.getClient(TextRecognizerOptions.DEFAULT_OPTIONS)
        val modules=ModuleInstall.getClient(context)
        var listener: InstallStatusListener?=null
        try {
            if (withTimeout(5000) { modules.areModulesAvailable(recognizer).await().areModulesAvailable() }) return true
            return withTimeout(45000) {
                suspendCancellableCoroutine { continuation ->
                    val progress=InstallStatusListener { update ->
                        if(continuation.isActive) when(update.installState) {
                            ModuleInstallStatusUpdate.InstallState.STATE_COMPLETED -> continuation.resume(true)
                            ModuleInstallStatusUpdate.InstallState.STATE_FAILED,
                            ModuleInstallStatusUpdate.InstallState.STATE_CANCELED -> continuation.resume(false)
                        }
                    }
                    listener=progress
                    modules.installModules(ModuleInstallRequest.newBuilder().addApi(recognizer).setListener(progress).build())
                        .addOnCompleteListener { task ->
                            if(continuation.isActive) {
                                if(!task.isSuccessful) continuation.resume(false)
                                else if(task.result.areModulesAlreadyInstalled()) continuation.resume(true)
                            }
                        }
                }
            }
        } finally { listener?.let { modules.unregisterListener(it) };recognizer.close() }
    }

    private suspend fun <T> Task<T>.await(): T = suspendCancellableCoroutine { continuation ->
        addOnCompleteListener { task ->
            if(continuation.isActive) {
                if(task.isSuccessful) continuation.resume(task.result)
                else continuation.resumeWithException(IllegalStateException("ocr.unavailable"))
            }
        }
    }

}
