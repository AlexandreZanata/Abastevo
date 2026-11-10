package com.anpfuel.app.ui.forum

import com.anpfuel.application.usecase.forum.GetForumCategoriesUseCase
import com.anpfuel.application.usecase.forum.GetForumFeedUseCase
import com.anpfuel.domain.forum.ForumCategoriesResult
import com.anpfuel.domain.forum.ForumCategory
import com.anpfuel.domain.forum.ForumCategoryItem
import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.forum.ForumFeedFilters
import com.anpfuel.domain.forum.ForumFeedPage
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
class ForumFeedViewModelTest {

    private val dispatcher = StandardTestDispatcher()
    private val getForumCategories = mockk<GetForumCategoriesUseCase>()
    private val getForumFeed = mockk<GetForumFeedUseCase>()

    private val sampleCategoryItem = ForumCategoryItem(
        code = ForumCategory.FUELS,
        rawCode = "FUELS",
        name = "Combustíveis",
        description = "Discussões sobre combustíveis",
    )
    private val samplePost1 = ForumPostSummary(
        id = "11111111-1111-1111-1111-111111111111",
        kind = ForumPostKind.EDITORIAL,
        rawKind = "EDITORIAL",
        category = ForumCategory.FUELS,
        rawCategory = "FUELS",
        title = "Post 1",
        excerpt = "Excerpt 1",
        firstPublishedAt = "2026-11-01T10:00:00Z",
        updatedAt = "2026-11-01T10:00:00Z",
        source = ForumSourceAttribution(name = "Abastevo", url = "https://abastevo.com.br", attribution = "Abastevo Editorial"),
    )
    private val samplePost2 = ForumPostSummary(
        id = "22222222-2222-2222-2222-222222222222",
        kind = ForumPostKind.NEWS,
        rawKind = "NEWS",
        category = ForumCategory.FUELS,
        rawCategory = "FUELS",
        title = "Post 2",
        excerpt = "Excerpt 2",
        firstPublishedAt = "2026-11-02T10:00:00Z",
        updatedAt = "2026-11-02T10:00:00Z",
        source = ForumSourceAttribution(name = "Agência Brasil", url = "https://agenciabrasil.ebc.com.br/artigo", attribution = "CC BY"),
    )

