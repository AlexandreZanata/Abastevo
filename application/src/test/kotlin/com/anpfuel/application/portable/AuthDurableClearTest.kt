package com.anpfuel.application.portable

import com.anpfuel.domain.portable.PortableAuth
import io.mockk.every
import io.mockk.mockk
import io.mockk.verify
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class AuthDurableClearTest {
    private class Store(var value: PortableAuth.Session?, var fail: Boolean) : AuthSessionStore {
        var clears = 0
        override fun load() = value
        override fun save(session: PortableAuth.Session) { value = session }
        override fun clear() { clears++; if (fail) error("private disk diagnostic"); value = null }
    }
    private class Keys(var fail: Boolean) : AuthKeyStore {
        var clears = 0
        override fun loadKey(): AuthFlow.KeyBackup? = null
        override fun saveKey(backup: AuthFlow.KeyBackup) = Unit
        override fun clearKey() { clears++; if (fail) error("private key diagnostic") }
    }
    private val session = PortableAuth.Session("family", "account", "synthetic-access", "synthetic-refresh", 900, 2000)
    private fun flow(store: Store, keys: Keys, api: AuthAccountApi) = AuthFlow(
        AuthPorts(store, keys, AuthWallClock { 1000 }, PortableNonceSource { "unused" }, api),
    )

    @Test
    fun logoutAttemptsBothLocalClearsAndRemoteRevocationWithoutFalseSuccess() {
        for ((failStore, failKeys) in listOf(true to false, false to true, true to true)) {
            val store = Store(session, failStore); val keys = Keys(failKeys)
            val api = mockk<AuthAccountApi>()
            every { api.revokeAll(session.familyId, session.accessToken) } returns AuthApiResult.Ok(Unit)
            val result = flow(store, keys, api).logout()
            assertEquals(AuthApiResult.Err(AuthFlow.SECURE_STORAGE_UNAVAILABLE), result)
            assertEquals(1, store.clears); assertEquals(1, keys.clears)
            verify(exactly = 1) { api.revokeAll(session.familyId, session.accessToken) }
        }
    }

    @Test
    fun missingSessionDoesNotHideFailedKeyRemoval() {
        val store = Store(null, false); val keys = Keys(true)
        val result = flow(store, keys, mockk()).logout()
        assertEquals(AuthApiResult.Err(AuthFlow.SECURE_STORAGE_UNAVAILABLE), result)
        assertEquals(1, keys.clears)
    }

    @Test
    fun revokedAndExpiredRefreshReportFailedClearWithoutThrowingOrGrantingNewAuthority() {
        for (expired in listOf(false, true)) {
            val store = Store(if (expired) session.copy(absoluteExpiresAt = 999) else session, true)
            val keys = Keys(false); val api = mockk<AuthAccountApi>()
            every { api.refresh(session.familyId, session.refreshToken) } returns AuthApiResult.Err(PortableAuth.Verdict.SESSION_REVOKED)
            val result = flow(store, keys, api).refreshSession()
            assertEquals(AuthApiResult.Err(AuthFlow.SECURE_STORAGE_UNAVAILABLE), result)
            assertEquals(1, store.clears); assertEquals(1, keys.clears)
        }
    }

    @Test
    fun foreignRefreshAndSuccessfulDeletionCannotMaskLocalCleanupFailure() {
        val store = Store(session, true); val keys = Keys(false); val api = mockk<AuthAccountApi>()
        every { api.refresh(session.familyId, session.refreshToken) } returns AuthApiResult.Ok(session.copy(accountId = "foreign"))
        assertEquals(AuthApiResult.Err(AuthFlow.SECURE_STORAGE_UNAVAILABLE), flow(store, keys, api).refreshSession())
        store.value = session.copy(accessExpiresAt = 2000)
        every { api.deleteAccount(any()) } returns AuthApiResult.Ok(Unit)
        assertEquals(AuthApiResult.Err(AuthFlow.SECURE_STORAGE_UNAVAILABLE), flow(store, keys, api).deleteAccount())
        assertEquals(2, store.clears); assertEquals(2, keys.clears)
    }
}
