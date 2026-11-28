package com.anpfuel.application.usecase.notification

import com.anpfuel.domain.model.AveragePrice
import com.anpfuel.domain.model.PriceSurvey
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.notification.AccountNotificationCache
import com.anpfuel.domain.notification.AccountNotificationGateway
import com.anpfuel.domain.notification.AccountNotificationPage
import com.anpfuel.domain.notification.AccountNotificationShownStore
import com.anpfuel.domain.notification.AppNotification
import com.anpfuel.domain.notification.AppNotificationContent
import com.anpfuel.domain.notification.AppNotificationSource
import com.anpfuel.domain.notification.LocalNotificationInbox
import com.anpfuel.domain.notification.SystemNotificationPoster
import com.anpfuel.domain.repository.AveragePriceRepository
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.PriceAmount
import com.anpfuel.domain.valueobject.SurveyWeek
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import java.io.IOException
import java.time.Instant
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/** B-BR-NOTIF-01/03/04/05 orchestration. */
class NotificationUseCasesTest {

    private class MemoryInbox : LocalNotificationInbox {
        val items = mutableListOf<AppNotification>()
        override suspend fun list() = items.toList()
        override suspend fun add(notification: AppNotification) { items.removeAll { it.id == notification.id }; items += notification }
        override suspend fun markRead(ids: Set<String>) { items.replaceAll { if (it.id in ids) it.copy(read = true) else it } }
        override suspend fun markAllRead() { items.replaceAll { it.copy(read = true) } }
    }

    private class MemoryCache : AccountNotificationCache {
        var items = listOf<AppNotification>()
        override suspend fun load() = items
        override suspend fun save(items: List<AppNotification>) { this.items = items }
        override suspend fun clear() { items = emptyList() }
    }

    private fun account(id: String, at: Long, read: Boolean = false) = AppNotification(
        id, AppNotificationSource.ACCOUNT, AppNotificationContent.ForumReply("t", "Topic", "Ana"), at, read,
    )
    private fun local(id: String, at: Long) = AppNotification(
        id, AppNotificationSource.LOCAL, AppNotificationContent.NewSurveyWeek("2026-11-22", "2026-11-28"), at, false,
    )

    private val gateway = mockk<AccountNotificationGateway>(relaxed = true)
    private val inbox = MemoryInbox()
    private val cache = MemoryCache()

    @Test
    fun `feed merges a fresh account page with local alerts and caches the page`() = runTest {
        inbox.add(local("l1", 20))
        coEvery { gateway.list(any(), any()) } returns AccountNotificationPage(listOf(account("a1", 30), account("a2", 10, read = true)), null, 1)

        val feed = LoadNotificationsUseCase(inbox, gateway, cache)()

        assertEquals(listOf("a1", "l1", "a2"), feed.items.map { it.id })
        assertEquals(2, feed.unread)
        assertEquals(AccountFeedState.FRESH, feed.accountState)
        assertEquals(2, cache.items.size)
    }

    @Test
    fun `signed out shows only local alerts and drops the cached page`() = runTest {
        cache.items = listOf(account("old", 1))
        inbox.add(local("l1", 20))
        coEvery { gateway.list(any(), any()) } returns null

        val feed = LoadNotificationsUseCase(inbox, gateway, cache)()

        assertEquals(listOf("l1"), feed.items.map { it.id })
        assertEquals(AccountFeedState.SIGNED_OUT, feed.accountState)
        assertTrue(cache.items.isEmpty())
    }

    @Test
    fun `offline falls back to the last page read, marked not refreshed`() = runTest {
        cache.items = listOf(account("cached", 5))
        coEvery { gateway.list(any(), any()) } throws IOException("offline")

        val feed = LoadNotificationsUseCase(inbox, gateway, cache)()

        assertEquals(listOf("cached"), feed.items.map { it.id })
        assertEquals(AccountFeedState.NOT_REFRESHED, feed.accountState)
    }

    @Test
    fun `marking routes each item to its own store`() = runTest {
        inbox.add(local("l1", 20))
        cache.items = listOf(account("a1", 30))
        val mark = MarkNotificationsReadUseCase(inbox, gateway, cache)

        mark.mark(listOf(local("l1", 20), account("a1", 30), account("done", 1, read = true)))

        assertTrue(inbox.items.single().read)
        assertTrue(cache.items.single().read)
        coVerify(exactly = 1) { gateway.markRead(setOf("a1")) }
    }

    @Test
    fun `mark all skips the server when no account item is listed`() = runTest {
        inbox.add(local("l1", 20))
        MarkNotificationsReadUseCase(inbox, gateway, cache).markAll(hasAccountItems = false)

        assertTrue(inbox.items.single().read)
        coVerify(exactly = 0) { gateway.markAllRead() }
    }

    @Test
    fun `background sync posts only new unread items and remembers them`() = runTest {
        val poster = mockk<SystemNotificationPoster>(relaxed = true)
        every { poster.canPost() } returns true
        val shown = mockk<AccountNotificationShownStore>(relaxed = true)
        coEvery { shown.shownIds() } returns setOf("a1")
        coEvery { gateway.list(any(), any()) } returns AccountNotificationPage(
            listOf(account("a1", 9), account("a2", 8), account("a3", 7, read = true)), null, 2,
        )

        val posted = SyncAccountNotificationsUseCase(gateway, cache, shown, poster)()

        assertEquals(1, posted)
        coVerify(exactly = 1) { poster.postAccount(match { it.id == "a2" }) }
        coVerify(exactly = 0) { poster.postAccountSummary(any()) }
        coVerify { shown.remember(listOf("a1", "a2")) }
    }

