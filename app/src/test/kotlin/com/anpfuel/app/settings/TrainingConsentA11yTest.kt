package com.anpfuel.app.settings

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

/**
 * CS11B TalkBack descriptions: state plus the terms line and the
 * pending notice, never a bare toggle alone. Pure for unit tests;
 * large text scales through MaterialTheme typography with no fixed
 * heights in the panel.
 */
class TrainingConsentA11yTest {

    @Test
    fun `granted composes state and terms`() {
        assertEquals(
            "Improve price recognition: on, GRANTED, Terms training-terms-v1",
            trainingConsentTalkBackDescription(true, "GRANTED", "training-terms-v1", false),
        )
    }

    @Test
    fun `pending notice appended`() {
        assertEquals(
            "Improve price recognition: on, GRANTED, Terms training-terms-v1, Revocation pending",
            trainingConsentTalkBackDescription(true, "GRANTED", "training-terms-v1", true),
        )
    }

    @Test
    fun `off state without terms`() {
        assertEquals(
            "Improve price recognition: off",
            trainingConsentTalkBackDescription(false, null, null, false),
        )
    }

    @Test
    fun `blank state ignored`() {
        assertEquals(
            "Improve price recognition: off",
            trainingConsentTalkBackDescription(false, "  ", "  ", false),
        )
    }
}
