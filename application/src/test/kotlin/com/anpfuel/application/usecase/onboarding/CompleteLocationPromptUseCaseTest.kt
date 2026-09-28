package com.anpfuel.application.usecase.onboarding

import com.anpfuel.domain.event.PreferencesUpdated
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.repository.DomainEventPublisher
import com.anpfuel.domain.repository.UserPreferencesRepository
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import io.mockk.slot
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

class CompleteLocationPromptUseCaseTest {

    private val userPreferencesRepository = mockk<UserPreferencesRepository>()
    private val eventPublisher = mockk<DomainEventPublisher>()

    private lateinit var useCase: CompleteLocationPromptUseCase

    @BeforeEach
    fun setUp() {
        useCase = CompleteLocationPromptUseCase(
            userPreferencesRepository = userPreferencesRepository,
            eventPublisher = eventPublisher,
        )
        coEvery { eventPublisher.publish(any()) } returns Unit
    }

    @Test
    fun marksLocationPromptCompletedAndEmitsEvent() = runTest {
        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences()
        val saved = slot<UserPreferences>()
        coEvery { userPreferencesRepository.savePreferences(capture(saved)) } returns Unit

        useCase.invoke()

        assertTrue(saved.captured.locationPromptCompleted)
        val eventSlot = slot<PreferencesUpdated>()
        coVerify(exactly = 1) { eventPublisher.publish(capture(eventSlot)) }
        assertTrue(
            eventSlot.captured.payload.changedKeys.contains(
                CompleteLocationPromptUseCase.KEY_LOCATION_PROMPT_COMPLETED,
            ),
        )
    }

    @Test
    fun isIdempotentWhenAlreadyCompleted() = runTest {
        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences(
            locationPromptCompleted = true,
        )

        useCase.invoke()

        coVerify(exactly = 0) { userPreferencesRepository.savePreferences(any()) }
        coVerify(exactly = 0) { eventPublisher.publish(any()) }
    }
}
