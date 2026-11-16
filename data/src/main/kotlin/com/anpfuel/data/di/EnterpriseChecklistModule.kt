package com.anpfuel.data.di

import com.anpfuel.application.enterprise.checklist.EnterpriseChecklistGateway
import com.anpfuel.data.enterprise.CompanyAccessClient
import com.anpfuel.data.enterprise.checklist.EnterpriseChecklistHttpGateway
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Singleton

/** EMP-CHECK-002: the applicable read and the run read on the account cookie plane (EDR-283); replaceable like the context gateway. */
@Module
@InstallIn(SingletonComponent::class)
object EnterpriseChecklistModule {
    @Provides
    @Singleton
    fun checklistGateway(client: CompanyAccessClient): EnterpriseChecklistGateway = EnterpriseChecklistHttpGateway(client)
}
