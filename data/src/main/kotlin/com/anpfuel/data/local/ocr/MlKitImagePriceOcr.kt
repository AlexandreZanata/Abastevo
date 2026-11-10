package com.anpfuel.data.local.ocr

import android.content.Context
import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.ColorMatrix
import android.graphics.ColorMatrixColorFilter
import android.graphics.Matrix
import android.graphics.Paint
import com.anpfuel.application.port.ImagePriceOcr
import com.anpfuel.data.local.media.BoundedPhotoBitmap
import com.anpfuel.domain.portable.FuelBoardFraming
import com.anpfuel.domain.portable.FuelBoardOcr
import com.google.android.gms.common.moduleinstall.InstallStatusListener
import com.google.android.gms.common.moduleinstall.ModuleInstall
import com.google.android.gms.common.moduleinstall.ModuleInstallRequest
import com.google.android.gms.common.moduleinstall.ModuleInstallStatusUpdate
import com.google.android.gms.tasks.Task
import com.google.mlkit.vision.common.InputImage
import com.google.mlkit.vision.text.TextRecognition
import com.google.mlkit.vision.text.TextRecognizer
import com.google.mlkit.vision.text.latin.TextRecognizerOptions
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import javax.inject.Singleton
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException
import kotlin.math.roundToInt
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeout

/**
 * Bounded on-device Latin recognition. Photos/output never leave the device through this adapter.
 *
 * Up to four readings of one photo, stopping as soon as two agree on every row:
 * 1. the whole photo, which also locates the board ([FuelBoardFraming]);
 * 2. the board alone, straightened and enlarged to a readable digit height,
 *    from the camera's own pixels when the bounded photo was downsampled;
 * 3. the whole photo in contrast-stretched grayscale (dark, hazy or glaring scenes);
 * 4. the board alone at a smaller digit height, which reads large digits and
 *    row icons differently from reading 2.
 * The readings are reconciled row by row, never merged digit by digit.
 */
@Singleton
class MlKitImagePriceOcr @Inject constructor(@ApplicationContext private val context: Context) : ImagePriceOcr {
    override suspend fun recognize(bytes: ByteArray): FuelBoardOcr.Result =
        FuelBoardOcr.reconcile(analyze(bytes).views)

    override suspend fun recognizeFramed(bytes: ByteArray): ImagePriceOcr.Recognition {
        val analysis = analyze(bytes)
        val frame = analysis.frame?.let {
            ImagePriceOcr.BoardFrame(
                left = it.left.toFloat() / analysis.width,
                top = it.top.toFloat() / analysis.height,
                right = it.right.toFloat() / analysis.width,
                bottom = it.bottom.toFloat() / analysis.height,
            )
        }
        return ImagePriceOcr.Recognition(FuelBoardOcr.reconcile(analysis.views), frame)
    }

    /** Per-reading provenance for private device evaluation; never logged or uploaded. */
    internal class Analysis(
        val views: List<FuelBoardOcr.Result>,
        val tokens: List<List<FuelBoardOcr.Token>>,
        val frame: FuelBoardFraming.Frame?,
        val width: Int,
        val height: Int,
    )

    /** Private instrumentation provenance; no views are logged or uploaded. */
    internal suspend fun recognizeViews(bytes: ByteArray): List<FuelBoardOcr.Result> = analyze(bytes).views

    internal suspend fun analyze(bytes: ByteArray, cameraDetail: Boolean = true): Analysis = withContext(Dispatchers.Default) {
        require(bytes.isNotEmpty() && bytes.size <= 32 * 1024 * 1024) { "ocr.input-size" }
        val recognizer = openRecognizer()
        try {
            val upright = BoundedPhotoBitmap.decode(bytes)
            try {
                val width = upright.width
                val height = upright.height
                val detail = if (cameraDetail) bytes else null
                val whole = FuelBoardFraming.Frame(0, 0, width, height, 1, 0.0, false)
                val tokens = mutableListOf(read(recognizer, upright))
                val views = mutableListOf(FuelBoardOcr.associate(tokens[0]))
                var frame = FuelBoardFraming.locate(tokens[0], width, height)
                suspend fun add(view: Bitmap) {
                    val read = readOwned(recognizer, view)
                    tokens += read
                    views += FuelBoardOcr.associate(read)
                }
                if (frame != null) add(boardBitmap(detail, upright, frame, enhance = false, digitHeight = FuelBoardFraming.TARGET_PRICE_HEIGHT))
                if (frame == null || FuelBoardOcr.needsAnotherView(views)) {
                    add(boardView(upright, whole, enhance = true, scale = 1f))
                    // A scene too dark for the first reading may show its board only now.
                    if (frame == null) {
                        frame = FuelBoardFraming.locate(tokens.last(), width, height)
                        if (frame != null) add(boardBitmap(detail, upright, frame, enhance = true, digitHeight = FuelBoardFraming.TARGET_PRICE_HEIGHT))
                    }
                }
                if (frame == null && detail != null) {
                    // Still nothing: a far board can be too small in the bounded photo. Search the camera's own pixels.
                    frame = locateInCameraDetail(recognizer, detail, width, height)
                    if (frame != null) add(boardBitmap(detail, upright, frame, enhance = false, digitHeight = FuelBoardFraming.TARGET_PRICE_HEIGHT))
                }
                if (frame != null && FuelBoardOcr.needsAnotherView(views)) {
                    add(boardBitmap(detail, upright, frame, enhance = false, digitHeight = FuelBoardFraming.COMPACT_PRICE_HEIGHT))
                }
                Analysis(views, tokens, frame, width, height)
            } finally { upright.recycle() }
        } finally { recognizer.close() }
    }

