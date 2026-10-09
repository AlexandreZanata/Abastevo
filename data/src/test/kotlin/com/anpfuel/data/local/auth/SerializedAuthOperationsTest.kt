package com.anpfuel.data.local.auth

import com.anpfuel.application.portable.*
import com.anpfuel.domain.portable.PortableAuth
import io.mockk.every
import io.mockk.mockk
import io.mockk.verify
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicReference
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class SerializedAuthOperationsTest {
    private val initial = PortableAuth.Session("family-1", "account-1", "access-1", "refresh-1", 100L, 10_000L)
    private val rotated = initial.copy(accessToken = "access-2", refreshToken = "refresh-2", accessExpiresAt = 2_000L)

    private class Store(initial: PortableAuth.Session) : AuthSessionStore {
        val value = AtomicReference<PortableAuth.Session?>(initial)
        override fun load() = value.get()
        override fun save(session: PortableAuth.Session) { value.set(session) }
        override fun clear() { value.set(null) }
    }

    private class Keys : AuthKeyStore {
        val value = AtomicReference<AuthFlow.KeyBackup?>(AuthFlow.KeyBackup("ana123", "synthetic-key"))
        override fun loadKey() = value.get()
        override fun saveKey(backup: AuthFlow.KeyBackup) { value.set(backup) }
        override fun clearKey() { value.set(null) }
    }

    @Test
    fun concurrentRefreshesRotateOnlyOnce() {
        exercise(logout = false)
    }

    @Test
    fun logoutDuringRefreshCannotResurrectSessionOrKey() {
        exercise(logout = true)
    }

    private fun exercise(logout: Boolean) {
        val store = Store(initial)
        val keys = Keys()
        val api = mockk<AuthAccountApi>()
        val entered = CountDownLatch(1)
        val release = CountDownLatch(1)
        val secondStarted = CountDownLatch(1)
        every { api.refresh("family-1", "refresh-1") } answers {
            entered.countDown()
            check(release.await(5, TimeUnit.SECONDS))
            AuthApiResult.Ok(rotated)
        }
        every { api.revokeAll(any(), any()) } returns AuthApiResult.Err(PortableAuth.UNAVAILABLE)
        val flow = AuthFlow(AuthPorts(store, keys, AuthWallClock { 1_000L }, PortableNonceSource { "nonce" }, api,
            SerializedAuthOperations()))
        val pool = Executors.newFixedThreadPool(2)
        try {
            val first = pool.submit<AuthApiResult<PortableAuth.Session?>> { flow.refreshSession() }
            assertTrue(entered.await(5, TimeUnit.SECONDS))
            val second = pool.submit<Any> {
                secondStarted.countDown()
                if (logout) flow.logout() else flow.refreshSession()
            }
            assertTrue(secondStarted.await(5, TimeUnit.SECONDS))
            release.countDown()
            assertEquals(AuthApiResult.Ok(rotated), first.get(5, TimeUnit.SECONDS))
            second.get(5, TimeUnit.SECONDS)
            verify(exactly = 1) { api.refresh(any(), any()) }
            if (logout) {
                assertNull(store.load())
                assertNull(keys.loadKey())
                assertEquals(AuthApiResult.Ok<PortableAuth.Session?>(null), flow.refreshSession())
            } else {
                assertEquals(rotated, store.load())
            }
        } finally {
            release.countDown()
            pool.shutdownNow()
        }
    }
}
