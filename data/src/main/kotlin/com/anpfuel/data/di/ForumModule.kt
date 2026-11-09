package com.anpfuel.data.di

import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.usecase.forum.EditForumCommentUseCase
import com.anpfuel.application.usecase.forum.EditForumTopicUseCase
import com.anpfuel.application.usecase.forum.GetForumCategoriesUseCase
import com.anpfuel.application.usecase.forum.GetForumCommentsUseCase
import com.anpfuel.application.usecase.forum.GetForumFeedUseCase
import com.anpfuel.application.usecase.forum.GetForumPostDetailUseCase
import com.anpfuel.application.usecase.forum.GetForumRepliesUseCase
import com.anpfuel.application.usecase.forum.GetOwnForumCommentIdsUseCase
import com.anpfuel.application.usecase.forum.GetOwnerTopicUseCase
import com.anpfuel.application.usecase.forum.RemoveForumCommentUseCase
import com.anpfuel.application.usecase.forum.RemoveForumTopicUseCase
import com.anpfuel.application.usecase.forum.ReportForumTargetUseCase
import com.anpfuel.application.usecase.forum.SubmitForumCommentUseCase
import com.anpfuel.application.usecase.forum.SubmitForumTopicUseCase
import com.anpfuel.application.usecase.forum.ValidateForumSafeLinkUseCase
import com.anpfuel.data.local.dao.ForumCacheDao
import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.data.remote.PhotoProofTransport
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
        sessions: AuthSessionStore,
        auth: AuthFlow,
        proof: PhotoProofTransport,
    ): ForumHttpClient {
        return ForumHttpClient(
            http = okHttpClient,
            baseUrl = environment.origin,
            sessions = sessions,
            auth = auth,
            proofHeaders = proof::postProofHeaders,
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

    @Provides
    fun provideGetForumCommentsUseCase(gateway: ForumGateway): GetForumCommentsUseCase {
        return GetForumCommentsUseCase(gateway)
    }

    @Provides
    fun provideGetForumRepliesUseCase(gateway: ForumGateway): GetForumRepliesUseCase {
        return GetForumRepliesUseCase(gateway)
    }

    @Provides
    fun provideSubmitForumCommentUseCase(gateway: ForumGateway): SubmitForumCommentUseCase {
        return SubmitForumCommentUseCase(gateway)
    }

    @Provides
    fun provideEditForumCommentUseCase(gateway: ForumGateway): EditForumCommentUseCase {
        return EditForumCommentUseCase(gateway)
    }

    @Provides
    fun provideRemoveForumCommentUseCase(gateway: ForumGateway): RemoveForumCommentUseCase {
        return RemoveForumCommentUseCase(gateway)
    }

    @Provides
    fun provideReportForumTargetUseCase(gateway: ForumGateway): ReportForumTargetUseCase {
        return ReportForumTargetUseCase(gateway)
    }

    @Provides
    fun provideSubmitForumTopicUseCase(gateway: ForumGateway): SubmitForumTopicUseCase {
        return SubmitForumTopicUseCase(gateway)
    }

    @Provides
    fun provideEditForumTopicUseCase(gateway: ForumGateway): EditForumTopicUseCase {
        return EditForumTopicUseCase(gateway)
    }

    @Provides
    fun provideRemoveForumTopicUseCase(gateway: ForumGateway): RemoveForumTopicUseCase {
        return RemoveForumTopicUseCase(gateway)
    }

    @Provides
    fun provideGetOwnerTopicUseCase(gateway: ForumGateway): GetOwnerTopicUseCase {
        return GetOwnerTopicUseCase(gateway)
    }

    @Provides
    fun provideGetOwnForumCommentIdsUseCase(gateway: ForumGateway): GetOwnForumCommentIdsUseCase {
        return GetOwnForumCommentIdsUseCase(gateway)
    }
}
