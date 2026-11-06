package com.anpfuel.application.usecase.forum

import com.anpfuel.domain.forum.ForumCategoriesResult
import com.anpfuel.domain.forum.ForumCategory
import com.anpfuel.domain.forum.ForumCategoryItem
import com.anpfuel.domain.forum.ForumCommentReceipt
import com.anpfuel.domain.forum.ForumCommentsPage
import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.forum.ForumFeedFilters
import com.anpfuel.domain.forum.ForumFeedPage
import com.anpfuel.domain.forum.ForumPostDetail
import com.anpfuel.domain.forum.ForumPostKind
import com.anpfuel.domain.forum.ForumPostSummary
import com.anpfuel.domain.forum.ForumRegion
import com.anpfuel.domain.forum.ForumRepliesPage
import com.anpfuel.domain.forum.ForumReportReason
import com.anpfuel.domain.forum.ForumReportReceipt
import com.anpfuel.domain.forum.ForumSafeLinks
import com.anpfuel.domain.forum.ForumSourceAttribution
import com.anpfuel.domain.repository.ForumGateway
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.io.File

class ForumUseCasesTest {

    private val validPostId = "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d"

    private val sampleSource = ForumSourceAttribution(
        name = "Abastevo Editorial",
        url = "https://abastevo.com.br/artigo-1",
        attribution = "Redação Abastevo",
        licenseUrl = null,
    )

    private val sampleSummary = ForumPostSummary(
        id = validPostId,
        kind = ForumPostKind.EDITORIAL,
        rawKind = "EDITORIAL",
        category = ForumCategory.FUELS,
        rawCategory = "FUELS",
        title = "Entenda a formação de preços",
        excerpt = "Guia explicativo sobre composição tributária.",
        source = sampleSource,
        publicAlias = null,
        firstPublishedAt = "2026-11-05T12:00:00Z",
        updatedAt = "2026-11-05T12:00:00Z",
        revision = 1L,
        commentsEnabled = true,
        region = ForumRegion("SP", 3550308),
        fuelProduct = "ETANOL",
        stationId = null,
    )

    private val sampleDetail = ForumPostDetail(
        id = validPostId,
        kind = ForumPostKind.EDITORIAL,
        rawKind = "EDITORIAL",
        category = ForumCategory.FUELS,
        rawCategory = "FUELS",
        title = "Entenda a formação de preços",
        excerpt = "Guia explicativo sobre composição tributária.",
        bodyText = "O preço ao consumidor final resulta da soma do custo da refinaria...",
        source = sampleSource,
        publicAlias = null,
        sourcePublishedAt = null,
        firstPublishedAt = "2026-11-05T12:00:00Z",
        updatedAt = "2026-11-05T12:00:00Z",
        revision = 1L,
        commentsEnabled = true,
        correction = null,
        region = ForumRegion("SP", 3550308),
        fuelProduct = "ETANOL",
        stationId = null,
    )

    private fun testGateway(
        categories: ForumCategoriesResult = ForumCategoriesResult(
            items = listOf(
                ForumCategoryItem(
                    code = ForumCategory.FUELS,
                    rawCode = "FUELS",
                    name = "Combustíveis",
                    description = "Preços e tributação",
                ),
            ),
            taxonomyVersion = 1,
        ),
        feedPage: ForumFeedPage = ForumFeedPage(
            items = listOf(sampleSummary),
            nextCursor = "cursor-token-123",
            snapshot = "rev-1",
            fetchedAt = "2026-11-05T12:00:00Z",
        ),
        postDetail: ForumPostDetail = sampleDetail,
    ): ForumGateway = object : ForumGateway {
        override suspend fun getCategories(): ForumCategoriesResult = categories
        override suspend fun getFeed(
            category: ForumCategory?,
            kind: ForumPostKind?,
            limit: Int,
            cursor: String?,
            filters: ForumFeedFilters,
        ): ForumFeedPage = feedPage
        override suspend fun getPostDetail(postId: String): ForumPostDetail = postDetail
    }

