package com.anpfuel.domain.notification

import com.anpfuel.domain.valueobject.FuelProduct
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows

/** B-BR-NOTIF-01/04/05. */
class NotificationFeedRuleTest {

    private fun item(id: String, at: Long, source: AppNotificationSource = AppNotificationSource.LOCAL, read: Boolean = false) =
        AppNotification(id, source, AppNotificationContent.NewSurveyWeek("2026-11-22", "2026-11-28"), at, read)

    @Test
    fun `merges both sources newest first`() {
        val merged = NotificationFeedRule.merge(
            account = listOf(item("a1", 30, AppNotificationSource.ACCOUNT), item("a2", 10, AppNotificationSource.ACCOUNT)),
            local = listOf(item("l1", 20)),
        )
        assertEquals(listOf("a1", "l1", "a2"), merged.map { it.id })
    }

    @Test
    fun `an id shows once per source`() {
        val merged = NotificationFeedRule.merge(
            account = listOf(item("x", 5, AppNotificationSource.ACCOUNT, read = true), item("x", 5, AppNotificationSource.ACCOUNT)),
            local = listOf(item("x", 5)),
        )
        assertEquals(2, merged.size)
        assertTrue(merged.first { it.source == AppNotificationSource.ACCOUNT }.read)
    }

    @Test
    fun `counts unread`() {
        assertEquals(1, NotificationFeedRule.unreadCount(listOf(item("a", 1), item("b", 2, read = true))))
    }

    @Test
    fun `local inbox keeps the newest hundred within thirty days`() {
        val now = NotificationFeedRule.LOCAL_MAX_AGE_MILLIS * 2
        val old = item("old", now - NotificationFeedRule.LOCAL_MAX_AGE_MILLIS - 1)
        val fresh = (1..120).map { item("n$it", now - it) }
        val bounded = NotificationFeedRule.boundLocal(fresh + old, now)
        assertEquals(NotificationFeedRule.LOCAL_MAX_ITEMS, bounded.size)
        assertEquals("n1", bounded.first().id)
        assertTrue(bounded.none { it.id == "old" })
    }

    @Test
    fun `city drops list only fuels that got cheaper, largest drop first`() {
        val drops = NotificationFeedRule.cityAverageDrops(
            previous = mapOf(FuelProduct.GASOLINE_REGULAR to 6_590, FuelProduct.ETHANOL to 4_200, FuelProduct.DIESEL_S10 to 6_100),
            current = mapOf(FuelProduct.GASOLINE_REGULAR to 6_470, FuelProduct.ETHANOL to 4_190, FuelProduct.DIESEL_S10 to 6_150, FuelProduct.CNG to 4_000),
        )
        assertEquals(listOf(FuelProduct.GASOLINE_REGULAR, FuelProduct.ETHANOL), drops.map { it.fuel })
        assertEquals(120, drops.first().dropMilli)
        assertEquals(6_470, drops.first().currentMilli)
    }

    @Test
    fun `no drop without both weeks or with an unchanged price`() {
        assertTrue(NotificationFeedRule.cityAverageDrops(emptyMap(), mapOf(FuelProduct.ETHANOL to 4_000)).isEmpty())
        assertTrue(NotificationFeedRule.cityAverageDrops(mapOf(FuelProduct.ETHANOL to 4_000), mapOf(FuelProduct.ETHANOL to 4_000)).isEmpty())
    }

    @Test
    fun `a drop must be positive`() {
        assertThrows<IllegalArgumentException> { FuelDrop(FuelProduct.ETHANOL, 4_000, 0) }
    }

    @Test
    fun `posts at most three fresh unread account items and summarizes the rest`() {
        val items = (1..5).map { item("a$it", it.toLong(), AppNotificationSource.ACCOUNT) } +
            item("read", 10, AppNotificationSource.ACCOUNT, read = true) + item("local", 11)
        val (post, more) = NotificationFeedRule.toPost(items, shownIds = setOf("a5"))
        assertEquals(listOf("a4", "a3", "a2"), post.map { it.id })
        assertEquals(1, more)
    }
}
