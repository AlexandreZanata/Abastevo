package com.anpfuel.domain.rule

import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.MunicipalityCatalogEntry
import com.anpfuel.domain.valueobject.SearchMatchType
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class IntelligentSearchRankingRuleTest {

    @Test
    fun exactPrefixRanksAboveAccentNormalizedMatch() {
        val exact = MunicipalityCatalogEntry(BrazilianState.SAO_PAULO, "SAO PAULO")
        val accent = MunicipalityCatalogEntry(BrazilianState.RIO_DE_JANEIRO, "NITERÓI")

        val ranked = IntelligentSearchRankingRule.rank(
            query = "SAO",
            candidates = listOf(accent, exact),
        )

        assertEquals(SearchMatchType.EXACT_PREFIX, ranked.first().matchType)
        assertEquals(BrazilianState.SAO_PAULO, ranked.first().entry.state)
    }

    @Test
    fun accentNormalizedMatchRanksAboveSubstring() {
        assertEquals(
            SearchMatchType.ACCENT_NORMALIZED,
            IntelligentSearchRankingRule.classifyMatch("NITEROI", "NITERÓI"),
        )
        assertEquals(
            SearchMatchType.SUBSTRING,
            IntelligentSearchRankingRule.classifyMatch("TEROI", "NITERÓI"),
        )
    }

    @Test
    fun typoSanPaoloClassifiesAsTypoTolerant() {
        assertEquals(
            SearchMatchType.TYPO_TOLERANT,
            IntelligentSearchRankingRule.classifyMatch("san paolo", "SAO PAULO"),
        )
    }

    @Test
    fun typoSanPaoloReturnsSaoPauloInTopThree() {
        val candidates = listOf(
            MunicipalityCatalogEntry(BrazilianState.SAO_PAULO, "SAO PAULO"),
            MunicipalityCatalogEntry(BrazilianState.RIO_GRANDE_DO_SUL, "SAO PAULO DO SUL"),
            MunicipalityCatalogEntry(BrazilianState.MINAS_GERAIS, "SAO PAULO DE OLIVENCA"),
            MunicipalityCatalogEntry(BrazilianState.AMAZONAS, "SAO PAULO DE OLIVENCA"),
            MunicipalityCatalogEntry(BrazilianState.PARANA, "PALOTINA"),
        )

        val ranked = IntelligentSearchRankingRule.rank("san paolo", candidates)
        val topThreeStates = ranked.take(3).map { it.entry.state }

        assertTrue(BrazilianState.SAO_PAULO in topThreeStates)
    }

    @Test
    fun homonymsTieBreakByStateAbbreviation() {
        val candidates = listOf(
            MunicipalityCatalogEntry(BrazilianState.RIO_GRANDE_DO_SUL, "BOM JESUS"),
            MunicipalityCatalogEntry(BrazilianState.SANTA_CATARINA, "BOM JESUS"),
            MunicipalityCatalogEntry(BrazilianState.PIAUI, "BOM JESUS"),
        )

        val ranked = IntelligentSearchRankingRule.rank("BOM JESUS", candidates)

        assertEquals(3, ranked.size)
        assertEquals(BrazilianState.PIAUI, ranked[0].entry.state)
        assertEquals(BrazilianState.RIO_GRANDE_DO_SUL, ranked[1].entry.state)
        assertEquals(BrazilianState.SANTA_CATARINA, ranked[2].entry.state)
    }

    @Test
    fun queryShorterThanTwoCharactersReturnsNullMatchType() {
        assertNull(IntelligentSearchRankingRule.classifyMatch("A", "ARACAJU"))
    }
}
