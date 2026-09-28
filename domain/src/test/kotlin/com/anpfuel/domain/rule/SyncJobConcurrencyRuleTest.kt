package com.anpfuel.domain.rule

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.state.SyncJobState
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.junit.jupiter.params.ParameterizedTest
import org.junit.jupiter.params.provider.EnumSource

class SyncJobConcurrencyRuleTest {

    @Test
    fun idleAllowsNewSync() {
        assertTrue(SyncJobConcurrencyRule.canStartSync(SyncJobState.IDLE))
    }

    @Test
    fun completedAllowsNewSync() {
        assertTrue(SyncJobConcurrencyRule.canStartSync(SyncJobState.COMPLETED))
    }

    @ParameterizedTest
    @EnumSource(
        value = SyncJobState::class,
        names = ["DISCOVERING", "DOWNLOADING", "PARSING", "IMPORTING"],
    )
    fun activeStateRejectsConcurrentSync(activeState: SyncJobState) {
        assertFalse(SyncJobConcurrencyRule.canStartSync(activeState))

        assertThrows(DomainException::class.java) {
            SyncJobConcurrencyRule.validateCanStartSync(activeState)
        }
    }
}
