package com.anpfuel.data.mapper

import com.anpfuel.domain.valueobject.GeoCoordinates
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class NominatimSearchResponseMapperTest {

    @Test
    fun parsesFirstResultCoordinates() {
        val coordinates = NominatimSearchResponseMapper.parse(
            """
            [
              {
                "lat": "-25.4293875",
                "lon": "-49.2718364",
                "display_name": "Rua XV de Novembro, Centro, Curitiba, Paraná, Brasil"
              }
            ]
            """.trimIndent(),
        )

        assertEquals(GeoCoordinates.of(-25.4293875, -49.2718364), coordinates)
    }

    @Test
    fun returnsNullForEmptyResultList() {
        assertNull(NominatimSearchResponseMapper.parse("[]"))
    }

    @Test
    fun returnsNullForMalformedPayload() {
        assertNull(NominatimSearchResponseMapper.parse("not json"))
        assertNull(NominatimSearchResponseMapper.parse("""[{"lat": "abc", "lon": "def"}]"""))
        assertNull(NominatimSearchResponseMapper.parse("""[{"lat": "200.0", "lon": "0.0"}]"""))
    }
}