    @BeforeEach
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        coEvery { getForumCategories() } returns ForumCategoriesResult(
            items = listOf(sampleCategoryItem),
            taxonomyVersion = 1,
        )
        coEvery { getForumFeed(any(), any(), any(), any(), any()) } returns ForumFeedPage(
            items = listOf(samplePost1),
            nextCursor = "cursor-1",
            snapshot = "snap-1",
            fetchedAt = "2026-11-05T12:00:00Z",
        )
    }

    @AfterEach
    fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test
    fun `initial load fetches categories and first page of posts`() = runTest(dispatcher) {
        val vm = ForumFeedViewModel(getForumCategories, getForumFeed)
        advanceUntilIdle()

        val state = vm.state.value
        assertFalse(state.loading)
        assertNull(state.error)
        assertEquals(1, state.categories.size)
        assertEquals(ForumCategory.FUELS, state.categories.first().code)
        assertEquals(1, state.items.size)
        assertEquals(samplePost1.id, state.items.first().id)
        assertEquals("cursor-1", state.nextCursor)
    }

    @Test
    fun `categories drop unknown and duplicate codes so chip keys stay unique`() = runTest(dispatcher) {
        fun item(raw: String) = ForumCategoryItem(
            code = ForumCategory.fromWire(raw),
            rawCode = raw,
            name = raw,
            description = "",
        )
        coEvery { getForumCategories() } returns ForumCategoriesResult(
            items = listOf(item("FUELS"), item("FUTURE_A"), item("FUTURE_B"), item("FUELS"), item("ELECTRIC")),
            taxonomyVersion = 2,
        )

        val vm = ForumFeedViewModel(getForumCategories, getForumFeed)
        advanceUntilIdle()

        assertEquals(
            listOf(ForumCategory.FUELS, ForumCategory.ELECTRIC),
            vm.state.value.categories.map { it.code },
        )
    }

    @Test
    fun `selectCategory resets pagination and fetches filtered page`() = runTest(dispatcher) {
        val vm = ForumFeedViewModel(getForumCategories, getForumFeed)
        advanceUntilIdle()

        coEvery { getForumFeed(ForumCategory.FUELS, null, 20, null, ForumFeedFilters()) } returns ForumFeedPage(
            items = listOf(samplePost2),
            nextCursor = null,
            snapshot = "snap-2",
            fetchedAt = "2026-11-05T12:00:00Z",
        )

        vm.selectCategory(ForumCategory.FUELS)
        advanceUntilIdle()

        val state = vm.state.value
        assertEquals(ForumCategory.FUELS, state.selectedCategory)
        assertEquals(1, state.items.size)
        assertEquals(samplePost2.id, state.items.first().id)
        assertNull(state.nextCursor)
    }

    @Test
    fun `refresh replaces snapshot and preserves categories`() = runTest(dispatcher) {
        val vm = ForumFeedViewModel(getForumCategories, getForumFeed)
        advanceUntilIdle()

        coEvery { getForumFeed(null, null, 20, null, ForumFeedFilters()) } returns ForumFeedPage(
            items = listOf(samplePost2),
            nextCursor = "cursor-2",
            snapshot = "snap-3",
            fetchedAt = "2026-11-05T12:00:00Z",
        )

        vm.refresh()
        advanceUntilIdle()

        val state = vm.state.value
        assertFalse(state.refreshing)
        assertEquals(1, state.items.size)
        assertEquals(samplePost2.id, state.items.first().id)
        assertEquals("cursor-2", state.nextCursor)
    }

    @Test
    fun `loadMore appends unique items and updates nextCursor`() = runTest(dispatcher) {
        val vm = ForumFeedViewModel(getForumCategories, getForumFeed)
        advanceUntilIdle()

        coEvery { getForumFeed(null, null, 20, "cursor-1", ForumFeedFilters()) } returns ForumFeedPage(
            items = listOf(samplePost2),
            nextCursor = "cursor-final",
            snapshot = "snap-4",
            fetchedAt = "2026-11-05T12:00:00Z",
        )

        vm.loadMore()
        advanceUntilIdle()

        val state = vm.state.value
        assertFalse(state.loadingMore)
        assertEquals(2, state.items.size)
        assertEquals(samplePost1.id, state.items[0].id)
        assertEquals(samplePost2.id, state.items[1].id)
        assertEquals("cursor-final", state.nextCursor)
    }

    @Test
    fun `offline with cached items keeps items and sets isOffline`() = runTest(dispatcher) {
        val vm = ForumFeedViewModel(getForumCategories, getForumFeed)
        advanceUntilIdle()

        coEvery { getForumFeed(null, null, 20, null, ForumFeedFilters()) } throws ForumException.Offline("no network")

        vm.refresh()
        advanceUntilIdle()

        val state = vm.state.value
        assertTrue(state.isOffline)
        assertEquals(1, state.items.size)
    }

    @Test
    fun `disabled error sets disabled state`() = runTest(dispatcher) {
        coEvery { getForumCategories() } throws ForumException.Disabled("forum disabled")

        val vm = ForumFeedViewModel(getForumCategories, getForumFeed)
        advanceUntilIdle()

        val state = vm.state.value
        assertFalse(state.loading)
        assertEquals(ForumUiError.DISABLED, state.error)
    }

    @Test
    fun `empty result shows empty state without error`() = runTest(dispatcher) {
        coEvery { getForumFeed(any(), any(), any(), any(), any()) } returns ForumFeedPage(
            items = emptyList(),
            nextCursor = null,
            snapshot = "snap-empty",
            fetchedAt = "2026-11-05T12:00:00Z",
        )

        val vm = ForumFeedViewModel(getForumCategories, getForumFeed)
        advanceUntilIdle()

        val state = vm.state.value
        assertFalse(state.loading)
        assertNull(state.error)
        assertTrue(state.items.isEmpty())
    }
}
