package com.anpfuel.data.remote.interceptor

import okhttp3.Interceptor
import okhttp3.Response

/**
 * Sets a stable User-Agent for ANP gov.br requests (avoids bot blocking).
 */
class AnpUserAgentInterceptor : Interceptor {

    override fun intercept(chain: Interceptor.Chain): Response {
        val request = chain.request().newBuilder()
            .header(USER_AGENT_HEADER, USER_AGENT_VALUE)
            .build()
        return chain.proceed(request)
    }

    private companion object {
        const val USER_AGENT_HEADER = "User-Agent"
        const val USER_AGENT_VALUE = "AnpFuel/1.0 (Android; open-source fuel price reader)"
    }
}
