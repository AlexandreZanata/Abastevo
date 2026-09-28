package com.anpfuel.data.local.catalog

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class IbgeMunicipalityAssetTest {

    @Test
    fun parsesBundledAssetStructure() {
        val json = """
            [
              {"ibgeCode":"3550308","state":"SP","municipality":"SÃO PAULO","normalizedName":"SAO PAULO"},
              {"ibgeCode":"4106902","state":"PR","municipality":"CURITIBA","normalizedName":"CURITIBA"}
            ]
        """.trimIndent()

        val entries = IbgeMunicipalityAsset.parse(json)

        assertEquals(2, entries.size)
        assertEquals("3550308", entries.first().ibgeCode)
        assertEquals("SAO PAULO", entries.first().normalizedName)
        assertEquals("SP", entries.first().toEntity().state)
    }

    @Test
    fun bundledAssetMeetsMinimumCatalogSize() {
        val json = resolveBundledAssetJson()

        val entries = IbgeMunicipalityAsset.parse(json)

        assertTrue(entries.size >= 5_570)
    }

    private fun resolveBundledAssetJson(): String {
        val candidates = listOf(
            java.io.File("src/main/assets/${IbgeMunicipalityAsset.assetFileName()}"),
            java.io.File("data/src/main/assets/${IbgeMunicipalityAsset.assetFileName()}"),
        )
        val file = candidates.firstOrNull { it.exists() }
            ?: error("Bundled asset ${IbgeMunicipalityAsset.assetFileName()} not found")
        return file.readText()
    }
}
