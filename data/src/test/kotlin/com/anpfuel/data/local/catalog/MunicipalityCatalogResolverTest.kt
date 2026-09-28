package com.anpfuel.data.local.catalog

import com.anpfuel.data.mapper.AnpStateMapper
import com.anpfuel.data.parser.SampleXlsxFiles
import com.anpfuel.data.parser.WeeklySummarySheetParser
import com.anpfuel.domain.rule.MunicipalitySearchTextNormalizer
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class MunicipalityCatalogResolverTest {

    @Test
    fun allAnpSummaryMunicipalitiesResolveAgainstIbgeAsset() {
        val catalogIndex = loadCatalogIndex()
        val rows = WeeklySummarySheetParser().parseToList(
            SampleXlsxFiles.resolve(SampleXlsxFiles.SUMMARY_SAMPLE),
        )
        val distinctMunicipalities = rows
            .map { row ->
                AnpStateMapper.toAbbreviation(row.state) to row.municipality
            }
            .distinct()

        assertTrue(distinctMunicipalities.size in 370..400)

        val unresolved = distinctMunicipalities.filter { (stateAbbrev, municipality) ->
            val normalized = MunicipalitySearchTextNormalizer.normalize(municipality)
            !catalogIndex.contains(stateAbbrev to normalized)
        }

        assertEquals(
            emptyList<Pair<String, String>>(),
            unresolved,
            "Unresolved municipalities: ${unresolved.take(10)}",
        )
    }

    private fun loadCatalogIndex(): Set<Pair<String, String>> {
        val json = resolveBundledAssetJson()
        return IbgeMunicipalityAsset.parse(json)
            .map { entry -> entry.state to entry.normalizedName }
            .toSet()
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
