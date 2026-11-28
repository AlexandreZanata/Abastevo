package com.anpfuel.app.location

import android.Manifest
import android.app.Activity
import android.content.Context
import android.content.ContextWrapper
import android.content.Intent
import android.net.Uri
import android.provider.Settings
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.ui.platform.LocalContext
import androidx.core.app.ActivityCompat

/** Foreground location only (precise or approximate); never background location. */
private val LOCATION_PERMISSIONS = arrayOf(
    Manifest.permission.ACCESS_FINE_LOCATION,
    Manifest.permission.ACCESS_COARSE_LOCATION,
)

/**
 * Launches Android's foreground location request. [onResult] receives whether
 * either permission was granted and whether Android will show the dialog
 * again (false after "don't ask again"), so callers can offer settings instead.
 */
@Composable
fun rememberLocationPermissionRequest(onResult: (granted: Boolean, canAskAgain: Boolean) -> Unit): () -> Unit {
    val context = LocalContext.current
    val latestOnResult = rememberUpdatedState(onResult)
    val launcher = rememberLauncherForActivityResult(ActivityResultContracts.RequestMultiplePermissions()) { grants ->
        val granted = grants.values.any { it }
        val canAskAgain = context.findActivity()?.let { activity ->
            ActivityCompat.shouldShowRequestPermissionRationale(activity, Manifest.permission.ACCESS_FINE_LOCATION)
        } ?: true
        latestOnResult.value(granted, canAskAgain)
    }
    return remember(launcher) { { launcher.launch(LOCATION_PERMISSIONS) } }
}

/** Opens this app's system settings page, where location can be allowed again. */
fun Context.openAppSettings() {
    val intent = Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS, Uri.fromParts("package", packageName, null))
        .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
    runCatching { startActivity(intent) }
}

private tailrec fun Context.findActivity(): Activity? = when (this) {
    is Activity -> this
    is ContextWrapper -> baseContext.findActivity()
    else -> null
}
