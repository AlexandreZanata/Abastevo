package com.anpfuel.app.ui.auth

import android.content.Context
import androidx.activity.ComponentActivity
import androidx.compose.runtime.remember
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import androidx.lifecycle.ViewModelStore
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.app.R
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.application.portable.*
import com.anpfuel.data.local.auth.AndroidKeystoreKeys
import com.anpfuel.data.local.auth.KeystoreAccountKey
import com.anpfuel.data.local.auth.KeystoreSessionStore
import com.anpfuel.data.local.auth.SerializedAuthOperations
import com.anpfuel.domain.portable.PortableAuth
import java.security.KeyStore
import java.util.UUID
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** Real screen + portable flow + real Keystore. The server port is synthetic/offline. */
@RunWith(AndroidJUnit4::class)
class AuthAccountContinuityDeviceTest {
    @get:Rule
    val compose = createAndroidComposeRule<ComponentActivity>()

    @Test
    fun createContinueReopenAndExplicitLogoutKeepCredentialCustody() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        val scope = "signup-regression-" + UUID.randomUUID()
        val prefs = context.getSharedPreferences(scope, Context.MODE_PRIVATE)
        val sessionAlias = "$scope-session"
        val keyAlias = "$scope-key"
        val api = SyntheticAccountApi()
        val holder = ViewModelStore()
        val database=androidx.room.Room.inMemoryDatabaseBuilder(context,com.anpfuel.data.local.AnpFuelDatabase::class.java).build()
        val erasure=com.anpfuel.data.repository.OwnerLocalErasure(
            com.anpfuel.data.repository.RoomContributionOutboxRepository(database.contributionOutboxDao()),
            database.photoUploadSessionDao(),
            object:PhotoCache {
                override fun put(id:String,bytes:ByteArray,capturedAtMillis:Long)=error("unexpected photo")
                override fun get(id:String):ByteArray?=null
                override fun delete(id:String)=Unit
                override fun sweepExpired(nowMillis:Long)=0
            },
            com.anpfuel.application.port.ContributionScopeProvider {
                com.anpfuel.domain.model.ContributionScope("account:account-test:synthetic-key","https://test.invalid")
            },
        )
        fun restoredFlow() = AuthFlow(AuthPorts(
            KeystoreSessionStore(prefs, AndroidKeystoreKeys(), sessionAlias),
            KeystoreAccountKey(prefs, AndroidKeystoreKeys(), keyAlias),
            AuthWallClock { 100L }, PortableNonceSource { "synthetic-nonce" }, api, SerializedAuthOperations(),
        ))
        val flow = restoredFlow()
        lateinit var model: AuthViewModel
        var navigated = false
        try {
            compose.setContent {
                model = remember { AuthViewModel(flow,erasure).also { holder.put("auth", it) } }
                AnpFuelTheme(dynamicColor = false) {
                    AuthRoute(onNavigateBack = { navigated = true }, viewModel = model)
                }
            }
            compose.waitUntil(10_000L) { model.uiState.value.step == AuthStep.ENTRY }
            fun field(resource: Int) = compose.onNodeWithText(compose.activity.getString(resource))
            field(R.string.auth_create_account).performScrollTo().performClick()
            field(R.string.auth_public_name).performScrollTo().performTextInput("Test Person")
            field(R.string.auth_email_label).performScrollTo().performTextInput("test@example.invalid")
            field(R.string.auth_send_code).performScrollTo().performClick()
            compose.waitUntil(10_000L) { model.uiState.value.step == AuthStep.CODE_SENT }
            field(R.string.auth_code_label).performScrollTo().performTextInput("012345")
            field(R.string.auth_verify_code).performScrollTo().performClick()
            compose.waitUntil(10_000L) { model.uiState.value.step == AuthStep.KEY_ISSUED }
            assertNull(model.uiState.value.error)
            assertNotNull(flow.currentSession())
            assertEquals("Test Person", flow.currentKey()?.username)
            compose.onNodeWithText(compose.activity.getString(R.string.auth_saved_enter)).performScrollTo().performClick()
            compose.waitUntil(10_000L) { navigated }
            assertEquals(AuthStep.AUTHENTICATED, model.uiState.value.step)
            assertEquals(1, api.signupCalls)
            assertEquals(0, api.loginCalls)
            assertTrue(restoredFlow().rehydrate() is AuthFlow.AuthState.Active)
            compose.onNodeWithText(compose.activity.getString(R.string.auth_logout)).performScrollTo().performClick()
            compose.waitUntil(10_000L) { model.uiState.value.step == AuthStep.ENTRY }
            assertNull(flow.currentKey())
            assertNull(flow.currentSession())
            assertEquals(AuthFlow.AuthState.LoggedOut, restoredFlow().rehydrate())
        } finally {
            compose.runOnUiThread { holder.clear() }
            database.close()
            prefs.edit().clear().commit()
            KeyStore.getInstance("AndroidKeyStore").apply { load(null) }.also {
                it.deleteEntry(keyAlias)
                it.deleteEntry(sessionAlias)
            }
        }
    }

    private class SyntheticAccountApi : AuthAccountApi {
        var loginCalls = 0
        var signupCalls = 0
        private val session = PortableAuth.Session("family-test", "account-test", "access-test", "refresh-test", 1000L, 2000L)
        override fun createKeyAccount(username: String): AuthApiResult<AuthFlow.KeyIssued> = error("Unexpected legacy signup")
        override fun requestCode(email: String, purpose: PortableAuth.CodePurpose, locale: String): AuthApiResult<Unit> {
            assertEquals("test@example.invalid", email)
            assertEquals(PortableAuth.CodePurpose.SIGNUP, purpose)
            return AuthApiResult.Ok(Unit)
        }
        override fun signUp(name: String, email: String, code: String): AuthApiResult<IssuedAccountKey> {
            signupCalls++
            assertEquals("Test Person", name)
            assertEquals("test@example.invalid", email)
            assertEquals("012345", code)
            return AuthApiResult.Ok(IssuedAccountKey(session, name, "synthetic-only-key"))
        }
        override fun loginWithKey(accountKey: String): AuthApiResult<KeyLogin> {
            loginCalls++
            return if (accountKey == "synthetic-only-key") AuthApiResult.Ok(KeyLogin(session, "Test Person"))
            else AuthApiResult.Err("key-invalid")
        }
        override fun refresh(familyId: String, refreshToken: String) = AuthApiResult.Ok(session)
        // Explicit logout must clear disk custody even when server revocation is offline.
        override fun revokeAll(familyId: String, accessToken: String) = AuthApiResult.Err(PortableAuth.UNAVAILABLE)
        override fun requestCode(email: String): AuthApiResult<Unit> = error("Unexpected email ceremony")
        override fun consumeCode(email: String, code: String): AuthApiResult<ConsumeOk> = error("Unexpected email ceremony")
        override fun linkProvider(session: PortableAuth.Session, provider: String, idToken: String, nonce: String): AuthApiResult<Unit> = error("Unexpected provider ceremony")
        override fun deleteAccount(session: PortableAuth.Session): AuthApiResult<Unit> = error("Unexpected deletion")
    }
}
