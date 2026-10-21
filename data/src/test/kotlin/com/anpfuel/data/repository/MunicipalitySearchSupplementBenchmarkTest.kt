package com.anpfuel.data.repository

import com.anpfuel.domain.rule.IntelligentSearchRankingRule
import com.anpfuel.domain.rule.MunicipalitySearchTextNormalizer
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.MunicipalityCatalogEntry
import java.io.File
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * MEX-05 host benchmark and recall-parity probe. The retired per-keystroke
 * full-catalog scan is reproduced here as the oracle; the bounded stem-pool
 * path must return the same visible top results for the documented probes
 * while keeping every pool within [MunicipalitySearchRepositoryImpl.SUPPLEMENT_LIMIT].
 * Host JVM values are not Android timing; native keystroke-to-visible proof
 * stays with MEX-06 and the deferred device matrix.
 */
class MunicipalitySearchSupplementBenchmarkTest {

    private val entries: List<MunicipalityCatalogEntry> by lazy { loadAssetEntries() }

    @Test
    fun boundedPoolsMatchFullScanTopResults() {
        val probes = listOf(
            "SORRISO",
            "SORISO",
            "san paolo",
            "SAO",
            "São Paulo",
            "CAMPINAS",
            "SO",
            "xyz",
            "z".repeat(128),
        )
        for (query in probes) {
            val expected = fullScanTop(query, limit = 20)
            val pool = supplementPool(query)
            assertTrue(
                pool.size <= 2 * MunicipalitySearchRepositoryImpl.SUPPLEMENT_LIMIT,
                "query=$query pool=${pool.size}",
            )
            val actual = IntelligentSearchRankingRule.rank(query, pool).take(20)
            // Visible parity: whatever the full scan surfaced first must
            // still surface first from the bounded pool.
            assertEquals(
                expected.take(3).map { it.entry.ibgeCode },
                actual.take(3).map { it.entry.ibgeCode },
                "query=$query",
            )
            if (query == "SORISO" || query == "SORRISO") {
                assertTrue(actual.any { it.entry.ibgeCode == "5107925" }, "query=$query")
            }
            if (query == "san paolo") {
                assertTrue(
                    actual.take(3).any { it.entry.state == BrazilianState.SAO_PAULO },
                    "query=$query",
                )
            }
            if (query == "xyz" || query == "z".repeat(128)) {
                assertEquals(0, actual.size, "query=$query")
            }
        }

        val stems1 = entries.map { it.normalized().take(1) }.toSet()
        val stems2 = entries.map { it.normalized().take(2) }.toSet()
        val maxPool1 = stems1.maxOf { stem -> entries.count { it.normalized().startsWith(stem) } }
        val maxPool2 = stems2.maxOf { stem -> entries.count { it.normalized().startsWith(stem) } }
        println("MEX05 national stems=${stems1.size}/${stems2.size} maxPool1=$maxPool1 maxPool2=$maxPool2")
        // Guards future catalog growth: every stem must stay fully covered.
        assertTrue(maxPool1 <= MunicipalitySearchRepositoryImpl.SUPPLEMENT_LIMIT)
        assertTrue(maxPool2 <= MunicipalitySearchRepositoryImpl.SUPPLEMENT_LIMIT)

        for (query in listOf("SORISO", "san paolo", "SAO")) {
            val cold = measureMs { supplementPool(query) }
            val samples = List(11) { measureMs { supplementPool(query) } }.sorted()
            println(
                "MEX05 query=${query.take(12)} pool=${supplementPool(query).size} " +
                    "cold_ms=$cold warm_p50_ms=${samples[5]} warm_p95_ms=${samples[10]} scope=HOST_POOL_ONLY",
            )
        }
        val oldCold = measureMs { fullScanTop("SORISO") }
        val oldSamples = List(11) { measureMs { fullScanTop("SORISO") } }.sorted()
        println(
            "MEX05 retired full-scan cold_ms=$oldCold warm_p50_ms=${oldSamples[5]} " +
                "warm_p95_ms=${oldSamples[10]} scope=HOST_ORACLE_ONLY",
        )
    }

    private fun fullScanTop(query: String, limit: Int = 20) =
        IntelligentSearchRankingRule.rank(
            query,
            entries.filter { IntelligentSearchRankingRule.classifyMatch(query, it.municipality) != null },
        ).take(limit)

    private fun supplementPool(query: String): List<MunicipalityCatalogEntry> {
        val stems = MunicipalitySearchTextNormalizer.normalize(query)
            .split(' ')
            .filter { it.length >= 2 }
            .take(2)
            .map { token -> if (token.length >= 4) token.take(2) else token.take(1) }
        if (stems.isEmpty()) {
            return emptyList()
        }
        val seen = mutableSetOf<String>()
        val pooled = mutableListOf<MunicipalityCatalogEntry>()
        for (stem in stems) {
            for (entry in entries.filter { it.normalized().startsWith(stem) }
                .sortedBy { it.normalized() }
                .take(MunicipalitySearchRepositoryImpl.SUPPLEMENT_LIMIT)) {
                if (seen.add(entry.ibgeCode.orEmpty())) {
                    pooled.add(entry)
                }
            }
        }
        return pooled
    }

    private fun measureMs(block: () -> Unit): Double {
        val start = System.nanoTime()
        block()
        return (System.nanoTime() - start) / 1_000_000.0
    }

    private fun MunicipalityCatalogEntry.normalized(): String =
        MunicipalitySearchTextNormalizer.normalize(municipality)

    private fun loadAssetEntries(): List<MunicipalityCatalogEntry> {
        val raw = File("src/main/assets/ibge_municipalities.json").readText()
        val pattern =
            Regex("""\{"ibgeCode":"(\d{7})","state":"([A-Z]{2})","municipality":"([^"]+)","normalizedName":"[^"]+"}""")
        val parsed = pattern.findAll(raw).map { match ->
            MunicipalityCatalogEntry(
                state = requireNotNull(BrazilianState.fromAbbreviation(match.groupValues[2])),
                municipality = match.groupValues[3],
                ibgeCode = match.groupValues[1],
            )
        }.toList()
        assertTrue(parsed.size > 5_000)
        return parsed
    }
}
