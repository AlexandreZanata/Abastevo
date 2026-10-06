package com.anpfuel.app.ui.auth

import android.content.Context
import androidx.activity.ComponentActivity
import androidx.compose.runtime.remember
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performImeAction
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
                model = remember { AuthViewModel(flow).also { holder.put("auth", it) } }
                AnpFuelTheme(dynamicColor = false) {
                    AuthRoute(onNavigateBack = { navigated = true }, viewModel = model)
                }
            }
            compose.waitUntil(10_000L) { model.uiState.value.step == AuthStep.USERNAME_ENTRY }
            val username = compose.onNodeWithText(compose.activity.getString(R.string.auth_username_label))
            username.performScrollTo().performTextInput("testuser")
            username.performImeAction()
            compose.waitUntil(10_000L) { model.uiState.value.step == AuthStep.KEY_ISSUED }
            assertNull(model.uiState.value.error)
            assertNotNull(flow.currentSession())
            assertEquals("testuser", flow.currentKey()?.username)
            compose.onNodeWithText(compose.activity.getString(R.string.auth_saved_enter)).performScrollTo().performClick()
            compose.waitUntil(10_000L) { navigated }
            assertEquals(AuthStep.AUTHENTICATED, model.uiState.value.step)
            assertEquals(1, api.loginCalls)
            assertTrue(restoredFlow().rehydrate() is AuthFlow.AuthState.Active)
            compose.onNodeWithText(compose.activity.getString(R.string.auth_logout)).performScrollTo().performClick()
            compose.waitUntil(10_000L) { model.uiState.value.step == AuthStep.USERNAME_ENTRY }
            assertNull(flow.currentKey())
            assertNull(flow.currentSession())
            assertEquals(AuthFlow.AuthState.LoggedOut, restoredFlow().rehydrate())
        } finally {
            compose.runOnUiThread { holder.clear() }
            prefs.edit().clear().commit()
            KeyStore.getInstance("AndroidKeyStore").apply { load(null) }.also {
                it.deleteEntry(keyAlias)
                it.deleteEntry(sessionAlias)
            }
        }
    }

    private class SyntheticAccountApi : AuthAccountApi {
        var loginCalls = 0
        private val session = PortableAuth.Session("family-test", "account-test", "access-test", "refresh-test", 1000L, 2000L)
        override fun createKeyAccount(username: String) = AuthApiResult.Ok(AuthFlow.KeyIssued(username, "synthetic-only-key"))
        override fun loginWithKey(accountKey: String): AuthApiResult<KeyLogin> {
            loginCalls++
            return if (accountKey == "synthetic-only-key") AuthApiResult.Ok(KeyLogin(session, "testuser"))
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
