package com.anpfuel.data.di

import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.data.remote.OkHttpClientFactory
import com.anpfuel.data.remote.ledger.LedgerSyncHttpClient
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Named
import javax.inject.Singleton

/**
 * MEX-14 private ledger transport bindings (backend /v1/ledger/commands).
 */
@Module
@InstallIn(SingletonComponent::class)
object LedgerSyncModule {

    @Provides
    @Singleton
    fun provideLedgerSyncHttpClient(
        @Named("apiOrigin") environment: ApiEnvironment,
    ): LedgerSyncHttpClient =
        LedgerSyncHttpClient(OkHttpClientFactory.create(maxRetries = 0), environment.origin)
}
