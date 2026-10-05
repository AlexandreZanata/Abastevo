package com.anpfuel.data.remote

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class ApiEnvironmentTest {

    @Test
    fun `staging selects teste origin and preview stays invalid`() {
        assertEquals("https://teste.abastevo.com.br", ApiEnvironment.STAGING.origin)
        assertEquals("https://api.anpfuel.example.invalid", ApiEnvironment.PREVIEW.origin)
        assertTrue(ApiEnvironment.STAGING.origin != ApiEnvironment.PREVIEW.origin)
    }

    @Test
    fun `join appends relative routes without double v1`() {
        val routes = listOf(
            "/health/live",
            "/health/ready",
            "/v1/stations",
            "/v1/stations/nearby",
            "/v1/stations/d6c74c23-63db-4c24-a2e5-408cb23bad26/prices",
            "/v1/accounts/email/codes",
            "/v1/feedback/ratings",
            "/v1/observations",
            "/v1/uploads",
        )
        for (route in routes) {
            val url = ApiEnvironment.STAGING.join(route)
            assertEquals("https://teste.abastevo.com.br$route", url)
            assertTrue(!url.contains("//v1"), url)
            assertTrue(!url.contains("/v1/v1"), url)
        }
    }

    @Test
    fun `malformed and non-https origins are refused`() {
        assertThrows(IllegalArgumentException::class.java) {
            ApiEnvironment.requireValidOrigin("http://teste.abastevo.com.br")
        }
        assertThrows(IllegalArgumentException::class.java) {
            ApiEnvironment.requireValidOrigin("")
        }
        assertThrows(IllegalArgumentException::class.java) {
            ApiEnvironment.requireValidOrigin("https://teste.abastevo.com.br/v1")
        }
        assertThrows(IllegalArgumentException::class.java) {
            ApiEnvironment.requireValidOrigin("not-a-url")
        }
    }

    @Test
    fun `origin carries no secret or precise location`() {
        for (env in ApiEnvironment.values()) {
            assertTrue(!env.origin.contains("token", ignoreCase = true))
            assertTrue(!env.origin.contains("key", ignoreCase = true))
            assertTrue(!env.origin.contains("-23.55"))
            assertTrue(!env.origin.contains("-46.63"))
        }
    }
}
