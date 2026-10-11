package com.anpfuel.app.settings

/**
 * CS11C — Per-photo training opt-in state (B-BR-CS06).
 *
 * Every photo starts disallowed. Checks apply only while server
 * consent is granted, so revocation (or a stale ON) can never
 * authorize selections; unchecks always apply. Clearing consent
 * wipes pending checks. Blank ids never store. Plain Kotlin with
 * a lock so review gestures from any thread converge; the future
 * queue producer reads [allowedIds] together with the current
 * server consent, which stays authoritative at commit time.
 */
class TrainingPhotoOptInState {

    private val lock = Any()
    private val allowed = mutableSetOf<String>()

    fun isAllowed(photoId: String): Boolean = synchronized(lock) {
        allowed.contains(photoId.trim())
    }

    /**
     * Records one photo decision. Returns false (storing nothing)
     * for blank ids and for checks without granted consent.
     */
    fun setAllowed(photoId: String, allowed: Boolean, consentGranted: Boolean): Boolean {
        val id = photoId.trim()
        if (id.isEmpty()) {
            return false
        }
        if (allowed && !consentGranted) {
            return false
        }
        synchronized(lock) {
            if (allowed) {
                this.allowed.add(id)
            } else {
                this.allowed.remove(id)
            }
        }
        return true
    }

    fun allowedIds(): Set<String> = synchronized(lock) {
        allowed.toSet()
    }

    fun onConsentCleared() = synchronized(lock) {
        allowed.clear()
    }
}
