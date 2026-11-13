package com.anpfuel.data.enterprise

import android.content.Context
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.application.enterprise.*
import com.anpfuel.application.portable.*
import com.anpfuel.data.local.auth.AccountHttpApi
import com.anpfuel.data.local.auth.AndroidAnonymousDeviceKeys
import com.anpfuel.data.local.auth.AndroidKeystoreKeys
import com.anpfuel.data.local.auth.KeystoreAccountKey
import com.anpfuel.data.local.auth.KeystoreSessionStore
import com.anpfuel.data.remote.AnonymousProofHttpClient
import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.data.remote.PhotoProofTransport
import com.anpfuel.domain.enterprise.EnterpriseLocalRecord
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterprisePendingOperation
import com.anpfuel.domain.portable.PortableAuth
import java.io.File
import java.net.InetAddress
import java.security.KeyStore
import java.security.cert.CertificateFactory
import java.util.UUID
import javax.net.ssl.SSLContext
import javax.net.ssl.TrustManagerFactory
import javax.net.ssl.X509TrustManager
import kotlinx.coroutines.runBlocking
import okhttp3.Dns
import okhttp3.OkHttpClient
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

/** Actual Android identity-v1 → ordinary PostGIS source → protected custody.
 * Supply the private ephemeral fixture produced by the opt-in nativefixture Go
 * test. There is no test-control endpoint, fake signer, trust-all manager or
 * production origin override. The source is reached through explicit adb reverse.
 */
@RunWith(AndroidJUnit4::class)
class EnterpriseSignedContactDeviceTest {
    @Test
    fun malformedPrivateFixtureAndTrustErrorsNeverEchoCredentialInput() {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val temporary = File(context.cacheDir, "synthetic-contact-parser-" + UUID.randomUUID())
        val sentinel = "SYNTHETIC-CREDENTIAL-MUST-NOT-ENTER-FAILURE-LOGS"
        try {
            for (input in listOf("{\"access_token\":\"$sentinel\",", "{\"origin\":\"$sentinel\"}")) {
                temporary.writeText(input)
                try { readFixture(temporary); fail("Malformed private fixture was accepted") }
                catch (failure: AssertionError) {
                    assertEquals("Disposable source fixture is invalid", failure.message)
                    assertNull(failure.cause)
                    assertFalse(failure.toString().contains(sentinel))
                }
            }
            try { fixtureHttp(sentinel); fail("Malformed trust anchor was accepted") }
            catch (failure: AssertionError) {
                assertEquals("Disposable source trust anchor is invalid", failure.message)
                assertNull(failure.cause)
                assertFalse(failure.toString().contains(sentinel))
            }
        } finally { temporary.delete() }
    }

