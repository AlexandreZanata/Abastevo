package com.anpfuel.data.enterprise

import android.content.Context
import android.os.Process
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.fleet.EnterpriseReferenceService
import com.anpfuel.application.enterprise.queue.EnterpriseQueueDeliveryFailure
import com.anpfuel.application.portable.*
import com.anpfuel.data.di.EnterpriseModule
import com.anpfuel.data.enterprise.fleet.EnterpriseReferenceHttpClient
import com.anpfuel.data.enterprise.fleet.ProtectedReferenceCacheStore
import com.anpfuel.data.local.auth.AccountHttpApi
import com.anpfuel.data.local.auth.AndroidAnonymousDeviceKeys
import com.anpfuel.data.local.auth.AndroidKeystoreKeys
import com.anpfuel.data.local.auth.KeystoreAccountKey
import com.anpfuel.data.local.auth.KeystoreSessionStore
import com.anpfuel.data.remote.AnonymousProofHttpClient
import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.data.remote.PhotoProofTransport
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.fleet.MyAssets
import com.anpfuel.domain.enterprise.fleet.ReferenceAssignmentKind
import com.anpfuel.domain.enterprise.fleet.ReferenceCache
import com.anpfuel.domain.portable.PortableAuth
import java.io.Closeable
import java.io.File
import java.net.InetAddress
import java.security.KeyStore
import java.security.cert.CertificateFactory
import java.time.Instant
import java.util.UUID
import java.util.concurrent.atomic.AtomicInteger
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

/**
 * Opt-in three-process EMP-FLEET-013 acceptance against the private ordinary PostGIS/HTTPS reference
 * fixture (orgsync_reference_native_fixture_test.go). Stage 1 pulls both organizations online; the
 * driver then force-stops only this target, removes the route to the source and lets the fixture end
 * the primary assignment; stage 2 proves the protected list survives process death offline; after the
 * route returns, stage 3 receives the server's tombstones. No test producer enters the product.
 */
@RunWith(AndroidJUnit4::class)
class EnterpriseReferenceFeedDeviceTest {
    private val context: Context get() = InstrumentationRegistry.getInstrumentation().targetContext

