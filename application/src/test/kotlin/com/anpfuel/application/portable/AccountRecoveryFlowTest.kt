package com.anpfuel.application.portable

import com.anpfuel.domain.portable.PortableAuth
import com.anpfuel.domain.portable.PortableAuth.CodePurpose
import io.mockk.every
import io.mockk.mockk
import io.mockk.verify
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class AccountRecoveryFlowTest {
    private val api = mockk<AuthAccountApi>()
    private val session = PortableAuth.Session("family", "account", "access", "refresh", 2000, 8000)
    private var storedSession: PortableAuth.Session? = null
    private var storedKey: AuthFlow.KeyBackup? = null
    private val flow = AuthFlow(
        AuthPorts(
            object : AuthSessionStore {
                override fun save(session: PortableAuth.Session) { storedSession = session }
                override fun load() = storedSession
                override fun clear() { storedSession = null }
            },
            object : AuthKeyStore {
                override fun saveKey(backup: AuthFlow.KeyBackup) { storedKey = backup }
                override fun loadKey() = storedKey
                override fun clearKey() { storedKey = null }
            },
            AuthWallClock { 1000 },
            PortableNonceSource { "unused" },
            api,
        ),
    )
    private val email = "test@example.invalid"
    private val name = "Example Person"
    private val code = "012345"
    private val rawNonce = "synthetic-raw-nonce"
    private val profile = PortableAuth.Profile("account", name, name, true, "t***@example.invalid", true, setOf("google"))

    @Test
    fun requestsPurposeAndLocaleButLocallyRejectsMalformedInput() {
        every { api.requestCode(email, CodePurpose.SIGNUP, "en") } returns AuthApiResult.Ok(Unit)
        assertEquals(AuthApiResult.Ok(Unit), flow.requestCode(" $email ", CodePurpose.SIGNUP, "en"))
        assertEquals(AuthApiResult.Err(AuthFlow.CLIENT_INVALID), flow.requestCode("bad@\nexample.invalid", CodePurpose.RECOVERY, "en"))
        verify(exactly = 1) { api.requestCode(any(), any(), any()) }
    }

    @Test
    fun signUpSavesIssuedKeyAndSessionWithoutLoggingInTwice() {
        every { api.signUp(name, email, code) } returns AuthApiResult.Ok(IssuedAccountKey(session, name, "new-key"))
        val result = flow.signUp(name, email, code) as AuthApiResult.Ok
        assertEquals("new-key", result.value.backup.accountKey)
        assertEquals(true, result.value.sessionSaved)
        assertEquals(session, storedSession)
        assertEquals(AuthFlow.KeyBackup(name, "new-key"), storedKey)
        verify(exactly = 0) { api.loginWithKey(any()) }
    }

    @Test
    fun recoveryReplacesAnOldBackupAndKeepsTheNewKeyAvailable() {
        storedKey = AuthFlow.KeyBackup("Old label", "old-key")
        every { api.recover(email, code) } returns AuthApiResult.Ok(IssuedAccountKey(session, name, "new-key"))
        val result = flow.recover(email, code) as AuthApiResult.Ok
        assertEquals(storedKey, result.value.backup)
        assertEquals("new-key", storedKey?.accountKey)
        assertEquals(session, storedSession)
    }

    @Test
    fun recoveryRefusalsDoNotReplaceTheBackup() {
        val backup = AuthFlow.KeyBackup(name, "old-key")
        storedKey = backup
        for (verdict in listOf("code-unknown", "code-consumed", "recovery-no-account", "account-suspended")) {
            every { api.recover(email, code) } returns AuthApiResult.Err(verdict)
            assertEquals(AuthApiResult.Err(verdict), flow.recover(email, code))
            assertEquals(backup, storedKey)
            assertNull(storedSession)
        }
    }

    @Test
    fun googleLoginPreservesProfileFlagAndDoesNotSwitchDuringReauthentication() {
        val google = GoogleLogin(session, created = true, profileComplete = false)
        every { api.loginWithGoogle("synthetic-id-token", rawNonce) } returns AuthApiResult.Ok(google)
        val current = session.copy(accountId = "different-account", familyId = "prior-family")
        storedSession = current
        assertEquals(AuthApiResult.Err(AuthFlow.CLIENT_INVALID), flow.loginWithGoogle("synthetic-id-token", rawNonce, current.accountId))
        assertEquals(current, storedSession)
        verify(exactly = 1) { api.loginWithGoogle("synthetic-id-token", rawNonce) }
        storedSession = null
        assertEquals(AuthApiResult.Ok(google), flow.loginWithGoogle("synthetic-id-token", rawNonce))
        assertEquals(session, storedSession)
    }

    @Test
    fun GoogleEmailConflictAndScopedProfileOperationsKeepVerdicts() {
        every { api.loginWithGoogle("synthetic-id-token", rawNonce) } returns AuthApiResult.Err("provider-email-in-use")
        assertEquals(AuthApiResult.Err("provider-email-in-use"), flow.loginWithGoogle("synthetic-id-token", rawNonce))
        storedSession = session
        every { api.profile(session) } returns AuthApiResult.Ok(profile)
        every { api.updateName(session, name) } returns AuthApiResult.Ok(profile)
        every { api.setRecoveryEmail(session, email, code) } returns AuthApiResult.Err("reauthentication-required")
        every { api.linkGoogle(session, "synthetic-id-token", rawNonce) } returns AuthApiResult.Err("link-cross-account-refused")
        assertEquals(AuthApiResult.Ok(profile), flow.profile())
        assertEquals(AuthApiResult.Ok(profile), flow.updateName(name))
        assertEquals(AuthApiResult.Err("reauthentication-required"), flow.setRecoveryEmail(email, code))
        assertEquals(AuthApiResult.Err("link-cross-account-refused"), flow.linkGoogle("synthetic-id-token", rawNonce))
    }
}
