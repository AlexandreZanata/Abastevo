package com.anpfuel.domain.rule

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.MunicipalityCatalogEntry
import com.anpfuel.domain.valueobject.SearchMatchType
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class MunicipalityCatalogCompletenessRuleTest {

    @Test
    fun catalogWithAtLeast5570EntriesIsComplete() {
        assertTrue(MunicipalityCatalogCompletenessRule.isCatalogComplete(5_570))
        assertTrue(MunicipalityCatalogCompletenessRule.isCatalogComplete(5_600))
    }

    @Test
    fun catalogBelow5570EntriesIsIncomplete() {
        assertFalse(MunicipalityCatalogCompletenessRule.isCatalogComplete(5_569))
    }

    @Test
    fun validateThrowsWhenCatalogIncomplete() {
        assertThrows(DomainException::class.java) {
            MunicipalityCatalogCompletenessRule.validate(100)
        }
    }
}
