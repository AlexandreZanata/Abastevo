package com.anpfuel.data.remote

import java.io.File

internal object NominatimFixtureFiles {

    fun readReverseCuritiba(): String = resolve(NOMINATIM_REVERSE_CURITIBA).readText()

    fun readSearchCuritiba(): String = resolve(NOMINATIM_SEARCH_CURITIBA).readText()

    fun resolve(fileName: String): File {
        val candidates = listOf(
            File("fixtures/$fileName"),
            File("data/fixtures/$fileName"),
            File("../fixtures/$fileName"),
            File("../data/fixtures/$fileName"),
        )
        return candidates.firstOrNull { it.exists() }
            ?: error("Fixture file not found: $fileName (cwd=${File(".").absolutePath})")
    }

    const val NOMINATIM_REVERSE_CURITIBA = "nominatim-reverse-curitiba.json"

    const val NOMINATIM_SEARCH_CURITIBA = "nominatim-search-curitiba.json"
}
