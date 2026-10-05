package com.anpfuel.app.ui.stations

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

/**
 * P32-T02 — Claim status labels RED→GREEN.
 */
class ClaimStatusUiMapperTest {

    @Test
    fun `submitted stays pending review`() {
        assertEquals("Enviado · aguardando análise", ClaimStatusUiMapper.toUi(ClaimStatus.SUBMITTED).label)
    }

    @Test
    fun `needs info requests action`() {
        assertEquals("Ação necessária: enviar documentos", ClaimStatusUiMapper.toUi(ClaimStatus.NEEDS_INFO).label)
    }

    @Test
    fun `appealable offers recourse`() {
        assertEquals("Recurso disponível", ClaimStatusUiMapper.toUi(ClaimStatus.APPEALABLE).label)
    }
}
