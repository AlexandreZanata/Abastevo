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
}
