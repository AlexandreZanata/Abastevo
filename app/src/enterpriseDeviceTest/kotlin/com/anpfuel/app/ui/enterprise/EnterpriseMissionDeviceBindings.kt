package com.anpfuel.app.ui.enterprise

import com.anpfuel.application.enterprise.EnterpriseAccessError
import com.anpfuel.application.enterprise.EnterpriseAccessResult
import com.anpfuel.application.enterprise.trip.EnterpriseMissionGateway
import com.anpfuel.data.di.EnterpriseMissionModule
import dagger.Module
import dagger.Provides
import dagger.hilt.components.SingletonComponent
import dagger.hilt.testing.TestInstallIn

/** The validation graph has no company cookie client: the own mission list is unavailable, the protected copy stays usable. */
@Module
@TestInstallIn(components = [SingletonComponent::class], replaces = [EnterpriseMissionModule::class])
object EnterpriseMissionDeviceModule {
    @Provides
    fun gateway(): EnterpriseMissionGateway = EnterpriseMissionGateway { EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE) }
}
