package com.anpfuel.application.portable

/** Launches the native credential selector with a hashed nonce. */
fun interface GoogleSignInGateway {
    suspend fun request(nonceClaim: String): GoogleSignInResult
}

sealed interface GoogleSignInResult {
    class Selected(val idToken: String, val displayName: String?) : GoogleSignInResult
    data object Cancelled : GoogleSignInResult
    data object Unavailable : GoogleSignInResult
}
