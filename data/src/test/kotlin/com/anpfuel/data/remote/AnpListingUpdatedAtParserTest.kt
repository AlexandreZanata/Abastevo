package com.anpfuel.data.remote

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test
import java.time.LocalDate

class AnpListingUpdatedAtParserTest {

    @Test
    fun parsesUpdatedAtSuffixFromLinkText() {
        val text =
            "Preços médios semanais: Brasil, regiões, estados e municípios (Atualizado em 12/6/2026)"

        assertEquals(LocalDate.of(2026, 6, 12), AnpListingUpdatedAtParser.parseUpdatedAt(text))
    }

    @Test
    fun parsesSingleDigitDayAndMonth() {
        val text = "Preços por posto revendedor (Atualizado em 1/2/2026)"

        assertEquals(LocalDate.of(2026, 2, 1), AnpListingUpdatedAtParser.parseUpdatedAt(text))
    }

    @Test
    fun returnsNullWhenSuffixMissing() {
        assertNull(
            AnpListingUpdatedAtParser.parseUpdatedAt(
                "Preços médios semanais: Brasil, regiões, estados e municípios",
            ),
        )
    }

    @Test
    fun returnsNullForInvalidDate() {
        assertNull(AnpListingUpdatedAtParser.parseUpdatedAt("(Atualizado em 32/13/2026)"))
    }
}
