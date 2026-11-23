package com.anpfuel.data.di

import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.data.remote.OkHttpClientFactory
import com.anpfuel.data.remote.PhotoProofTransport
import com.anpfuel.data.remote.ledger.LedgerSyncHttpClient
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Named
import javax.inject.Singleton

/**
 * MEX-14 private ledger transport bindings (backend `/v1/ledger/*`).
 * Commands and consent are signed by the device key bound to the account
 * ([PhotoProofTransport]); record pages stay plain session reads.
 */
@Module
@InstallIn(SingletonComponent::class)
object LedgerSyncModule {

    @Provides
    @Singleton
    fun provideLedgerSyncHttpClient(
        @Named("apiOrigin") environment: ApiEnvironment,
        proof: PhotoProofTransport,
        auth: AuthFlow,
    ): LedgerSyncHttpClient =
        LedgerSyncHttpClient(
            OkHttpClientFactory.create(maxRetries = 0),
            environment.origin,
            LedgerSyncHttpClient.signed(proof, auth),
        )
}