    @Test
    fun actualKeyEnrollmentLeaseGenericRefusalAndExactSecurityErasure(): Unit = runBlocking {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val fixtureFile = File(context.noBackupFilesDir, "enterprise-native-contact-fixture.json")
        assertTrue("Supply the private disposable source fixture", fixtureFile.isFile)
        val fixture = readFixture(fixtureFile)
        val origin = fixture.origin
        val http = fixtureHttp(fixture.certificate)
        val fence = EnterpriseSessionFence()
        val preferences = context.getSharedPreferences("enterprise-native-contact", Context.MODE_PRIVATE)
        val platformKeys = AndroidKeystoreKeys()
        val sessionAlias = "enterprise_native_contact_session"
        val backupAlias = "enterprise_native_contact_backup"
        val sessions = EnterpriseFencedSessionStore(KeystoreSessionStore(preferences, platformKeys, sessionAlias), fence)
        val accountKeys = KeystoreAccountKey(preferences, platformKeys, backupAlias)
        val original = fixture.session
        sessions.save(original)
        val auth = AuthFlow(AuthPorts(sessions, accountKeys, AuthWallClock { System.currentTimeMillis() / 1000 },
            PortableNonceSource { UUID.randomUUID().toString() }, AccountHttpApi(http, origin)))
        val deviceKeys = AndroidAnonymousDeviceKeys(auth, ApiEnvironment.PREVIEW)
        val proof = PhotoProofTransport(deviceKeys, AnonymousProofHttpClient(http, origin), auth, origin, http)
        val client = EnterpriseDeviceContactClient(http, origin, sessions, auth, fence, EnterpriseIdentityV1ContactSigner(proof))
        val partition = EnterprisePartition.of(original.accountId, origin, fixture.organization)
        val target = EnterpriseDeviceTarget(partition, fixture.membership)
        val root = File(context.noBackupFilesDir, "enterprise-native-contact-vault")
        val files = AtomicEnterpriseVaultFiles(root)
        val keys = AndroidEnterpriseVaultKeys()
        val erasure = EnterpriseVaultErasure(files, keys, fence)
        val clock = AndroidEnterpriseDeviceClock(context)
        var actualReply: EnterpriseContactReply? = null
        val recordedAuthority = com.anpfuel.application.enterprise.EnterpriseContactAuthority { request ->
            client.confirm(request).also { actualReply = it }
        }
        val persistence = ProtectedEnterprisePersistence(files, keys, fence, erasure,
            { sessions.load()?.let { com.anpfuel.application.enterprise.EnterpriseLocalIdentity(it.accountId, origin, it.familyId) } },
            clock::read, recordedAuthority)
        val local = EnterpriseLocalContext({ sessions.load()?.accountId }, { origin }, persistence, { fence.epoch })
        fun binding() = sessions.load()?.let { EnterpriseAccessBinding(it.accountId, origin, it.familyId, fence.epoch) }
        val operation = UUID.randomUUID().toString()
        val record = UUID.randomUUID().toString()
        val payload = "SYNTHETIC-NATIVE-ORDINARY-SOURCE-PENDING-WORK".toByteArray()
        try {
            val first = binding()!!
            val owner = local.reserveContact(local.acquireOwner(), first, fence, ::binding)
            val handoff = persistence.confirm(target, first, owner)
            val selected = local.select(local.consumeContact(owner, handoff, binding()), partition)
            val active = actualReply as? EnterpriseContactReply.Active ?: error("Real source did not grant active custody")
            assertEquals(target, active.grant.enrollment.target)
            assertEquals(original.familyId, active.grant.family)
            assertEquals(30L * 24 * 60 * 60 * 1000, active.grant.validityMillis)
            assertNull(active.grant.managedUnlockSeconds)
            assertTrue(deviceKeys.hasKey())
            selected.put(EnterpriseLocalRecord(partition, record, payload))
            assertTrue(selected.enqueue(EnterprisePendingOperation(partition, operation, record, payload)))
            assertArrayEquals(payload, selected.pending().single().envelope)
            // A slow fixture start may legitimately rotate the same family.
            // Keep the current proof rather than restoring an obsolete token.
            val retainedProof = sessions.load() ?: error("Current source proof unavailable")
            check(retainedProof.accountId == original.accountId && retainedProof.familyId == original.familyId)

            // An actual invalid parent token is a generic refusal, never a wipe.
            sessions.save(retainedProof.copy(accessToken = "SYNTHETIC-INVALID-PARENT-PROOF"))
            try {
                client.confirm(EnterpriseContactRequest(binding()!!, target, active.grant.enrollment))
                fail("The source accepted an invalid personal proof")
            } catch (expected: EnterpriseProtectionRefused) {
                assertEquals(EnterpriseAccessError.SIGN_IN_REQUIRED, expected.error)
            }
            assertArrayEquals(payload, persistence.pending(partition, 50).single().envelope)
            sessions.save(retainedProof)

            // Revoke this exact source enrollment through its actual own-device
            // company cookie/CSRF route, then consume the real signed ERASE reply.
            val company = CompanyAccessClient(http, origin, sessions, auth, fence)
            val companyOwner = CompanyAccessClient.Owner()
            val current = binding()!!
            assertTrue(company.open(companyOwner) is EnterpriseAccessResult.Ok)
            val revoked = company.post(companyOwner, current,
                "/v1/orgs/${partition.organizationId}/memberships/${target.membershipId}/devices/self/${active.grant.enrollment.enrollmentId}/revoke",
                JSONObject().put("schema_version", 1).put("operation_id", UUID.randomUUID().toString())
                    .put("revision", active.grant.enrollment.revision).put("revocation_class", "LOST_OR_COMPROMISED")
                    .put("reason", "Synthetic native protection acceptance"))
            assertTrue("Actual own-device revocation failed", revoked is EnterpriseAccessResult.Ok)
            val next = binding()!!
            val terminalOwner = local.reserveContact(local.acquireOwner(), next, fence, ::binding)
            try {
                persistence.confirm(target, next, terminalOwner)
                fail("A revoked device acquired protected custody")
            } catch (_: EnterpriseVaultUnrecoverable) { /* The exact signed verdict reports lost protected work. */ }
            val terminal = actualReply as? EnterpriseContactReply.Erase ?: error("Missing real exact security verdict")
            assertEquals(active.grant.enrollment.enrollmentId, terminal.enrollment.enrollmentId)
            assertEquals(active.grant.enrollment.revision + 1, terminal.enrollment.revision)
            try { persistence.pending(partition, 50); fail("Revoked protected work reopened") }
            catch (_: EnterpriseLocalDenied) { /* Durable denial, not an in-memory clear. */ }
            val token = EnterpriseVaultCodec.token(partition)
            assertTrue(files.denied(token))
            assertFalse("First source erasure left unfinished cleanup", erasure.cleanupRequired)
            assertTrue("Source erasure retained generation files", files.generations(token).isEmpty())
            assertNull(files.read(token))
            assertTrue("Source erasure retained generation keys", keys.aliases(EnterpriseVaultGenerationNames.prefix(token)).isEmpty())
            assertNull(keys.existing(EnterpriseVaultCodec.alias(token)))
            val restarted = ProtectedEnterprisePersistence(files, keys, fence, EnterpriseVaultErasure(files, keys, fence),
                { com.anpfuel.application.enterprise.EnterpriseLocalIdentity(original.accountId, origin, original.familyId) },
                clock::read, client)
            try { restarted.pending(partition, 50); fail("Recreated source-erased store reopened") }
            catch (_: EnterpriseLocalDenied) { /* Reads the durable tombstone, not the old object. */ }
        } finally {
            // Cleanup only this test's exact partition/account-owned key handles.
            erasure.partitionRevoked(partition)
            deviceKeys.forgetKey()
            sessions.clear()
            accountKeys.clearKey()
            KeyStore.getInstance("AndroidKeyStore").apply { load(null) }.also {
                it.deleteEntry(sessionAlias); it.deleteEntry(backupAlias)
            }
            assertTrue(preferences.edit().clear().commit())
            root.deleteRecursively()
            fixtureFile.delete()
            http.connectionPool.evictAll()
            http.dispatcher.executorService.shutdown()
        }
    }

