package com.anpfuel.app.settings

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * CS11C: per-photo opt-in state (B-BR-CS06).
 *
 * Every photo starts disallowed; checks apply only while server
 * consent is granted, so a stale ON can never authorize old
 * selections. Clearing consent wipes pending checks; guests hold
 * nothing.
 */
class TrainingPhotoOptInStateTest {

    @Test
    fun `photos start disallowed`() {
        val state = TrainingPhotoOptInState()
        assertFalse(state.isAllowed("photo-1"))
        assertEquals(emptySet<String>(), state.allowedIds())
    }

    @Test
    fun `check applies while consent granted`() {
        val state = TrainingPhotoOptInState()
        assertTrue(state.setAllowed("photo-1", true, consentGranted = true))
        assertTrue(state.isAllowed("photo-1"))
        assertFalse(state.isAllowed("photo-2"))
    }

    @Test
    fun `check refuses without granted consent`() {
        val state = TrainingPhotoOptInState()
        assertFalse(state.setAllowed("photo-1", true, consentGranted = false))
        assertFalse(state.isAllowed("photo-1"))
        assertEquals(emptySet<String>(), state.allowedIds())
    }

    @Test
    fun `uncheck always applies`() {
        val state = TrainingPhotoOptInState()
        state.setAllowed("photo-1", true, consentGranted = true)
        assertTrue(state.setAllowed("photo-1", false, consentGranted = true))
        assertTrue(state.setAllowed("photo-1", false, consentGranted = false))
        assertFalse(state.isAllowed("photo-1"))
    }

    @Test
    fun `clearing consent wipes checks`() {
        val state = TrainingPhotoOptInState()
        state.setAllowed("photo-1", true, consentGranted = true)
        state.setAllowed("photo-2", true, consentGranted = true)
        state.onConsentCleared()
        assertEquals(emptySet<String>(), state.allowedIds())
    }

    @Test
    fun `blank ids never stored`() {
        val state = TrainingPhotoOptInState()
        assertFalse(state.setAllowed("  ", true, consentGranted = true))
        assertFalse(state.isAllowed(""))
    }
}