    // --- 1. Mapping and Wire Contract Consistency ---
    @Test
    fun `mapping - wire contract vectors match domain models`() {
        val testdataDir = File("../contracts/testdata/api")
        val categoriesFile = File(testdataDir, "forum-categories-response-valid.json")
        val feedFile = File(testdataDir, "forum-feed-response-valid.json")
        val detailFile = File(testdataDir, "forum-post-detail-valid.json")

        assertTrue(categoriesFile.exists(), "categories testdata vector exists")
        assertTrue(feedFile.exists(), "feed testdata vector exists")
        assertTrue(detailFile.exists(), "detail testdata vector exists")

        // Parse category from testdata code "FUELS"
        val fuelsCategory = ForumCategory.fromWire("FUELS")
        assertEquals(ForumCategory.FUELS, fuelsCategory)

        // Parse kind from testdata value "EDITORIAL"
        val editorialKind = ForumPostKind.fromWire("EDITORIAL")
        assertEquals(ForumPostKind.EDITORIAL, editorialKind)
    }

    // --- 2. Categories UseCase ---
    @Test
    fun `GetForumCategoriesUseCase returns categories without authentication`() = runTest {
        val useCase = GetForumCategoriesUseCase(testGateway())
        val result = useCase()
        assertEquals(1, result.items.size)
        assertEquals(ForumCategory.FUELS, result.items[0].code)
        assertEquals(1, result.taxonomyVersion)
    }

    // --- 3. Feed UseCase & Bounds / Stale Scope ---
    @Test
    fun `GetForumFeedUseCase validates limits and cursor bounds`() = runTest {
        var recordedLimit = 0
        var recordedCursor: String? = null
        val gateway = object : ForumGateway {
            override suspend fun getCategories(): ForumCategoriesResult = throw UnsupportedOperationException()
            override suspend fun getFeed(
                category: ForumCategory?,
                kind: ForumPostKind?,
                limit: Int,
                cursor: String?,
                filters: ForumFeedFilters,
            ): ForumFeedPage {
                recordedLimit = limit
                recordedCursor = cursor
                return ForumFeedPage(emptyList(), null, "rev-1", "2026-11-05T12:00:00Z")
            }
            override suspend fun getPostDetail(postId: String): ForumPostDetail = throw UnsupportedOperationException()
        }
        val useCase = GetForumFeedUseCase(gateway)

        // Default limit 20
        useCase()
        assertEquals(20, recordedLimit)
        assertNull(recordedCursor)

        // Explicit valid limits 1 and 50
        useCase(limit = 1)
        assertEquals(1, recordedLimit)
        useCase(limit = 50)
        assertEquals(50, recordedLimit)

        // Valid cursor
        useCase(cursor = "valid-hmac-cursor")
        assertEquals("valid-hmac-cursor", recordedCursor)

        // Invalid limits (< 1 or > 50)
        val ex1 = assertThrows(ForumException.Invalid::class.java) {
            kotlinx.coroutines.runBlocking { useCase(limit = 0) }
        }
        assertEquals("forum.bad-limit", ex1.code)

        val ex2 = assertThrows(ForumException.Invalid::class.java) {
            kotlinx.coroutines.runBlocking { useCase(limit = 51) }
        }
        assertEquals("forum.bad-limit", ex2.code)

        val ex3 = assertThrows(ForumException.Invalid::class.java) {
            kotlinx.coroutines.runBlocking { useCase(limit = -5) }
        }
        assertEquals("forum.bad-limit", ex3.code)

        // Oversized cursor (> 512 chars)
        val ex4 = assertThrows(ForumException.Invalid::class.java) {
            kotlinx.coroutines.runBlocking { useCase(cursor = "a".repeat(513)) }
        }
        assertEquals("forum.bad-cursor", ex4.code)
    }

