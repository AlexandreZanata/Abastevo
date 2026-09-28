package com.anpfuel.data.local.entity

import androidx.room.ColumnInfo
import androidx.room.Entity
import androidx.room.Index
import androidx.room.PrimaryKey

/**
 * IBGE municipality master catalog (BR-016). FTS external-content source for UC-004.
 */
@Entity(
    tableName = "municipality_catalog",
    indices = [
        Index(value = ["state", "normalized_name"], unique = true),
        Index(value = ["ibge_code"], unique = true),
    ],
)
data class MunicipalityCatalogEntity(
    @PrimaryKey
    @ColumnInfo(name = "id")
    val id: String,
    @ColumnInfo(name = "ibge_code")
    val ibgeCode: String,
    val state: String,
    val municipality: String,
    @ColumnInfo(name = "normalized_name")
    val normalizedName: String,
    @ColumnInfo(name = "anp_alias")
    val anpAlias: String? = null,
)
