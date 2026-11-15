package com.anpfuel.app.ui.forum

import androidx.lifecycle.SavedStateHandle
import com.anpfuel.app.ui.safety.FakeSafetyGateway
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.usecase.forum.GetForumPostDetailUseCase
import com.anpfuel.application.usecase.forum.GetForumRepliesUseCase
import com.anpfuel.application.usecase.forum.GetOwnForumCommentIdsUseCase
import com.anpfuel.application.usecase.forum.GetOwnForumReactionsUseCase
import com.anpfuel.application.usecase.forum.GetOwnerTopicUseCase
import com.anpfuel.application.usecase.forum.GetRankedForumCommentsUseCase
import com.anpfuel.application.usecase.forum.ReportForumTargetUseCase
import com.anpfuel.application.usecase.safety.BlockCommunityUserUseCase
import com.anpfuel.application.usecase.safety.CommunityBlockState
import com.anpfuel.application.usecase.safety.FilterHiddenContentUseCase
import com.anpfuel.application.usecase.safety.ReportCommunityUserUseCase
import com.anpfuel.domain.forum.ForumCategory
import com.anpfuel.domain.forum.ForumComment
import com.anpfuel.domain.forum.ForumCommentsPage
import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.forum.ForumLiveEvent
import com.anpfuel.domain.forum.ForumOwnReactions
import com.anpfuel.domain.forum.ForumPostDetail
import com.anpfuel.domain.forum.ForumPostKind
import com.anpfuel.domain.forum.ForumRepliesPage
import com.anpfuel.domain.forum.ForumTopicReceipt
import com.anpfuel.domain.portable.PortableAuth
import com.anpfuel.domain.safety.CommunitySafetyException
import com.anpfuel.domain.safety.SafetyContentKind
import com.anpfuel.domain.safety.SafetyContentRef
import com.anpfuel.domain.safety.UserReportReason
import io.mockk.coEvery
import io.mockk.coVerify
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

/** BUC-SAFE-01..03 and B-BR-SAFE-06 on the opened conversation. */
@OptIn(ExperimentalCoroutinesApi::class)
class ForumDetailSafetyViewModelTest {

    private val dispatcher = StandardTestDispatcher()
    private val getForumPostDetail = mockk<GetForumPostDetailUseCase>()
    private val getRankedComments = mockk<GetRankedForumCommentsUseCase>()
    private val getForumReplies = mockk<GetForumRepliesUseCase>()
    private val getOwnerTopic = mockk<GetOwnerTopicUseCase>()
    private val getOwnForumCommentIds = mockk<GetOwnForumCommentIdsUseCase>()
    private val getOwnForumReactions = mockk<GetOwnForumReactionsUseCase>()
    private val reportForumTarget = mockk<ReportForumTargetUseCase>(relaxed = true)

    private val postId = "11111111-1111-4111-8111-111111111111"
    private val troll = "troll"
    private val friend = "friend"
    private val topic = ForumPostDetail(
        id = postId,
        kind = ForumPostKind.TOPIC,
        rawKind = "TOPIC",
        category = ForumCategory.FUELS,
        rawCategory = "FUELS",
        title = "Etanol ou gasolina?",
        excerpt = "Dúvida",
        bodyText = "Qual compensa?",
        publicAlias = "Autor",
        firstPublishedAt = "2026-11-15T10:00:00Z",
        updatedAt = "2026-11-15T10:00:00Z",
    )

    private fun id(n: Int) = "00000000-0000-4000-8000-%012d".format(n)

    private fun comment(n: Int, parentId: String? = null, topReply: ForumComment? = null) = ForumComment(
        id = id(n),
        postId = postId,
        parentId = parentId,
        sequence = n.toLong(),
        bodyText = "comment $n",
        createdAt = "2026-11-15T10:0$n:00Z",
        updatedAt = "2026-11-15T10:0$n:00Z",
        replyCount = if (topReply != null) 1 else 0,
        topReply = topReply,
    )

    private val trollComment = comment(2)
    private val friendPreviewByTroll = comment(4, parentId = id(3))
    private val friendComment = comment(3, topReply = friendPreviewByTroll)
    private val laterTrollComment = comment(5)

