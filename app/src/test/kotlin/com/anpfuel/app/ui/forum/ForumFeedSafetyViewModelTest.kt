package com.anpfuel.app.ui.forum

import com.anpfuel.app.ui.safety.FakeSafetyGateway
import com.anpfuel.application.usecase.forum.GetForumCategoriesUseCase
import com.anpfuel.application.usecase.forum.GetForumFeedUseCase
import com.anpfuel.application.usecase.safety.BlockCommunityUserUseCase
import com.anpfuel.application.usecase.safety.CommunityBlockState
import com.anpfuel.application.usecase.safety.FilterHiddenContentUseCase
import com.anpfuel.domain.forum.ForumCategoriesResult
import com.anpfuel.domain.forum.ForumCategory
import com.anpfuel.domain.forum.ForumFeedPage
import com.anpfuel.domain.forum.ForumPostKind
import com.anpfuel.domain.forum.ForumPostSummary
import com.anpfuel.domain.safety.CommunitySafetyException
import com.anpfuel.domain.safety.SafetyContentKind
import com.anpfuel.domain.safety.SafetyContentRef
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
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

/** B-BR-SAFE-06 and BUC-SAFE-03 on the forum feed: topics by blocked authors disappear. */
@OptIn(ExperimentalCoroutinesApi::class)
class ForumFeedSafetyViewModelTest {

    private val dispatcher = StandardTestDispatcher()
    private val getForumCategories = mockk<GetForumCategoriesUseCase>()
    private val getForumFeed = mockk<GetForumFeedUseCase>()
    private var account: String? = "viewer"
    private val gateway = FakeSafetyGateway()
    private val blockState = CommunityBlockState(gateway) { account }
    private val filter = FilterHiddenContentUseCase(gateway, blockState)

    private fun post(n: Int, kind: ForumPostKind) = ForumPostSummary(
        id = "00000000-0000-4000-8000-%012d".format(n),
        kind = kind,
        rawKind = kind.wireValue,
        category = ForumCategory.FUELS,
        rawCategory = "FUELS",
        title = "Post $n",
        excerpt = "Excerpt $n",
        firstPublishedAt = "2026-11-15T10:00:00Z",
        updatedAt = "2026-11-15T10:00:00Z",
    )

    private val trollTopic = post(1, ForumPostKind.TOPIC)
    private val friendTopic = post(2, ForumPostKind.TOPIC)
    private val editorial = post(3, ForumPostKind.EDITORIAL)
    private val laterTrollTopic = post(4, ForumPostKind.TOPIC)

    private fun page(vararg items: ForumPostSummary, next: String? = null) =
        ForumFeedPage(items = items.toList(), nextCursor = next, snapshot = "s", fetchedAt = "2026-11-15T10:00:00Z")

    @BeforeEach
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        gateway.authors[trollTopic.id] = "troll"
        gateway.authors[laterTrollTopic.id] = "troll"
        gateway.authors[friendTopic.id] = "friend"
        coEvery { getForumCategories() } returns ForumCategoriesResult(items = emptyList(), taxonomyVersion = 1)
        coEvery { getForumFeed(any(), any(), any(), null, any()) } returns page(trollTopic, friendTopic, editorial, next = "c1")
        coEvery { getForumFeed(any(), any(), any(), "c1", any()) } returns page(laterTrollTopic)
    }

    @AfterEach
    fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test
    fun `topics by blocked authors are removed from every page`() = runTest(dispatcher) {
        gateway.blockedAuthors += "troll"
        val vm = ForumFeedViewModel(getForumCategories, getForumFeed, filter)
        advanceUntilIdle()
        assertEquals(listOf(friendTopic.id, editorial.id), vm.state.value.items.map { it.id })

        vm.loadMore()
        advanceUntilIdle()
        assertEquals(listOf(friendTopic.id, editorial.id), vm.state.value.items.map { it.id })

        vm.refresh()
        advanceUntilIdle()
        assertEquals(listOf(friendTopic.id, editorial.id), vm.state.value.items.map { it.id })
    }

    @Test
    fun `only topics are asked about`() = runTest(dispatcher) {
        gateway.blockedAuthors += "someone"
        ForumFeedViewModel(getForumCategories, getForumFeed, filter)
        advanceUntilIdle()
        val asked = gateway.hiddenQuestions.flatten()
        assertEquals(
            setOf(SafetyContentRef(SafetyContentKind.FORUM_POST, trollTopic.id), SafetyContentRef(SafetyContentKind.FORUM_POST, friendTopic.id)),
            asked.toSet(),
        )
    }

    @Test
    fun `a block made elsewhere drops listed topics at once`() = runTest(dispatcher) {
        val vm = ForumFeedViewModel(getForumCategories, getForumFeed, filter)
        advanceUntilIdle()
        assertEquals(3, vm.state.value.items.size)

        BlockCommunityUserUseCase(gateway, blockState)(SafetyContentRef(SafetyContentKind.FORUM_POST, trollTopic.id))
        advanceUntilIdle()
        assertEquals(listOf(friendTopic.id, editorial.id), vm.state.value.items.map { it.id })
    }

    @Test
    fun `the feed fails open when the hidden question fails`() = runTest(dispatcher) {
        gateway.blockedAuthors += "troll"
        gateway.hiddenFailure = CommunitySafetyException.Offline()
        val vm = ForumFeedViewModel(getForumCategories, getForumFeed, filter)
        advanceUntilIdle()
        assertEquals(3, vm.state.value.items.size)
    }

    @Test
    fun `visitors see every topic and ask nothing`() = runTest(dispatcher) {
        account = null
        gateway.blockedAuthors += "troll"
        val vm = ForumFeedViewModel(getForumCategories, getForumFeed, filter)
        advanceUntilIdle()
        assertEquals(3, vm.state.value.items.size)
        assertTrue(gateway.hiddenQuestions.isEmpty())
        assertEquals(0, gateway.listCalls)
    }
}