    /**
     * Looks for the board in overlapping tiles of the full-resolution photo, centre first, and
     * returns its frame in bounded-photo coordinates. Only used when the bounded photo was
     * downsampled and showed no board; it stops at the first tile that shows one.
     */
    private suspend fun locateInCameraDetail(recognizer: TextRecognizer, bytes: ByteArray, width: Int, height: Int): FuelBoardFraming.Frame? {
        val finer = BoundedPhotoBitmap.fullLongSide(bytes).toFloat() / maxOf(width, height)
        if (finer < 1.5f) return null
        for (tile in detailTiles(width * finer, height * finer)) {
            val region = try {
                BoundedPhotoBitmap.decodeRegion(bytes, tile[0], tile[1], tile[2], tile[3], MAX_VIEW_SIDE)
            } catch (_: Exception) { return null }
            val gain = region.width / ((tile[2] - tile[0]) * width)
            val offsetX = tile[0] * width
            val offsetY = tile[1] * height
            val tokens = readOwned(recognizer, region).map { token ->
                token.copy(
                    left = (token.left / gain + offsetX).roundToInt(), top = (token.top / gain + offsetY).roundToInt(),
                    right = (token.right / gain + offsetX).roundToInt().coerceAtMost(width),
                    bottom = (token.bottom / gain + offsetY).roundToInt().coerceAtMost(height))
            }
            FuelBoardFraming.locate(tokens, width, height)?.let { return it }
        }
        return null
    }

    /**
     * Overlapping tiles (fractions of the photo) that each decode at full sampling, or at half
     * sampling for very large photos so the grid stays at most 3 x 3. Centre-most first.
     */
    internal fun detailTiles(fullWidth: Float, fullHeight: Float): List<FloatArray> {
        val side = MAX_VIEW_SIDE * (if (maxOf(fullWidth, fullHeight) > MAX_VIEW_SIDE * 2.5f) 2f else 1f)
        fun starts(full: Float): List<Float> {
            val span = (side / full).coerceAtMost(1f)
            if (span >= 1f) return listOf(0f)
            val count = (Math.ceil(((1f - span) / (span * 0.8f)).toDouble()).toInt() + 1).coerceIn(2, 3)
            return List(count) { index -> (1f - span) * index / (count - 1) }
        }
        val spanX = (side / fullWidth).coerceAtMost(1f)
        val spanY = (side / fullHeight).coerceAtMost(1f)
        return starts(fullHeight).flatMap { top -> starts(fullWidth).map { left -> floatArrayOf(left, top, left + spanX, top + spanY) } }
            .sortedBy { tile -> Math.hypot((tile[0] + tile[2]) / 2.0 - 0.5, (tile[1] + tile[3]) / 2.0 - 0.5) }
    }

