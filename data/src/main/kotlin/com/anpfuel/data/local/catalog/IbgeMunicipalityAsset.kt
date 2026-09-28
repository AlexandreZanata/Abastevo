package com.anpfuel.data.local.catalog

import com.anpfuel.data.local.entity.MunicipalityCatalogEntity
import org.json.JSONArray

internal data class IbgeMunicipalityAssetEntry(
    val ibgeCode: String,
    val state: String,
    val municipality: String,
    val normalizedName: String,
) {
    fun toEntity(): MunicipalityCatalogEntity =
        MunicipalityCatalogEntity(
            id = ibgeCode,
            ibgeCode = ibgeCode,
            state = state,
            municipality = municipality,
            normalizedName = normalizedName,
            anpAlias = null,
        )
}

internal object IbgeMunicipalityAsset {

    private const val ASSET_FILE = "ibge_municipalities.json"

    fun parse(json: String): List<IbgeMunicipalityAssetEntry> {
        val array = JSONArray(json)
        return buildList(array.length()) {
            for (index in 0 until array.length()) {
                val objectNode = array.getJSONObject(index)
                add(
                    IbgeMunicipalityAssetEntry(
                        ibgeCode = objectNode.getString("ibgeCode"),
                        state = objectNode.getString("state"),
                        municipality = objectNode.getString("municipality"),
                        normalizedName = objectNode.getString("normalizedName"),
                    ),
                )
            }
        }
    }

    fun assetFileName(): String = ASSET_FILE
}
