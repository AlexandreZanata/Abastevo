package com.anpfuel.application.portable

import com.anpfuel.domain.portable.PortableAuth
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

private class FakeStore : AuthSessionStore {
    var saved: PortableAuth.Session? = null
    var clears: Int = 0

    override fun save(session: PortableAuth.Session) {
        saved = session
    }

    override fun load(): PortableAuth.Session? = saved

    override fun clear() {
        saved = null
        clears += 1
    }
}

private class FakeClock(var now: Long) : AuthWallClock {
    override fun nowEpochSeconds(): Long = now
}

private class FakeKeys : AuthKeyStore {
    var saved: AuthFlow.KeyBackup? = null
    var clears: Int = 0

    override fun saveKey(backup: AuthFlow.KeyBackup) {
        saved = backup
    }

    override fun loadKey(): AuthFlow.KeyBackup? = saved

    override fun clearKey() {
        saved = null
        clears += 1
    }
}

private class FakeNonces(vararg values: String) : PortableNonceSource {
    private val queue = values.toMutableList()
    var calls: Int = 0

    override fun nextNonce(): String {
        calls += 1
        return queue.removeFirstOrNull() ?: "nonce-$calls"
    }
}

private class FakeApi : AuthAccountApi {
    var sessions: Int = 0
    var linkCalls: Int = 0
    var lastLinkedNonce: String? = null
    var verdict: String? = null

    private fun session(): PortableAuth.Session {
        sessions += 1
        return PortableAuth.Session(
            familyId = "family-$sessions", accountId = "account-1",
            accessToken = "access-$sessions", refreshToken = "refresh-$sessions",
            accessExpiresAt = 1_000_900L, absoluteExpiresAt = 4_259_200L,
        )
    }

    override fun requestCode(email: String): AuthApiResult<Unit> = AuthApiResult.Ok(Unit)

    override fun consumeCode(email: String, code: String): AuthApiResult<ConsumeOk> {
        verdict?.let { return AuthApiResult.Err(it) }
        return AuthApiResult.Ok(ConsumeOk(session(), created = sessions == 1))
    }

    override fun refresh(familyId: String, refreshToken: String): AuthApiResult<PortableAuth.Session> {
        verdict?.let { return AuthApiResult.Err(it) }
        return AuthApiResult.Ok(session())
    }

    override fun revokeAll(familyId: String, accessToken: String): AuthApiResult<Unit> {
        verdict?.let { return AuthApiResult.Err(it) }
        return AuthApiResult.Ok(Unit)
    }

    override fun linkProvider(
        session: PortableAuth.Session,
        provider: String,
        idToken: String,
        nonce: String,
    ): AuthApiResult<Unit> {
        linkCalls += 1
        lastLinkedNonce = nonce
        verdict?.let { return AuthApiResult.Err(it) }
        return AuthApiResult.Ok(Unit)
    }

    override fun deleteAccount(session: PortableAuth.Session): AuthApiResult<Unit> {
        verdict?.let { return AuthApiResult.Err(it) }
        return AuthApiResult.Ok(Unit)
    }

    override fun createKeyAccount(username: String): AuthApiResult<AuthFlow.KeyIssued> {
        verdict?.let { return AuthApiResult.Err(it) }
        return AuthApiResult.Ok(AuthFlow.KeyIssued(username, "key-for-$username"))
    }

    override fun loginWithKey(accountKey: String): AuthApiResult<KeyLogin> {
        verdict?.let { return AuthApiResult.Err(it) }
        return AuthApiResult.Ok(KeyLogin(session(), "ana123"))
    }
}

private fun flow(
    now: Long = 1_000_000L,
    nonces: FakeNonces = FakeNonces(),
    api: FakeApi = FakeApi(),
    store: FakeStore = FakeStore(),
    keys: FakeKeys = FakeKeys(),
): AuthFlow {
    return AuthFlow(AuthPorts(store, keys, FakeClock(now), nonces, api))
}

private fun signup(flow: AuthFlow, email: String = "case-01@example.invalid"): PortableAuth.Session {
    val login = flow.consumeEmailCode(email, "482916")
    check(login is AuthApiResult.Ok)
    return login.value.session
}

class PortableAuthFlowTest {

