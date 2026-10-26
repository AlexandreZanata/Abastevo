package com.anpfuel.app.ui.map

import android.content.Context
import android.content.SharedPreferences
import com.anpfuel.domain.routing.NativeRouteRequest
import com.anpfuel.domain.routing.RoadAvoidance
import com.anpfuel.domain.routing.RoutePoint
import org.json.JSONArray
import org.json.JSONObject

/**
 * Private owner-bound最小 planning recovery: only the planning request and its
 * owner, never the sampled GPS stream, tokens or telemetry. Backed by private
 * SharedPreferences (excluded from OS backup) with synchronous commit so a
 * storage failure is honest; stop always invalidates.
 */
class GuidanceRecoveryStore(context: Context) {
    private val prefs: SharedPreferences =
        context.getSharedPreferences(PREFS, Context.MODE_PRIVATE)

    data class Saved(val owner: String, val request: NativeRouteRequest)

    fun save(owner: String, request: NativeRouteRequest): Boolean {
        return try {
            val payload = JSONObject()
                .put("owner", owner)
                .put("language", request.language)
                .put("avoid", JSONArray(request.avoid.map { it.name }))
                .put(
                    "points",
                    JSONArray().apply {
                        request.points.forEach { point ->
                            put(
                                JSONObject()
                                    .put("lat", point.latitude)
                                    .put("lon", point.longitude)
                                    .put("label", point.label.take(512))
                                    .put("approx", point.approximateEntrance),
                            )
                        }
                    },
                ).toString()
            prefs.edit().putString(KEY, payload).commit()
        } catch (_: Exception) {
            false
        }
    }

    fun load(): Saved? {
        return try {
            val raw = prefs.getString(KEY, null) ?: return null
            val json = JSONObject(raw)
            val owner = json.optString("owner").takeIf { it.isNotEmpty() } ?: return null
            val language = json.optString("language", "en").takeIf { it.isNotEmpty() } ?: "en"
            val avoid = mutableSetOf<RoadAvoidance>()
            json.optJSONArray("avoid")?.let { array ->
                for (i in 0 until array.length()) {
                    runCatching { RoadAvoidance.valueOf(array.optString(i)) }.getOrNull()?.let { avoid += it }
                }
            }
            val points = mutableListOf<RoutePoint>()
            json.optJSONArray("points")?.let { array ->
                for (i in 0 until array.length()) {
                    val item = array.optJSONObject(i) ?: continue
                    val lat = item.optDouble("lat", Double.NaN)
                    val lon = item.optDouble("lon", Double.NaN)
                    if (!lat.isFinite() || !lon.isFinite()) continue
                    points += RoutePoint(lat, lon, item.optString("label", ""), item.optBoolean("approx", true))
                }
            }
            val request = runCatching { NativeRouteRequest(points, avoid, language).also { it.validate() } }.getOrNull()
                ?: return null
            Saved(owner, request)
        } catch (_: Exception) {
            null
        }
    }

    fun clear() {
        runCatching { prefs.edit().remove(KEY).commit() }
    }

    companion object {
        const val PREFS = "guidance_recovery"
        const val KEY = "owner_bound_request"
    }
}
