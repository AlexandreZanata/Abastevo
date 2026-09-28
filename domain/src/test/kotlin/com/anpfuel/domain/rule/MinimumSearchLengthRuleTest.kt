package com.anpfuel.domain.rule

import com.anpfuel.domain.exception.DomainException
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class MinimumSearchLengthRuleTest {

    @Test
    fun queryShorterThanTwoCharactersIsRejected() {
        assertFalse(MinimumSearchLengthRule.isSearchAllowed("A"))
        assertThrows(DomainException::class.java) {
            MinimumSearchLengthRule.validate("A")
        }
    }

    @Test
    fun queryWithTwoOrMoreCharactersIsAllowed() {
        assertTrue(MinimumSearchLengthRule.isSearchAllowed("CU"))
    }
}
