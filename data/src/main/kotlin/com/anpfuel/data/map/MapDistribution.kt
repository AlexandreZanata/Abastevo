package com.anpfuel.data.map

import javax.inject.Inject
import javax.inject.Singleton
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.Request

/**
 * Pinned map-distribution origin. Empty fields mean "not configured" (the UI
 * shows the specific pendency); keys are public verification keys only and a
 * malformed non-empty key set is reported, never silently dropped.
 */
data class MapDistributionConfig(
    val baseUrl: String,
    val region: String,
    val trustedKeys: Map<String, ByteArray>,
    val keysInvalid: Boolean,
) {
    val missingUrl: Boolean get() = baseUrl.isBlank()
    val missingRegion: Boolean get() = region.isBlank()
    val missingKeys: Boolean get() = trustedKeys.isEmpty() && !keysInvalid
    val configured: Boolean get() = !missingUrl && !missingRegion && !missingKeys && !keysInvalid
}

/**
 * Fetches the signed distribution envelope for the configured region.
 * Bounded (1 MiB), no silent retry; HTTP failures surface as
 * [MapDownloadException] with the status so the UI can distinguish outage
 * (retryable at the worker layer) from configuration errors.
 */
open class MapDistribution @Inject constructor(
    private val http: okhttp3.OkHttpClient,
    private val config: MapDistributionConfig,
) {
    open suspend fun fetchEnvelope(): ByteArray = withContext(Dispatchers.IO) {
        if (!config.configured) throw MapDownloadException("map-distribution-unconfigured")
        if (!config.region.matches(Regex("^[a-z0-9-]{1,64}$"))) throw MapDownloadException("map-distribution-unconfigured")
        val request = Request.Builder()
            .url("${config.baseUrl.trimEnd('/')}/v1/map/regions/${config.region}/manifest")
            .header("Accept", "application/json")
            .build()
        try {
            http.newCall(request).execute().use { response ->
                if (!response.isSuccessful) throw MapDownloadException("http-${response.code}")
                val bytes = response.body?.bytes() ?: throw MapDownloadException("map-manifest-empty")
                if (bytes.size > 1 * 1024 * 1024) throw MapDownloadException("map-manifest-too-large")
                bytes
            }
        } catch (error: MapDownloadException) {
            throw error
        } catch (error: java.io.IOException) {
            throw error
        } catch (error: Exception) {
            throw MapDownloadException("map-manifest-failed", error)
        }
    }
}
