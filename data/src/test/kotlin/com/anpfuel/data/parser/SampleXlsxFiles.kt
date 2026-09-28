package com.anpfuel.data.parser

import java.io.File

internal object SampleXlsxFiles {

    fun resolve(fileName: String): File {
        val candidates = listOf(
            File("samples/$fileName"),
            File("../samples/$fileName"),
            File("data/samples/$fileName"),
            File("../data/samples/$fileName"),
        )
        return candidates.firstOrNull { it.exists() }
            ?: error("Sample XLSX not found: $fileName (cwd=${File(".").absolutePath})")
    }

    const val SUMMARY_SAMPLE = "resumo_semanal_lpc_2026-06-07_2026-06-13.xlsx"
    const val STATION_SAMPLE = "revendas_lpc_2026-06-07_2026-06-13.xlsx"
}
