package com.anpfuel.domain.portable

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class PortableFeedbackTest {

    @Test
    fun acceptsOneToFiveStars() {
        for (stars in 1..5) {
            assertTrue(PortableFeedback.isValidRating(stars))
        }
        assertFalse(PortableFeedback.isValidRating(0))
        assertFalse(PortableFeedback.isValidRating(6))
        assertFalse(PortableFeedback.isValidRating(-1))
    }

    @Test
    fun floorsAgreementBasisPoints() {
        assertEquals(6666L, PortableFeedback.agreementBasisPoints(2L, 1L))
        assertEquals(0L, PortableFeedback.agreementBasisPoints(0L, 3L))
        assertEquals(10000L, PortableFeedback.agreementBasisPoints(3L, 0L))
        assertEquals(5000L, PortableFeedback.agreementBasisPoints(1L, 1L))
    }

    @Test
    fun zeroVotesIsNullNeverZeroPercent() {
        assertNull(PortableFeedback.agreementBasisPoints(0L, 0L))
    }

    @Test
    fun negativeCountsThrow() {
        assertThrows(IllegalArgumentException::class.java) {
            PortableFeedback.agreementBasisPoints(-1L, 0L)
        }
    }

    @Test
    fun textLimitStaysShared() {
        assertEquals(280, PortableText.MAX_COMMENT_SCALARS)
        assertTrue(PortableText.isValidComment("x".repeat(280)))
        assertFalse(PortableText.isValidComment("x".repeat(281)))
    }
}
