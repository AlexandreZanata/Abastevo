package com.anpfuel.data.di

import com.anpfuel.application.usecase.forum.GetForumCategoriesUseCase
import com.anpfuel.application.usecase.forum.GetForumFeedUseCase
import com.anpfuel.application.usecase.forum.GetForumPostDetailUseCase
import com.anpfuel.application.usecase.forum.ValidateForumSafeLinkUseCase
import com.anpfuel.data.local.dao.ForumCacheDao
import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.data.remote.forum.ForumHttpClient
import com.anpfuel.data.repository.ForumRepository
import com.anpfuel.domain.repository.ForumGateway
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import okhttp3.OkHttpClient
import javax.inject.Named
import javax.inject.Singleton

@Module
@InstallIn(SingletonComponent::class)
object ForumModule {

    @Provides
    @Singleton
    fun provideForumHttpClient(
        okHttpClient: OkHttpClient,
        @Named("apiOrigin") environment: ApiEnvironment,
    ): ForumHttpClient {
        return ForumHttpClient(
            http = okHttpClient,
            baseUrl = environment.origin,
        )
    }

    @Provides
    @Singleton
    fun provideForumGateway(
        client: ForumHttpClient,
        dao: ForumCacheDao,
        @Named("apiOrigin") environment: ApiEnvironment,
    ): ForumGateway {
        return ForumRepository(
            client = client,
            dao = dao,
            baseUrl = environment.origin,
        )
    }

    @Provides
    fun provideGetForumCategoriesUseCase(gateway: ForumGateway): GetForumCategoriesUseCase {
        return GetForumCategoriesUseCase(gateway)
    }

    @Provides
    fun provideGetForumFeedUseCase(gateway: ForumGateway): GetForumFeedUseCase {
        return GetForumFeedUseCase(gateway)
    }

    @Provides
    fun provideGetForumPostDetailUseCase(gateway: ForumGateway): GetForumPostDetailUseCase {
        return GetForumPostDetailUseCase(gateway)
    }

    @Provides
    fun provideValidateForumSafeLinkUseCase(): ValidateForumSafeLinkUseCase {
        return ValidateForumSafeLinkUseCase()
    }
}
