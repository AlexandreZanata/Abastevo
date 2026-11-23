package com.anpfuel.data.di

import com.anpfuel.application.port.StoreBillingPort
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.usecase.premium.GetPremiumStatusUseCase
import com.anpfuel.application.usecase.premium.HandleStorePurchaseUpdateUseCase
import com.anpfuel.application.usecase.premium.LoadStorePlansUseCase
import com.anpfuel.application.usecase.premium.RecoverStorePurchasesUseCase
import com.anpfuel.application.usecase.premium.RestoreStorePurchasesUseCase
import com.anpfuel.application.usecase.premium.StoreOperationIds
import com.anpfuel.application.usecase.premium.SubmitStorePurchaseUseCase
import com.anpfuel.data.billing.PlayBillingGateway
import com.anpfuel.data.remote.PhotoProofTransport
import com.anpfuel.data.remote.premium.StorePurchaseHttpClient
import com.anpfuel.domain.repository.StorePurchaseGateway
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Singleton

/** Google Play Billing and the signed purchase verification transport (B-BR-STORE-01..10). */
@Module
@InstallIn(SingletonComponent::class)
object StoreBillingModule {

    @Provides
    @Singleton
    fun provideStoreBillingPort(gateway: PlayBillingGateway): StoreBillingPort = gateway

    @Provides
    @Singleton
    fun provideStorePurchaseGateway(proof: PhotoProofTransport, auth: AuthFlow): StorePurchaseGateway =
        StorePurchaseHttpClient.signed(proof, auth)

    @Provides
    @Singleton
    fun provideStoreOperationIds(): StoreOperationIds = StoreOperationIds()

    @Provides
    @Singleton
    fun provideSubmitStorePurchaseUseCase(
        gateway: StorePurchaseGateway,
        sessions: AuthSessionStore,
        ids: StoreOperationIds,
    ): SubmitStorePurchaseUseCase = SubmitStorePurchaseUseCase(gateway, sessions, ids)

    @Provides
    fun provideLoadStorePlansUseCase(store: StoreBillingPort): LoadStorePlansUseCase = LoadStorePlansUseCase(store)

    @Provides
    fun provideHandleStorePurchaseUpdateUseCase(submit: SubmitStorePurchaseUseCase): HandleStorePurchaseUpdateUseCase =
        HandleStorePurchaseUpdateUseCase(submit)

    @Provides
    fun provideRestoreStorePurchasesUseCase(
        store: StoreBillingPort,
        submit: SubmitStorePurchaseUseCase,
    ): RestoreStorePurchasesUseCase = RestoreStorePurchasesUseCase(store, submit)

    @Provides
    fun provideRecoverStorePurchasesUseCase(
        store: StoreBillingPort,
        status: GetPremiumStatusUseCase,
        submit: SubmitStorePurchaseUseCase,
        sessions: AuthSessionStore,
    ): RecoverStorePurchasesUseCase = RecoverStorePurchasesUseCase(store, status, submit, sessions)
}
