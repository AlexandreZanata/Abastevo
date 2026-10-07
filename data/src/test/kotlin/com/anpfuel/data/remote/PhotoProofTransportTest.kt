package com.anpfuel.data.remote

import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.data.local.auth.AndroidAnonymousDeviceKeys
import com.anpfuel.domain.portable.PortableAnonymousProof
import io.mockk.*
import java.security.MessageDigest
import java.util.Base64
import kotlinx.coroutines.test.runTest
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.json.JSONObject
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class PhotoProofTransportTest {
    @Test fun `proof uses exact issued nonce and frozen exact JSON body digest without redirects`() = runTest {
        MockWebServer().use { server ->
            server.start()
            val keys = mockk<AndroidAnonymousDeviceKeys>()
            val identity = mockk<AnonymousProofHttpClient>()
            val auth = mockk<AuthFlow>()
            val fp = "a".repeat(64)
            val registerNonce = "123e4567-e89b-12d3-a456-426614174000." + "1".repeat(32)
            val signNonce = "123e4567-e89b-12d3-a456-426614174001." + "2".repeat(32)
            every { keys.ensureKey() } returns true
            every { keys.fingerprint() } returns fp
            every { keys.publicJwk() } returns ("synthetic-x" to "synthetic-y")
            every { auth.currentSession() } returns null
            val digests = mutableListOf<ByteArray>()
            every { keys.signDigest(capture(digests)) } returns ByteArray(64) { 1 }
            every { identity.requestChallenge(fp, "REGISTER") } returns AnonymousProofHttpClient.ChallengeResponse("reg", registerNonce, fp, "REGISTER", "{}")
            every { identity.requestChallenge(fp, "SIGN") } returns AnonymousProofHttpClient.ChallengeResponse("sign", signNonce, fp, "SIGN", "{}")
            val registration = slot<AnonymousProofHttpClient.Proof>()
            every { identity.register(any(), any(), any(), capture(registration)) } returns "{\"contributor_id\":\"synthetic\",\"key_id\":\"synthetic\"}"
            val origin = server.url("/").toString().trimEnd('/')
            val transport = PhotoProofTransport(keys, identity, auth, origin, OkHttpClient())
            server.enqueue(MockResponse().setBody("{\"ok\":true}"))
            transport.post("/v1/photo-captures", "stable-client-id", JSONObject().put("label", "preço"))
            val request = server.takeRequest()
            assertEquals(signNonce, request.getHeader("Signature-Nonce"))
            assertEquals("stable-client-id", request.getHeader("Idempotency-Key"))
            assertEquals("application/json", request.getHeader("Content-Type"))
            assertTrue(registration.captured.lines.any { it.contains(registerNonce) })
            val bytes = request.body.readByteArray()
            val digest = "\"sha-512=:${Base64.getEncoder().encodeToString(MessageDigest.getInstance("SHA-512").digest(bytes))}:\""
            val created = request.getHeader("Signature-Created")!!.toLong()
            val expires = request.getHeader("Signature-Expires")!!.toLong()
            val lines = PortableAnonymousProof.buildBaseLines("POST", java.net.URI(origin).rawAuthority,
                "/v1/photo-captures", "", "application/json", digest, created, expires, fp, signNonce)
            assertArrayEquals(MessageDigest.getInstance("SHA-512").digest(PortableAnonymousProof.buildBase(lines).toByteArray()), digests.last())
            assertEquals(120L, expires - created)
            server.enqueue(MockResponse().setResponseCode(307).addHeader("Location", server.url("/redirect")))
            try { transport.post("/v1/photo-captures", "stable-client-id", JSONObject()); fail<Unit>("redirect accepted") }
            catch (failure: PhotoOperationRefused) { assertEquals(307, failure.status) }
            assertEquals(2, server.requestCount)
            verify(exactly = 1) { identity.register(any(), any(), any(), any()) }
        }
    }
}
