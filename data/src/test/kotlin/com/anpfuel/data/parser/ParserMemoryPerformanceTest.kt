package com.anpfuel.data.parser

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Tag
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.Timeout
import java.util.concurrent.TimeUnit

class ParserMemoryPerformanceTest {

    private val parser = StationDetailSheetParser()
    private val stationFile = SampleXlsxFiles.resolve(SampleXlsxFiles.STATION_SAMPLE)

    @Test
    @Tag("parser-memory")
    @Timeout(value = 2, unit = TimeUnit.MINUTES)
    fun parsesFullStationFileWithoutOomOn64MbHeap() {
        var rowCount = 0
        parser.parse(stationFile) { rowCount++ }

        assertEquals(EXPECTED_STATION_ROW_COUNT, rowCount)
    }

    @Test
    @Timeout(value = 2, unit = TimeUnit.MINUTES)
    fun measuresStationFileParseDuration() {
        val startNanos = System.nanoTime()
        var rowCount = 0
        parser.parse(stationFile) { rowCount++ }
        val elapsedMs = (System.nanoTime() - startNanos) / 1_000_000

        assertEquals(EXPECTED_STATION_ROW_COUNT, rowCount)
        println("STATION_PARSE_DURATION_MS=$elapsedMs")
        assertTrue(
            elapsedMs < SANITY_PARSE_LIMIT_MS,
            "Parse exceeded sanity limit of ${SANITY_PARSE_LIMIT_MS}ms: ${elapsedMs}ms",
        )
    }

    companion object {
        const val EXPECTED_STATION_ROW_COUNT = 19_676
        const val TARGET_PARSE_DURATION_MS = 30_000L
        const val SANITY_PARSE_LIMIT_MS = 120_000L
    }
}
