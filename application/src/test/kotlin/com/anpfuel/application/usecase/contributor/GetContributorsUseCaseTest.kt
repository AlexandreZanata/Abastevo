package com.anpfuel.application.usecase.contributor

import com.anpfuel.domain.contributor.Contributor
import com.anpfuel.domain.contributor.ContributorException
import com.anpfuel.domain.contributor.ContributorPage
import com.anpfuel.domain.repository.ContributorsGateway
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test

class GetContributorsUseCaseTest {

    private val ana = Contributor("Ana", emptyList(), "octocat", "2026-03", listOf("Suporte"))

    private fun gateway(pages: Map<String?, ContributorPage>): ContributorsGateway =
        object : ContributorsGateway {
            override suspend fun page(cursor: String?): ContributorPage =
                pages[cursor] ?: throw ContributorException.Unavailable("contributors.unavailable")
        }

    @Test fun `first and second pages pass through`() = runTest {
        val useCase = GetContributorsUseCase(
            gateway(mapOf(null to ContributorPage(listOf(ana), "c2"), "c2" to ContributorPage(emptyList(), ""))),
        )
        val first = useCase(null)
        assertEquals(listOf(ana), first.items)
        assertEquals("c2", first.nextCursor)
        val second = useCase("c2")
        assertEquals("", second.nextCursor)
    }

    @Test fun `oversized cursor is refused before any call`() = runTest {
        var calls = 0
        val useCase = GetContributorsUseCase(
            object : ContributorsGateway {
                override suspend fun page(cursor: String?): ContributorPage {
                    calls += 1
                    return ContributorPage(emptyList(), "")
                }
            },
        )
        assertThrows(ContributorException.Invalid::class.java) {
            kotlinx.coroutines.runBlocking { useCase("x".repeat(4097)) }
        }
        assertEquals(0, calls)
    }

    @Test fun `gateway failures propagate`() = runTest {
        val useCase = GetContributorsUseCase(gateway(emptyMap()))
        assertThrows(ContributorException.Unavailable::class.java) {
            kotlinx.coroutines.runBlocking { useCase(null) }
        }
    }
}
