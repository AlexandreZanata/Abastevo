package com.anpfuel.application.port

import com.anpfuel.domain.model.ContributionScope

fun interface ContributionScopeProvider {
    suspend fun currentScope(): ContributionScope
}