    private class SourceFixture(val origin: String, val certificate: String, val session: PortableAuth.Session,
        val organization: String, val membership: String) {
        override fun toString(): String = "SourceFixture(redacted)"
    }

    private fun readFixture(file: File): SourceFixture = try {
        val value = JSONObject(file.readText())
        val origin = value.getString("origin").also { require(it == ApiEnvironment.PREVIEW.origin) }
        val account = EnterprisePartition.requireId(value.getString("account_id"))
        val family = EnterprisePartition.requireId(value.getString("family_id"))
        val organization = EnterprisePartition.requireId(value.getString("organization_id"))
        val membership = EnterprisePartition.requireId(value.getString("membership_id"))
        fun token(name: String) = value.getString(name).also { raw -> require(raw.length in 1..512 && raw.all { it.code in 33..126 }) }
        val session = PortableAuth.Session(family, account, token("access_token"), token("refresh_token"),
            value.getLong("access_expires_at"), value.getLong("refresh_expires_at"))
        val certificate = value.getString("certificate_pem").also { require(it.length in 1..8192) }
        SourceFixture(origin, certificate, session, organization, membership)
    } catch (_: Exception) {
        // Android JSONTokener can echo its entire source in a syntax error.
        // Never attach the private fixture, parser message or original cause.
        throw AssertionError("Disposable source fixture is invalid")
    }

    private fun fixtureHttp(pem: String): OkHttpClient = try {
        val certificate = CertificateFactory.getInstance("X.509").generateCertificate(pem.byteInputStream())
        val anchors = KeyStore.getInstance(KeyStore.getDefaultType()).apply { load(null); setCertificateEntry("disposable-source", certificate) }
        val trust = TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm()).apply { init(anchors) }
            .trustManagers.single() as X509TrustManager
        val tls = SSLContext.getInstance("TLS").apply { init(null, arrayOf(trust), null) }
        OkHttpClient.Builder().sslSocketFactory(tls.socketFactory, trust)
            .dns(object : Dns {
                override fun lookup(hostname: String): List<InetAddress> {
                    check(hostname == "api.anpfuel.example.invalid") { "Acceptance refused an unrelated host" }
                    return listOf(InetAddress.getByAddress(byteArrayOf(127, 0, 0, 1)))
                }
            }).followRedirects(false).followSslRedirects(false).retryOnConnectionFailure(false).build()
    } catch (_: Exception) {
        throw AssertionError("Disposable source trust anchor is invalid")
    }
}
