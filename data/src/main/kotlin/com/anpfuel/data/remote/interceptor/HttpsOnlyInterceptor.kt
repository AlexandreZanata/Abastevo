package com.anpfuel.data.remote.interceptor

import com.anpfuel.data.BuildConfig
import okhttp3.Interceptor
import okhttp3.HttpUrl
import okhttp3.Response
import java.io.IOException

/**
 * Rejects non-HTTPS requests (architecture security — TLS only).
 *
 * The debug-only local API proof may use the exact loopback proof origin;
 * release builds and default debug builds keep the HTTPS-only rule. The
 * release network security config also forbids cleartext, so this cannot
 * silently enable plain HTTP in a shipped build.
 */
class HttpsOnlyInterceptor(
    private val allowLocalProof: Boolean = BuildConfig.DEBUG && BuildConfig.MAP_API_LOCAL_PROOF,
) : Interceptor {

    override fun intercept(chain: Interceptor.Chain): Response {
        val request = chain.request()
        if (request.url.scheme != HTTPS_SCHEME && !isLocalProof(request.url)) {
            throw IOException("Cleartext HTTP is not allowed: ${request.url}")
        }
        return chain.proceed(request)
    }

    private fun isLocalProof(url: HttpUrl): Boolean =
        allowLocalProof && url.host == LOCAL_PROOF_HOST && url.port == LOCAL_PROOF_PORT

    private companion object {
        const val HTTPS_SCHEME = "https"
        const val LOCAL_PROOF_HOST = "127.0.0.1"
        const val LOCAL_PROOF_PORT = 18080
    }
}
