package com.anpfuel.app.ui.stations

import com.anpfuel.domain.profile.ManagementAction
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

/**
 * P32-T03 — Management action labels RED→GREEN.
 */
class ManagementActionUiMapperTest {

    @Test
    fun `invite explains review limitation`() {
        val ui = ManagementActionUiMapper.toUi(ManagementAction.InviteManager)
        assertEquals("Convidar responsável", ui.label)
        assertEquals("Sujeito a análise independente", ui.limitation)
    }

    @Test
    fun `contest explains server authority`() {
        val ui = ManagementActionUiMapper.toUi(ManagementAction.ContestAccess)
        assertEquals("Contestar acesso", ui.label)
        assertEquals("Decisão do servidor", ui.limitation)
    }
}