    private val session = PortableAuth.Session(
        familyId = "fam-1",
        accountId = "viewer",
        accessToken = "tok",
        refreshToken = "ref",
        accessExpiresAt = Long.MAX_VALUE,
        absoluteExpiresAt = Long.MAX_VALUE,
    )
    private var current: PortableAuth.Session? = session
    private val sessions = object : AuthSessionStore {
        override fun save(session: PortableAuth.Session) = Unit
        override fun load(): PortableAuth.Session? = current
        override fun clear() = Unit
    }

    private val gateway = FakeSafetyGateway().apply {
        authors[postId] = friend
        authors[trollComment.id] = troll
        authors[friendComment.id] = friend
        authors[friendPreviewByTroll.id] = troll
        authors[laterTrollComment.id] = troll
    }
    private val blockState = CommunityBlockState(gateway) { current?.accountId }

    private fun viewModel(withSafety: Boolean = true) = ForumDetailViewModel(
        getForumPostDetail = getForumPostDetail,
        validateForumSafeLink = mockk(relaxed = true),
        getForumReplies = getForumReplies,
        submitForumComment = mockk(relaxed = true),
        editForumComment = mockk(relaxed = true),
        removeForumComment = mockk(relaxed = true),
        reportForumTarget = reportForumTarget,
        getOwnerTopic = getOwnerTopic,
        getOwnForumCommentIds = getOwnForumCommentIds,
        getRankedComments = getRankedComments,
        setForumReaction = mockk(relaxed = true),
        getOwnForumReactions = getOwnForumReactions,
        observeConversation = mockk(relaxed = true),
        sessionStore = sessions,
        savedStateHandle = SavedStateHandle(mapOf("postId" to postId)),
        filterHiddenContent = if (withSafety) FilterHiddenContentUseCase(gateway, blockState) else null,
        reportCommunityUser = if (withSafety) ReportCommunityUserUseCase(gateway) else null,
        blockCommunityUser = if (withSafety) BlockCommunityUserUseCase(gateway, blockState) else null,
    )

