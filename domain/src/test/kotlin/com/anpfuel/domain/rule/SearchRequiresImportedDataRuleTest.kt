package com.anpfuel.domain.rule

import com.anpfuel.domain.exception.DomainException
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class SearchRequiresImportedDataRuleTest {

    @Test
    fun zeroImportedWeeksFailsBr005() {
        assertFalse(SearchRequiresImportedDataRule.isSatisfied(0))
        assertThrows(DomainException::class.java) {
            SearchRequiresImportedDataRule.validate(0)
        }
    }

    @Test
    fun oneOrMoreImportedWeeksSatisfiesBr005() {
        assertTrue(SearchRequiresImportedDataRule.isSatisfied(1))
    }
}
