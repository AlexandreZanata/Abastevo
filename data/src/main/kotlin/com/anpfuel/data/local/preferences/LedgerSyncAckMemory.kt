package com.anpfuel.data.local.preferences

import android.content.Context
import android.content.SharedPreferences
import com.anpfuel.domain.repository.LedgerAck
import com.anpfuel.domain.repository.LedgerSyncAckStore
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import javax.inject.Singleton

/**
 * MEX-15 last-ack memory: last server acknowledgement per dataset for
 * honest sync status ("last receipt: revision R at time T").
 * Pending local edits are never advertised as backed up: this store
 * only moves on real server receipts.
 */
@Singleton
class LedgerSyncAckMemory internal constructor(
    private val readLong: (String, Long) -> Long,
    private val hasKey: (String) -> Boolean,
    private val writeAck: (String, Long, String, Long) -> Unit,
    private val removeKeys: (List<String>) -> Unit,
    private val allKeys: () -> Set<String>,
) : LedgerSyncAckStore {

    @Inject
    constructor(@ApplicationContext context: Context) : this(
        readLong = { key, def ->
            context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE).getLong(key, def)
        },
        hasKey = { key ->
            context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE).contains(key)
        },
        writeAck = { revisionKey, revision, timeKey, at ->
            val prefs: SharedPreferences =
                context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
            prefs.edit().putLong(revisionKey, revision).putLong(timeKey, at).apply()
        },
        removeKeys = { keys ->
            val prefs: SharedPreferences =
                context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
            val editor = prefs.edit()
            keys.forEach { editor.remove(it) }
            editor.apply()
        },
        allKeys = {
            context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE).all.keys
        },
    )

    override suspend fun recordAck(accountId: String, origin: String, dataset: String, revision: Long, nowMillis: Long) {
        if (accountId.isBlank() || dataset.isBlank() || revision < 0) return
        writeAck(revKey(accountId, origin, dataset), revision, atKey(accountId, origin, dataset), nowMillis)
    }

    override suspend fun lastAck(accountId: String, origin: String, dataset: String): LedgerAck? {
        if (accountId.isBlank() || dataset.isBlank()) return null
        if (!hasKey(revKey(accountId, origin, dataset)) || !hasKey(atKey(accountId, origin, dataset))) return null
        return LedgerAck(
            dataset = dataset,
            revision = readLong(revKey(accountId, origin, dataset), 0L),
            atMillis = readLong(atKey(accountId, origin, dataset), 0L),
        )
    }

    override suspend fun clearOwner(accountId: String, origin: String) {
        if (accountId.isBlank()) return
        val prefix = "$accountId|$origin|"
        removeKeys(allKeys().filter { key ->
            val rest = when {
                key.startsWith("rev|") -> key.removePrefix("rev|")
                key.startsWith("at|") -> key.removePrefix("at|")
                else -> return@filter false
            }
            rest.startsWith(prefix)
        }.toList())
    }

    companion object {
        const val PREFS_NAME = "ledger_sync_ack"

        fun revKey(accountId: String, origin: String, dataset: String): String =
            "rev|$accountId|$origin|$dataset"

        fun atKey(accountId: String, origin: String, dataset: String): String =
            "at|$accountId|$origin|$dataset"
    }
}
