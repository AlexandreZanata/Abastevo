package com.anpfuel.data.map

import java.io.ByteArrayOutputStream
import java.io.IOException
import javax.inject.Inject
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import okhttp3.Request

private const val MAX_ENVELOPE_BYTES = 1 * 1024 * 1024

private val REGION_SLUG = Regex("^[a-z0-9-]{1,64}$")

/**
 * Pinned map-distribution origin. Empty fields mean "not configured" (the UI
 * shows the specific pendency); keys are public verification keys only and a
 * malformed non-empty key set is reported, never silently dropped. [region]
 * is an optional build-time default slug: the app chooses the region at
 * runtime from the city the user opened, so it is not part of [configured].
 */
data class MapDistributionConfig(
    val baseUrl: String,
    val region: String,
    val trustedKeys: Map<String, ByteArray>,
    val keysInvalid: Boolean,
) {
    val missingUrl: Boolean get() = baseUrl.isBlank()
    val missingKeys: Boolean get() = trustedKeys.isEmpty() && !keysInvalid
    val configured: Boolean get() = !missingUrl && !missingKeys && !keysInvalid

    /** Build-time default region slug, or null when unset/invalid. */
    val defaultRegion: String? get() = region.takeIf { isMapRegionSlug(it) }
}

fun isMapRegionSlug(value: String): Boolean = REGION_SLUG.matches(value)

/**
 * Static signed manifest location of one region (D10): the envelope is a
 * pre-signed file served next to the generation directories.
 */
internal fun mapRegionManifestUrl(baseUrl: String, regionSlug: String): String {
    if (!isMapRegionSlug(regionSlug)) throw MapDownloadException("map-region-unknown")
    return "${baseUrl.trimEnd('/')}/dados/mapa-dist/regions/$regionSlug/manifest"
}

/**
 * Fetches the signed distribution envelope of one region slug.
 * Bounded (1 MiB enforced during stream read, even without Content-Length),
 * no silent retry; HTTP failures surface as [MapDownloadException] with the
 * status so the UI can distinguish outage (retryable at the worker layer) from
 * configuration errors.
 */
open class MapDistribution(
    private val http: OkHttpClient,
    private val config: MapDistributionConfig,
    private val ioDispatcher: CoroutineDispatcher = Dispatchers.IO,
) {
    @Inject
    constructor(
        http: OkHttpClient,
        config: MapDistributionConfig,
    ) : this(http, config, Dispatchers.IO)

    open suspend fun fetchEnvelope(regionSlug: String): ByteArray = withContext(ioDispatcher) {
        if (!config.configured) throw MapDownloadException("map-distribution-unconfigured")
        val request = Request.Builder()
            .url(mapRegionManifestUrl(config.baseUrl, regionSlug))
            .header("Accept", "application/json")
            .build()
        val call = http.newCall(request)
        val completionHandle = coroutineContext[Job]?.invokeOnCompletion { cause ->
            if (cause is CancellationException) {
                call.cancel()
            }
        }
        try {
            call.execute().use { response ->
                if (!response.isSuccessful) throw MapDownloadException("http-${response.code}")
                val declaredLength = response.header("Content-Length")?.toLongOrNull()
                if (declaredLength != null && declaredLength > MAX_ENVELOPE_BYTES) {
                    throw MapDownloadException("map-manifest-too-large")
                }
                val body = response.body ?: throw MapDownloadException("map-manifest-empty")
                body.byteStream().use { input ->
                    val out = ByteArrayOutputStream(
                        declaredLength?.toInt()?.coerceIn(256, 64 * 1024) ?: 8 * 1024,
                    )
                    val buffer = ByteArray(8 * 1024)
                    var total = 0
                    while (true) {
                        coroutineContext.ensureActive()
                        val read = input.read(buffer)
                        if (read < 0) break
                        if (read == 0) continue
                        total += read
                        if (total > MAX_ENVELOPE_BYTES) {
                            throw MapDownloadException("map-manifest-too-large")
                        }
                        out.write(buffer, 0, read)
                    }
                    if (total == 0) throw MapDownloadException("map-manifest-empty")
                    out.toByteArray()
                }
            }
        } catch (cancelled: CancellationException) {
            throw cancelled
        } catch (error: MapDownloadException) {
            throw error
        } catch (error: IOException) {
            coroutineContext.ensureActive()
            throw error
        } catch (error: Exception) {
            coroutineContext.ensureActive()
            throw MapDownloadException("map-manifest-failed", error)
        } finally {
            completionHandle?.dispose()
        }
    }
}
