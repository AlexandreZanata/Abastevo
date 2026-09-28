package com.anpfuel.data.remote

import com.anpfuel.data.mapper.NominatimResponseMapper
import com.anpfuel.domain.valueobject.DeviceLocation
import java.io.IOException
import javax.inject.Inject
import javax.inject.Singleton
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.OkHttpClient
import okhttp3.Request

@Singleton
class NominatimReverseGeocodeClient @Inject constructor(
    @NominatimClient private val okHttpClient: OkHttpClient,
) {

    suspend fun reverseGeocode(location: DeviceLocation): NominatimResponseMapper.ParsedAddress? =
        withContext(Dispatchers.IO) {
            val url = BASE_URL.toHttpUrl().newBuilder()
                .addQueryParameter("lat", location.latitude.toString())
                .addQueryParameter("lon", location.longitude.toString())
                .addQueryParameter("format", "jsonv2")
                .addQueryParameter("addressdetails", "1")
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
                NominatimResponseMapper.parse(body)
            }
        }

    companion object {
        const val BASE_URL = "https://nominatim.openstreetmap.org/reverse"
    }
}
