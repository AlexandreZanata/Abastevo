package com.anpfuel.app.ui.map

import android.Manifest
import android.content.Context
import android.content.pm.PackageManager
import android.location.LocationManager
import android.os.SystemClock
import androidx.core.content.ContextCompat
import androidx.core.location.LocationManagerCompat
import android.os.CancellationSignal
import com.anpfuel.domain.routing.RoutePoint
import kotlin.coroutines.resume
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withTimeoutOrNull

/** Explicit one-shot request; cancellation/timeout never leaves a subscription. */
fun requestRouteFix(context:Context, label:String, result:(RoutePoint?)->Unit):()->Unit {
    val cancellation=CancellationSignal()
    val fine=ContextCompat.checkSelfPermission(context,Manifest.permission.ACCESS_FINE_LOCATION)==PackageManager.PERMISSION_GRANTED
    val coarse=ContextCompat.checkSelfPermission(context,Manifest.permission.ACCESS_COARSE_LOCATION)==PackageManager.PERMISSION_GRANTED
    if(!fine && !coarse) { result(null);return {} }
    val manager=context.getSystemService(LocationManager::class.java) ?: run { result(null); return {} }
    val provider=if(fine && manager.isProviderEnabled(LocationManager.GPS_PROVIDER)) LocationManager.GPS_PROVIDER else LocationManager.NETWORK_PROVIDER
    try {
        LocationManagerCompat.getCurrentLocation(manager,provider,cancellation,ContextCompat.getMainExecutor(context)) { fix ->
            val age=fix?.let { SystemClock.elapsedRealtimeNanos()-it.elapsedRealtimeNanos }
            result(if(fix!=null && fix.hasAccuracy() && fix.accuracy in 0f..100f && age!=null && age in 0..15_000_000_000L)
                RoutePoint(fix.latitude,fix.longitude,label) else null)
        }
    } catch (_:SecurityException) { result(null) } catch (_:IllegalArgumentException) { result(null) }
    return { cancellation.cancel() }
}


internal suspend fun findCurrentRoutePoint(context: Context, label: String): RoutePoint? =
    withTimeoutOrNull(15_000L) {
        suspendCancellableCoroutine { continuation ->
            val cancel = requestRouteFix(context, label) { point ->
                if (continuation.isActive) continuation.resume(point)
            }
            continuation.invokeOnCancellation { cancel() }
        }
    }
