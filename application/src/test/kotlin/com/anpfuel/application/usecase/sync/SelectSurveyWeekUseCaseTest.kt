package com.anpfuel.application.usecase.sync

import com.anpfuel.domain.event.PreferencesUpdated
import com.anpfuel.domain.event.SurveyWeekSelected
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.repository.DomainEventPublisher
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.valueobject.SurveyWeek
import com.anpfuel.domain.valueobject.SurveyWeekSelectionMode
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import io.mockk.slot
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

class SelectSurveyWeekUseCaseTest {

    private val userPreferencesRepository = mockk<UserPreferencesRepository>()
    private val eventPublisher = mockk<DomainEventPublisher>()

    private lateinit var useCase: SelectSurveyWeekUseCase

    private val latestWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val historicalWeek = SurveyWeek.fromIsoDates("2026-05-31", "2026-06-06")

    @BeforeEach
    fun setUp() {
        useCase = SelectSurveyWeekUseCase(
            userPreferencesRepository = userPreferencesRepository,
            eventPublisher = eventPublisher,
        )
        coEvery { eventPublisher.publish(any()) } returns Unit
    }

    @Test
    fun persistsActiveSurveyWeekForLatestSelection() = runTest {
        val savedPreferences = slot<UserPreferences>()
        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences(
            preferredMunicipality = "CURITIBA",
        )
        coEvery { userPreferencesRepository.savePreferences(capture(savedPreferences)) } returns Unit

        val result = useCase.invoke(
            surveyWeek = latestWeek,
            selectionMode = SurveyWeekSelectionMode.LATEST,
        )

        assertEquals(latestWeek, savedPreferences.captured.activeSurveyWeek)
        assertEquals("CURITIBA", savedPreferences.captured.preferredMunicipality)
        assertEquals(latestWeek, result.preferences.activeSurveyWeek)
        assertEquals(SurveyWeekSelectionMode.LATEST, result.surveyWeekSelected.payload.selectionMode)
    }

    @Test
    fun persistsActiveSurveyWeekForSpecificSelection() = runTest {
        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences()
        coEvery { userPreferencesRepository.savePreferences(any()) } returns Unit

        val result = useCase.invoke(
            surveyWeek = historicalWeek,
            selectionMode = SurveyWeekSelectionMode.SPECIFIC,
        )

        assertEquals(historicalWeek, result.preferences.activeSurveyWeek)
        assertEquals(SurveyWeekSelectionMode.SPECIFIC, result.surveyWeekSelected.payload.selectionMode)
    }

    @Test
    fun emitsSurveyWeekSelectedAndPreferencesUpdated() = runTest {
        val publishedEvents = mutableListOf<Any>()
        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences()
        coEvery { userPreferencesRepository.savePreferences(any()) } returns Unit
        coEvery { eventPublisher.publish(any()) } answers {
            publishedEvents.add(firstArg())
            Unit
        }

        useCase.invoke(
            surveyWeek = latestWeek,
            selectionMode = SurveyWeekSelectionMode.LATEST,
        )

        assertEquals(2, publishedEvents.size)
        val surveyWeekSelected = publishedEvents.filterIsInstance<SurveyWeekSelected>().single()
        val preferencesUpdated = publishedEvents.filterIsInstance<PreferencesUpdated>().single()

        assertEquals(latestWeek, surveyWeekSelected.payload.surveyWeek)
        assertEquals(
            setOf(SelectSurveyWeekUseCase.KEY_ACTIVE_SURVEY_WEEK),
            preferencesUpdated.payload.changedKeys,
        )
        coVerify(exactly = 2) { eventPublisher.publish(any()) }
    }
}
