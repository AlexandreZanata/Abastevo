package com.anpfuel.data.di

import com.anpfuel.application.enterprise.EnterpriseLocalContext
import com.anpfuel.application.enterprise.EnterprisePersistence
import com.anpfuel.application.enterprise.UnavailableEnterprisePersistence
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.data.remote.ApiEnvironment
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Named
import javax.inject.Singleton

/** No DB builder, worker or UI activation before protected-store integration. */
@Module
@InstallIn(SingletonComponent::class)
object EnterpriseModule {
    @Provides
    @Singleton
    fun persistence(): EnterprisePersistence = UnavailableEnterprisePersistence

    @Provides
    @Singleton
    fun localContext(auth: AuthFlow, @Named("apiOrigin") environment: ApiEnvironment, persistence: EnterprisePersistence): EnterpriseLocalContext =
        EnterpriseLocalContext(
            ownerAccount = {
                when (val state = auth.rehydrate()) {
                    is AuthFlow.AuthState.Active -> state.session.accountId
                    is AuthFlow.AuthState.NeedsRefresh -> state.session.accountId
                    else -> null
                }
            },
            serverOrigin = { environment.origin },
            persistence = persistence,
        )
}
