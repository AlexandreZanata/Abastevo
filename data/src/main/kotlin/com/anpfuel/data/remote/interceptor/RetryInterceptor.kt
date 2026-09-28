package com.anpfuel.data.remote.interceptor

import okhttp3.Interceptor
import okhttp3.Response
import java.io.IOException
import java.net.HttpURLConnection

/**
 * Retries transient network and server errors up to [maxRetries] times.
 */
class RetryInterceptor(
    private val maxRetries: Int = DEFAULT_MAX_RETRIES,
) : Interceptor {

    init {
        require(maxRetries >= 0) { "maxRetries must be non-negative" }
    }

    override fun intercept(chain: Interceptor.Chain): Response {
        var attempt = 0
        var lastException: IOException? = null

        while (attempt <= maxRetries) {
            try {
                val response = chain.proceed(chain.request())
                if (attempt < maxRetries && response.code in RETRYABLE_HTTP_CODES) {
                    response.close()
                    attempt++
                    continue
                }
                return response
            } catch (error: IOException) {
                lastException = error
                if (attempt >= maxRetries) {
                    throw error
                }
                attempt++
            }
        }

        throw lastException ?: IOException("Request failed after $maxRetries retries")
    }

    companion object {
        const val DEFAULT_MAX_RETRIES = 3

        private val RETRYABLE_HTTP_CODES = setOf(
            HttpURLConnection.HTTP_CLIENT_TIMEOUT,
            HttpURLConnection.HTTP_UNAVAILABLE,
            429,
            502,
            504,
        )
    }
}
