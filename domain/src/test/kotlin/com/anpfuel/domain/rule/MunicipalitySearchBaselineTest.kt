package com.anpfuel.domain.rule

import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.MunicipalityCatalogEntry
import java.io.File
import java.security.MessageDigest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/** MEX-03 actual fallback CPU probe. Host JVM values are not Android UI timing. */
class MunicipalitySearchBaselineTest {
    @Test
    fun fullCatalogFallbackBaseline() {
        val asset = File("../data/src/main/assets/ibge_municipalities.json")
        val raw = asset.readText()
        // Deliberately pin the checked-in asset profile; fail on changed framing.
        val pattern = Regex("""\{"ibgeCode":"(\d{7})","state":"([A-Z]{2})","municipality":"([^"]+)","normalizedName":"[^"]+"}""")
        val entries = pattern.findAll(raw).map { match ->
            MunicipalityCatalogEntry(
                state = requireNotNull(BrazilianState.fromAbbreviation(match.groupValues[2])),
                municipality = match.groupValues[3],
                ibgeCode = match.groupValues[1],
            )
        }.toList()
        assertEquals(Regex("ibgeCode").findAll(raw).count(), entries.size)
        assertTrue(entries.size > 5_000)
        assertEquals(entries.size, entries.map { it.ibgeCode }.toSet().size)
        val hash = MessageDigest.getInstance("SHA-256").digest(asset.readBytes()).joinToString("") { "%02x".format(it) }
        println("MEX03 asset=$hash rows=${entries.size} scope=HOST_FALLBACK_CPU_ONLY")
        for (query in listOf("SORRISO", "SORISO", "SAO", "z".repeat(128))) {
            fun search() = IntelligentSearchRankingRule.rank(
                query,
                entries.filter { IntelligentSearchRankingRule.classifyMatch(query, it.municipality) != null },
            ).take(20)
            val firstStart = System.nanoTime()
            val expected = search()
            val firstMs = (System.nanoTime() - firstStart) / 1_000_000.0
            if (query == "SORRISO" || query == "SORISO") {
                assertTrue(expected.any { it.entry.ibgeCode == "5107925" })
            }
            val samples = List(11) {
                val start = System.nanoTime()
                assertEquals(expected, search())
                (System.nanoTime() - start) / 1_000_000.0
            }.sorted()
            println("MEX03 query=${query.take(12)} length=${query.length} first_ms=$firstMs warm_p50_ms=${samples[5]} warm_p95_ms=${samples[10]} rows=${expected.size}")
        }
    }
}
