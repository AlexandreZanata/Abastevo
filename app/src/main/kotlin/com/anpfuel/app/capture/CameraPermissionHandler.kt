package com.anpfuel.app.capture

import android.Manifest
import android.content.Context
import android.content.pm.PackageManager
import androidx.core.content.ContextCompat
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import javax.inject.Singleton

/**
 * P10-T04 camera permission gate (mirrors [com.anpfuel.app.location.LocationPermissionHandler]).
 *
 * Read-only check only; the system permission dialog launches from the
 * capture screen. A denied camera keeps the contributor on the
 * metadata-only path and performs no capture, OCR or upload work.
 */
@Singleton
class CameraPermissionHandler @Inject constructor(
    @ApplicationContext private val context: Context,
) {
    fun hasCameraPermission(): Boolean =
        ContextCompat.checkSelfPermission(context, Manifest.permission.CAMERA) ==
            PackageManager.PERMISSION_GRANTED
}
