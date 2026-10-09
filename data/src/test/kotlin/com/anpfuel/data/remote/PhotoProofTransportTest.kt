package com.anpfuel.data.remote

import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.AuthPorts
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.portable.AuthKeyStore
import com.anpfuel.application.portable.AuthWallClock
import com.anpfuel.application.portable.PortableNonceSource
import com.anpfuel.application.portable.AuthAccountApi
import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.data.local.auth.AndroidAnonymousDeviceKeys
import com.anpfuel.domain.portable.PortableAnonymousProof
import com.anpfuel.domain.portable.PortableAuth
import io.mockk.*
import java.security.MessageDigest
import java.util.Base64
import kotlinx.coroutines.test.runTest
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.RecordedRequest
import org.json.JSONObject
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class PhotoProofTransportTest {
    private class OwnerFixture {
        @Volatile var now=1000L
        var session: PortableAuth.Session? = PortableAuth.Session("synthetic-family","synthetic-account",
            "synthetic-access","synthetic-refresh",1100L,10000L)
        var backup: AuthFlow.KeyBackup? = null
        val api=mockk<AuthAccountApi>()
        val keys=mockk<AndroidAnonymousDeviceKeys>()
        val identity=mockk<AnonymousProofHttpClient>()
        val fingerprint="a".repeat(64)
        val auth=AuthFlow(AuthPorts(object:AuthSessionStore {
            override fun save(session:PortableAuth.Session) { this@OwnerFixture.session=session }
            override fun load()=session
            override fun clear() { session=null }
        },object:AuthKeyStore {
            override fun saveKey(backup:AuthFlow.KeyBackup) { this@OwnerFixture.backup=backup }
            override fun loadKey()=backup
            override fun clearKey() { backup=null }
        },AuthWallClock { now },PortableNonceSource { "synthetic-nonce" },api))
        init {
            every { keys.fingerprint() } answers { fingerprint }
            every { keys.ensureKey() } returns true
        }
        fun transport()=PhotoProofTransport(keys,identity,auth,"https://synthetic.example.invalid",OkHttpClient())
    }

    @Test fun `access expiry preserves retained account scope without refreshing or sending expired tokens`() {
        val fixture=OwnerFixture();val transport=fixture.transport()
        val scope=transport.ownerScope()
        assertEquals("account:synthetic-account:${fixture.fingerprint}",scope)
        fixture.now=1101L
        assertNull(fixture.auth.currentSession())
        assertTrue(fixture.auth.rehydrate() is AuthFlow.AuthState.NeedsRefresh)
        assertEquals(scope,transport.ownerScope())
        verify { fixture.api wasNot Called; fixture.identity wasNot Called }
    }

    @Test fun `offline logout cannot dispatch the previous account intent`() = runTest {
        val fixture=OwnerFixture();val transport=fixture.transport();val previous=transport.ownerScope()!!
        every { fixture.api.revokeAll(any(),any()) } returns AuthApiResult.Err("synthetic-offline")
        fixture.auth.logout()
        assertEquals("anonymous:${fixture.fingerprint}",transport.ownerScope())
        try { transport.post("/v1/observations","same-command",JSONObject(),previous); fail<Unit>("old owner dispatched") }
        catch (_:java.io.IOException) { }
        verify { fixture.identity wasNot Called }
    }

    @Test fun `pending key recovery never silently downgrades account ownership to anonymous`() = runTest {
        val fixture=OwnerFixture();fixture.session=null
        fixture.backup=AuthFlow.KeyBackup("synthetic-user","synthetic-account-key")
        val transport=fixture.transport()
        assertNull(transport.ownerScope())
        try { transport.localScope(); fail<Unit>("unverified recovery received a scope") }
        catch (_:java.io.IOException) { }
        verify { fixture.api wasNot Called; fixture.identity wasNot Called }
    }

    @Test fun `other account and other device key refuse the old intent before network proof`() = runTest {
        val fixture=OwnerFixture();val transport=fixture.transport();val previous=transport.ownerScope()!!
        fixture.session=fixture.session!!.copy(accountId="synthetic-other")
        for (changeKey in listOf(false,true)) {
            if(changeKey) every { fixture.keys.fingerprint() } returns "b".repeat(64)
            assertNotEquals(previous,transport.ownerScope())
            try { transport.post("/v1/observations","same-command",JSONObject(),previous); fail<Unit>("foreign scope dispatched") }
            catch (_:java.io.IOException) { }
        }
        verify { fixture.api wasNot Called; fixture.identity wasNot Called }
    }

    @Test fun `missing device key cannot derive any owner scope`() {
        val fixture=OwnerFixture();every { fixture.keys.fingerprint() } returns null
        assertNull(fixture.transport().ownerScope())
        verify { fixture.identity wasNot Called }
    }

    @Test fun `access expiry during signed response does not discard the same owners receipt`() = runTest {
        val fixture=OwnerFixture()
        every { fixture.keys.publicJwk() } returns ("synthetic-x" to "synthetic-y")
        every { fixture.keys.signDigest(any()) } returns ByteArray(64) { 1 }
        val nonce="123e4567-e89b-12d3-a456-426614174000."+"1".repeat(32)
        every { fixture.identity.requestChallenge(fixture.fingerprint,any()) } answers {
            AnonymousProofHttpClient.ChallengeResponse("synthetic-challenge",nonce,fixture.fingerprint,secondArg(),"{}")
        }
        every { fixture.identity.register(any(),any(),any(),any()) } returns
            "{\"contributor_id\":\"synthetic\",\"key_id\":\"synthetic\"}"
        MockWebServer().use { server ->
            server.dispatcher=object:Dispatcher() {
                override fun dispatch(request:RecordedRequest):MockResponse {
                    fixture.now=1101L
                    return MockResponse().setBody("{\"received\":true}")
                }
            }
            server.start()
            val transport=PhotoProofTransport(fixture.keys,fixture.identity,fixture.auth,
                server.url("/").toString().trimEnd('/'),OkHttpClient())
            val scope=transport.ownerScope()!!
            assertTrue(transport.post("/v1/observations","same-command",JSONObject(),scope).getBoolean("received"))
            assertNull(fixture.auth.currentSession())
            assertEquals(scope,transport.ownerScope())
            val request=server.takeRequest()
            assertNull(request.getHeader("Authorization"))
            assertEquals(nonce,request.getHeader("Signature-Nonce"))
            assertNotNull(request.getHeader("Signature"))
            verify { fixture.api wasNot Called }
        }
    }
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
            every { auth.rehydrate() } returns AuthFlow.AuthState.LoggedOut
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
            server.enqueue(MockResponse().setBody("{\"validation_state\":\"RECEIVED\"}"))
            transport.get("/v1/observations/123e4567-e89b-12d3-a456-426614174010", "anonymous:$fp")
            val read = server.takeRequest() // Drain the preceding refused POST.
            assertEquals("POST", read.method)
            val status = server.takeRequest()
            assertEquals("GET", status.method)
            assertNull(status.getHeader("Content-Type"))
            assertNull(status.getHeader("Idempotency-Key"))
            val readLines = PortableAnonymousProof.buildBaseLines("GET", java.net.URI(origin).rawAuthority,
                status.path!!, "", null, null, status.getHeader("Signature-Created")!!.toLong(),
                status.getHeader("Signature-Expires")!!.toLong(), fp, signNonce)
            assertArrayEquals(MessageDigest.getInstance("SHA-512").digest(PortableAnonymousProof.buildBase(readLines).toByteArray()), digests.last())
            try { transport.post("/v1/observations", "row", JSONObject(), "different-owner"); fail<Unit>("owner changed") }
            catch (_: java.io.IOException) { }
            assertEquals(3, server.requestCount)
            verify(exactly = 3) { identity.requestChallenge(fp, "SIGN") }
            verify(exactly = 1) { identity.register(any(), any(), any(), any()) }
        }
    }
}
