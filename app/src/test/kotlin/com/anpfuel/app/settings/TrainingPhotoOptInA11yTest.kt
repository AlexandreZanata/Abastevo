package com.anpfuel.app.settings

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

/**
 * CS11C TalkBack descriptions: the photo label plus the opt-in
 * outcome and the consent gate, never a bare checkbox alone. Pure
 * for unit tests; large text scales through MaterialTheme
 * typography with no fixed heights in the row.
 */
class TrainingPhotoOptInA11yTest {

    @Test
    fun `allowed composes label and outcome`() {
        assertEquals(
            "Photo 1, allowed for training review, consent granted",
            trainingPhotoOptInTalkBackDescription("Photo 1", true, true),
        )
    }

    @Test
    fun `gate blocks without consent`() {
        assertEquals(
            "Photo 1, not allowed, needs consent first",
            trainingPhotoOptInTalkBackDescription("Photo 1", true, false),
        )
    }

    @Test
    fun `plain disallow`() {
        assertEquals(
            "Photo 1, not allowed",
            trainingPhotoOptInTalkBackDescription("Photo 1", false, true),
        )
    }

    @Test
    fun `blank label falls back`() {
        assertEquals(
            "Photo, not allowed",
            trainingPhotoOptInTalkBackDescription("  ", false, true),
        )
    }
}
