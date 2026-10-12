package com.anpfuel.application.usecase.privacy

import com.anpfuel.application.port.TrainingConsentLocalStore

class ResumeTrainingRevocation(private val local:TrainingConsentLocalStore,private val useCase:TrainingConsentUseCase) {
    suspend operator fun invoke(caller:TrainingConsentCaller?) {
        val scope=caller?.scope ?: return
        if(local.load(scope).pending?.grant==false) useCase.syncPending(caller)
    }
}
