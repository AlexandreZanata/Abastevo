package com.anpfuel.data.di

import com.anpfuel.application.enterprise.tracking.EnterpriseTrackingPolicyGateway
import com.anpfuel.data.enterprise.CompanyAccessClient
import com.anpfuel.data.enterprise.tracking.EnterpriseTrackingPolicyHttpGateway
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Singleton

/** EMP-GPS-002 (B-BR-EMP-495, EDR-321): the driver's policy read on the account cookie plane; replaceable like the context gateway. */
@Module
@InstallIn(SingletonComponent::class)
object EnterpriseTrackingModule {
    @Provides
    @Singleton
    fun trackingPolicyGateway(client: CompanyAccessClient): EnterpriseTrackingPolicyGateway = EnterpriseTrackingPolicyHttpGateway(client)
}
