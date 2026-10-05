package com.anpfuel.data.remote.profile

import com.anpfuel.domain.profile.StationProfile
import com.anpfuel.domain.repository.StationProfileGateway
import java.io.IOException
import java.util.UUID
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import okhttp3.Request
import org.json.JSONObject

/** Anonymous, bounded public read. Private proof/session fields are never decoded. */
class StationProfileHttpClient(
    private val client: OkHttpClient,
    private val baseUrl: String,
) : StationProfileGateway {
    override suspend fun getProfile(stationId: String): StationProfile? = withContext(Dispatchers.IO) {
        require(UUID.fromString(stationId).toString() == stationId.lowercase())
        val request = Request.Builder()
            .url("${baseUrl.trimEnd('/')}/v1/stations/$stationId/profile")
            .header("Cache-Control", "no-cache")
            .get().build()
        client.newCall(request).execute().use { response ->
            if (response.code == 404) return@withContext null
            if (!response.isSuccessful) throw IOException("Profile unavailable (${response.code})")
            val body = response.body ?: throw IOException("Profile unavailable")
            val source = body.source()
            if (source.request(64L * 1024L + 1L)) throw IOException("Profile exceeds limit")
            val doc = try { JSONObject(source.readUtf8()) } catch (_: Exception) {
                throw IOException("Invalid profile response")
            }
            if (doc.optString("station_id") != stationId) throw IOException("Profile identity mismatch")
            val rawBusiness = doc.optJSONObject("business") ?: JSONObject()
            val business = com.anpfuel.domain.profile.PUBLIC_BUSINESS_KEYS.mapNotNull { key ->
                (rawBusiness.opt(key) as? String)?.let { key to it }
            }.toMap()
            StationProfileDto(
                stationId, doc.optString("display_name"), business,
                doc.optInt("revision"), doc.optJSONObject("operator")?.optString("source").orEmpty(),
                doc.optBoolean("has_badge", false),
            ).toDomain()
        }
    }
}
