package com.anpfuel.app.ui.auth

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class KeyUnlockSessionTest {
    @Test fun `successful unlock covers subsequent actions for only that account`() {
        val session = KeyUnlockSession()
        assertFalse(session.isAuthorized("account-a"))
        assertTrue(session.complete(session.begin("account-a"), true))
        repeat(4) { assertTrue(session.isAuthorized("account-a")) }
        assertFalse(session.isAuthorized("account-b"))
    }
    @Test fun `cancelled authentication never authorizes`() {
        val session = KeyUnlockSession()
        assertFalse(session.complete(session.begin("account-a"), false))
        assertFalse(session.isAuthorized("account-a"))
    }
    @Test fun `close logout and late result cannot restore an old grant`() {
        val session = KeyUnlockSession()
        val pending = session.begin("account-a")
        session.reset()
        assertFalse(session.complete(pending, true))
        assertFalse(session.isAuthorized("account-a"))
        assertTrue(session.complete(session.begin("account-a"), true))
        session.reset()
        assertFalse(session.isAuthorized("account-a"))
        assertFalse(KeyUnlockSession().isAuthorized("account-a"))
    }
    @Test fun `overlapping and cross-account callbacks fail closed`() {
        val session = KeyUnlockSession()
        val old = session.begin("account-a")
        val current = session.begin("account-b")
        assertFalse(session.complete(old, true))
        assertTrue(session.complete(current, true))
        assertFalse(session.isAuthorized("account-a"))
        assertTrue(session.isAuthorized("account-b"))
        assertFalse(session.complete(current, true))
    }
}
