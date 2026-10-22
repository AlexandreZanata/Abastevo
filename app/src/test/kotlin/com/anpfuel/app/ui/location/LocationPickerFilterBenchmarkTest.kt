package com.anpfuel.app.ui.location

import com.anpfuel.application.usecase.location.CatalogMunicipalityItem
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DataAvailability
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * MEX-06 host benchmark for the measured debounce policy. Simulates
 * keystrokes over realistic list sizes (27 states; a 645-municipality
 * state) through the old per-keystroke path (renormalize every item) and
 * the indexed path (one query normalization + substring checks).
 *
 * Host JVM values size the composition work only; they are not Android
 * keystroke-to-visible proof, which stays OWED with the device matrix.
 * The measured policy ([LocationPickerSearchPolicy]) keeps zero artificial
 * delay because the indexed path is orders of magnitude below a frame.
 */
class LocationPickerFilterBenchmarkTest {

    private val states = BrazilianState.entries.toList()
    private val municipalities: List<CatalogMunicipalityItem> by lazy {
        buildList {
            // Realistic 645-row state list with accents and shared prefixes.
            val prefixes = listOf("SÃO", "SANTA", "ALTO", "ALTA", "NOVA", "NOVO", "BOA", "BOM", "PORTO", "VILA")
            var i = 0
            while (size < 645) {
                val prefix = prefixes[i % prefixes.size]
                add(
                    CatalogMunicipalityItem(
                        municipality = "$prefix ${'A' + (i / prefixes.size % 26)}${"X".repeat(i % 7)}",
                        dataAvailability = DataAvailability.HAS_DATA,
                    ),
                )
                i++
            }
        }
    }

    @Test
    fun indexedKeystrokesStayImmediateAndMatchPlainFilter() {
        val keystrokes = listOf("s", "sa", "sao", "sao ", "sao p", "sao pa", "sao pau", "sao paul", "sao paulo")
        val index = LocationPickerFilter.indexMunicipalities(municipalities)
        assertEquals(municipalities.size, index.size)

        for (query in keystrokes) {
            assertEquals(
                LocationPickerFilter.filterMunicipalities(municipalities, query),
                LocationPickerFilter.filterIndexed(index, query),
                "query=$query",
            )
        }

        fun oldPath(): List<List<CatalogMunicipalityItem>> =
            keystrokes.map { LocationPickerFilter.filterMunicipalities(municipalities, it) }

        fun newPath(): List<List<CatalogMunicipalityItem>> =
            keystrokes.map { LocationPickerFilter.filterIndexed(index, it) }

        assertEquals(oldPath(), newPath())
        val coldOld = measureMs { oldPath() }
        val coldNew = measureMs { newPath() }
        val warmOld = List(11) { measureMs { oldPath() } }.sorted()
        val warmNew = List(11) { measureMs { newPath() } }.sorted()
        println(
            "MEX06 keystrokes=${keystrokes.size} rows=${municipalities.size} " +
                "old_cold_ms=$coldOld old_warm_p50_ms=${warmOld[5]} old_warm_p95_ms=${warmOld[10]} " +
                "new_cold_ms=$coldNew new_warm_p50_ms=${warmNew[5]} new_warm_p95_ms=${warmNew[10]} " +
                "scope=HOST_FILTER_ONLY",
        )
        assertTrue(warmNew[10] < 16.0, "indexed keystroke p95 must stay well below a 16ms frame")
    }

    private fun measureMs(block: () -> Unit): Double {
        val start = System.nanoTime()
        block()
        return (System.nanoTime() - start) / 1_000_000.0
    }
}
