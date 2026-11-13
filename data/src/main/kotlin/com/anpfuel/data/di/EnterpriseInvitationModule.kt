package com.anpfuel.data.di

import com.anpfuel.application.enterprise.EnterpriseInvitationFlow
import com.anpfuel.application.enterprise.EnterpriseInvitationGateway
import com.anpfuel.application.enterprise.EnterpriseSessionFence
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.data.enterprise.CompanyAccessClient
import com.anpfuel.data.enterprise.EnterpriseInvitationHttpGateway
import com.anpfuel.data.remote.ApiEnvironment
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import java.util.UUID
import java.util.concurrent.TimeUnit
import javax.inject.Named
import javax.inject.Singleton
import okhttp3.OkHttpClient

/** The UI ceremony and cookie transport do not activate protected enterprise persistence. */
@Module
@InstallIn(SingletonComponent::class)
object EnterpriseInvitationModule {
    @Provides @Singleton
    fun companyClient(@Named("apiOrigin") environment: ApiEnvironment, sessions: AuthSessionStore, auth: AuthFlow, fence: EnterpriseSessionFence): CompanyAccessClient =
        CompanyAccessClient(OkHttpClient.Builder().connectTimeout(10, TimeUnit.SECONDS).readTimeout(10, TimeUnit.SECONDS).build(),
            environment.origin, sessions, auth, fence)

    @Provides @Singleton
    fun invitations(client: CompanyAccessClient): EnterpriseInvitationGateway = EnterpriseInvitationHttpGateway(client)

    @Provides
    fun invitationFlow(gateway: EnterpriseInvitationGateway): EnterpriseInvitationFlow = EnterpriseInvitationFlow(gateway) { UUID.randomUUID().toString() }
}