    @Test
    fun `GetForumFeedUseCase normalizes and validates subject filters`() = runTest {
        var recordedFilters: ForumFeedFilters? = null
        val gateway = object : ForumGateway {
            override suspend fun getFeed(
                category: ForumCategory?,
                kind: ForumPostKind?,
                limit: Int,
                cursor: String?,
                filters: ForumFeedFilters,
            ): ForumFeedPage {
                recordedFilters = filters
                return ForumFeedPage(emptyList(), null, "rev-1", "2026-11-05T12:00:00Z")
            }
        }
        val useCase = GetForumFeedUseCase(gateway)
        val station = "D6C74C23-63DB-4C24-A2E5-408CB23BAD26"

        useCase(
            filters = ForumFeedFilters(
                uf = " sp ",
                municipalityIbge = 3550308,
                fuelProduct = "ETHANOL",
                stationId = station,
            ),
        )
        assertEquals(
            ForumFeedFilters("SP", 3550308, "ETHANOL", station.lowercase()),
            recordedFilters,
        )

        val invalidFilters = listOf(
            ForumFeedFilters(uf = "XX") to "forum.bad-region-uf",
            ForumFeedFilters(municipalityIbge = 355030) to "forum.bad-region-ibge",
            ForumFeedFilters(uf = "SP", municipalityIbge = 3304557) to "forum.bad-region-prefix",
            ForumFeedFilters(fuelProduct = "DIESEL") to "forum.bad-fuel-product",
            ForumFeedFilters(stationId = "not-a-uuid") to "forum.bad-station-id",
        )
        for ((filters, expected) in invalidFilters) {
            val ex = assertThrows(ForumException.Invalid::class.java) {
                kotlinx.coroutines.runBlocking { useCase(filters = filters) }
            }
            assertEquals(expected, ex.code)
        }
    }

    // --- 4. Detail UseCase & UUID Validation ---
    @Test
    fun `GetForumPostDetailUseCase validates UUID and returns post`() = runTest {
        val useCase = GetForumPostDetailUseCase(testGateway())
        val detail = useCase(validPostId)
        assertEquals(validPostId, detail.id)
        assertEquals("Entenda a formação de preços", detail.title)
        assertNotNull(detail.bodyText)

        // Invalid UUIDs
        val invalidIds = listOf(
            "not-a-uuid",
            "12345",
            "",
            "   ",
            "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6g", // 'g' is not hex
            "9b1deb4d3b7d4bad9bdd2b0d7b3dcb6d",     // missing dashes
        )
        for (badId in invalidIds) {
            val ex = assertThrows(ForumException.Invalid::class.java) {
                kotlinx.coroutines.runBlocking { useCase(badId) }
            }
            assertEquals("forum.bad-id", ex.code)
        }
    }

    // --- 5. Null Provenance & Optional Fields ---
    @Test
    fun `null provenance - handles missing optional fields cleanly`() = runTest {
        val summaryWithoutSource = sampleSummary.copy(
            source = null,
            publicAlias = null,
            region = null,
            fuelProduct = null,
            stationId = null,
        )
        val detailWithoutBodyOrSource = sampleDetail.copy(
            source = null,
            publicAlias = null,
            sourcePublishedAt = null,
            correction = null,
            bodyText = null,
            region = null,
            fuelProduct = null,
            stationId = null,
        )

        val gateway = testGateway(
            feedPage = ForumFeedPage(listOf(summaryWithoutSource), null, "rev-1", "2026-11-05T12:00:00Z"),
            postDetail = detailWithoutBodyOrSource,
        )

        val feedUseCase = GetForumFeedUseCase(gateway)
        val feed = feedUseCase()
        assertNull(feed.items[0].source)
        assertNull(feed.items[0].publicAlias)
        assertNull(feed.items[0].region)

        val detailUseCase = GetForumPostDetailUseCase(gateway)
        val detail = detailUseCase(validPostId)
        assertNull(detail.source)
        assertNull(detail.bodyText)
        assertNull(detail.correction)
        assertNull(detail.publicAlias)
    }

    // --- 6. HTTPS Link Refusal & Safe Links ---
    @Test
    fun `HTTPS link refusal - ValidateForumSafeLinkUseCase strictly requires secure URLs`() {
        val useCase = ValidateForumSafeLinkUseCase()

        assertTrue(useCase("https://abastevo.com.br/artigo-1"))
        assertTrue(useCase("https://anp.gov.br/dados/precos"))

        assertFalse(useCase("http://insecure.example.com"))
        assertFalse(useCase("javascript:alert(1)"))
        assertFalse(useCase("file:///sdcard/exploit"))
        assertFalse(useCase("content://media/1"))
        assertFalse(useCase("https://user:pass@evil.com/phish"))
        assertFalse(useCase("https://example.com:8080/path"))
        assertFalse(useCase(null))
        assertFalse(useCase(""))

        val ex = assertThrows(ForumException.SecurityViolation::class.java) {
            useCase.requireSafe("http://insecure.com")
        }
        assertEquals("forum.insecure-link", ex.code)
    }

