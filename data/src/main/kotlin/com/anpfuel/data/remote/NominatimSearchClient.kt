package com.anpfuel.data.remote

import com.anpfuel.data.mapper.NominatimSearchResponseMapper
import com.anpfuel.domain.valueobject.GeoCoordinates
import java.io.IOException
import javax.inject.Inject
import javax.inject.Singleton
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.OkHttpClient
import okhttp3.Request

/**
 * UC-015 — resolves a normalized Brazilian address into coordinates via Nominatim `/search`.
 */
@Singleton
class NominatimSearchClient @Inject constructor(
    @NominatimClient private val okHttpClient: OkHttpClient,
) {

    suspend fun search(query: String): GeoCoordinates? =
        executeSearch(
            BASE_URL.toHttpUrl().newBuilder()
                .addQueryParameter("q", query),
        )

    /**
     * Structured search using Nominatim's component parameters. Prefer this over
     * [search] whenever the street / city / state parts are known separately —
     * Nominatim resolves it far more accurately than an ambiguous free-text string.
     */
    suspend fun searchStructured(
        street: String,
        city: String,
        state: String,
    ): GeoCoordinates? =
        executeSearch(
            BASE_URL.toHttpUrl().newBuilder()
                .addQueryParameter("street", street)
                .addQueryParameter("city", city)
                .addQueryParameter("state", state),
        )

    private suspend fun executeSearch(urlBuilder: okhttp3.HttpUrl.Builder): GeoCoordinates? =
        withContext(Dispatchers.IO) {
            val url = urlBuilder
                .addQueryParameter("format", "jsonv2")
                .addQueryParameter("limit", "1")
                .addQueryParameter("countrycodes", "br")
                .addQueryParameter("accept-language", "pt-BR,en")
                .build()

            val request = Request.Builder()
                .url(url)
                .header("Accept", "application/json")
                .get()
                .build()

            okHttpClient.newCall(request).execute().use { response ->
                if (!response.isSuccessful) {
                    throw IOException("Nominatim HTTP ${response.code}")
                }
                val body = response.body?.string().orEmpty()
                NominatimSearchResponseMapper.parse(body)
            }
        }

    companion object {
        const val BASE_URL = "https://nominatim.openstreetmap.org/search"
    }
}
