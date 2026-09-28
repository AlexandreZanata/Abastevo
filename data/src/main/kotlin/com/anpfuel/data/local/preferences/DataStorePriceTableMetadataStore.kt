package com.anpfuel.data.local.preferences

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import com.anpfuel.domain.model.PriceTable
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.PriceTableType
import com.anpfuel.domain.valueobject.SurveyWeek
import dagger.hilt.android.qualifiers.ApplicationContext
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import java.time.LocalDate
import javax.inject.Inject
import javax.inject.Singleton

private val Context.priceTableMetadataDataStore: DataStore<Preferences> by preferencesDataStore(
    name = "price_table_metadata",
)

/**
 * Persists discovered [PriceTable] checksum metadata outside Room (no dedicated table in schema).
 */
@Singleton
class DataStorePriceTableMetadataStore @Inject constructor(
    @ApplicationContext context: Context,
) : PriceTableMetadataStore {
    private val dataStore = context.priceTableMetadataDataStore

    override suspend fun findByUrl(sourceUrl: String): PriceTable? =
        readAll()[sourceUrl]

    override suspend fun save(priceTable: PriceTable) {
        dataStore.edit { preferences ->
            val all = decodeEntries(preferences[Keys.ENTRIES].orEmpty()).toMutableMap()
            all[priceTable.sourceUrl] = priceTable
            val rawEntries = all.mapValues { (_, table) -> encodeEntry(table) }
            preferences[Keys.ENTRIES] = encodeRawEntries(rawEntries)
        }
    }

    override suspend fun clearAll() {
        dataStore.edit { preferences ->
            preferences.remove(Keys.ENTRIES)
        }
    }

    private suspend fun readAll(): Map<String, PriceTable> =
        dataStore.data.map { preferences ->
            decodeEntries(preferences[Keys.ENTRIES].orEmpty())
        }.first()

    private fun encodeRawEntries(entries: Map<String, String>): String =
        entries.entries.joinToString(separator = RECORD_SEPARATOR) { (url, payload) ->
            "$url$FIELD_SEPARATOR$payload"
        }

    private fun decodeEntries(raw: String): Map<String, PriceTable> {
        if (raw.isBlank()) {
            return emptyMap()
        }

        return raw.split(RECORD_SEPARATOR)
            .mapNotNull { record ->
                val separatorIndex = record.indexOf(FIELD_SEPARATOR)
                if (separatorIndex <= 0) {
                    return@mapNotNull null
                }
                val url = record.substring(0, separatorIndex)
                val payload = record.substring(separatorIndex + 1)
                decodeEntry(url, payload)?.let { url to it }
            }
            .toMap()
    }

    private fun encodeEntry(priceTable: PriceTable): String =
        listOf(
            priceTable.id.value,
            priceTable.surveyWeek.startDate.toString(),
            priceTable.surveyWeek.endDate.toString(),
            priceTable.tableType.name,
            priceTable.checksum.orEmpty(),
        ).joinToString(separator = FIELD_SEPARATOR)

    private fun decodeEntry(sourceUrl: String, payload: String): PriceTable? {
        val parts = payload.split(FIELD_SEPARATOR)
        if (parts.size < 4) {
            return null
        }

        return runCatching {
            PriceTable(
                id = DomainId.from(parts[0]),
                surveyWeek = SurveyWeek(
                    startDate = LocalDate.parse(parts[1]),
                    endDate = LocalDate.parse(parts[2]),
                ),
                tableType = PriceTableType.valueOf(parts[3]),
                sourceUrl = sourceUrl,
                checksum = parts.getOrNull(4)?.takeIf { it.isNotBlank() },
            )
        }.getOrNull()
    }

    private object Keys {
        val ENTRIES = stringPreferencesKey("discovered_price_tables")
    }

    companion object {
        private const val RECORD_SEPARATOR = "\u001E"
        private const val FIELD_SEPARATOR = "\u001F"
    }
}