    /**
     * The board view, drawn from the camera's own pixels when the bounded photo was
     * downsampled and the board needs enlarging; otherwise from the bounded photo.
     */
    private fun boardBitmap(bytes: ByteArray?, upright: Bitmap, frame: FuelBoardFraming.Frame, enhance: Boolean, digitHeight: Int): Bitmap {
        val zoom = FuelBoardFraming.zoomFor(frame, MAX_VIEW_SIDE, digitHeight).toFloat()
        val finer = bytes?.let { BoundedPhotoBitmap.fullLongSide(it).toFloat() / maxOf(upright.width, upright.height) } ?: 1f
        if (bytes == null || finer < 1.5f || zoom <= 1f) return boardView(upright, frame, enhance, zoom)
        // Room for the corners that a tilted board swings into when it is straightened.
        val pad = if (frame.angleDegrees == 0.0) 0 else maxOf(frame.width, frame.height) / 6
        val left = (frame.left - pad).coerceAtLeast(0)
        val top = (frame.top - pad).coerceAtLeast(0)
        val right = (frame.right + pad).coerceAtMost(upright.width)
        val bottom = (frame.bottom + pad).coerceAtMost(upright.height)
        val region = try {
            BoundedPhotoBitmap.decodeRegion(bytes, left.toFloat() / upright.width, top.toFloat() / upright.height,
                right.toFloat() / upright.width, bottom.toFloat() / upright.height, MAX_VIEW_SIDE)
        } catch (_: Exception) {
            return boardView(upright, frame, enhance, zoom)
        }
        try {
            val gain = region.width.toFloat() / (right - left)
            val local = FuelBoardFraming.Frame(
                ((frame.left - left) * gain).roundToInt(), ((frame.top - top) * gain).roundToInt(),
                ((frame.right - left) * gain).roundToInt(), ((frame.bottom - top) * gain).roundToInt(),
                (frame.priceHeight * gain).roundToInt().coerceAtLeast(1), frame.angleDegrees, frame.labeled)
            return boardView(region, local, enhance, zoom / gain)
        } finally { region.recycle() }
    }

    /** The board alone: straightened by the text tilt and enlarged to the target digit height. */
    private fun boardView(source: Bitmap, frame: FuelBoardFraming.Frame, enhance: Boolean, scale: Float): Bitmap {
        val output = Bitmap.createBitmap(
            (frame.width * scale).roundToInt().coerceIn(1, MAX_VIEW_SIDE),
            (frame.height * scale).roundToInt().coerceIn(1, MAX_VIEW_SIDE), Bitmap.Config.ARGB_8888)
        try {
            val paint = Paint(Paint.ANTI_ALIAS_FLAG or Paint.FILTER_BITMAP_FLAG)
            if (enhance) paint.colorFilter = ColorMatrixColorFilter(stretchedGray(source, frame))
            val matrix = Matrix().apply {
                postTranslate(-(frame.left + frame.width / 2f), -(frame.top + frame.height / 2f))
                postRotate(-frame.angleDegrees.toFloat())
                postScale(scale, scale)
                postTranslate(output.width / 2f, output.height / 2f)
            }
            Canvas(output).apply { drawColor(Color.GRAY); drawBitmap(source, matrix, paint) }
            return output
        } catch (error: Exception) { output.recycle(); throw error }
    }

    /** Grayscale with the board's own 2nd..98th luminance percentiles stretched to full range (dark or hazy photos). */
    private fun stretchedGray(source: Bitmap, frame: FuelBoardFraming.Frame): ColorMatrix {
        val gray = ColorMatrix().apply { setSaturation(0f) }
        val step = maxOf(1, maxOf(frame.width, frame.height) / 96)
        val histogram = IntArray(256)
        var count = 0
        var y = frame.top
        while (y < frame.bottom) {
            var x = frame.left
            while (x < frame.right) {
                val pixel = source.getPixel(x.coerceIn(0, source.width - 1), y.coerceIn(0, source.height - 1))
                histogram[(Color.red(pixel) * 299 + Color.green(pixel) * 587 + Color.blue(pixel) * 114) / 1000]++
                count++
                x += step
            }
            y += step
        }
        if (count == 0) return gray
        fun percentile(fraction: Double): Int {
            var seen = 0
            for (level in 0..255) { seen += histogram[level]; if (seen >= count * fraction) return level }
            return 255
        }
        val low = percentile(0.02)
        val high = percentile(0.98)
        if (high - low < 16) return gray
        val gain = 255f / (high - low)
        val offset = -low * gain
        gray.postConcat(ColorMatrix(floatArrayOf(
            gain, 0f, 0f, 0f, offset,
            0f, gain, 0f, 0f, offset,
            0f, 0f, gain, 0f, offset,
            0f, 0f, 0f, 1f, 0f)))
        return gray
    }

    // Transient geometry only; used by private device evaluation, never logs or uploads.
    suspend fun recognizeTokens(bytes: ByteArray): List<FuelBoardOcr.Token> = withContext(Dispatchers.Default) {
        require(bytes.isNotEmpty() && bytes.size <= 32 * 1024 * 1024) { "ocr.input-size" }
        val recognizer = openRecognizer()
        try { readOwned(recognizer, BoundedPhotoBitmap.decode(bytes)) } finally { recognizer.close() }
    }

