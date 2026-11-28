package com.anpfuel.app.location

import com.anpfuel.app.location.DeviceCityFollowGate.Companion.FOLLOW_INTERVAL_MILLIS
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/** B-BR-LOC-03 pacing. */
class DeviceCityFollowGateTest {

    @Test
    fun `asks for location once per process`() {
        val gate = DeviceCityFollowGate()

        assertTrue(gate.claimPermissionPrompt())
        assertFalse(gate.claimPermissionPrompt())
    }

    @Test
    fun `follows at most once per interval`() {
        val gate = DeviceCityFollowGate()

        assertTrue(gate.tryBeginFollow(1_000))
        assertFalse(gate.tryBeginFollow(1_000 + FOLLOW_INTERVAL_MILLIS - 1))
        assertTrue(gate.tryBeginFollow(1_000 + FOLLOW_INTERVAL_MILLIS))
    }

    @Test
    fun `a clock moved backwards starts a new interval`() {
        val gate = DeviceCityFollowGate()

        assertTrue(gate.tryBeginFollow(10_000_000))
        assertTrue(gate.tryBeginFollow(1_000))
    }

    @Test
    fun `undo or a manual choice stops following for the session`() {
        val gate = DeviceCityFollowGate()
        gate.suppressForSession()

        assertTrue(gate.isSuppressed)
        assertFalse(gate.tryBeginFollow(1_000))
        assertFalse(gate.tryBeginFollow(1_000 + FOLLOW_INTERVAL_MILLIS * 4))
    }
}