    @Test
    fun stage1PullsBothOrganizationsOnlineIntoTheProtectedCache(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = true)
        try {
            assertEquals("Unexpected previous native reference stage", 0, graph.preferences.getInt(STAGE, 0))
            graph.establishCustody()
            val (a, b) = fixture.organizations
            val first = graph.service.refresh(graph.source.deliveryAccount(), a.partition(fixture))
            val cacheA = requireNotNull(first.cache)
            assertTrue(first.applied)
            assertEquals("AC1: exactly the assets with an active assignment", setOf(a.primary, a.temporary!!), cacheA.assets.map { it.id }.toSet())
            assertFalse("Another driver's asset reached this device", cacheA.assets.any { it.id == a.foreign })
            val primary = cacheA.assets.single { it.id == a.primary }
            assertEquals(ReferenceAssignmentKind.PRIMARY, primary.assignment.kind)
            assertEquals(listOf(a.meter!!), primary.meters.map { it.id }); assertEquals(listOf(a.segment!!), primary.meters.map { it.segmentId })
            val temporary = cacheA.assets.single { it.id == a.temporary }
            assertEquals(ReferenceAssignmentKind.TEMPORARY, temporary.assignment.kind)
            assertEquals(fixture.temporaryEnd, temporary.assignment.validTo)
            val cacheB = requireNotNull(graph.service.refresh(graph.source.deliveryAccount(), b.partition(fixture)).cache)
            assertEquals("AC4: the other organization holds only its own asset", listOf(b.primary), cacheB.assets.map { it.id })
            assertEquals(listOf(a.primary, a.temporary).sorted(), requireNotNull(graph.cached(a)).assets.map { it.id })
            assertEquals(2, graph.pulls.get())
            graph.assertProtectedSurfaces((cacheA.assets + cacheB.assets).flatMap { listOf(it.id, it.name) }.filter { it.length >= 4 })
            graph.preferences.edit().putString(CACHE_PREFIX + a.id, String(cacheA.canonicalBytes)).commit()
            assertTrue("Stage marker must be durable before process termination",
                graph.preferences.edit().putInt(STAGE, Process.myPid()).putInt(STAGE_NUMBER, 1).commit())
        } finally { graph.close() }
    }

    @Test
    fun stage2OfflineAfterProcessDeathKeepsTheListAndNeedsNoNetwork(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = false)
        try {
            val previous = graph.preferences.getInt(STAGE, 0)
            assertTrue("Stage 1 must run in an earlier process", previous > 0 && previous != Process.myPid() && graph.preferences.getInt(STAGE_NUMBER, 0) == 1)
            assertTrue("The server-side end must have happened while this device was offline", File(context.noBackupFilesDir, "enterprise-native-reference-ended").exists())
            val (a, _) = fixture.organizations
            val cached = requireNotNull(graph.cached(a)) { "Process death lost the protected list" }
            assertEquals("The protected cache changed across process death", graph.preferences.getString(CACHE_PREFIX + a.id, null), String(cached.canonicalBytes))
            // The assignment already ended on the server: still listed, the server decides later.
            val snapshot = MyAssets.present(cached, graph.clock.read())
            assertEquals(setOf(a.primary, a.temporary), snapshot.assets.map { it.asset.id }.toSet())
            assertTrue(snapshot.listConfirmed)
            val temporary = snapshot.assets.single { it.asset.id == a.temporary }
            assertTrue("The cached assignment end has passed on the device clock", temporary.assignmentEnded)
            assertFalse(temporary.confirmed); assertTrue(temporary.reviewWarning)
            // The picker's choice needs no network.
            val chosen = requireNotNull(MyAssets.choose(a.id, snapshot, a.primary, warningAccepted = false))
            assertEquals(a.primary, chosen.assetId); assertFalse(chosen.reviewWarningAccepted)
            assertNull(MyAssets.choose(a.id, snapshot, a.temporary!!, warningAccepted = false))
            assertTrue(requireNotNull(MyAssets.choose(a.id, snapshot, a.temporary, warningAccepted = true)).reviewWarningAccepted)
            // A refresh without a route fails in the actual reference client and changes nothing.
            try {
                graph.service.refresh(graph.source.account(), a.partition(fixture))
                fail("A refresh succeeded without a route to the source")
            } catch (_: EnterpriseQueueDeliveryFailure) { }
            assertEquals(String(cached.canonicalBytes), String(requireNotNull(graph.cached(a)).canonicalBytes))
            assertTrue(graph.preferences.edit().putInt(STAGE, Process.myPid()).putInt(STAGE_NUMBER, 2).commit())
        } finally { graph.close() }
    }

    @Test
    fun stage3ReconnectionAppliesTheServerTombstones(): Unit = runBlocking {
        val fixture = readFixture(fixtureFile())
        val graph = Graph(context, fixture, seed = false)
        try {
            val previous = graph.preferences.getInt(STAGE, 0)
            assertTrue("Stage 2 must run in an earlier process", previous > 0 && previous != Process.myPid() && graph.preferences.getInt(STAGE_NUMBER, 0) == 2)
            val (a, b) = fixture.organizations
            val refreshed = graph.service.refresh(graph.source.deliveryAccount(), a.partition(fixture))
            assertTrue(refreshed.applied)
            assertEquals("The ended and the expired assignments left by tombstones", emptyList<String>(), requireNotNull(refreshed.cache).assets.map { it.id })
            assertFalse(requireNotNull(refreshed.cache).forbidden)
            val stillB = requireNotNull(graph.service.refresh(graph.source.deliveryAccount(), b.partition(fixture)).cache)
            assertEquals(listOf(b.primary), stillB.assets.map { it.id })
            assertEquals(2, graph.pulls.get())
        } finally {
            graph.cleanup()
            graph.close()
        }
    }

    private fun fixtureFile() = File(context.noBackupFilesDir, "enterprise-native-reference-fixture.json")

    private class Organization(val id: String, val member: String, val primary: String, val meter: String?, val segment: String?,
        val temporary: String?, val foreign: String?) {
        fun partition(fixture: SourceFixture) = EnterprisePartition.of(fixture.session.accountId, fixture.origin, id)
        override fun toString(): String = "ReferenceFixtureOrganization(redacted)"
    }
    private class SourceFixture(val origin: String, val certificate: String, val session: PortableAuth.Session,
        val temporaryEnd: Instant, val organizations: List<Organization>) {
        override fun toString(): String = "ReferenceSourceFixture(redacted)"
    }

    private class Graph(private val context: Context, private val fixture: SourceFixture, seed: Boolean) : Closeable {
        val preferences = context.getSharedPreferences("enterprise-native-reference", Context.MODE_PRIVATE)
        private val fence = EnterpriseSessionFence()
        private val platformKeys = AndroidKeystoreKeys()
        private val sessions = EnterpriseFencedSessionStore(KeystoreSessionStore(preferences, platformKeys, SESSION_ALIAS), fence)
        private val accountKeys = KeystoreAccountKey(preferences, platformKeys, BACKUP_ALIAS)
        init {
            if (seed) {
                assertNull("Unexpected existing native reference session", sessions.load())
                sessions.save(fixture.session)
            }
            val current = sessions.load() ?: throw AssertionError("Retained native reference session is unavailable")
            assertTrue("Retained source changed ownership", current.accountId == fixture.session.accountId && current.familyId == fixture.session.familyId)
        }
        private val http = fixtureHttp(fixture.certificate)
        private val auth = AuthFlow(AuthPorts(sessions, accountKeys, AuthWallClock { System.currentTimeMillis() / 1000 },
            PortableNonceSource { UUID.randomUUID().toString() }, AccountHttpApi(http, fixture.origin)))
        private val deviceKeys = AndroidAnonymousDeviceKeys(auth, ApiEnvironment.PREVIEW)
        private val proof = PhotoProofTransport(deviceKeys, AnonymousProofHttpClient(http, fixture.origin), auth, fixture.origin, http)
        private val actualSigner = EnterpriseIdentityV1ContactSigner(proof)
        val pulls = AtomicInteger()
        private val signer = EnterpriseDeviceSigner { binding, path, bytes ->
            if (path.endsWith("/sync/reference")) pulls.incrementAndGet()
            actualSigner.headers(binding, path, bytes)
        }
        private val contact = EnterpriseDeviceContactClient(http, fixture.origin, sessions, auth, fence, signer)
        private val files = EnterpriseModule.vaultFiles(context)
        private val keys = EnterpriseModule.vaultKeys()
        private val erasure = EnterpriseVaultErasure(files, keys, fence)
        val clock = AndroidEnterpriseDeviceClock(context)
        private val persistence = ProtectedEnterprisePersistence(files, keys, fence, erasure,
            { sessions.load()?.let { EnterpriseLocalIdentity(it.accountId, fixture.origin, it.familyId) } }, clock::read, contact)
        val source = EnterpriseOperationSessionSource(fixture.origin, sessions, auth, fence, persistence::protectedEnrollment)
        private val cache = ProtectedReferenceCacheStore(persistence)
        val service = EnterpriseReferenceService(source, cache, EnterpriseReferenceHttpClient(http, source, signer), clock::read)

        suspend fun cached(organization: Organization): ReferenceCache? = service.cached(source.account(), organization.partition(fixture))

        suspend fun establishCustody() {
            val local = EnterpriseLocalContext({ sessions.load()?.accountId }, { fixture.origin }, persistence, { fence.epoch })
            fun binding() = sessions.load()?.let { EnterpriseAccessBinding(it.accountId, fixture.origin, it.familyId, fence.epoch) }
            for (organization in fixture.organizations) {
                val before = binding() ?: throw AssertionError("Current native proof is unavailable")
                val owner = local.reserveContact(local.acquireOwner(), before, fence, ::binding)
                val handoff = persistence.confirm(EnterpriseDeviceTarget(organization.partition(fixture), organization.member), before, owner)
                local.select(local.consumeContact(owner, handoff, binding()), organization.partition(fixture))
            }
            assertTrue("Actual identity-v1 signing key was not created", deviceKeys.hasKey())
        }

        /** No cached name, identifier or credential appears outside encryption. */
        fun assertProtectedSurfaces(secrets: List<String>) {
            for (organization in fixture.organizations) {
                val token = EnterpriseVaultCodec.token(organization.partition(fixture))
                val generations = files.generations(token)
                assertTrue("Committed generations were not retained", generations.isNotEmpty())
                for (generation in generations) for (descriptor in listOf(false, true)) {
                    val text = String(files.readGeneration(token, generation, descriptor) ?: continue, Charsets.ISO_8859_1)
                    for (secret in secrets + fixture.session.accountId + fixture.session.accessToken)
                        assertFalse("A cached value appeared outside encryption", text.contains(secret))
                }
            }
        }

        fun cleanup() {
            fixture.organizations.forEach { assertTrue("Native reference cleanup was incomplete", erasure.partitionRevoked(it.partition(fixture))) }
            val identityAlias = AndroidAnonymousDeviceKeys.aliasFor(AuthFlow.AuthState.Active(fixture.session), fixture.origin)
            KeyStore.getInstance("AndroidKeyStore").apply {
                load(null); deleteEntry(identityAlias); deleteEntry(SESSION_ALIAS); deleteEntry(BACKUP_ALIAS)
            }
            assertTrue(preferences.edit().clear().commit())
            for (name in listOf("enterprise-native-reference-fixture.json", "enterprise-native-reference-ended")) {
                val file = File(context.noBackupFilesDir, name)
                assertTrue("Private native proof cleanup failed", !file.exists() || file.delete())
                assertFalse("Private native proof survived cleanup", file.exists())
            }
        }

        override fun close() {
            http.connectionPool.evictAll()
            http.dispatcher.executorService.shutdown()
        }
    }

    companion object {
        private const val STAGE = "reference_stage_process"
        private const val STAGE_NUMBER = "reference_stage_number"
        private const val CACHE_PREFIX = "reference_cache_"
        private const val SESSION_ALIAS = "enterprise_native_reference_session"
        private const val BACKUP_ALIAS = "enterprise_native_reference_backup"

        private fun readFixture(file: File): SourceFixture = try {
            require(file.isFile && file.length() in 1..32_768)
            val value = JSONObject(file.readText())
            val origin = value.getString("origin").also { require(it == ApiEnvironment.PREVIEW.origin) }
            val account = EnterprisePartition.requireId(value.getString("account_id"))
            val family = EnterprisePartition.requireId(value.getString("family_id"))
            fun token(name: String) = value.getString(name).also { require(it.length in 1..512 && it.all { c -> c.code in 33..126 }) }
            val session = PortableAuth.Session(family, account, token("access_token"), token("refresh_token"),
                value.getLong("access_expires_at"), value.getLong("refresh_expires_at"))
            val certificate = value.getString("certificate_pem").also { require(it.length in 1..8192) }
            val organizations = value.getJSONArray("organizations").also { require(it.length() == 2) }.let { rows ->
                (0 until rows.length()).map { index -> rows.getJSONObject(index).let {
                    fun optional(name: String) = if (it.has(name)) EnterprisePartition.requireId(it.getString(name)) else null
                    Organization(EnterprisePartition.requireId(it.getString("org_id")), EnterprisePartition.requireId(it.getString("membership_id")),
                        EnterprisePartition.requireId(it.getString("primary_asset_id")), optional("meter_id"), optional("segment_id"),
                        optional("temporary_asset_id"), optional("foreign_asset_id"))
                } }
            }
            require(organizations.map { it.id }.distinct().size == 2 && organizations[0].temporary != null)
            SourceFixture(origin, certificate, session, Instant.parse(value.getString("temporary_end")), organizations)
        } catch (_: Exception) { throw AssertionError("Disposable reference fixture is invalid") }

        private fun fixtureHttp(pem: String): OkHttpClient = try {
            val certificate = CertificateFactory.getInstance("X.509").generateCertificate(pem.byteInputStream())
            val anchors = KeyStore.getInstance(KeyStore.getDefaultType()).apply { load(null); setCertificateEntry("disposable-reference", certificate) }
            val trust = TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm()).apply { init(anchors) }
                .trustManagers.single() as X509TrustManager
            val tls = SSLContext.getInstance("TLS").apply { init(null, arrayOf(trust), null) }
            OkHttpClient.Builder().sslSocketFactory(tls.socketFactory, trust).dns(object : Dns {
                override fun lookup(hostname: String): List<InetAddress> {
                    check(hostname == "api.anpfuel.example.invalid") { "Native reference refused an unrelated host" }
                    return listOf(InetAddress.getByAddress(byteArrayOf(127, 0, 0, 1)))
                }
            }).followRedirects(false).followSslRedirects(false).retryOnConnectionFailure(false).build()
        } catch (_: Exception) { throw AssertionError("Disposable reference trust anchor is invalid") }
    }
}
