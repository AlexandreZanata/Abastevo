package com.anpfuel.data.di

import com.anpfuel.application.enterprise.trip.EnterpriseMissionGateway
import com.anpfuel.data.enterprise.CompanyAccessClient
import com.anpfuel.data.enterprise.trip.EnterpriseMissionHttpGateway
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Singleton

/** EMP-TRIP-021: the own mission list on the account cookie plane (EDR-342); replaceable like the checklist gateway. */
@Module
@InstallIn(SingletonComponent::class)
object EnterpriseMissionModule {
    @Provides
    @Singleton
    fun missionGateway(client: CompanyAccessClient): EnterpriseMissionGateway = EnterpriseMissionHttpGateway(client)
}
