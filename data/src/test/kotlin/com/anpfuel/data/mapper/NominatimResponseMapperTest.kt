package com.anpfuel.data.mapper

import com.anpfuel.domain.valueobject.BrazilianState
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class NominatimResponseMapperTest {

    @Test
    fun parsesBrazilianCityFromJsonV2Response() {
        val parsed = NominatimResponseMapper.parse(
            """
            {
              "display_name": "Curitiba, Paraná, Brasil",
              "address": {
                "city": "Curitiba",
                "state": "Paraná",
                "country_code": "br"
              }
            }
            """.trimIndent(),
        )

        assertNotNull(parsed)
        assertEquals(BrazilianState.PARANA, parsed?.state)
        assertEquals("Curitiba", parsed?.municipality)
    }

    @Test
    fun rejectsNonBrazilianResponses() {
        val parsed = NominatimResponseMapper.parse(
            """
            {
              "address": {
                "city": "Buenos Aires",
                "state": "Buenos Aires",
                "country_code": "ar"
              }
            }
            """.trimIndent(),
        )

        assertNull(parsed)
    }
}
