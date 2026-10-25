package com.anpfuel.app.ui.premium

import com.anpfuel.app.R
import com.anpfuel.domain.premium.PremiumState
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class PremiumPresentationTest {

    @Test
    fun `every verified state has its own label`() {
        val labels = PremiumState.entries.map(::premiumStateLabelRes)
        assertEquals(PremiumState.entries.size, labels.toSet().size)
        assertEquals(R.string.premium_status_hold, premiumStateLabelRes(PremiumState.HOLD))
        assertEquals(R.string.premium_status_paused, premiumStateLabelRes(PremiumState.PAUSED))
        assertEquals(R.string.premium_status_none, premiumStateLabelRes(PremiumState.NONE))
        assertEquals(R.string.premium_status_unknown, premiumStateLabelRes(null))
    }

    @Test
    fun `access until is only shown for states that keep paid access`() {
        val date = "2026-11-30T23:59:59Z"
        assertTrue(premiumShowsAccessUntil(PremiumState.ACTIVE, date))
        assertTrue(premiumShowsAccessUntil(PremiumState.CANCELED, date))
        assertTrue(premiumShowsAccessUntil(PremiumState.GRACE, date))
        PremiumState.entries
            .filterNot { it in setOf(PremiumState.ACTIVE, PremiumState.CANCELED, PremiumState.GRACE) }
            .forEach { state ->
                assertFalse(premiumShowsAccessUntil(state, date), "$state must not imply access")
            }
        assertFalse(premiumShowsAccessUntil(PremiumState.ACTIVE, null))
    }

    @Test
    fun `unknown capabilities never expose technical identifiers`() {
        assertEquals(R.string.premium_capability_cloud_sync, premiumCapabilityLabelRes("cloud.sync"))
        assertNull(premiumCapabilityLabelRes("invented.capability"))
        assertNull(premiumCapabilityLabelRes(""))
    }

    @Test
    fun `known capabilities keep a deterministic presentation order`() {
        assertEquals(
            listOf(
                R.string.premium_capability_vehicles,
                R.string.premium_capability_cloud_sync,
            ),
            premiumCapabilityLabels(setOf("cloud.sync", "vehicles.multiple")),
        )
    }

    @Test
    fun `store failure has a recoverable notice message`() {
        assertEquals(
            R.string.premium_store_failed,
            premiumNoticeMessageRes(PremiumNotice.STORE_FAILED),
        )
    }
}
