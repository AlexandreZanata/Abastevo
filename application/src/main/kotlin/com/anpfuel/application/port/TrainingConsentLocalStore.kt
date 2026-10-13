package com.anpfuel.application.port

import com.anpfuel.domain.model.ContributionScope
import com.anpfuel.domain.repository.TrainingConsentLocalState

/** Atomic, synchronous durable journal, scoped by account+key and exact origin. */
interface TrainingConsentLocalStore {
    fun load(scope: ContributionScope): TrainingConsentLocalState
    fun update(scope: ContributionScope, change: (TrainingConsentLocalState) -> TrainingConsentLocalState): TrainingConsentLocalState
}
