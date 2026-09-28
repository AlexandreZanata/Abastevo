package com.anpfuel.data.remote.interceptor

import okhttp3.Interceptor
import okhttp3.Response
import java.io.IOException

/**
 * Rejects non-HTTPS requests (architecture security — TLS only).
 */
class HttpsOnlyInterceptor : Interceptor {

    override fun intercept(chain: Interceptor.Chain): Response {
        val request = chain.request()
        if (request.url.scheme != HTTPS_SCHEME) {
            throw IOException("Cleartext HTTP is not allowed: ${request.url}")
        }
        return chain.proceed(request)
    }

    private companion object {
        const val HTTPS_SCHEME = "https"
    }
}
