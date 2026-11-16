package com.anpfuel.app.ui.enterprise

import android.content.Context
import com.anpfuel.app.ui.enterprise.tracking.AndroidTrackingServiceLauncher
import com.anpfuel.app.ui.enterprise.tracking.TrackingServiceLauncher
import com.anpfuel.application.enterprise.tracking.TrackingRuntime
import com.anpfuel.application.port.NetworkConnectivityGateway
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.android.qualifiers.ApplicationContext
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

/** EMP-GPS-002: arms the visible start grant and starts the tracking service (EDR-323); separate so screen tests keep it. */
@Module
@InstallIn(SingletonComponent::class)
object EnterpriseTrackingPresentationModule {
    @Provides
    fun trackingLauncher(@ApplicationContext context: Context, runtime: TrackingRuntime): TrackingServiceLauncher =
        AndroidTrackingServiceLauncher(context, runtime)
}
