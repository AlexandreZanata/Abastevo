package com.anpfuel.data.local.entity

import androidx.room.ColumnInfo
import androidx.room.Entity
import androidx.room.Fts4
import androidx.room.FtsOptions

/**
 * External-content FTS index for UC-004 municipality search.
 *
 * Uses FTS4 via Room 2.6 ([Fts4]); architecture targets FTS5 semantics with
 * `unicode61 remove_diacritics 2` tokenization on the same columns.
 */
@Fts4(
    contentEntity = MunicipalityCatalogEntity::class,
    tokenizer = FtsOptions.TOKENIZER_UNICODE61,
    tokenizerArgs = ["remove_diacritics=2"],
)
@Entity(tableName = "municipality_fts")
class MunicipalityFtsEntity {

    @ColumnInfo(name = "municipality")
    var municipality: String = ""

    @ColumnInfo(name = "state")
    var state: String = ""
}
