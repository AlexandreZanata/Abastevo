package com.anpfuel.data.remote

import com.anpfuel.data.remote.interceptor.AnpUserAgentInterceptor
import com.anpfuel.data.remote.interceptor.HttpsOnlyInterceptor
import com.anpfuel.data.remote.interceptor.RetryInterceptor
import okhttp3.OkHttpClient
import java.util.concurrent.TimeUnit

object OkHttpClientFactory {

    private const val CONNECT_TIMEOUT_SECONDS = 30L
    private const val READ_TIMEOUT_SECONDS = 60L
    private const val WRITE_TIMEOUT_SECONDS = 60L

    fun create(
        maxRetries: Int = RetryInterceptor.DEFAULT_MAX_RETRIES,
    ): OkHttpClient =
        OkHttpClient.Builder()
            .connectTimeout(CONNECT_TIMEOUT_SECONDS, TimeUnit.SECONDS)
            .readTimeout(READ_TIMEOUT_SECONDS, TimeUnit.SECONDS)
            .writeTimeout(WRITE_TIMEOUT_SECONDS, TimeUnit.SECONDS)
            .addInterceptor(HttpsOnlyInterceptor())
            .addInterceptor(AnpUserAgentInterceptor())
            .addInterceptor(RetryInterceptor(maxRetries = maxRetries))
            .build()
}
