package com.anpfuel.data.local.media

import android.content.Context
import android.graphics.Bitmap
import com.anpfuel.domain.valueobject.DomainId
import dagger.hilt.android.qualifiers.ApplicationContext
import java.io.ByteArrayOutputStream
import java.io.File
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Private on-device vehicle photos (JPEG, app storage only).
 *
 * Local-only by design; a future premium cloud sync would upload these
 * bytes per account and device. Files are named by vehicle id so the
 * Room `photo_path` column stays the single source of truth for
 * presence; deleting a vehicle removes its file best-effort.
 */
@Singleton
class VehiclePhotoStore @Inject constructor(
    @ApplicationContext context: Context,
) {

    private val directory = File(context.filesDir, DIRECTORY).apply { mkdirs() }

    fun fileName(vehicleId: DomainId): String = "${vehicleId.value}.jpg"

    fun fileFor(vehicleId: DomainId): File = File(directory, fileName(vehicleId))

    fun fileForName(photoPath: String?): File? =
        photoPath?.takeIf { it.isNotBlank() }?.let { File(directory, File(it).name) }

    fun savePhoto(vehicleId: DomainId, bytes: ByteArray): String {
        require(bytes.isNotEmpty() && bytes.size <= MAX_INPUT_BYTES) { "vehicle-photo.input-size" }
        val decoded = BoundedPhotoBitmap.decode(bytes, MAX_SIDE_PX)
        try {
            val scaled = decoded
            val out = ByteArrayOutputStream()
            require(scaled.compress(Bitmap.CompressFormat.JPEG, QUALITY, out)) { "vehicle-photo.encode" }
            val name = fileName(vehicleId)
            File(directory, name).writeBytes(out.toByteArray())
            return name
        } finally {
            decoded.recycle()
        }
    }

    fun readPhotoBytes(photoPath: String?): ByteArray? =
        fileForName(photoPath)?.takeIf { it.isFile }?.readBytes()

    fun deletePhoto(vehicleId: DomainId) {
        runCatching { fileFor(vehicleId).delete() }
    }

    companion object {
        const val DIRECTORY = "vehicle_photos"
        const val MAX_INPUT_BYTES = 16 * 1024 * 1024
        const val MAX_SIDE_PX = 1024
        const val QUALITY = 85
    }
}
