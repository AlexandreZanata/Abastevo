package com.anpfuel.app.ui.suggest

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.usecase.intake.CancelOwnedSuggestionUseCase
import com.anpfuel.application.usecase.intake.GetOwnedSuggestionsUseCase
import com.anpfuel.application.usecase.intake.GetSuggestionStatusUseCase
import com.anpfuel.application.usecase.intake.OwnedSuggestionsOutcome
import com.anpfuel.application.usecase.intake.SubmitStationSuggestionUseCase
import com.anpfuel.application.usecase.intake.SubmitSuggestionOutcome
import com.anpfuel.application.usecase.intake.SuggestionStatusOutcome
import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.domain.discovery.ServerStationPage
import com.anpfuel.domain.discovery.StationLocationQuality
import com.anpfuel.domain.portable.PortableAuth
import com.anpfuel.domain.repository.ServerStationCache
import io.mockk.coEvery
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
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class SuggestStationViewModelTest {

    private val dispatcher = StandardTestDispatcher()

    private val submit = mockk<SubmitStationSuggestionUseCase>()
    private val status = mockk<GetSuggestionStatusUseCase>()
    private val cancel = mockk<CancelOwnedSuggestionUseCase>()
    private val mine = mockk<GetOwnedSuggestionsUseCase>()
    private val sessions = mockk<AuthSessionStore>()
    private val catalog = mockk<ServerStationCache>()

    private val session = PortableAuth.Session(
        familyId = "fam-1",
        accountId = "acc-1",
        accessToken = "tok-1",
        refreshToken = "ref-1",
        accessExpiresAt = 9_999_999_999L,
        absoluteExpiresAt = 9_999_999_999L,
    )

    private lateinit var viewModel: SuggestStationViewModel

    @BeforeEach
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        every { sessions.load() } returns session
        coEvery { catalog.loadPage() } returns null
        every { mine.invoke(any(), any()) } returns OwnedSuggestionsOutcome.Found(emptyList())
        viewModel = SuggestStationViewModel(submit, status, cancel, mine, sessions, catalog)
    }

    @AfterEach
    fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test
    fun `guest submit is gated without network`() = runTest(dispatcher) {
        every { sessions.load() } returns null
        viewModel.onField("Posto", "3550308", "SP", "")
        viewModel.submitProposal()
        advanceUntilIdle()

        assertTrue(viewModel.uiState.value.submit is SubmitSuggestionOutcome.SessionInvalid)
    }

    @Test
    fun `duplicate cnpj warns with cached display name`() = runTest(dispatcher) {
        val cached = ServerStation.create(
            stationId = "d6c74c23-63db-4c24-a2e5-408cb23bad26",
            displayName = "Posto Central",
            locationQuality = StationLocationQuality.REVIEWED,
            latitude = -23.55,
            longitude = -46.63,
            cnpjNormalized = "04218406000104",
            municipalityCode = "3550308",
            state = "SP",
            currentRevisionId = null,
        )
        coEvery { catalog.loadPage() } returns ServerStationPage(listOf(cached), null)
        every { submit.invoke(any(), any(), any(), any()) } returns
            SubmitSuggestionOutcome.Submitted("sug-1")

        viewModel.onField("Posto Novo", "3550308", "SP", "04218406000104")
        viewModel.submitProposal()
        advanceUntilIdle()

        assertEquals("Posto Central", viewModel.uiState.value.duplicateHint)
        assertTrue(viewModel.uiState.value.submit is SubmitSuggestionOutcome.Submitted)
    }

    @Test
    fun `offline retry reuses the same key without duplicating`() = runTest(dispatcher) {
        val keys = mutableListOf<String>()
        every { submit.invoke(any(), any(), capture(keys), any()) } returns
            SubmitSuggestionOutcome.Submitted("sug-1")

        viewModel.onField("Posto", "3550308", "SP", "")
        viewModel.submitProposal()
        advanceUntilIdle()
        viewModel.submitProposal()
        advanceUntilIdle()

        assertEquals(2, keys.size)
        assertEquals(keys[0], keys[1])
    }

    @Test
    fun `status and cancel refresh the private list`() = runTest(dispatcher) {
        every { status.invoke(any(), any(), any()) } returns
            SuggestionStatusOutcome.Found("s", "approved")

        viewModel.loadStatus("s")
        advanceUntilIdle()

        val selected = viewModel.uiState.value.selectedStatus
        assertNotNull(selected)
        assertNull(viewModel.uiState.value.submit)
    }
}