    @Test
    fun emailSignupPersistsSession() {
        val store = FakeStore()
        val api = FakeApi()
        val f = flow(store = store, api = api)
        val res = f.consumeEmailCode("case-01@example.invalid", "482916")
        check(res is AuthApiResult.Ok)
        assertTrue(res.value.created)
        assertEquals(res.value.session, store.saved)
        assertEquals(res.value.session, f.currentSession())
    }

    @Test
    fun malformedEmailAndCodeRefuseLocally() {
        val api = FakeApi()
        val f = flow(api = api)
        for (bad in listOf("", "not-an-address", "a@b c.invalid")) {
            val res = f.requestEmailCode(bad)
            check(res is AuthApiResult.Err)
            assertEquals(AuthFlow.CLIENT_INVALID, res.verdict)
        }
        val res = f.consumeEmailCode("case-01@example.invalid", "12")
        check(res is AuthApiResult.Err)
        assertEquals(AuthFlow.CLIENT_INVALID, res.verdict)
    }

    @Test
    fun serverVerdictsPassThroughVerbatim() {
        val api = FakeApi()
        val f = flow(api = api)
        api.verdict = PortableAuth.Verdict.CODE_UNKNOWN
        val res = f.consumeEmailCode("nobody@example.invalid", "000000")
        check(res is AuthApiResult.Err)
        assertEquals(PortableAuth.Verdict.CODE_UNKNOWN, res.verdict)
        api.verdict = PortableAuth.Verdict.ACCOUNT_SUSPENDED
        val res2 = f.consumeEmailCode("case-01@example.invalid", "482916")
        check(res2 is AuthApiResult.Err)
        assertEquals(PortableAuth.Verdict.ACCOUNT_SUSPENDED, res2.verdict)
    }

    @Test
    fun refreshRotatesWhenAccessExpires() {
        val store = FakeStore()
        val api = FakeApi()
        val f = flow(now = 1_000_000L, store = store, api = api)
        val first = signup(f)
        val rotated = f.refreshSession()
        check(rotated is AuthApiResult.Ok)
        assertEquals(first, rotated.value)
        val late = AuthFlow(AuthPorts(store, FakeKeys(), FakeClock(1_000_901L), FakeNonces(), api))
        val res = late.refreshSession()
        check(res is AuthApiResult.Ok)
        assertTrue(res.value != null && res.value != first)
        assertEquals(res.value, store.saved)
    }

    @Test
    fun deadSessionNeedsRelogin() {
        val store = FakeStore()
        val f = flow(now = 1_000_000L, store = store)
        signup(f)
        val late = AuthFlow(AuthPorts(store, FakeKeys(), FakeClock(9_999_999L), FakeNonces(), FakeApi()))
        assertNull(late.currentSession())
        val res = late.refreshSession()
        check(res is AuthApiResult.Ok)
        assertNull(res.value)
        assertNull(store.saved)
    }

    @Test
    fun logoutAlwaysClearsEvenOffline() {
        val store = FakeStore()
        val api = FakeApi()
        val f = flow(store = store, api = api)
        signup(f)
        api.verdict = "unavailable"
        val res = f.logout()
        check(res is AuthApiResult.Err)
        assertNull(store.saved)
        assertEquals(1, store.clears)
    }

    @Test
    fun rehydrateSurvivesProcessDeath() {
        val store = FakeStore()
        val early = flow(now = 1_000_000L, store = store)
        val session = signup(early)
        // New flow instance over the same store: process death simulation.
        val restarted = flow(now = 1_000_100L, store = store)
        val state = restarted.rehydrate()
        check(state is AuthFlow.AuthState.Active)
        assertEquals(session, state.session)
        val expired = AuthFlow(AuthPorts(store, FakeKeys(), FakeClock(1_000_901L), FakeNonces(), FakeApi()))
        val needsRefresh = expired.rehydrate()
        check(needsRefresh is AuthFlow.AuthState.NeedsRefresh)
        val dead = AuthFlow(AuthPorts(store, FakeKeys(), FakeClock(9_999_999L), FakeNonces(), FakeApi()))
        assertTrue(dead.rehydrate() is AuthFlow.AuthState.LoggedOut)
    }

    @Test
    fun rehydrateRefusesCorruptBlobs() {
        val store = FakeStore()
        store.save(
            PortableAuth.Session("", "", "", "", 9_999_999L, 9_999_999L),
        )
        val f = flow(store = store)
        assertTrue(f.rehydrate() is AuthFlow.AuthState.LoggedOut)
    }

