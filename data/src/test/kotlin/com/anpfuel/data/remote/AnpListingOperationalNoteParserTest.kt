package com.anpfuel.data.remote

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class AnpListingOperationalNoteParserTest {

    @Test
    fun parsesNotaParagraph() {
        val text =
            "NOTA: Os preços médios de Belo Horizonte não foram publicados entre 26/04/2026 e 16/05/2026."

        assertEquals(
            "Os preços médios de Belo Horizonte não foram publicados entre 26/04/2026 e 16/05/2026.",
            AnpListingOperationalNoteParser.parseOperationalNote(text),
        )
    }

    @Test
    fun parsesAvisoParagraphCaseInsensitive() {
        assertEquals(
            "Correção dos preços de Goiatuba.",
            AnpListingOperationalNoteParser.parseOperationalNote("Aviso: Correção dos preços de Goiatuba."),
        )
    }

    @Test
    fun returnsNullForRegularParagraph() {
        assertNull(
            AnpListingOperationalNoteParser.parseOperationalNote(
                "Preços médios semanais: Brasil, regiões, estados e municípios",
            ),
        )
    }
}
