package com.anpfuel.app.ui.auth

/** Memory-only native-auth grant. No account key or session token is stored here. */
class KeyUnlockSession {
    class Attempt internal constructor(val scope: String, val generation: Long)
    private var generation = 0L
    private var authorizedScope: String? = null
    private var pending: Attempt? = null

    @Synchronized fun isAuthorized(scope: String): Boolean = authorizedScope == scope
    @Synchronized fun begin(scope: String): Attempt {
        if (authorizedScope != scope) authorizedScope = null
        return Attempt(scope, ++generation).also { pending = it }
    }
    @Synchronized fun complete(attempt: Attempt, successful: Boolean): Boolean {
        if (pending != attempt || generation != attempt.generation) return false
        pending = null
        if (successful) authorizedScope = attempt.scope
        return successful
    }
    @Synchronized fun reset() { generation++; pending = null; authorizedScope = null }
}

/** Process lifetime only; explicitly cleared on close and account boundaries. */
internal object AccountKeyUnlockSession { val grant = KeyUnlockSession() }
