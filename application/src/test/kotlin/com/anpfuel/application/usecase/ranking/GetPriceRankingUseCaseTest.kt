package com.anpfuel.application.usecase.ranking

import com.anpfuel.domain.exploration.InterpretationState
import com.anpfuel.domain.exploration.MasterQuestions
import com.anpfuel.domain.exploration.PriceQuery
import com.anpfuel.domain.premium.PremiumValidationException
import com.anpfuel.domain.repository.MunicipalityIbgeLookup
import com.anpfuel.domain.repository.PriceRankingGateway
import com.anpfuel.domain.repository.RankingPage
import com.anpfuel.domain.repository.RankingQuery
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import java.time.Instant
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

class GetPriceRankingUseCaseTest {

    private val gateway = mockk<PriceRankingGateway>()
    private val catalog = mockk<MunicipalityIbgeLookup>()

    private lateinit var useCase: GetPriceRankingUseCase

    @BeforeEach
    fun setUp() {
        useCase = GetPriceRankingUseCase(gateway, catalog)
    }

    private fun resolved(question: String): PriceQuery {
        val got = MasterQuestions.interpret(question, Instant.parse("2026-10-22T12:00:00Z"))
        assertEquals(InterpretationState.RESOLVED, got.state)
        return got.query as PriceQuery
    }

    @Test fun `interpreted queries execute server-filtered and keep order`() = runTest {
        val page = RankingPage(emptyList(), emptyList(), "c2", "rev-1", "2026-10-19T00:00:00Z")
        val slot = io.mockk.slot<RankingQuery>()
        coEvery { gateway.rank(capture(slot), isNull()) } returns page
        val query = resolved("Qual o maior preço observado da gasolina comum no Brasil?")
        assertEquals(page, useCase(query, null))
        coVerify(exactly = 1) { gateway.rank(any(), isNull()) }
        assertEquals("GASOLINE_REGULAR", slot.captured.fuel)
        assertEquals("COUNTRY", slot.captured.geography)
        assertEquals("DESC", slot.captured.sort)
        assertEquals(20, slot.captured.limit)
    }

    @Test fun `municipal scope without resolvable ibge refuses before wire`() = runTest {
        coEvery { gateway.rank(any(), any()) } returns mockk()
        coEvery { catalog.lookup(any(), any()) } returns null
        coEvery { catalog.lookupAnywhere(any()) } returns null
        val municipal = resolved("Qual o maior preço observado do etanol em Campinas?")
        assertEquals("MUNICIPALITY", municipal.geography.level)
        assertThrows(PremiumValidationException::class.java) {
            runBlocking { useCase(municipal, null) }
        }
        coVerify(exactly = 0) { gateway.rank(any(), any()) }
    }

    @Test fun `unique global spelling executes municipal scope`() = runTest {
        val page = RankingPage(emptyList(), emptyList(), null, "rev-1", "2026-10-19T00:00:00Z")
        coEvery { gateway.rank(any(), any()) } returns page
        coEvery { catalog.lookupAnywhere("CAMPINAS") } returns "3550308"
        val municipal = resolved("Qual o maior preço observado do etanol em Campinas?")
        val got = useCase(municipal, null)
        assertEquals("rev-1", got.revisionId)
        val params = io.mockk.slot<com.anpfuel.domain.repository.RankingQuery>()
        coVerify(exactly = 1) { gateway.rank(capture(params), null) }
        assertEquals("3550308", params.captured.ibgeCode)
    }
}
