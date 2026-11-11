package com.anpfuel.app.ui.forum

import androidx.lifecycle.SavedStateHandle
import com.anpfuel.application.usecase.forum.GetForumCategoriesUseCase
import com.anpfuel.application.usecase.forum.GetForumFeedUseCase
import com.anpfuel.application.usecase.forum.GetForumPostDetailUseCase
import com.anpfuel.application.usecase.forum.ValidateForumSafeLinkUseCase
import com.anpfuel.domain.forum.ForumCategoriesResult
import com.anpfuel.domain.forum.ForumCategory
import com.anpfuel.domain.forum.ForumCategoryItem
import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.forum.ForumFeedPage
import com.anpfuel.domain.forum.ForumPostDetail
import com.anpfuel.domain.forum.ForumPostKind
import com.anpfuel.domain.forum.ForumPostSummary
import com.anpfuel.domain.forum.ForumSourceAttribution
import io.mockk.coEvery
import io.mockk.mockk
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class ForumEditorialMvpAcceptanceTest {

    private val dispatcher = StandardTestDispatcher()
    private val getForumCategories = mockk<GetForumCategoriesUseCase>()
    private val getForumFeed = mockk<GetForumFeedUseCase>()
    private val getForumPostDetail = mockk<GetForumPostDetailUseCase>()
    private val validateForumSafeLink = ValidateForumSafeLinkUseCase()

    private val postId = "99999999-9999-4999-8999-000000000001"
    private val sampleCategory = ForumCategoryItem(
        code = ForumCategory.FUELS,
        rawCode = "FUELS",
        name = "Combustíveis",
        description = "Preços e tendências",
    )
    private val samplePost = ForumPostSummary(
        id = postId,
        kind = ForumPostKind.EDITORIAL,
        rawKind = "EDITORIAL",
        category = ForumCategory.FUELS,
        rawCategory = "FUELS",
        title = "Paridade de Importação e Preços dos Combustíveis",
        excerpt = "Análise editorial completa sobre a formação de preços no Brasil.",
        source = ForumSourceAttribution(
            name = "Abastevo",
            url = "https://abastevo.com.br",
            attribution = "Redação Abastevo",
        ),
        firstPublishedAt = "2026-11-05T12:00:00Z",
        updatedAt = "2026-11-05T12:00:00Z",
    )

    private val sampleDetail = ForumPostDetail(
        id = postId,
        kind = ForumPostKind.EDITORIAL,
        rawKind = "EDITORIAL",
        category = ForumCategory.FUELS,
        rawCategory = "FUELS",
        title = "Paridade de Importação e Preços dos Combustíveis",
        excerpt = "Análise editorial completa sobre a formação de preços no Brasil.",
        bodyText = "Texto integral do artigo explicando PPI, margens de distribuição e tributos estaduais.",
        source = ForumSourceAttribution(
            name = "Abastevo",
            url = "https://abastevo.com.br",
            attribution = "Redação Abastevo",
        ),
        firstPublishedAt = "2026-11-05T12:00:00Z",
        updatedAt = "2026-11-05T12:00:00Z",
    )

    @BeforeEach
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        coEvery { getForumCategories() } returns ForumCategoriesResult(listOf(sampleCategory), 1)
    }

    @AfterEach
    fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test
    fun testEditorialAdminToAndroidPipelineAndOfflineDegradation() = runTest(dispatcher) {
        // 1. Online: feed returns live published item
        coEvery { getForumFeed(any(), any(), any(), any()) } returns ForumFeedPage(
            items = listOf(samplePost),
            nextCursor = null,
            snapshot = "rev-1",
            fetchedAt = "2026-11-05T12:00:00Z",
        )

        val feedVm = ForumFeedViewModel(getForumCategories, getForumFeed)
        advanceUntilIdle()

        val stateOnline = feedVm.state.value
        assertEquals(1, stateOnline.items.size)
        assertEquals("Paridade de Importação e Preços dos Combustíveis", stateOnline.items[0].title)
        assertFalse(stateOnline.isOffline)
        assertNull(stateOnline.error)

        // 2. Offline degradation: network loss serves Room-cached item with isOffline = true
        coEvery { getForumFeed(any(), any(), any(), any()) } throws ForumException.Offline("offline")

        feedVm.refresh()
        advanceUntilIdle()

        val stateOffline = feedVm.state.value
        assertEquals(1, stateOffline.items.size)
        assertTrue(stateOffline.isOffline)
        assertNull(stateOffline.error)
    }

    @Test
    fun testSourceLinkRefusalSecurity() {
        // Clean HTTPS links are accepted
        assertTrue(validateForumSafeLink("https://abastevo.com.br/artigos/paridade-de-importacao"))

        // Non-HTTPS schemes refused
        assertFalse(validateForumSafeLink("http://abastevo.com.br/artigo"))
        assertFalse(validateForumSafeLink("javascript:alert(1)"))
        assertFalse(validateForumSafeLink("file:///etc/passwd"))
        assertFalse(validateForumSafeLink("data:text/html,<html>"))

        // User info credentials refused (prevent credential phishing / SSRF)
        assertFalse(validateForumSafeLink("https://admin:secret@abastevo.com.br/"))

        // Custom ports refused (must use standard HTTPS port 443)
        assertFalse(validateForumSafeLink("https://abastevo.com.br:8443/artigo"))

        // Excessive length refused (> 2048 chars)
        val hugeUrl = "https://abastevo.com.br/" + "a".repeat(2050)
        assertFalse(validateForumSafeLink(hugeUrl))

        // Malformed / blank refused
        assertFalse(validateForumSafeLink(""))
        assertFalse(validateForumSafeLink("   "))
        assertFalse(validateForumSafeLink("not-a-valid-url"))
        assertFalse(validateForumSafeLink(null))
    }

    @Test
    fun testLiveCacheInvalidationAndPrivacy() = runTest(dispatcher) {
        // Post taken down or hidden on server -> surfaces 404 NotFound
        coEvery { getForumPostDetail(postId) } throws ForumException.NotFound("forum.not-found")

        val savedStateHandle = SavedStateHandle(mapOf("postId" to postId))
        val detailVm = ForumDetailViewModel(
            getForumPostDetail = getForumPostDetail,
            validateForumSafeLink = validateForumSafeLink,
            getForumReplies = mockk(relaxed = true),
            submitForumComment = mockk(relaxed = true),
            editForumComment = mockk(relaxed = true),
            removeForumComment = mockk(relaxed = true),
            reportForumTarget = mockk(relaxed = true),
            getOwnerTopic = mockk(relaxed = true),
            getOwnForumCommentIds = mockk(relaxed = true),
            getRankedComments = mockk(relaxed = true),
            setForumReaction = mockk(relaxed = true),
            getOwnForumReactions = mockk(relaxed = true),
            observeConversation = mockk(relaxed = true),
            sessionStore = null,
            savedStateHandle = savedStateHandle,
        )

        advanceUntilIdle()

        val state = detailVm.state.value
        assertNull(state.post)
        assertEquals(ForumDetailError.NOT_FOUND, state.error)
    }

    @Test
    fun testDefaultOffAndSafeRollback() = runTest(dispatcher) {
        // When forum reads are disabled by feature toggle -> returns Disabled
        coEvery { getForumFeed(any(), any(), any(), any()) } throws ForumException.Disabled("forum.disabled")

        val feedVm = ForumFeedViewModel(getForumCategories, getForumFeed)
        advanceUntilIdle()

        val state = feedVm.state.value
        assertEquals(ForumUiError.DISABLED, state.error)
        assertTrue(state.items.isEmpty())
    }
}
