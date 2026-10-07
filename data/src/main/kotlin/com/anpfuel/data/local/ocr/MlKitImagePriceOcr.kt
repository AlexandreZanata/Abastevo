package com.anpfuel.data.local.ocr

import android.content.Context
import dagger.hilt.android.qualifiers.ApplicationContext
import com.google.android.gms.common.moduleinstall.ModuleInstall
import com.google.android.gms.common.moduleinstall.ModuleInstallRequest
import com.google.android.gms.common.moduleinstall.InstallStatusListener
import com.google.android.gms.common.moduleinstall.ModuleInstallStatusUpdate
import com.google.android.gms.tasks.Task
import kotlinx.coroutines.withTimeout
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.graphics.Matrix
import androidx.exifinterface.media.ExifInterface
import com.anpfuel.application.port.ImagePriceOcr
import com.anpfuel.domain.portable.FuelBoardOcr
import com.google.mlkit.vision.common.InputImage
import com.google.mlkit.vision.text.TextRecognition
import com.google.mlkit.vision.text.latin.TextRecognizerOptions
import java.io.ByteArrayInputStream
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
        FuelBoardOcr.associate(recognizeTokens(bytes))

    // Transient geometry only; used by private device evaluation, never logs or uploads.
    suspend fun recognizeTokens(bytes: ByteArray): List<FuelBoardOcr.Token> = withContext(Dispatchers.Default) {
        require(bytes.isNotEmpty() && bytes.size <= 32 * 1024 * 1024) { "ocr.input-size" }
        val bitmap = decode(bytes)
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
        suspendCancellableCoroutine { continuation ->
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

    private fun decode(bytes: ByteArray): Bitmap {
        val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
        BitmapFactory.decodeByteArray(bytes, 0, bytes.size, bounds)
        require(bounds.outWidth > 0 && bounds.outHeight > 0 && bounds.outWidth.toLong() * bounds.outHeight <= 100_000_000) { "ocr.invalid-image" }
        var sample = 1
        while (maxOf(bounds.outWidth, bounds.outHeight) / sample > 2048) sample *= 2
        val raw = BitmapFactory.decodeByteArray(bytes, 0, bytes.size, BitmapFactory.Options().apply { inSampleSize = sample })
            ?: error("ocr.invalid-image")
        val orientation = runCatching { ExifInterface(ByteArrayInputStream(bytes)).getAttributeInt(ExifInterface.TAG_ORIENTATION, ExifInterface.ORIENTATION_NORMAL) }.getOrDefault(ExifInterface.ORIENTATION_NORMAL)
        val matrix = Matrix().apply {
            when (orientation) {
                ExifInterface.ORIENTATION_FLIP_HORIZONTAL -> setScale(-1f, 1f)
                ExifInterface.ORIENTATION_ROTATE_180 -> setRotate(180f)
                ExifInterface.ORIENTATION_FLIP_VERTICAL -> setScale(1f, -1f)
                ExifInterface.ORIENTATION_TRANSPOSE -> { setRotate(90f); postScale(-1f,1f) }
                ExifInterface.ORIENTATION_ROTATE_90 -> setRotate(90f)
                ExifInterface.ORIENTATION_TRANSVERSE -> { setRotate(270f); postScale(-1f,1f) }
                ExifInterface.ORIENTATION_ROTATE_270 -> setRotate(270f)
            }
        }
        if (matrix.isIdentity) return raw
        return Bitmap.createBitmap(raw, 0, 0, raw.width, raw.height, matrix, true).also { if (it !== raw) raw.recycle() }
    }
}
