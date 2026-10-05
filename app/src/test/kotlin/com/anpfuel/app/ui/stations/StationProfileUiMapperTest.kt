package com.anpfuel.app.ui.stations

import com.anpfuel.domain.profile.ProfileBadge
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Test

/**
 * P32-T01 — Profile badge presentation RED→GREEN.
 */
class StationProfileUiMapperTest {

    @Test
    fun `unclaimed label carries no price quality claim`() {
        val ui = StationProfileUiMapper.toUi(ProfileBadge.Unclaimed)
        assertEquals("Perfil não reivindicado", ui.label)
        assertFalse(ui.label.contains("preço", ignoreCase = true))
        assertFalse(ui.label.contains("combustível", ignoreCase = true))
    }

    @Test
    fun `pending label is honest about review`() {
        val ui = StationProfileUiMapper.toUi(ProfileBadge.PendingReview)
        assertEquals("Em análise", ui.label)
    }

    @Test
    fun `verified label names representation only`() {
        val ui = StationProfileUiMapper.toUi(ProfileBadge.Verified(source = "registry"))
        assertEquals("Representação verificada", ui.label)
        assertFalse(ui.label.contains("preço", ignoreCase = true))
    }

    @Test
    fun `stale verification never implies privilege`() {
        val ui = StationProfileUiMapper.toUi(ProfileBadge.StaleUnverified)
        assertEquals("Verificação desatualizada", ui.label)
    }
}
