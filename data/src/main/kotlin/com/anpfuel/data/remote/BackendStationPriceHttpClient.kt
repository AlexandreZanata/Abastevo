package com.anpfuel.data.remote

import java.io.IOException
import java.net.URLEncoder
import okhttp3.OkHttpClient
import okhttp3.Request

/**
 * P10-T02 bounded backend HTTP reader for
 * `GET /v1/stations/{id}/prices`.
 *
 * Network-only: transport failures and non-2xx responses throw
 * [IOException]; empty bodies throw as well. Cache fallback lives in
 * the application use case, never here. HTTPS only; the preview base
 * URL never resolves until deployment configuration lands.
 */
class BackendStationPriceHttpClient(
    private val client: OkHttpClient,
    private val baseUrl: String,
    private val nowMillis: () -> Long = { System.currentTimeMillis() },
) {
    data class RawResponse(val body: String, val fetchedAtMillis: Long)

    fun fetch(stationId: String, fuelProductWire: String? = null): RawResponse {
        require(stationId.isNotBlank()) { "station_id is blank" }
        val encodedId = URLEncoder.encode(stationId, Charsets.UTF_8.name())
        val url = buildString {
            append(baseUrl.trimEnd('/'))
            append("/v1/stations/")
            append(encodedId)
            append("/prices")
            if (fuelProductWire != null) {
                append("?fuel_product=")
                append(URLEncoder.encode(fuelProductWire, Charsets.UTF_8.name()))
            }
        }
        val request = Request.Builder()
            .url(url)
            .get()
            .header("Accept", "application/json")
            .build()
        val fetchedAt = nowMillis()
        try {
            client.newCall(request).execute().use { response ->
                if (!response.isSuccessful) {
                    throw IOException("backend price read failed: HTTP ${response.code}")
                }
                val body = response.body?.string()
                if (body.isNullOrBlank()) {
                    throw IOException("backend price read failed: empty body")
                }
                return RawResponse(body = body, fetchedAtMillis = fetchedAt)
            }
        } catch (error: IOException) {
            throw error
        } catch (error: Exception) {
            throw IOException("backend price read failed", error)
        }
    }
}
