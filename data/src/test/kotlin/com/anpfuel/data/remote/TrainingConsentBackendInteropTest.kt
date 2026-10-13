package com.anpfuel.data.remote

import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.data.local.auth.AndroidAnonymousDeviceKeys
import com.anpfuel.data.local.auth.AnonymousProofCrypto
import com.anpfuel.domain.model.ContributionScope
import com.anpfuel.domain.portable.PortableAuth
import com.anpfuel.domain.repository.*
import io.mockk.*
import java.io.File
import java.security.*
import java.security.cert.CertificateFactory
import java.security.cert.X509Certificate
import java.security.spec.PKCS8EncodedKeySpec
import java.util.Base64
import javax.net.ssl.*
import kotlinx.coroutines.test.runTest
import okhttp3.*
import org.json.JSONObject
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Assumptions.assumeTrue
import org.junit.jupiter.api.Test

class TrainingConsentBackendInteropTest {
    @Test fun `actual Kotlin signed consent interoperates with Go verifier and PostgreSQL`() = runTest {
        val path=System.getenv("ANPFUEL_ANDROID_CONSENT_FIXTURE")
        assumeTrue(!path.isNullOrBlank(),"explicit Go cross-runtime harness required")
        val fixture=JSONObject(File(path!!).readText())
        val origin=fixture.getString("origin")
        val cert=CertificateFactory.getInstance("X.509").generateCertificate(fixture.getString("certificate").byteInputStream()) as X509Certificate
        val trustStore=KeyStore.getInstance(KeyStore.getDefaultType()).apply {load(null);setCertificateEntry("owned-fixture",cert)}
        val factory=TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm()).apply {init(trustStore)}
        val trust=factory.trustManagers.single() as X509TrustManager
        val tls=SSLContext.getInstance("TLS").apply {init(null,arrayOf(trust),SecureRandom())}
        val http=OkHttpClient.Builder().sslSocketFactory(tls.socketFactory,trust).followRedirects(false).build()
        var current="owner"
        var tamper=false
        val auth=mockk<AuthFlow>()
        every {auth.rehydrate()} answers { AuthFlow.AuthState.Active(PortableAuth.Session("family",current,"access","refresh",Long.MAX_VALUE,Long.MAX_VALUE)) }
        val keys=mockk<AndroidAnonymousDeviceKeys>()
        every {keys.ensureKey()} returns true
        every {keys.fingerprint()} answers {fixture.getJSONObject(current).getString("fingerprint")}
        every {keys.publicJwk()} answers {fixture.getJSONObject(current).let {it.getString("x") to it.getString("y")}}
        every {keys.signDigest(any())} answers {
            val key=KeyFactory.getInstance("EC").generatePrivate(PKCS8EncodedKeySpec(Base64.getDecoder().decode(fixture.getJSONObject(current).getString("key"))))
            val signer=Signature.getInstance("NONEwithECDSAinP1363Format").apply {initSign(key);update(firstArg<ByteArray>())}
            signer.sign().also {if(tamper) it[0]=(it[0].toInt() xor 1).toByte()}
        }
        val identity=mockk<AnonymousProofHttpClient>()
        every {identity.requestChallenge(any(),any())} answers {
            val request=Request.Builder().url("$origin/fixture/challenge/$current").build()
            val nonce=http.newCall(request).execute().use { assertEquals(200,it.code);JSONObject(it.body!!.string()).getString("nonce") }
            AnonymousProofHttpClient.ChallengeResponse("fixture",nonce,firstArg(),secondArg(),"{}")
        }
        // Keys were enrolled with the actual registrar before the harness started.
        every {identity.register(any(),any(),any(),any())} returns "{\"contributor_id\":\"enrolled\",\"key_id\":\"enrolled\"}"
        val proof=PhotoProofTransport(keys,identity,auth,origin,http)
        val client=TrainingConsentHttpClient(proof)
        val scope=ContributionScope(proof.ownerScope()!!,origin)
        val digest=fixture.getString("digest")
        assertEquals(100L,client.consentStatus(scope).revision)
        val grant=client.grantConsent(scope,"training-terms-v1",digest,101,"grant-101")
        assertEquals("GRANTED",grant.state)
        assertTrue(client.consentStatus(scope).granted)
        assertEquals("REVOKED",client.revokeConsent(scope,102,"off-102").state)
        assertTrue(client.revokeConsent(scope,102,"off-102").replayed)
        try {client.grantConsent(scope,"training-terms-v1",digest,101,"grant-101");fail<Unit>("stale grant accepted")}
        catch(e:TrainingConsentException) {assertEquals(TrainingConsentRejectKind.STALE_REVISION,e.kind)}
        assertFalse(client.consentStatus(scope).granted)
        tamper=true
        try {client.consentStatus(scope);fail<Unit>("forged signature accepted")}
        catch(e:TrainingConsentException) {assertEquals(TrainingConsentRejectKind.AUTH_REQUIRED,e.kind)}
        tamper=false;current="other"
        val other=ContributionScope(proof.ownerScope()!!,origin)
        assertFalse(client.consentStatus(other).granted)
        try {client.consentStatus(scope);fail<Unit>("foreign scope accepted")}
        catch(_:TrainingConsentException) {}
        http.newCall(Request.Builder().url("$origin/v1/privacy/training-consent").build()).execute().use {assertEquals(401,it.code)}
        http.newCall(Request.Builder().url("$origin/v1/privacy/training-terms").build()).execute().use {
            assertEquals(200,it.code)
            val document=JSONObject(it.body!!.string())
            assertEquals(digest,AnonymousProofCrypto.sha256Hex(document.getString("text").toByteArray()))
            assertFalse(document.getBoolean("collection_available"))
        }
    }
}
