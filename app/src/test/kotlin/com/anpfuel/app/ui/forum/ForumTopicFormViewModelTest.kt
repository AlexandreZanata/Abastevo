package com.anpfuel.app.ui.forum

import androidx.lifecycle.SavedStateHandle
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.usecase.forum.EditForumTopicUseCase
import com.anpfuel.application.usecase.forum.GetOwnerTopicUseCase
import com.anpfuel.application.usecase.forum.RemoveForumTopicUseCase
import com.anpfuel.application.usecase.forum.SubmitForumTopicUseCase
import com.anpfuel.domain.forum.ForumCategory
import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.forum.ForumRegion
import com.anpfuel.domain.forum.ForumTopicReceipt
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
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class ForumTopicFormViewModelTest {

    private val dispatcher = StandardTestDispatcher()
    private val submitForumTopic = mockk<SubmitForumTopicUseCase>()
    private val editForumTopic = mockk<EditForumTopicUseCase>()
    private val removeForumTopic = mockk<RemoveForumTopicUseCase>()
    private val getOwnerTopic = mockk<GetOwnerTopicUseCase>()
    private val sessionStore = mockk<AuthSessionStore>()

    private val sampleSession = PortableAuth.Session(
        familyId = "fam-1",
        accountId = "user-123",
        accessToken = "tok-access",
        refreshToken = "tok-refresh",
        accessExpiresAt = Long.MAX_VALUE,
        absoluteExpiresAt = Long.MAX_VALUE,
    )

    private val sampleTopicId = "11111111-1111-1111-1111-111111111111"

    private val sampleReceipt = ForumTopicReceipt(
        id = sampleTopicId,
        category = ForumCategory.FUELS,
        rawCategory = "FUELS",
        title = "Preços na Rodovia",
        bodyText = "Relato sobre os preços cobrados nos postos da rodovia principal.",
        revision = 1L,
        publicationState = "PUBLISHED",
        visibility = "PUBLIC",
        region = ForumRegion(uf = "SP", municipalityIbge = 3550308),
        fuelProduct = "GASOLINE_REGULAR",
        stationId = null,
        publicAlias = "Motorista da Estrada",
        createdAt = "2026-11-06T10:00:00Z",
        updatedAt = "2026-11-06T10:00:00Z",
        publishedAt = "2026-11-06T10:00:00Z",
        replayed = false,
    )

    @BeforeEach
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        every { sessionStore.load() } returns sampleSession
    }

    @AfterEach
    fun tearDown() {
        Dispatchers.resetMain()
    }

    private fun createViewModel(
        topicId: String? = null,
        store: AuthSessionStore? = sessionStore,
    ): ForumTopicFormViewModel {
        val savedStateHandle = if (topicId != null) {
            SavedStateHandle(mapOf("topicId" to topicId))
        } else {
            SavedStateHandle()
        }
        return ForumTopicFormViewModel(
            submitForumTopic = submitForumTopic,
            editForumTopic = editForumTopic,
            removeForumTopic = removeForumTopic,
            getOwnerTopic = getOwnerTopic,
            sessionStore = store,
            savedStateHandle = savedStateHandle,
        )
    }

    @Test
    fun `initial state create mode - empty fields, not dirty, submit validates empty title`() = runTest {
        val vm = createViewModel()

        assertFalse(vm.state.value.isEditMode)
        assertNull(vm.state.value.topicId)
        assertFalse(vm.state.value.isDirty)
        assertTrue(vm.state.value.isAuthenticated)
        assertEquals("", vm.state.value.title)
        assertEquals("", vm.state.value.bodyText)
        assertNull(vm.state.value.selectedCategory)

        vm.submit()
        advanceUntilIdle()

        assertEquals(ForumTopicValidationError.TITLE_EMPTY, vm.state.value.validationError)
        assertFalse(vm.state.value.isSubmitting)
    }

    @Test
    fun `title validation bounds - too long`() = runTest {
        val vm = createViewModel()
        vm.updateTitle("A".repeat(121))
        vm.updateBodyText("Corpo válido do tópico com detalhes suficientes.")
        vm.updateCategory(ForumCategory.FUELS)

        vm.submit()
        advanceUntilIdle()

        assertEquals(ForumTopicValidationError.TITLE_TOO_LONG, vm.state.value.validationError)
    }

    @Test
    fun `body validation bounds - empty and too long`() = runTest {
        val vm = createViewModel()
        vm.updateTitle("Título Válido")
        vm.updateCategory(ForumCategory.FUELS)

        vm.submit()
        advanceUntilIdle()
        assertEquals(ForumTopicValidationError.BODY_EMPTY, vm.state.value.validationError)

        vm.updateBodyText("B".repeat(4001))
        vm.submit()
        advanceUntilIdle()
        assertEquals(ForumTopicValidationError.BODY_TOO_LONG, vm.state.value.validationError)
    }

    @Test
    fun `category validation - required`() = runTest {
        val vm = createViewModel()
        vm.updateTitle("Título Válido")
        vm.updateBodyText("Corpo válido do tópico.")

        vm.submit()
        advanceUntilIdle()

        assertEquals(ForumTopicValidationError.CATEGORY_REQUIRED, vm.state.value.validationError)
    }

    @Test
    fun `unauthenticated submission prompts auth while preserving in-memory draft`() = runTest {
        val emptySessionStore = mockk<AuthSessionStore>()
        every { emptySessionStore.load() } returns null

        val vm = createViewModel(store = emptySessionStore)
        assertFalse(vm.state.value.isAuthenticated)

        vm.updateTitle("Título do Rascunho")
        vm.updateBodyText("Conteúdo importante digitado pelo usuário anônimo.")
        vm.updateCategory(ForumCategory.MARKET)
        assertTrue(vm.state.value.isDirty)

        vm.submit()
        advanceUntilIdle()

        assertTrue(vm.state.value.requireAuthPrompt)
        assertFalse(vm.state.value.isSubmitting)
        // Draft stays intact in memory
        assertEquals("Título do Rascunho", vm.state.value.title)
        assertEquals("Conteúdo importante digitado pelo usuário anônimo.", vm.state.value.bodyText)
        assertEquals(ForumCategory.MARKET, vm.state.value.selectedCategory)

        vm.dismissAuthPrompt()
        assertFalse(vm.state.value.requireAuthPrompt)
        assertEquals("Título do Rascunho", vm.state.value.title)
    }

    @Test
    fun `account change intent safety resets draft`() = runTest {
        val dynamicSessionStore = mockk<AuthSessionStore>()
        var currentSession: PortableAuth.Session? = sampleSession
        every { dynamicSessionStore.load() } answers { currentSession }

        val vm = createViewModel(store = dynamicSessionStore)
        vm.updateTitle("Privado do Usuário A")
        vm.updateBodyText("Texto particular.")
        vm.updateCategory(ForumCategory.FUELS)
        assertTrue(vm.state.value.isDirty)

        // Switch to User B
        currentSession = sampleSession.copy(accountId = "user-456")
        vm.checkSessionConsistency()

        // Draft is wiped for intent safety
        assertEquals("", vm.state.value.title)
        assertEquals("", vm.state.value.bodyText)
        assertNull(vm.state.value.selectedCategory)
        assertFalse(vm.state.value.isDirty)
    }

    @Test
    fun `back navigation discard dialog handling`() = runTest {
        val vm = createViewModel()
        var popped = false

        // When clean: pops immediately
        vm.onBackPressed { popped = true }
        assertTrue(popped)
        assertFalse(vm.state.value.showDiscardDialog)

        // When dirty: shows discard dialog
        popped = false
        vm.updateTitle("Modificado")
        assertTrue(vm.state.value.isDirty)

        vm.onBackPressed { popped = true }
        assertFalse(popped)
        assertTrue(vm.state.value.showDiscardDialog)

        // Dismiss keeps draft
        vm.dismissDiscardDialog()
        assertFalse(vm.state.value.showDiscardDialog)
        assertEquals("Modificado", vm.state.value.title)

        // Confirm discard resets and executes pop
        vm.confirmDiscard { popped = true }
        assertTrue(popped)
        assertFalse(vm.state.value.showDiscardDialog)
        assertFalse(vm.state.value.isDirty)
        assertEquals("", vm.state.value.title)
    }

    @Test
    fun `valid create topic submission receives receipt and resets dirty`() = runTest {
        coEvery {
            submitForumTopic(
                title = "Novo Tópico",
                bodyText = "Discussão importante sobre preços.",
                category = ForumCategory.FUELS,
                operationId = any(),
                region = ForumRegion(uf = "SP", municipalityIbge = 3550308),
                fuelProduct = "GASOLINE_REGULAR",
                stationId = null,
                publicAlias = "Alias Teste",
            )
        } returns sampleReceipt

        val vm = createViewModel()
        vm.updateTitle("Novo Tópico")
        vm.updateBodyText("Discussão importante sobre preços.")
        vm.updateCategory(ForumCategory.FUELS)
        vm.updateUf("SP")
        vm.updateMunicipalityIbge("3550308")
        vm.updateFuelProduct("GASOLINE_REGULAR")
        vm.updatePublicAlias("Alias Teste")

        vm.submit()
        advanceUntilIdle()

        assertFalse(vm.state.value.isSubmitting)
        assertFalse(vm.state.value.isDirty)
        assertNotNull(vm.state.value.submissionReceipt)
        assertEquals(sampleReceipt, vm.state.value.submissionReceipt)
    }

    @Test
    fun `stale conflict 409 maps to ForumActionError Conflict`() = runTest {
        coEvery {
            submitForumTopic(
                title = any(),
                bodyText = any(),
                category = any(),
                operationId = any(),
                region = any(),
                fuelProduct = any(),
                stationId = any(),
                publicAlias = any(),
            )
        } throws ForumException.Conflict("conflict")

        val vm = createViewModel()
        vm.updateTitle("Tópico com Conflito")
        vm.updateBodyText("Texto do tópico")
        vm.updateCategory(ForumCategory.MARKET)

        vm.submit()
        advanceUntilIdle()

        assertFalse(vm.state.value.isSubmitting)
        val error = vm.state.value.actionError
        assertTrue(error is ForumActionError.Conflict)
    }

    @Test
    fun `rate limited 429 maps to ForumActionError RateLimited`() = runTest {
        coEvery {
            submitForumTopic(
                title = any(),
                bodyText = any(),
                category = any(),
                operationId = any(),
                region = any(),
                fuelProduct = any(),
                stationId = any(),
                publicAlias = any(),
            )
        } throws ForumException.RateLimited(code = "rate_limited", retryAfterSeconds = 120L)

        val vm = createViewModel()
        vm.updateTitle("Tópico Rápido Demais")
        vm.updateBodyText("Texto do tópico")
        vm.updateCategory(ForumCategory.REGULATION)

        vm.submit()
        advanceUntilIdle()

        assertFalse(vm.state.value.isSubmitting)
        val error = vm.state.value.actionError
        assertTrue(error is ForumActionError.RateLimited)
        assertEquals(120L, (error as ForumActionError.RateLimited).retryAfterSeconds)
    }

    @Test
    fun `edit mode initialization loads owner topic and populates state`() = runTest {
        coEvery { getOwnerTopic(sampleTopicId) } returns sampleReceipt

        val vm = createViewModel(topicId = sampleTopicId)
        advanceUntilIdle()

        assertTrue(vm.state.value.isEditMode)
        assertEquals(sampleTopicId, vm.state.value.topicId)
        assertFalse(vm.state.value.isLoading)
        assertEquals("Preços na Rodovia", vm.state.value.title)
        assertEquals("Relato sobre os preços cobrados nos postos da rodovia principal.", vm.state.value.bodyText)
        assertEquals(ForumCategory.FUELS, vm.state.value.selectedCategory)
        assertEquals("SP", vm.state.value.selectedUf)
        assertEquals("3550308", vm.state.value.municipalityIbge)
        assertEquals("GASOLINE_REGULAR", vm.state.value.selectedFuelProduct)
        assertEquals(1L, vm.state.value.expectedRevision)
        assertFalse(vm.state.value.isDirty)
    }

    @Test
    fun `edit mode submit calls EditForumTopicUseCase with expectedRevision`() = runTest {
        coEvery { getOwnerTopic(sampleTopicId) } returns sampleReceipt
        val updatedReceipt = sampleReceipt.copy(
            bodyText = "Texto revisado pelo autor com mais dados.",
            revision = 2L,
        )
        coEvery {
            editForumTopic(
                topicId = sampleTopicId,
                expectedRevision = 1L,
                title = "Preços na Rodovia",
                bodyText = "Texto revisado pelo autor com mais dados.",
                category = ForumCategory.FUELS,
                operationId = any(),
                region = ForumRegion(uf = "SP", municipalityIbge = 3550308),
                fuelProduct = "GASOLINE_REGULAR",
                stationId = null,
                publicAlias = "Motorista da Estrada",
            )
        } returns updatedReceipt

        val vm = createViewModel(topicId = sampleTopicId)
        advanceUntilIdle()

        vm.updateBodyText("Texto revisado pelo autor com mais dados.")
        assertTrue(vm.state.value.isDirty)

        vm.submit()
        advanceUntilIdle()

        assertFalse(vm.state.value.isSubmitting)
        assertFalse(vm.state.value.isDirty)
        assertEquals(updatedReceipt, vm.state.value.submissionReceipt)
        assertEquals(2L, vm.state.value.expectedRevision)
    }

    @Test
    fun `edit mode remove topic prompts confirmation and executes deletion`() = runTest {
        coEvery { getOwnerTopic(sampleTopicId) } returns sampleReceipt
        coEvery { removeForumTopic(topicId = sampleTopicId, operationId = any()) } returns sampleReceipt

        val vm = createViewModel(topicId = sampleTopicId)
        advanceUntilIdle()

        assertFalse(vm.state.value.showDeleteDialog)
        vm.confirmRemove()
        assertTrue(vm.state.value.showDeleteDialog)

        vm.dismissDeleteDialog()
        assertFalse(vm.state.value.showDeleteDialog)

        vm.confirmRemove()
        vm.executeRemove()
        advanceUntilIdle()

        assertFalse(vm.state.value.isRemoving)
        assertFalse(vm.state.value.showDeleteDialog)
        assertTrue(vm.state.value.isRemoved)

        coVerify { removeForumTopic(topicId = sampleTopicId, operationId = any()) }
    }
}
