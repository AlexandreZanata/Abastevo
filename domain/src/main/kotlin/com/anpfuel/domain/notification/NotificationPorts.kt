package com.anpfuel.domain.notification

/** Device inbox of local alerts (B-BR-NOTIF-05); never leaves the device. */
interface LocalNotificationInbox {
    suspend fun list(): List<AppNotification>
    suspend fun add(notification: AppNotification)
    suspend fun markRead(ids: Set<String>)
    suspend fun markAllRead()
}

/** Account inbox on the server (B-BR-NOTIF-03). */
interface AccountNotificationGateway {
    /** Null when no account is signed in. */
    suspend fun list(limit: Int, before: String? = null): AccountNotificationPage?
    suspend fun markRead(ids: Set<String>)
    suspend fun markAllRead()
}

/** Last account page read, shown offline as not refreshed (B-BR-NOTIF-01). */
interface AccountNotificationCache {
    suspend fun load(): List<AppNotification>
    suspend fun save(items: List<AppNotification>)
    suspend fun clear()
}

/** Posts Android notifications (B-BR-NOTIF-04/05); every call is a no-op without the OS permission. */
interface SystemNotificationPoster {
    fun canPost(): Boolean
    suspend fun postAccount(notification: AppNotification)
    suspend fun postAccountSummary(moreCount: Int)
    suspend fun postLocal(notification: AppNotification)
}

/** Ids of account items already posted as Android notifications (B-BR-NOTIF-04). */
interface AccountNotificationShownStore {
    suspend fun shownIds(): Set<String>
    suspend fun remember(ids: Collection<String>)
}
