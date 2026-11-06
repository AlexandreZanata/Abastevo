package com.anpfuel.app.ui.forum

import androidx.lifecycle.SavedStateHandle
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.usecase.forum.EditForumCommentUseCase
import com.anpfuel.application.usecase.forum.GetForumCommentsUseCase
import com.anpfuel.application.usecase.forum.GetForumPostDetailUseCase
import com.anpfuel.application.usecase.forum.GetForumRepliesUseCase
import com.anpfuel.application.usecase.forum.RemoveForumCommentUseCase
import com.anpfuel.application.usecase.forum.ReportForumTargetUseCase
import com.anpfuel.application.usecase.forum.SubmitForumCommentUseCase
import com.anpfuel.application.usecase.forum.ValidateForumSafeLinkUseCase
import com.anpfuel.domain.forum.ForumCategory
import com.anpfuel.domain.forum.ForumComment
import com.anpfuel.domain.forum.ForumCommentReceipt
import com.anpfuel.domain.forum.ForumCommentsPage
import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.forum.ForumPostDetail
import com.anpfuel.domain.forum.ForumPostKind
import com.anpfuel.domain.forum.ForumRepliesPage
import com.anpfuel.domain.forum.ForumReportReason
import com.anpfuel.domain.forum.ForumReportReceipt
import com.anpfuel.domain.forum.ForumSourceAttribution
import com.anpfuel.domain.portable.PortableAuth
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
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
    private val getForumComments = mockk<GetForumCommentsUseCase>()
    private val getForumReplies = mockk<GetForumRepliesUseCase>()
    private val submitForumComment = mockk<SubmitForumCommentUseCase>()
    private val editForumComment = mockk<EditForumCommentUseCase>()
    private val removeForumComment = mockk<RemoveForumCommentUseCase>()
    private val reportForumTarget = mockk<ReportForumTargetUseCase>()
    private val sessionStore = mockk<AuthSessionStore>()

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

    private val sampleComment1 = ForumComment(
        id = "22222222-2222-2222-2222-222222222222",
        postId = postId,
        parentId = null,
        publicAlias = "Motorista #1",
        bodyText = "Comentário de teste 1",
        sequence = 1L,
        replyCount = 2,
        createdAt = "2026-11-01T10:05:00Z",
        updatedAt = "2026-11-01T10:05:00Z",
    )

    private val sampleReply1 = ForumComment(
        id = "33333333-3333-3333-3333-333333333333",
        postId = postId,
        parentId = sampleComment1.id,
        publicAlias = "Motorista #2",
        bodyText = "Resposta ao comentário 1",
        sequence = 1L,
        replyCount = 0,
        createdAt = "2026-11-01T10:10:00Z",
        updatedAt = "2026-11-01T10:10:00Z",
    )

    private val sampleSession = PortableAuth.Session(
        familyId = "fam-1",
        accountId = "user-123",
        accessToken = "tok-access",
        refreshToken = "tok-refresh",
        accessExpiresAt = Long.MAX_VALUE,
        absoluteExpiresAt = Long.MAX_VALUE,
    )

    private fun createViewModel(
        postIdArg: String = postId,
        session: PortableAuth.Session? = null,
    ): ForumDetailViewModel {
        val savedState = SavedStateHandle(mapOf("postId" to postIdArg))
        every { sessionStore.load() } returns session
        return ForumDetailViewModel(
            getForumPostDetail = getForumPostDetail,
            validateForumSafeLink = validateForumSafeLink,
            getForumComments = getForumComments,
            getForumReplies = getForumReplies,
            submitForumComment = submitForumComment,
            editForumComment = editForumComment,
            removeForumComment = removeForumComment,
            reportForumTarget = reportForumTarget,
            sessionStore = sessionStore,
            savedStateHandle = savedState,
        )
    }

    @BeforeEach
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        coEvery { getForumPostDetail(postId) } returns sampleDetail
        coEvery { validateForumSafeLink(any()) } returns true
        coEvery { getForumComments(postId) } returns ForumCommentsPage(items = listOf(sampleComment1), nextCursor = null)
        coEvery { getForumReplies(sampleComment1.id) } returns ForumRepliesPage(items = listOf(sampleReply1), nextCursor = null)
        every { sessionStore.load() } returns null
    }

    @AfterEach
    fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test
    fun `initial load with postIdArg fetches post detail`() = runTest(dispatcher) {
        val vm = createViewModel()
        advanceUntilIdle()

        val state = vm.state.value
        assertFalse(state.loading)
        assertNull(state.error)
        assertEquals(sampleDetail, state.post)
    }

    @Test
    fun `not found error marks error as NOT_FOUND`() = runTest(dispatcher) {
        coEvery { getForumPostDetail(postId) } throws ForumException.NotFound("not found")

        val vm = createViewModel()
        advanceUntilIdle()

        val state = vm.state.value
        assertFalse(state.loading)
        assertEquals(ForumDetailError.NOT_FOUND, state.error)
        assertNull(state.post)
    }

    @Test
    fun `offline with cached post keeps post and sets isOffline`() = runTest(dispatcher) {
        val vm = createViewModel()
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
        val vm = createViewModel()
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

        val vm = createViewModel()
        advanceUntilIdle()

        vm.onExternalLinkClick("http://insecure.test")

        val state = vm.state.value
        assertTrue(state.linkError)
        assertNull(state.linkConfirmationTarget)
    }

    @Test
    fun `dismissLinkDialog clears confirmation state`() = runTest(dispatcher) {
        val vm = createViewModel()
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
        val vm = createViewModel()
        advanceUntilIdle()

        val updatedDetail = sampleDetail.copy(title = "Updated Title")
        coEvery { getForumPostDetail(postId) } returns updatedDetail

        vm.refresh()
        advanceUntilIdle()

        val state = vm.state.value
        assertFalse(state.refreshing)
        assertEquals("Updated Title", state.post?.title)
    }

    // --- FOR-19 Social Discussion Tests ---

    @Test
    fun `social toggle loads comments when enabled`() = runTest(dispatcher) {
        val vm = createViewModel()
        advanceUntilIdle()

        assertFalse(vm.state.value.isSocialEnabled)
        assertTrue(vm.state.value.comments.isEmpty())

        vm.setSocialEnabled(true)
        advanceUntilIdle()

        assertTrue(vm.state.value.isSocialEnabled)
        assertEquals(1, vm.state.value.comments.size)
        assertEquals(sampleComment1, vm.state.value.comments.first())
        coVerify(exactly = 1) { getForumComments(postId) }
    }

    @Test
    fun `toggleReplies expands and fetches replies on demand`() = runTest(dispatcher) {
        val vm = createViewModel()
        advanceUntilIdle()
        vm.setSocialEnabled(true)
        advanceUntilIdle()

        // First toggle: expand and load replies
        vm.toggleReplies(sampleComment1.id)
        advanceUntilIdle()

        assertTrue(vm.state.value.expandedReplies.contains(sampleComment1.id))
        assertEquals(listOf(sampleReply1), vm.state.value.repliesMap[sampleComment1.id])
        coVerify(exactly = 1) { getForumReplies(sampleComment1.id) }

        // Second toggle: collapses without reloading
        vm.toggleReplies(sampleComment1.id)
        advanceUntilIdle()

        assertFalse(vm.state.value.expandedReplies.contains(sampleComment1.id))
        coVerify(exactly = 1) { getForumReplies(sampleComment1.id) }
    }

    @Test
    fun `reply controls update replyingTo state`() = runTest(dispatcher) {
        val vm = createViewModel()
        advanceUntilIdle()

        vm.startReply(sampleComment1)
        assertEquals(sampleComment1, vm.state.value.replyingTo)

        vm.cancelReply()
        assertNull(vm.state.value.replyingTo)
    }

    @Test
    fun `updateComposerText clamps text at 280 characters`() = runTest(dispatcher) {
        val vm = createViewModel()
        advanceUntilIdle()

        val longText = "a".repeat(300)
        vm.updateComposerText(longText)

        assertEquals(280, vm.state.value.composerText.length)
    }

    @Test
    fun `submitComment without active session prompts for auth while preserving draft`() = runTest(dispatcher) {
        val vm = createViewModel(session = null)
        advanceUntilIdle()

        vm.updateComposerText("Meu comentário importante")
        vm.submitComment()
        advanceUntilIdle()

        assertTrue(vm.state.value.requireAuthPrompt)
        assertEquals("Meu comentário importante", vm.state.value.composerText)
        coVerify(exactly = 0) { submitForumComment(any(), any(), any(), any()) }

        vm.dismissAuthPrompt()
        assertFalse(vm.state.value.requireAuthPrompt)
    }

    @Test
    fun `submitComment with active session creates comment and clears draft`() = runTest(dispatcher) {
        val vm = createViewModel(session = sampleSession)
        advanceUntilIdle()

        val receipt = ForumCommentReceipt(
            id = "comment-new-1",
            postId = postId,
            parentId = null,
            depth = 0,
            sequence = 1L,
            revision = 1L,
            publicationState = "PUBLISHED",
            visibility = "VISIBLE",
            bodyText = "Comentário postado com sucesso",
            createdAt = "2026-11-01T11:00:00Z",
            updatedAt = "2026-11-01T11:00:00Z",
            replayed = false,
        )
        coEvery { submitForumComment(postId = postId, bodyText = "Comentário postado com sucesso", parentId = null, operationId = any()) } returns receipt

        vm.updateComposerText("Comentário postado com sucesso")
        vm.submitComment()
        advanceUntilIdle()

        assertFalse(vm.state.value.isSubmittingComment)
        assertEquals("", vm.state.value.composerText)
        assertEquals(receipt, vm.state.value.submissionReceipt)

        vm.dismissSubmissionReceipt()
        assertNull(vm.state.value.submissionReceipt)
    }

    @Test
    fun `submitComment handles 429 rate limit preserving draft`() = runTest(dispatcher) {
        val vm = createViewModel(session = sampleSession)
        advanceUntilIdle()

        coEvery { submitForumComment(any(), any(), any(), any()) } throws ForumException.RateLimited("rate limited", retryAfterSeconds = 45L)

        vm.updateComposerText("Comentário retido por limite")
        vm.submitComment()
        advanceUntilIdle()

        assertEquals("Comentário retido por limite", vm.state.value.composerText)
        val err = vm.state.value.actionError
        assertTrue(err is ForumActionError.RateLimited)
        assertEquals(45L, (err as ForumActionError.RateLimited).retryAfterSeconds)

        vm.dismissActionError()
        assertNull(vm.state.value.actionError)
    }

    @Test
    fun `submitComment handles 409 conflict preserving draft`() = runTest(dispatcher) {
        val vm = createViewModel(session = sampleSession)
        advanceUntilIdle()

        coEvery { submitForumComment(any(), any(), any(), any()) } throws ForumException.Conflict("conflict")

        vm.updateComposerText("Rascunho preservado após conflito")
        vm.submitComment()
        advanceUntilIdle()

        assertEquals("Rascunho preservado após conflito", vm.state.value.composerText)
        assertEquals(ForumActionError.Conflict, vm.state.value.actionError)
    }

    @Test
    fun `edit comment flow starts, updates text, cancels and saves`() = runTest(dispatcher) {
        val vm = createViewModel(session = sampleSession)
        advanceUntilIdle()

        vm.startEdit(sampleComment1)
        assertEquals(sampleComment1, vm.state.value.editingComment)
        assertEquals(sampleComment1.bodyText, vm.state.value.editDraftText)

        vm.updateEditDraftText("Texto editado atualizado")
        assertEquals("Texto editado atualizado", vm.state.value.editDraftText)

        vm.cancelEdit()
        assertNull(vm.state.value.editingComment)
        assertEquals("", vm.state.value.editDraftText)

        // Save edit success
        val receipt = ForumCommentReceipt(
            id = sampleComment1.id,
            postId = postId,
            parentId = null,
            depth = 0,
            sequence = 2L,
            revision = 2L,
            publicationState = "PUBLISHED",
            visibility = "VISIBLE",
            bodyText = "Texto editado salvo",
            createdAt = sampleComment1.createdAt,
            updatedAt = "2026-11-01T11:30:00Z",
            replayed = false,
        )
        coEvery { editForumComment(sampleComment1.id, sampleComment1.sequence, "Texto editado salvo", any()) } returns receipt

        vm.startEdit(sampleComment1)
        vm.updateEditDraftText("Texto editado salvo")
        vm.saveEdit()
        advanceUntilIdle()

        assertNull(vm.state.value.editingComment)
        assertEquals("", vm.state.value.editDraftText)
        assertEquals(receipt, vm.state.value.submissionReceipt)
    }

    @Test
    fun `saveEdit with conflict preserves draft text for recovery`() = runTest(dispatcher) {
        val vm = createViewModel(session = sampleSession)
        advanceUntilIdle()

        coEvery { editForumComment(any(), any(), any(), any()) } throws ForumException.Conflict("revision conflict")

        vm.startEdit(sampleComment1)
        vm.updateEditDraftText("Edição que deu conflito")
        vm.saveEdit()
        advanceUntilIdle()

        assertEquals("Edição que deu conflito", vm.state.value.editDraftText)
        assertEquals(sampleComment1, vm.state.value.editingComment)
        assertEquals(ForumActionError.Conflict, vm.state.value.actionError)
    }

    @Test
    fun `remove comment flow confirms, cancels and executes removal`() = runTest(dispatcher) {
        val vm = createViewModel(session = sampleSession)
        advanceUntilIdle()

        vm.confirmRemove(sampleComment1)
        assertEquals(sampleComment1, vm.state.value.commentPendingRemoval)

        vm.cancelRemove()
        assertNull(vm.state.value.commentPendingRemoval)

        val receipt = ForumCommentReceipt(
            id = sampleComment1.id,
            postId = postId,
            parentId = null,
            depth = 0,
            sequence = 2L,
            revision = 2L,
            publicationState = "SUPPRESSED",
            visibility = "HIDDEN",
            bodyText = "[Comentário removido pelo autor]",
            createdAt = sampleComment1.createdAt,
            updatedAt = "2026-11-01T12:00:00Z",
            replayed = false,
        )
        coEvery { removeForumComment(sampleComment1.id, sampleComment1.sequence, any()) } returns receipt

        vm.confirmRemove(sampleComment1)
        vm.executeRemove()
        advanceUntilIdle()

        assertNull(vm.state.value.commentPendingRemoval)
        assertFalse(vm.state.value.isRemovingComment)
        coVerify(exactly = 1) { removeForumComment(sampleComment1.id, sampleComment1.sequence, any()) }
    }

    @Test
    fun `report flow opens, cancels and submits with receipt`() = runTest(dispatcher) {
        val vm = createViewModel(session = sampleSession)
        advanceUntilIdle()

        vm.openReport("FORUM_COMMENT", sampleComment1.id)
        assertEquals(Pair("FORUM_COMMENT", sampleComment1.id), vm.state.value.reportingTarget)

        vm.cancelReport()
        assertNull(vm.state.value.reportingTarget)

        val receipt = ForumReportReceipt(
            reportId = "rep-1",
            caseId = "case-1",
            targetType = "FORUM_COMMENT",
            targetId = sampleComment1.id,
            reason = "SPAM",
            status = "PENDING_REVIEW",
            createdAt = "2026-11-01T12:00:00Z",
            replayed = false,
        )
        coEvery { reportForumTarget("FORUM_COMMENT", sampleComment1.id, ForumReportReason.SPAM, "Spam evidente") } returns receipt

        vm.openReport("FORUM_COMMENT", sampleComment1.id)
        vm.submitReport(ForumReportReason.SPAM, "Spam evidente")
        advanceUntilIdle()

        assertNull(vm.state.value.reportingTarget)
        assertFalse(vm.state.value.isSubmittingReport)
        assertEquals(receipt, vm.state.value.reportReceipt)

        vm.dismissReportReceipt()
        assertNull(vm.state.value.reportReceipt)
    }

    @Test
    fun `session consistency clears private in-flight drafts on account switch`() = runTest(dispatcher) {
        val vm = createViewModel(session = sampleSession)
        advanceUntilIdle()

        vm.startEdit(sampleComment1)
        vm.updateEditDraftText("Edição em andamento")
        vm.openReport("FORUM_COMMENT", sampleComment1.id)

        // Session switches to another user
        val otherSession = sampleSession.copy(accountId = "other-user-999")
        every { sessionStore.load() } returns otherSession

        vm.checkSessionConsistency()

        assertNull(vm.state.value.editingComment)
        assertEquals("", vm.state.value.editDraftText)
        assertNull(vm.state.value.reportingTarget)
    }
}
