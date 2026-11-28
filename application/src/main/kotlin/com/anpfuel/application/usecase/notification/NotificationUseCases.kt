package com.anpfuel.application.usecase.notification

import com.anpfuel.domain.notification.AccountNotificationCache
import com.anpfuel.domain.notification.AccountNotificationGateway
import com.anpfuel.domain.notification.AccountNotificationShownStore
import com.anpfuel.domain.notification.AppNotification
import com.anpfuel.domain.notification.AppNotificationSource
import com.anpfuel.domain.notification.LocalNotificationInbox
import com.anpfuel.domain.notification.NotificationFeedRule
import com.anpfuel.domain.notification.SystemNotificationPoster
import kotlinx.coroutines.CancellationException

/** How fresh the account part of the feed is (B-BR-NOTIF-01). */
enum class AccountFeedState { SIGNED_OUT, FRESH, NOT_REFRESHED }

data class NotificationFeed(
    val items: List<AppNotification>,
    val unread: Int,
    val accountState: AccountFeedState,
)

/**
 * B-BR-NOTIF-01 — the Notifications screen list: local alerts plus, for a
 * signed account, the first account page. Offline (or on a server error) it
 * falls back to the last page read, marked as not refreshed.
 */
class LoadNotificationsUseCase(
    private val local: LocalNotificationInbox,
    private val account: AccountNotificationGateway,
    private val cache: AccountNotificationCache,
) {
    suspend operator fun invoke(): NotificationFeed {
        val localItems = local.list()
        val (accountItems, state) = try {
            val page = account.list(limit = PAGE_SIZE)
            if (page == null) {
                cache.clear()
                emptyList<AppNotification>() to AccountFeedState.SIGNED_OUT
            } else {
                cache.save(page.items)
                page.items to AccountFeedState.FRESH
            }
        } catch (cancelled: CancellationException) {
            throw cancelled
        } catch (_: Exception) {
            cache.load() to AccountFeedState.NOT_REFRESHED
        }
        val merged = NotificationFeedRule.merge(accountItems, localItems)
        return NotificationFeed(merged, NotificationFeedRule.unreadCount(merged), state)
    }

    companion object {
        const val PAGE_SIZE = 30
    }
}

/** Marks items read in the store that owns each one (B-BR-NOTIF-01/03/05). */
class MarkNotificationsReadUseCase(
    private val local: LocalNotificationInbox,
    private val account: AccountNotificationGateway,
    private val cache: AccountNotificationCache,
) {
    /** Marks [items]; an account failure leaves local reads applied and is rethrown. */
    suspend fun mark(items: Collection<AppNotification>) {
        val unread = items.filter { !it.read }
        val localIds = unread.filter { it.source == AppNotificationSource.LOCAL }.map { it.id }.toSet()
        val accountIds = unread.filter { it.source == AppNotificationSource.ACCOUNT }.map { it.id }.toSet()
        if (localIds.isNotEmpty()) local.markRead(localIds)
        if (accountIds.isNotEmpty()) {
            account.markRead(accountIds)
            cache.save(cache.load().map { if (it.id in accountIds) it.copy(read = true) else it })
        }
    }

    suspend fun markAll(hasAccountItems: Boolean) {
        local.markAllRead()
        if (hasAccountItems) {
            account.markAllRead()
            cache.save(cache.load().map { it.copy(read = true) })
        }
    }
}

/**
 * B-BR-NOTIF-04 — background delivery of account notifications without a push
 * SDK: read the first page, post each unread item not shown before (at most
 * three, then one summary) and remember what was posted.
 */
class SyncAccountNotificationsUseCase(
    private val account: AccountNotificationGateway,
    private val cache: AccountNotificationCache,
    private val shown: AccountNotificationShownStore,
    private val poster: SystemNotificationPoster,
) {
    /** Number of Android notifications posted; 0 when signed out or nothing is new. */
    suspend operator fun invoke(): Int {
        val page = account.list(limit = LoadNotificationsUseCase.PAGE_SIZE) ?: return 0
        cache.save(page.items)
        if (!poster.canPost()) return 0
        val (post, more) = NotificationFeedRule.toPost(page.items, shown.shownIds())
        post.forEach { poster.postAccount(it) }
        if (more > 0) poster.postAccountSummary(more)
        val fresh = page.items.filter { it.source == AppNotificationSource.ACCOUNT && !it.read }.map { it.id }
        if (fresh.isNotEmpty()) shown.remember(fresh)
        return post.size + if (more > 0) 1 else 0
    }
}
