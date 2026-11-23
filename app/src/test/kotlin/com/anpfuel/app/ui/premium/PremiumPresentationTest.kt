package com.anpfuel.app.ui.premium

import com.anpfuel.app.R
import com.anpfuel.domain.premium.PremiumState
import com.anpfuel.domain.premium.StoreBillingPeriod
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

    @Test
    fun `every notice has its own message`() {
        val messages = PremiumNotice.entries.map(::premiumNoticeMessageRes)
        assertEquals(PremiumNotice.entries.size, messages.toSet().size)
        assertTrue(premiumNoticeIsError(PremiumNotice.PURCHASE_DENIED))
        assertFalse(premiumNoticeIsError(PremiumNotice.PURCHASE_PENDING))
    }

    @Test
    fun `purchase phases describe the running step`() {
        assertNull(premiumPurchasePhaseRes(PremiumPurchasePhase.IDLE))
        assertEquals(R.string.premium_purchase_launching, premiumPurchasePhaseRes(PremiumPurchasePhase.LAUNCHING))
        assertEquals(R.string.premium_purchase_verifying, premiumPurchasePhaseRes(PremiumPurchasePhase.VERIFYING))
        assertEquals(R.string.premium_purchase_restoring, premiumPurchasePhaseRes(PremiumPurchasePhase.RESTORING))
    }

    @Test
    fun `plan labels come from the store billing period, never from plan ids`() {
        val month = StoreBillingPeriod(StoreBillingPeriod.PeriodUnit.MONTH, 1)
        val year = StoreBillingPeriod(StoreBillingPeriod.PeriodUnit.YEAR, 1)
        val week = StoreBillingPeriod(StoreBillingPeriod.PeriodUnit.WEEK, 1)
        val quarter = StoreBillingPeriod(StoreBillingPeriod.PeriodUnit.MONTH, 3)
        val days = StoreBillingPeriod(StoreBillingPeriod.PeriodUnit.DAY, 3)
        assertEquals(PremiumPlanLabel(R.string.premium_plan_title_month), premiumPlanTitle(month))
        assertEquals(PremiumPlanLabel(R.string.premium_plan_title_year), premiumPlanTitle(year))
        assertEquals(PremiumPlanLabel(R.string.premium_plan_title_week), premiumPlanTitle(week))
        assertEquals(PremiumPlanLabel(R.string.premium_plan_title_months, 3), premiumPlanTitle(quarter))
        assertEquals(PremiumPlanLabel(R.string.premium_plan_title_other), premiumPlanTitle(days))
        assertEquals(PremiumPlanLabel(R.string.premium_plan_title_other), premiumPlanTitle(null))
        assertEquals(PremiumPlanLabel(R.string.premium_plan_price_month), premiumPlanPrice(month))
        assertEquals(PremiumPlanLabel(R.string.premium_plan_price_year), premiumPlanPrice(year))
        assertEquals(PremiumPlanLabel(R.string.premium_plan_price_week), premiumPlanPrice(week))
        assertEquals(PremiumPlanLabel(R.string.premium_plan_price_every_months, 3), premiumPlanPrice(quarter))
        assertEquals(PremiumPlanLabel(R.string.premium_plan_price_other), premiumPlanPrice(days))
        assertEquals(PremiumPlanLabel(R.string.premium_plan_price_other), premiumPlanPrice(null))
    }
}
