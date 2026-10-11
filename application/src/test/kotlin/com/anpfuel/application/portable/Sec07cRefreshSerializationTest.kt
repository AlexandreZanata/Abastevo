package com.anpfuel.application.portable

import com.anpfuel.domain.portable.PortableAuth
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger
import java.util.concurrent.locks.ReentrantLock
import kotlin.concurrent.withLock
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * SEC07-C (AUD-08/09): Android refresh serialization. Concurrent
 * [AuthFlow.refreshSession] calls behind a mutex lock (the Android
 * [SerializedAuthOperations] shape) never overlap inside the account
 * API; one ambiguous reply costs exactly one API call (no transparent
 * replay of the refresh token); unavailable authority keeps the stored
 * session while permanent denials (incl. the SEC07-A session-invalid)
 * clear it; owner/account checks fail closed.
 */
private class SerialLock : AuthOperationLock {
    private val lock = ReentrantLock()
    override fun <T> withLock(operation: () -> T): T = lock.withLock(operation)
}

private class OverlapApi : AuthAccountApi {
    val refreshCalls = AtomicInteger(0)
    val revokeCalls = AtomicInteger(0)
    private val inside = AtomicInteger(0)
    val maxOverlap = AtomicInteger(0)
    var verdict: String? = null
    var accountOverride: String? = null
    var delayMs: Long = 0L
    // Synthetic contention mode: minted sessions still need refresh, so
    // every caller reaches the API and the lock carries the proof.
    var staleFresh: Boolean = false

    private fun <T> tracked(block: () -> T): T {
        val now = inside.incrementAndGet()
        maxOverlap.accumulateAndGet(now) { a, b -> maxOf(a, b) }
        try {
            if (delayMs > 0) Thread.sleep(delayMs)
            return block()
        } finally {
            inside.decrementAndGet()
        }
    }

    private fun fresh(): PortableAuth.Session = PortableAuth.Session(
        familyId = "family-1", accountId = accountOverride ?: "account-1",
        accessToken = "access-new", refreshToken = "refresh-new",
        accessExpiresAt = if (staleFresh) 1_000L else 9_999_999L,
        absoluteExpiresAt = 9_999_999L,
    )

    override fun requestCode(email: String): AuthApiResult<Unit> = AuthApiResult.Ok(Unit)
    override fun consumeCode(email: String, code: String): AuthApiResult<ConsumeOk> =
        AuthApiResult.Ok(ConsumeOk(fresh(), created = true))
    override fun createKeyAccount(username: String): AuthApiResult<AuthFlow.KeyIssued> =
        AuthApiResult.Ok(AuthFlow.KeyIssued(username, "key"))
    override fun loginWithKey(accountKey: String): AuthApiResult<KeyLogin> =
        AuthApiResult.Ok(KeyLogin(fresh(), "ana123"))

    override fun refresh(familyId: String, refreshToken: String): AuthApiResult<PortableAuth.Session> =
        tracked {
            refreshCalls.incrementAndGet()
            verdict?.let { return@tracked AuthApiResult.Err(it) }
            AuthApiResult.Ok(fresh())
        }

    override fun revokeAll(familyId: String, accessToken: String): AuthApiResult<Unit> =
        tracked {
            revokeCalls.incrementAndGet()
            AuthApiResult.Ok(Unit)
        }

    override fun linkProvider(
        session: PortableAuth.Session,
        provider: String,
        idToken: String,
        nonce: String,
    ): AuthApiResult<Unit> = AuthApiResult.Ok(Unit)

    override fun deleteAccount(session: PortableAuth.Session): AuthApiResult<Unit> =
        AuthApiResult.Ok(Unit)
}

private class MapStore(var session: PortableAuth.Session?) : AuthSessionStore {
    var clears = 0
    override fun save(session: PortableAuth.Session) {
        this.session = session
    }
    override fun load(): PortableAuth.Session? = session
    override fun clear() {
        session = null
        clears++
    }
}

private class MapKeys : AuthKeyStore {
    var backup: AuthFlow.KeyBackup? = null
    override fun saveKey(backup: AuthFlow.KeyBackup) {
        this.backup = backup
    }
    override fun loadKey(): AuthFlow.KeyBackup? = backup
    override fun clearKey() {
        backup = null
    }
}

private fun expiredSession() = PortableAuth.Session(
    familyId = "family-1", accountId = "account-1",
    accessToken = "access-old", refreshToken = "refresh-old",
    accessExpiresAt = 1_000L, absoluteExpiresAt = 9_999_999L,
)

