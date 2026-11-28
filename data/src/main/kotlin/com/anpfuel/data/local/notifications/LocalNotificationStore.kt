package com.anpfuel.data.local.notifications

import android.content.Context
import android.content.SharedPreferences
import com.anpfuel.domain.notification.AccountNotificationCache
import com.anpfuel.domain.notification.AccountNotificationShownStore
import com.anpfuel.domain.notification.AppNotification
import com.anpfuel.domain.notification.LocalNotificationInbox
import com.anpfuel.domain.notification.NotificationFeedRule
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import javax.inject.Singleton
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext

/**
 * Device storage for B-BR-NOTIF-01/04/05: the local alert inbox (bounded to
 * 100 items and 30 days), the last account page read (shown offline) and the
 * ids of account items already posted. Private app storage; nothing here is
 * sent to the server.
 */
@Singleton
class LocalNotificationStore internal constructor(
    private val prefs: SharedPreferences,
    private val nowMillis: () -> Long,
) : LocalNotificationInbox, AccountNotificationCache, AccountNotificationShownStore {

    @Inject constructor(@ApplicationContext context: Context) :
        this(context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE), System::currentTimeMillis)

    private val mutex = Mutex()

    override suspend fun list(): List<AppNotification> = locked {
        NotificationFeedRule.boundLocal(NotificationJsonCodec.decodeList(prefs.getString(KEY_LOCAL, null)), nowMillis())
    }

    override suspend fun add(notification: AppNotification) = locked {
        val current = NotificationJsonCodec.decodeList(prefs.getString(KEY_LOCAL, null)).filterNot { it.id == notification.id }
        writeLocal(NotificationFeedRule.boundLocal(current + notification, nowMillis()))
    }

    override suspend fun markRead(ids: Set<String>) = locked {
        val current = NotificationJsonCodec.decodeList(prefs.getString(KEY_LOCAL, null))
        writeLocal(current.map { if (it.id in ids) it.copy(read = true) else it })
    }

    override suspend fun markAllRead() = locked {
        writeLocal(NotificationJsonCodec.decodeList(prefs.getString(KEY_LOCAL, null)).map { it.copy(read = true) })
    }

    override suspend fun load(): List<AppNotification> = locked {
        NotificationJsonCodec.decodeList(prefs.getString(KEY_ACCOUNT_CACHE, null))
    }

    override suspend fun save(items: List<AppNotification>) = locked {
        prefs.edit().putString(KEY_ACCOUNT_CACHE, NotificationJsonCodec.encode(items)).apply()
    }

    override suspend fun clear() = locked {
        prefs.edit().remove(KEY_ACCOUNT_CACHE).remove(KEY_SHOWN).apply()
    }

    override suspend fun remember(ids: Collection<String>) = locked {
        val current = prefs.getString(KEY_SHOWN, null).orEmpty().split(',').filter { it.isNotBlank() }
        val merged = (current.filterNot { it in ids } + ids).takeLast(NotificationFeedRule.SHOWN_IDS_MAX)
        prefs.edit().putString(KEY_SHOWN, merged.joinToString(",")).apply()
    }

    override suspend fun shownIds(): Set<String> = locked {
        prefs.getString(KEY_SHOWN, null).orEmpty().split(',').filter { it.isNotBlank() }.toSet()
    }

    private fun writeLocal(items: List<AppNotification>) {
        prefs.edit().putString(KEY_LOCAL, NotificationJsonCodec.encode(items)).apply()
    }

    private suspend fun <T> locked(block: () -> T): T = withContext(Dispatchers.IO) { mutex.withLock { block() } }

    companion object {
        const val PREFS_NAME = "notifications"
        private const val KEY_LOCAL = "local_inbox"
        private const val KEY_ACCOUNT_CACHE = "account_page"
        private const val KEY_SHOWN = "account_shown_ids"
    }
}
