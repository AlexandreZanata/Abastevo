package com.anpfuel.data.local.preferences

import android.content.SharedPreferences
import com.anpfuel.domain.charging.ChargingCondition
import com.anpfuel.domain.charging.ChargingConnector
import com.anpfuel.domain.charging.ChargingObservationDraft
import com.anpfuel.domain.charging.ChargingPendingStore
import com.anpfuel.domain.charging.PendingChargingKind
import com.anpfuel.domain.charging.PendingChargingOperation
import javax.inject.Inject
import javax.inject.Named
import javax.inject.Singleton
import org.json.JSONArray
import org.json.JSONObject

/**
 * EE-22 durable pending community operations, partitioned by account and
 * origin. Only explicit draft fields are stored; no GPS, route or home
 * context exists. A corrupt/oversized entry is dropped on read instead of
 * crashing the screen; the write path always logs the actual server error.
 */
@Singleton
class ChargingPendingStoreImpl @Inject constructor(
    @Named("auth") private val prefs: SharedPreferences,
) : ChargingPendingStore {

    override fun load(accountId: String, origin: String): List<PendingChargingOperation> {
        val raw = prefs.getString(key(accountId, origin), null) ?: return emptyList()
        return try {
            ChargingPendingCodec.decode(raw)
        } catch (_: Exception) {
            emptyList()
        }
    }

    override fun save(accountId: String, origin: String, operations: List<PendingChargingOperation>) {
        require(operations.size <= MAX_PENDING)
        prefs.edit().putString(key(accountId, origin), ChargingPendingCodec.encode(operations)).commit()
    }

    override fun clear(accountId: String, origin: String) {
        prefs.edit().remove(key(accountId, origin)).commit()
    }

    private fun key(accountId: String, origin: String): String = "charging_pending|$accountId|$origin"

    companion object {
        const val MAX_PENDING = 50
    }
}

object ChargingPendingCodec {

    fun encode(operations: List<PendingChargingOperation>): String {
        val array = JSONArray()
        operations.forEach { operation ->
            val row = JSONObject()
                .put("operation_id", operation.operationId)
                .put("kind", operation.kind.name)
                .put("created_at", operation.createdAtMillis)
            operation.targetId?.let { row.put("target_id", it) }
            operation.expectedRevision?.let { row.put("expected_revision", it) }
            operation.lastError?.let { row.put("last_error", it) }
            operation.draft?.let { draft ->
                row.put(
                    "draft",
                    JSONObject()
                        .put("connector", draft.connector.name)
                        .put("condition", draft.condition.name)
                        .put("observed_day", draft.observedDay)
                        .put("note", draft.note)
                        .apply {
                            draft.placeId?.let { put("place_id", it) }
                            draft.priceMilliBrlPerKwh?.let { put("price_milli_brl_per_kwh", it) }
                        },
                )
            }
            array.put(row)
        }
        return array.toString()
    }

    fun decode(raw: String): List<PendingChargingOperation> {
        require(raw.length <= 64 * 1024)
        val array = JSONArray(raw)
        require(array.length() <= ChargingPendingStoreImpl.MAX_PENDING)
        return (0 until array.length()).map { index ->
            val row = array.getJSONObject(index)
            val draft = if (row.has("draft")) {
                val objectDraft = row.getJSONObject("draft")
                ChargingObservationDraft(
                    placeId = if (objectDraft.has("place_id")) objectDraft.getString("place_id") else null,
                    connector = ChargingConnector.valueOf(objectDraft.getString("connector")),
                    priceMilliBrlPerKwh = if (objectDraft.has("price_milli_brl_per_kwh")) {
                        objectDraft.getLong("price_milli_brl_per_kwh")
                    } else {
                        null
                    },
                    condition = ChargingCondition.valueOf(objectDraft.getString("condition")),
                    observedDay = objectDraft.getString("observed_day"),
                    note = objectDraft.optString("note", ""),
                )
            } else {
                null
            }
            PendingChargingOperation(
                operationId = row.getString("operation_id"),
                kind = PendingChargingKind.valueOf(row.getString("kind")),
                targetId = if (row.has("target_id")) row.getString("target_id") else null,
                expectedRevision = if (row.has("expected_revision")) row.getLong("expected_revision") else null,
                draft = draft,
                createdAtMillis = row.getLong("created_at"),
                lastError = if (row.has("last_error")) row.getString("last_error") else null,
            )
        }
    }
}
