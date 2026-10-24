package com.anpfuel.data.repository

import com.anpfuel.data.local.dao.MunicipalityCatalogDao
import com.anpfuel.data.local.entity.MunicipalityCatalogEntity
import com.anpfuel.data.remote.ranking.PriceRankingArea
import com.anpfuel.data.remote.ranking.PriceRankingHttpClient
import com.anpfuel.data.remote.ranking.PriceRankingPage
import com.anpfuel.data.remote.ranking.PriceRankingStation
import com.anpfuel.domain.repository.RankingQuery
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class PriceRankingGatewayTest {

    private val client = mockk<PriceRankingHttpClient>()
    private val dao = mockk<MunicipalityCatalogDao>(relaxed = true)

    private fun entity(code: String) = MunicipalityCatalogEntity(
        id = "1", state = "SP", ibgeCode = code, municipality = "CAMPINAS",
        normalizedName = "CAMPINAS",
    )

    @Test fun `gateway maps wire shapes without re-sorting`() = runTest {
        val gateway = PriceRankingGatewayImpl(client)
        val page = PriceRankingPage(
            stations = listOf(
                PriceRankingStation("11111111-1111-4111-8111-111111111111", 5990, "2026-10-18", "DF", "5300108"),
                PriceRankingStation("22222222-2222-4222-8222-222222222222", 5890, "2026-10-18", null, null),
            ),
            areas = listOf(
                PriceRankingArea("STATE", "DF", "", "599/100", "1", 5900, 6100, 200, 8),
            ),
            nextCursor = null, revisionId = "rev-1", generatedAt = "2026-10-19T00:00:00Z",
        )
        coEvery { client.rank(any()) } returns page
        val got = gateway.rank(
            RankingQuery("GASOLINE_REGULAR", "L", "STANDARD", "COUNTRY", null, null, "OBSERVATION_PRICE", "STATION", "DESC", null, 20, null),
            null,
        )
        assertEquals(2, got.stations.size)
        assertEquals(5990L, got.stations[0].amountMilliBrl)
        assertEquals("599/100", got.areas.single().meanNumerator)
        coVerify(exactly = 1) { client.rank(any()) }
    }

    @Test fun `lookup resolves exact unique match and refuses the rest`() = runTest {
        val lookup = MunicipalityIbgeLookupImpl(dao)
        coEvery { dao.findByStateAndNormalizedName("SP", "CAMPINAS") } returns entity("3550308")
        assertEquals("3550308", lookup.lookup("SP", "campinas"))
        coEvery { dao.findByStateAndNormalizedName(any(), any()) } returns null
        assertEquals(null, lookup.lookup("SP", "nowhere"))
        assertEquals(null, lookup.lookup("XX", "CAMPINAS"))
        assertEquals(null, lookup.lookup("", "CAMPINAS"))
        coEvery { dao.findByNormalizedNameAnywhere("CAMPINAS") } returns listOf(entity("3550308"))
        assertEquals("3550308", lookup.lookupAnywhere("campinas"))
        coEvery { dao.findByNormalizedNameAnywhere(any()) } returns listOf(entity("1"), entity("2"))
        assertEquals(null, lookup.lookupAnywhere("ambiguous"))
    }
}
