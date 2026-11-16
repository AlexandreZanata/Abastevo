package com.anpfuel.app.ui.enterprise

import com.anpfuel.application.enterprise.EnterpriseAccessError
import com.anpfuel.application.enterprise.EnterpriseAccessResult
import com.anpfuel.application.enterprise.tracking.EnterpriseTrackingPolicyGateway
import com.anpfuel.data.di.EnterpriseTrackingModule
import dagger.Module
import dagger.Provides
import dagger.hilt.components.SingletonComponent
import dagger.hilt.testing.TestInstallIn
import javax.inject.Singleton

/** Synthetic outcome only: no fixture credential can reach an HTTP service; the policy reads as unavailable. */
@Module
@TestInstallIn(components = [SingletonComponent::class], replaces = [EnterpriseTrackingModule::class])
object EnterpriseTrackingDeviceBindings {
    @Provides
    @Singleton
    fun trackingPolicyGateway(): EnterpriseTrackingPolicyGateway =
        EnterpriseTrackingPolicyGateway { _, _ -> EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE) }
}
