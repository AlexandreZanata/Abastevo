package com.anpfuel.app.ui.forum

import androidx.lifecycle.SavedStateHandle
import com.anpfuel.application.usecase.forum.GetForumPostDetailUseCase
import com.anpfuel.application.usecase.forum.ValidateForumSafeLinkUseCase
import com.anpfuel.domain.forum.ForumCategory
import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.forum.ForumPostDetail
import com.anpfuel.domain.forum.ForumPostKind
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
class ForumDetailViewModelTest {

    private val dispatcher = StandardTestDispatcher()
    private val getForumPostDetail = mockk<GetForumPostDetailUseCase>()
    private val validateForumSafeLink = mockk<ValidateForumSafeLinkUseCase>()

    private val postId = "11111111-1111-1111-1111-111111111111"
    private val sampleDetail = ForumPostDetail(
        id = postId,
        kind = ForumPostKind.NEWS,
        rawKind = "NEWS",
        category = ForumCategory.FUELS,
        rawCategory = "FUELS",
        title = "Title Detail",
        excerpt = "Excerpt Detail",
        bodyText = "Full Body Content Here",
        source = ForumSourceAttribution(name = "AutoEsporte", url = "https://autoesporte.globo.com/artigo", attribution = "Globo"),
        firstPublishedAt = "2026-11-01T10:00:00Z",
        updatedAt = "2026-11-01T10:00:00Z",
    )

    @BeforeEach
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        coEvery { getForumPostDetail(postId) } returns sampleDetail
        coEvery { validateForumSafeLink(any()) } returns true
    }

    @AfterEach
    fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test
    fun `initial load with postIdArg fetches post detail`() = runTest(dispatcher) {
        val savedState = SavedStateHandle(mapOf("postId" to postId))
        val vm = ForumDetailViewModel(getForumPostDetail, validateForumSafeLink, savedState)
        advanceUntilIdle()

        val state = vm.state.value
        assertFalse(state.loading)
        assertNull(state.error)
        assertEquals(sampleDetail, state.post)
    }

    @Test
    fun `not found error marks error as NOT_FOUND`() = runTest(dispatcher) {
        coEvery { getForumPostDetail(postId) } throws ForumException.NotFound("not found")

        val savedState = SavedStateHandle(mapOf("postId" to postId))
        val vm = ForumDetailViewModel(getForumPostDetail, validateForumSafeLink, savedState)
        advanceUntilIdle()

        val state = vm.state.value
        assertFalse(state.loading)
        assertEquals(ForumDetailError.NOT_FOUND, state.error)
        assertNull(state.post)
    }

    @Test
    fun `offline with cached post keeps post and sets isOffline`() = runTest(dispatcher) {
        val savedState = SavedStateHandle(mapOf("postId" to postId))
        val vm = ForumDetailViewModel(getForumPostDetail, validateForumSafeLink, savedState)
        advanceUntilIdle()

        coEvery { getForumPostDetail(postId) } throws ForumException.Offline("offline")

        vm.refresh()
        advanceUntilIdle()

        val state = vm.state.value
        assertTrue(state.isOffline)
        assertEquals(sampleDetail, state.post)
    }

    @Test
    fun `valid external link triggers confirmation dialog with target host`() = runTest(dispatcher) {
        val savedState = SavedStateHandle(mapOf("postId" to postId))
        val vm = ForumDetailViewModel(getForumPostDetail, validateForumSafeLink, savedState)
        advanceUntilIdle()

        vm.onExternalLinkClick("https://autoesporte.globo.com/artigo")

        val state = vm.state.value
        assertEquals("https://autoesporte.globo.com/artigo", state.linkConfirmationTarget)
        assertEquals("autoesporte.globo.com", state.linkConfirmationHost)
        assertFalse(state.linkError)
    }

    @Test
    fun `invalid external link sets linkError`() = runTest(dispatcher) {
        coEvery { validateForumSafeLink("http://insecure.test") } returns false

        val savedState = SavedStateHandle(mapOf("postId" to postId))
        val vm = ForumDetailViewModel(getForumPostDetail, validateForumSafeLink, savedState)
        advanceUntilIdle()

        vm.onExternalLinkClick("http://insecure.test")

        val state = vm.state.value
        assertTrue(state.linkError)
        assertNull(state.linkConfirmationTarget)
    }

    @Test
    fun `dismissLinkDialog clears confirmation state`() = runTest(dispatcher) {
        val savedState = SavedStateHandle(mapOf("postId" to postId))
        val vm = ForumDetailViewModel(getForumPostDetail, validateForumSafeLink, savedState)
        advanceUntilIdle()

        vm.onExternalLinkClick("https://autoesporte.globo.com/artigo")
        assertTrue(vm.state.value.linkConfirmationTarget != null)

        vm.dismissLinkDialog()
        assertNull(vm.state.value.linkConfirmationTarget)
        assertNull(vm.state.value.linkConfirmationHost)
        assertFalse(vm.state.value.linkError)
    }

    @Test
    fun `refresh reloads post detail`() = runTest(dispatcher) {
        val savedState = SavedStateHandle(mapOf("postId" to postId))
        val vm = ForumDetailViewModel(getForumPostDetail, validateForumSafeLink, savedState)
        advanceUntilIdle()

        val updatedDetail = sampleDetail.copy(title = "Updated Title")
        coEvery { getForumPostDetail(postId) } returns updatedDetail

        vm.refresh()
        advanceUntilIdle()

        val state = vm.state.value
        assertFalse(state.refreshing)
        assertEquals("Updated Title", state.post?.title)
    }
}