    /** One recognizer per photo; the model check never sends image bytes. */
    private suspend fun openRecognizer(): TextRecognizer {
        val recognizer = TextRecognition.getClient(TextRecognizerOptions.DEFAULT_OPTIONS)
        try {
            val modules = ModuleInstall.getClient(context)
            if (!withTimeout(5000) { modules.areModulesAvailable(recognizer).await().areModulesAvailable() }) {
                // Request the model only; image bytes are never part of this request.
                modules.installModules(ModuleInstallRequest.newBuilder().addApi(recognizer).build())
                error("ocr.model-pending")
            }
            return recognizer
        } catch (error: Exception) {
            recognizer.close()
            throw IllegalStateException(if (error.message == "ocr.model-pending") "ocr.model-pending" else "ocr.unavailable")
        }
    }

    private suspend fun readOwned(recognizer: TextRecognizer, bitmap: Bitmap): List<FuelBoardOcr.Token> =
        try { read(recognizer, bitmap) } finally { bitmap.recycle() }

    /**
     * The recognizer keeps using the bitmap until its task completes, so a
     * cancellation waits for that one reading (a fraction of a second) and is
     * honoured right after; the caller can then recycle the bitmap safely.
     */
    private suspend fun read(recognizer: TextRecognizer, bitmap: Bitmap): List<FuelBoardOcr.Token> {
        val tokens = withContext(NonCancellable) {
            suspendCancellableCoroutine { continuation ->
                try {
                    recognizer.process(InputImage.fromBitmap(bitmap, 0)).addOnCompleteListener { task ->
                        if (!task.isSuccessful) {
                            continuation.resumeWithException(IllegalStateException("ocr.unavailable"))
                        } else {
                            val lines = task.result.textBlocks.flatMap { it.lines }.take(256)
                            continuation.resume(lines.flatMap { line ->
                                val entries = listOf(Triple(line.text, line.boundingBox, line.angle)) +
                                    line.elements.map { Triple(it.text, it.boundingBox, it.angle) }
                                entries.mapNotNull { (text, rect, angle) ->
                                    rect?.let { FuelBoardOcr.Token(text.take(256), it.left, it.top, it.right, it.bottom, angle) }
                                }
                            }.distinct().take(512))
                        }
                    }
                } catch (_: Exception) {
                    continuation.resumeWithException(IllegalStateException("ocr.unavailable"))
                }
            }
        }
        kotlin.coroutines.coroutineContext.ensureActive()
        return tokens
    }

    /** Device harness readiness: production capture offers retry/manual while the model downloads. */
    suspend fun prepareModel(): Boolean {
        val recognizer = TextRecognition.getClient(TextRecognizerOptions.DEFAULT_OPTIONS)
        val modules = ModuleInstall.getClient(context)
        var listener: InstallStatusListener? = null
        try {
            if (withTimeout(5000) { modules.areModulesAvailable(recognizer).await().areModulesAvailable() }) return true
            return withTimeout(45000) {
                suspendCancellableCoroutine { continuation ->
                    val progress = InstallStatusListener { update ->
                        if (continuation.isActive) when (update.installState) {
                            ModuleInstallStatusUpdate.InstallState.STATE_COMPLETED -> continuation.resume(true)
                            ModuleInstallStatusUpdate.InstallState.STATE_FAILED,
                            ModuleInstallStatusUpdate.InstallState.STATE_CANCELED -> continuation.resume(false)
                        }
                    }
                    listener = progress
                    modules.installModules(ModuleInstallRequest.newBuilder().addApi(recognizer).setListener(progress).build())
                        .addOnCompleteListener { task ->
                            if (continuation.isActive) {
                                if (!task.isSuccessful) continuation.resume(false)
                                else if (task.result.areModulesAlreadyInstalled()) continuation.resume(true)
                            }
                        }
                }
            }
        } finally { listener?.let { modules.unregisterListener(it) }; recognizer.close() }
    }

    private suspend fun <T> Task<T>.await(): T = suspendCancellableCoroutine { continuation ->
        addOnCompleteListener { task ->
            if (continuation.isActive) {
                if (task.isSuccessful) continuation.resume(task.result)
                else continuation.resumeWithException(IllegalStateException("ocr.unavailable"))
            }
        }
    }

    private companion object {
        const val MAX_VIEW_SIDE = 2048
    }
}