    @BeforeEach
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        coEvery { getForumPostDetail(postId) } returns topic
        coEvery { getRankedComments(postId, any(), any(), any()) } returns
            ForumCommentsPage(items = listOf(trollComment, friendComment), nextCursor = null)
        coEvery { getForumReplies(friendComment.id) } returns
            ForumRepliesPage(items = listOf(friendPreviewByTroll), nextCursor = null)
        coEvery { getOwnerTopic(any()) } throws ForumException.NotFound("forum.not-found")
        coEvery { getOwnForumCommentIds(any()) } returns emptySet()
        coEvery { getOwnForumReactions(any()) } returns ForumOwnReactions()
    }

    @AfterEach
    fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test
    fun `a topic the viewer did not write offers report and block`() = runTest(dispatcher) {
        val vm = viewModel()
        advanceUntilIdle()
        assertTrue(vm.state.value.showsTopicSafetyActions)
    }

    @Test
    fun `the topic owner sees no report or block action`() = runTest(dispatcher) {
        coEvery { getOwnerTopic(postId) } returns mockk<ForumTopicReceipt>()
        val vm = viewModel()
        advanceUntilIdle()
        assertTrue(vm.state.value.isTopicOwner)
        assertFalse(vm.state.value.showsTopicSafetyActions)
    }

    @Test
    fun `editorial posts are not user content and offer no topic actions`() = runTest(dispatcher) {
        coEvery { getForumPostDetail(postId) } returns topic.copy(kind = ForumPostKind.EDITORIAL, rawKind = "EDITORIAL")
        val vm = viewModel()
        advanceUntilIdle()
        assertFalse(vm.state.value.showsTopicSafetyActions)
        assertTrue(gateway.hiddenQuestions.flatten().none { it.kind == SafetyContentKind.FORUM_POST })
    }

    @Test
    fun `reporting the topic itself uses the forum report flow`() = runTest(dispatcher) {
        val vm = viewModel()
        advanceUntilIdle()
        vm.openReport("FORUM_POST", postId)
        assertEquals("FORUM_POST" to postId, vm.state.value.reportingTarget)
    }

    @Test
    fun `visitors are sent to sign in and no dialog opens`() = runTest(dispatcher) {
        current = null
        val vm = viewModel()
        advanceUntilIdle()
        vm.openReportUser(SafetyContentRef(SafetyContentKind.FORUM_COMMENT, trollComment.id))
        assertTrue(vm.state.value.requireAuthPrompt)
        assertNull(vm.state.value.userReportTarget)
        vm.dismissAuthPrompt()
        vm.openBlockUser(SafetyContentRef(SafetyContentKind.FORUM_POST, postId))
        assertTrue(vm.state.value.requireAuthPrompt)
        assertNull(vm.state.value.blockTarget)
        assertEquals(0, gateway.listCalls, "visitors never ask about blocks")
    }

    @Test
    fun `reporting a user shows the received notice`() = runTest(dispatcher) {
        val vm = viewModel()
        advanceUntilIdle()
        val target = SafetyContentRef(SafetyContentKind.FORUM_COMMENT, trollComment.id)
        vm.openReportUser(target)
        assertEquals(target, vm.state.value.userReportTarget)
        vm.submitReportUser(UserReportReason.HARASSMENT, "  ofensas  ")
        advanceUntilIdle()
        val state = vm.state.value
        assertNull(state.userReportTarget)
        assertFalse(state.isSubmittingUserReport)
        assertFalse(state.userReportReceipt!!.replayed)
        assertEquals(Triple(target, UserReportReason.HARASSMENT, "ofensas"), gateway.reports.single())
    }

    @Test
    fun `reporting the same user again shows the replayed receipt`() = runTest(dispatcher) {
        gateway.reportReplayed = true
        val vm = viewModel()
        advanceUntilIdle()
        vm.openReportUser(SafetyContentRef(SafetyContentKind.FORUM_POST, postId))
        vm.submitReportUser(UserReportReason.SPAM, null)
        advanceUntilIdle()
        assertTrue(vm.state.value.userReportReceipt!!.replayed)
    }

    @Test
    fun `a rate limited report keeps the retry delay`() = runTest(dispatcher) {
        gateway.reportFailure = CommunitySafetyException.RateLimited(90)
        val vm = viewModel()
        advanceUntilIdle()
        vm.openReportUser(SafetyContentRef(SafetyContentKind.FORUM_COMMENT, trollComment.id))
        vm.submitReportUser(UserReportReason.SPAM, null)
        advanceUntilIdle()
        assertEquals(ForumActionError.RateLimited(90), vm.state.value.actionError)
        assertNull(vm.state.value.userReportReceipt)
    }

    @Test
    fun `reporting oneself shows the friendly self target message`() = runTest(dispatcher) {
        gateway.reportFailure = CommunitySafetyException.SelfTarget()
        val vm = viewModel()
        advanceUntilIdle()
        vm.openReportUser(SafetyContentRef(SafetyContentKind.FORUM_COMMENT, friendComment.id))
        vm.submitReportUser(UserReportReason.OTHER, null)
        advanceUntilIdle()
        assertEquals(ForumActionError.SelfTarget, vm.state.value.actionError)
        assertNull(vm.state.value.userReportTarget)
    }

    @Test
    fun `an expired session during a report goes to sign in`() = runTest(dispatcher) {
        gateway.reportFailure = CommunitySafetyException.AuthRequired()
        val vm = viewModel()
        advanceUntilIdle()
        vm.openReportUser(SafetyContentRef(SafetyContentKind.FORUM_COMMENT, trollComment.id))
        vm.submitReportUser(UserReportReason.OTHER, null)
        advanceUntilIdle()
        assertTrue(vm.state.value.requireAuthPrompt)
        assertNull(vm.state.value.actionError)
    }

    @Test
    fun `blocking an author removes all their content at once`() = runTest(dispatcher) {
        val vm = viewModel()
        advanceUntilIdle()
        vm.toggleReplies(friendComment.id)
        advanceUntilIdle()
        assertEquals(listOf(friendPreviewByTroll), vm.state.value.repliesMap[friendComment.id])

        vm.openBlockUser(SafetyContentRef(SafetyContentKind.FORUM_COMMENT, trollComment.id))
        vm.confirmBlockUser()
        advanceUntilIdle()

        val state = vm.state.value
        assertTrue(state.blockedNotice)
        assertNull(state.blockTarget)
        assertEquals(listOf(friendComment.id), state.comments.map { it.id })
        assertNull(state.comments.single().topReply, "the best answer preview by the blocked author is gone")
        assertEquals(emptyList<ForumComment>(), state.repliesMap[friendComment.id])
        assertFalse(state.topicHidden)
    }

    @Test
    fun `content by blocked authors stays hidden after refresh and live updates`() = runTest(dispatcher) {
        gateway.blockedAuthors += troll
        val vm = viewModel()
        advanceUntilIdle()
        assertEquals(listOf(friendComment.id), vm.state.value.comments.map { it.id })
        assertNull(vm.state.value.comments.single().topReply)

        vm.refresh()
        advanceUntilIdle()
        assertEquals(listOf(friendComment.id), vm.state.value.comments.map { it.id })

        vm.onLiveEvent(ForumLiveEvent.CommentPublished(laterTrollComment))
        advanceUntilIdle()
        assertTrue(vm.state.value.pendingLiveComments.isEmpty())
    }

    @Test
    fun `a topic by a blocked author shows the neutral notice and no conversation`() = runTest(dispatcher) {
        gateway.authors[postId] = troll
        gateway.blockedAuthors += troll
        val vm = viewModel()
        advanceUntilIdle()
        val state = vm.state.value
        assertTrue(state.topicHidden)
        assertFalse(state.showsTopicSafetyActions)
        assertTrue(state.comments.isEmpty())
        coVerify(exactly = 0) { getRankedComments(any(), any(), any(), any()) }
    }

    @Test
    fun `blocking the topic author from the top bar hides the topic`() = runTest(dispatcher) {
        gateway.authors[postId] = troll
        val vm = viewModel()
        advanceUntilIdle()
        vm.openBlockUser(SafetyContentRef(SafetyContentKind.FORUM_POST, postId))
        vm.confirmBlockUser()
        advanceUntilIdle()
        assertTrue(vm.state.value.topicHidden)
        assertTrue(vm.state.value.blockedNotice)
    }

    @Test
    fun `the block limit is explained and nothing is hidden`() = runTest(dispatcher) {
        gateway.blockFailure = CommunitySafetyException.BlockLimit()
        val vm = viewModel()
        advanceUntilIdle()
        vm.openBlockUser(SafetyContentRef(SafetyContentKind.FORUM_COMMENT, trollComment.id))
        vm.confirmBlockUser()
        advanceUntilIdle()
        assertEquals(ForumActionError.BlockLimit, vm.state.value.actionError)
        assertEquals(2, vm.state.value.comments.size)
        assertFalse(vm.state.value.blockedNotice)
    }

    @Test
    fun `blocking oneself is refused with the self target message`() = runTest(dispatcher) {
        gateway.blockFailure = CommunitySafetyException.SelfTarget()
        val vm = viewModel()
        advanceUntilIdle()
        vm.openBlockUser(SafetyContentRef(SafetyContentKind.FORUM_COMMENT, friendComment.id))
        vm.confirmBlockUser()
        advanceUntilIdle()
        assertEquals(ForumActionError.SelfTarget, vm.state.value.actionError)
        assertEquals(2, vm.state.value.comments.size)
    }

    @Test
    fun `filtering fails open when the hidden question fails`() = runTest(dispatcher) {
        gateway.blockedAuthors += troll
        gateway.hiddenFailure = CommunitySafetyException.Unavailable()
        val vm = viewModel()
        advanceUntilIdle()
        assertEquals(2, vm.state.value.comments.size)
        assertNull(vm.state.value.error)
    }

    @Test
    fun `an account without blocks never asks the hidden question`() = runTest(dispatcher) {
        val vm = viewModel()
        advanceUntilIdle()
        vm.refresh()
        advanceUntilIdle()
        assertEquals(1, gateway.listCalls)
        assertTrue(gateway.hiddenQuestions.isEmpty())
        assertEquals(2, vm.state.value.comments.size)
    }
}