    @Test
    fun `background sync posts nothing without permission or account`() = runTest {
        val poster = mockk<SystemNotificationPoster>(relaxed = true)
        every { poster.canPost() } returns false
        val shown = mockk<AccountNotificationShownStore>(relaxed = true)
        coEvery { gateway.list(any(), any()) } returns AccountNotificationPage(listOf(account("a1", 9)), null, 1)

        assertEquals(0, SyncAccountNotificationsUseCase(gateway, cache, shown, poster)())
        coEvery { gateway.list(any(), any()) } returns null
        assertEquals(0, SyncAccountNotificationsUseCase(gateway, cache, shown, poster)())
        coVerify(exactly = 0) { poster.postAccount(any()) }
    }

    // B-BR-NOTIF-05 weekly local alerts ---------------------------------------------------

    private val previous = SurveyWeek.fromIsoDates("2026-11-15", "2026-11-21")
    private val current = SurveyWeek.fromIsoDates("2026-11-22", "2026-11-28")
    private fun survey(week: SurveyWeek) = PriceSurvey.restore(DomainId.forSurveyWeek(week), week, Instant.parse("2026-11-28T10:00:00Z"), null)
    private fun average(week: SurveyWeek, fuel: FuelProduct, value: String) = AveragePrice.create(
        DomainId.forSurveyWeek(week), week, BrazilianState.MATO_GROSSO, "SORRISO", fuel, average = PriceAmount.of(value),
    )

    private class MemoryBaseline(var end: String? = null) : com.anpfuel.domain.notification.LocalAlertBaseline {
        override suspend fun newestWeekEnd() = end
        override suspend fun setNewestWeekEnd(endIso: String) { end = endIso }
    }

    private val baseline = MemoryBaseline()

    private fun weekly(surveys: List<PriceSurvey>, poster: SystemNotificationPoster): EvaluateWeeklyLocalAlertsUseCase {
        val tables = mockk<PriceTableRepository>()
        coEvery { tables.getImportedPriceSurveys() } returns surveys
        val prefs = mockk<UserPreferencesRepository>()
        coEvery { prefs.getPreferences() } returns UserPreferences(preferredState = BrazilianState.MATO_GROSSO, preferredMunicipality = "SORRISO")
        val averages = mockk<AveragePriceRepository>()
        coEvery { averages.getPricesByMunicipality(any(), any(), previous) } returns listOf(
            average(previous, FuelProduct.GASOLINE_REGULAR, "6.59"), average(previous, FuelProduct.ETHANOL, "4.20"),
        )
        coEvery { averages.getPricesByMunicipality(any(), any(), current) } returns listOf(
            average(current, FuelProduct.GASOLINE_REGULAR, "6.47"), average(current, FuelProduct.ETHANOL, "4.29"),
        )
        return EvaluateWeeklyLocalAlertsUseCase(averages, tables, prefs, inbox, poster, baseline) { 1_000L }
    }

    @Test
    fun `a newer week records the new week and the city drop once`() = runTest {
        val poster = mockk<SystemNotificationPoster>(relaxed = true)
        every { poster.canPost() } returns true
        baseline.end = previous.endDate.toString()
        val useCase = weekly(listOf(survey(previous), survey(current)), poster)

        assertEquals(2, useCase())
        assertEquals(current.endDate.toString(), baseline.end)
        val drop = inbox.items.first { it.content is AppNotificationContent.CityAverageDrop }.content as AppNotificationContent.CityAverageDrop
        assertEquals(listOf(FuelProduct.GASOLINE_REGULAR), drop.drops.map { it.fuel })
        assertEquals(120, drop.drops.single().dropMilli)
        coVerify(exactly = 2) { poster.postLocal(any()) }

        assertEquals(0, useCase())
        assertEquals(2, inbox.items.size)
    }

    @Test
    fun `the first import only sets the baseline and alerts stay in the inbox without permission`() = runTest {
        val poster = mockk<SystemNotificationPoster>(relaxed = true)
        every { poster.canPost() } returns false

        assertEquals(0, weekly(listOf(survey(previous)), poster)())
        assertEquals(previous.endDate.toString(), baseline.end)
        assertEquals(2, weekly(listOf(survey(previous), survey(current)), poster)())
        coVerify(exactly = 0) { poster.postLocal(any()) }
    }

    @Test
    fun `downloading an older week announces no new week but may report the city drop`() = runTest {
        val poster = mockk<SystemNotificationPoster>(relaxed = true)
        every { poster.canPost() } returns true
        baseline.end = current.endDate.toString()

        assertEquals(1, weekly(listOf(survey(previous), survey(current)), poster)())
        assertTrue(inbox.items.single().content is AppNotificationContent.CityAverageDrop)
        assertEquals(current.endDate.toString(), baseline.end)
    }
}
