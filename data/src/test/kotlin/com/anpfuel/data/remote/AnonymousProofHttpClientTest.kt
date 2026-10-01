package com.anpfuel.data.remote

import java.io.IOException
import java.util.concurrent.TimeUnit
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * P10-T03: anonymous-proof HTTP bindings (challenge/register/rotate).
 *
 * Network-only with MockWebServer: paths, methods and JSON envelopes are
 * asserted; failures and empty bodies throw. Bodies cap at 64 KiB.
 */
class AnonymousProofHttpClientTest {

    private val fingerprint = "fp:e568c4f52605d14215f6e649000e4ffc2da4caa4bdcbbf2b8bcd9937ff9ea624"

    private fun client(server: MockWebServer): AnonymousProofHttpClient =
        AnonymousProofHttpClient(
            client = OkHttpClient.Builder()
                .connectTimeout(2L, TimeUnit.SECONDS)
                .readTimeout(2L, TimeUnit.SECONDS)
                .build(),
            baseUrl = server.url("/").toString(),
        )

    @Test
    fun `challenge posts fingerprint and parses reply`() {
        val server = MockWebServer()
        try {
            server.enqueue(
                MockResponse().setResponseCode(201)
                    .setBody(
                        "{\"challenge_id\":\"ch-1\",\"nonce\":\"ch-1.client-1\"," +
                            "\"fingerprint\":\"$fingerprint\",\"purpose\":\"REGISTER\"}",
                    ),
            )
            val out = client(server).requestChallenge(fingerprint, "REGISTER")
            assertTrue(out.challengeId == "ch-1")
            val recorded = server.takeRequest()
            assertTrue(recorded.path == "/v1/identity/challenges")
            assertTrue(recorded.method == "POST")
            assertTrue(recorded.body.readUtf8().contains(fingerprint))
        } finally {
            server.shutdown()
        }
    }

    @Test
    fun `register and rotate post frozen envelopes`() {
        val server = MockWebServer()
        try {
            val api = client(server)
            server.enqueue(MockResponse().setResponseCode(201).setBody("""{"contributor_id":"c1"}"""))
            val lines = listOf(
                "\"@method\": POST",
                "\"@authority\": api.example.invalid",
                "\"@path\": /v1/contributors",
                "\"@query\": ",
                "\"created\": 1",
                "\"expires\": 2",
                "\"keyid\": \"$fingerprint\"",
                "\"nonce\": \"ch-1.client-1\"",
            )
            api.register("X", "Y", "ch-1", AnonymousProofHttpClient.Proof(lines, "sig"))
            val recorded = server.takeRequest()
            assertTrue(recorded.path == "/v1/contributors")
            assertTrue(recorded.body.readUtf8().contains("\"public_jwk\""))

            server.enqueue(MockResponse().setResponseCode(200).setBody("""{"contributor_id":"c1"}"""))
            api.rotate(
                "old-1",
                AnonymousProofHttpClient.Proof(lines, "sig-old"),
                "NX",
                "NY",
                "new-1",
                AnonymousProofHttpClient.Proof(lines, "sig-new"),
            )
            val rotated = server.takeRequest()
            assertTrue(rotated.path == "/v1/contributors/me/keys/rotate")
            assertTrue(rotated.body.readUtf8().contains("\"new_jwk\""))
        } finally {
            server.shutdown()
        }
    }

    @Test
    fun `down and empty bodies throw`() {
        val server = MockWebServer()
        try {
            val api = client(server)
            server.enqueue(MockResponse().setResponseCode(500).setBody("down"))
            assertThrows(IOException::class.java) {
                api.requestChallenge(fingerprint, "REGISTER")
            }
            server.enqueue(MockResponse().setResponseCode(201).setBody("  "))
            assertThrows(IOException::class.java) {
                api.requestChallenge(fingerprint, "REGISTER")
            }
        } finally {
            server.shutdown()
        }
    }
}
