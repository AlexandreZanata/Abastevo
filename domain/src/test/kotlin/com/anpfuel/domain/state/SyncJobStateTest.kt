package com.anpfuel.domain.state

import com.anpfuel.domain.exception.DomainException
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test
import org.junit.jupiter.params.ParameterizedTest
import org.junit.jupiter.params.provider.Arguments
import org.junit.jupiter.params.provider.MethodSource
import java.util.stream.Stream

class SyncJobStateTest {

    @ParameterizedTest(name = "{0} -> {1} is allowed")
    @MethodSource("validTransitions")
    fun validTransitionsFromUserBusinessLogicDiagram(
        from: SyncJobState,
        to: SyncJobState,
    ) {
        assertEquals(to, from.transitionTo(to))
    }

    @ParameterizedTest(name = "{0} -> {1} is rejected")
    @MethodSource("invalidTransitions")
    fun invalidTransitionThrowsDomainException(
        from: SyncJobState,
        to: SyncJobState,
    ) {
        assertThrows(DomainException::class.java) {
            from.transitionTo(to)
        }
    }

    @Test
    fun completedCanOnlyTransitionToIdle() {
        assertEquals(setOf(SyncJobState.IDLE), SyncJobState.COMPLETED.allowedTransitions())
    }

    @Test
    fun failedCanOnlyTransitionToIdle() {
        assertEquals(setOf(SyncJobState.IDLE), SyncJobState.FAILED.allowedTransitions())
    }

    @Test
    fun terminalStatesCannotTransitionToActiveStates() {
        val activeStates = setOf(
            SyncJobState.DISCOVERING,
            SyncJobState.DOWNLOADING,
            SyncJobState.PARSING,
            SyncJobState.IMPORTING,
        )

        SyncJobState.entries.filter { it.isTerminal }.forEach { terminal ->
            activeStates.forEach { active ->
                assertThrows(DomainException::class.java) {
                    terminal.transitionTo(active)
                }
            }
        }
    }

    @Test
    fun happyPathFromIdleToCompleted() {
        val completed = SyncJobState.IDLE
            .transitionTo(SyncJobState.DISCOVERING)
            .transitionTo(SyncJobState.DOWNLOADING)
            .transitionTo(SyncJobState.PARSING)
            .transitionTo(SyncJobState.IMPORTING)
            .transitionTo(SyncJobState.COMPLETED)
            .transitionTo(SyncJobState.IDLE)

        assertEquals(SyncJobState.IDLE, completed)
    }

    @Test
    fun discoveringNothingNewCompletesWithoutDownload() {
        val idle = SyncJobState.IDLE
            .transitionTo(SyncJobState.DISCOVERING)
            .transitionTo(SyncJobState.COMPLETED)
            .transitionTo(SyncJobState.IDLE)

        assertEquals(SyncJobState.IDLE, idle)
    }

    companion object {
        @JvmStatic
        fun validTransitions(): Stream<Arguments> = Stream.of(
            Arguments.of(SyncJobState.IDLE, SyncJobState.DISCOVERING),
            Arguments.of(SyncJobState.DISCOVERING, SyncJobState.DOWNLOADING),
            Arguments.of(SyncJobState.DISCOVERING, SyncJobState.COMPLETED),
            Arguments.of(SyncJobState.DISCOVERING, SyncJobState.FAILED),
            Arguments.of(SyncJobState.DOWNLOADING, SyncJobState.PARSING),
            Arguments.of(SyncJobState.DOWNLOADING, SyncJobState.FAILED),
            Arguments.of(SyncJobState.PARSING, SyncJobState.IMPORTING),
            Arguments.of(SyncJobState.PARSING, SyncJobState.FAILED),
            Arguments.of(SyncJobState.IMPORTING, SyncJobState.COMPLETED),
            Arguments.of(SyncJobState.IMPORTING, SyncJobState.FAILED),
            Arguments.of(SyncJobState.COMPLETED, SyncJobState.IDLE),
            Arguments.of(SyncJobState.FAILED, SyncJobState.IDLE),
        )

        @JvmStatic
        fun invalidTransitions(): Stream<Arguments> =
            SyncJobState.entries.flatMap { from ->
                SyncJobState.entries
                    .filter { to -> from != to && (from to to) !in VALID_TRANSITION_PAIRS }
                    .map { to -> Arguments.of(from, to) }
            }.stream()

        private val VALID_TRANSITION_PAIRS = setOf(
            SyncJobState.IDLE to SyncJobState.DISCOVERING,
            SyncJobState.DISCOVERING to SyncJobState.DOWNLOADING,
            SyncJobState.DISCOVERING to SyncJobState.COMPLETED,
            SyncJobState.DISCOVERING to SyncJobState.FAILED,
            SyncJobState.DOWNLOADING to SyncJobState.PARSING,
            SyncJobState.DOWNLOADING to SyncJobState.FAILED,
            SyncJobState.PARSING to SyncJobState.IMPORTING,
            SyncJobState.PARSING to SyncJobState.FAILED,
            SyncJobState.IMPORTING to SyncJobState.COMPLETED,
            SyncJobState.IMPORTING to SyncJobState.FAILED,
            SyncJobState.COMPLETED to SyncJobState.IDLE,
            SyncJobState.FAILED to SyncJobState.IDLE,
        )
    }
}
