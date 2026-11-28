package com.anpfuel.domain.notification

/**
 * B-BR-NOTIF-01/05 — how the Notifications screen orders and bounds items, and
 * when a weekly comparison becomes a local alert. Pure.
 */
object NotificationFeedRule {
    /** Local inbox bounds (B-BR-NOTIF-05). */
    const val LOCAL_MAX_ITEMS = 100
    const val LOCAL_MAX_AGE_MILLIS = 30L * 24 * 60 * 60 * 1000

    /** Device memory of account items already posted as Android notifications (B-BR-NOTIF-04). */
    const val SHOWN_IDS_MAX = 200
    const val SYSTEM_NOTIFICATIONS_PER_RUN = 3

    /** One list, newest first; an id appears once (the account copy wins over a stale cache). */
    fun merge(account: List<AppNotification>, local: List<AppNotification>): List<AppNotification> =
        (account + local)
            .distinctBy { it.source to it.id }
            .sortedWith(compareByDescending<AppNotification> { it.createdAtMillis }.thenBy { it.id })

    fun unreadCount(items: List<AppNotification>): Int = items.count { !it.read }

    /** Keeps the newest [LOCAL_MAX_ITEMS] local alerts younger than [LOCAL_MAX_AGE_MILLIS]. */
    fun boundLocal(items: List<AppNotification>, nowMillis: Long): List<AppNotification> =
        items
            .filter { nowMillis - it.createdAtMillis in 0..LOCAL_MAX_AGE_MILLIS }
            .sortedByDescending { it.createdAtMillis }
            .take(LOCAL_MAX_ITEMS)

    /**
     * Fuels whose city average fell from [previous] to [current] (same fuel,
     * milli-BRL), largest drop first. A fuel missing in either week is skipped.
     */
    fun cityAverageDrops(
        previous: Map<com.anpfuel.domain.valueobject.FuelProduct, Long>,
        current: Map<com.anpfuel.domain.valueobject.FuelProduct, Long>,
    ): List<FuelDrop> =
        current.mapNotNull { (fuel, now) ->
            val before = previous[fuel] ?: return@mapNotNull null
            if (now in 1 until before) FuelDrop(fuel, now, before - now) else null
        }.sortedWith(compareByDescending<FuelDrop> { it.dropMilli }.thenBy { it.fuel.ordinal })

    /** Account items to post now: unread, never shown, newest first, at most [SYSTEM_NOTIFICATIONS_PER_RUN]. */
    fun toPost(items: List<AppNotification>, shownIds: Set<String>): Pair<List<AppNotification>, Int> {
        val fresh = items.filter { it.source == AppNotificationSource.ACCOUNT && !it.read && it.id !in shownIds }
            .sortedByDescending { it.createdAtMillis }
        return fresh.take(SYSTEM_NOTIFICATIONS_PER_RUN) to (fresh.size - SYSTEM_NOTIFICATIONS_PER_RUN).coerceAtLeast(0)
    }
}
