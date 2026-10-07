package com.anpfuel.application.port

/** Transient OS fix: never persisted, logged or queued. */
data class CaptureFix(
    val latitude: Double,
    val longitude: Double,
    val accuracyMeters: Double,
    val capturedAtMillis: Long,
    val permissionGranted: Boolean,
    val sourceInfoPresent: Boolean,
    val simulated: Boolean,
)

fun interface CaptureLocationSource {
    suspend fun freshFix(): CaptureFix?
}

data class PhotoCapturePermission(
    val captureId: String,
    val stationId: String,
    val issuedAtMillis: Long,
    val cameraExpiresAtMillis: Long,
    val expiresAtMillis: Long,
    val ownerScope: String,
    val origin: String,
)

interface PhotoCaptureGate {
    fun isCurrent(permission: PhotoCapturePermission): Boolean
    suspend fun authorize(stationId: String, clientCaptureId: String, fix: CaptureFix): PhotoCapturePermission
}

/** Client rejection only; the signed backend/PostGIS decision remains authoritative. */
fun CaptureFix.isEligibleForPhotoCapture(nowMillis: Long): Boolean =
    permissionGranted && sourceInfoPresent && !simulated &&
        latitude.isFinite() && latitude in -90.0..90.0 &&
        longitude.isFinite() && longitude in -180.0..180.0 &&
        accuracyMeters.isFinite() && accuracyMeters in 0.0..100.0 &&
        capturedAtMillis > 0 && capturedAtMillis <= nowMillis &&
        nowMillis - capturedAtMillis <= 120_000