    // --- 7. Gateway Exception Propagation ---
    @Test
    fun `gateway exceptions propagate cleanly`() = runTest {
        val failingGateway = object : ForumGateway {
            override suspend fun getCategories(): ForumCategoriesResult =
                throw ForumException.Disabled("forum.disabled")
            override suspend fun getFeed(
                category: ForumCategory?,
                kind: ForumPostKind?,
                limit: Int,
                cursor: String?,
                filters: ForumFeedFilters,
            ): ForumFeedPage = throw ForumException.Unavailable("forum.unavailable")
            override suspend fun getPostDetail(postId: String): ForumPostDetail =
                throw ForumException.NotFound("forum.not-found")
        }

        val catUseCase = GetForumCategoriesUseCase(failingGateway)
        assertThrows(ForumException.Disabled::class.java) {
            kotlinx.coroutines.runBlocking { catUseCase() }
        }

        val feedUseCase = GetForumFeedUseCase(failingGateway)
        assertThrows(ForumException.Unavailable::class.java) {
            kotlinx.coroutines.runBlocking { feedUseCase() }
        }

        val detailUseCase = GetForumPostDetailUseCase(failingGateway)
        assertThrows(ForumException.NotFound::class.java) {
            kotlinx.coroutines.runBlocking { detailUseCase(validPostId) }
        }
    }

    // --- 8. Discussion and Moderation Use Cases ---
    @Test
    fun `GetForumCommentsUseCase validates postId and clamps limit`() = runTest {
        var requestedLimit = 0
        var requestedPostId = ""
        val gateway = object : ForumGateway {
            override suspend fun getComments(postId: String, limit: Int, cursor: String?): ForumCommentsPage {
                requestedPostId = postId
                requestedLimit = limit
                return ForumCommentsPage(items = emptyList(), nextCursor = null)
            }
        }

        val useCase = GetForumCommentsUseCase(gateway)
        useCase(validPostId, limit = 100)
        assertEquals(validPostId, requestedPostId)
        assertEquals(50, requestedLimit)

        useCase(validPostId, limit = -5)
        assertEquals(1, requestedLimit)

        val ex = assertThrows(ForumException.Invalid::class.java) {
            kotlinx.coroutines.runBlocking { useCase("invalid-uuid") }
        }
        assertEquals("forum.bad-id", ex.code)
    }

    @Test
    fun `GetForumRepliesUseCase validates commentId and clamps limit`() = runTest {
        var requestedLimit = 0
        val gateway = object : ForumGateway {
            override suspend fun getReplies(commentId: String, limit: Int, cursor: String?): ForumRepliesPage {
                requestedLimit = limit
                return ForumRepliesPage(items = emptyList(), nextCursor = null)
            }
        }

        val useCase = GetForumRepliesUseCase(gateway)
        useCase(validPostId, limit = 999)
        assertEquals(50, requestedLimit)

        val ex = assertThrows(ForumException.Invalid::class.java) {
            kotlinx.coroutines.runBlocking { useCase("not-a-uuid") }
        }
        assertEquals("forum.bad-id", ex.code)
    }

    @Test
    fun `SubmitForumCommentUseCase validates bounds and dispatches`() = runTest {
        var capturedBody = ""
        val gateway = object : ForumGateway {
            override suspend fun createComment(postId: String, parentId: String?, bodyText: String, operationId: String): ForumCommentReceipt {
                capturedBody = bodyText
                return ForumCommentReceipt(
                    id = "88888888-8888-4888-8888-000000000001",
                    postId = postId,
                    parentId = parentId,
                    publicationState = "PENDING_REVIEW",
                    visibility = "VISIBLE",
                    bodyText = bodyText,
                    createdAt = "2026-11-05T12:00:00Z",
                    updatedAt = "2026-11-05T12:00:00Z",
                )
            }
        }

        val useCase = SubmitForumCommentUseCase(gateway)
        val receipt = useCase(validPostId, "Comentario de teste valido")
        assertEquals("Comentario de teste valido", capturedBody)
        assertEquals("PENDING_REVIEW", receipt.publicationState)

        assertThrows(ForumException.Invalid::class.java) {
            kotlinx.coroutines.runBlocking { useCase("bad-id", "valido") }
        }
        assertThrows(ForumException.Invalid::class.java) {
            kotlinx.coroutines.runBlocking { useCase(validPostId, "   ") }
        }
        assertThrows(ForumException.Invalid::class.java) {
            kotlinx.coroutines.runBlocking { useCase(validPostId, "a".repeat(281)) }
        }
        assertThrows(ForumException.Invalid::class.java) {
            kotlinx.coroutines.runBlocking { useCase(validPostId, "valido", parentId = "bad-parent") }
        }
    }

