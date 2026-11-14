package com.anpfuel.data.di

import com.anpfuel.application.enterprise.EnterpriseContextFlow
import com.anpfuel.application.enterprise.EnterpriseContextGateway
import com.anpfuel.application.enterprise.EnterpriseLocalContext
import com.anpfuel.application.enterprise.EnterpriseContextProtection
import com.anpfuel.application.enterprise.EnterpriseSessionFence
import com.anpfuel.data.enterprise.CompanyAccessClient
import com.anpfuel.data.enterprise.EnterpriseContextHttpGateway
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent

/** Metadata selection consumes only the explicitly bound protection port. */
@Module
@InstallIn(SingletonComponent::class)
object EnterpriseContextModule {
    @Provides
    fun contexts(client: CompanyAccessClient): EnterpriseContextGateway = EnterpriseContextHttpGateway(client)

    @Provides
    fun contextFlow(gateway: EnterpriseContextGateway, localContext: EnterpriseLocalContext,
        protection: EnterpriseContextProtection, fence: EnterpriseSessionFence): EnterpriseContextFlow =
        EnterpriseContextFlow(gateway, localContext, protection, fence)
}
