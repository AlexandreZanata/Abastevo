package com.anpfuel.data.remote

import java.io.IOException
import java.net.URLEncoder
import okhttp3.OkHttpClient
import okhttp3.Request

/**
 * P35-T01 bounded Directory HTTP reader.
 *
 * Consumes `GET /v1/stations` (paginated list), `GET /v1/stations/nearby`
 * (bounded lat/lon/radius/limit, server never echoes the request point)
 * and `GET /v1/stations/{station_id}` (UUID detail). Network-only:
 * transport failures and non-2xx/empty bodies throw [IOException]; cache
 * fallback lives in the repository, never here. HTTPS only; out-of-range
 * coordinates are refused before any network call. No user GPS is logged.
 */
class DirectoryStationHttpClient(
    private val client: OkHttpClient,
    private val baseUrl: String,
) {
    fun list(limit: Int, cursor: String?): String {
        require(limit in 1..100) { "limit must be 1..100" }
        val url = buildString {
            append(baseUrl.trimEnd('/'))
            append("/v1/stations?limit=").append(limit)
            if (cursor != null) {
                append("&cursor=").append(URLEncoder.encode(cursor, Charsets.UTF_8.name()))
            }
        }
        return get(url)
    }

    /**
     * City-scoped station search. A blank query lists every station in
     * the city (the `q` parameter is omitted); a non-blank query needs
     * at least 2 characters and filters `display_name` server-side.
     */
    fun search(municipalityCode: String, query: String, limit: Int): String {
        val trimmed = query.trim()
        require(municipalityCode.matches(Regex("[0-9]{7}")) && (trimmed.isEmpty() || trimmed.length in 2..100) && limit in 1..100)
        val url = buildString {
            append(baseUrl.trimEnd('/'))
            append("/v1/stations?limit=").append(limit)
            append("&municipality_code=").append(municipalityCode)
            if (trimmed.isNotEmpty()) {
                append("&q=").append(URLEncoder.encode(trimmed, Charsets.UTF_8.name()))
            }
        }
        return get(url)
    }

    fun nearby(lat: Double, lon: Double, radiusMeters: Int, limit: Int): String {
        require(lat in -90.0..90.0) { "lat out of range" }
        require(lon in -180.0..180.0) { "lon out of range" }
        require(radiusMeters in 100..15000) { "radius_m must be 100..15000" }
        require(limit in 1..100) { "limit must be 1..100" }
        val url = buildString {
            append(baseUrl.trimEnd('/'))
            append("/v1/stations/nearby?lat=").append(lat)
            append("&lon=").append(lon)
            append("&radius_m=").append(radiusMeters)
            append("&limit=").append(limit)
        }
        return get(url)
    }

    fun detail(stationId: String): String {
        require(stationId.isNotBlank()) { "station_id is blank" }
        val encoded = URLEncoder.encode(stationId, Charsets.UTF_8.name())
        return get(baseUrl.trimEnd('/') + "/v1/stations/" + encoded)
    }

    fun byCnpj(cnpj: String): String? {
        require(cnpj.matches(Regex("[A-Z0-9]{12}[0-9]{2}"))) { "invalid CNPJ shape" }
        return try { get(baseUrl.trimEnd('/') + "/v1/stations/by-cnpj/" + cnpj) }
        catch (_: StationNotFound) { null }
    }

    private class StationNotFound : IOException("station not found")

    private fun get(url: String): String {
        val request = Request.Builder()
            .url(url)
            .get()
            .header("Accept", "application/json")
            .build()
        try {
            client.newCall(request).execute().use { response ->
                if (response.code == 404) throw StationNotFound()
                if (!response.isSuccessful) {
                    throw IOException("directory read failed: HTTP ${response.code}")
                }
                val body = response.body?.string()
                if (body.isNullOrBlank()) {
                    throw IOException("directory read failed: empty body")
                }
                return body
            }
        } catch (error: IOException) {
            throw error
        } catch (error: Exception) {
            throw IOException("directory read failed", error)
        }
    }
}