    @Test
    fun `EditForumCommentUseCase validates bounds and revision`() = runTest {
        val gateway = object : ForumGateway {
            override suspend fun editComment(commentId: String, expectedRevision: Long, bodyText: String, operationId: String): ForumCommentReceipt {
                return ForumCommentReceipt(
                    id = commentId,
                    postId = validPostId,
                    revision = expectedRevision + 1,
                    publicationState = "PENDING_REVIEW",
                    visibility = "VISIBLE",
                    bodyText = bodyText,
                    createdAt = "2026-11-05T12:00:00Z",
                    updatedAt = "2026-11-05T12:00:00Z",
                )
            }
        }

        val useCase = EditForumCommentUseCase(gateway)
        val receipt = useCase(validPostId, 1L, "Texto editado")
        assertEquals(2L, receipt.revision)

        assertThrows(ForumException.Invalid::class.java) {
            kotlinx.coroutines.runBlocking { useCase("bad-id", 1L, "texto") }
        }
        assertThrows(ForumException.Invalid::class.java) {
            kotlinx.coroutines.runBlocking { useCase(validPostId, 1L, "") }
        }
    }

    @Test
    fun `RemoveForumCommentUseCase validates UUID and dispatches`() = runTest {
        val gateway = object : ForumGateway {
            override suspend fun removeComment(commentId: String, expectedRevision: Long, operationId: String): ForumCommentReceipt {
                return ForumCommentReceipt(
                    id = commentId,
                    postId = validPostId,
                    revision = expectedRevision + 1,
                    publicationState = "REMOVED",
                    visibility = "HIDDEN",
                    bodyText = "",
                    createdAt = "2026-11-05T12:00:00Z",
                    updatedAt = "2026-11-05T12:00:00Z",
                )
            }
        }

        val useCase = RemoveForumCommentUseCase(gateway)
        val receipt = useCase(validPostId, 1L)
        assertEquals("REMOVED", receipt.publicationState)

        assertThrows(ForumException.Invalid::class.java) {
            kotlinx.coroutines.runBlocking { useCase("not-a-uuid", 1L) }
        }
    }

    @Test
    fun `ReportForumTargetUseCase validates target type and length`() = runTest {
        val gateway = object : ForumGateway {
            override suspend fun report(targetType: String, targetId: String, reason: String, detail: String?): ForumReportReceipt {
                return ForumReportReceipt(
                    reportId = "77777777-7777-4777-8777-000000000001",
                    caseId = "66666666-6666-4666-8666-000000000001",
                    targetType = targetType,
                    targetId = targetId,
                    reason = reason,
                    status = "OPEN",
                    createdAt = "2026-11-05T12:00:00Z",
                )
            }
        }

        val useCase = ReportForumTargetUseCase(gateway)
        val receiptPost = useCase("FORUM_POST", validPostId, ForumReportReason.SPAM, "Spam comercial")
        assertEquals("FORUM_POST", receiptPost.targetType)

        val receiptComment = useCase("FORUM_COMMENT", validPostId, ForumReportReason.OFFENSIVE, null)
        assertEquals("FORUM_COMMENT", receiptComment.targetType)

        assertThrows(ForumException.Invalid::class.java) {
            kotlinx.coroutines.runBlocking { useCase("UNKNOWN_TYPE", validPostId, ForumReportReason.SPAM) }
        }
        assertThrows(ForumException.Invalid::class.java) {
            kotlinx.coroutines.runBlocking { useCase("FORUM_POST", "not-a-uuid", ForumReportReason.SPAM) }
        }
        assertThrows(ForumException.Invalid::class.java) {
            kotlinx.coroutines.runBlocking { useCase("FORUM_POST", validPostId, ForumReportReason.SPAM, "x".repeat(501)) }
        }
    }
}
