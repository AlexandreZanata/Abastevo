package com.anpfuel.application.usecase.forum

import com.anpfuel.domain.forum.ForumCategory
import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.forum.ForumRegion
import com.anpfuel.domain.forum.ForumTopicReceipt
import com.anpfuel.domain.repository.ForumGateway
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class ForumTopicUseCasesTest {

    private val validTopicId = "3fa85f64-5717-4562-b3fc-2c963f66afa6"
    private val validStationId = "550e8400-e29b-41d4-a716-446655440000"

    private fun sampleReceipt(
        id: String = validTopicId,
        revision: Long = 1L,
        state: String = "PENDING_REVIEW",
        replayed: Boolean = false,
    ) = ForumTopicReceipt(
        id = id,
        kind = "TOPIC",
        category = ForumCategory.FUELS,
        rawCategory = "FUELS",
        title = "Preço da gasolina em SP",
        bodyText = "Discussão detalhada sobre postos na capital.",
        revision = revision,
        publicationState = state,
        visibility = "VISIBLE",
        region = ForumRegion("SP", 3550308),
        fuelProduct = "GASOLINE_REGULAR",
        stationId = validStationId,
        publicAlias = "Alex",
        createdAt = "2026-11-06T12:00:00Z",
        updatedAt = "2026-11-06T12:00:00Z",
        publishedAt = null,
        replayed = replayed,
    )

    private fun mockGateway(
        onReceiveCreate: ((String, String, ForumCategory, String, ForumRegion?, String?, String?, String?) -> ForumTopicReceipt)? = null,
        onReceiveEdit: ((String, Long, String, String, ForumCategory, String, ForumRegion?, String?, String?, String?) -> ForumTopicReceipt)? = null,
        onReceiveRemove: ((String, String) -> ForumTopicReceipt)? = null,
        onReceiveOwnerRead: ((String) -> ForumTopicReceipt)? = null,
    ): ForumGateway = object : ForumGateway {
        override suspend fun createTopic(
            title: String,
            bodyText: String,
            category: ForumCategory,
            operationId: String,
            region: ForumRegion?,
            fuelProduct: String?,
            stationId: String?,
            publicAlias: String?,
        ): ForumTopicReceipt {
            return onReceiveCreate?.invoke(title, bodyText, category, operationId, region, fuelProduct, stationId, publicAlias)
                ?: sampleReceipt()
        }

        override suspend fun editTopic(
            topicId: String,
            expectedRevision: Long,
            title: String,
            bodyText: String,
            category: ForumCategory,
            operationId: String,
            region: ForumRegion?,
            fuelProduct: String?,
            stationId: String?,
            publicAlias: String?,
        ): ForumTopicReceipt {
            return onReceiveEdit?.invoke(topicId, expectedRevision, title, bodyText, category, operationId, region, fuelProduct, stationId, publicAlias)
                ?: sampleReceipt(id = topicId, revision = expectedRevision + 1)
        }

        override suspend fun removeTopic(
            topicId: String,
            operationId: String,
        ): ForumTopicReceipt {
            return onReceiveRemove?.invoke(topicId, operationId)
                ?: sampleReceipt(id = topicId, state = "REMOVED")
        }

        override suspend fun getOwnerTopic(topicId: String): ForumTopicReceipt {
            return onReceiveOwnerRead?.invoke(topicId)
                ?: sampleReceipt(id = topicId)
        }
    }

    @Test
    fun `submit topic - valid inputs succeeds and delegates to gateway`() = runTest {
        var capturedTitle: String? = null
        var capturedCategory: ForumCategory? = null
        var capturedRegion: ForumRegion? = null
        var capturedFuel: String? = null

        val gateway = mockGateway(
            onReceiveCreate = { title, body, cat, opId, reg, fuel, station, alias ->
                capturedTitle = title
                capturedCategory = cat
                capturedRegion = reg
                capturedFuel = fuel
                sampleReceipt()
            }
        )

        val useCase = SubmitForumTopicUseCase(gateway)
        val receipt = useCase(
            title = "  Postos baratos no centro  ",
            bodyText = "  Encontrei etanol a R\$ 3,49 na Av. Paulista  ",
            category = ForumCategory.FUELS,
            region = ForumRegion("sp", 3550308),
            fuelProduct = "ETHANOL",
            stationId = validStationId,
            publicAlias = "Alex",
        )

        assertNotNull(receipt)
        assertEquals("Postos baratos no centro", capturedTitle)
        assertEquals(ForumCategory.FUELS, capturedCategory)
        assertEquals("SP", capturedRegion?.uf)
        assertEquals(3550308, capturedRegion?.municipalityIbge)
        assertEquals("ETHANOL", capturedFuel)
    }

    @Test
    fun `submit topic - boundary validation`() {
        val useCase = SubmitForumTopicUseCase(mockGateway())

        // Empty title
        assertThrows(ForumException.Invalid::class.java) {
            runBlocking { useCase(title = "   ", bodyText = "valid body", category = ForumCategory.FUELS) }
        }

        // Title too long (> 120 chars)
        assertThrows(ForumException.Invalid::class.java) {
            runBlocking { useCase(title = "A".repeat(121), bodyText = "valid body", category = ForumCategory.FUELS) }
        }

        // Empty body
        assertThrows(ForumException.Invalid::class.java) {
            runBlocking { useCase(title = "Valid title", bodyText = "   ", category = ForumCategory.FUELS) }
        }

        // Body too long (> 4000 chars)
        assertThrows(ForumException.Invalid::class.java) {
            runBlocking { useCase(title = "Valid title", bodyText = "A".repeat(4001), category = ForumCategory.FUELS) }
        }

        // Category UNKNOWN
        assertThrows(ForumException.Invalid::class.java) {
            runBlocking { useCase(title = "Valid title", bodyText = "Valid body", category = ForumCategory.UNKNOWN) }
        }

        // Bad operationId
        assertThrows(ForumException.Invalid::class.java) {
            runBlocking { useCase(title = "Valid title", bodyText = "Valid body", category = ForumCategory.FUELS, operationId = "not-a-uuid") }
        }

        // Invalid UF
        assertThrows(ForumException.Invalid::class.java) {
            runBlocking { useCase(title = "Valid title", bodyText = "Valid body", category = ForumCategory.FUELS, region = ForumRegion("XX")) }
        }

        // Invalid IBGE range
        assertThrows(ForumException.Invalid::class.java) {
            runBlocking { useCase(title = "Valid title", bodyText = "Valid body", category = ForumCategory.FUELS, region = ForumRegion(municipalityIbge = 999)) }
        }

        // Mismatched IBGE prefix (SP is 35, RJ code is 3304557)
        assertThrows(ForumException.Invalid::class.java) {
            runBlocking { useCase(title = "Valid title", bodyText = "Valid body", category = ForumCategory.FUELS, region = ForumRegion("SP", 3304557)) }
        }

        // Invalid fuel product
        assertThrows(ForumException.Invalid::class.java) {
            runBlocking { useCase(title = "Valid title", bodyText = "Valid body", category = ForumCategory.FUELS, fuelProduct = "KEROSENE") }
        }

        // Invalid station ID
        assertThrows(ForumException.Invalid::class.java) {
            runBlocking { useCase(title = "Valid title", bodyText = "Valid body", category = ForumCategory.FUELS, stationId = "bad-uuid") }
        }
    }

    @Test
    fun `edit topic - validates topicId, expectedRevision and boundaries`() = runTest {
        var capturedRevision: Long = 0
        val gateway = mockGateway(
            onReceiveEdit = { id, rev, title, body, cat, opId, reg, fuel, station, alias ->
                capturedRevision = rev
                sampleReceipt(id = id, revision = rev + 1)
            }
        )
        val useCase = EditForumTopicUseCase(gateway)

        // Valid edit
        val receipt = useCase(
            topicId = validTopicId,
            expectedRevision = 2L,
            title = "Título atualizado",
            bodyText = "Conteúdo atualizado",
            category = ForumCategory.ELECTRIC,
        )
        assertEquals(2L, capturedRevision)
        assertEquals(3L, receipt.revision)

        // Invalid topic ID
        assertThrows(ForumException.Invalid::class.java) {
            runBlocking {
                useCase(
                    topicId = "invalid-uuid",
                    expectedRevision = 1L,
                    title = "Título",
                    bodyText = "Conteúdo",
                    category = ForumCategory.ELECTRIC,
                )
            }
        }

        // Invalid expectedRevision < 1
        assertThrows(ForumException.Invalid::class.java) {
            runBlocking {
                useCase(
                    topicId = validTopicId,
                    expectedRevision = 0L,
                    title = "Título",
                    bodyText = "Conteúdo",
                    category = ForumCategory.ELECTRIC,
                )
            }
        }
    }

    @Test
    fun `remove topic - validates topicId and operationId`() = runTest {
        var removedId: String? = null
        val gateway = mockGateway(
            onReceiveRemove = { id, opId ->
                removedId = id
                sampleReceipt(id = id, state = "REMOVED")
            }
        )
        val useCase = RemoveForumTopicUseCase(gateway)

        val receipt = useCase(validTopicId)
        assertEquals(validTopicId, removedId)
        assertEquals("REMOVED", receipt.publicationState)

        // Invalid UUID
        assertThrows(ForumException.Invalid::class.java) {
            runBlocking { useCase("bad-uuid") }
        }
    }

    @Test
    fun `get owner topic - validates topicId and returns receipt`() = runTest {
        var readId: String? = null
        val gateway = mockGateway(
            onReceiveOwnerRead = { id ->
                readId = id
                sampleReceipt(id = id)
            }
        )
        val useCase = GetOwnerTopicUseCase(gateway)

        val receipt = useCase(validTopicId)
        assertEquals(validTopicId, readId)
        assertEquals(validTopicId, receipt.id)

        // Invalid UUID
        assertThrows(ForumException.Invalid::class.java) {
            runBlocking { useCase("bad-uuid") }
        }
    }
}
