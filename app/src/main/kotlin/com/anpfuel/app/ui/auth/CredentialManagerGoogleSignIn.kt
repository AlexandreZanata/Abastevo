package com.anpfuel.app.ui.auth

import android.app.Activity
import android.content.Context
import android.content.ContextWrapper
import androidx.credentials.ClearCredentialStateRequest
import androidx.credentials.CredentialManager
import androidx.credentials.exceptions.ClearCredentialException
import androidx.credentials.exceptions.GetCredentialCancellationException
import androidx.credentials.exceptions.GetCredentialException
import com.anpfuel.app.BuildConfig
import com.anpfuel.application.portable.GoogleSignInGateway
import com.anpfuel.application.portable.GoogleSignInResult
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject

class CredentialManagerGoogleSignIn(private val context: Context) : GoogleSignInGateway {
    override suspend fun request(nonceClaim: String): GoogleSignInResult {
        val activity = context.findActivity() ?: return GoogleSignInResult.Unavailable
        val clientId = BuildConfig.GOOGLE_SERVER_CLIENT_ID
        if (clientId.isBlank()) return GoogleSignInResult.Unavailable
        val request = GoogleIdentity.buildRequest(clientId, nonceClaim)
        return try {
            val credential = CredentialManager.create(activity).getCredential(activity, request).credential
            val selected = GoogleIdentity.parse(credential) ?: return GoogleSignInResult.Unavailable
            GoogleSignInResult.Selected(selected.idToken, selected.displayName)
        } catch (_: GetCredentialCancellationException) {
            GoogleSignInResult.Cancelled
        } catch (_: GetCredentialException) {
            GoogleSignInResult.Unavailable
        }
    }

    private tailrec fun Context.findActivity(): Activity? = when (this) {
        is Activity -> this
        is ContextWrapper -> baseContext.findActivity()
        else -> null
    }
}

class GoogleCredentialState @Inject constructor(@ApplicationContext private val context: Context) {
    suspend fun clear() {
        try {
            CredentialManager.create(context).clearCredentialState(ClearCredentialStateRequest())
        } catch (_: ClearCredentialException) {
            // Account logout and erasure are independent of OS credential providers.
        }
    }
}

class AuthProtectionPrompt @Inject constructor(@ApplicationContext context: Context) {
    private val preferences = context.getSharedPreferences("auth_protection_prompt", Context.MODE_PRIVATE)

    fun isDismissed(profileId: String): Boolean = preferences.getBoolean(profileId, false)

    fun dismiss(profileId: String) {
        preferences.edit().putBoolean(profileId, true).apply()
    }

    fun clear(profileId: String) {
        preferences.edit().remove(profileId).apply()
    }
}
