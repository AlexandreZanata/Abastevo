package com.anpfuel.app.ui.enterprise

import com.anpfuel.application.port.NetworkConnectivityGateway
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Named

/** Uses the app's current connectivity source; server proof remains independent. */
@Module
@InstallIn(SingletonComponent::class)
object EnterprisePresentationModule {
    @Provides
    @Named("enterpriseConnectivity")
    fun connectivity(source: NetworkConnectivityGateway): NetworkConnectivityGateway = source
}
