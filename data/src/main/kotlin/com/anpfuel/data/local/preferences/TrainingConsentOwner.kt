package com.anpfuel.data.local.preferences

import com.anpfuel.application.port.ContributionScopeProvider
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.usecase.privacy.TrainingConsentCaller
import javax.inject.Inject

/** Resolves local account identity, never registration or an anonymous write fallback. */
class TrainingConsentOwner @Inject constructor(private val auth:AuthFlow,private val scopes:ContributionScopeProvider) {
    suspend fun current():TrainingConsentCaller? {
        val id=when(val state=auth.rehydrate()) {
            is AuthFlow.AuthState.Active -> state.session.accountId
            is AuthFlow.AuthState.NeedsRefresh -> state.session.accountId
            else -> return null
        }
        val scope=scopes.currentScope()
        if(!scope.ownerScope.startsWith("account:$id:")) return null
        return TrainingConsentCaller(id,scope)
    }
}
