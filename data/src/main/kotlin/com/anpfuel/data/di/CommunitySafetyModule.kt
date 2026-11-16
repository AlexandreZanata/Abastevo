package com.anpfuel.data.di

import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.usecase.safety.BlockCommunityUserUseCase
import com.anpfuel.application.usecase.safety.CommunityBlockState
import com.anpfuel.application.usecase.safety.FilterHiddenContentUseCase
import com.anpfuel.application.usecase.safety.ListBlockedUsersUseCase
import com.anpfuel.application.usecase.safety.ReportCommunityUserUseCase
import com.anpfuel.application.usecase.safety.UnblockCommunityUserUseCase
import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.data.remote.PhotoProofTransport
import com.anpfuel.data.remote.safety.CommunitySafetyHttpClient
import com.anpfuel.domain.safety.CommunitySafetyGateway
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Named
import javax.inject.Singleton
import okhttp3.OkHttpClient

/**
 * Community safety bindings (COMMUNITY_SAFETY_CONTRACT): the signed
 * transport shares the Forum session and request-proof wiring, and one
 * process-wide [CommunityBlockState] keyed by the signed account backs
 * every screen (B-BR-SAFE-06).
 */
@Module
@InstallIn(SingletonComponent::class)
object CommunitySafetyModule {

    @Provides
    @Singleton
    fun provideCommunitySafetyGateway(
        okHttpClient: OkHttpClient,
        @Named("apiOrigin") environment: ApiEnvironment,
        sessions: AuthSessionStore,
        auth: AuthFlow,
        proof: PhotoProofTransport,
    ): CommunitySafetyGateway = CommunitySafetyHttpClient(
        http = okHttpClient,
        baseUrl = environment.origin,
        sessions = sessions,
        auth = auth,
        proofHeaders = proof::postProofHeaders,
    )

    @Provides
    @Singleton
    fun provideCommunityBlockState(
        gateway: CommunitySafetyGateway,
        sessions: AuthSessionStore,
    ): CommunityBlockState = CommunityBlockState(gateway) { sessions.load()?.accountId }

    @Provides
    fun provideReportCommunityUserUseCase(gateway: CommunitySafetyGateway) = ReportCommunityUserUseCase(gateway)

    @Provides
    fun provideBlockCommunityUserUseCase(gateway: CommunitySafetyGateway, state: CommunityBlockState) =
        BlockCommunityUserUseCase(gateway, state)

    @Provides
    fun provideUnblockCommunityUserUseCase(gateway: CommunitySafetyGateway, state: CommunityBlockState) =
        UnblockCommunityUserUseCase(gateway, state)

    @Provides
    fun provideListBlockedUsersUseCase(gateway: CommunitySafetyGateway, state: CommunityBlockState) =
        ListBlockedUsersUseCase(gateway, state)

    @Provides
    fun provideFilterHiddenContentUseCase(gateway: CommunitySafetyGateway, state: CommunityBlockState) =
        FilterHiddenContentUseCase(gateway, state)
}
