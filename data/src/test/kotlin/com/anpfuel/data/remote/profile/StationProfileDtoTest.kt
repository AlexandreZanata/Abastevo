package com.anpfuel.data.remote.profile

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

/**
 * P32-T01 — Profile DTO codec RED→GREEN.
 */
class StationProfileDtoTest {

    @Test
    fun `decode keeps only public business keys`() {
        val dto = StationProfileDto(
            stationId = "9f6d8a2e-3b4c-4d5e-8f90-1234567890ab",
            displayName = "Posto Exemplo",
            business = mapOf(
                "opening_hours" to "Seg–Sáb 06:00–22:00",
                "cpf" to "123.456.789-00",
                "password" to "hunter2",
            ),
        )
        val profile = dto.toDomain()
        assertEquals(
            mapOf("opening_hours" to "Seg–Sáb 06:00–22:00"),
            profile?.business,
        )
    }

    @Test
    fun `blank station id decodes to null`() {
        val dto = StationProfileDto(
            stationId = "  ",
            displayName = "Posto Exemplo",
        )
        assertNull(dto.toDomain())
    }
}
