package com.anpfuel.data.local.entity

import androidx.room.ColumnInfo
import androidx.room.Entity
import androidx.room.Fts4
import androidx.room.FtsOptions

/**
 * External-content FTS index for UC-004 municipality search.
 *
 * Uses FTS4 via Room 2.6 ([Fts4]) with the plain `unicode61`
 * tokenizer: the only form every bundled SQLite back to minSdk 26
 * opens (`remove_diacritics` needs SQLite 3.20+, absent on API 26 —
 * `unknown tokenizer`, P24-T01). Accent-insensitive search survives
 * through the indexed `normalized_name` column (pre-normalized
 * uppercase ASCII from the IBGE asset): "SAO*" hits the normalized
 * column, "SÃO*" hits the raw column. No ICU, no app-layer
 * rewriting, identical behavior on all API levels.
 */
@Fts4(
    contentEntity = MunicipalityCatalogEntity::class,
    tokenizer = FtsOptions.TOKENIZER_UNICODE61,
)
@Entity(tableName = "municipality_fts")
class MunicipalityFtsEntity {

    @ColumnInfo(name = "municipality")
    var municipality: String = ""

    @ColumnInfo(name = "state")
    var state: String = ""

    @ColumnInfo(name = "normalized_name")
    var normalizedName: String = ""
}
