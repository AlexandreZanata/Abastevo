package com.anpfuel.data.remote.interceptor

import okhttp3.Interceptor
import okhttp3.Response

/**
 * Custom User-Agent required by Nominatim usage policy (BR-021).
 */
class NominatimUserAgentInterceptor(
    private val userAgent: String = DEFAULT_USER_AGENT,
) : Interceptor {

    override fun intercept(chain: Interceptor.Chain): Response {
        val request = chain.request().newBuilder()
            .header(USER_AGENT_HEADER, userAgent)
            .build()
        return chain.proceed(request)
    }

    companion object {
        const val USER_AGENT_HEADER = "User-Agent"
        const val DEFAULT_USER_AGENT =
            "AnpFuel/3.1.0 (Android; https://github.com/AlexandreZanata/TABELA-ANP-COMBUSTIVEIS/issues)"
    }
}