class Sec07cRefreshSerializationTest {

    private fun serialFlow(
        api: OverlapApi,
        store: MapStore,
        keys: MapKeys = MapKeys(),
        now: Long = 2_000_000L,
    ) = AuthFlow(AuthPorts(store, keys, AuthWallClock { now }, PortableNonceSource { "n" }, api, SerialLock()))

    @Test
    fun concurrentRefreshNeverOverlapsApi() {
        val api = OverlapApi().apply { delayMs = 50L; staleFresh = true }
        val store = MapStore(expiredSession())
        val flow = serialFlow(api, store)
        val gate = CountDownLatch(1)
        val done = CountDownLatch(8)
        val errors = java.util.Collections.synchronizedList(mutableListOf<Throwable>())
        repeat(8) {
            Thread {
                try {
                    gate.await(10, TimeUnit.SECONDS)
                    flow.refreshSession()
                } catch (t: Throwable) {
                    errors.add(t)
                } finally {
                    done.countDown()
                }
            }.start()
        }
        gate.countDown()
        assertTrue(done.await(30, TimeUnit.SECONDS), "workers must finish")
        assertTrue(errors.isEmpty(), "no worker may throw: $errors")
        assertEquals(1, api.maxOverlap.get(), "refresh API calls must serialize")
        assertEquals(8, api.refreshCalls.get(), "every caller gets its own single attempt")
    }

    @Test
    fun refreshAndLogoutNeverOverlapApi() {
        val api = OverlapApi().apply { delayMs = 30L }
        val store = MapStore(expiredSession())
        val flow = serialFlow(api, store)
        val gate = CountDownLatch(1)
        val done = CountDownLatch(4)
        repeat(2) {
            Thread {
                gate.await(10, TimeUnit.SECONDS)
                flow.refreshSession()
                done.countDown()
            }.start()
        }
        repeat(2) {
            Thread {
                gate.await(10, TimeUnit.SECONDS)
                flow.logout()
                done.countDown()
            }.start()
        }
        gate.countDown()
        assertTrue(done.await(30, TimeUnit.SECONDS), "workers must finish")
        assertEquals(1, api.maxOverlap.get(), "refresh and logout must serialize")
    }

    @Test
    fun ambiguousReplyCostsOneCallAndKeepsSession() {
        val api = OverlapApi().apply { verdict = PortableAuth.UNAVAILABLE }
        val store = MapStore(expiredSession())
        val flow = serialFlow(api, store)
        assertEquals(AuthApiResult.Err(PortableAuth.UNAVAILABLE), flow.refreshSession())
        assertEquals(1, api.refreshCalls.get(), "no transparent replay after ambiguity")
        assertEquals(expiredSession(), store.session, "stored session survives ambiguity")
        // An explicit second attempt is a new caller decision, not a replay.
        assertEquals(AuthApiResult.Err(PortableAuth.UNAVAILABLE), flow.refreshSession())
        assertEquals(2, api.refreshCalls.get())
    }

    @Test
    fun permanentDenialsClearLocalAuthority() {
        for (verdict in listOf(
            PortableAuth.Verdict.SESSION_REVOKED,
            PortableAuth.Verdict.SESSION_REUSE,
            PortableAuth.Verdict.SESSION_INVALID,
            PortableAuth.Verdict.ACCOUNT_SUSPENDED,
            PortableAuth.Verdict.ACCOUNT_DELETED,
            PortableAuth.Verdict.CODE_UNKNOWN,
        )) {
            val api = OverlapApi().apply { this.verdict = verdict }
            val store = MapStore(expiredSession())
            val keys = MapKeys()
            val flow = serialFlow(api, store, keys)
            assertEquals(AuthApiResult.Err(verdict), flow.refreshSession(), "verdict $verdict")
            assertNull(store.session, "verdict $verdict clears the session")
            assertNull(keys.backup, "verdict $verdict clears the key backup")
            assertEquals(1, api.refreshCalls.get(), "verdict $verdict costs one call")
        }
    }

    @Test
    fun foreignAccountRefreshClearsAndRefuses() {
        val api = OverlapApi().apply { accountOverride = "account-2" }
        val store = MapStore(expiredSession())
        val flow = serialFlow(api, store)
        assertEquals(AuthApiResult.Err(AuthFlow.CLIENT_INVALID), flow.refreshSession())
        assertNull(store.session)
    }
}
