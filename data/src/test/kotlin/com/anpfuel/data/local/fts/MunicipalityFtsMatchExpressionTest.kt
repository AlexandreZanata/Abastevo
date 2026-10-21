package com.anpfuel.data.local.fts

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class MunicipalityFtsMatchExpressionTest {

    @Test
    fun buildsPrefixTokensForSingleWordQuery() {
        assertEquals("CAMP*", MunicipalityFtsMatchExpression.fromUserQuery("camp"))
    }

    @Test
    fun buildsPrefixTokensForMultiWordQuery() {
        assertEquals("SAO* PAULO*", MunicipalityFtsMatchExpression.fromUserQuery("sao paulo"))
    }

    @Test
    fun stripsFtsSpecialCharacters() {
        assertEquals("SAO*", MunicipalityFtsMatchExpression.fromUserQuery("\"sao*\""))
    }

    @Test
    fun normalizesAccentedTokensForStableMatching() {
        // MEX-05: FTS4 simple tokenizer does not fold diacritics or case
        // above ASCII, so accented queries are normalized up front; the
        // accent-aware supplement pool covers the folded forms.
        assertEquals("SAO*", MunicipalityFtsMatchExpression.fromUserQuery("são"))
        assertEquals("SAO*", MunicipalityFtsMatchExpression.fromUserQuery("SÃO"))
        assertEquals("NITEROI*", MunicipalityFtsMatchExpression.fromUserQuery("Niterói"))
    }
}
