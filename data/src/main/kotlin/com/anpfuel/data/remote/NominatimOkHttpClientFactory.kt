package com.anpfuel.data.remote

import com.anpfuel.data.remote.interceptor.HttpsOnlyInterceptor
import com.anpfuel.data.remote.interceptor.NominatimUserAgentInterceptor
import okhttp3.OkHttpClient
import java.util.concurrent.TimeUnit

object NominatimOkHttpClientFactory {

    private const val CONNECT_TIMEOUT_SECONDS = 15L
    private const val READ_TIMEOUT_SECONDS = 15L

    fun create(): OkHttpClient =
        OkHttpClient.Builder()
            .connectTimeout(CONNECT_TIMEOUT_SECONDS, TimeUnit.SECONDS)
            .readTimeout(READ_TIMEOUT_SECONDS, TimeUnit.SECONDS)
            .writeTimeout(READ_TIMEOUT_SECONDS, TimeUnit.SECONDS)
            .addInterceptor(HttpsOnlyInterceptor())
            .addInterceptor(NominatimUserAgentInterceptor())
            .build()
}