    @Test
    fun providerLinkBindsNonceAndSendsOnce() {
        val api = FakeApi()
        val f = flow(api = api, nonces = FakeNonces("n-1", "s-1"))
        val session = signup(f)
        val attempt = f.beginProviderLogin("google")
        check(attempt is AuthApiResult.Ok)
        assertEquals("n-1", attempt.value.nonce)
        val callback = PortableAuth.ProviderCallback("google", "tok", "n-1", "s-1")
        val res = f.completeProviderLogin(session, attempt.value, callback)
        check(res is AuthApiResult.Ok)
        assertEquals(1, api.linkCalls)
        assertEquals("n-1", api.lastLinkedNonce)
        // Replay of the same callback refuses without touching the API.
        val replay = f.completeProviderLogin(session, attempt.value, callback)
        check(replay is AuthApiResult.Err)
        assertEquals(1, api.linkCalls)
    }

    @Test
    fun substitutedCallbackNeverReachesApi() {
        val api = FakeApi()
        val f = flow(api = api, nonces = FakeNonces("n-1", "s-1"))
        val session = signup(f)
        val attempt = f.beginProviderLogin("google")
        check(attempt is AuthApiResult.Ok)
        val swapped = PortableAuth.ProviderCallback("google", "tok", "n-2", "s-1")
        val res = f.completeProviderLogin(session, attempt.value, swapped)
        check(res is AuthApiResult.Err)
        assertEquals(AuthFlow.CLIENT_INVALID, res.verdict)
        assertEquals(0, api.linkCalls)
        val badProvider = f.beginProviderLogin("github")
        check(badProvider is AuthApiResult.Err)
    }

    @Test
    fun deleteAccountClearsLocal() {
        val store = FakeStore()
        val f = flow(store = store)
        signup(f)
        val res = f.deleteAccount()
        check(res is AuthApiResult.Ok)
        assertNull(store.saved)
        assertTrue(f.rehydrate() is AuthFlow.AuthState.LoggedOut)
    }

    @Test
    fun keySignupPersistsBackupWithoutSession() {
        val store = FakeStore()
        val keys = FakeKeys()
        val f = flow(store = store, keys = keys)
        val res = f.createKeyAccount("Ana123")
        check(res is AuthApiResult.Ok)
        assertEquals("ana123", res.value.username)
        assertEquals("key-for-ana123", res.value.accountKey)
        assertEquals(res.value.username, keys.saved?.username)
        assertEquals(res.value.accountKey, keys.saved?.accountKey)
        assertNull(store.saved)
        assertEquals(keys.saved, f.currentKey())
    }

    @Test
    fun malformedUsernamesRefuseLocally() {
        val f = flow()
        for (bad in listOf("", "ab", "1abc", "ana!", "ana sorriso", "averylongusernamethatexceeds")) {
            val res = f.createKeyAccount(bad)
            check(res is AuthApiResult.Err)
            assertEquals(AuthFlow.CLIENT_INVALID, res.verdict)
        }
        val blank = f.loginWithKey("   ")
        check(blank is AuthApiResult.Err)
        assertEquals(AuthFlow.CLIENT_INVALID, blank.verdict)
    }

    @Test
    fun keyLoginPersistsSessionAndBackup() {
        val store = FakeStore()
        val keys = FakeKeys()
        val f = flow(store = store, keys = keys)
        val res = f.loginWithKey("key-for-ana123")
        check(res is AuthApiResult.Ok)
        assertEquals(res.value.session, store.saved)
        assertEquals("ana123", keys.saved?.username)
        assertEquals("key-for-ana123", keys.saved?.accountKey)
    }

    @Test
    fun keyVerdictsPassThroughAndLogoutClearsKey() {
        val store = FakeStore()
        val keys = FakeKeys()
        val api = FakeApi()
        val f = flow(store = store, keys = keys, api = api)
        api.verdict = "key-invalid"
        val res = f.loginWithKey("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
        check(res is AuthApiResult.Err)
        assertEquals("key-invalid", res.verdict)
        assertNull(store.saved)
        api.verdict = null
        f.loginWithKey("key-for-ana123")
        f.logout()
        assertNull(store.saved)
        assertNull(keys.saved)
        assertEquals(1, keys.clears)
    }
}
