package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.*
import com.anpfuel.application.portable.*
import com.anpfuel.domain.enterprise.*
import com.anpfuel.domain.portable.PortableAuth
import com.anpfuel.data.enterprise.VaultFixture.Companion.A
import com.anpfuel.data.enterprise.VaultFixture.Companion.B
import com.anpfuel.data.enterprise.VaultFixture.Companion.ORIGIN
import com.anpfuel.data.enterprise.VaultFixture.Companion.partition
import io.mockk.every
import io.mockk.mockk
import java.nio.file.Path
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.async
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir

class EnterpriseVaultLifecycleTest {
    @TempDir lateinit var root: Path
    private val p get() = partition()

    private class Sessions : AuthSessionStore {
        var value: PortableAuth.Session? = PortableAuth.Session(A, A, "synthetic-access", "synthetic-refresh", 100, 200)
        var fail = false
        override fun save(session: PortableAuth.Session) { if (fail) error("private save"); value = session }
        override fun load() = value
        override fun clear() { if (fail) error("private clear"); value = null }
    }

    @Test
    fun logoutKeepsEncryptedWorkButInvalidatesOldHandlesEvenAfterSameOwnerReturns(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault"))
        val raw = Sessions()
        val sessions = EnterpriseFencedSessionStore(raw, f.fence)
        f.store.confirm(p)
        f.store.enqueue(EnterprisePendingOperation(p, A, B, byteArrayOf(7)))
        val context = EnterpriseLocalContext({ sessions.load()?.accountId }, { ORIGIN }, f.store, { f.fence.epoch })
        val bound = context.select(p)
        sessions.clear()
        assertTrue(f.keys.values.isNotEmpty())
        val epoch = f.fence.epoch
        sessions.save(PortableAuth.Session(B, A, "new-access", "new-refresh", 100, 200))
        assertTrue(f.fence.epoch > epoch)
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { bound.pending() } }
        f.identity = EnterpriseLocalIdentity(A, ORIGIN, B)
        f.server++
        f.store.confirm(p)
        assertEquals(1, context.select(p).pending().size)
        val latest = context.select(p)
        raw.fail = true
        assertThrows(IllegalStateException::class.java) { sessions.clear() }
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { latest.pending() } }
    }

    @Test
    fun refreshWithinSameFamilyKeepsContextWhileOwnerOrFamilyChangesFenceIt() {
        val raw = Sessions()
        val fence = EnterpriseSessionFence()
        val sessions = EnterpriseFencedSessionStore(raw, fence)
        val old = raw.value!!
        sessions.save(old.copy(accessToken = "rotated"))
        assertEquals(0L, fence.epoch)
        sessions.save(old.copy(accountId = B))
        assertTrue(fence.epoch > 0)
        val previous = fence.epoch
        sessions.save(old.copy(familyId = B))
        assertTrue(fence.epoch > previous)
    }

    @Test
    fun sessionClearWaitsForInFlightDiskAccessAndAlwaysInvalidatesItsEpoch() {
        val raw = Sessions()
        val fence = EnterpriseSessionFence()
        val sessions = EnterpriseFencedSessionStore(raw, fence)
        val entered = CountDownLatch(1)
        val release = CountDownLatch(1)
        val clearing = CountDownLatch(1)
        val executor = Executors.newFixedThreadPool(2)
        try {
            val reader = executor.submit { fence.access { entered.countDown(); assertTrue(release.await(5, TimeUnit.SECONDS)) } }
            assertTrue(entered.await(5, TimeUnit.SECONDS))
            val logout = executor.submit { clearing.countDown(); sessions.clear() }
            assertTrue(clearing.await(5, TimeUnit.SECONDS))
            assertNotNull(raw.value)
            release.countDown()
            reader.get(5, TimeUnit.SECONDS); logout.get(5, TimeUnit.SECONDS)
            assertNull(raw.value)
            assertTrue(fence.epoch > 0)
        } finally { release.countDown(); executor.shutdownNow() }
    }

    @Test
    fun backgroundInvalidationNeverWaitsForDiskAndWithholdsAnInFlightReply() {
        val entered = CountDownLatch(1)
        val release = CountDownLatch(1)
        var hold = false
        val f = VaultFixture(root.resolve("vault"), transform = { delegate -> object : EnterpriseVaultFiles by delegate {
            override fun read(token: String): ByteArray? {
                if (hold) { entered.countDown(); check(release.await(10, TimeUnit.SECONDS)) }
                return delegate.read(token)
            }
        } })
        runBlocking { f.store.confirm(p); f.store.put(com.anpfuel.domain.enterprise.EnterpriseLocalRecord(p, A, byteArrayOf(7))) }
        hold = true
        val executor = Executors.newFixedThreadPool(2)
        try {
            val read = executor.submit<Throwable?> { runCatching { runBlocking { f.store.record(p, A) } }.exceptionOrNull() }
            assertTrue(entered.await(5, TimeUnit.SECONDS))
            executor.submit { f.fence.markBackground() }.get(5, TimeUnit.SECONDS)
            release.countDown()
            assertTrue(read.get(5, TimeUnit.SECONDS) is EnterpriseLocalDenied)
        } finally { release.countDown(); executor.shutdownNow() }
    }

    @Test
    fun lateTrustedContactCannotUnlockAfterLogoutOrAChangedScope(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault"))
        val started = CompletableDeferred<Unit>()
        val finish = CompletableDeferred<Unit>()
        val store = ProtectedEnterprisePersistence(f.files, f.keys, f.fence, f.erasure, { f.identity }, { f.now },
            EnterpriseContactAuthority {
                started.complete(Unit); finish.await()
                EnterpriseContactGrant(p, A, 10000)
            })
        val confirmation = async {
            runCatching { store.confirm(p) }.exceptionOrNull()
        }
        started.await()
        f.fence.lock()
        finish.complete(Unit)
        assertTrue(confirmation.await() is EnterpriseLocalDenied)
        assertTrue(f.keys.values.isEmpty())
        val wrong = ProtectedEnterprisePersistence(f.files, f.keys, f.fence, f.erasure, { f.identity }, { f.now },
            EnterpriseContactAuthority { EnterpriseContactGrant(partition(org = B), A, 10000) })
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { wrong.confirm(p) } }
        val noProvider = ProtectedEnterprisePersistence(f.files, f.keys, f.fence, f.erasure, { f.identity }, { f.now }, UnavailableEnterpriseContact)
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { noProvider.confirm(p) } }
    }

    @Test
    fun eachIndependentCleanupFailureStaysDeniedAfterRecreationAndReportsIncompleteErasure(): Unit = runBlocking {
        for (failure in listOf("marker", "key", "files")) {
            val directory = root.resolve(failure)
            var fail = false
            val f = VaultFixture(directory, transform = { delegate -> object : EnterpriseVaultFiles by delegate {
                override fun deny(token: String) { if (fail && failure == "marker") error("private marker failure"); delegate.deny(token) }
                override fun erase(token: String) { if (fail && failure == "files") error("private unlink failure"); delegate.erase(token) }
            } })
            f.store.confirm(p)
            f.store.enqueue(EnterprisePendingOperation(p, A, B, byteArrayOf(7)))
            fail = true
            f.keys.failDelete = failure == "key"
            assertFalse(f.erasure.partitionRevoked(p))
            assertTrue(f.erasure.cleanupRequired)
            assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { f.store.pending(p, 1) } }
            val recreated = VaultFixture(directory, f.keys)
            assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { recreated.store.pending(p, 1) } }
            fail = false; f.keys.failDelete = false
            assertTrue(f.erasure.partitionRevoked(p))
            assertFalse(f.erasure.cleanupRequired)
            assertTrue(f.keys.values.isEmpty())
        }
    }

    @Test
    fun lockAndCompleteCleanupOutagesStillFenceCurrentProcessAndNeverClaimErasure(): Unit = runBlocking {
        var fail = false
        val f = VaultFixture(root.resolve("vault"), transform = { delegate -> object : EnterpriseVaultFiles by delegate {
            override fun <T> locked(accountTag: String, work: () -> T): T {
                if (fail) error("private lock failure")
                return delegate.locked(accountTag, work)
            }
        } })
        f.store.confirm(p)
        f.keys.failDelete = true; fail = true
        val previous = f.fence.epoch
        assertFalse(f.erasure.partitionRevoked(p))
        assertTrue(f.fence.epoch > previous)
        assertTrue(f.erasure.cleanupRequired)
        assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking { f.store.pending(p, 1) } }
        fail = false
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { f.store.pending(p, 1) } }
        assertFalse(f.erasure.accountRevoked(A, "http://127.0.0.1"))
    }

    @Test
    fun accountLockOutageStillAttemptsEveryScopedKeyDespiteOneDeletionFailure(): Unit = runBlocking {
        var fail = false
        val f = VaultFixture(root.resolve("vault"), transform = { delegate -> object : EnterpriseVaultFiles by delegate {
            override fun <T> locked(accountTag: String, work: () -> T): T {
                if (fail) error("private filesystem-lock failure")
                return delegate.locked(accountTag, work)
            }
        } })
        f.store.confirm(p)
        f.store.confirm(partition(org = B))
        val first = f.keys.values.keys.first()
        f.keys.failAliases = setOf(first)
        fail = true
        assertFalse(f.erasure.accountRevoked(A, ORIGIN))
        assertEquals(setOf(first), f.keys.values.keys)
        assertTrue(f.erasure.cleanupRequired)
    }

    @Test
    fun definitiveCurrentServerRevocationErasesOnlyThatAccountAndOrigin(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault"))
        f.store.confirm(p); f.store.put(EnterpriseLocalRecord(p, A, byteArrayOf(1)))
        val other = partition(owner = B)
        f.identity = EnterpriseLocalIdentity(B, ORIGIN, A)
        f.store.confirm(other); f.store.put(EnterpriseLocalRecord(other, A, byteArrayOf(2)))
        f.identity = EnterpriseLocalIdentity(A, ORIGIN, A)
        val raw = Sessions()
        val api = mockk<AuthAccountApi>()
        val wrapped = EnterpriseRevokingAccountApi(api, raw, ORIGIN, f.erasure)
        for (verdict in listOf("unavailable", PortableAuth.Verdict.SESSION_EXPIRED, PortableAuth.Verdict.SESSION_INVALID)) {
            every { api.refresh(A, "synthetic-refresh") } returns AuthApiResult.Err(verdict)
            assertEquals(AuthApiResult.Err(verdict), wrapped.refresh(A, "synthetic-refresh"))
            assertNotNull(f.store.record(p, A))
        }
        every { api.refresh(A, "synthetic-refresh") } returns AuthApiResult.Err(PortableAuth.Verdict.SESSION_REVOKED)
        assertEquals(AuthApiResult.Err(PortableAuth.Verdict.SESSION_REVOKED), wrapped.refresh(A, "synthetic-refresh"))
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { f.store.record(p, A) } }
        f.identity = EnterpriseLocalIdentity(B, ORIGIN, A)
        assertArrayEquals(byteArrayOf(2), f.store.record(other, A)!!.envelope)
        assertFalse(f.erasure.cleanupRequired)
    }

    @Test
    fun freshServerProofAndExplicitLossAcknowledgementCanStartEmptyAfterErasure(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault"))
        f.store.confirm(p)
        f.store.enqueue(EnterprisePendingOperation(p, A, B, byteArrayOf(7)))
        val oldKey = f.keys.values.values.single().secret.encoded
        assertTrue(f.erasure.accountRevoked(A, ORIGIN))
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { f.store.confirm(p) } }
        f.denyContact = true
        assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking { f.store.confirm(p, acknowledgeErasedWork = true) } }
        assertTrue(f.keys.values.isEmpty())
        f.denyContact = false
        f.identity = EnterpriseLocalIdentity(A, ORIGIN, B)
        f.server++
        f.store.confirm(p, acknowledgeErasedWork = true)
        assertTrue(f.store.pending(p, 50).isEmpty())
        assertFalse(oldKey.contentEquals(f.keys.values.values.single().secret.encoded))
        assertFalse(f.erasure.cleanupRequired)
    }

    @Test
    fun failedCleanupCannotBeBypassedByAcknowledgementOrFreshAuthority(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault"))
        f.store.confirm(p)
        f.keys.failDelete = true
        assertFalse(f.erasure.partitionRevoked(p))
        val creates = f.keys.creates
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { f.store.confirm(p, acknowledgeErasedWork = true) } }
        assertEquals(creates, f.keys.creates)
        assertTrue(f.erasure.cleanupRequired)
        f.keys.failDelete = false
        f.store.confirm(p, acknowledgeErasedWork = true)
        assertTrue(f.store.pending(p, 1).isEmpty())
        assertFalse(f.erasure.cleanupRequired)
    }

    @Test
    fun foreignRefreshOrLateRejectionCannotEraseNewlyLoggedInSessionWork(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault"))
        f.store.confirm(p)
        val raw = Sessions()
        val api = mockk<AuthAccountApi>()
        val wrapped = EnterpriseRevokingAccountApi(api, raw, ORIGIN, f.erasure)
        every { api.refresh(B, "foreign") } returns AuthApiResult.Err(PortableAuth.Verdict.SESSION_REVOKED)
        wrapped.refresh(B, "foreign")
        assertTrue(f.keys.values.isNotEmpty())
        every { api.refresh(A, "synthetic-refresh") } answers {
            raw.value = raw.value!!.copy(familyId = B)
            AuthApiResult.Err(PortableAuth.Verdict.SESSION_REUSE)
        }
        wrapped.refresh(A, "synthetic-refresh")
        assertTrue(f.keys.values.isNotEmpty())
    }
}
